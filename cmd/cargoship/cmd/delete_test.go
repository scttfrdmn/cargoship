package cmd

import (
	"testing"

	"github.com/scttfrdmn/cargoship/pkg/manifest"
)

// TestUploadObjectKeys_DirectUpload guards #487: a direct upload has no
// chunks/shards, so delete must enumerate one object per file — otherwise it
// deletes only the manifest and orphans every file object.
func TestUploadObjectKeys_DirectUpload(t *testing.T) {
	m := &manifest.Manifest{
		Files: []manifest.FileEntry{
			{Path: "a/f1.bin", S3Key: "pfx/a/f1.bin"},
			{Path: "b/f2.bin", S3Key: "pfx/b/f2.bin"},
			// A dedup duplicate references the original's key — must de-dupe.
			{Path: "c/f3.bin", S3Key: "pfx/a/f1.bin", IsDuplicate: true},
		},
		// no Chunks → direct upload
	}
	got := uploadObjectKeys(m)
	want := map[string]bool{"pfx/a/f1.bin": true, "pfx/b/f2.bin": true}
	if len(got) != len(want) {
		t.Fatalf("got %d keys %v, want %d unique", len(got), got, len(want))
	}
	for _, k := range got {
		if !want[k] {
			t.Errorf("unexpected key %q", k)
		}
	}
}

// TestUploadObjectKeys_Chunked enumerates chunk object keys for a chunked upload.
func TestUploadObjectKeys_Chunked(t *testing.T) {
	m := &manifest.Manifest{
		Chunks: []manifest.ChunkEntry{
			{ID: 0, ShardID: 0, S3Key: "pfx/uploads/id/shard-0/chunk-0.tar.zst"},
			{ID: 1, ShardID: 1, S3Key: "pfx/uploads/id/shard-1/chunk-1.tar.zst"},
		},
		Files: []manifest.FileEntry{{Path: "x", S3Key: "pfx/uploads/id/shard-0/chunk-0.tar.zst"}},
	}
	got := uploadObjectKeys(m)
	if len(got) != 2 {
		t.Fatalf("got %d keys %v, want 2 chunk keys", len(got), got)
	}
}

// TestUploadObjectKeys_ResolvesAgainstPrefix pins #906 for delete: manifest keys
// are relative to the upload prefix. Deleting them raw missed every chunk of an
// upload under a prefix and could delete an unrelated object at the bucket root.
// Direct-upload keys already carry the prefix and must not get it twice.
func TestUploadObjectKeys_ResolvesAgainstPrefix(t *testing.T) {
	chunked := &manifest.Manifest{
		Prefix: "team/data", Bucket: "bkt",
		Chunks: []manifest.ChunkEntry{
			{ID: 0, S3Key: "uploads/id/shard-0/chunk-0.tar.zst"},
			{ID: 1, S3Key: "uploads/id/shard-1/chunk-1.tar.zst"},
		},
	}
	got := uploadObjectKeys(chunked)
	want := []string{"team/data/uploads/id/shard-0/chunk-0.tar.zst", "team/data/uploads/id/shard-1/chunk-1.tar.zst"}
	if len(got) != len(want) || got[0] != want[0] || got[1] != want[1] {
		t.Errorf("chunked keys = %v, want %v", got, want)
	}

	direct := &manifest.Manifest{
		Prefix: "team/data", Bucket: "bkt",
		Files: []manifest.FileEntry{
			{Path: "a", S3Key: "team/data/uploads/id/a"},
			{Path: "b", S3Key: "team/data/uploads/id/a", IsDuplicate: true},
		},
	}
	got = uploadObjectKeys(direct)
	if len(got) != 1 || got[0] != "team/data/uploads/id/a" {
		t.Errorf("direct keys = %v, want [team/data/uploads/id/a] (no double prefix, deduplicated)", got)
	}
}
