package tools

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestSchemaToMap_Nil(t *testing.T) {
	t.Parallel()
	m, err := SchemaToMap(nil)
	require.NoError(t, err)

	assert.Equal(t, map[string]any{
		"type":       "object",
		"properties": map[string]any{},
	}, m)
}

func TestSchemaToMap_MissingType(t *testing.T) {
	t.Parallel()
	m, err := SchemaToMap(map[string]any{
		"properties": map[string]any{},
	})
	require.NoError(t, err)

	assert.Equal(t, map[string]any{
		"type":       "object",
		"properties": map[string]any{},
	}, m)
}

func TestSchemaToMap_MissingEmptyProperties(t *testing.T) {
	t.Parallel()
	m, err := SchemaToMap(map[string]any{
		"type": "object",
	})
	require.NoError(t, err)

	assert.Equal(t, map[string]any{
		"type":       "object",
		"properties": map[string]any{},
	}, m)
}

func TestSchemaToMap_PropertyWithoutType(t *testing.T) {
	t.Parallel()
	m, err := SchemaToMap(map[string]any{
		"type": "object",
		"properties": map[string]any{
			"name": map[string]any{
				"type": "string",
			},
			"metadata": map[string]any{
				"description": "some metadata",
			},
		},
	})
	require.NoError(t, err)

	assert.Equal(t, map[string]any{
		"type": "object",
		"properties": map[string]any{
			"name": map[string]any{
				"type": "string",
			},
			"metadata": map[string]any{
				"type":        "object",
				"description": "some metadata",
			},
		},
	}, m)
}

func TestSchemaToMap_NestedPropertyWithoutType(t *testing.T) {
	t.Parallel()
	m, err := SchemaToMap(map[string]any{
		"type": "object",
		"properties": map[string]any{
			"config": map[string]any{
				"type": "object",
				"properties": map[string]any{
					"host": map[string]any{
						"type": "string",
					},
					"metadata": map[string]any{
						"description": "nested metadata without type",
					},
				},
			},
		},
	})
	require.NoError(t, err)

	assert.Equal(t, map[string]any{
		"type": "object",
		"properties": map[string]any{
			"config": map[string]any{
				"type": "object",
				"properties": map[string]any{
					"host": map[string]any{
						"type": "string",
					},
					"metadata": map[string]any{
						"type":        "object",
						"description": "nested metadata without type",
					},
				},
			},
		},
	}, m)
}

func TestSchemaToMap_ArrayItemsPropertyWithoutType(t *testing.T) {
	t.Parallel()
	m, err := SchemaToMap(map[string]any{
		"type": "object",
		"properties": map[string]any{
			"items": map[string]any{
				"type": "array",
				"items": map[string]any{
					"type": "object",
					"properties": map[string]any{
						"value": map[string]any{
							"description": "value without type",
						},
					},
				},
			},
		},
	})
	require.NoError(t, err)

	assert.Equal(t, map[string]any{
		"type": "object",
		"properties": map[string]any{
			"items": map[string]any{
				"type": "array",
				"items": map[string]any{
					"type": "object",
					"properties": map[string]any{
						"value": map[string]any{
							"type":        "object",
							"description": "value without type",
						},
					},
				},
			},
		},
	}, m)
}

func TestSchemaToMap_DeeplyNestedPropertyWithoutType(t *testing.T) {
	t.Parallel()
	m, err := SchemaToMap(map[string]any{
		"type": "object",
		"properties": map[string]any{
			"level1": map[string]any{
				"type": "object",
				"properties": map[string]any{
					"level2": map[string]any{
						"type": "object",
						"properties": map[string]any{
							"level3": map[string]any{
								"description": "deeply nested without type",
							},
						},
					},
				},
			},
		},
	})
	require.NoError(t, err)

	assert.Equal(t, map[string]any{
		"type": "object",
		"properties": map[string]any{
			"level1": map[string]any{
				"type": "object",
				"properties": map[string]any{
					"level2": map[string]any{
						"type": "object",
						"properties": map[string]any{
							"level3": map[string]any{
								"type":        "object",
								"description": "deeply nested without type",
							},
						},
					},
				},
			},
		},
	}, m)
}

func TestSchemaToMap_StripsNullFromRequiredArrayTypes(t *testing.T) {
	t.Parallel()
	m, err := SchemaToMap(map[string]any{
		"type": "object",
		"properties": map[string]any{
			"paths": map[string]any{
				"type":  []any{"null", "array"},
				"items": map[string]any{"type": "string"},
			},
			"excludePatterns": map[string]any{
				"type":  []any{"null", "array"},
				"items": map[string]any{"type": "string"},
			},
		},
		"required": []any{"paths"},
	})
	require.NoError(t, err)

	assert.Equal(t, map[string]any{
		"type": "object",
		"properties": map[string]any{
			"paths": map[string]any{
				"type":  "array",
				"items": map[string]any{"type": "string"},
			},
			"excludePatterns": map[string]any{
				"type":  []any{"null", "array"},
				"items": map[string]any{"type": "string"},
			},
		},
		"required": []any{"paths"},
	}, m)
}

// TestSchemaToMap_FlattensOptional guards the fix for MCP servers that declare
// optional params as the Optional[T] shape, anyOf:[{type:T},{type:null}]. Before
// the fix such a property had no top-level "type", so ensurePropertyTypes
// stamped it "object"; the object+anyOf schema made models emit unquoted string
// values ({"filter_key":region}) that failed json.Unmarshal in the MCP toolset.
// It must collapse to a plain {type:T}.
func TestSchemaToMap_FlattensOptional(t *testing.T) {
	m, err := SchemaToMap(map[string]any{
		"type": "object",
		"properties": map[string]any{
			"filter_key": map[string]any{
				"anyOf":   []any{map[string]any{"type": "string"}, map[string]any{"type": "null"}},
				"default": nil,
				"title":   "Filter Key",
			},
		},
	})
	require.NoError(t, err)

	filterKey := m["properties"].(map[string]any)["filter_key"].(map[string]any)
	assert.Equal(t, "string", filterKey["type"], "Optional[str] should collapse to a plain string type")
	assert.NotContains(t, filterKey, "anyOf", "the anyOf compositor should be removed")
	assert.Nil(t, filterKey["default"], "existing keys (default) survive the flatten")
	assert.Equal(t, "Filter Key", filterKey["title"])
}

// TestSchemaToMap_PreservesRealUnions ensures a genuine multi-type union is left
// intact and is NOT stamped "object" — only the single-non-null-branch Optional
// shape is collapsed.
func TestSchemaToMap_PreservesRealUnions(t *testing.T) {
	m, err := SchemaToMap(map[string]any{
		"type": "object",
		"properties": map[string]any{
			"value": map[string]any{
				"anyOf": []any{map[string]any{"type": "string"}, map[string]any{"type": "integer"}},
			},
		},
	})
	require.NoError(t, err)

	value := m["properties"].(map[string]any)["value"].(map[string]any)
	assert.Contains(t, value, "anyOf", "a real union keeps its anyOf")
	assert.NotEqual(t, "object", value["type"], "a real union must not be stamped object")
}
