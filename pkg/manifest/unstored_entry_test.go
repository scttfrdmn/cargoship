package manifest

import (
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// TestComputeDelta_UnstoredEntryIsModified pins #902's delta half: a manifest entry
// with no S3Key was never stored, so an unchanged local file must still be uploaded.
func TestComputeDelta_UnstoredEntryIsModified(t *testing.T) {
	mod := time.Unix(1_700_000_000, 0).UTC()
	prev := &Manifest{Files: []FileEntry{
		{Path: "stored.txt", Size: 5, ModTime: mod, S3Key: "p/uploads/v1/shard-0/chunk-0.tar.zst"},
		{Path: "failed.txt", Size: 5, ModTime: mod}, // its chunk failed
	}}
	local := []FileInfo{
		{Path: "stored.txt", Size: 5, ModTime: mod},
		{Path: "failed.txt", Size: 5, ModTime: mod},
	}

	d, err := ComputeDelta(local, prev, &SyncOptions{})
	require.NoError(t, err)
	require.Len(t, d.Same, 1)
	assert.Equal(t, "stored.txt", d.Same[0].Path)
	require.Len(t, d.Modified, 1, "an unstored entry must be uploaded again")
	assert.Equal(t, "failed.txt", d.Modified[0].Path)
}

// TestMergeChain_UnstoredEntryDoesNotShadowOlderVersion pins #902's merge half:
// newest-mention-wins must not let a never-stored entry hide the previous version's
// restorable copy, and a path stored nowhere drops out rather than resolving to
// nothing.
func TestMergeChain_UnstoredEntryDoesNotShadowOlderVersion(t *testing.T) {
	v1Key := "p/uploads/v1/shard-0/chunk-0.tar.zst"
	v1 := &Manifest{
		UploadID: "v1",
		Files: []FileEntry{
			fileEntry("a.txt", v1Key, 10),
			fileEntry("b.txt", v1Key, 20),
		},
		Chunks: []ChunkEntry{{ID: 0, S3Key: v1Key}},
	}
	v2 := &Manifest{
		UploadID:           "v2",
		PreviousManifestID: "v1",
		SyncType:           SyncTypeIncremental,
		Files: []FileEntry{
			fileEntry("a.txt", "", 11),   // changed, but its chunk failed
			fileEntry("new.txt", "", 30), // new, and its chunk failed
		},
	}

	eff := MergeChain([]*Manifest{v2, v1})
	byPath := map[string]FileEntry{}
	for _, f := range eff.Files {
		byPath[f.Path] = f
	}
	require.Contains(t, byPath, "a.txt")
	assert.Equal(t, v1Key, byPath["a.txt"].S3Key, "v1's stored copy must survive")
	assert.Equal(t, int64(10), byPath["a.txt"].Size)
	assert.Contains(t, byPath, "b.txt")
	assert.NotContains(t, byPath, "new.txt", "a path stored in no version is not in the dataset")
	require.Len(t, eff.Chunks, 1)
	assert.Equal(t, v1Key, eff.Chunks[0].S3Key)
}

// TestNewBuilderFromExisting_DropsUnstoredEntries pins #904: a partial manifest's
// entries for files whose chunk never uploaded carry the old run's chunk IDs, which
// mean nothing to the resumed run. They are dropped, and totals follow.
func TestNewBuilderFromExisting_DropsUnstoredEntries(t *testing.T) {
	b, err := NewBuilderFromExisting(&Manifest{
		TotalFiles: 3, TotalBytes: 60,
		Files: []FileEntry{
			{Path: "stored", Size: 10, ChunkID: 0, S3Key: "uploads/u/shard-0/chunk-0.tar.zst"},
			{Path: "pending", Size: 50, ChunkID: 4},
		},
	})
	require.NoError(t, err)
	m := b.Build()
	require.Len(t, m.Files, 1)
	assert.Equal(t, "stored", m.Files[0].Path)
	assert.Equal(t, int64(1), m.TotalFiles)
	assert.Equal(t, int64(10), m.TotalBytes)

	// The re-scan records the pending file afresh instead of skipping it as seen.
	b.AddFile(FileEntry{Path: "pending", Size: 50, ChunkID: 9})
	m = b.Build()
	require.Len(t, m.Files, 2)
	assert.Equal(t, 9, m.Files[1].ChunkID)
}

// TestBindChunkFiles pins #904's binding: by path and part, never by chunk ID.
func TestBindChunkFiles(t *testing.T) {
	old := time.Unix(1_700_000_000, 0).UTC()
	now := old.Add(time.Hour)
	b, err := NewBuilderFromExisting(&Manifest{Files: []FileEntry{
		// Preloaded, stored in the OLD run's chunk 2, with that copy's checksum.
		{Path: "a", Size: 1, ModTime: old, ChunkID: 2, S3Key: "uploads/u/shard-0/chunk-2.tar.zst", Checksum: "old", ArchiveOffset: 512},
		// Preloaded, stored elsewhere, and NOT in the new chunk, though it shares
		// the new chunk's ID: binding by ID would have re-pointed it.
		{Path: "b", Size: 2, ModTime: old, ChunkID: 5, S3Key: "uploads/u/shard-1/chunk-5.tar.zst"},
	}})
	require.NoError(t, err)
	b.AddFile(FileEntry{Path: "c", Size: 3, ModTime: now, ChunkID: 5})

	b.BindChunkFiles(5, 3, "uploads/u/shard-3/chunk-5x.tar.zst", []ChunkFile{
		{Path: "a", Size: 4, ModTime: now},
		{Path: "c", Size: 3, ModTime: now},
	})
	b.BindChunkFiles(9, 0, "unused", nil) // no-op

	m := b.Build()
	byPath := map[string]FileEntry{}
	for _, f := range m.Files {
		byPath[f.Path] = f
	}
	assert.Equal(t, "uploads/u/shard-3/chunk-5x.tar.zst", byPath["a"].S3Key)
	assert.Equal(t, 5, byPath["a"].ChunkID)
	assert.Equal(t, 3, byPath["a"].ShardID)
	assert.Equal(t, int64(4), byPath["a"].Size, "size follows the new scan")
	assert.True(t, byPath["a"].ModTime.Equal(now))
	assert.Empty(t, byPath["a"].Checksum, "the old copy's checksum must not survive")
	assert.Zero(t, byPath["a"].ArchiveOffset)
	assert.Equal(t, "uploads/u/shard-1/chunk-5.tar.zst", byPath["b"].S3Key, "a file not in the chunk is untouched")
	assert.Equal(t, "uploads/u/shard-3/chunk-5x.tar.zst", byPath["c"].S3Key)
	assert.Equal(t, int64(4+2+3), m.TotalBytes)
}
