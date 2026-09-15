package manifest

import (
	"context"
	"testing"
)

// buildChain3 builds a 3-version chunked dataset:
//
//	v1: fileA in chunk k1
//	v2: fileA modified → chunk k2 (supersedes k1)
//	v3: adds fileB → chunk k3
//
// v3's effective view references {k2, k3}; k1 is superseded (safe to prune).
func buildChain3() (v1, v2, v3 *Manifest, fetch ChainFetcher) {
	v1 = &Manifest{
		UploadID: "u1", DatasetID: "u1", VersionOrdinal: 1,
		Files:  []FileEntry{fileEntry("a", "k1", 1)},
		Chunks: []ChunkEntry{{S3Key: "k1"}},
	}
	v2 = &Manifest{
		UploadID: "u2", DatasetID: "u1", VersionOrdinal: 2, PreviousManifestID: "u1",
		Files:  []FileEntry{fileEntry("a", "k2", 2)},
		Chunks: []ChunkEntry{{S3Key: "k2"}},
	}
	v3 = &Manifest{
		UploadID: "u3", DatasetID: "u1", VersionOrdinal: 3, PreviousManifestID: "u2",
		Files:  []FileEntry{fileEntry("b", "k3", 3)},
		Chunks: []ChunkEntry{{S3Key: "k3"}},
	}
	fetch = chainFetch(map[string]*Manifest{"u1": v1, "u2": v2, "u3": v3})
	return
}

func TestPlanKeepLast(t *testing.T) {
	v1, v2, v3, fetch := buildChain3()
	all := []*Manifest{v2, v1, v3} // unsorted on purpose

	plan, err := PlanKeepLast(context.Background(), all, 1, fetch)
	if err != nil {
		t.Fatalf("PlanKeepLast: %v", err)
	}
	if len(plan.Keep) != 1 || plan.Keep[0].UploadID != "u3" {
		t.Fatalf("Keep = %v, want [u3]", ids(plan.Keep))
	}
	if len(plan.Prune) != 2 || plan.Prune[0].UploadID != "u2" || plan.Prune[1].UploadID != "u1" {
		t.Fatalf("Prune = %v, want [u2 u1]", ids(plan.Prune))
	}
	// HEAD (u3) chains through pruned ancestors → must be compacted.
	if plan.CompactID != "u3" {
		t.Errorf("CompactID = %q, want u3", plan.CompactID)
	}
	// u3's effective view references k2 (current fileA) and k3 (fileB); both kept.
	if !plan.KeepObjectKeys["k2"] || !plan.KeepObjectKeys["k3"] {
		t.Errorf("KeepObjectKeys must include k2,k3; got %v", plan.KeepObjectKeys)
	}
	// Only k1 (superseded fileA from v1) is prunable; k2/k3 are protected.
	prunable := plan.PrunableObjectKeys()
	if len(prunable) != 1 || prunable[0] != "k1" {
		t.Errorf("PrunableObjectKeys = %v, want [k1] (k2/k3 must survive)", prunable)
	}
}

func TestPlanKeepLast_KeepAll(t *testing.T) {
	v1, v2, v3, fetch := buildChain3()
	plan, err := PlanKeepLast(context.Background(), []*Manifest{v1, v2, v3}, 3, fetch)
	if err != nil {
		t.Fatalf("PlanKeepLast: %v", err)
	}
	if len(plan.Prune) != 0 || plan.CompactID != "" || len(plan.PrunableObjectKeys()) != 0 {
		t.Errorf("keep-all should prune nothing, got prune=%v compact=%q", ids(plan.Prune), plan.CompactID)
	}
}

func TestPlanKeepLast_Errors(t *testing.T) {
	v1, v2, v3, fetch := buildChain3()
	if _, err := PlanKeepLast(context.Background(), []*Manifest{v1, v2, v3}, 0, fetch); err == nil {
		t.Error("keep-last 0 should error")
	}
	// Mixed mode: make v2 direct (no chunks).
	v2.Chunks = nil
	if _, err := PlanKeepLast(context.Background(), []*Manifest{v1, v2, v3}, 1, fetch); err == nil {
		t.Error("mixed direct/chunked dataset should be refused")
	}
}

func ids(ms []*Manifest) []string {
	out := make([]string, len(ms))
	for i, m := range ms {
		out[i] = m.UploadID
	}
	return out
}
