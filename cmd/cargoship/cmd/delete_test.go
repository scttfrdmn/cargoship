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
