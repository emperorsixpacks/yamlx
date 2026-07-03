# yamlx

A lightweight Go package for parsing YAML with environment variables, file includes, inline variables, conditionals, and enum validation. One function, zero config.

```go
yamlx.Unmarshal(yml, &cfg)
```

## Install

```bash
go get github.com/emperorsixpacks/yamlx
```

## Quick Start

```go
package main

import (
    "fmt"
    "os"

    "github.com/emperorsixpacks/yamlx"
)

type Config struct {
    Name    string `yaml:"name"`
    Port    int    `yaml:"port"`
    Network struct {
        Type   string `yaml:"type"`
        Subnet string `yaml:"subnet"`
    } `yaml:"network"`
}

func main() {
    os.Setenv("APP_NAME", "myapp")
    os.Setenv("APP_ENV", "production")

    yml := []byte(`
name: ${APP_NAME}
port: !if "${APP_ENV}" == "production" 443 else 8080
network: !include ./network.yaml
`)

    var cfg Config
    if err := yamlx.Unmarshal(yml, &cfg); err != nil {
        panic(err)
    }

    fmt.Printf("%+v\n", cfg)
}
```

---

## Features

### Environment Variables `${VAR}`

Basic substitution from OS environment:

```yaml
name: ${CLIENT_NAME}
log_level: ${LOG_LEVEL:-info}
database_url: ${DATABASE_URL:?}
```

| Syntax | Behavior |
|---|---|
| `${VAR}` | Replace with env value (empty string if unset) |
| `${VAR:-default}` | Use `default` if unset or empty |
| `${VAR:?}` | Error if unset or empty |
| `${VAR:\|a\|b\|c}` | Error if value is not one of `a`, `b`, `c` |

Multiple placeholders in one string:

```yaml
endpoint: ${SCHEME}://${HOST:-localhost}:${PORT:-8080}/${BASE_PATH:-v1}
```

### Enum Validation `${VAR:|val1|val2}`

Constrain a variable to a set of allowed values:

```yaml
environment: ${APP_ENV:|production|staging|development}
region: ${AWS_REGION:|us-east-1|us-west-2|eu-west-1}
```

If `APP_ENV` is `invalid`, you get:

```
invalid value "invalid" for variable APP_ENV: must be one of [production|staging|development]
```

### Load Env Files `!env`

Load a `.env` file at the top of your YAML. Env vars are set via `os.Setenv` before `${VAR}` resolution:

```yaml
!env ./.env
!env ./secrets.env
db_host: ${DB_HOST}
db_port: ${DB_PORT}
```

Uses [joho/godotenv](https://github.com/joho/godotenv) under the hood — never sends env vars to a server, reads from local files only.

### Inline Variables `$var`

Define a key in your YAML and reference it below:

```yaml
env: production
region: us-east-1

# $env and $region are available because they appear above
port: !if "$env" == "production" 443 else 8080
log_level: !if "$env" == "production" warn else debug
```

Variables are resolved top to bottom. Each key becomes available as `$key` for lines below it.

### Dot-Path Variables `$a.b.c`

Reference nested values anywhere in the document using dot notation:

```yaml
storage:
  redis:
    redis_port: ${REDIS_PORT:-6379}
    redis_host: ${REDIS_HOST:-redis}
  database:
    database_port: ${DATABASE_PORT:-5432}
    database_host: ${DATABASE_HOST:-postgres}

indexer:
  redis_port: $storage.redis.redis_port
  db_port: $storage.database.database_port
```

**Constraint:** references must be at the same level or above the target root. You cannot use `$storage.database.port` from anywhere inside the `storage` subtree:

```yaml
storage:
  database:
    port: 5432
  cache:
    # ❌ ERROR - inside storage subtree
    fallback: $storage.database.port
```

Unknown paths are left as-is (unlike simple `$var` which must exist).

### Conditionals `!if`

Inline if/else directly in YAML:

```yaml
env: production

port: !if "$env" == "production" 443 else 8080
log: !if "$env" != "production" debug else warn
```

Works with OS env vars too:

```yaml
log: !if "${APP_ENV}" == "production" warn else debug
```

Conditionals are pre-processed before YAML parsing, so the syntax stays clean.

### File Includes `!include`

Load other YAML files:

**network.yaml**
```yaml
type: p2p
subnet: 10.0.0.0/24
```

**main.yaml**
```yaml
name: ${CLIENT_NAME}
network: !include ./network.yaml
ports:
  - 30303
  - 8545
```

| Syntax | Behavior |
|---|---|
| `!include ./file.yaml` | Load file (error if missing) |
| `!include ./file.yaml:?` | Required (error if missing) |
| `!include ./file.yaml:-./fallback.yaml` | Use fallback if primary missing |

Recursive includes work. Circular includes are detected:

```
circular include detected: ./file_a.yaml
```

Env var substitution works inside included files:

**defaults.yaml**
```yaml
log_level: ${LOG_LEVEL:-info}
region: ${REGION:-us-east-1}
```

**main.yaml**
```yaml
settings: !include ./defaults.yaml
```

---

## Validation

Implement `Validate() error` on your struct and it runs automatically after unmarshalling:

```go
type Config struct {
    Name string `yaml:"name"`
    Port int    `yaml:"port"`
}

func (c Config) Validate() error {
    if c.Name == "" {
        return errors.New("name is required")
    }
    if c.Port < 1 || c.Port > 65535 {
        return errors.New("port must be between 1 and 65535")
    }
    return nil
}

var cfg Config
err := yamlx.Unmarshal(yml, &cfg)
// err will be non-nil if validation fails
```

Validation runs **after** all env vars, includes, and conditionals are resolved. If your struct doesn't implement `Validate()`, it's simply skipped — zero overhead.

## Environment Loading

Implement `LoadEnv() error` on your struct and it runs automatically **before** `${VAR}` placeholders are resolved:

```go
type Config struct {
    DBHost string `yaml:"db_host"`
    DBPort int    `yaml:"db_port"`
}

func (c *Config) LoadEnv() error {
    // Load a .env file, call APIs, or set vars programmatically.
    // By the time this returns, os.Getenv will see everything.
    return godotenv.Load(".env")
}

var cfg Config
err := yamlx.Unmarshal(yml, &cfg)
// ${DB_HOST} and ${DB_PORT} in YAML resolve using the env vars loaded above
```

`LoadEnv()` runs **even if** you pass `SkipEnvVars()` — it sets env vars into the OS, not into the YAML directly. If your struct doesn't implement `EnvLoader`, it's skipped — zero overhead.

---

## Processing Order

```
 1. Extract $var definitions from raw bytes
 2. Preprocess !if conditionals
 3. Load !env files (godotenv)
 4. Parse YAML into AST
 5. Resolve $var and $a.b.c dot-path references
 6. Resolve !include tags
 7. Call LoadEnv() if implemented
 8. Resolve ${VAR} env substitution
 9. Unmarshal into Go struct
10. Call Validate() if implemented
```

## Error Handling

All errors come from `yamlx.Unmarshal`:

| Error | Cause |
|---|---|
| `required environment variable X is not set` | `${X:?}` used but `X` is unset |
| `invalid value "X" for variable Y: must be one of [...]` | `${Y:\|a\|b\|c}` but value not in set |
| `include file not found: ./file.yaml` | `!include` references missing file |
| `circular include detected: ./file.yaml` | Two files include each other |
| `max include depth exceeded` | Includes nested more than 10 levels deep |

## API

```go
func Unmarshal(in []byte, o any) error
```

One function. Parses YAML, resolves everything, unmarshals into your struct.

## License

MIT
