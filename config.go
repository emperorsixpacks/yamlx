// Package yamlx provides YAML unmarshalling with environment variable substitution and !include support.
package yamlx

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"gopkg.in/yaml.v3"
)

const defaultMaxDepth = 10

// Unmarshal takes a YAML byte slice `in` and unmarshals it into the object `o`.
// It supports environment variable substitution for fields using the ${VAR_NAME} format
// and !include tags for loading other YAML files.
func Unmarshal(in []byte, o any) error {
	var doc yaml.Node
	if err := yaml.Unmarshal(in, &doc); err != nil {
		return err
	}

	basePath, _ := os.Getwd()
	seen := make(map[string]bool)
	if err := resolveIncludes(&doc, basePath, seen, 0, defaultMaxDepth); err != nil {
		return err
	}

	out, err := yaml.Marshal(&doc)
	if err != nil {
		return err
	}

	ymlBytes, err := resolveEnvVars(out)
	if err != nil {
		return err
	}

	return yaml.Unmarshal(ymlBytes, o)
}

// validMapping checks if the input is a map[string]any and returns it.
func validMapping(in any) (map[string]any, error) {
	comma, ok := in.(map[string]any)
	if !ok {
		err := fmt.Sprintf("Invalid config: got %T", in)
		return nil, NewConfigError(err)
	}
	return comma, nil
}

// resolveEnvVars converts the input YAML bytes into a map, resolves
// environment variable placeholders, and marshals it back into bytes.
func resolveEnvVars(in []byte) ([]byte, error) {
	var config any
	if err := yaml.Unmarshal(in, &config); err != nil {
		return nil, err
	}
	if err := resolveConfig(&config); err != nil {
		return nil, err
	}
	return yaml.Marshal(config)
}

// resolveConfig recursively walks through the config map and replaces environment variable placeholders.
func resolveConfig(config *any) error {
	MapConfig, err := validMapping(*config)
	if err != nil {
		return err
	}
	for k, v := range MapConfig {
		if MapConfig[k], err = resolveConfigVars(v); err != nil {
			return err
		}
	}
	return nil
}

// resolveConfigVars recursively resolves values in nested maps, replacing strings like ${VAR} with os.Getenv(VAR).
func resolveConfigVars(config any) (any, error) {
	MapConfig, err := validMapping(config)
	if err != nil {
		return resolvePlaceHolder(config)
	}
	for k, v := range MapConfig {
		if value, ok := v.(string); ok {
			resolved, err := resolvePlaceHolder(value)
			if err != nil {
				return nil, err
			}
			MapConfig[k] = resolved
			continue
		}
		if MapConfig[k], err = resolveConfigVars(v); err != nil {
			return nil, err
		}
	}
	return config, nil
}

// resolvePlaceHolder checks if a string contains a placeholder of the form ${VAR},
// ${VAR:-default}, or ${VAR:?error} and replaces it with the appropriate value.
func resolvePlaceHolder(value any) (any, error) {
	strValue, ok := value.(string)
	if !ok {
		return value, nil
	}
	if !strings.Contains(strValue, "${") {
		return value, nil
	}

	result := strValue
	for {
		start := strings.Index(result, "${")
		if start == -1 {
			break
		}
		end := strings.Index(result[start:], "}")
		if end == -1 {
			break
		}
		end += start

		inner := result[start+2 : end]

		var replacement string

		if idx := strings.Index(inner, ":-"); idx != -1 {
			varName := inner[:idx]
			defaultVal := inner[idx+2:]
			envVal := os.Getenv(varName)
			if envVal == "" {
				replacement = defaultVal
			} else {
				replacement = envVal
			}
		} else if idx := strings.Index(inner, ":?"); idx != -1 {
			varName := inner[:idx]
			envVal := os.Getenv(varName)
			if envVal == "" {
				return nil, NewRequiredError(varName)
			}
			replacement = envVal
		} else {
			varName := inner
			replacement = os.Getenv(varName)
		}

		result = result[:start] + replacement + result[end+1:]
	}

	return result, nil
}

