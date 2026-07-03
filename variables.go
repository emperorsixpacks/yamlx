package yamlx

import (
	"fmt"
	"strconv"
	"strings"

	"gopkg.in/yaml.v3"
)

// extractRawVars extracts simple key: value pairs from raw YAML bytes.
// Used before YAML parsing to make variables available for !if conditionals.
func extractRawVars(in []byte) map[string]string {
	vars := make(map[string]string)
	lines := strings.Split(string(in), "\n")
	for _, line := range lines {
		trimmed := strings.TrimSpace(line)
		if trimmed == "" || trimmed[0] == '#' {
			continue
		}
		match := rawVarPattern.FindStringSubmatch(trimmed)
		if match != nil {
			val := strings.TrimSpace(match[2])
			val = strings.Trim(val, "\"'")
			vars[match[1]] = val
		}
	}
	return vars
}

// collectYamlVars walks the AST and collects top-level scalar key-value pairs as variables.
func collectYamlVars(node *yaml.Node) map[string]string {
	vars := make(map[string]string)
	if node == nil || node.Kind != yaml.DocumentNode {
		return vars
	}
	for _, child := range node.Content {
		if child.Kind == yaml.MappingNode {
			for i := 0; i < len(child.Content); i += 2 {
				key := child.Content[i]
				val := child.Content[i+1]
				if key.Kind == yaml.ScalarNode && val.Kind == yaml.ScalarNode {
					vars[key.Value] = val.Value
				}
			}
		}
	}
	return vars
}

// pathVar holds a scalar value accessible by dot-path and the first path segment
// for constraint checking.
type pathVar struct {
	value        string
	firstSegment string
}

// hasDotPathRefs scans the AST for any $a.b.c style references.
func hasDotPathRefs(node *yaml.Node) bool {
	if node == nil {
		return false
	}
	switch node.Kind {
	case yaml.DocumentNode, yaml.MappingNode, yaml.SequenceNode:
		for _, child := range node.Content {
			if hasDotPathRefs(child) {
				return true
			}
		}
	case yaml.ScalarNode:
		if node.Tag != "!!str" {
			return false
		}
		v := node.Value
		for i := 0; i < len(v); i++ {
			if v[i] == '$' {
				if i+1 < len(v) && v[i+1] == '{' {
					continue
				}
				end := i + 1
				sawDot := false
				for end < len(v) {
					c := v[end]
					if c == '.' {
						sawDot = true
						end++
						continue
					}
					if (c >= 'a' && c <= 'z') || (c >= 'A' && c <= 'Z') || (c >= '0' && c <= '9') || c == '_' {
						end++
					} else {
						break
					}
				}
				if sawDot {
					return true
				}
				i = end - 1
			}
		}
	}
	return false
}

// buildPathMap walks the AST and collects all scalar values with their dot-paths.
// Uses a single reusable stack slice to avoid allocations.
func buildPathMap(node *yaml.Node, stack []string, depth int, m map[string]pathVar) []string {
	if node == nil {
		return stack
	}
	switch node.Kind {
	case yaml.DocumentNode:
		for _, child := range node.Content {
			stack = buildPathMap(child, stack, depth, m)
		}
	case yaml.MappingNode:
		for i := 0; i < len(node.Content); i += 2 {
			key := node.Content[i]
			val := node.Content[i+1]
			stack = pushStack(stack, depth, key.Value)

			firstSeg := key.Value
			if depth > 0 {
				firstSeg = stack[0]
			}
			if val.Kind == yaml.ScalarNode {
				path := strings.Join(stack[:depth+1], ".")
				m[path] = pathVar{value: val.Value, firstSegment: firstSeg}
			}
			stack = buildPathMap(val, stack, depth+1, m)
		}
	case yaml.SequenceNode:
		for j, child := range node.Content {
			seg := strconv.Itoa(j)
			stack = pushStack(stack, depth, seg)

			firstSeg := ""
			if depth > 0 {
				firstSeg = stack[0]
			}
			if child.Kind == yaml.ScalarNode {
				path := strings.Join(stack[:depth+1], ".")
				m[path] = pathVar{value: child.Value, firstSegment: firstSeg}
			}
			stack = buildPathMap(child, stack, depth+1, m)
		}
	}
	return stack
}

