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
