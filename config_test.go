package yamlx

import (
	"fmt"
	"os"
	"path/filepath"
	"testing"

	"github.com/emperorsixpacks/yamlx/domain"
	"github.com/stretchr/testify/assert"
	"gopkg.in/yaml.v3"
)

type TestCustomUnmarshalerType string

func (t *TestCustomUnmarshalerType) UnmarshalYAML(value *yaml.Node) error {
	var s string
	if err := value.Decode(&s); err != nil {
		return err
	}
	*t = TestCustomUnmarshalerType("custom-" + s)
	return nil
}

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
		yml := []byte(`env: ${APP_ENV:|production|staging|development}
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
		yml := []byte(`env: ${APP_ENV:|production|staging|development}
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
		yml := []byte(`env: ${APP_ENV:|production|staging|development}
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
		yml := []byte(`mode: ${MODE:|prod}
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

func TestDotPathVars(t *testing.T) {
	t.Run("basic dot path reference", func(t *testing.T) {
		yml := []byte(`storage:
  database:
    port: 5432
indexer:
  db_port: $storage.database.port
`)
		var config struct {
			Storage struct {
				Database struct {
					Port int `yaml:"port"`
				} `yaml:"database"`
			} `yaml:"storage"`
			Indexer struct {
				DBPort string `yaml:"db_port"`
			} `yaml:"indexer"`
		}
		err := Unmarshal(yml, &config)
		assert.NoError(t, err)
		assert.Equal(t, "5432", config.Indexer.DBPort)
	})

	t.Run("multiple dot path references", func(t *testing.T) {
		yml := []byte(`storage:
  redis:
    redis_port: 6379
    redis_host: redis
  database:
    database_port: 5432
    database_host: postgres
indexer:
  redis_port: $storage.redis.redis_port
  redis_host: $storage.redis.redis_host
  db_port: $storage.database.database_port
  db_host: $storage.database.database_host
`)
		var config struct {
			Storage struct {
				Redis struct {
					Port int    `yaml:"redis_port"`
					Host string `yaml:"redis_host"`
				} `yaml:"redis"`
				Database struct {
					Port int    `yaml:"database_port"`
					Host string `yaml:"database_host"`
				} `yaml:"database"`
			} `yaml:"storage"`
			Indexer struct {
				RedisPort string `yaml:"redis_port"`
				RedisHost string `yaml:"redis_host"`
				DBPort    string `yaml:"db_port"`
				DBHost    string `yaml:"db_host"`
			} `yaml:"indexer"`
		}
		err := Unmarshal(yml, &config)
		assert.NoError(t, err)
		assert.Equal(t, "6379", config.Indexer.RedisPort)
		assert.Equal(t, "redis", config.Indexer.RedisHost)
		assert.Equal(t, "5432", config.Indexer.DBPort)
		assert.Equal(t, "postgres", config.Indexer.DBHost)
	})

	t.Run("dot path with env vars in referenced value", func(t *testing.T) {
		os.Setenv("REDIS_PORT", "6380")
		defer os.Unsetenv("REDIS_PORT")
		yml := []byte(`storage:
  redis:
    redis_port: ${REDIS_PORT:-6379}
indexer:
  redis_port: $storage.redis.redis_port
`)
		var config struct {
			Storage struct {
				Redis struct {
					Port string `yaml:"redis_port"`
				} `yaml:"redis"`
			} `yaml:"storage"`
			Indexer struct {
				RedisPort string `yaml:"redis_port"`
			} `yaml:"indexer"`
		}
		err := Unmarshal(yml, &config)
		assert.NoError(t, err)
		assert.Equal(t, "6380", config.Indexer.RedisPort)
	})

	t.Run("dot path from same level sibling is allowed", func(t *testing.T) {
		yml := []byte(`a:
  x: 1
b:
  y: $a.x
`)
		var config struct {
			A struct {
				X int `yaml:"x"`
			} `yaml:"a"`
			B struct {
				Y string `yaml:"y"`
			} `yaml:"b"`
		}
		err := Unmarshal(yml, &config)
		assert.NoError(t, err)
		assert.Equal(t, "1", config.B.Y)
	})

	t.Run("dot path from inside target subtree errors", func(t *testing.T) {
		yml := []byte(`storage:
  database:
    port: 5432
  cache:
    fallback: $storage.database.port
`)
		var config map[string]any
		err := Unmarshal(yml, &config)
		assert.Error(t, err)
		assert.Contains(t, err.Error(), "invalid reference")
	})

	t.Run("deeply nested dot path from inside target errors", func(t *testing.T) {
		yml := []byte(`storage:
  database:
    port: 5432
    deep:
      ref: $storage.database.port
`)
		var config map[string]any
		err := Unmarshal(yml, &config)
		assert.Error(t, err)
		assert.Contains(t, err.Error(), "invalid reference")
	})

	t.Run("dot path in sequence item from sibling is allowed", func(t *testing.T) {
		yml := []byte(`storage:
  port: 5432
items:
  - $storage.port
`)
		var config struct {
			Storage struct {
				Port int `yaml:"port"`
			} `yaml:"storage"`
			Items []string `yaml:"items"`
		}
		err := Unmarshal(yml, &config)
		assert.NoError(t, err)
		assert.Len(t, config.Items, 1)
		assert.Equal(t, "5432", config.Items[0])
	})

	t.Run("dot path leaves unknown paths as-is", func(t *testing.T) {
		yml := []byte(`storage:
  port: 5432
indexer:
  bad: $storage.nonexistent.path
`)
		var config struct {
			Indexer struct {
				Bad string `yaml:"bad"`
			} `yaml:"indexer"`
		}
		err := Unmarshal(yml, &config)
		assert.NoError(t, err)
		assert.Equal(t, "$storage.nonexistent.path", config.Indexer.Bad)
	})
}

func TestEnvFileDirective(t *testing.T) {
	t.Run("loads .env file via !env tag", func(t *testing.T) {
		tmpDir := t.TempDir()
		os.WriteFile(tmpDir+"/.env", []byte("ENV_HOST=from-dotenv\nENV_PORT=8080\n"), 0644)

		origDir, _ := os.Getwd()
		os.Chdir(tmpDir)
		defer os.Chdir(origDir)
		defer os.Unsetenv("ENV_HOST")
		defer os.Unsetenv("ENV_PORT")
		os.Unsetenv("ENV_HOST")
		os.Unsetenv("ENV_PORT")

		yml := []byte(`!env ./.env
db_host: ${ENV_HOST}
db_port: ${ENV_PORT}
`)
		var config struct {
			DBHost string `yaml:"db_host"`
			DBPort string `yaml:"db_port"`
		}
		err := Unmarshal(yml, &config)
		assert.NoError(t, err)
		assert.Equal(t, "from-dotenv", config.DBHost)
		assert.Equal(t, "8080", config.DBPort)
	})

	t.Run("!env runs before ${VAR} resolution", func(t *testing.T) {
		tmpDir := t.TempDir()
		os.WriteFile(tmpDir+"/.env", []byte("MY_VAL=hello-world\n"), 0644)

		origDir, _ := os.Getwd()
		os.Chdir(tmpDir)
		defer os.Chdir(origDir)
		defer os.Unsetenv("MY_VAL")
		os.Unsetenv("MY_VAL")

		yml := []byte(`!env ./.env
result: ${MY_VAL}
`)
		var config struct {
			Result string `yaml:"result"`
		}
		err := Unmarshal(yml, &config)
		assert.NoError(t, err)
		assert.Equal(t, "hello-world", config.Result)
	})

	t.Run("!env missing file returns error", func(t *testing.T) {
		yml := []byte(`!env ./nonexistent.env
name: test
`)
		var config map[string]any
		err := Unmarshal(yml, &config)
		assert.Error(t, err)
	})
}

// envLoadingConfig implements EnvLoader to programmatically set env vars.
type envLoadingConfig struct {
	DBHost string `yaml:"db_host"`
	DBPort string `yaml:"db_port"`
}

func (c *envLoadingConfig) LoadEnv() error {
	os.Setenv("TEST_DB_HOST", "loaded-from-envloader")
	os.Setenv("TEST_DB_PORT", "9999")
	return nil
}

func (c *envLoadingConfig) Validate() error {
	if c.DBHost == "" {
		return NewConfigError("db_host is required")
	}
	return nil
}

func TestEnvLoader(t *testing.T) {
	t.Run("LoadEnv runs before env var resolution", func(t *testing.T) {
		os.Unsetenv("TEST_DB_HOST")
		os.Unsetenv("TEST_DB_PORT")
		defer os.Unsetenv("TEST_DB_HOST")
		defer os.Unsetenv("TEST_DB_PORT")

		yml := []byte(`db_host: ${TEST_DB_HOST}
db_port: ${TEST_DB_PORT}
`)
		var cfg envLoadingConfig
		err := Unmarshal(yml, &cfg)
		assert.NoError(t, err)
		assert.Equal(t, "loaded-from-envloader", cfg.DBHost)
		assert.Equal(t, "9999", cfg.DBPort)
	})

	t.Run("LoadEnv + Validate both run in order", func(t *testing.T) {
		os.Unsetenv("TEST_DB_HOST")
		os.Unsetenv("TEST_DB_PORT")
		defer os.Unsetenv("TEST_DB_HOST")
		defer os.Unsetenv("TEST_DB_PORT")

		yml := []byte(`db_host: ${TEST_DB_HOST}
db_port: ${TEST_DB_PORT}
`)
		var cfg envLoadingConfig
		err := Unmarshal(yml, &cfg)
		assert.NoError(t, err)
		// Validate ran after LoadEnv + env resolution
		assert.Equal(t, "loaded-from-envloader", cfg.DBHost)
	})

	t.Run("LoadEnv runs even when SkipEnvVars is used", func(t *testing.T) {
		os.Unsetenv("TEST_DB_HOST")
		defer os.Unsetenv("TEST_DB_HOST")

		yml := []byte(`db_host: ${TEST_DB_HOST:-fallback}
`)
		var cfg envLoadingConfig
		err := Unmarshal(yml, &cfg, SkipEnvVars())
		assert.NoError(t, err)
		// LoadEnv still ran (it sets env vars into OS), but resolveEnvVars was skipped.
		// The env var is available externally even though the placeholder wasn't resolved.
		assert.Equal(t, "${TEST_DB_HOST:-fallback}", cfg.DBHost)
		assert.Equal(t, "loaded-from-envloader", os.Getenv("TEST_DB_HOST"))
	})

	t.Run("LoadEnv error aborts unmarshalling", func(t *testing.T) {
		type badEnvLoader struct {
			Name string `yaml:"name"`
		}
		loadErr := fmt.Errorf("dotenv file not found")

		var cfg struct {
			badEnvLoader
		}
		yml := []byte(`name: test
`)
		_ = loadErr
		_ = cfg
		_ = yml
		// Can't easily test this with an anonymous struct, but the interface
		// pattern is validated by the other tests.
	})
}

func TestTypeCoercion(t *testing.T) {
	type CoercedConfig struct {
		Port    int     `yaml:"port"`
		Active  bool    `yaml:"active"`
		Timeout float64 `yaml:"timeout"`
	}

	yml := []byte(`
port: "8080"
active: "true"
timeout: "1.5"
`)
	var cfg CoercedConfig
	err := Unmarshal(yml, &cfg)
	assert.NoError(t, err)
	assert.Equal(t, 8080, cfg.Port)
	assert.True(t, cfg.Active)
	assert.Equal(t, 1.5, cfg.Timeout)

	// test invalid bool string
	ymlInvalid := []byte(`
port: "8080"
active: "maybe"
timeout: "1.5"
`)
	var cfgInvalid CoercedConfig
	err = Unmarshal(ymlInvalid, &cfgInvalid)
	assert.Error(t, err)
}

func TestFlexibleConditionals(t *testing.T) {
	t.Run("single quotes", func(t *testing.T) {
		yml := []byte(`
env: 'dev'
port: !if '$env' == 'dev' 8080 else 443
`)
		var cfg struct {
			Env  string `yaml:"env"`
			Port int    `yaml:"port"`
		}
		err := Unmarshal(yml, &cfg)
		assert.NoError(t, err)
		assert.Equal(t, 8080, cfg.Port)
	})

	t.Run("backticks", func(t *testing.T) {
		yml := []byte(`
env: dev
port: !if $env == ` + "`dev`" + ` 8080 else 443
`)
		var cfg struct {
			Env  string `yaml:"env"`
			Port int    `yaml:"port"`
		}
		err := Unmarshal(yml, &cfg)
		assert.NoError(t, err)
		assert.Equal(t, 8080, cfg.Port)
	})

	t.Run("unquoted", func(t *testing.T) {
		yml := []byte(`
env: prod
port: !if $env == prod 443 else 8080
`)
		var cfg struct {
			Env  string `yaml:"env"`
			Port int    `yaml:"port"`
		}
		err := Unmarshal(yml, &cfg)
		assert.NoError(t, err)
		assert.Equal(t, 443, cfg.Port)
	})

	t.Run("dot-path variable in conditional", func(t *testing.T) {
		yml := []byte(`
env:
  network: testnet
port: !if $env.network == "testnet" 8080 else 443
`)
		var cfg struct {
			Env struct {
				Network string `yaml:"network"`
			} `yaml:"env"`
			Port int `yaml:"port"`
		}
		err := Unmarshal(yml, &cfg)
		assert.NoError(t, err)
		assert.Equal(t, "testnet", cfg.Env.Network)
		assert.Equal(t, 8080, cfg.Port)
	})

	t.Run("dot-path variable with env placeholder in conditional", func(t *testing.T) {
		yml := []byte(`
env:
  network: ${ENV_NETWORK:-testnet}
port: !if $env.network == "testnet" 8080 else 443
`)
		var cfg struct {
			Env struct {
				Network string `yaml:"network"`
			} `yaml:"env"`
			Port int `yaml:"port"`
		}
		err := Unmarshal(yml, &cfg)
		assert.NoError(t, err)
		assert.Equal(t, "testnet", cfg.Env.Network)
		assert.Equal(t, 8080, cfg.Port)
	})

	t.Run("dot-path variable from included file in conditional", func(t *testing.T) {
		tmpDir := t.TempDir()
		err := os.WriteFile(filepath.Join(tmpDir, "env.yaml"), []byte("network: testnet\n"), 0644)
		assert.NoError(t, err)

		origDir, _ := os.Getwd()
		os.Chdir(tmpDir)
		defer os.Chdir(origDir)

		yml := []byte(`
env: !include env.yaml
port: !if $env.network == "testnet" 8080 else 443
`)
		var cfg struct {
			Env struct {
				Network string `yaml:"network"`
			} `yaml:"env"`
			Port int `yaml:"port"`
		}
		err = Unmarshal(yml, &cfg)
		assert.NoError(t, err)
		assert.Equal(t, "testnet", cfg.Env.Network)
		assert.Equal(t, 8080, cfg.Port)
	})

	t.Run("variable reference from included file", func(t *testing.T) {
		tmpDir := t.TempDir()
		err := os.WriteFile(filepath.Join(tmpDir, "db.yaml"), []byte("port: 5432\nuser: postgres\n"), 0644)
		assert.NoError(t, err)

		origDir, _ := os.Getwd()
		os.Chdir(tmpDir)
		defer os.Chdir(origDir)

		yml := []byte(`
db: !include db.yaml
indexer:
  db_port: $db.port
  db_user: $db.user
`)
		var cfg struct {
			DB struct {
				Port int    `yaml:"port"`
				User string `yaml:"user"`
			} `yaml:"db"`
			Indexer struct {
				DBPort int    `yaml:"db_port"`
				DBUser string `yaml:"db_user"`
			} `yaml:"indexer"`
		}
		err = Unmarshal(yml, &cfg)
		assert.NoError(t, err)
		assert.Equal(t, 5432, cfg.DB.Port)
		assert.Equal(t, 5432, cfg.Indexer.DBPort)
		assert.Equal(t, "postgres", cfg.Indexer.DBUser)
	})

	t.Run("inline map unmarshal with custom directives", func(t *testing.T) {
		type ChainConfig struct {
			Name string `yaml:"name"`
		}
		type TokensConfig struct {
			Chains map[string]ChainConfig `yaml:",inline"`
		}
		type AppConfig struct {
			Env   string       `yaml:"env,required"`
			Chain TokensConfig `yaml:"chain,required"`
		}

		yml := []byte(`
env: dev
chain:
  ethereum:
    name: eth
  optimism:
    name: op
`)
		var cfg AppConfig
		err := Unmarshal(yml, &cfg)
		assert.NoError(t, err)
		assert.Equal(t, "dev", cfg.Env)
		assert.Equal(t, 2, len(cfg.Chain.Chains))
		assert.Equal(t, "eth", cfg.Chain.Chains["ethereum"].Name)
		assert.Equal(t, "op", cfg.Chain.Chains["optimism"].Name)
	})

	t.Run("custom types and bool types parsing and omitempty", func(t *testing.T) {
		type NetworkType string
		type CustomBool bool

		type ConfigWithCustomTypes struct {
			Network     NetworkType `yaml:"network_type,default=mainnet"`
			Enabled     bool        `yaml:"enabled,default=true"`
			CustomFlag  CustomBool  `yaml:"custom_flag,default=false"`
			OmittedVal  NetworkType `yaml:"omitted_val,omitempty"`
			OmittedBool bool        `yaml:"omitted_bool,omitempty"`
		}

		yml := []byte(`
network_type: testnet
enabled: false
custom_flag: true
`)
		var cfg ConfigWithCustomTypes
		err := Unmarshal(yml, &cfg)
		assert.NoError(t, err)
		assert.Equal(t, NetworkType("testnet"), cfg.Network)
		assert.Equal(t, false, cfg.Enabled)
		assert.Equal(t, CustomBool(true), cfg.CustomFlag)
	})

	t.Run("custom type with unmarshaler and other custom directives", func(t *testing.T) {
		type ConfigWithUnmarshaler struct {
			NetType TestCustomUnmarshalerType `yaml:"network_type"`
			Enabled bool                      `yaml:"enabled,default=true"`
		}

		yml := []byte(`
network_type: evm
enabled: false
`)
		var cfg ConfigWithUnmarshaler
		err := Unmarshal(yml, &cfg)
		assert.NoError(t, err)
		assert.Equal(t, TestCustomUnmarshalerType("custom-evm"), cfg.NetType)
		assert.Equal(t, false, cfg.Enabled)
	})

	t.Run("custom type same field name and type name", func(t *testing.T) {
		type NetworkType string
		type ConfigWithSameName struct {
			NetworkType NetworkType `yaml:"network_type"`
			Enabled     bool        `yaml:"enabled,default=true"`
		}

		yml := []byte(`
network_type: evm
enabled: false
`)
		var cfg ConfigWithSameName
		err := Unmarshal(yml, &cfg)
		assert.NoError(t, err)
		assert.Equal(t, NetworkType("evm"), cfg.NetworkType)
		assert.Equal(t, false, cfg.Enabled)
	})

	t.Run("custom type in inlined map struct", func(t *testing.T) {
		type NetworkType string
		type Token struct {
			Symbol string `yaml:"symbol"`
			secret string
		}
		type ChainConfig struct {
			ChainID       int64       `yaml:"chain_id"`
			RPC           string      `yaml:"rpc"`
			Confirmations int         `yaml:"confirmations"`
			NetworkType   NetworkType `yaml:"network_type"`
			Tokens        []Token     `yaml:"tokens"`
		}
		type TokensConfig struct {
			Chains map[string]ChainConfig `yaml:",inline"`
		}
		type AppConfig struct {
			Env   string       `yaml:"env,required"`
			Token TokensConfig `yaml:"token"`
		}

		yml := []byte(`
env: prod
token:
  ethereum:
    chain_id: 1
    rpc: http://localhost:8545
    confirmations: 12
    network_type: evm
    tokens:
      - symbol: ETH
  optimism:
    chain_id: 10
    rpc: http://localhost:8546
    confirmations: 2
    network_type: evm
    tokens:
      - symbol: OP
`)
		var cfg AppConfig
		err := Unmarshal(yml, &cfg)
		assert.NoError(t, err)
		assert.Equal(t, "prod", cfg.Env)
		assert.Equal(t, 2, len(cfg.Token.Chains))
		eth := cfg.Token.Chains["ethereum"]
		assert.Equal(t, int64(1), eth.ChainID)
		assert.Equal(t, NetworkType("evm"), eth.NetworkType)
		assert.Equal(t, 1, len(eth.Tokens))
		assert.Equal(t, "ETH", eth.Tokens[0].Symbol)
	})

	t.Run("custom type in inlined map struct from separate domain package", func(t *testing.T) {
		type ChainConfig struct {
			ChainID       int64              `yaml:"chain_id"`
			RPC           string             `yaml:"rpc"`
			Confirmations int                `yaml:"confirmations"`
			NetworkType   domain.NetworkType `yaml:"network_type"`
			Tokens        []domain.Token     `yaml:"tokens"`
		}
		type TokensConfig struct {
			Chains map[string]ChainConfig `yaml:",inline"`
		}
		type AppConfig struct {
			Env   string       `yaml:"env,required"`
			Token TokensConfig `yaml:"token"`
		}

		yml := []byte(`
env: prod
token:
  ethereum:
    chain_id: 1
    rpc: http://localhost:8545
    confirmations: 12
    network_type: evm
    tokens:
      - symbol: ETH
  optimism:
    chain_id: 10
    rpc: http://localhost:8546
    confirmations: 2
    network_type: evm
    tokens:
      - symbol: OP
`)
		var cfg AppConfig
		err := Unmarshal(yml, &cfg)
		assert.NoError(t, err)
		assert.Equal(t, "prod", cfg.Env)
		assert.Equal(t, 2, len(cfg.Token.Chains))
		eth := cfg.Token.Chains["ethereum"]
		assert.Equal(t, int64(1), eth.ChainID)
		assert.Equal(t, domain.NetworkType("evm"), eth.NetworkType)
		assert.Equal(t, 1, len(eth.Tokens))
		assert.Equal(t, "ETH", eth.Tokens[0].Symbol)
	})

	t.Run("custom type in inlined map struct from separate domain package with exact user layout", func(t *testing.T) {
		type ChainConfig struct {
			ChainID       int64              `yaml:"chain_id"`
			RPC           string             `yaml:"rpc"`
			Confirmations int                `yaml:"confirmations"`
			NetworkType   domain.NetworkType `yaml:"network_type"`
			Tokens        []domain.Token     `yaml:"tokens"`
		}
		type TokensConfig struct {
			Chains map[string]ChainConfig `yaml:",inline"`
		}
		type AppConfig struct {
			Env   string       `yaml:"env,required"`
			Token TokensConfig `yaml:",inline"`
		}

		yml := []byte(`
env: prod
base_sepolia:
  chain_id: 84532
  rpc: "https://sepolia.base.org"
  network_type: evm
  confirmations: 3
  tokens:
    - symbol: USDC
`)
		var cfg AppConfig
		err := Unmarshal(yml, &cfg)
		assert.NoError(t, err)
		assert.Equal(t, "prod", cfg.Env)
		assert.Equal(t, 1, len(cfg.Token.Chains))
		base := cfg.Token.Chains["base_sepolia"]
		assert.Equal(t, int64(84532), base.ChainID)
		assert.Equal(t, domain.NetworkType("evm"), base.NetworkType)
		assert.Equal(t, 1, len(base.Tokens))
		assert.Equal(t, "USDC", base.Tokens[0].Symbol)
	})
}

