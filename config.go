package yamlx

import (
	"maps"
	"os"
	"reflect"
	"strings"

	"gopkg.in/yaml.v3"
)

// Validator is an interface that types can implement to validate themselves
// after unmarshalling. If the target implements Validator, Unmarshal will
// call Validate() automatically after parsing is complete.
type Validator interface {
	Validate() error
}

// EnvLoader is an interface that types can implement to load environment
// variables before ${VAR} placeholders are resolved. If the target implements
// EnvLoader, Unmarshal will call LoadEnv() automatically before env substitution.
// Typical use: read a .env file with os.Setenv calls so ${VAR} resolves correctly.
type EnvLoader interface {
	LoadEnv() error
}

// Unmarshal takes a YAML byte slice `in` and unmarshals it into the object `o`.
// Processing order:
//  1. Extract $var definitions from raw bytes
//  2. Preprocess !if conditionals
//  3. Parse YAML into AST
//  4. Resolve $var references
//  5. Resolve !include tags
//  6. Call LoadEnv() if implemented
//  7. Resolve ${VAR} env substitution (directly on AST)
//  8. Unmarshal into target struct
//  9. Call Validate() if implemented
func Unmarshal(in []byte, o any, opts ...Option) error {
	cfg := defaultConfig()
	for _, opt := range opts {
		opt(&cfg)
	}

	vars := extractRawVars(in)

	// Merge extra vars from WithVars option
	for k, v := range cfg.extraVars {
		vars[k] = v
	}

	maps.Copy(vars, cfg.extraVars)

	// Extract dot-path variables from temporary AST parsing to support dot-paths in conditionals.
	// We sanitise the raw bytes first (replacing !if and !include values with a placeholder)
	// so yaml.Unmarshal can parse the document structure even when custom tags are present.
	var tempDoc yaml.Node
	if err := yaml.Unmarshal(sanitiseForTempParse(in), &tempDoc); err == nil {
		if !cfg.skipIncludes {
			_ = resolveIncludes(&tempDoc, cfg.basePath)
		}
		tempVars := collectYamlVars(&tempDoc)
		for k, v := range tempVars {
			if resolved, err := resolvePlaceHolder(v); err == nil {
				v = resolved
			}
			vars[k] = v
		}
		tempPathVars := make(map[string]pathVar)
		buildPathMap(&tempDoc, nil, 0, tempPathVars)
		for k, v := range tempPathVars {
			val := v.value
			if resolved, err := resolvePlaceHolder(val); err == nil {
				val = resolved
			}
			vars[k] = val
		}
	}

	out := in
	if !cfg.skipIf {
		out = preprocessIf(in, vars)
	}

	out, envErr := preprocessEnvFiles(out, cfg.basePath)
	if envErr != nil {
		return envErr
	}

	var doc yaml.Node
	if err := yaml.Unmarshal(out, &doc); err != nil {
		return err
	}

	if !cfg.skipIncludes {
		if err := resolveIncludes(&doc, cfg.basePath); err != nil {
			return err
		}
	}

	if !cfg.skipVars {
		pathVars := make(map[string]pathVar)
		if hasDotPathRefs(&doc) {
			buildPathMap(&doc, nil, 0, pathVars)
		}
		if _, err := resolveYamlVarRefs(&doc, vars, pathVars, nil, 0); err != nil {
			return err
		}
	}

	if v, ok := o.(EnvLoader); ok {
		if err := v.LoadEnv(); err != nil {
			return err
		}
	}

	if !cfg.skipEnvVars {
		if err := resolveEnvVars(&doc); err != nil {
			return err
		}
	}

	resolveNodeTypes(&doc, reflect.TypeOf(o))

	if err := unmarshalClean(nodeToBytes(&doc), o); err != nil {
		return err
	}

	if !cfg.skipValidation {
		if err := validateStruct(o, &doc); err != nil {
			return err
		}
	}

	if v, ok := o.(Validator); ok {
		return v.Validate()
	}

	return nil
}

