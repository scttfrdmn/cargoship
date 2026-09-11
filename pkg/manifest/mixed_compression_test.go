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

	// The exported wrapper (used by the rebalance path, #482) must agree — so
	// `balance --execute` decodes a mixed upload's plain .tar chunk as plain tar
	// instead of wrapping a zstd reader around it.
	assert.Equal(t, "none", ChunkCompression("x/chunk-0.tar", "zstd"))
	assert.Equal(t, "zstd", ChunkCompression("x/chunk-0.tar.zst", "none"))
}

// TestVerifyFilesDuplicateChunkIDsAcrossShards is the #455 regression guard:
// chunk IDs repeat across shards in a multi-prefix upload, so verify must key on
// the (ShardID, ID) composite. Keying on ID alone collapsed distinct chunks —
// verify hashed files against the wrong object and flagged a bogus duplicate.
func TestVerifyFilesDuplicateChunkIDsAcrossShards(t *testing.T) {
	a := []byte("bytes that live in shard 0's chunk 0")
	b := []byte("different bytes in shard 1's chunk 0")
	c0 := makeTarZst(t, map[string][]byte{"a.txt": a})
	c1 := makeTarZst(t, map[string][]byte{"b.txt": b})
	dl := &fakeDownloader{objects: map[string][]byte{
		"shard-0/chunk-0.tar.zst": c0,
		"shard-1/chunk-0.tar.zst": c1,
	}}
	m := &Manifest{
		Version: ManifestVersion, Bucket: "bkt",
		CompressionType: "zstd", ChecksumAlgorithm: ChecksumAlgorithmSHA256,
		ShardCount: 2, TotalChunks: 2,
		Chunks: []ChunkEntry{
			{ID: 0, ShardID: 0, S3Key: "shard-0/chunk-0.tar.zst"},
			{ID: 0, ShardID: 1, S3Key: "shard-1/chunk-0.tar.zst"}, // SAME id, different shard
		},
		Files: []FileEntry{
			{Path: "a.txt", ChunkID: 0, ShardID: 0, S3Key: "shard-0/chunk-0.tar.zst", Checksum: sha256hex(a)},
			{Path: "b.txt", ChunkID: 0, ShardID: 1, S3Key: "shard-1/chunk-0.tar.zst", Checksum: sha256hex(b)},
		},
	}

	// The composite identity means these are NOT duplicates.
	require.True(t, NewValidator(m).Validate().Checks["chunk_consistency"],
		"distinct shards with the same chunk id must not be flagged as duplicates")

	// Each file must verify against ITS OWN chunk, not the first chunk with id 0.
	res, err := NewDeepVerifier(m, dl).VerifyFiles(context.Background())
	require.NoError(t, err)
	assert.Equal(t, 2, res.OK, "both files should verify against their correct chunk")
	assert.True(t, res.Passed())
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