func pushStack(stack []string, depth int, val string) []string {
	if depth < len(stack) {
		stack[depth] = val
		return stack
	}
	return append(stack, val)
}

// resolveYamlVarRefs walks the AST and replaces $var references with variable values.
// Uses a single reusable stack slice to avoid per-call allocations.
func resolveYamlVarRefs(node *yaml.Node, vars map[string]string, pathVars map[string]pathVar, stack []string, depth int) ([]string, error) {
	if node == nil {
		return stack, nil
	}

	switch node.Kind {
	case yaml.DocumentNode:
		for _, child := range node.Content {
			var err error
			stack, err = resolveYamlVarRefs(child, vars, pathVars, stack, depth)
			if err != nil {
				return stack, err
			}
		}
	case yaml.MappingNode:
		for i := 0; i < len(node.Content); i += 2 {
			key := node.Content[i]
			val := node.Content[i+1]
			stack = pushStack(stack, depth, key.Value)
			var err error
			stack, err = resolveYamlVarRefs(val, vars, pathVars, stack, depth+1)
			if err != nil {
				return stack, err
			}
		}
	case yaml.SequenceNode:
		for j, child := range node.Content {
			stack = pushStack(stack, depth, strconv.Itoa(j))
			var err error
			stack, err = resolveYamlVarRefs(child, vars, pathVars, stack, depth+1)
			if err != nil {
				return stack, err
			}
		}
	case yaml.ScalarNode:
		if node.Tag == "!!str" && strings.Contains(node.Value, "$") {
			resolved, err := replaceYamlVars(node.Value, vars, pathVars, stack[:depth])
			if err != nil {
				return stack, err
			}
			node.Value = resolved
		}
	}
	return stack, nil
}

// replaceYamlVars replaces $var references with variable values, skipping ${...} (env var syntax).
// It also supports dot-path references like $storage.database.port with a depth constraint.
func replaceYamlVars(s string, vars map[string]string, pathVars map[string]pathVar, currentPath []string) (string, error) {
	var result strings.Builder
	result.Grow(len(s))
	i := 0
	for i < len(s) {
		if s[i] == '$' {
			// skip ${...} env var syntax
			if i+1 < len(s) && s[i+1] == '{' {
				closeIdx := strings.Index(s[i:], "}")
				if closeIdx == -1 {
					result.WriteString(s[i:])
					break
				}
				result.WriteString(s[i : i+closeIdx+1])
				i += closeIdx + 1
				continue
			}
			// extract $var name, allowing dots for path references
			end := i + 1
			sawDot := false
			for end < len(s) {
				c := s[end]
				if c == '.' {
					sawDot = true
					end++
					continue
				}
				if (c >= 'a' && c <= 'z') || (c >= 'A' && c <= 'Z') || (c >= '0' && c <= '9') || c == '_' {
					end++
				} else {
					break
				}
			}
			varName := s[i+1 : end]
			if sawDot {
				// dot path reference
				if pv, ok := pathVars[varName]; ok {
					if len(currentPath) > 0 && currentPath[0] == pv.firstSegment {
						return "", fmt.Errorf("invalid reference: cannot use $%s from inside %s", varName, pv.firstSegment)
					}
					result.WriteString(pv.value)
				} else {
					// not found, leave as-is
					result.WriteByte('$')
					result.WriteString(varName)
				}
			} else {
				// simple variable reference
				if val, ok := vars[varName]; ok {
					result.WriteString(val)
				}
			}
			i = end
		} else {
			result.WriteByte(s[i])
			i++
		}
	}
	return result.String(), nil
}
