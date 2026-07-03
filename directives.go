package yamlx

import "regexp"

// YAML tag directives recognized by yamlx.
const (
	TagEnv     = "!env"
	TagInclude = "!include"
	TagIf      = "!if"
)

// Placeholder delimiters for ${VAR} env substitution.
const (
	DelimVar      = "$"
	DelimEnv      = "${"
	DelimDefault  = ":-"
	DelimRequired = ":?"
	DelimEnum     = ":|"
)

// Regex patterns for parsing directives and variables.
var (
	// ifPattern matches: !if "$var" == "value" true_val else false_val
	ifPattern = regexp.MustCompile(`!if\s+"([^"]+)"\s*(==|!=)\s+"([^"]+)"\s+(.+?)\s+else\s+(.+)`)

	// rawVarPattern matches simple key: value lines for $var extraction.
	rawVarPattern = regexp.MustCompile(`^(\w+)\s*:\s*(.+)$`)
)
