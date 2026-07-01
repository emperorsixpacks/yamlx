package yamlx

import (
	"fmt"
	"os"
	"strings"

	"gopkg.in/yaml.v3"
)

// Unmarshal takes a YAML byte slice `in` and unmarshals it into the object `o`.
// Processing order:
//  1. Collect $var definitions from YAML keys (top to bottom)
//  2. Resolve $var references
//  3. Resolve !if conditionals
//  4. Resolve !include tags
//  5. Resolve ${VAR} environment variable substitution
//  6. Unmarshal into target struct
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

	marshalled, err := yaml.Marshal(&doc)
	if err != nil {
		return err
	}

	ymlBytes, err := resolveEnvVars(marshalled)
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
				return nil, NewInvalidValueError(varName, envVal, allowed)
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
