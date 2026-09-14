package manifest

import (
	"context"
	"errors"
	"testing"
)

// chainFetch builds a ChainFetcher backed by an in-memory map of uploadID→manifest.
func chainFetch(store map[string]*Manifest) ChainFetcher {
	return func(_ context.Context, id string) (*Manifest, error) {
		m, ok := store[id]
		if !ok {
			return nil, errors.New("not found: " + id)
		}
		return m, nil
	}
}

func fileEntry(path, s3key string, size int64) FileEntry {
	return FileEntry{Path: path, S3Key: s3key, Size: size}
}

// TestResolveEffective_IncrementalChain is the #552 regression: restoring the
// latest incremental version must recover unchanged files carried in ancestors.
func TestResolveEffective_IncrementalChain(t *testing.T) {
	// v1 (full): files a.txt, b.txt in chunk under uploads/v1.
	v1 := &Manifest{
		UploadID: "v1",
		Files: []FileEntry{
			fileEntry("a.txt", "p/uploads/v1/shard-0/chunk-0.tar.zst", 10),
			fileEntry("b.txt", "p/uploads/v1/shard-0/chunk-0.tar.zst", 20),
		},
		Chunks: []ChunkEntry{{ID: 0, ShardID: 0, S3Key: "p/uploads/v1/shard-0/chunk-0.tar.zst"}},
	}
	// v2 (incremental): only a.txt changed → new chunk under uploads/v2.
	v2 := &Manifest{
		UploadID:           "v2",
		PreviousManifestID: "v1",
		SyncType:           SyncTypeIncremental,
		Files:              []FileEntry{fileEntry("a.txt", "p/uploads/v2/shard-0/chunk-0.tar.zst", 15)},
		Chunks:             []ChunkEntry{{ID: 0, ShardID: 0, S3Key: "p/uploads/v2/shard-0/chunk-0.tar.zst"}},
	}

	eff, err := ResolveEffective(context.Background(), v2, chainFetch(map[string]*Manifest{"v1": v1}))
	if err != nil {
		t.Fatalf("ResolveEffective: %v", err)
	}

	byPath := map[string]FileEntry{}
	for _, f := range eff.Files {
		byPath[f.Path] = f
	}
	// The whole dataset must be present.
	if len(eff.Files) != 2 {
		t.Fatalf("want 2 files in effective view, got %d: %+v", len(eff.Files), eff.Files)
	}
	// b.txt (unchanged) must be recovered from v1's chunk — the bug is that it vanishes.
	b, ok := byPath["b.txt"]
	if !ok {
		t.Fatal("b.txt missing from effective manifest — unchanged file was silently lost (#552)")
	}
	if b.S3Key != "p/uploads/v1/shard-0/chunk-0.tar.zst" {
		t.Errorf("b.txt should point at v1's chunk, got %s", b.S3Key)
	}
	// a.txt (modified) must be the NEW version from v2.
	a := byPath["a.txt"]
	if a.S3Key != "p/uploads/v2/shard-0/chunk-0.tar.zst" || a.Size != 15 {
		t.Errorf("a.txt should be v2's version (size 15, v2 chunk), got size=%d key=%s", a.Size, a.S3Key)
	}
	// Both backing chunks must be present so restore can fetch either file.
	keys := map[string]bool{}
	for _, c := range eff.Chunks {
		keys[c.S3Key] = true
	}
	if !keys["p/uploads/v1/shard-0/chunk-0.tar.zst"] || !keys["p/uploads/v2/shard-0/chunk-0.tar.zst"] {
		t.Errorf("effective chunks must include both v1 and v2 chunks, got %+v", eff.Chunks)
	}
	if eff.TotalFiles != 2 || eff.TotalBytes != 35 {
		t.Errorf("rollups: want TotalFiles=2 TotalBytes=35, got %d/%d", eff.TotalFiles, eff.TotalBytes)
	}
	// Metadata comes from the newest manifest.
	if eff.UploadID != "v2" {
		t.Errorf("effective UploadID should be the newest (v2), got %s", eff.UploadID)
	}
}