// nodeToBytes marshals a yaml.Node back to bytes for final unmarshalling.
func nodeToBytes(node *yaml.Node) []byte {
	out, err := yaml.Marshal(node)
	if err != nil {
		return nil
	}
	return out
}

// resolveEnvVars walks the yaml.Node tree and resolves ${VAR} placeholders directly.
func resolveEnvVars(node *yaml.Node) error {
	if node == nil {
		return nil
	}

	switch node.Kind {
	case yaml.DocumentNode:
		for _, child := range node.Content {
			if err := resolveEnvVars(child); err != nil {
				return err
			}
		}
	case yaml.MappingNode:
		for i := 0; i < len(node.Content); i += 2 {
			if err := resolveEnvVars(node.Content[i+1]); err != nil {
				return err
			}
		}
	case yaml.SequenceNode:
		for _, child := range node.Content {
			if err := resolveEnvVars(child); err != nil {
				return err
			}
		}
	case yaml.ScalarNode:
		if node.Tag == "!!str" && strings.Contains(node.Value, "${") {
			resolved, err := resolvePlaceHolder(node.Value)
			if err != nil {
				return err
			}
			node.Value = resolved
		}
	}
	return nil
}

// resolvePlaceHolder checks if a string contains a placeholder of the form ${VAR},
// ${VAR:-default}, ${VAR:?}, or ${VAR:|opt1|opt2} and resolves it.
func resolvePlaceHolder(value string) (string, error) {
	if !strings.Contains(value, DelimEnv) {
		return value, nil
	}

	result := value
	for {
		start := strings.Index(result, DelimEnv)
		if start == -1 {
			break
		}
		end := strings.Index(result[start:], "}")
		if end == -1 {
			break
		}
		end += start

		inner := result[start+2 : end]

		var replacement string

		if idx := strings.Index(inner, DelimDefault); idx != -1 {
			varName := inner[:idx]
			defaultVal := inner[idx+len(DelimDefault):]
			envVal := os.Getenv(varName)
			if envVal == "" {
				replacement = defaultVal
			} else {
				replacement = envVal
			}
		} else if idx := strings.Index(inner, DelimRequired); idx != -1 {
			varName := inner[:idx]
			envVal := os.Getenv(varName)
			if envVal == "" {
				return "", NewRequiredError(varName)
			}
			replacement = envVal
		} else if idx := strings.Index(inner, DelimEnum); idx != -1 {
			varName := inner[:idx]
			allowed := inner[idx+len(DelimEnum):]
			envVal := os.Getenv(varName)
			options := strings.Split(allowed, "|")
			valid := false
			for _, opt := range options {
				if envVal == opt {
					valid = true
					break
				}
			}
			if !valid {
				return "", NewInvalidValueError(varName, envVal, allowed)
			}
			replacement = envVal
		} else {
			varName := inner
			replacement = os.Getenv(varName)
		}

		result = result[:start] + replacement + result[end+1:]
	}

	return result, nil
}

// unmarshalClean preprocesses the target to strip custom yaml directives,
// unmarshals into the clean type, then copies values back.
func unmarshalClean(data []byte, o any) error {
	ptr := reflect.ValueOf(o)
	if ptr.Kind() != reflect.Pointer || ptr.Elem().Kind() != reflect.Struct {
		return yaml.Unmarshal(data, o)
	}

	structType := ptr.Elem().Type()

	if !hasCustomDirectives(structType) {
		return yaml.Unmarshal(data, o)
	}

	cleanType := cleanYamlTags(structType)
	cleanPtr := reflect.New(cleanType)

	if err := yaml.Unmarshal(data, cleanPtr.Interface()); err != nil {
		return err
	}

	cleanVal := cleanPtr.Elem()
	origVal := ptr.Elem()
	copyValue(origVal, cleanVal)

	// reflect.StructOf-generated types don't support double-inline (inline struct containing
	// inline map). Detect those fields and re-unmarshal them directly into the original type.
	fixDoubleInlineFields(data, origVal, structType)

	return nil
}

