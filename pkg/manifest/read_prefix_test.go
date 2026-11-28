package manifest

import (
	"context"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestResolveObjectKeyAt(t *testing.T) {
	tests := []struct {
		name              string
		recorded, at, key string
		want              string
	}{
		{"same prefix is ResolveObjectKey", "orig", "orig", "uploads/u/c.tar.zst", "orig/uploads/u/c.tar.zst"},
		{"relative key moves", "orig", "copy", "uploads/u/c.tar.zst", "copy/uploads/u/c.tar.zst"},
		{"prefix-scoped key moves, not doubled", "orig", "copy", "orig/uploads/u/c.tar.zst", "copy/uploads/u/c.tar.zst"},
		{"full URL key moves", "orig", "a/b", "http://host/bkt/orig/uploads/u/c.tar.zst", "a/b/uploads/u/c.tar.zst"},
		{"bucket-qualified key moves", "orig", "copy", "bkt/orig/uploads/u/c.tar.zst", "copy/uploads/u/c.tar.zst"},
		{"to the bucket root", "orig", "", "orig/uploads/u/c.tar.zst", "uploads/u/c.tar.zst"},
		{"from the bucket root", "", "copy", "uploads/u/c.tar.zst", "copy/uploads/u/c.tar.zst"},
		{"slashes ignored", "orig/", "/copy/", "uploads/u/c.tar.zst", "copy/uploads/u/c.tar.zst"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			assert.Equal(t, tt.want, ResolveObjectKeyAt(tt.recorded, tt.at, "bkt", tt.key))
		})
	}
}

// TestSetPrefix_FetchesFromTheReadPrefix pins #1032 at the unit level: restore
// and deep verify fetch under the prefix they were given, not the manifest's.
func TestSetPrefix_FetchesFromTheReadPrefix(t *testing.T) {
	content := []byte("cargoship-read-prefix")
	chunk := oneFileTarZst(t, "/abs/src/f.bin", content)
	m := singleFileManifest("/abs/src/f.bin", len(content)) // records Prefix "p"
	m.ChecksumAlgorithm = ChecksumAlgorithmSHA256

	t.Run("restore", func(t *testing.T) {
		client := &mockS3Client{chunks: map[string][]byte{"moved/chunk-0.tar.zst": chunk}}
		se := NewSelectiveExtractor(m, client, 0).SetPrefix("moved")
		assert.Equal(t, []string{"moved/chunk-0.tar.zst"}, se.ChunkKeysForPaths([]string{"/abs/src/f.bin"}),
			"the Glacier pre-flight must check the same keys the restore reads")
		stats, err := se.BatchRestore(context.Background(), []string{"/abs/src/f.bin"}, t.TempDir())
		require.NoError(t, err)
		assert.Equal(t, int64(1), stats.Restored, "reasons: %v", stats.Errors)
		assert.Equal(t, []string{"moved/chunk-0.tar.zst"}, client.requestedKeys())
	})

	t.Run("restore at the bucket root", func(t *testing.T) {
		client := &mockS3Client{chunks: map[string][]byte{"chunk-0.tar.zst": chunk}}
		stats, err := NewSelectiveExtractor(m, client, 0).SetPrefix("").BatchRestore(context.Background(), []string{"/abs/src/f.bin"}, t.TempDir())
		require.NoError(t, err)
		assert.Equal(t, int64(1), stats.Restored, "reasons: %v", stats.Errors)
	})

	t.Run("deep verify", func(t *testing.T) {
		dl := &fakeDownloader{objects: map[string][]byte{"moved/chunk-0.tar.zst": chunk}}
		res, err := NewDeepVerifier(m, dl).SetPrefix("moved").VerifyFiles(context.Background())
		require.NoError(t, err)
		assert.Zero(t, res.Missing, "the chunk must be read from the prefix the manifest was read from")

		res, err = NewDeepVerifier(m, dl).VerifyFiles(context.Background())
		require.NoError(t, err)
		assert.Positive(t, res.Missing, "without SetPrefix the recorded prefix is used")
	})
}
