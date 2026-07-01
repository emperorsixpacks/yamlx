package envsubt

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

func NewTypeError(s string) typeError {
	return typeError{s}
}

func NewConfigError(s string) configError {
	return configError{s}
}

func NewRequiredError(varName string) requiredError {
	return requiredError{varName: varName}
}
