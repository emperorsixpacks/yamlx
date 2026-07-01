package envsubt

import (
	"os"
	"path/filepath"
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

func TestCompose(t *testing.T) {
	t.Run("multiple placeholders in one string", func(t *testing.T) {
		os.Setenv("HOST", "example.com")
		os.Setenv("PORT", "8080")
		defer os.Unsetenv("HOST")
		defer os.Unsetenv("PORT")
		yml := []byte(`url: http://${HOST}:${PORT}`)
		var config struct {
			URL string `yaml:"url"`
		}
		err := Unmarshal(yml, &config)
		assert.NoError(t, err)
		assert.Equal(t, "http://example.com:8080", config.URL)
	})

	t.Run("mixed plain, default, and required placeholders", func(t *testing.T) {
		os.Setenv("SCHEME", "https")
		os.Setenv("API_PORT", "443")
		os.Unsetenv("BASE_PATH")
		defer os.Unsetenv("SCHEME")
		defer os.Unsetenv("API_PORT")
		defer os.Unsetenv("BASE_PATH")
		yml := []byte(`endpoint: ${SCHEME}://${HOST:-localhost}:${API_PORT:?}/${BASE_PATH:-v1}`)
		var config struct {
			Endpoint string `yaml:"endpoint"`
		}
		err := Unmarshal(yml, &config)
		assert.NoError(t, err)
		assert.Equal(t, "https://localhost:443/v1", config.Endpoint)
	})
}

func TestIncludes(t *testing.T) {
	tmpDir := t.TempDir()

	writeFile := func(name, content string) {
		t.Helper()
		err := os.WriteFile(filepath.Join(tmpDir, name), []byte(content), 0644)
		assert.NoError(t, err)
	}

	writeFile("network.yaml", `
type: p2p
subnet: 10.0.0.0/24
`)
	writeFile("ports.yaml", `- 30303
- 8545
`)
	writeFile("deep.yaml", `
level: deep
child: !include child.yaml
`)
	writeFile("child.yaml", `
level: child
`)
	writeFile("circular_a.yaml", `next: !include circular_b.yaml
`)
	writeFile("circular_b.yaml", `next: !include circular_a.yaml
`)
	writeFile("env_include.yaml", `greeting: ${HELLO:-hello}
value: world
`)

	origDir, _ := os.Getwd()
	os.Chdir(tmpDir)
	defer os.Chdir(origDir)

	t.Run("basic include", func(t *testing.T) {
		yml := []byte(`name: lighthouse
network: !include network.yaml
`)
		var config struct {
			Name    string `yaml:"name"`
			Network struct {
				Type   string `yaml:"type"`
				Subnet string `yaml:"subnet"`
			} `yaml:"network"`
		}
		err := Unmarshal(yml, &config)
		assert.NoError(t, err)
		assert.Equal(t, "lighthouse", config.Name)
		assert.Equal(t, "p2p", config.Network.Type)
		assert.Equal(t, "10.0.0.0/24", config.Network.Subnet)
	})

	t.Run("include sequence", func(t *testing.T) {
		yml := []byte(`ports: !include ports.yaml
`)
		var config struct {
			Ports []int `yaml:"ports"`
		}
		err := Unmarshal(yml, &config)
		assert.NoError(t, err)
		assert.Equal(t, []int{30303, 8545}, config.Ports)
	})

	t.Run("recursive include", func(t *testing.T) {
		yml := []byte(`config: !include deep.yaml
`)
		var config struct {
			Config struct {
				Level string `yaml:"level"`
				Child struct {
					Level string `yaml:"level"`
				} `yaml:"child"`
			} `yaml:"config"`
		}
		err := Unmarshal(yml, &config)
		assert.NoError(t, err)
		assert.Equal(t, "deep", config.Config.Level)
		assert.Equal(t, "child", config.Config.Child.Level)
	})

	t.Run("env vars in included files", func(t *testing.T) {
		os.Setenv("HELLO", "hi")
		defer os.Unsetenv("HELLO")
		yml := []byte(`data: !include env_include.yaml
`)
		var config struct {
			Data struct {
				Greeting string `yaml:"greeting"`
				Value    string `yaml:"value"`
			} `yaml:"data"`
		}
		err := Unmarshal(yml, &config)
		assert.NoError(t, err)
		assert.Equal(t, "hi", config.Data.Greeting)
		assert.Equal(t, "world", config.Data.Value)
	})

	t.Run("missing file returns error", func(t *testing.T) {
		yml := []byte(`data: !include nonexistent.yaml
`)
		var config struct {
			Data any `yaml:"data"`
		}
		err := Unmarshal(yml, &config)
		assert.Error(t, err)
		assert.Contains(t, err.Error(), "include file not found")
	})

	t.Run("circular include returns error", func(t *testing.T) {
		yml := []byte(`data: !include circular_a.yaml
`)
		var config struct {
			Data any `yaml:"data"`
		}
		err := Unmarshal(yml, &config)
		assert.Error(t, err)
		assert.Contains(t, err.Error(), "circular include detected")
	})
}
