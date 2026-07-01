package yamlx

import (
	"regexp"
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

// resolveYamlVarRefs walks the AST and replaces $var references (not ${VAR}) with variable values.
func resolveYamlVarRefs(node *yaml.Node, vars map[string]string) {
	if node == nil {
		return
	}

	switch node.Kind {
	case yaml.DocumentNode:
		for _, child := range node.Content {
			resolveYamlVarRefs(child, vars)
		}
	case yaml.MappingNode:
		for i := 0; i < len(node.Content); i += 2 {
			resolveYamlVarRefs(node.Content[i+1], vars)
		}
	case yaml.SequenceNode:
		for _, child := range node.Content {
			resolveYamlVarRefs(child, vars)
		}
	case yaml.ScalarNode:
		if node.Tag == "!!str" && strings.Contains(node.Value, "$") {
			node.Value = replaceYamlVars(node.Value, vars)
		}
	}
}

// replaceYamlVars replaces $var references with variable values, skipping ${...} (env var syntax).
func replaceYamlVars(s string, vars map[string]string) string {
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
			// extract $var name
			end := i + 1
			for end < len(s) {
				c := s[end]
				if (c >= 'a' && c <= 'z') || (c >= 'A' && c <= 'Z') || (c >= '0' && c <= '9') || c == '_' {
					end++
				} else {
					break
				}
			}
			varName := s[i+1 : end]
			if val, ok := vars[varName]; ok {
				result.WriteString(val)
			}
			i = end
		} else {
			result.WriteByte(s[i])
			i++
		}
	}
	return result.String()
}