func TestResolveEffective_ThreeVersionsNewestWins(t *testing.T) {
	v1 := &Manifest{UploadID: "v1", Files: []FileEntry{fileEntry("f.txt", "k1", 1)}, Chunks: []ChunkEntry{{S3Key: "k1"}}}
	v2 := &Manifest{UploadID: "v2", PreviousManifestID: "v1", Files: []FileEntry{fileEntry("f.txt", "k2", 2)}, Chunks: []ChunkEntry{{S3Key: "k2"}}}
	v3 := &Manifest{UploadID: "v3", PreviousManifestID: "v2", Files: []FileEntry{fileEntry("f.txt", "k3", 3)}, Chunks: []ChunkEntry{{S3Key: "k3"}}}

	eff, err := ResolveEffective(context.Background(), v3, chainFetch(map[string]*Manifest{"v1": v1, "v2": v2}))
	if err != nil {
		t.Fatalf("ResolveEffective: %v", err)
	}
	if len(eff.Files) != 1 || eff.Files[0].S3Key != "k3" || eff.Files[0].Size != 3 {
		t.Errorf("newest (v3) must win for f.txt, got %+v", eff.Files)
	}
	if len(eff.Chunks) != 1 || eff.Chunks[0].S3Key != "k3" {
		t.Errorf("only the referenced (k3) chunk should remain, got %+v", eff.Chunks)
	}
}

// TestResolveEffective_DeleteTombstone is the #555 regression: a file deleted in
// a newer version must be dropped from the merged dataset, even though an
// ancestor still lists it.
func TestResolveEffective_DeleteTombstone(t *testing.T) {
	v1 := &Manifest{
		UploadID: "v1",
		Files: []FileEntry{
			fileEntry("keep.txt", "p/uploads/v1/c0", 10),
			fileEntry("gone.txt", "p/uploads/v1/c0", 20),
		},
		Chunks: []ChunkEntry{{S3Key: "p/uploads/v1/c0"}},
	}
	// v2 deletes gone.txt (nothing new uploaded); records the tombstone.
	v2 := &Manifest{
		UploadID:           "v2",
		PreviousManifestID: "v1",
		SyncType:           SyncTypeIncremental,
		DeletedPaths:       []string{"gone.txt"},
	}

	eff, err := ResolveEffective(context.Background(), v2, chainFetch(map[string]*Manifest{"v1": v1}))
	if err != nil {
		t.Fatalf("ResolveEffective: %v", err)
	}
	byPath := map[string]FileEntry{}
	for _, f := range eff.Files {
		byPath[f.Path] = f
	}
	if _, ok := byPath["gone.txt"]; ok {
		t.Error("gone.txt was deleted in v2 but survived the merge (#555 tombstone not honored)")
	}
	if _, ok := byPath["keep.txt"]; !ok {
		t.Error("keep.txt should survive")
	}
	if eff.TotalFiles != 1 {
		t.Errorf("want 1 file after delete, got %d", eff.TotalFiles)
	}
	if eff.DeletedPaths != nil {
		t.Error("effective manifest should not carry DeletedPaths (resolved into the file set)")
	}
}

