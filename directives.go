package yamlx

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
