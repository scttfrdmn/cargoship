package cmd

import (
	"context"
	"errors"
	"fmt"
	"time"

	"github.com/aws/aws-sdk-go-v2/service/s3"

	versionpkg "github.com/scttfrdmn/cargoship/internal/version"
	cargoconfig "github.com/scttfrdmn/cargoship/pkg/aws/config"
	"github.com/scttfrdmn/cargoship/pkg/aws/cost"
	"github.com/scttfrdmn/cargoship/pkg/manifest"
	"github.com/scttfrdmn/cargoship/pkg/manifestcache"
	"github.com/scttfrdmn/cargoship/pkg/pipeline"
)

// syncRunParams carries the inputs for one headless incremental sync cycle (#604).
// It mirrors the inputs `cargoship sync` resolves from flags; the daemon
// (`cargoship ghostship run`) reuses it to drive the same real engine on an
// interval. prefix is already writer-folded (see pipeline.WriterPrefix).
type syncRunParams struct {
	s3Client         *s3.Client
	bucket           string
	prefix           string
	sourcePath       string
	region           string
	writerID         string
	storageClass     string
	shardCount       int
	shardStrategy    string
	compressionLevel int // 0 = content-aware per-chunk selection
	useChecksum      bool
	trackDeletes     bool
	// #710: patterns from the signed config's watch_paths[].exclude_patterns. Matched
	// per path segment relative to the source, so a directory pattern excludes its
	// whole subtree.
	excludePatterns []string
	force           bool
	dryRun          bool
	projectID       string        // #629: cost/cap attribution key (ghostship: the writer id)
	costMgr         *cost.Manager // #629: pre-write cap gate; nil = enforcement off

	// P1: local manifest cache. A write-only identity cannot GET its own previous
	// manifest, so without this the delta is always computed against nil and every
	// cycle re-uploads the whole source. nil disables the cache entirely.
	manifestCache       *manifestcache.Store
	manifestCacheMaxAge time.Duration // 0 = manifestcache.DefaultMaxAge
}

// syncRunResult reports what one sync cycle did, for the caller to log.
type syncRunResult struct {
	NoChanges    bool
	Delta        *manifest.DeltaResult
	Result       *pipeline.Result
	SyncType     string
	PrevUploadID string

	// DeltaSource is "s3", "cache" or "none" — see fleet.SourceStatus.DeltaSource.
	DeltaSource string
	// CacheNote carries a non-fatal manifest-cache problem for the caller to log: an
	// entry that existed but was rejected (corrupt, expired, wrong destination), or a
	// save that failed. Empty when the cache behaved normally, including when it simply
	// had no entry yet.
	CacheNote string
}

