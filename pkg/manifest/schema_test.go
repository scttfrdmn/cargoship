package manifest

import (
	"encoding/json"
	"reflect"
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// jsonFieldNames returns the JSON property names for a struct type, following
// the `json:"name,..."` tags and skipping fields tagged `json:"-"`.
func jsonFieldNames(t reflect.Type) []string {
	var names []string
	for i := 0; i < t.NumField(); i++ {
		tag := t.Field(i).Tag.Get("json")
		if tag == "" || tag == "-" {
			continue
		}
		name := strings.Split(tag, ",")[0]
		if name == "" {
			continue
		}
		names = append(names, name)
	}
	return names
}

// schemaDoc is the parsed shape of schema.json we assert against.
type schemaDoc struct {
	Properties  map[string]json.RawMessage `json:"properties"`
	Definitions map[string]struct {
		Properties map[string]json.RawMessage `json:"properties"`
	} `json:"definitions"`
}

func loadSchema(t *testing.T) schemaDoc {
	t.Helper()
	var doc schemaDoc
	require.NoError(t, json.Unmarshal(SchemaJSON, &doc), "schema.json must be valid JSON")
	return doc
}

// TestSchemaMatchesStructs is the drift guard (#274): every JSON field on the
// core manifest structs must appear in schema.json, and every schema property
// must correspond to a real struct field. If someone adds/renames/removes a
// field in types.go without updating schema.json (or vice versa), this fails —
// so the embedded schema, the Go structs, and the documented format can't
// silently diverge.
func TestSchemaMatchesStructs(t *testing.T) {
	doc := loadSchema(t)

	cases := []struct {
		name       string
		typ        reflect.Type
		schemaKeys map[string]json.RawMessage
	}{
		{"Manifest", reflect.TypeOf(Manifest{}), doc.Properties},
		{"FileEntry", reflect.TypeOf(FileEntry{}), doc.Definitions["fileEntry"].Properties},
		{"ChunkEntry", reflect.TypeOf(ChunkEntry{}), doc.Definitions["chunkEntry"].Properties},
		{"ShardEntry", reflect.TypeOf(ShardEntry{}), doc.Definitions["shardEntry"].Properties},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			require.NotNil(t, tc.schemaKeys, "schema must define properties for %s", tc.name)
			structFields := jsonFieldNames(tc.typ)

			// Every struct field must be in the schema.
			for _, f := range structFields {
				_, ok := tc.schemaKeys[f]
				assert.True(t, ok, "%s field %q is missing from schema.json (struct changed without updating the schema?)", tc.name, f)
			}

			// Every schema property must correspond to a struct field.
			structSet := make(map[string]bool, len(structFields))
			for _, f := range structFields {
				structSet[f] = true
			}
			for prop := range tc.schemaKeys {
				assert.True(t, structSet[prop], "schema.json defines %s property %q with no matching struct field (stale schema?)", tc.name, prop)
			}
		})
	}
}

// TestSchemaRequiredFieldsArePresent asserts the schema's required arrays only
// list fields that exist (a required field the struct dropped would be a trap).
func TestSchemaRequiredFieldsExist(t *testing.T) {
	var full struct {
		Required    []string `json:"required"`
		Definitions map[string]struct {
			Required []string `json:"required"`
		} `json:"definitions"`
	}
	require.NoError(t, json.Unmarshal(SchemaJSON, &full))

	check := func(name string, typ reflect.Type, required []string) {
		fields := make(map[string]bool)
		for _, f := range jsonFieldNames(typ) {
			fields[f] = true
		}
		for _, r := range required {
			assert.True(t, fields[r], "%s: required field %q not found on struct", name, r)
		}
	}
	check("Manifest", reflect.TypeOf(Manifest{}), full.Required)
	check("FileEntry", reflect.TypeOf(FileEntry{}), full.Definitions["fileEntry"].Required)
	check("ChunkEntry", reflect.TypeOf(ChunkEntry{}), full.Definitions["chunkEntry"].Required)
	check("ShardEntry", reflect.TypeOf(ShardEntry{}), full.Definitions["shardEntry"].Required)
}

// TestSchemaEmbedded confirms the schema is embedded and parseable.
func TestSchemaEmbedded(t *testing.T) {
	assert.NotEmpty(t, SchemaJSON)
	var doc map[string]any
	require.NoError(t, json.Unmarshal(SchemaJSON, &doc))
	assert.Equal(t, "CargoShip Manifest", doc["title"])
}
