package yamlx

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

	t.Run("required include errors when file missing", func(t *testing.T) {
		yml := []byte(`data: !include missing.yaml:?
`)
		var config struct {
			Data any `yaml:"data"`
		}
		err := Unmarshal(yml, &config)
		assert.Error(t, err)
		assert.Contains(t, err.Error(), "include file not found")
	})

	t.Run("required include succeeds when file exists", func(t *testing.T) {
		yml := []byte(`data: !include network.yaml:?
`)
		var config struct {
			Data struct {
				Type string `yaml:"type"`
			} `yaml:"data"`
		}
		err := Unmarshal(yml, &config)
		assert.NoError(t, err)
		assert.Equal(t, "p2p", config.Data.Type)
	})

	t.Run("default include uses fallback when file missing", func(t *testing.T) {
		writeFile("fallback.yaml", `source: fallback
value: from_fallback
`)
		yml := []byte(`data: !include nonexistent.yaml:-fallback.yaml
`)
		var config struct {
			Data struct {
				Source string `yaml:"source"`
				Value  string `yaml:"value"`
			} `yaml:"data"`
		}
		err := Unmarshal(yml, &config)
		assert.NoError(t, err)
		assert.Equal(t, "fallback", config.Data.Source)
		assert.Equal(t, "from_fallback", config.Data.Value)
	})

	t.Run("default include uses primary when file exists", func(t *testing.T) {
		yml := []byte(`data: !include network.yaml:-fallback.yaml
`)
		var config struct {
			Data struct {
				Type   string `yaml:"type"`
				Source string `yaml:"source"`
			} `yaml:"data"`
		}
		err := Unmarshal(yml, &config)
		assert.NoError(t, err)
		assert.Equal(t, "p2p", config.Data.Type)
		assert.Equal(t, "", config.Data.Source)
	})

	t.Run("default include also works with env vars in fallback", func(t *testing.T) {
		writeFile("env_fallback.yaml", `greeting: ${HELLO:-hello}
`)
		os.Setenv("HELLO", "hey")
		defer os.Unsetenv("HELLO")
		yml := []byte(`data: !include missing.yaml:-env_fallback.yaml
`)
		var config struct {
			Data struct {
				Greeting string `yaml:"greeting"`
			} `yaml:"data"`
		}
		err := Unmarshal(yml, &config)
		assert.NoError(t, err)
		assert.Equal(t, "hey", config.Data.Greeting)
	})
}

func TestYamlVars(t *testing.T) {
	t.Run("basic $var reference", func(t *testing.T) {
		yml := []byte(`name: hello
greeting: $name world
`)
		var config struct {
			Name     string `yaml:"name"`
			Greeting string `yaml:"greeting"`
		}
		err := Unmarshal(yml, &config)
		assert.NoError(t, err)
		assert.Equal(t, "hello", config.Name)
		assert.Equal(t, "hello world", config.Greeting)
	})

	t.Run("$var defined above is available below", func(t *testing.T) {
		yml := []byte(`env: production
port: $env
`)
		var config struct {
			Env  string `yaml:"env"`
			Port string `yaml:"port"`
		}
		err := Unmarshal(yml, &config)
		assert.NoError(t, err)
		assert.Equal(t, "production", config.Env)
		assert.Equal(t, "production", config.Port)
	})

	t.Run("$var does not replace ${VAR} env syntax", func(t *testing.T) {
		os.Setenv("TEST_HOST", "example.com")
		defer os.Unsetenv("TEST_HOST")
		yml := []byte(`host: ${TEST_HOST}
url: http://$host
`)
		var config struct {
			Host string `yaml:"host"`
			URL  string `yaml:"url"`
		}
		err := Unmarshal(yml, &config)
		assert.NoError(t, err)
		assert.Equal(t, "example.com", config.Host)
		assert.Equal(t, "http://example.com", config.URL)
	})
}

func TestConditionals(t *testing.T) {
	t.Run("basic !if with ==", func(t *testing.T) {
		yml := []byte(`env: production
port: !if "$env" == "production" 443 else 8080
`)
		var config struct {
			Env  string `yaml:"env"`
			Port string `yaml:"port"`
		}
		err := Unmarshal(yml, &config)
		assert.NoError(t, err)
		assert.Equal(t, "production", config.Env)
		assert.Equal(t, "443", config.Port)
	})

	t.Run("basic !if with !=", func(t *testing.T) {
		yml := []byte(`env: dev
port: !if "$env" == "production" 443 else 8080
`)
		var config struct {
			Env  string `yaml:"env"`
			Port string `yaml:"port"`
		}
		err := Unmarshal(yml, &config)
		assert.NoError(t, err)
		assert.Equal(t, "dev", config.Env)
		assert.Equal(t, "8080", config.Port)
	})

	t.Run("!if with env var from OS", func(t *testing.T) {
		os.Setenv("APP_ENV", "staging")
		defer os.Unsetenv("APP_ENV")
		yml := []byte(`log: !if "${APP_ENV}" == "production" warn else debug
`)
		var config struct {
			Log string `yaml:"log"`
		}
		err := Unmarshal(yml, &config)
		assert.NoError(t, err)
		assert.Equal(t, "debug", config.Log)
	})

	t.Run("mixed $var and ${VAR} in condition", func(t *testing.T) {
		os.Setenv("MY_REGION", "us-east-1")
		defer os.Unsetenv("MY_REGION")
		yml := []byte(`region: us-east-1
result: !if "$region" == "${MY_REGION}" match else mismatch
`)
		var config struct {
			Region string `yaml:"region"`
			Result string `yaml:"result"`
		}
		err := Unmarshal(yml, &config)
		assert.NoError(t, err)
		assert.Equal(t, "match", config.Result)
	})
}

