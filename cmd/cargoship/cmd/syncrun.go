package cmd

import (
	"context"
	"fmt"

	"github.com/aws/aws-sdk-go-v2/service/s3"

	"github.com/scttfrdmn/cargoship/pkg/manifest"
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
	force            bool
	dryRun           bool
}

// syncRunResult reports what one sync cycle did, for the caller to log.
type syncRunResult struct {
	NoChanges    bool
	Delta        *manifest.DeltaResult
	Result       *pipeline.Result
	SyncType     string
	PrevUploadID string
}

// runOneSync performs exactly one incremental-sync cycle headlessly (no prints, no
// prompts): download the previous manifest (unless forced) → scan → compute delta →
// short-circuit on no-changes/dry-run → resolve the dataset version → build the sync
// pipeline config → run the real pipeline. It is the same sequence `cargoship sync`
// performs, factored for reuse by the ghostship daemon. Unlike the sync command it
// also treats a non-Success pipeline result as an error, so a daemon cycle that
// fails is surfaced rather than silently logged as done.
func runOneSync(ctx context.Context, p syncRunParams) (*syncRunResult, error) {
	var previousManifest *manifest.Manifest
	syncType := manifest.SyncTypeFull
	if !p.force {
		if pm, err := downloadLatestManifest(ctx, p.s3Client, p.bucket, p.prefix, p.sourcePath); err == nil {
			previousManifest = pm
			syncType = manifest.SyncTypeIncremental
		}
	}

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
		return &syncRunResult{NoChanges: true, Delta: delta, SyncType: syncType}, nil
	}
	if p.dryRun {
		return &syncRunResult{Delta: delta, SyncType: syncType, PrevUploadID: prevUploadID(previousManifest)}, nil
	}

	includeFiles := make([]string, 0, len(delta.GetChangedFiles()))
	for _, f := range delta.GetChangedFiles() {
		includeFiles = append(includeFiles, f.Path)
	}

	previousUploadID := prevUploadID(previousManifest)
	datasetFetch := func(fctx context.Context, id string) (*manifest.Manifest, error) {
		return manifest.DownloadFromS3(fctx, p.s3Client, p.bucket, p.prefix, id)
	}
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
	res := &syncRunResult{Delta: delta, Result: result, SyncType: syncType, PrevUploadID: previousUploadID}
	if !result.Success {
		return res, fmt.Errorf("sync completed with %d error(s)", len(result.Errors))
	}
	return res, nil
}

func prevUploadID(m *manifest.Manifest) string {
	if m != nil {
		return m.UploadID
	}
	return ""
}