// fixDoubleInlineFields finds struct fields that are yaml:",inline" and whose type
// also contains a yaml:",inline" map field (double-inline). For these, yaml.Unmarshal
// into reflect.StructOf-generated types fails silently, so we re-unmarshal directly
// into the original field type, filtering the data to only the keys that belong there.
func fixDoubleInlineFields(data []byte, val reflect.Value, t reflect.Type) {
	if t.Kind() != reflect.Struct {
		return
	}

	// Collect all non-inline field names so we can exclude them when building the
	// filtered document for inline-map fields.
	nonInlineKeys := collectNonInlineKeys(t)

	for i := 0; i < t.NumField(); i++ {
		f := t.Field(i)
		tag := f.Tag.Get("yaml")
		if !isInlineTag(tag) {
			continue
		}
		ft := f.Type
		for ft.Kind() == reflect.Pointer {
			ft = ft.Elem()
		}
		if ft.Kind() != reflect.Struct {
			continue
		}
		if !hasInlineMapField(ft) {
			// Recurse into inline struct in case of deeper nesting
			fieldVal := val.Field(i)
			for fieldVal.Kind() == reflect.Pointer {
				fieldVal = fieldVal.Elem()
			}
			if fieldVal.Kind() == reflect.Struct {
				fixDoubleInlineFields(data, fieldVal, ft)
			}
			continue
		}

		// Build a filtered YAML document that excludes non-inline (named) keys from
		// the parent struct, so we don't get type errors when unmarshalling the inline map.
		filtered := filterYAMLKeys(data, nonInlineKeys)
		fieldPtr := reflect.New(f.Type)
		if err := yaml.Unmarshal(filtered, fieldPtr.Interface()); err == nil {
			if val.Field(i).CanSet() {
				val.Field(i).Set(fieldPtr.Elem())
			}
		}
	}
}

// collectNonInlineKeys returns all yaml key names defined by non-inline fields in struct t.
func collectNonInlineKeys(t reflect.Type) map[string]bool {
	keys := make(map[string]bool)
	for i := 0; i < t.NumField(); i++ {
		f := t.Field(i)
		tag := f.Tag.Get("yaml")
		if isInlineTag(tag) {
			continue
		}
		name := ""
		if tag != "" {
			parts := strings.Split(tag, ",")
			name = strings.TrimSpace(parts[0])
		}
		if name == "" || name == "-" {
			name = strings.ToLower(f.Name)
		}
		if name != "-" {
			keys[name] = true
		}
	}
	return keys
}

// filterYAMLKeys returns a YAML document with top-level keys excluded by the given set.
func filterYAMLKeys(data []byte, exclude map[string]bool) []byte {
	var doc yaml.Node
	if err := yaml.Unmarshal(data, &doc); err != nil || len(doc.Content) == 0 {
		return data
	}
	root := doc.Content[0]
	if root.Kind != yaml.MappingNode {
		return data
	}
	filtered := &yaml.Node{Kind: yaml.MappingNode, Tag: root.Tag, Style: root.Style}
	for j := 0; j+1 < len(root.Content); j += 2 {
		keyNode := root.Content[j]
		valNode := root.Content[j+1]
		if !exclude[keyNode.Value] {
			filtered.Content = append(filtered.Content, keyNode, valNode)
		}
	}
	docNode := &yaml.Node{Kind: yaml.DocumentNode, Content: []*yaml.Node{filtered}}
	out, err := yaml.Marshal(docNode)
	if err != nil {
		return data
	}
	return out
}

// isInlineTag reports whether a yaml tag contains ",inline".
func isInlineTag(tag string) bool {
	for _, part := range strings.Split(tag, ",") {
		if strings.TrimSpace(part) == "inline" {
			return true
		}
	}
	return false
}

// hasInlineMapField reports whether struct type t has a direct field with yaml:",inline"
// whose type is a map.
func hasInlineMapField(t reflect.Type) bool {
	if t.Kind() != reflect.Struct {
		return false
	}
	for i := 0; i < t.NumField(); i++ {
		f := t.Field(i)
		if !isInlineTag(f.Tag.Get("yaml")) {
			continue
		}
		ft := f.Type
		for ft.Kind() == reflect.Pointer {
			ft = ft.Elem()
		}
		if ft.Kind() == reflect.Map {
			return true
		}
	}
	return false
}

