package manifest

import (
	"bytes"
	"context"
	"os"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// TestFrameRestore_ManifestSizeLargerThanTarEntry pins #910's restore half. An
// archive written before #910 could record the scan-time size of a file that
// shrank before it was archived. Frame restore sliced that size, so with no
// per-file checksum it "restored" the file padded with the next tar header and
// the next file's bytes: corruption plus a cross-file leak, reported as success.
// The tar header's own size now bounds the slice.
func TestFrameRestore_ManifestSizeLargerThanTarEntry(t *testing.T) {
	files := []framedFile{
		{"data/a.txt", bytes.Repeat([]byte("a"), 2000)},
		{"data/b.txt", bytes.Repeat([]byte("SECRET-B"), 500)},
	}
	key := "backups/uploads/20260909-frame/shard-0/chunk-0.tar.zst"
	chunkBytes, frames, offsets := buildStdlibFramedChunk(t, files, 1<<20) // one frame holds both
	m := framedManifest(key, files, frames, offsets)
	m.Files[0].Size = 3000 // the scan saw a.txt at 3000 bytes; 2000 were archived

	se := NewSelectiveExtractor(m, &rangeDownloader{object: chunkBytes, honorRange: true}, 0).SetBucket("test-bucket")
	dest := t.TempDir()
	stats, err := se.BatchRestore(context.Background(), []string{"data/a.txt"}, dest)
	require.NoError(t, err)
	assert.Zero(t, stats.Restored, "a size disagreement must not restore")
	assert.Equal(t, int64(1), stats.Failed)
	assert.True(t, hasReason(stats, "archive holds 2000 bytes"), "reasons: %v", stats.Errors)

	got, err := os.ReadFile(filepath.Join(dest, "data", "a.txt"))
	if err == nil {
		assert.NotContains(t, string(got), "SECRET-B", "another file's bytes must never be written")
	}
}

func TestTarHeaderSizeBefore(t *testing.T) {
	files := []framedFile{{"x", bytes.Repeat([]byte("x"), 700)}}
	chunk, frames, offsets := buildStdlibFramedChunk(t, files, 1<<20)
	raw, err := decodeZstdFrame(chunk[frames[0].CompressedOffset : frames[0].CompressedOffset+frames[0].CompressedSize])
	require.NoError(t, err)

	size, ok := tarHeaderSizeBefore(raw, offsets["x"])
	require.True(t, ok)
	assert.Equal(t, int64(700), size)

	_, ok = tarHeaderSizeBefore(raw, 100) // no room for a header
	assert.False(t, ok)
	_, ok = tarHeaderSizeBefore(raw, offsets["x"]+512) // inside file data, not a header
	assert.False(t, ok)
	_, ok = tarHeaderSizeBefore(raw, int64(len(raw))+1)
	assert.False(t, ok)
}