// runOneSync performs exactly one incremental-sync cycle headlessly (no prints, no
// prompts): download the previous manifest (unless forced) → scan → compute delta →
// short-circuit on no-changes/dry-run → resolve the dataset version → build the sync
// pipeline config → run the real pipeline. It is the same sequence `cargoship sync`
// performs, factored for reuse by the ghostship daemon. Unlike the sync command it
// also treats a non-Success pipeline result as an error, so a daemon cycle that
// fails is surfaced rather than silently logged as done.
func runOneSync(ctx context.Context, p syncRunParams) (*syncRunResult, error) {
	// One fetcher for both chain resolution and NextVersion below.
	datasetFetch := func(fctx context.Context, id string) (*manifest.Manifest, error) {
		return manifest.DownloadFromS3(fctx, p.s3Client, p.bucket, p.prefix, id)
	}
	prev := resolvePreviousManifest(p, func() (*manifest.Manifest, error) {
		latest, err := downloadLatestManifest(ctx, p.s3Client, p.bucket, p.prefix, p.sourcePath)
		if err != nil {
			return nil, err
		}
		// #691: the newest manifest lists only ITS OWN increment, so diffing against it
		// marks everything stored by earlier versions as New and re-uploads the dataset.
		// Resolve the chain to the effective dataset first — the same view restore and
		// verify already use.
		return manifest.ResolveEffective(ctx, latest, datasetFetch)
	})
	previousManifest := prev.manifest
	syncType := prev.syncType()
	deltaSource := prev.source
	cacheNote := prev.note

	localFiles, err := manifest.ScanLocalFiles(p.sourcePath)
	if err != nil {
		return nil, fmt.Errorf("scan local files: %w", err)
	}

	delta, err := manifest.ComputeDelta(localFiles, previousManifest, &manifest.SyncOptions{
		UseChecksum:  p.useChecksum,
		TrackDeletes: p.trackDeletes,
	})
	if err != nil {
		return nil, fmt.Errorf("compute delta: %w", err)
	}

	if !delta.HasChanges() {
		// Deliberately NOT refreshing the cache entry's timestamp here. A no-change cycle
		// proves the local tree still matches the cached manifest; it proves nothing about
		// whether the objects are still in S3, which is exactly what the trust window
		// exists to bound. Letting a quiescent dataset's entry expire forces a periodic
		// full re-establish, which is the intended cost.
		return &syncRunResult{NoChanges: true, Delta: delta, SyncType: syncType, DeltaSource: deltaSource, CacheNote: cacheNote}, nil
	}
	if p.dryRun {
		return &syncRunResult{Delta: delta, SyncType: syncType, PrevUploadID: prevUploadID(previousManifest),
			DeltaSource: deltaSource, CacheNote: cacheNote}, nil
	}

	// #629: pre-write cap gate — refuse a cycle that would exceed this writer's
	// configured volume quota / cost budget, before any bytes move.
	if p.costMgr != nil {
		var deltaBytes int64
		for _, f := range delta.GetChangedFiles() {
			deltaBytes += f.Size
		}
		sizeGB := float64(deltaBytes) / (1024 * 1024 * 1024)
		if err := enforceBudgetCaps(ctx, p.costMgr, p.projectID, sizeGB, cargoconfig.StorageClass(p.storageClass), p.region); err != nil {
			return nil, err
		}
	}

	includeFiles := make([]string, 0, len(delta.GetChangedFiles()))
	for _, f := range delta.GetChangedFiles() {
		includeFiles = append(includeFiles, f.Path)
	}

	previousUploadID := prevUploadID(previousManifest)
	datasetID, versionOrdinal := manifest.NextVersion(ctx, previousManifest, datasetFetch)

	pc := newSyncPipelineConfig(syncPipelineParams{
		bucket:           p.bucket,
		prefix:           p.prefix,
		region:           p.region,
		writerID:         p.writerID,
		storageClass:     p.storageClass,
		shardCount:       p.shardCount,
		shardStrategy:    p.shardStrategy,
		compressionLevel: p.compressionLevel,
		sourcePath:       p.sourcePath,
		includeFiles:     includeFiles,
		excludePatterns:  p.excludePatterns,
		syncType:         syncType,
		previousUploadID: previousUploadID,
		deletedPaths:     delta.Deleted,
		datasetID:        datasetID,
		versionOrdinal:   versionOrdinal,
		s3Client:         p.s3Client,
	})
	pc.MagikaConfig = magikaConfigFromViper()

	pipe, err := pipeline.NewPipeline(pc)
	if err != nil {
		return nil, fmt.Errorf("create pipeline: %w", err)
	}
	result, err := pipe.Run(ctx, p.sourcePath)
	if err != nil {
		return nil, fmt.Errorf("sync run: %w", err)
	}
	res := &syncRunResult{Delta: delta, Result: result, SyncType: syncType, PrevUploadID: previousUploadID,
		DeltaSource: deltaSource, CacheNote: cacheNote}
	if !result.Success {
		// Do NOT cache a partial upload. Its manifest would list files as stored that
		// never landed, and the next delta would skip them forever — the one failure mode
		// of this feature that loses data rather than wasting bandwidth.
		return res, fmt.Errorf("sync completed with %d error(s)", len(result.Errors))
	}

	// Record what this cycle stored so the next one has a previous manifest even without
	// read access. Best-effort: a cache failure costs a future full re-upload, so it must
	// not fail a cycle whose bytes are already safely in S3.
	if p.manifestCache != nil {
		if m := pipe.GetManifest(); m != nil {
			// #691: cache the EFFECTIVE dataset, not this cycle's increment. Merging the
			// new manifest over the previous effective one is the same newest-wins
			// operation MergeChain performs, done here with what is already in memory so
			// a write-only agent never needs a read it does not have.
			effective := m
			if previousManifest != nil {
				effective = manifest.MergeChain([]*manifest.Manifest{m, previousManifest})
			}
			if sErr := p.manifestCache.Save(cacheKeyFor(p), effective, versionpkg.Version); sErr != nil {
				res.CacheNote = sErr.Error()
			}
		} else {
			res.CacheNote = "pipeline reported success but exposed no manifest to cache"
		}
	}
	return res, nil
}

