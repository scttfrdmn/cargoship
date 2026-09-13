package manifest

import (
	"context"
	"errors"
	"fmt"
	"sort"
)

// maxChainDepth bounds how many versions ResolveEffective will walk. A dataset
// with more incremental versions than this is almost certainly a corrupt or
// looping chain; refusing to resolve is safer than silently truncating the
// reconstructed dataset.
const maxChainDepth = 10000

// ChainFetcher downloads an ancestor manifest by its UploadID, from the same
// bucket/prefix as the starting manifest. It returns a non-nil error if the
// ancestor cannot be fetched.
type ChainFetcher func(ctx context.Context, uploadID string) (*Manifest, error)

// ResolveEffective returns a manifest describing the COMPLETE dataset state as
// of m, by following the PreviousManifestID chain and merging files
// newest-wins per path (#552).
//
// Incremental `sync` uploads only changed files, so the newest manifest alone
// describes just that run's delta; unchanged files live in ancestor manifests.
// Without this, restore/verify of the latest version silently omit every file
// that hasn't changed since an earlier version. ResolveEffective reconstructs
// the full logical file set so a single (synthetic) manifest again describes the
// whole dataset.
//
// Merge rules:
//   - Files are unioned newest-wins by Path: the newest manifest that contains a
//     path supplies its FileEntry (so a modified file uses its latest bytes).
//   - Each surviving FileEntry keeps its own S3Key, which still points at the
//     chunk that physically stores its bytes (S3Keys embed the origin UploadID,
//     so they are unique across versions — the extractor joins file→chunk by
//     S3Key, not by (chunk_id, shard_id), so the merge cannot collide).
//   - Chunks are the union (by S3Key) of every chunk a surviving file references.
//   - Top-level metadata (UploadID, bucket, encryption, ...) comes from m.
//
// If m has no PreviousManifestID it is returned unchanged. A cyclic chain, or
// one deeper than maxChainDepth, is an error — a corrupt chain must fail loudly
// rather than return a partial dataset.
//
// Deletes recorded by `sync --track-deletes` (Manifest.DeletedPaths) are honored
// as tombstones: a path deleted in a version is dropped from the reconstructed
// dataset (unless a still-newer version re-adds it). Limitation (tracked): a
// dataset that mixed direct-upload and chunked uploads across versions is not
// handled (the merged view assumes a consistent mode).
func ResolveEffective(ctx context.Context, m *Manifest, fetch ChainFetcher) (*Manifest, error) {
	if m == nil {
		return nil, errors.New("resolve effective manifest: nil manifest")
	}
	if m.PreviousManifestID == "" {
		return m, nil
	}
	chain, err := ResolveChain(ctx, m, fetch)
	if err != nil {
		return nil, err
	}
	return MergeChain(chain), nil
}

// ResolveChain walks the PreviousManifestID chain from m (newest) back to the
// root, returning the manifests newest-first (chain[0] == m). A cycle, or a
// chain deeper than maxChainDepth, is an error — a corrupt chain must fail
// loudly rather than silently truncate the dataset. Callers that only need the
// merged view should use ResolveEffective; verify uses this to validate each
// version individually (single-manifest invariants hold per-manifest, not on
// the merged view).
func ResolveChain(ctx context.Context, m *Manifest, fetch ChainFetcher) ([]*Manifest, error) {
	if m == nil {
		return nil, errors.New("resolve chain: nil manifest")
	}
	if fetch == nil {
		return nil, errors.New("resolve chain: nil fetcher")
	}
	chain := []*Manifest{m}
	seen := map[string]bool{m.UploadID: true}
	cur := m
	for cur.PreviousManifestID != "" {
		prev := cur.PreviousManifestID
		if seen[prev] {
			return nil, fmt.Errorf("resolve chain: cycle at upload %q", prev)
		}
		if len(chain) >= maxChainDepth {
			return nil, fmt.Errorf("resolve chain: exceeds %d versions; refusing to resolve", maxChainDepth)
		}
		anc, err := fetch(ctx, prev)
		if err != nil {
			return nil, fmt.Errorf("resolve chain: fetch ancestor %q: %w", prev, err)
		}
		if anc == nil {
			return nil, fmt.Errorf("resolve chain: ancestor %q not found", prev)
		}
		seen[prev] = true
		chain = append(chain, anc)
		cur = anc
	}
	return chain, nil
}

// MergeChain merges a newest-first chain (from ResolveChain) into one effective
// manifest describing the full current dataset: files unioned newest-wins per
// path, deletes tombstoned, chunks the union (by S3Key) of those a surviving
// file references. Top-level metadata comes from the newest manifest. A
// single-element chain returns that manifest unchanged.
func MergeChain(chain []*Manifest) *Manifest {
	newest := chain[0]
	if len(chain) == 1 {
		return newest
	}

	// Walk newest -> oldest. The newest MENTION of a path wins, whether that
	// mention is a file (include) or a delete (tombstone). `decided` records
	// paths already resolved, so an older version can neither re-add a tombstoned
	// path nor override a newer file.
	fileByPath := make(map[string]FileEntry)
	decided := make(map[string]bool)
	for _, mm := range chain {
		for _, p := range mm.DeletedPaths {
			if !decided[p] {
				decided[p] = true // tombstone: decided, not added
			}
		}
		for _, f := range mm.Files {
			if !decided[f.Path] {
				decided[f.Path] = true
				fileByPath[f.Path] = f
			}
		}
	}

	// Union of all chunks by S3Key, then keep only those a surviving file references.
	chunkByKey := make(map[string]ChunkEntry)
	for _, mm := range chain {
		for _, c := range mm.Chunks {
			if _, ok := chunkByKey[c.S3Key]; !ok {
				chunkByKey[c.S3Key] = c
			}
		}
	}
	needed := make(map[string]bool)
	for _, f := range fileByPath {
		needed[f.S3Key] = true
	}

	eff := *newest // copy top-level metadata from the newest manifest
	eff.Files = make([]FileEntry, 0, len(fileByPath))
	var totalBytes int64
	for _, f := range fileByPath {
		eff.Files = append(eff.Files, f)
		totalBytes += f.Size
	}
	eff.Chunks = make([]ChunkEntry, 0, len(needed))
	for key := range needed {
		if c, ok := chunkByKey[key]; ok {
			eff.Chunks = append(eff.Chunks, c)
		}
	}

	// Deterministic ordering so the reconstructed manifest is stable.
	sort.Slice(eff.Files, func(i, j int) bool { return eff.Files[i].Path < eff.Files[j].Path })
	sort.Slice(eff.Chunks, func(i, j int) bool { return eff.Chunks[i].S3Key < eff.Chunks[j].S3Key })

	eff.TotalFiles = int64(len(eff.Files))
	eff.TotalChunks = len(eff.Chunks)
	eff.TotalBytes = totalBytes
	eff.DeletedPaths = nil // tombstones are resolved into the merged file set
	// Shard rollups are not recomputed: the effective manifest is for file/chunk
	// resolution (restore/verify join by S3Key), not shard-level statistics.

	return &eff
}
