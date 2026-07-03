package yamlx

import (
	"bytes"
	"os"
	"regexp"
	"strings"
)

var ifPattern = regexp.MustCompile(`!if\s+"([^"]+)"\s*(==|!=)\s+"([^"]+)"\s+(.+?)\s+else\s+(.+)`)

// preprocessIf resolves !if conditionals in raw YAML bytes before parsing.
// Syntax: key: !if "$var" == "value" true_val else false_val
func preprocessIf(in []byte, vars map[string]string) []byte {
	lines := bytes.Split(in, []byte("\n"))
	for i, line := range lines {
		if !bytes.Contains(line, []byte(TagIf)) {
			continue
		}
		resolved := resolveIfLine(string(line), vars)
		lines[i] = []byte(resolved)
	}
	return bytes.Join(lines, []byte("\n"))
}

// resolveIfLine finds and resolves !if patterns in a single line.
func resolveIfLine(line string, vars map[string]string) string {
	for {
		idx := strings.Index(line, TagIf)
		if idx == -1 {
			return line
		}

		rest := strings.TrimSpace(line[idx+len(TagIf):])
		match := ifPattern.FindStringSubmatch(TagIf + " " + rest)
		if match == nil {
			return line
		}

		left := resolveIfRef(match[1], vars)
		op := match[2]
		right := resolveIfRef(match[3], vars)
		trueVal := strings.TrimSpace(match[4])
		falseVal := strings.TrimSpace(match[5])

		var result string
		switch op {
		case "==":
			if left == right {
				result = trueVal
			} else {
				result = falseVal
			}
		case "!=":
			if left != right {
				result = trueVal
			} else {
				result = falseVal
			}
		}

		fullMatch := match[0]
		line = strings.Replace(line, fullMatch, result, 1)
	}
}

// resolveIfRef resolves a $var or ${VAR} reference.
func resolveIfRef(s string, vars map[string]string) string {
	s = strings.TrimSpace(s)
	if strings.HasPrefix(s, DelimVar) && !strings.HasPrefix(s, DelimEnv) {
		varName := s[1:]
		if val, ok := vars[varName]; ok {
			return val
		}
		return os.Getenv(varName)
	}
	if strings.HasPrefix(s, DelimEnv) && strings.HasSuffix(s, "}") {
		varName := s[2 : len(s)-1]
		return os.Getenv(varName)
	}
	return s
}
