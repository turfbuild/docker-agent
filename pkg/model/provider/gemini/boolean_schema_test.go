package gemini

import (
	"testing"
)

// A tool input schema with a boolean sub-schema (the shape mcp-go >= v0.46
// emits for a Go `any`/interface{} field) must convert without error. Before
// the boolean-schema normalization this failed with
// "cannot unmarshal bool into Go struct field Schema.properties of type genai.Schema".
func TestConvertParametersToSchema_BooleanSubSchema(t *testing.T) {
	params := map[string]any{
		"type": "object",
		"properties": map[string]any{
			"address": map[string]any{"type": "string"},
			// interface{} field -> boolean "true" schema (any value).
			"count":    true,
			"for_each": true,
			"enabled":  true,
			"nested": map[string]any{
				"type": "array",
				"items": map[string]any{
					"type": "object",
					"properties": map[string]any{
						"value": true, // any value
					},
				},
			},
		},
		"required": []any{"address"},
	}

	schema, err := ConvertParametersToSchema(params)
	if err != nil {
		t.Fatalf("ConvertParametersToSchema: %v", err)
	}
	if schema == nil {
		t.Fatal("nil schema")
	}
	if _, ok := schema.Properties["count"]; !ok {
		t.Errorf("count property dropped; got %v", schema.Properties)
	}
	if _, ok := schema.Properties["nested"]; !ok {
		t.Errorf("nested property dropped; got %v", schema.Properties)
	}
}

func TestNormalizeBooleanSchemas(t *testing.T) {
	m := map[string]any{
		"properties": map[string]any{
			"any":  true,
			"none": false,
			"obj": map[string]any{
				"type":  "array",
				"items": true,
			},
		},
		// additionalProperties booleans must be preserved (genai ignores them,
		// and false carries meaning for other clients).
		"additionalProperties": false,
	}
	normalizeBooleanSchemas(m)

	props := m["properties"].(map[string]any)
	if _, ok := props["any"].(map[string]any); !ok {
		t.Errorf("true schema not coerced to object: %T", props["any"])
	}
	if _, ok := props["none"].(map[string]any); !ok {
		t.Errorf("false schema not coerced to object: %T", props["none"])
	}
	obj := props["obj"].(map[string]any)
	if _, ok := obj["items"].(map[string]any); !ok {
		t.Errorf("items true schema not coerced: %T", obj["items"])
	}
	if m["additionalProperties"] != false {
		t.Errorf("additionalProperties changed: %v", m["additionalProperties"])
	}
}
