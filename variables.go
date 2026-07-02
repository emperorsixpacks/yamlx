package yamlx

import (
	"fmt"
	"regexp"
	"strconv"
	"strings"

	"gopkg.in/yaml.v3"
)

var rawVarPattern = regexp.MustCompile(`^(\w+)\s*:\s*(.+)$`)

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

// buildPathMap walks the AST and collects all scalar values with their dot-paths.
func buildPathMap(node *yaml.Node, prefix string, firstSeg string, m map[string]pathVar) {
	if node == nil {
		return
	}
	switch node.Kind {
	case yaml.DocumentNode:
		for _, child := range node.Content {
			buildPathMap(child, "", "", m)
		}
	case yaml.MappingNode:
		for i := 0; i < len(node.Content); i += 2 {
			key := node.Content[i]
			val := node.Content[i+1]
			path := key.Value
			if prefix != "" {
				path = prefix + "." + key.Value
			}
			seg := firstSeg
			if seg == "" {
				seg = key.Value
			}
			if val.Kind == yaml.ScalarNode {
				m[path] = pathVar{value: val.Value, firstSegment: seg}
			}
			buildPathMap(val, path, seg, m)
		}
	case yaml.SequenceNode:
		for j, child := range node.Content {
			path := prefix + "." + strconv.Itoa(j)
			if child.Kind == yaml.ScalarNode {
				m[path] = pathVar{value: child.Value, firstSegment: firstSeg}
			}
			buildPathMap(child, path, firstSeg, m)
		}
	}
}

// resolveYamlVarRefs walks the AST and replaces $var references (not ${VAR}) with variable values.
// currentPath tracks the ancestor mapping keys to enforce that dot-path references cannot be
// used from inside the target root's subtree.
func resolveYamlVarRefs(node *yaml.Node, vars map[string]string, pathVars map[string]pathVar, currentPath []string) error {
	if node == nil {
		return nil
	}

	switch node.Kind {
	case yaml.DocumentNode:
		for _, child := range node.Content {
			if err := resolveYamlVarRefs(child, vars, pathVars, currentPath); err != nil {
				return err
			}
		}
	case yaml.MappingNode:
		for i := 0; i < len(node.Content); i += 2 {
			key := node.Content[i]
			val := node.Content[i+1]
			newPath := append(currentPath, key.Value)
			if err := resolveYamlVarRefs(val, vars, pathVars, newPath); err != nil {
				return err
			}
		}
	case yaml.SequenceNode:
		for j, child := range node.Content {
			newPath := append(currentPath, strconv.Itoa(j))
			if err := resolveYamlVarRefs(child, vars, pathVars, newPath); err != nil {
				return err
			}
		}
	case yaml.ScalarNode:
		if node.Tag == "!!str" && strings.Contains(node.Value, "$") {
			resolved, err := replaceYamlVars(node.Value, vars, pathVars, currentPath)
			if err != nil {
				return err
			}
			node.Value = resolved
		}
	}
	return nil
}

// replaceYamlVars replaces $var references with variable values, skipping ${...} (env var syntax).
// It also supports dot-path references like $storage.database.port with a depth constraint:
// the reference must not be used from inside the subtree of the path's first segment.
func replaceYamlVars(s string, vars map[string]string, pathVars map[string]pathVar, currentPath []string) (string, error) {
	var result strings.Builder
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
			for end < len(s) {
				c := s[end]
				if (c >= 'a' && c <= 'z') || (c >= 'A' && c <= 'Z') || (c >= '0' && c <= '9') || c == '_' || c == '.' {
					end++
				} else {
					break
				}
			}
			varName := s[i+1 : end]
			if strings.Contains(varName, ".") {
				// dot path reference
				if pv, ok := pathVars[varName]; ok {
					if len(currentPath) > 0 && currentPath[0] == pv.firstSegment {
						return "", fmt.Errorf("invalid reference: cannot use $%s from inside %s", varName, pv.firstSegment)
					}
					result.WriteString(pv.value)
				} else {
					// if not found, leave as-is (same behavior as simple $var)
					result.WriteString("$")
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