// hasCustomDirectives checks if a struct type has any custom yaml directives.
func hasCustomDirectives(t reflect.Type) bool {
	visited := make(map[reflect.Type]bool)
	return hasCustomDirectivesRecursive(t, visited)
}

func hasCustomDirectivesRecursive(t reflect.Type, visited map[reflect.Type]bool) bool {
	if t.Kind() == reflect.Pointer {
		t = t.Elem()
	}
	if visited[t] {
		return false
	}
	visited[t] = true

	switch t.Kind() {
	case reflect.Slice, reflect.Array:
		return hasCustomDirectivesRecursive(t.Elem(), visited)
	case reflect.Map:
		return hasCustomDirectivesRecursive(t.Key(), visited) || hasCustomDirectivesRecursive(t.Elem(), visited)
	case reflect.Struct:
		for i := 0; i < t.NumField(); i++ {
			f := t.Field(i)
			tag := f.Tag.Get("yaml")
			if tag != "" {
				for _, part := range strings.Split(tag, ",") {
					part = strings.TrimSpace(part)
					if isDirective(part) {
						return true
					}
				}
			}
			if hasCustomDirectivesRecursive(f.Type, visited) {
				return true
			}
		}
	}
	return false
}

func copyValue(dst, src reflect.Value) {
	if !dst.CanSet() {
		return
	}

	if src.Type().AssignableTo(dst.Type()) {
		dst.Set(src)
		return
	}

	// Handle named types with a convertible underlying kind (e.g. type NetworkType string,
	// type CustomBool bool). cleanYamlTags produces plain underlying types, so src is string
	// while dst is NetworkType — not assignable but convertible.
	if src.Type().ConvertibleTo(dst.Type()) {
		switch dst.Kind() {
		case reflect.String, reflect.Bool,
			reflect.Int, reflect.Int8, reflect.Int16, reflect.Int32, reflect.Int64,
			reflect.Uint, reflect.Uint8, reflect.Uint16, reflect.Uint32, reflect.Uint64, reflect.Uintptr,
			reflect.Float32, reflect.Float64:
			dst.Set(src.Convert(dst.Type()))
			return
		}
	}

	switch dst.Kind() {
	case reflect.Pointer:
		if src.IsNil() {
			dst.Set(reflect.Zero(dst.Type()))
			return
		}
		if dst.IsNil() {
			dst.Set(reflect.New(dst.Type().Elem()))
		}
		copyValue(dst.Elem(), src.Elem())

	case reflect.Struct:
		for i := 0; i < dst.NumField(); i++ {
			copyValue(dst.Field(i), src.Field(i))
		}

	case reflect.Slice:
		if src.IsNil() {
			dst.Set(reflect.Zero(dst.Type()))
			return
		}
		n := src.Len()
		slice := reflect.MakeSlice(dst.Type(), n, n)
		for i := 0; i < n; i++ {
			copyValue(slice.Index(i), src.Index(i))
		}
		dst.Set(slice)

	case reflect.Array:
		n := src.Len()
		for i := 0; i < n; i++ {
			copyValue(dst.Index(i), src.Index(i))
		}

	case reflect.Map:
		if src.IsNil() {
			dst.Set(reflect.Zero(dst.Type()))
			return
		}
		dst.Set(reflect.MakeMap(dst.Type()))
		for _, key := range src.MapKeys() {
			dstKey := reflect.New(dst.Type().Key()).Elem()
			copyValue(dstKey, key)

			// MapIndex returns a non-addressable value; copy into an addressable
			// temporary so that nested copyValue calls (struct fields, slices, etc.)
			// can be set correctly.
			rawVal := src.MapIndex(key)
			srcVal := reflect.New(rawVal.Type()).Elem()
			srcVal.Set(rawVal)

			dstVal := reflect.New(dst.Type().Elem()).Elem()
			copyValue(dstVal, srcVal)

			dst.SetMapIndex(dstKey, dstVal)
		}

	case reflect.Interface:
		if src.IsValid() {
			dst.Set(src)
		}
	}
}

