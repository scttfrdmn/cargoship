package manifest

import (
	"encoding/json"
	"fmt"
	"sort"
	"strings"
)

// ValidateAgainstSchema checks that a manifest JSON document conforms to the
// embedded manifest schema (schema.json). It is a focused validator for the
// draft-07 subset the manifest schema actually uses — object/array/string/
// integer/number/boolean types (including ["object","null"] unions), required
// field lists, declared properties, local "#/definitions/..." $refs, and typed
// array items. It is NOT a general-purpose JSON Schema engine; it exists so
// real uploaded manifests can be proven to comply with the documented schema
// (#274) without adding a dependency.
//
// It returns a sorted list of human-readable violations (empty means valid).
func ValidateAgainstSchema(manifestJSON []byte) ([]string, error) {
	var schema map[string]json.RawMessage
	if err := json.Unmarshal(SchemaJSON, &schema); err != nil {
		return nil, fmt.Errorf("parse embedded schema: %w", err)
	}
	var doc interface{}
	if err := json.Unmarshal(manifestJSON, &doc); err != nil {
		return nil, fmt.Errorf("parse manifest: %w", err)
	}

	v := &schemaValidator{root: schema}
	v.validate("(root)", schema, doc)
	sort.Strings(v.errs)
	return v.errs, nil
}

type schemaValidator struct {
	root map[string]json.RawMessage
	errs []string
}

func (v *schemaValidator) fail(path, format string, args ...interface{}) {
	v.errs = append(v.errs, path+": "+fmt.Sprintf(format, args...))
}

// resolveRef returns the schema object a "#/definitions/x" ref points at.
func (v *schemaValidator) resolveRef(ref string) (map[string]json.RawMessage, bool) {
	const prefix = "#/definitions/"
	if !strings.HasPrefix(ref, prefix) {
		return nil, false
	}
	var defs map[string]json.RawMessage
	if raw, ok := v.root["definitions"]; ok {
		if err := json.Unmarshal(raw, &defs); err != nil {
			return nil, false
		}
	}
	raw, ok := defs[strings.TrimPrefix(ref, prefix)]
	if !ok {
		return nil, false
	}
	var obj map[string]json.RawMessage
	if err := json.Unmarshal(raw, &obj); err != nil {
		return nil, false
	}
	return obj, true
}

// validate checks value against a schema node.
func (v *schemaValidator) validate(path string, node map[string]json.RawMessage, value interface{}) {
	// Follow a local $ref first.
	if raw, ok := node["$ref"]; ok {
		var ref string
		if err := json.Unmarshal(raw, &ref); err == nil {
			if target, ok := v.resolveRef(ref); ok {
				v.validate(path, target, value)
			} else {
				v.fail(path, "unresolvable $ref %q", ref)
			}
		}
		return
	}

	if raw, ok := node["type"]; ok {
		if !v.checkType(path, raw, value) {
			return // wrong type; deeper checks would be noise
		}
	}

	// Object: required + properties.
	if obj, isObj := value.(map[string]interface{}); isObj {
		if raw, ok := node["required"]; ok {
			var required []string
			_ = json.Unmarshal(raw, &required)
			for _, r := range required {
				if _, present := obj[r]; !present {
					v.fail(path, "missing required field %q", r)
				}
			}
		}
		if raw, ok := node["properties"]; ok {
			var props map[string]json.RawMessage
			_ = json.Unmarshal(raw, &props)
			for name, propSchemaRaw := range props {
				child, present := obj[name]
				if !present {
					continue // presence enforced by "required", not here
				}
				var propSchema map[string]json.RawMessage
				if err := json.Unmarshal(propSchemaRaw, &propSchema); err == nil {
					v.validate(path+"."+name, propSchema, child)
				}
			}
		}
	}

	// Array items.
	if arr, isArr := value.([]interface{}); isArr {
		if raw, ok := node["items"]; ok {
			var itemSchema map[string]json.RawMessage
			if err := json.Unmarshal(raw, &itemSchema); err == nil {
				for i, item := range arr {
					v.validate(fmt.Sprintf("%s[%d]", path, i), itemSchema, item)
				}
			}
		}
	}
}

// checkType validates value against the schema "type" (a string or array of
// strings). Returns false on mismatch. JSON numbers decode to float64; an
// "integer" type accepts a float64 with no fractional part.
func (v *schemaValidator) checkType(path string, raw json.RawMessage, value interface{}) bool {
	var types []string
	var single string
	if err := json.Unmarshal(raw, &single); err == nil {
		types = []string{single}
	} else if err := json.Unmarshal(raw, &types); err != nil {
		return true // unrecognized type spec; don't block
	}

	for _, ty := range types {
		if matchesType(ty, value) {
			return true
		}
	}
	v.fail(path, "expected type %v, got %s", types, jsonTypeOf(value))
	return false
}

func matchesType(ty string, value interface{}) bool {
	switch ty {
	case "null":
		return value == nil
	case "object":
		_, ok := value.(map[string]interface{})
		return ok
	case "array":
		_, ok := value.([]interface{})
		return ok
	case "string":
		_, ok := value.(string)
		return ok
	case "boolean":
		_, ok := value.(bool)
		return ok
	case "number":
		_, ok := value.(float64)
		return ok
	case "integer":
		f, ok := value.(float64)
		return ok && f == float64(int64(f))
	}
	return false
}

func jsonTypeOf(value interface{}) string {
	switch value.(type) {
	case nil:
		return "null"
	case map[string]interface{}:
		return "object"
	case []interface{}:
		return "array"
	case string:
		return "string"
	case bool:
		return "boolean"
	case float64:
		return "number"
	}
	return "unknown"
}
