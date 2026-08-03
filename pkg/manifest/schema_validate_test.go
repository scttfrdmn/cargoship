package manifest

import (
	"encoding/json"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// buildRealManifest constructs a manifest through the actual Builder API the
// pipeline uses, so the validator is exercised against genuine CargoShip output
// shape (not a hand-rolled document).
func buildRealManifest(t *testing.T) *Manifest {
	t.Helper()
	b, err := NewBuilder("20260724-abc123", "/tmp/src", "example-bucket", "backups", "us-east-1")
	require.NoError(t, err)

	b.SetShardCount(1)
	b.AddFile(FileEntry{
		Path: "data/a.txt", Size: 100, ModTime: time.Unix(1_700_000_000, 0),
		ChunkID: 0, ShardID: 0, S3Key: "backups/uploads/20260724-abc123/shard-0/chunk-0.tar.zst",
		Checksum: "abc123",
	})
	b.AddChunk(ChunkEntry{
		ID: 0, ShardID: 0, S3Key: "backups/uploads/20260724-abc123/shard-0/chunk-0.tar.zst",
		FileCount: 1, FilePaths: []string{"data/a.txt"},
		UncompressedSize: 100, CompressedSize: 50,
		CreatedAt: time.Unix(1_700_000_000, 0), UploadedAt: time.Unix(1_700_000_060, 0),
		Checksum: "def456",
	})
	b.UpdateShardStats(0, "backups/uploads/20260724-abc123/shard-0/chunk-0.tar.zst", 1, 100, 50)
	b.SetCompression("zstd", 3, 0.5)
	return b.Finalize()
}

// TestValidateAgainstSchema_RealOutput proves that a manifest produced by the
// real Builder validates against the embedded schema (#274) — closing the
// "real CargoShip output complies with the spec" half of compliance.
func TestValidateAgainstSchema_RealOutput(t *testing.T) {
	m := buildRealManifest(t)
	data, err := m.ToJSON()
	require.NoError(t, err)

	violations, err := ValidateAgainstSchema(data)
	require.NoError(t, err)
	assert.Empty(t, violations, "real manifest should satisfy the schema; violations: %v", violations)
}

// TestValidateAgainstSchema_MissingRequired flags a dropped required field.
func TestValidateAgainstSchema_MissingRequired(t *testing.T) {
	m := buildRealManifest(t)
	data, err := m.ToJSON()
	require.NoError(t, err)

	var obj map[string]interface{}
	require.NoError(t, json.Unmarshal(data, &obj))
	delete(obj, "bucket") // required
	mangled, _ := json.Marshal(obj)

	violations, err := ValidateAgainstSchema(mangled)
	require.NoError(t, err)
	assert.Contains(t, violationsJoined(violations), `missing required field "bucket"`)
}

// TestValidateAgainstSchema_WrongType flags a field with the wrong JSON type.
func TestValidateAgainstSchema_WrongType(t *testing.T) {
	m := buildRealManifest(t)
	data, err := m.ToJSON()
	require.NoError(t, err)

	var obj map[string]interface{}
	require.NoError(t, json.Unmarshal(data, &obj))
	obj["total_files"] = "not-a-number" // should be integer
	mangled, _ := json.Marshal(obj)

	violations, err := ValidateAgainstSchema(mangled)
	require.NoError(t, err)
	assert.Contains(t, violationsJoined(violations), "total_files")
}

// TestValidateAgainstSchema_NestedWrongType flags a bad field inside a file entry.
func TestValidateAgainstSchema_NestedWrongType(t *testing.T) {
	m := buildRealManifest(t)
	data, err := m.ToJSON()
	require.NoError(t, err)

	var obj map[string]interface{}
	require.NoError(t, json.Unmarshal(data, &obj))
	files := obj["files"].([]interface{})
	files[0].(map[string]interface{})["size"] = "big" // should be integer
	mangled, _ := json.Marshal(obj)

	violations, err := ValidateAgainstSchema(mangled)
	require.NoError(t, err)
	assert.Contains(t, violationsJoined(violations), "files[0].size")
}

// TestValidateAgainstSchema_MissingNestedRequired flags a chunk missing s3_key.
func TestValidateAgainstSchema_MissingNestedRequired(t *testing.T) {
	m := buildRealManifest(t)
	data, err := m.ToJSON()
	require.NoError(t, err)

	var obj map[string]interface{}
	require.NoError(t, json.Unmarshal(data, &obj))
	chunks := obj["chunks"].([]interface{})
	delete(chunks[0].(map[string]interface{}), "s3_key")
	mangled, _ := json.Marshal(obj)

	violations, err := ValidateAgainstSchema(mangled)
	require.NoError(t, err)
	assert.Contains(t, violationsJoined(violations), `chunks[0]: missing required field "s3_key"`)
}

// TestValidateAgainstSchema_OptionalNullBlocks confirms nullable optional blocks
// (encryption/deduplication/etc.) validate whether absent, null, or an object.
func TestValidateAgainstSchema_OptionalNullBlocks(t *testing.T) {
	m := buildRealManifest(t)
	data, err := m.ToJSON()
	require.NoError(t, err)

	var obj map[string]interface{}
	require.NoError(t, json.Unmarshal(data, &obj))
	obj["encryption"] = nil                                        // null allowed
	obj["deduplication"] = map[string]interface{}{"enabled": true} // object allowed
	mangled, _ := json.Marshal(obj)

	violations, err := ValidateAgainstSchema(mangled)
	require.NoError(t, err)
	assert.Empty(t, violations, "nullable optional blocks should validate: %v", violations)
}

// TestValidateAgainstSchema_EmptyCollectionsAreArrays is the named regression
// test for the nil-slice defect FuzzValidateAgainstSchema found (#302/#303).
//
// The schema declares files, chunks, and shards required with type "array", but
// encoding/json renders a nil slice as `null`. An upload that produced no chunks
// (e.g. everything routed to the direct path) therefore wrote a manifest in a
// shape the format's own published schema rejects — a document CargoShip emitted
// and a conforming third-party reader would refuse. `ToJSON` normalizes the nil
// slices to empty arrays; this pins that.
//
// The fuzz target checks this as a differential invariant over generated input;
// this states it as a readable case, so the guarantee is legible in the test
// suite rather than implicit in a corpus file.
func TestValidateAgainstSchema_EmptyCollectionsAreArrays(t *testing.T) {
	// A manifest with every collection left nil — the shape a no-chunk upload
	// produces. Built directly rather than through Builder, which pre-populates.
	m := &Manifest{
		Version:           ManifestVersion,
		UploadID:          "20260802-empty",
		SourcePath:        "/tmp/src",
		Bucket:            "example-bucket",
		Prefix:            "backups",
		Region:            "us-east-1",
		CreatedAt:         time.Unix(1_700_000_000, 0).UTC(),
		ChecksumAlgorithm: ChecksumAlgorithmSHA256,
	}
	require.Nil(t, m.Files, "precondition: Files is nil")
	require.Nil(t, m.Chunks, "precondition: Chunks is nil")
	require.Nil(t, m.Shards, "precondition: Shards is nil")

	data, err := m.ToJSON()
	require.NoError(t, err)

	// The wire form must carry arrays, never null. Asserted on the raw JSON: a
	// round trip through Manifest would hide the difference, since both `null`
	// and `[]` unmarshal to a slice that reads as empty.
	var obj map[string]json.RawMessage
	require.NoError(t, json.Unmarshal(data, &obj))
	for _, field := range []string{"files", "chunks", "shards"} {
		raw, ok := obj[field]
		require.True(t, ok, "%q must be present, not omitted", field)
		assert.Equal(t, "[]", string(raw), "%q must serialize as an empty array, not null", field)
	}

	violations, err := ValidateAgainstSchema(data)
	require.NoError(t, err)
	assert.Empty(t, violations,
		"a manifest with no files/chunks/shards must still satisfy the published schema: %s",
		violationsJoined(violations))
}

func violationsJoined(v []string) string {
	out := ""
	for _, s := range v {
		out += s + "\n"
	}
	return out
}
