package tools

import (
	"encoding/json"

	"github.com/google/jsonschema-go/jsonschema"
)

func MustSchemaFor[T any]() any {
	schema, err := SchemaFor[T]()
	if err != nil {
		panic(err)
	}
	return schema
}

func SchemaFor[T any]() (any, error) {
	schema, err := jsonschema.For[T](&jsonschema.ForOptions{})
	if err != nil {
		return nil, err
	}
	return schema, nil
}

func SchemaToMap(params any) (map[string]any, error) {
	m := map[string]any{}
	if params != nil {
		buf, err := json.Marshal(params)
		if err != nil {
			return nil, err
		}

		if err := json.Unmarshal(buf, &m); err != nil {
			return nil, err
		}
	}

	// Ensure we have at least an empty object schema.
	// That's especially important for DMR but can't hurt for others.
	if m["type"] == nil {
		m["type"] = "object"
	}
	if m["properties"] == nil {
		m["properties"] = map[string]any{}
	}
	if m["required"] == nil {
		delete(m, "required")
	}

	// Collapse the Optional[T] shape (anyOf:[{type:T},{type:null}]) into a plain
	// {type:T} BEFORE ensurePropertyTypes runs. Such a property carries no
	// top-level "type", so ensurePropertyTypes would otherwise stamp it "object";
	// the resulting object+anyOf schema leads models to emit unquoted string
	// values like {"filter_key":region}, which then fail json.Unmarshal in the
	// MCP toolset. This is a common way for MCP servers to declare optional
	// string params.
	flattenNullableComposites(m)

	// Ensure all properties have a type set, recursively.
	ensurePropertyTypes(m)

	// Drop "null" from the type of required fields. Required + nullable
	// is contradictory and only inflates the schema's token cost.
	// jsonschema-go emits ["null", "array"] for any Go slice, including
	// required ones; this normalizes those back to a plain "array".
	stripNullFromRequiredTypes(m)

	return m, nil
}

// stripNullFromRequiredTypes recursively walks a JSON Schema map and removes
// "null" from the type of every property listed in its parent's "required"
// array. Optional properties are left untouched.
func stripNullFromRequiredTypes(schema map[string]any) {
	props, ok := schema["properties"].(map[string]any)
	if !ok {
		return
	}

	required := requiredSet(schema)

	for name, v := range props {
		prop, ok := v.(map[string]any)
		if !ok {
			continue
		}

		if required[name] {
			removeNullFromType(prop)
		}

		stripNullFromRequiredTypes(prop)
		if items, ok := prop["items"].(map[string]any); ok {
			stripNullFromRequiredTypes(items)
		}
	}
}

func requiredSet(schema map[string]any) map[string]bool {
	set := map[string]bool{}
	switch r := schema["required"].(type) {
	case []any:
		for _, name := range r {
			if s, ok := name.(string); ok {
				set[s] = true
			}
		}
	case []string:
		for _, s := range r {
			set[s] = true
		}
	}
	return set
}

func removeNullFromType(prop map[string]any) {
	typeVal, exists := prop["type"]
	if !exists {
		return
	}
	arr, ok := typeVal.([]any)
	if !ok {
		return
	}

	filtered := arr[:0]
	for _, t := range arr {
		if s, ok := t.(string); ok && s == "null" {
			continue
		}
		filtered = append(filtered, t)
	}

	switch len(filtered) {
	case 0:
		// All entries were "null"; leave the schema alone.
	case 1:
		prop["type"] = filtered[0]
	default:
		prop["type"] = filtered
	}
}

// ensurePropertyTypes recursively walks a JSON Schema map and ensures
// every property has a "type" set, defaulting to "object" if missing.
// It descends into nested "properties" and array "items".
func ensurePropertyTypes(schema map[string]any) {
	props, ok := schema["properties"].(map[string]any)
	if !ok {
		return
	}

	for _, v := range props {
		prop, ok := v.(map[string]any)
		if !ok {
			continue
		}

		// Don't stamp "object" onto a property that legitimately omits "type"
		// because it uses a schema compositor or reference — doing so produces a
		// contradictory schema that confuses models.
		if prop["type"] == nil && !hasComposite(prop) {
			prop["type"] = "object"
		}

		// Recurse into nested object properties.
		ensurePropertyTypes(prop)

		// Recurse into array items.
		if items, ok := prop["items"].(map[string]any); ok {
			ensurePropertyTypes(items)
		}
	}
}

// hasComposite reports whether a schema uses a compositor or reference keyword
// that legitimately stands in for a "type": anyOf/oneOf/allOf/$ref/enum.
func hasComposite(prop map[string]any) bool {
	for _, k := range []string{"anyOf", "oneOf", "allOf", "$ref", "enum"} {
		if _, ok := prop[k]; ok {
			return true
		}
	}
	return false
}

// flattenNullableComposites recursively collapses the Optional[T] shape
// — {"anyOf":[{"type":T},{"type":"null"}]} — into a plain {"type":T}, descending
// into nested object properties and array items. It only touches the unambiguous
// single-non-null-branch case; richer unions are left for the model to handle.
func flattenNullableComposites(schema map[string]any) {
	props, ok := schema["properties"].(map[string]any)
	if !ok {
		return
	}
	for _, v := range props {
		prop, ok := v.(map[string]any)
		if !ok {
			continue
		}
		flattenNullableComposite(prop)
		// Recurse after flattening: a hoisted object branch may bring its own
		// properties/items along.
		flattenNullableComposites(prop)
		if items, ok := prop["items"].(map[string]any); ok {
			flattenNullableComposites(items)
		}
	}
}

// flattenNullableComposite hoists the sole non-null branch of a property's
// "anyOf"/"oneOf" list up into the property itself, when every other branch is
// {"type":"null"}. Keys already present on the property (title, description,
// default) win over the branch's, so the field stays optional.
func flattenNullableComposite(prop map[string]any) {
	for _, key := range []string{"anyOf", "oneOf"} {
		branches, ok := prop[key].([]any)
		if !ok {
			continue
		}
		var nonNull []map[string]any
		ok = true
		for _, b := range branches {
			bm, isMap := b.(map[string]any)
			if !isMap {
				ok = false
				break
			}
			if isNullType(bm) {
				continue
			}
			nonNull = append(nonNull, bm)
		}
		// Only collapse the unambiguous single-branch case with well-formed
		// object branches.
		if !ok || len(nonNull) != 1 {
			continue
		}
		delete(prop, key)
		for k, val := range nonNull[0] {
			if _, exists := prop[k]; !exists {
				prop[k] = val
			}
		}
	}
}

// isNullType reports whether a schema branch describes only the null type,
// i.e. {"type":"null"} or {"type":["null"]}.
func isNullType(m map[string]any) bool {
	switch t := m["type"].(type) {
	case string:
		return t == "null"
	case []any:
		if len(t) == 0 {
			return false
		}
		for _, e := range t {
			if s, ok := e.(string); !ok || s != "null" {
				return false
			}
		}
		return true
	}
	return false
}

func ConvertSchema(params, v any) error {
	// First unmarshal to a map to check we have a type and non-nil properties
	m, err := SchemaToMap(params)
	if err != nil {
		return err
	}

	// Then another JSON marshal/unmarshal roundtrip to the destination type
	buf, err := json.Marshal(m)
	if err != nil {
		return err
	}

	return json.Unmarshal(buf, v)
}
