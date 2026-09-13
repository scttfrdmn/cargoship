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
