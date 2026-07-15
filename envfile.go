package yamlx

import (
	"bytes"
	"errors"
	"os"
	"path/filepath"
	"strings"

	"github.com/joho/godotenv"
)

// preprocessEnvFiles scans raw YAML bytes for lines starting with "!env" and loads
// the referenced .env files via godotenv. The !env lines are removed from the output
// so YAML parsing doesn't see them.
//
// Syntax:
//   - !env ./file.env    — required (error if file missing)
//   - !env? ./file.env   — optional (skip silently if file missing)
func preprocessEnvFiles(in []byte, basePath string) ([]byte, error) {
	lines := bytes.Split(in, []byte("\n"))
	var out [][]byte
	if basePath == "" {
		basePath, _ = os.Getwd()
	}

	for _, line := range lines {
		trimmed := strings.TrimSpace(string(line))

		optional := false
		pathPart := ""

		if strings.HasPrefix(trimmed, "!env? ") {
			optional = true
			pathPart = strings.TrimSpace(trimmed[6:])
		} else if strings.HasPrefix(trimmed, "!env ") {
			pathPart = strings.TrimSpace(trimmed[5:])
		} else {
			out = append(out, line)
			continue
		}

		absPath, err := filepath.Abs(filepath.Join(basePath, pathPart))
		if err != nil {
			return nil, err
		}

		envMap, readErr := godotenv.Read(absPath)
		if readErr != nil {
			if optional && errors.Is(readErr, os.ErrNotExist) {
				continue // skip missing file silently
			}
			return nil, readErr
		}
		for k, v := range envMap {
			os.Setenv(k, v)
		}
		// skip the !env line
	}

	return bytes.Join(out, []byte("\n")), nil
}
