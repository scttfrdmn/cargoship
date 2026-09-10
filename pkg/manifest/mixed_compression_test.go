package manifest

import (
	"archive/tar"
	"bytes"
	"context"
	"os"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// TestChunkCompression pins the per-chunk decode signal: the key extension wins
// when present (regardless of the manifest's top-level type), and only an
// extensionless key falls back to the manifest type.
func TestChunkCompression(t *testing.T) {
	// Extension is authoritative even when the manifest says otherwise.
	assert.Equal(t, "zstd", chunkCompression("x/chunk-0.tar.zst", "none"))
	assert.Equal(t, "gzip", chunkCompression("x/chunk-0.tar.gz", "zstd"))
	assert.Equal(t, "none", chunkCompression("x/chunk-0.tar", "zstd")) // the #452 case
	// No recognized extension → fall back to the manifest's declared type.
	assert.Equal(t, "zstd", chunkCompression("x/chunk-0", "zstd"))
	assert.Equal(t, "zstd", chunkCompression("x/chunk-0", "")) // historical default
	assert.Equal(t, "none", chunkCompression("x/chunk-0", "none"))
}

// TestRestoreDecodesPlainTarChunkInMixedUpload is the #452 regression guard: a
// plain .tar chunk (incompressible content) must restore even when the manifest's
// top-level compression_type is "zstd" — the mixed-upload case that made files in
// plain chunks unrestorable because the reader decoded by that field instead of
// the chunk's key extension.
func TestRestoreDecodesPlainTarChunkInMixedUpload(t *testing.T) {
	content := []byte("bytes that live in a plain, uncompressed .tar chunk\n")

	// Build a plain tar chunk with stdlib — no zstd wrapper.
	var buf bytes.Buffer
	tw := tar.NewWriter(&buf)
	require.NoError(t, tw.WriteHeader(&tar.Header{Name: "data/x.bin", Mode: 0644, Size: int64(len(content))}))
	_, err := tw.Write(content)
	require.NoError(t, err)
	require.NoError(t, tw.Close())

	key := "backups/uploads/mixed/shard-0/chunk-0.tar" // .tar — a plain chunk
	m := &Manifest{
		Version:         ManifestVersion,
		Bucket:          "test-bucket",
		CompressionType: "zstd", // mixed upload: the top-level field says zstd...
		Chunks:          []ChunkEntry{{ID: 0, S3Key: key, FileCount: 1, FilePaths: []string{"data/x.bin"}}},
		Files:           []FileEntry{{Path: "data/x.bin", Size: int64(len(content)), S3Key: key, ChunkID: 0}},
	}

	dl := &rangeDownloader{object: buf.Bytes(), honorRange: false} // whole-object GET
	se := NewSelectiveExtractor(m, dl, 0).SetBucket("test-bucket")

	dest := t.TempDir()
	stats, err := se.BatchRestore(context.Background(), []string{"data/x.bin"}, dest)
	require.NoError(t, err)
	require.Equal(t, int64(1), stats.Restored, "plain .tar chunk must restore even when manifest.compression_type is zstd")
	require.Zero(t, stats.Failed)

	got, err := os.ReadFile(filepath.Join(dest, "data", "x.bin"))
	require.NoError(t, err)
	assert.Equal(t, content, got)
}