// Delta sources reported in syncRunResult.DeltaSource and fleet.SourceStatus.
const (
	deltaSourceS3    = "s3"
	deltaSourceCache = "cache"
	deltaSourceNone  = "none"
)

// previousManifestResolution is the decision about what this cycle diffs against.
type previousManifestResolution struct {
	manifest *manifest.Manifest
	source   string // deltaSourceS3 | deltaSourceCache | deltaSourceNone
	note     string // non-fatal cache problem worth logging
}

func (r previousManifestResolution) syncType() string {
	if r.manifest == nil {
		return manifest.SyncTypeFull
	}
	return manifest.SyncTypeIncremental
}

// resolvePreviousManifest picks the manifest to compute this cycle's delta against.
// fromS3 is injected so the decision can be tested without a live S3 client or pipeline.
//
// The ordering is the whole trust argument:
//
//   - force: no previous manifest at all, and the cache is NOT consulted. --force must
//     mean a genuine full re-upload, otherwise there is no way to rebuild from scratch.
//   - S3 first, always: it reflects what is actually stored. The cache only reflects
//     what this agent believes it stored, so it must never override a readable S3.
//   - cache only when S3 is unreadable: the write-only case this exists for.
//   - a rejected entry yields no manifest and a note. Failing closed means a needless
//     full re-upload, which costs bandwidth; trusting a bad entry skips files, which
//     loses data.
func resolvePreviousManifest(p syncRunParams, fromS3 func() (*manifest.Manifest, error)) previousManifestResolution {
	if p.force {
		return previousManifestResolution{source: deltaSourceNone}
	}
	if pm, err := fromS3(); err == nil && pm != nil {
		return previousManifestResolution{manifest: pm, source: deltaSourceS3}
	}
	if p.manifestCache == nil {
		return previousManifestResolution{source: deltaSourceNone}
	}
	pm, err := p.manifestCache.Load(cacheKeyFor(p), p.manifestCacheMaxAge)
	switch {
	case err == nil:
		return previousManifestResolution{manifest: pm, source: deltaSourceCache}
	case errors.Is(err, manifestcache.ErrNotFound):
		// First cycle for this source: a full sync is correct, not a problem.
		return previousManifestResolution{source: deltaSourceNone}
	default:
		return previousManifestResolution{source: deltaSourceNone, note: err.Error()}
	}
}

// cacheKeyFor builds the manifest-cache key for a sync. p.prefix is already
// writer-folded, so two writers sharing a bucket cannot read each other's entries.
func cacheKeyFor(p syncRunParams) manifestcache.Key {
	return manifestcache.Key{
		Bucket:     p.bucket,
		Prefix:     p.prefix,
		SourcePath: p.sourcePath,
		WriterID:   p.writerID,
	}
}

func prevUploadID(m *manifest.Manifest) string {
	if m != nil {
		return m.UploadID
	}
	return ""
}
