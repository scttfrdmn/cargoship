package manifest

import (
	"context"
	"fmt"
	"sort"
)

// PrunePlan is a garbage-collection plan for one dataset (#521 phase 3): which
// versions to keep vs prune, whether the oldest kept version must be compacted
// into a self-contained manifest first, and the object keys that must survive
// because a kept version's effective view still references them. It is computed
// purely — no S3 mutation — so the deletion decision can be proven correct (and
// previewed with --dry-run) before any object is removed.
type PrunePlan struct {
	Keep           []*Manifest     // versions retained (newest-first)
	Prune          []*Manifest     // versions whose objects/manifest become deletion candidates (newest-first)
	CompactID      string          // upload ID of the oldest kept version to rewrite self-contained; "" if none needed
	KeepObjectKeys map[string]bool // stored S3Keys referenced by a kept version's effective view — MUST NOT be deleted
}

// PlanKeepLast computes a keep-last-N prune plan for the dataset `members` (all
// versions, any order). It keeps the newest n by version ordinal (HEAD always
// kept) and prunes the rest. Because the kept versions are contiguous from HEAD,
// the oldest kept version's chain still walks the pruned ancestors, so it is
// marked for compaction (rewrite as a self-contained manifest); after that the
// pruned manifests are removable. KeepObjectKeys is the union of every kept
// version's effective object keys (resolved via fetch) so an object that a
// pruned version also lists is still protected when a kept version needs it.
//
// It errors if the dataset mixes direct and chunked uploads (MergeChain assumes a
// single mode) or if n < 1. When n >= len(members) nothing is pruned.
func PlanKeepLast(ctx context.Context, members []*Manifest, n int, fetch ChainFetcher) (*PrunePlan, error) {
	if n < 1 {
		return nil, fmt.Errorf("keep-last must be >= 1")
	}
	if len(members) == 0 {
		return nil, fmt.Errorf("no versions found for this dataset")
	}
	// Reject a mixed-mode chain up front: every member must be all-direct
	// (no chunks) or all-chunked. Compaction via MergeChain assumes one mode.
	direct := len(members[0].Chunks) == 0
	for _, m := range members {
		if (len(m.Chunks) == 0) != direct {
			return nil, fmt.Errorf("dataset mixes direct-upload and chunked versions, which this first cut of prune cannot compact safely; refusing to prune (a version chain can become mixed when the pipeline auto-selects the direct fast path for a small delta but packs a larger one — this is a known limitation, tracked for a follow-up; the data itself is fully restorable)")
		}
	}

	sorted := append([]*Manifest(nil), members...)
	sort.Slice(sorted, func(i, j int) bool {
		if sorted[i].VersionOrdinal != sorted[j].VersionOrdinal {
			return sorted[i].VersionOrdinal > sorted[j].VersionOrdinal
		}
		return sorted[i].CreatedAt.After(sorted[j].CreatedAt)
	})

	plan := &PrunePlan{KeepObjectKeys: map[string]bool{}}
	if n >= len(sorted) {
		plan.Keep = sorted
		return plan, nil // nothing to prune
	}
	plan.Keep = sorted[:n]
	plan.Prune = sorted[n:]

	// Protect every object a kept version's effective view references.
	for _, k := range plan.Keep {
		eff, err := ResolveEffective(ctx, k, fetch)
		if err != nil {
			return nil, fmt.Errorf("resolve effective view of kept version %s: %w", k.UploadID, err)
		}
		for _, key := range objectKeysOf(eff) {
			plan.KeepObjectKeys[key] = true
		}
	}
	// The oldest kept version chains through pruned ancestors → compact it so it
	// no longer depends on a pruned manifest.
	if oldestKept := plan.Keep[len(plan.Keep)-1]; oldestKept.PreviousManifestID != "" {
		plan.CompactID = oldestKept.UploadID
	}
	return plan, nil
}

// PrunableObjectKeys returns the stored S3Keys of the pruned versions' own data
// objects that no kept version references — i.e. the objects safe to delete.
func (p *PrunePlan) PrunableObjectKeys() []string {
	seen := map[string]bool{}
	var out []string
	for _, m := range p.Prune {
		for _, key := range objectKeysOf(m) {
			if key != "" && !p.KeepObjectKeys[key] && !seen[key] {
				seen[key] = true
				out = append(out, key)
			}
		}
	}
	sort.Strings(out)
	return out
}

// objectKeysOf returns the stored S3Keys of the data objects a manifest lists:
// chunk keys for a chunked upload, or per-file keys for a direct upload (no
// chunks). On a delta manifest these are the version's OWN objects; on an
// effective (merged) manifest they are the full set the version references.
func objectKeysOf(m *Manifest) []string {
	if len(m.Chunks) == 0 {
		keys := make([]string, 0, len(m.Files))
		for i := range m.Files {
			if m.Files[i].S3Key != "" {
				keys = append(keys, m.Files[i].S3Key)
			}
		}
		return keys
	}
	keys := make([]string, 0, len(m.Chunks))
	for i := range m.Chunks {
		if m.Chunks[i].S3Key != "" {
			keys = append(keys, m.Chunks[i].S3Key)
		}
	}
	return keys
}
