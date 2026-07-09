package yamlx

import (
	"reflect"
	"strconv"
	"strings"
)

var typeCache = make(map[reflect.Type]reflect.Type)

// cleanYamlTags creates a new struct type with custom directives stripped from yaml tags.
func cleanYamlTags(t reflect.Type) reflect.Type {
	if cached, ok := typeCache[t]; ok {
		return cached
	}

	switch t.Kind() {
	case reflect.Pointer:
		inner := cleanYamlTags(t.Elem())
		res := reflect.PointerTo(inner)
		typeCache[t] = res
		return res
	case reflect.Slice:
		inner := cleanYamlTags(t.Elem())
		res := reflect.SliceOf(inner)
		typeCache[t] = res
		return res
	case reflect.Array:
		inner := cleanYamlTags(t.Elem())
		res := reflect.ArrayOf(t.Len(), inner)
		typeCache[t] = res
		return res
	case reflect.Map:
		key := cleanYamlTags(t.Key())
		val := cleanYamlTags(t.Elem())
		res := reflect.MapOf(key, val)
		typeCache[t] = res
		return res
	case reflect.Struct:
		fields := make([]reflect.StructField, t.NumField())
		for i := 0; i < t.NumField(); i++ {
			f := t.Field(i)
			tag := f.Tag.Get("yaml")
			if tag != "" {
				f.Tag = reflect.StructTag(`yaml:"` + stripCustomDirectives(tag) + `"`)
			}
			f.Type = cleanYamlTags(f.Type)
			fields[i] = f
		}
		newType := reflect.StructOf(fields)
		typeCache[t] = newType
		return newType
	default:
		typeCache[t] = t
		return t
	}
}

// stripCustomDirectives removes our custom directives from a yaml tag string.
func stripCustomDirectives(tag string) string {
	parts := strings.Split(tag, ",")
	fieldName := ""
	standard := []string{}

	for i, part := range parts {
		part = strings.TrimSpace(part)
		if i == 0 && !isDirective(part) {
			fieldName = part
			continue
		}
		if isStandardYamlDirective(part) {
			standard = append(standard, part)
		}
	}

	result := fieldName
	if len(standard) > 0 {
		result += "," + strings.Join(standard, ",")
	}
	return result
}

func isDirective(s string) bool {
	return s == "omitempty" || s == "string" || s == "flow" ||
		strings.HasPrefix(s, "default=") || s == "required" ||
		strings.HasPrefix(s, "enum=") || strings.HasPrefix(s, "min=") ||
		strings.HasPrefix(s, "max=")
}

func isStandardYamlDirective(s string) bool {
	return s == "omitempty" || s == "string" || s == "flow"
}

// validateStruct checks custom directives in yaml: tags and validates values.
func validateStruct(s any) error {
	val := reflect.ValueOf(s)
	if val.Kind() == reflect.Pointer {
		val = val.Elem()
	}
	if val.Kind() != reflect.Struct {
		return nil
	}

	typ := val.Type()
	for i := 0; i < typ.NumField(); i++ {
		field := typ.Field(i)
		fieldVal := val.Field(i)

		tag := field.Tag.Get("yaml")
		if tag != "" {
			if err := validateField(field.Name, fieldVal, tag); err != nil {
				return err
			}
		}

		if err := validateRecursive(fieldVal); err != nil {
			return err
		}
	}
	return nil
}

func validateRecursive(val reflect.Value) error {
	switch val.Kind() {
	case reflect.Pointer:
		if !val.IsNil() {
			return validateRecursive(val.Elem())
		}
	case reflect.Struct:
		if val.CanAddr() {
			return validateStruct(val.Addr().Interface())
		} else {
			tmp := reflect.New(val.Type()).Elem()
			tmp.Set(val)
			return validateStruct(tmp.Addr().Interface())
		}
	case reflect.Slice, reflect.Array:
		for i := 0; i < val.Len(); i++ {
			if err := validateRecursive(val.Index(i)); err != nil {
				return err
			}
		}
	case reflect.Map:
		for _, key := range val.MapKeys() {
			if err := validateRecursive(val.MapIndex(key)); err != nil {
				return err
			}
		}
	}
	return nil
}

func validateField(name string, val reflect.Value, tag string) error {
	directives := parseYamlTag(tag)

	if def, ok := directives["default"]; ok && val.IsZero() {
		applyDefault(val, def)
	}

	if _, ok := directives["required"]; ok {
		if val.IsZero() {
			return NewConfigError("field " + name + " is required")
		}
	}

	if enumStr, ok := directives["enum"]; ok {
		if !val.IsZero() {
			s := formatValue(val)
			options := strings.Split(enumStr, "|")
			valid := false
			for _, opt := range options {
				if s == opt {
					valid = true
					break
				}
			}
			if !valid {
				return NewConfigError("field " + name + ": invalid value \"" + s + "\", must be one of [" + enumStr + "]")
			}
		}
	}

	if minStr, ok := directives["min"]; ok {
		if val.Kind() == reflect.Int || val.Kind() == reflect.Int64 || val.Kind() == reflect.Int32 {
			min, err := strconv.ParseInt(minStr, 10, 64)
			if err != nil {
				return NewConfigError("field " + name + ": invalid min value \"" + minStr + "\"")
			}
			if val.Int() < min {
				return NewConfigError("field " + name + ": value " + strconv.FormatInt(val.Int(), 10) + " is less than minimum " + minStr)
			}
		}
	}

	if maxStr, ok := directives["max"]; ok {
		if val.Kind() == reflect.Int || val.Kind() == reflect.Int64 || val.Kind() == reflect.Int32 {
			max, err := strconv.ParseInt(maxStr, 10, 64)
			if err != nil {
				return NewConfigError("field " + name + ": invalid max value \"" + maxStr + "\"")
			}
			if val.Int() > max {
				return NewConfigError("field " + name + ": value " + strconv.FormatInt(val.Int(), 10) + " is greater than maximum " + maxStr)
			}
		}
	}

	return nil
}

func formatValue(val reflect.Value) string {
	switch val.Kind() {
	case reflect.String:
		return val.String()
	case reflect.Int, reflect.Int64, reflect.Int32:
		return strconv.FormatInt(val.Int(), 10)
	case reflect.Float64, reflect.Float32:
		return strconv.FormatFloat(val.Float(), 'f', -1, 64)
	case reflect.Bool:
		return strconv.FormatBool(val.Bool())
	default:
		return ""
	}
}

func parseYamlTag(tag string) map[string]string {
	directives := make(map[string]string)
	for _, part := range strings.Split(tag, ",") {
		part = strings.TrimSpace(part)
		if part == "" {
			continue
		}
		if part == "omitempty" || part == "string" || part == "flow" {
			continue
		}
		if idx := strings.Index(part, "="); idx != -1 {
			directives[part[:idx]] = part[idx+1:]
		} else {
			directives[part] = ""
		}
	}
	return directives
}

func applyDefault(val reflect.Value, def string) {
	switch val.Kind() {
	case reflect.String:
		val.SetString(def)
	case reflect.Int, reflect.Int64, reflect.Int32:
		if n, err := strconv.ParseInt(def, 10, 64); err == nil {
			val.SetInt(n)
		}
	case reflect.Float64, reflect.Float32:
		if f, err := strconv.ParseFloat(def, 64); err == nil {
			val.SetFloat(f)
		}
	case reflect.Bool:
		if b, err := strconv.ParseBool(def); err == nil {
			val.SetBool(b)
		}
	}
}
