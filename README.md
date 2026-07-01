# yamlx

A lightweight Go package for parsing YAML with environment variable substitution and file includes. Drop-in replacement for `gopkg.in/yaml.v3` with superpowers.

## Features

- **Environment variable substitution** — `${VAR}`, `${VAR:-default}`, `${VAR:?}`
- **File includes** — `!include ./other.yaml` with recursive and circular-include detection
- **Same syntax for both** — `:?` means required, `:-` means default fallback
- **Fully typed** — unmarshals directly into your Go structs
- **Zero config** — one function, no setup

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
    Version string `yaml:"version"`
    Port    int    `yaml:"port"`
}

func main() {
    os.Setenv("APP_NAME", "myapp")

    yml := []byte(`
name: ${APP_NAME}
version: v1.0.0
port: 8080
`)

    var cfg Config
    if err := yamlx.Unmarshal(yml, &cfg); err != nil {
        panic(err)
    }

    fmt.Printf("%+v\n", cfg)
    // {Name:myapp Version:v1.0.0 Port:8080}
}
```

## Environment Variables

### Basic substitution

```yaml
name: ${CLIENT_NAME}
```

Replaces `${CLIENT_NAME}` with the value of the `CLIENT_NAME` environment variable. If unset, the value is an empty string.

### Default values

```yaml
name: ${NAME:-anonymous}
log_level: ${LOG_LEVEL:-info}
```

Uses the provided default when the variable is unset or empty.

### Required variables

```yaml
database_url: ${DATABASE_URL:?}
api_key: ${API_KEY:?}
```

Returns an error if the variable is unset or empty. Use this for values that must be present.

### Composing values

```yaml
endpoint: ${SCHEME}://${HOST:-localhost}:${PORT:-8080}/${BASE_PATH:-v1}
```

Multiple placeholders in a single string work fine.

## File Includes

### Basic include

Split your config into multiple files and compose them:

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

Result after loading:
```yaml
name: lighthouse
network:
  type: p2p
  subnet: 10.0.0.0/24
ports:
  - 30303
  - 8545
```

### Required include

```yaml
config: !include ./network.yaml:?
```

Errors if the file does not exist. Same behavior as `${VAR:?}`.

### Default include (fallback)

```yaml
config: !include ./custom.yaml:-./defaults.yaml
```

If `./custom.yaml` does not exist, loads `./defaults.yaml` instead. Same behavior as `${VAR:-default}`.

### Recursive includes

Included files can include other files. There is a max depth of 10 to prevent infinite loops.

**base.yaml**
```yaml
type: p2p
```

**network.yaml**
```yaml
!include ./base.yaml
subnet: 10.0.0.0/24
```

**main.yaml**
```yaml
network: !include ./network.yaml
```

### Circular include detection

If file A includes file B and file B includes file A, you get a clear error:

```
circular include detected: ./file_a.yaml
```

### Env vars in included files

Environment variable substitution works inside included files too:

**defaults.yaml**
```yaml
log_level: ${LOG_LEVEL:-info}
region: ${REGION:-us-east-1}
```

**main.yaml**
```yaml
settings: !include ./defaults.yaml
```

## Error Handling

All errors come from `yamlx.Unmarshal`:

| Error | Cause |
|---|---|
| `include file not found: ./file.yaml` | `!include` references a file that does not exist |
| `circular include detected: ./file.yaml` | Two files include each other |
| `max include depth exceeded` | Includes nested more than 10 levels deep |
| `required environment variable X is not set` | `${X:?}` used but `X` is unset |

## API

```go
// Unmarshal parses YAML bytes into a struct.
// Resolves !include tags first, then environment variables, then unmarshals.
func Unmarshal(in []byte, o any) error
```

That's it. One function.

## License

MIT
