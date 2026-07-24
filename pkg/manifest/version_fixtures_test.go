package manifest

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// TestVersionFixturesParseAndValidate is the version-compatibility guard (#274):
// committed manifest fixtures for each supported format version must keep
// parsing via FromJSON AND satisfy the embedded schema. If a future change to
// the structs or schema breaks an older version, this fails — so a version bump
// can't silently orphan archives written by an earlier CargoShip.
func TestVersionFixturesParseAndValidate(t *testing.T) {
	fixtures := []struct {
		file        string
		wantVersion string
	}{
		{"v2.0.json", "2.0"},
		{"v1.0.json", "1.0"},
	}

	for _, f := range fixtures {
		t.Run(f.file, func(t *testing.T) {
			data, err := os.ReadFile(filepath.Join("testdata", "manifests", f.file))
			require.NoError(t, err, "fixture must exist")

			// 1. It must still parse through the public loader.
			m, err := FromJSON(data)
			require.NoError(t, err, "%s must parse via FromJSON", f.file)
			assert.Equal(t, f.wantVersion, m.Version)
			assert.NotEmpty(t, m.Files, "parsed manifest should retain files")
			assert.NotEmpty(t, m.Chunks, "parsed manifest should retain chunks")

			// 2. It must satisfy the embedded schema.
			violations, err := ValidateAgainstSchema(data)
			require.NoError(t, err)
			assert.Empty(t, violations, "%s must satisfy the schema; violations: %v", f.file, violations)
		})
	}
}

// TestVersionFixture_RoundTripReserialize confirms a parsed fixture re-serializes
// to schema-valid JSON (a parse that silently dropped required data would fail
// re-validation).
func TestVersionFixture_RoundTripReserialize(t *testing.T) {
	data, err := os.ReadFile(filepath.Join("testdata", "manifests", "v2.0.json"))
	require.NoError(t, err)

	m, err := FromJSON(data)
	require.NoError(t, err)

	reserialized, err := m.ToJSON()
	require.NoError(t, err)

	violations, err := ValidateAgainstSchema(reserialized)
	require.NoError(t, err)
	assert.Empty(t, violations, "re-serialized manifest must still satisfy the schema: %v", violations)
}
