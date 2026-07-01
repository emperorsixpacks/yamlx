package yamlx

type typeError struct {
	s string
}

func (e typeError) Error() string {
	return e.s
}

type configError struct {
	s string
}

func (e configError) Error() string {
	return e.s
}

type requiredError struct {
	varName string
}

func (e requiredError) Error() string {
	return "required environment variable " + e.varName + " is not set"
}

type includeError struct {
	file string
	kind string
}

func (e includeError) Error() string {
	switch e.kind {
	case "not_found":
		return "include file not found: " + e.file
	case "cycle":
		return "circular include detected: " + e.file
	case "depth":
		return "max include depth exceeded (possible infinite include chain starting at: " + e.file + ")"
	default:
		return "include error for " + e.file
	}
}

func NewTypeError(s string) typeError {
	return typeError{s}
}

func NewConfigError(s string) configError {
	return configError{s}
}

func NewRequiredError(varName string) requiredError {
	return requiredError{varName: varName}
}

func NewIncludeError(file, kind string) includeError {
	return includeError{file: file, kind: kind}
}
