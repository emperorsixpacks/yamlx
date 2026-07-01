package yamlx

import (
	"time"

	"gopkg.in/yaml.v3"
)

// Timing holds duration information for each phase of Unmarshal processing.
type Timing struct {
	Total        time.Duration
	ExtractVars  time.Duration
	IfPreprocess time.Duration
	YAMLParse    time.Duration
	VarRefs      time.Duration
	Includes     time.Duration
	Marshal      time.Duration
	EnvVars      time.Duration
	FinalParse   time.Duration
}

// UnmarshalWithTiming works like Unmarshal but also returns timing information
// for each processing phase.
func UnmarshalWithTiming(in []byte, o any) (Timing, error) {
	var t Timing
	start := time.Now()

	t1 := time.Now()
	vars := extractRawVars(in)
	t.ExtractVars = time.Since(t1)

	t2 := time.Now()
	out := preprocessIf(in, vars)
	t.IfPreprocess = time.Since(t2)

	t3 := time.Now()
	var doc yaml.Node
	if err := yaml.Unmarshal(out, &doc); err != nil {
		return t, err
	}
	t.YAMLParse = time.Since(t3)

	t4 := time.Now()
	resolveYamlVarRefs(&doc, vars)
	t.VarRefs = time.Since(t4)

	t5 := time.Now()
	if err := resolveIncludes(&doc); err != nil {
		return t, err
	}
	t.Includes = time.Since(t5)

	t6 := time.Now()
	marshalled, err := yaml.Marshal(&doc)
	if err != nil {
		return t, err
	}
	t.Marshal = time.Since(t6)

	t7 := time.Now()
	ymlBytes, err := resolveEnvVars(marshalled)
	if err != nil {
		return t, err
	}
	t.EnvVars = time.Since(t7)

	t8 := time.Now()
	if err := yaml.Unmarshal(ymlBytes, o); err != nil {
		return t, err
	}
	t.FinalParse = time.Since(t8)

	t.Total = time.Since(start)
	return t, nil
}
