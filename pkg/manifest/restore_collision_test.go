package manifest

import (
	"context"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// sameBasenameManifest describes one chunk holding two files whose basenames
// match, so --flatten maps both to the same output path.
func sameBasenameManifest(t *testing.T, a, b string) (*Manifest, *mockS3Client) {
	t.Helper()
	const key = "uploads/u/shard-0/chunk-0.tar.zst"
	m := &Manifest{
		Version: ManifestVersion, UploadID: "collide", CreatedAt: time.Now(),
		Bucket: "test-bucket", CompressionType: "zstd",
		Chunks: []ChunkEntry{{ID: 0, S3Key: key}},
		Files: []FileEntry{
			{Path: a, Size: 40, ChunkID: 0, S3Key: key},
			{Path: b, Size: 40, ChunkID: 0, S3Key: key},
		},
	}
	return m, &mockS3Client{chunks: map[string][]byte{key: buildZstdTar(t, m.Files)}}
}

func dirEntries(t *testing.T, dir string) []string {
	t.Helper()
	var out []string
	require.NoError(t, filepath.Walk(dir, func(p string, info os.FileInfo, err error) error {
		if err == nil && !info.IsDir() {
			rel, _ := filepath.Rel(dir, p)
			out = append(out, rel)
		}
		return err
	}))
	return out
}

// TestBatchRestore_FlattenCollisionIsRefused pins #908: --flatten reduced both
// files to config.txt with no collision check, so restore could report both
// restored with one on disk (exit 0, a file silently gone).
func TestBatchRestore_FlattenCollisionIsRefused(t *testing.T) {
	m, client := sameBasenameManifest(t, "src/a/config.txt", "src/b/config.txt")
	dest := t.TempDir()
	se := NewSelectiveExtractor(m, client, 0).SetFlatten(true)

	_, err := se.BatchRestore(context.Background(), []string{"src/a/config.txt", "src/b/config.txt"}, dest)
	require.Error(t, err, "two files restoring to one path must be refused")
	assert.Contains(t, err.Error(), "src/a/config.txt")
	assert.Contains(t, err.Error(), "src/b/config.txt")
	assert.Contains(t, err.Error(), "config.txt")
	assert.Empty(t, dirEntries(t, dest), "nothing may be written when the restore is refused")
}

func TestBatchRestore_SameBasenameWithoutFlattenRestoresBoth(t *testing.T) {
	m, client := sameBasenameManifest(t, "src/a/config.txt", "src/b/config.txt")
	dest := t.TempDir()
	stats, err := NewSelectiveExtractor(m, client, 0).
		BatchRestore(context.Background(), []string{"src/a/config.txt", "src/b/config.txt", "src/a/config.txt"}, dest)
	require.NoError(t, err, "distinct output paths (and a repeated target) are not collisions")
	assert.Zero(t, stats.Failed, "reasons: %v", stats.Errors)
	assert.Len(t, dirEntries(t, dest), 2)
}

// TestBatchRestore_CaseOnlyCollision: README and readme are two files on a
// case-sensitive source, but one file on macOS or Windows. Refused only where
// the destination filesystem ignores case.
func TestBatchRestore_CaseOnlyCollision(t *testing.T) {
	m, client := sameBasenameManifest(t, "docs/README", "docs/readme")
	dest := t.TempDir()
	root, err := os.OpenRoot(dest)
	require.NoError(t, err)
	fold := caseInsensitive(root)
	_ = root.Close()

	_, err = NewSelectiveExtractor(m, client, 0).
		BatchRestore(context.Background(), []string{"docs/README", "docs/readme"}, dest)
	if fold {
		require.Error(t, err, "a case-only collision must be refused on a case-insensitive filesystem")
		assert.Contains(t, err.Error(), "ignores case")
		assert.Empty(t, dirEntries(t, dest))
	} else {
		require.NoError(t, err, "a case-sensitive filesystem holds both")
		assert.Len(t, dirEntries(t, dest), 2)
	}
}

func TestCheckOutputCollisions_SplitPartsAndLimits(t *testing.T) {
	dest := t.TempDir()
	root, err := os.OpenRoot(dest)
	require.NoError(t, err)
	defer func() { _ = root.Close() }()
	se := NewSelectiveExtractor(&Manifest{}, &mockS3Client{}, 0).SetFlatten(true)

	parts := []*FileEntry{
		{Path: "big.bin", PartIndex: 0, TotalParts: 2},
		{Path: "big.bin", PartIndex: 1, TotalParts: 2},
		nil, // unresolved target
	}
	assert.NoError(t, se.checkOutputCollisions(root, dest, parts), "parts of one file share a path legitimately")

	var many []*FileEntry
	for i := 0; i < 15; i++ {
		many = append(many, &FileEntry{Path: filepath.Join("d", string(rune('a'+i)), "same.txt")})
	}
	err = se.checkOutputCollisions(root, dest, many)
	require.Error(t, err)
	assert.Contains(t, err.Error(), "14 output path collision(s)")
	assert.Equal(t, maxListedCollisions, strings.Count(err.Error(), "both restore to"), "the list is capped")

	assert.Empty(t, dirEntries(t, dest), "the case probe must clean up after itself")
}