// TestResolveEffective_DeleteThenReAdd: a path deleted in v2 and re-created in v3
// must be present (v3's version wins over v2's tombstone).
func TestResolveEffective_DeleteThenReAdd(t *testing.T) {
	v1 := &Manifest{UploadID: "v1", Files: []FileEntry{fileEntry("f.txt", "k1", 1)}, Chunks: []ChunkEntry{{S3Key: "k1"}}}
	v2 := &Manifest{UploadID: "v2", PreviousManifestID: "v1", DeletedPaths: []string{"f.txt"}}
	v3 := &Manifest{UploadID: "v3", PreviousManifestID: "v2", Files: []FileEntry{fileEntry("f.txt", "k3", 3)}, Chunks: []ChunkEntry{{S3Key: "k3"}}}

	eff, err := ResolveEffective(context.Background(), v3, chainFetch(map[string]*Manifest{"v1": v1, "v2": v2}))
	if err != nil {
		t.Fatalf("ResolveEffective: %v", err)
	}
	if len(eff.Files) != 1 || eff.Files[0].S3Key != "k3" || eff.Files[0].Size != 3 {
		t.Errorf("re-added f.txt (v3) must win over the v2 tombstone, got %+v", eff.Files)
	}
}

func TestResolveEffective_NoChain(t *testing.T) {
	m := &Manifest{UploadID: "solo", Files: []FileEntry{fileEntry("x", "k", 1)}}
	eff, err := ResolveEffective(context.Background(), m, nil) // nil fetcher is fine with no chain
	if err != nil {
		t.Fatalf("ResolveEffective: %v", err)
	}
	if eff != m {
		t.Error("a manifest with no PreviousManifestID should be returned unchanged")
	}
}

func TestResolveEffective_Cycle(t *testing.T) {
	// a -> b -> a
	a := &Manifest{UploadID: "a", PreviousManifestID: "b"}
	b := &Manifest{UploadID: "b", PreviousManifestID: "a"}
	_, err := ResolveEffective(context.Background(), a, chainFetch(map[string]*Manifest{"a": a, "b": b}))
	if err == nil {
		t.Fatal("expected an error on a cyclic chain")
	}
}

func TestResolveEffective_MissingAncestor(t *testing.T) {
	v2 := &Manifest{UploadID: "v2", PreviousManifestID: "v1", Files: []FileEntry{fileEntry("a", "k", 1)}}
	_, err := ResolveEffective(context.Background(), v2, chainFetch(map[string]*Manifest{}))
	if err == nil {
		t.Fatal("expected an error when an ancestor cannot be fetched (must fail loudly, not truncate)")
	}
}

// TestDependentUploads is the #592 core: identifying uploads whose chain passes
// through a target upload, so a chain-aware delete can refuse to strand them.
func TestDependentUploads(t *testing.T) {
	v1 := &Manifest{UploadID: "v1"}
	v2 := &Manifest{UploadID: "v2", PreviousManifestID: "v1"}
	v3 := &Manifest{UploadID: "v3", PreviousManifestID: "v2"}
	standalone := &Manifest{UploadID: "s1"}
	all := []*Manifest{v1, v2, v3, standalone}

	// Root has two descendants (v2 directly, v3 transitively).
	deps := DependentUploads("v1", all)
	if len(deps) != 2 || !containsID(deps, "v2") || !containsID(deps, "v3") {
		t.Errorf("v1 dependents = %v, want [v2 v3]", deps)
	}
	// A middle version has one descendant.
	if deps := DependentUploads("v2", all); len(deps) != 1 || deps[0] != "v3" {
		t.Errorf("v2 dependents = %v, want [v3]", deps)
	}
	// The head and a standalone upload have no dependents → safe to delete.
	if deps := DependentUploads("v3", all); len(deps) != 0 {
		t.Errorf("v3 (head) dependents = %v, want none", deps)
	}
	if deps := DependentUploads("s1", all); len(deps) != 0 {
		t.Errorf("standalone dependents = %v, want none", deps)
	}
	// A cyclic chain must not hang.
	c1 := &Manifest{UploadID: "c1", PreviousManifestID: "c2"}
	c2 := &Manifest{UploadID: "c2", PreviousManifestID: "c1"}
	_ = DependentUploads("x", []*Manifest{c1, c2})
}

func containsID(ss []string, s string) bool {
	for _, x := range ss {
		if x == s {
			return true
		}
	}
	return false
}
