# envsubst

A lightweight Go package for substituting environment variables in configuration strings, similar to Docker Compose's `${VAR}` syntax.

## Features

- ✅ Replaces `${VAR}` with the value of the corresponding environment variable
- ✅ Supports default values with `${VAR:-default}` (uses `default` when `VAR` is unset or empty)
- ✅ Supports required variables with `${VAR:?}` (returns error when `VAR` is unset or empty)

## Installation

```bash
go get github.com/emperorsixpacks/envsubst
````

## Usage

```go
import (
	"fmt"
	"os"

	"github.com/emperorsixpacks/envsubst"
)

type Config struct {
	Name    string   `yaml:"name"`
	Version string   `yaml:"version"`
	Port    []int    `yaml:"port"`
	Network string   `yaml:"network"`
}

func main() {
	os.Setenv("CLIENT_NAME", "lighthouse")
	yml := []byte(`
name: ${CLIENT_NAME}
version: v1.2.3
port:
  - 30303
  - 8545
network: testnet
`)

	var cfg Config
	if err := envsubst.Unmarshal(yml, &cfg); err != nil {
		panic(err)
	}

	fmt.Printf("%+v\n", cfg)
}
```

### Default Values

Use `${VAR:-default}` to provide a fallback when the environment variable is not set:

```go
yml := []byte(`
name: ${NAME:-anonymous}
log_level: ${LOG_LEVEL:-info}
`)
```

### Required Variables

Use `${VAR:?}` to fail with an error if the environment variable is not set:

```go
yml := []byte(`
database_url: ${DATABASE_URL:?}
api_key: ${API_KEY:?}
`)
```

### Include Other Files

Use `!include` to load other YAML files:

```go
// network.yaml
yml := []byte(`
type: p2p
subnet: 10.0.0.0/24
`)

// main.yaml
yml := []byte(`
name: ${CLIENT_NAME}
network: !include ./network.yaml
ports:
  - 30303
  - 8545
`)

var cfg Config
envsubst.Unmarshal(yml, &cfg)
// cfg.Network.Type == "p2p"
```

Recursive includes are supported. Circular includes return an error.

