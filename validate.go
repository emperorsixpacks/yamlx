package yamlx

import (
	"reflect"
	"strconv"
	"strings"

	"gopkg.in/yaml.v3"
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
				stripped := stripCustomDirectives(tag)
				f.Tag = reflect.StructTag(`yaml:"` + stripped + `"`)
			}
			f.Type = cleanYamlTags(f.Type)
			f.PkgPath = ""
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
	hasFieldName := false
	standard := []string{}

	for i, part := range parts {
		part = strings.TrimSpace(part)
		if i == 0 {
			if !isDirective(part) && part != "" {
				fieldName = part
				hasFieldName = true
			}
			continue
		}
		if isStandardYamlDirective(part) {
			standard = append(standard, part)
		}
	}

	result := fieldName
	if len(standard) > 0 {
		if hasFieldName {
			result += "," + strings.Join(standard, ",")
		} else {
			result = "," + strings.Join(standard, ",")
		}
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
	return s == "omitempty" || s == "string" || s == "flow" || s == "inline"
}

// validateStruct checks custom directives in yaml: tags and validates values.
func validateStruct(s any, node *yaml.Node) error {
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

		var fieldNode *yaml.Node
		if node != nil {
			fieldNode = findValueNode(node, getFieldName(field))
		}

		tag := field.Tag.Get("yaml")
		if tag != "" {
			if err := validateField(field.Name, fieldVal, tag, fieldNode != nil); err != nil {
				return err
			}
		}

		if err := validateRecursive(fieldVal, fieldNode); err != nil {
			return err
		}
	}
	return nil
}

func findValueNode(node *yaml.Node, key string) *yaml.Node {
	if node == nil {
		return nil
	}
	if node.Kind == yaml.DocumentNode {
		for _, child := range node.Content {
			if val := findValueNode(child, key); val != nil {
				return val
			}
		}
		return nil
	}
	if node.Kind == yaml.MappingNode {
		for i := 0; i < len(node.Content); i += 2 {
			if node.Content[i].Value == key {
				return node.Content[i+1]
			}
		}
	}
	return nil
}

func getFieldName(field reflect.StructField) string {
	tag := field.Tag.Get("yaml")
	if tag != "" {
		parts := strings.Split(tag, ",")
		name := strings.TrimSpace(parts[0])
		if name != "" && name != "-" {
			return name
		}
	}
	return strings.ToLower(field.Name)
}

func validateRecursive(val reflect.Value, node *yaml.Node) error {
	switch val.Kind() {
	case reflect.Pointer:
		if !val.IsNil() {
			return validateRecursive(val.Elem(), node)
		}
	case reflect.Struct:
		if val.CanAddr() {
			return validateStruct(val.Addr().Interface(), node)
		} else {
			tmp := reflect.New(val.Type()).Elem()
			tmp.Set(val)
			err := validateStruct(tmp.Addr().Interface(), node)
			if err == nil && val.CanSet() {
				val.Set(tmp)
			}
			return err
		}
	case reflect.Slice, reflect.Array:
		var elements []*yaml.Node
		if node != nil && node.Kind == yaml.SequenceNode {
			elements = node.Content
		}
		for i := 0; i < val.Len(); i++ {
			var elemNode *yaml.Node
			if i < len(elements) {
				elemNode = elements[i]
			}
			if err := validateRecursive(val.Index(i), elemNode); err != nil {
				return err
			}
		}
	case reflect.Map:
		if node != nil && node.Kind == yaml.MappingNode {
			for _, key := range val.MapKeys() {
				var valNode *yaml.Node
				keyStr := formatValue(key)
				for i := 0; i < len(node.Content); i += 2 {
					if node.Content[i].Value == keyStr {
						valNode = node.Content[i+1]
						break
					}
				}
				mapVal := val.MapIndex(key)
				tmp := reflect.New(mapVal.Type()).Elem()
				tmp.Set(mapVal)
				if err := validateRecursive(tmp, valNode); err != nil {
					return err
				}
				val.SetMapIndex(key, tmp)
			}
		} else {
			for _, key := range val.MapKeys() {
				mapVal := val.MapIndex(key)
				tmp := reflect.New(mapVal.Type()).Elem()
				tmp.Set(mapVal)
				if err := validateRecursive(tmp, nil); err != nil {
					return err
				}
				val.SetMapIndex(key, tmp)
			}
		}
	}
	return nil
}

func validateField(name string, val reflect.Value, tag string, present bool) error {
	directives := parseYamlTag(tag)

	_, hasOmitEmpty := directives["omitempty"]
	shouldApplyDefault := !present || (hasOmitEmpty && val.IsZero())

	if def, ok := directives["default"]; ok && shouldApplyDefault {
		applyDefault(val, def)
	}

	if _, ok := directives["required"]; ok {
		if !present || val.IsZero() {
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
			directives[part] = ""
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