// resolveIncludes walks a yaml.Node tree and resolves any nodes tagged with !include.
func resolveIncludes(node *yaml.Node, basePath string, seen map[string]bool, depth, maxDepth int) error {
	if node == nil {
		return nil
	}

	switch node.Kind {
	case yaml.DocumentNode:
		for _, child := range node.Content {
			if err := resolveIncludes(child, basePath, seen, depth, maxDepth); err != nil {
				return err
			}
		}

	case yaml.MappingNode:
		for i := 0; i < len(node.Content); i += 2 {
			val := node.Content[i+1]
			if err := resolveIncludes(val, basePath, seen, depth, maxDepth); err != nil {
				return err
			}
		}

	case yaml.SequenceNode:
		for _, child := range node.Content {
			if err := resolveIncludes(child, basePath, seen, depth, maxDepth); err != nil {
				return err
			}
		}

	case yaml.ScalarNode:
		if node.Tag == "!include" {
			return resolveIncludeNode(node, basePath, seen, depth, maxDepth)
		}
	}

	return nil
}

// resolveIncludeNode replaces a !include node with the content of the referenced file.
// Supports: !include ./file.yaml, !include ./file.yaml:? (required), !include ./file.yaml:-fallback.yaml
func resolveIncludeNode(node *yaml.Node, basePath string, seen map[string]bool, depth, maxDepth int) error {
	if depth >= maxDepth {
		return NewIncludeError(node.Value, "depth")
	}

	rawPath := node.Value
	var incPath, mode, fallback string

	if idx := strings.Index(rawPath, ":-"); idx != -1 {
		incPath = rawPath[:idx]
		mode = "default"
		fallback = rawPath[idx+2:]
	} else if idx := strings.Index(rawPath, ":?"); idx != -1 {
		incPath = rawPath[:idx]
		mode = "required"
	} else {
		incPath = rawPath
	}

	absPath, err := filepath.Abs(filepath.Join(basePath, incPath))
	if err != nil {
		if mode == "default" {
			return resolveIncludeFile(node, fallback, basePath, seen, depth, maxDepth)
		}
		return NewIncludeError(incPath, "not_found")
	}

	if seen[absPath] {
		return NewIncludeError(incPath, "cycle")
	}
	seen[absPath] = true

	data, err := os.ReadFile(absPath)
	if err != nil {
		delete(seen, absPath)
		if mode == "default" {
			return resolveIncludeFile(node, fallback, basePath, seen, depth, maxDepth)
		}
		if mode == "required" {
			return NewIncludeError(incPath, "not_found")
		}
		return NewIncludeError(incPath, "not_found")
	}

	return loadIncludedContent(node, data, absPath, seen)
}

// resolveIncludeFile loads a YAML file by path and replaces the node.
func resolveIncludeFile(node *yaml.Node, filePath, basePath string, seen map[string]bool, depth, maxDepth int) error {
	if depth >= maxDepth {
		return NewIncludeError(filePath, "depth")
	}

	absPath, err := filepath.Abs(filepath.Join(basePath, filePath))
	if err != nil {
		return NewIncludeError(filePath, "not_found")
	}

	if seen[absPath] {
		return NewIncludeError(filePath, "cycle")
	}
	seen[absPath] = true

	data, err := os.ReadFile(absPath)
	if err != nil {
		delete(seen, absPath)
		return NewIncludeError(filePath, "not_found")
	}

	return loadIncludedContent(node, data, absPath, seen)
}

// loadIncludedContent parses YAML data, resolves nested includes, and replaces the node.
func loadIncludedContent(node *yaml.Node, data []byte, absPath string, seen map[string]bool) error {
	var included yaml.Node
	if err := yaml.Unmarshal(data, &included); err != nil {
		return err
	}

	incBase := filepath.Dir(absPath)
	if err := resolveIncludes(&included, incBase, seen, 1, defaultMaxDepth); err != nil {
		return err
	}

	delete(seen, absPath)

	if included.Kind == yaml.DocumentNode && len(included.Content) == 1 {
		*node = *included.Content[0]
	} else {
		*node = included
	}

	return nil
}
