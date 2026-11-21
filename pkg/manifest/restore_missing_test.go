package manifest

import (
	"context"
	"strings"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// twoFileChunkManifest describes one chunk holding a.txt and b.txt.
func twoFileChunkManifest() *Manifest {
	const key = "uploads/u/shard-0/chunk-0.tar.zst"
	return &Manifest{
		Version:         ManifestVersion,
		UploadID:        "missing-test",
		CreatedAt:       time.Now(),
		Bucket:          "test-bucket",
		CompressionType: "zstd",
		Chunks:          []ChunkEntry{{ID: 0, S3Key: key}},
		Files: []FileEntry{
			{Path: "data/a.txt", Size: 40, ChunkID: 0, S3Key: key},
			{Path: "data/b.txt", Size: 40, ChunkID: 0, S3Key: key},
		},
	}
}

func hasReason(stats *RestoreStats, substr string) bool {
	for _, e := range stats.Errors {
		if strings.Contains(e, substr) {
			return true
		}
	}
	return false
}

// TestBatchRestore_EntryMissingFromChunkIsAFailure pins #905: a chunk that ends
// cleanly without a requested entry used to return nil, so restore reported
// Failed=0 and exited 0 with the file silently absent.
func TestBatchRestore_EntryMissingFromChunkIsAFailure(t *testing.T) {
	m := twoFileChunkManifest()
	// The stored chunk holds only a.txt: b.txt is in the manifest but not the chunk.
	client := &mockS3Client{chunks: map[string][]byte{
		m.Chunks[0].S3Key: buildZstdTar(t, []FileEntry{m.Files[0]}),
	}}
	se := NewSelectiveExtractor(m, client, 0)

	stats, err := se.BatchRestore(context.Background(), []string{"data/a.txt", "data/b.txt"}, t.TempDir())
	require.NoError(t, err)
	assert.Equal(t, int64(1), stats.Restored)
	assert.Equal(t, int64(1), stats.Failed, "a requested file absent from its chunk must count as failed")
	assert.True(t, hasReason(stats, "missing from chunk"), "the reason must name the cause: %v", stats.Errors)
	assert.True(t, hasReason(stats, "data/b.txt"), "the reason must name the missing file: %v", stats.Errors)
}

// TestBatchRestore_ShortChunkObjectIsAFailure pins #905's download-side check:
// an object smaller than the manifest's CompressedSize (an empty chunk left by
// an upload retry that re-sent a drained stream, #901) must fail every file in
// it, not decode to "nothing to restore".
func TestBatchRestore_ShortChunkObjectIsAFailure(t *testing.T) {
	m := twoFileChunkManifest()
	full := buildZstdTar(t, m.Files)
	m.Chunks[0].CompressedSize = int64(len(full))
	m.Chunks[0].Checksum = sha256hex(full) // proves the size was counted (see trustedChunkSize)

	for name, stored := range map[string][]byte{
		"empty":     {},
		"truncated": full[:len(full)/2],
	} {
		t.Run(name, func(t *testing.T) {
			client := &mockS3Client{chunks: map[string][]byte{m.Chunks[0].S3Key: stored}}
			se := NewSelectiveExtractor(m, client, 0)

			stats, err := se.BatchRestore(context.Background(), []string{"data/a.txt", "data/b.txt"}, t.TempDir())
			require.NoError(t, err)
			assert.Equal(t, int64(0), stats.Restored)
			assert.Equal(t, int64(2), stats.Failed)
			assert.True(t, hasReason(stats, "truncated or incomplete upload"), "reasons: %v", stats.Errors)
		})
	}
}

// TestBatchRestore_ShortDirectObjectIsAFailure pins #905 for direct uploads: a
// file with no recorded checksum whose object is shorter than its recorded size
// used to be written short and counted as restored.
func TestBatchRestore_ShortDirectObjectIsAFailure(t *testing.T) {
	m, client := buildDirectUploadManifest(map[string]string{"/src/notes.txt": "the full contents of the file"})
	key := m.Files[0].S3Key
	client.chunks[key] = []byte("the full")
	se := NewSelectiveExtractor(m, client, 0)

	stats, err := se.BatchRestore(context.Background(), []string{m.Files[0].Path}, t.TempDir())
	require.NoError(t, err)
	assert.Equal(t, int64(0), stats.Restored, "a short direct object must not be restored as if complete")
	assert.Equal(t, int64(1), stats.Failed)
	assert.NotEmpty(t, stats.Errors)
}

// TestBatchRestore_UnknownTargetRecordsReason: a target not in the manifest was
// already counted as failed, but silently. It now says which one.
func TestBatchRestore_UnknownTargetRecordsReason(t *testing.T) {
	m := twoFileChunkManifest()
	client := &mockS3Client{chunks: map[string][]byte{m.Chunks[0].S3Key: buildZstdTar(t, m.Files)}}
	se := NewSelectiveExtractor(m, client, 0)

	stats, err := se.BatchRestore(context.Background(), []string{"nope.txt"}, t.TempDir())
	require.NoError(t, err)
	assert.Equal(t, int64(1), stats.Failed)
	assert.True(t, hasReason(stats, "nope.txt: not found in manifest"), "reasons: %v", stats.Errors)
}

func TestNoteFailure_CapsRecordedErrors(t *testing.T) {
	se := &SelectiveExtractor{}
	stats := &RestoreStats{}
	for i := 0; i < maxRecordedErrors+10; i++ {
		se.noteFailure(stats, 1, assert.AnError)
	}
	se.noteFailure(stats, 0, assert.AnError) // n <= 0 is a no-op
	assert.Equal(t, int64(maxRecordedErrors+10), stats.Failed)
	assert.Len(t, stats.Errors, maxRecordedErrors)
}

// TestTrustedChunkSize pins when a recorded CompressedSize may be used as a lower
// bound: v0.16-v0.18 manifests (format 2.0) stored the uncompressed estimate, and
// an unhashed chunk may still carry it, so neither is trusted.
func TestTrustedChunkSize(t *testing.T) {
	mk := func(version, checksum string, frames int) *SelectiveExtractor {
		c := ChunkEntry{S3Key: "k", CompressedSize: 100, Checksum: checksum}
		for i := 0; i < frames; i++ {
			c.Frames = append(c.Frames, FrameEntry{})
		}
		return &SelectiveExtractor{manifest: &Manifest{Version: version, Chunks: []ChunkEntry{c}}}
	}
	tests := []struct {
		name string
		se   *SelectiveExtractor
		want int64
	}{
		{"2.0 with checksum (pre-#445)", mk("2.0", "abc", 0), 0},
		{"2.1 with checksum", mk("2.1", "abc", 0), 100},
		{"2.2 framed", mk("2.2", "", 2), 100},
		{"2.2 neither (estimate fallback)", mk("2.2", "", 0), 0},
		{"3.0 with checksum", mk("3.0", "abc", 0), 100},
		{"garbage version", mk("x", "abc", 0), 0},
	}
	for _, tt := range tests {
		if got := tt.se.trustedChunkSize("k"); got != tt.want {
			t.Errorf("%s: got %d, want %d", tt.name, got, tt.want)
		}
	}
	if got := mk("2.2", "abc", 0).trustedChunkSize("other"); got != 0 {
		t.Errorf("unknown key: got %d", got)
	}
}