func resolveNodeTypes(node *yaml.Node, t reflect.Type) {
	if node == nil || t == nil {
		return
	}
	for t.Kind() == reflect.Pointer {
		t = t.Elem()
	}

	switch node.Kind {
	case yaml.DocumentNode:
		for _, child := range node.Content {
			resolveNodeTypes(child, t)
		}

	case yaml.MappingNode:
		if t.Kind() == reflect.Struct {
			for i := 0; i < len(node.Content); i += 2 {
				keyNode := node.Content[i]
				valNode := node.Content[i+1]
				if field, ok := findStructField(t, keyNode.Value); ok {
					isInline := false
					tag := field.Tag.Get("yaml")
					if tag != "" {
						parts := strings.Split(tag, ",")
						for _, p := range parts {
							if strings.TrimSpace(p) == "inline" {
								isInline = true
								break
							}
						}
					}

					if isInline {
						ft := field.Type
						for ft.Kind() == reflect.Pointer {
							ft = ft.Elem()
						}
						if ft.Kind() == reflect.Map {
							resolveNodeTypes(valNode, ft.Elem())
						} else if ft.Kind() == reflect.Struct {
							if subField, ok := findStructField(ft, keyNode.Value); ok {
								resolveNodeTypes(valNode, subField.Type)
							}
						}
					} else {
						resolveNodeTypes(valNode, field.Type)
					}
				}
			}
		} else if t.Kind() == reflect.Map {
			keyType := t.Key()
			valType := t.Elem()
			for i := 0; i < len(node.Content); i += 2 {
				resolveNodeTypes(node.Content[i], keyType)
				resolveNodeTypes(node.Content[i+1], valType)
			}
		}

	case yaml.SequenceNode:
		if t.Kind() == reflect.Slice || t.Kind() == reflect.Array {
			elemType := t.Elem()
			for _, child := range node.Content {
				resolveNodeTypes(child, elemType)
			}
		}

	case yaml.ScalarNode:
		if node.Tag == "!!str" {
			switch t.Kind() {
			case reflect.Int, reflect.Int8, reflect.Int16, reflect.Int32, reflect.Int64:
				node.Tag = "!!int"
				node.Style = 0
			case reflect.Uint, reflect.Uint8, reflect.Uint16, reflect.Uint32, reflect.Uint64, reflect.Uintptr:
				node.Tag = "!!int"
				node.Style = 0
			case reflect.Float32, reflect.Float64:
				node.Tag = "!!float"
				node.Style = 0
			case reflect.Bool:
				node.Tag = "!!bool"
				node.Style = 0
			}
		}
	}
}

func findStructField(t reflect.Type, key string) (reflect.StructField, bool) {
	if t.Kind() != reflect.Struct {
		return reflect.StructField{}, false
	}
	var inlineField *reflect.StructField
	for i := 0; i < t.NumField(); i++ {
		f := t.Field(i)
		tag := f.Tag.Get("yaml")
		
		isInline := false
		if tag != "" {
			parts := strings.Split(tag, ",")
			for _, p := range parts {
				if strings.TrimSpace(p) == "inline" {
					isInline = true
					break
				}
			}
		}

		if isInline {
			inlineField = &f
			ft := f.Type
			for ft.Kind() == reflect.Pointer {
				ft = ft.Elem()
			}
			if ft.Kind() == reflect.Struct {
				if _, ok := findStructField(ft, key); ok {
					return f, true
				}
			}
			continue
		}

		name := ""
		if tag != "" {
			parts := strings.Split(tag, ",")
			name = strings.TrimSpace(parts[0])
		}
		if name == "-" {
			continue
		}
		if name == "" {
			name = strings.ToLower(f.Name)
		}
		if name == strings.ToLower(key) {
			return f, true
		}
	}

	if inlineField != nil {
		ft := inlineField.Type
		for ft.Kind() == reflect.Pointer {
			ft = ft.Elem()
		}
		if ft.Kind() == reflect.Map {
			return *inlineField, true
		}
	}

	return reflect.StructField{}, false
}
