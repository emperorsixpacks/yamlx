package yamlx

import (
	"os"
	"strings"

	"gopkg.in/yaml.v3"
)

// Validator is an interface that types can implement to validate themselves
// after unmarshalling. If the target implements Validator, Unmarshal will
// call Validate() automatically after parsing is complete.
type Validator interface {
	Validate() error
}

// Unmarshal takes a YAML byte slice `in` and unmarshals it into the object `o`.
// Processing order:
//  1. Extract $var definitions from raw bytes
//  2. Preprocess !if conditionals
//  3. Parse YAML into AST
//  4. Resolve $var references
//  5. Resolve !include tags
//  6. Resolve ${VAR} env substitution (directly on AST)
//  7. Unmarshal into target struct
//  8. Call Validate() if implemented
func Unmarshal(in []byte, o any) error {
	vars := extractRawVars(in)
	out := preprocessIf(in, vars)

	var doc yaml.Node
	if err := yaml.Unmarshal(out, &doc); err != nil {
		return err
	}

	resolveYamlVarRefs(&doc, vars)

	if err := resolveIncludes(&doc); err != nil {
		return err
	}

	if err := resolveEnvVars(&doc); err != nil {
		return err
	}

	if err := yaml.Unmarshal(nodeToBytes(&doc), o); err != nil {
		return err
	}

	if v, ok := o.(Validator); ok {
		return v.Validate()
	}

	return nil
}

// nodeToBytes marshals a yaml.Node back to bytes for final unmarshalling.
func nodeToBytes(node *yaml.Node) []byte {
	out, err := yaml.Marshal(node)
	if err != nil {
		return nil
	}
	return out
}

// resolveEnvVars walks the yaml.Node tree and resolves ${VAR} placeholders directly.
func resolveEnvVars(node *yaml.Node) error {
	if node == nil {
		return nil
	}

	switch node.Kind {
	case yaml.DocumentNode:
		for _, child := range node.Content {
			if err := resolveEnvVars(child); err != nil {
				return err
			}
		}
	case yaml.MappingNode:
		for i := 0; i < len(node.Content); i += 2 {
			if err := resolveEnvVars(node.Content[i+1]); err != nil {
				return err
			}
		}
	case yaml.SequenceNode:
		for _, child := range node.Content {
			if err := resolveEnvVars(child); err != nil {
				return err
			}
		}
	case yaml.ScalarNode:
		if node.Tag == "!!str" && strings.Contains(node.Value, "${") {
			resolved, err := resolvePlaceHolder(node.Value)
			if err != nil {
				return err
			}
			node.Value = resolved
		}
	}
	return nil
}

// resolvePlaceHolder checks if a string contains a placeholder of the form ${VAR},
// ${VAR:-default}, ${VAR:?}, or ${VAR:|opt1|opt2} and resolves it.
func resolvePlaceHolder(value string) (string, error) {
	if !strings.Contains(value, "${") {
		return value, nil
	}

	result := value
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
				return "", NewRequiredError(varName)
			}
			replacement = envVal
		} else if idx := strings.Index(inner, ":|"); idx != -1 {
			varName := inner[:idx]
			allowed := inner[idx+2:]
			envVal := os.Getenv(varName)
			options := strings.Split(allowed, "|")
			valid := false
			for _, opt := range options {
				if envVal == opt {
					valid = true
					break
				}
			}
			if !valid {
				return "", NewInvalidValueError(varName, envVal, allowed)
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
