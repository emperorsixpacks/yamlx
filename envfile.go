package yamlx

import (
	"bytes"
	"os"
	"path/filepath"
	"strings"

	"github.com/joho/godotenv"
)

// preprocessEnvFiles scans raw YAML bytes for lines starting with "!env " and loads
// the referenced .env files via godotenv. The !env lines are removed from the output
// so YAML parsing doesn't see them.
func preprocessEnvFiles(in []byte) ([]byte, error) {
	lines := bytes.Split(in, []byte("\n"))
	var out [][]byte
	basePath, _ := os.Getwd()

	for _, line := range lines {
		trimmed := strings.TrimSpace(string(line))
		if strings.HasPrefix(trimmed, "!env ") {
			rawPath := strings.TrimSpace(trimmed[5:])
			absPath, err := filepath.Abs(filepath.Join(basePath, rawPath))
			if err != nil {
				return nil, err
			}
			envMap, err := godotenv.Read(absPath)
			if err != nil {
				return nil, err
			}
			for k, v := range envMap {
				os.Setenv(k, v)
			}
			continue // skip the !env line
		}
		out = append(out, line)
	}

	return bytes.Join(out, []byte("\n")), nil
}
