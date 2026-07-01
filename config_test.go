package envsubt

import (
	"os"
	"testing"

	"github.com/stretchr/testify/assert"
)

// ClientSettings struct for testing
type ClientSettings struct {
	Name    string `json:"name" yaml:"name"`
	Version string `json:"version" yaml:"version"`
	Port    []int  `json:"port,omitempty" yaml:"port,omitempty"`
	Network string `json:"network,omitempty" yaml:"network,omitempty"`
}

func TestLoadConfig(t *testing.T) {
	// Test cases
	tests_c := struct {
		name        string
		yamlContent []byte
		setupEnv    map[string]string
		expected    ClientSettings
		expectError bool
		errorMsg    string
	}{
		name: "Valid YAML with placeholders",
		yamlContent: []byte(`
      name: ${CLIENT_NAME}
      version: v1.2.3
      port:
        - 30303
        - 8545
      network: testnet
      `),
		setupEnv: map[string]string{
			"CLIENT_NAME": "lighthouse",
		},
		expected: ClientSettings{
			Name:    "lighthouse",
			Version: "v1.2.3",
			Port:    []int{30303, 8545},
			Network: "testnet",
		},
		errorMsg: "",
	}

	t.Run(tests_c.name, func(t *testing.T) {
		// Setup file

		// Setup environment variables
		for k, v := range tests_c.setupEnv {
			os.Setenv(k, v)
			defer os.Unsetenv(k)
		}

		// Run LoadConfig
		var config ClientSettings
		err := Unmarshal(tests_c.yamlContent, &config)

		if err != nil {
			assert.Equal(t, tests_c.expected, err)
		} else {
			assert.NoError(t, err)
			assert.Equal(t, tests_c.expected, config)
		}
	})
}

func TestDefaultValues(t *testing.T) {
	t.Run("uses default when env var is unset", func(t *testing.T) {
		os.Unsetenv("UNSET_VAR")
		yml := []byte(`name: ${UNSET_VAR:-fallback}`)
		var config struct {
			Name string `yaml:"name"`
		}
		err := Unmarshal(yml, &config)
		assert.NoError(t, err)
		assert.Equal(t, "fallback", config.Name)
	})

	t.Run("uses env var when set over default", func(t *testing.T) {
		os.Setenv("SET_VAR", "actual")
		defer os.Unsetenv("SET_VAR")
		yml := []byte(`name: ${SET_VAR:-fallback}`)
		var config struct {
			Name string `yaml:"name"`
		}
		err := Unmarshal(yml, &config)
		assert.NoError(t, err)
		assert.Equal(t, "actual", config.Name)
	})

	t.Run("default with empty string", func(t *testing.T) {
		os.Unsetenv("EMPTY_DEFAULT")
		yml := []byte(`name: ${EMPTY_DEFAULT:-}`)
		var config struct {
			Name string `yaml:"name"`
		}
		err := Unmarshal(yml, &config)
		assert.NoError(t, err)
		assert.Equal(t, "", config.Name)
	})

	t.Run("default with special characters", func(t *testing.T) {
		os.Unsetenv("SPECIAL_DEFAULT")
		yml := []byte(`url: ${SPECIAL_DEFAULT:-http://localhost:8080}`)
		var config struct {
			URL string `yaml:"url"`
		}
		err := Unmarshal(yml, &config)
		assert.NoError(t, err)
		assert.Equal(t, "http://localhost:8080", config.URL)
	})
}

func TestRequiredValues(t *testing.T) {
	t.Run("returns error when required var is unset", func(t *testing.T) {
		os.Unsetenv("REQUIRED_VAR")
		yml := []byte(`name: ${REQUIRED_VAR:?}`)
		var config struct {
			Name string `yaml:"name"`
		}
		err := Unmarshal(yml, &config)
		assert.Error(t, err)
		assert.Contains(t, err.Error(), "required environment variable REQUIRED_VAR is not set")
	})

	t.Run("succeeds when required var is set", func(t *testing.T) {
		os.Setenv("REQUIRED_VAR_OK", "value")
		defer os.Unsetenv("REQUIRED_VAR_OK")
		yml := []byte(`name: ${REQUIRED_VAR_OK:?}`)
		var config struct {
			Name string `yaml:"name"`
		}
		err := Unmarshal(yml, &config)
		assert.NoError(t, err)
		assert.Equal(t, "value", config.Name)
	})
}
