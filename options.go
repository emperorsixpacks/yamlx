package yamlx

// Option configures the behavior of Unmarshal.
type Option func(*config)

// config holds all configuration for an Unmarshal call.
type config struct {
	basePath      string
	maxDepth      int
	skipIncludes  bool
	skipEnvVars   bool
	skipIf        bool
	skipVars      bool
	skipValidation bool
	extraVars     map[string]string
}

func defaultConfig() config {
	return config{
		maxDepth: defaultMaxDepth,
	}
}

// WithBasePath sets the base directory for resolving !include paths.
// Defaults to the current working directory.
func WithBasePath(path string) Option {
	return func(c *config) {
		c.basePath = path
	}
}

// WithMaxDepth sets the maximum recursive include depth.
// Defaults to 10.
func WithMaxDepth(depth int) Option {
	return func(c *config) {
		c.maxDepth = depth
	}
}

// SkipIncludes disables !include tag resolution.
func SkipIncludes() Option {
	return func(c *config) {
		c.skipIncludes = true
	}
}

// SkipEnvVars disables ${VAR} environment variable substitution.
func SkipEnvVars() Option {
	return func(c *config) {
		c.skipEnvVars = true
	}
}

// SkipIf disables !if conditional preprocessing.
func SkipIf() Option {
	return func(c *config) {
		c.skipIf = true
	}
}

// SkipVars disables $var references to YAML-defined variables.
func SkipVars() Option {
	return func(c *config) {
		c.skipVars = true
	}
}

// SkipValidation disables automatic Validate() calls.
func SkipValidation() Option {
	return func(c *config) {
		c.skipValidation = true
	}
}

// WithVars adds extra variables that are available in $var references
// and !if conditionals, in addition to YAML-defined variables.
func WithVars(vars map[string]string) Option {
	return func(c *config) {
		c.extraVars = vars
	}
}
