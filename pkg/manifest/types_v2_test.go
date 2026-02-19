package manifest

import (
	"encoding/json"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// TestManifestVersion verifies the version constant was bumped.
func TestManifestVersion(t *testing.T) {
	assert.Equal(t, "2.0", ManifestVersion)
	assert.Equal(t, "1.0", ManifestVersionV1)
}

// TestManifestV2Fields verifies new v2 fields round-trip through JSON.
func TestManifestV2Fields(t *testing.T) {
	m := &Manifest{
		Version:  ManifestVersion,
		UploadID: "test-v2-123",
		VersionInfo: &VersionInfo{
			DataVersion:  "v1.0.0",
			ExperimentID: "exp-abc123",
			Tag:          "baseline",
		},
		GitMetadata: &GitMetadata{
			Commit: "abc123def456abc123def456abc123def456abc1",
			Branch: "main",
			Tag:    "v1.0.0",
			Remote: "https://github.com/org/repo.git",
			Dirty:  false,
		},
		DVCCompatibility: &DVCCompatibility{
			Enabled:           true,
			DVCVersion:        "3.51.2",
			CacheDir:          ".dvc/cache",
			DVCFilesGenerated: true,
		},
	}

	data, err := json.Marshal(m)
	require.NoError(t, err)

	var decoded Manifest
	require.NoError(t, json.Unmarshal(data, &decoded))

	require.NotNil(t, decoded.VersionInfo)
	assert.Equal(t, "v1.0.0", decoded.VersionInfo.DataVersion)
	assert.Equal(t, "exp-abc123", decoded.VersionInfo.ExperimentID)
	assert.Equal(t, "baseline", decoded.VersionInfo.Tag)

	require.NotNil(t, decoded.GitMetadata)
	assert.Equal(t, "abc123def456abc123def456abc123def456abc1", decoded.GitMetadata.Commit)
	assert.Equal(t, "main", decoded.GitMetadata.Branch)
	assert.Equal(t, "v1.0.0", decoded.GitMetadata.Tag)
	assert.Equal(t, "https://github.com/org/repo.git", decoded.GitMetadata.Remote)
	assert.False(t, decoded.GitMetadata.Dirty)

	require.NotNil(t, decoded.DVCCompatibility)
	assert.True(t, decoded.DVCCompatibility.Enabled)
	assert.Equal(t, "3.51.2", decoded.DVCCompatibility.DVCVersion)
	assert.Equal(t, ".dvc/cache", decoded.DVCCompatibility.CacheDir)
	assert.True(t, decoded.DVCCompatibility.DVCFilesGenerated)
}

// TestFileEntryV2Fields verifies ContentHash and DVCMetadata round-trip through JSON.
func TestFileEntryV2Fields(t *testing.T) {
	entry := FileEntry{
		Path:        "data/train.csv",
		Size:        1024,
		ModTime:     time.Now().Truncate(time.Second),
		ContentHash: "d8e8fca2dc0f896fd7cb4cb0031ba249",
		DVCMetadata: &DVCMetadata{
			Stage:        "preprocess",
			Pipeline:     "train",
			ExperimentID: "exp-xyz789",
		},
	}

	data, err := json.Marshal(entry)
	require.NoError(t, err)

	var decoded FileEntry
	require.NoError(t, json.Unmarshal(data, &decoded))

	assert.Equal(t, "d8e8fca2dc0f896fd7cb4cb0031ba249", decoded.ContentHash)
	require.NotNil(t, decoded.DVCMetadata)
	assert.Equal(t, "preprocess", decoded.DVCMetadata.Stage)
	assert.Equal(t, "train", decoded.DVCMetadata.Pipeline)
	assert.Equal(t, "exp-xyz789", decoded.DVCMetadata.ExperimentID)
}

// TestFileEntryV2OmitEmpty verifies new fields are omitted when zero-valued.
func TestFileEntryV2OmitEmpty(t *testing.T) {
	entry := FileEntry{
		Path: "file.txt",
		Size: 100,
	}

	data, err := json.Marshal(entry)
	require.NoError(t, err)

	// content_hash and dvc_metadata must not appear when unset
	assert.NotContains(t, string(data), "content_hash")
	assert.NotContains(t, string(data), "dvc_metadata")
}

// TestManifestV1BackwardCompat verifies a v1.0 manifest (no v2 fields) unmarshals cleanly.
func TestManifestV1BackwardCompat(t *testing.T) {
	v1JSON := `{
		"version": "1.0",
		"upload_id": "old-upload-abc",
		"created_at": "2025-01-01T00:00:00Z",
		"completed_at": "2025-01-01T00:05:00Z",
		"source_path": "/data",
		"hostname": "workstation",
		"bucket": "my-bucket",
		"prefix": "backups",
		"region": "us-west-2",
		"total_files": 3,
		"total_bytes": 300,
		"total_chunks": 1,
		"shard_count": 4,
		"compression_type": "zstd",
		"compression_level": 3,
		"compression_ratio": 0.55,
		"files": [
			{"path": "a.txt", "size": 100, "mod_time": "2025-01-01T00:00:00Z", "chunk_id": 0, "shard_id": 0, "s3_key": "shard-0/chunk-0.tar.zst"},
			{"path": "b.txt", "size": 100, "mod_time": "2025-01-01T00:00:00Z", "chunk_id": 0, "shard_id": 0, "s3_key": "shard-0/chunk-0.tar.zst"},
			{"path": "c.txt", "size": 100, "mod_time": "2025-01-01T00:00:00Z", "chunk_id": 0, "shard_id": 0, "s3_key": "shard-0/chunk-0.tar.zst"}
		],
		"chunks": [],
		"shards": []
	}`

	m, err := FromJSON([]byte(v1JSON))
	require.NoError(t, err, "v1.0 manifest must unmarshal without error")

	assert.Equal(t, "1.0", m.Version)
	assert.Equal(t, "old-upload-abc", m.UploadID)
	assert.Equal(t, int64(3), m.TotalFiles)
	assert.Len(t, m.Files, 3)

	// v2 fields must be nil — not present in v1 JSON
	assert.Nil(t, m.VersionInfo, "VersionInfo should be nil for v1 manifest")
	assert.Nil(t, m.GitMetadata, "GitMetadata should be nil for v1 manifest")
	assert.Nil(t, m.DVCCompatibility, "DVCCompatibility should be nil for v1 manifest")

	// Per-file v2 fields must be zero-valued
	for _, f := range m.Files {
		assert.Empty(t, f.ContentHash, "ContentHash should be empty for v1 file entries")
		assert.Nil(t, f.DVCMetadata, "DVCMetadata should be nil for v1 file entries")
	}
}

// TestManifestV2OmitEmpty verifies new Manifest-level v2 fields are omitted when nil.
func TestManifestV2OmitEmpty(t *testing.T) {
	m := &Manifest{
		Version:  ManifestVersion,
		UploadID: "no-dvc-upload",
	}

	data, err := json.Marshal(m)
	require.NoError(t, err)

	s := string(data)
	assert.NotContains(t, s, "version_info")
	assert.NotContains(t, s, "git_metadata")
	assert.NotContains(t, s, "dvc_compatibility")
}

// TestNewBuilderFromExistingCopiesV2Fields verifies the builder clone preserves v2 fields.
func TestNewBuilderFromExistingCopiesV2Fields(t *testing.T) {
	original := &Manifest{
		Version:    ManifestVersion,
		UploadID:   "original",
		SourcePath: "/data",
		Bucket:     "b",
		Prefix:     "p",
		Region:     "us-east-1",
		VersionInfo: &VersionInfo{
			DataVersion: "v2.0",
			Tag:         "experiment-1",
		},
		GitMetadata: &GitMetadata{
			Commit: "deadbeef",
			Branch: "feature/dvc",
		},
		DVCCompatibility: &DVCCompatibility{
			Enabled:    true,
			DVCVersion: "3.50.0",
		},
		Files:  []FileEntry{},
		Chunks: []ChunkEntry{},
		Shards: []ShardEntry{},
	}

	builder, err := NewBuilderFromExisting(original)
	require.NoError(t, err)

	cloned := builder.Build()

	require.NotNil(t, cloned.VersionInfo)
	assert.Equal(t, "v2.0", cloned.VersionInfo.DataVersion)
	assert.Equal(t, "experiment-1", cloned.VersionInfo.Tag)

	require.NotNil(t, cloned.GitMetadata)
	assert.Equal(t, "deadbeef", cloned.GitMetadata.Commit)
	assert.Equal(t, "feature/dvc", cloned.GitMetadata.Branch)

	require.NotNil(t, cloned.DVCCompatibility)
	assert.True(t, cloned.DVCCompatibility.Enabled)
	assert.Equal(t, "3.50.0", cloned.DVCCompatibility.DVCVersion)
}

// TestDVCMetadataZeroValue verifies DVCMetadata with only zero fields serializes cleanly.
func TestDVCMetadataZeroValue(t *testing.T) {
	m := &DVCMetadata{}
	data, err := json.Marshal(m)
	require.NoError(t, err)
	// All omitempty fields should be absent
	assert.Equal(t, "{}", string(data))
}

// TestGitMetadataDirtyField verifies the Dirty bool serializes when true but omits when false.
func TestGitMetadataDirtyField(t *testing.T) {
	clean := &GitMetadata{Commit: "abc", Dirty: false}
	dirty := &GitMetadata{Commit: "abc", Dirty: true}

	cleanData, err := json.Marshal(clean)
	require.NoError(t, err)
	assert.NotContains(t, string(cleanData), "dirty", "dirty=false should be omitted")

	dirtyData, err := json.Marshal(dirty)
	require.NoError(t, err)
	assert.Contains(t, string(dirtyData), "dirty", "dirty=true should be present")
}