func TestEnumValidation(t *testing.T) {
	t.Run("valid value passes", func(t *testing.T) {
		os.Setenv("APP_ENV", "production")
		defer os.Unsetenv("APP_ENV")
		yml := []byte(`env: ${APP_ENV:,production,staging,development}
`)
		var config struct {
			Env string `yaml:"env"`
		}
		err := Unmarshal(yml, &config)
		assert.NoError(t, err)
		assert.Equal(t, "production", config.Env)
	})

	t.Run("invalid value errors", func(t *testing.T) {
		os.Setenv("APP_ENV", "invalid")
		defer os.Unsetenv("APP_ENV")
		yml := []byte(`env: ${APP_ENV:,production,staging,development}
`)
		var config struct {
			Env string `yaml:"env"`
		}
		err := Unmarshal(yml, &config)
		assert.Error(t, err)
		assert.Contains(t, err.Error(), "invalid value")
		assert.Contains(t, err.Error(), "must be one of")
	})

	t.Run("empty value errors", func(t *testing.T) {
		os.Unsetenv("APP_ENV")
		yml := []byte(`env: ${APP_ENV:,production,staging,development}
`)
		var config struct {
			Env string `yaml:"env"`
		}
		err := Unmarshal(yml, &config)
		assert.Error(t, err)
		assert.Contains(t, err.Error(), "invalid value")
	})

	t.Run("single allowed value", func(t *testing.T) {
		os.Setenv("MODE", "prod")
		defer os.Unsetenv("MODE")
		yml := []byte(`mode: ${MODE:,prod}
`)
		var config struct {
			Mode string `yaml:"mode"`
		}
		err := Unmarshal(yml, &config)
		assert.NoError(t, err)
		assert.Equal(t, "prod", config.Mode)
	})
}

func TestTiming(t *testing.T) {
	yml := []byte(`
name: test
port: 8080
`)
	var config struct {
		Name string `yaml:"name"`
		Port int    `yaml:"port"`
	}
	timing, err := UnmarshalWithTiming(yml, &config)
	assert.NoError(t, err)
	assert.Equal(t, "test", config.Name)
	assert.Equal(t, 8080, config.Port)
	assert.True(t, timing.Total > 0, "total time should be > 0")
	assert.True(t, timing.YAMLParse > 0, "yaml parse time should be > 0")
	assert.True(t, timing.FinalParse > 0, "final parse time should be > 0")
}

type validConfig struct {
	Name string `yaml:"name"`
	Port int    `yaml:"port"`
}

func (c validConfig) Validate() error {
	if c.Name == "" {
		return NewConfigError("name is required")
	}
	if c.Port < 1 || c.Port > 65535 {
		return NewConfigError("port must be between 1 and 65535")
	}
	return nil
}

type noValidatorConfig struct {
	Name string `yaml:"name"`
}

func TestValidator(t *testing.T) {
	t.Run("valid config passes", func(t *testing.T) {
		yml := []byte(`name: myapp
port: 8080
`)
		var cfg validConfig
		err := Unmarshal(yml, &cfg)
		assert.NoError(t, err)
		assert.Equal(t, "myapp", cfg.Name)
		assert.Equal(t, 8080, cfg.Port)
	})

	t.Run("empty name fails validation", func(t *testing.T) {
		yml := []byte(`name: ""
port: 8080
`)
		var cfg validConfig
		err := Unmarshal(yml, &cfg)
		assert.Error(t, err)
		assert.Contains(t, err.Error(), "name is required")
	})

	t.Run("invalid port fails validation", func(t *testing.T) {
		yml := []byte(`name: myapp
port: 99999
`)
		var cfg validConfig
		err := Unmarshal(yml, &cfg)
		assert.Error(t, err)
		assert.Contains(t, err.Error(), "port must be between 1 and 65535")
	})

	t.Run("struct without Validate skips validation", func(t *testing.T) {
		yml := []byte(`name: myapp
`)
		var cfg noValidatorConfig
		err := Unmarshal(yml, &cfg)
		assert.NoError(t, err)
		assert.Equal(t, "myapp", cfg.Name)
	})

	t.Run("validator runs with env vars resolved", func(t *testing.T) {
		os.Setenv("APP_NAME", "prod-app")
		defer os.Unsetenv("APP_NAME")
		yml := []byte(`name: ${APP_NAME}
port: 443
`)
		var cfg validConfig
		err := Unmarshal(yml, &cfg)
		assert.NoError(t, err)
		assert.Equal(t, "prod-app", cfg.Name)
	})

	t.Run("validator runs after includes resolved", func(t *testing.T) {
		tmpDir := t.TempDir()
		os.WriteFile(tmpDir+"/base.yaml", []byte(`name: included-app
port: 3000
`), 0644)
		origDir, _ := os.Getwd()
		os.Chdir(tmpDir)
		defer os.Chdir(origDir)

		yml := []byte(`name: ${APP_NAME:-default}
port: 8080
`)
		var cfg validConfig
		err := Unmarshal(yml, &cfg)
		assert.NoError(t, err)
		assert.Equal(t, "default", cfg.Name)
	})
}
