package manifest

import (
	"context"
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// Regression tests for Issue #228: direct-upload manifests (no chunks — each file
// stored as its own S3 object) must be restorable and verifiable. Before the fix,
// restore treated every object as a tar.zst chunk (extracting nothing) and verify
// failed file_consistency because chunk_id 0 was "out of range" when TotalChunks
// was 0.

// buildDirectUploadManifest builds a direct-upload manifest (TotalChunks == 0)
// with the given file paths, and a mock S3 client serving each file's raw bytes
// at its S3 key. The stored Path is absolute, mirroring the real scanner.
func buildDirectUploadManifest(paths map[string]string) (*Manifest, *mockS3Client) {
	const shardCount = 4
	m := &Manifest{
		Version:         ManifestVersion,
		UploadID:        "direct-test-001",
		CreatedAt:       time.Now(),
		Bucket:          "test-bucket",
		Region:          "us-east-1",
		CompressionType: "none",
		TotalChunks:     0,
		ShardCount:      shardCount,
		// A real direct-upload manifest has ShardCount shard entries (created by
		// Builder.SetShardCount), just with zero chunks each.
		Shards: make([]ShardEntry, shardCount),
	}
	for i := range m.Shards {
		m.Shards[i] = ShardEntry{ID: i, Prefix: filepath.Join("uploads", "direct-test-001", "shard-"+string(rune('0'+i)))}
	}
	client := &mockS3Client{chunks: make(map[string][]byte)}
	for absPath, content := range paths {
		s3Key := "archives/" + filepath.Base(absPath)
		m.Files = append(m.Files, FileEntry{
			Path:    absPath,
			Size:    int64(len(content)),
			ModTime: time.Now(),
			ChunkID: 0, // no chunks exist; this is the direct-upload sentinel
			ShardID: 0,
			S3Key:   s3Key,
		})
		client.chunks[s3Key] = []byte(content)
	}
	m.TotalFiles = int64(len(m.Files))
	m.TotalBytes = func() int64 {
		var t int64
		for _, f := range m.Files {
			t += f.Size
		}
		return t
	}()
	return m, client
}

func TestBatchRestore_DirectUpload_WritesRawFile(t *testing.T) {
	m, client := buildDirectUploadManifest(map[string]string{
		"/abs/src/greeting.txt": "hello cargoship",
		"/abs/src/notes.txt":    "some notes",
	})
	se := NewSelectiveExtractor(m, client, 0)
	dest := t.TempDir()

	// Select by basename — the manifest stores the absolute path, but --file
	// greeting.txt should still resolve. (Issue #228, root cause 2)
	stats, err := se.BatchRestore(context.Background(), []string{"greeting.txt"}, dest)
	require.NoError(t, err)
	assert.Equal(t, int64(1), stats.Restored)
	assert.Equal(t, int64(0), stats.Failed)

	// File written by basename under dest, with the original raw content.
	got, err := os.ReadFile(filepath.Join(dest, "greeting.txt"))
	require.NoError(t, err)
	assert.Equal(t, "hello cargoship", string(got))
}

func TestBatchRestore_DirectUpload_ExactAndSuffixMatch(t *testing.T) {
	m, client := buildDirectUploadManifest(map[string]string{
		"/abs/src/data/report.csv": "a,b,c",
	})
	se := NewSelectiveExtractor(m, client, 0)

	for _, target := range []string{
		"/abs/src/data/report.csv", // exact
		"data/report.csv",          // relative suffix
		"report.csv",               // basename
	} {
		dest := t.TempDir()
		stats, err := se.BatchRestore(context.Background(), []string{target}, dest)
		require.NoError(t, err, "target %q", target)
		assert.Equal(t, int64(1), stats.Restored, "target %q should resolve", target)
	}
}

func TestBatchRestore_DirectUpload_UnknownTargetFails(t *testing.T) {
	m, client := buildDirectUploadManifest(map[string]string{
		"/abs/src/greeting.txt": "hi",
	})
	se := NewSelectiveExtractor(m, client, 0)
	stats, err := se.BatchRestore(context.Background(), []string{"missing.txt"}, t.TempDir())
	require.NoError(t, err)
	assert.Equal(t, int64(0), stats.Restored)
	assert.Equal(t, int64(1), stats.Failed)
}

func TestValidate_DirectUpload_Passes(t *testing.T) {
	m, _ := buildDirectUploadManifest(map[string]string{
		"/abs/src/greeting.txt": "hello",
		"/abs/src/notes.txt":    "notes",
	})
	result := NewValidator(m).Validate()
	// A direct-upload manifest (no chunks, files carry S3 keys) must not fail
	// file_consistency. (Issue #228, root cause 3)
	assert.True(t, result.Checks["file_consistency"],
		"file_consistency should pass for a direct-upload manifest")
	assert.False(t, result.HasErrors(), "no hard errors expected: %s", result.Summary())
}

func TestValidate_DirectUpload_MissingS3KeyFails(t *testing.T) {
	m, _ := buildDirectUploadManifest(map[string]string{
		"/abs/src/greeting.txt": "hello",
	})
	m.Files[0].S3Key = "" // a direct-upload file with no object key is invalid
	result := NewValidator(m).Validate()
	assert.False(t, result.Checks["file_consistency"],
		"a direct-upload file without an S3 key should fail validation")
}
