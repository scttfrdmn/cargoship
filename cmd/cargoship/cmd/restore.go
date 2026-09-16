package cmd

import (
	"context"
	"fmt"
	"strings"
	"time"

	"github.com/aws/aws-sdk-go-v2/config"
	"github.com/aws/aws-sdk-go-v2/service/kms"
	"github.com/aws/aws-sdk-go-v2/service/s3"
	"github.com/dustin/go-humanize"
	"github.com/spf13/cobra"

	s3pkg "github.com/scttfrdmn/cargoship/pkg/aws/s3"
	"github.com/scttfrdmn/cargoship/pkg/manifest"
	"github.com/scttfrdmn/cargoship/pkg/restore"
)

// NewRestoreCmd creates the 'restore' command for hash-based and DVC-aware
// selective file restoration (Issue #189, #200, #201).
func NewRestoreCmd() *cobra.Command {
	var (
		hash           string
		filePaths      []string
		gitCommit      string
		dvcStage       string
		region         string
		cacheGB        int64
		jsonOutput     bool
		tier           string
		wait           bool
		dryRun         bool
		maxRestoreCost float64
		restoreDays    int32
		noVerify       bool
		flatten        bool
		all            bool   // #617: restore every file in the manifest (disaster recovery)
		datasetID      string // #521: version-aware restore
		version        int
		asOfStr        string
	)

	cmd := &cobra.Command{
		Use:   "restore S3_URL OUTPUT_DIR",
		Short: "Restore specific files from a CargoShip archive using hash, path, commit, or DVC stage",
		Long: `Restore targeted files from a CargoShip archive without downloading the whole dataset.

Restoration modes (pick one or combine --file with others):
  --hash        : Restore a single file by its MD5 content hash
  --file        : Restore one or more exact file paths
  --git-commit  : Restore all files from a specific git commit
  --dvc-stage   : Restore all files produced by a DVC pipeline stage
  --all         : Restore every file in the upload (disaster recovery)

Glacier/Deep Archive support:
  --tier        : Retrieval tier: expedited (1-5 min), standard (3-5 h), bulk (5-12 h)
  --wait        : Block until Glacier restoration completes before downloading
  --dry-run     : Show what would be restored (size, cost) without downloading

Budget controls:
  --max-restore-cost : Abort if estimated retrieval cost exceeds this USD limit

Examples:
  # Restore a file by its MD5 hash
  cargoship restore s3://my-bucket/uploads/20240101-abc123 ./out \
    --hash d8e8fca2dc0f896fd7cb4cb0031ba249

  # Restore specific files by path
  cargoship restore s3://my-bucket/uploads/20240101-abc123 ./out \
    --file data/train.csv --file models/model.pkl

  # Restore all files from a DVC pipeline stage
  cargoship restore s3://my-bucket/uploads/20240101-abc123 ./out \
    --dvc-stage preprocess

  # Restore an ENTIRE upload — disaster recovery. Needs only read access + the
  # decryption key, not the box (or writer id) that produced it.
  cargoship restore s3://my-bucket/prefix/writers/lab-nas-1/uploads/20240101-abc123 ./out --all

  # Restore from Glacier with standard retrieval tier, wait for completion
  cargoship restore s3://my-bucket/uploads/20240101-abc123 ./out \
    --dvc-stage train --tier standard --wait

  # Dry-run: show estimated cost without restoring
  cargoship restore s3://my-bucket/uploads/20240101-abc123 ./out \
    --dvc-stage train --dry-run
`,
		Args: cobra.ExactArgs(2),
		RunE: func(cmd *cobra.Command, args []string) error {
			ctx := context.Background()
			s3URL := args[0]
			outputDir := args[1]

			bucket, prefix, err := parseS3URL(s3URL)
			if err != nil {
				return fmt.Errorf("invalid S3 URL: %w", err)
			}

			// --all restores the whole upload; it is standalone (validated before any
			// AWS call so a bad flag combo fails fast, not after a manifest download).
			if all && (hash != "" || len(filePaths) > 0 || gitCommit != "" || dvcStage != "") {
				return fmt.Errorf("--all cannot be combined with --hash/--file/--git-commit/--dvc-stage")
			}

			cfg, err := config.LoadDefaultConfig(ctx, config.WithRegion(region))
			if err != nil {
				return fmt.Errorf("failed to load AWS config: %w", err)
			}

			s3Client := s3.NewFromConfig(cfg)
			kmsClient := kms.NewFromConfig(cfg)

			// #521: version-aware restore. With --dataset-id the S3_URL is a
			// bucket/prefix (not a specific /uploads/<id>); resolve the requested
			// version of the dataset to a concrete upload, then restore it via the
			// existing path (chain-follow below still assembles the full dataset).
			var actualPrefix, uploadID string
			switch {
			case datasetID != "":
				if version > 0 && asOfStr != "" {
					return fmt.Errorf("--version and --as-of are mutually exclusive")
				}
				var asOf time.Time
				if asOfStr != "" {
					d, perr := time.Parse("2006-01-02", asOfStr)
					if perr != nil {
						return fmt.Errorf("invalid --as-of %q (use YYYY-MM-DD): %w", asOfStr, perr)
					}
					asOf = d.Add(24*time.Hour - time.Nanosecond) // inclusive of the whole day
				}
				all, lerr := listUploadManifests(ctx, s3Client, kmsClient, bucket, prefix)
				if lerr != nil {
					return lerr
				}
				sel, serr := selectDatasetVersion(ctx, all, inMemoryFetch(all), datasetID, version, asOf)
				if serr != nil {
					return serr
				}
				actualPrefix, uploadID = prefix, sel.UploadID
				fmt.Printf("🎯 Dataset %s: restoring v%d (upload %s)\n", datasetID, sel.VersionOrdinal, uploadID)
			default:
				if version > 0 || asOfStr != "" {
					return fmt.Errorf("--version/--as-of require --dataset-id")
				}
				// Parse upload ID from the prefix.
				if idx := strings.Index(prefix, "/uploads/"); idx != -1 {
					actualPrefix = prefix[:idx]
					uploadID = prefix[idx+9:]
				} else {
					uploadID = prefix
				}
			}

			// Load the manifest and, for an incremental sync, resolve the chain to
			// the full effective dataset (#552). Shared with `browse` so the two
			// can't diverge on chain handling.
			m, err := loadEffectiveManifest(ctx, s3Client, kmsClient, bucket, actualPrefix, uploadID)
			if err != nil {
				return err
			}

			maxCacheBytes := cacheGB * 1024 * 1024 * 1024
			// SetBucket: fetch from the bucket the manifest was just read from,
			// not the stale name recorded inside it, so a copied or replicated
			// archive restores from where it actually is (#335).
			se := manifest.NewSelectiveExtractor(m, s3Client, maxCacheBytes).
				SetBucket(bucket).
				SetVerify(!noVerify).
				SetFlatten(flatten)

			// Resolve target paths/keys.
			var targetPaths []string
			var chunkKeys []string

			switch {
			case all:
				targetPaths = allFilePaths(m)
				if len(targetPaths) == 0 {
					return fmt.Errorf("manifest records no files to restore")
				}
				chunkKeys = se.ChunkKeysForPaths(targetPaths)
				// Treat --all as "every recorded path" downstream (Glacier job-save
				// and the actual BatchRestore both read filePaths).
				filePaths = targetPaths

			case hash != "":
				// Resolve hash to path so we can get chunk keys.
				q := manifest.NewManifestQuery(m)
				entry, ok := q.FindFileByHash(hash)
				if !ok {
					return fmt.Errorf("no file with content hash %q in manifest", hash)
				}
				targetPaths = []string{entry.Path}
				chunkKeys = se.ChunkKeysForPaths(targetPaths)

			case dvcStage != "":
				chunkKeys = se.ChunkKeysForDVCStage(dvcStage)
				if len(chunkKeys) == 0 {
					return fmt.Errorf("no files found for DVC stage %q", dvcStage)
				}

			case gitCommit != "":
				chunkKeys = se.ChunkKeysForCommit(gitCommit)
				if len(chunkKeys) == 0 {
					return fmt.Errorf("no files found for git commit %q", gitCommit)
				}

			case len(filePaths) > 0:
				targetPaths = filePaths
				chunkKeys = se.ChunkKeysForPaths(targetPaths)

			default:
				return fmt.Errorf("specify at least one of --hash, --file, --git-commit, or --dvc-stage")
			}

			// Glacier pre-flight check when a tier is requested or as a safety check.
			restoreTier := s3pkg.RestoreTier(tier)
			if restoreTier == "" {
				restoreTier = s3pkg.DefaultRestoreTier
			}

			gr := s3pkg.NewGlacierRestorer(s3Client, restoreDays)
			report, err := gr.CheckAndRestore(ctx, bucket, chunkKeys, restoreTier)
			if err != nil {
				return fmt.Errorf("glacier pre-flight check failed: %w", err)
			}

			// Budget guard.
			if maxRestoreCost > 0 && report.EstimatedCostUSD > maxRestoreCost {
				return fmt.Errorf("estimated retrieval cost $%.4f exceeds --max-restore-cost $%.4f; aborting",
					report.EstimatedCostUSD, maxRestoreCost)
			}

			// Print Glacier status.
			if summary := s3pkg.FormatAccessibilityReport(report, restoreTier); summary != "" {
				fmt.Print(summary)
			}

			if dryRun {
				// Calculate total restore size from manifest chunks.
				var totalBytes int64
				for _, c := range m.Chunks {
					for _, key := range chunkKeys {
						if c.S3Key == key {
							totalBytes += c.CompressedSize
							break
						}
					}
				}
				fmt.Printf("\n📊 Dry-run summary:\n")
				fmt.Printf("   Chunks:            %d\n", len(chunkKeys))
				fmt.Printf("   Data (compressed): %s\n", humanize.Bytes(uint64(totalBytes)))
				if report.EstimatedCostUSD > 0 {
					fmt.Printf("   Retrieval cost:    $%.4f USD (%s tier)\n", report.EstimatedCostUSD, restoreTier)
				}
				return nil
			}

			// If objects are not yet accessible, either wait or advise the user.
			if !report.AllAccessible() {
				if wait {
					pending := append(report.InProgress, report.JustRequested...)
					fmt.Printf("⏳ Waiting for %d chunk(s) to be restored from Glacier…\n", len(pending))
					if err := gr.WaitForRestore(ctx, bucket, pending, s3pkg.DefaultGlacierPollInterval, func(n int) {
						fmt.Printf("   Still waiting for %d chunk(s)…\n", n)
					}); err != nil {
						return fmt.Errorf("waiting for Glacier restore: %w", err)
					}
					fmt.Println("✅ All chunks are accessible.")
				} else {
					// Save a job so the user can check and download later.
					jobStore, storeErr := restore.NewDefaultStore()
					if storeErr == nil {
						sel := restore.SelectionCriteria{
							Hash:      hash,
							FilePaths: filePaths,
							GitCommit: gitCommit,
							DVCStage:  dvcStage,
						}
						job, jobErr := jobStore.NewJob(s3URL, outputDir, region,
							string(restoreTier), restoreDays, bucket, chunkKeys, sel,
							report.EstimatedCostUSD)
						if jobErr == nil {
							fmt.Printf("\n💾 Restore job saved: %s\n", job.ID)
							fmt.Printf("   Check status:  cargoship restore jobs check %s\n", job.ID)
							fmt.Printf("   Download when ready: cargoship restore jobs download %s\n", job.ID)
						}
					}
					fmt.Printf("\n⚠️  %d chunk(s) are not yet accessible.\n", len(report.Frozen)+len(report.InProgress))
					return nil
				}
			}

			// Perform the actual restore.
			var stats *manifest.RestoreStats

			switch {
			case hash != "":
				fmt.Printf("🔍 Restoring file by hash: %s\n", hash)
				stats, err = se.ExtractFileByHash(ctx, hash, outputDir)

			case dvcStage != "":
				fmt.Printf("🔍 Restoring DVC stage: %s\n", dvcStage)
				stats, err = se.BatchRestoreByDVCStage(ctx, dvcStage, outputDir)

			case gitCommit != "":
				fmt.Printf("🔍 Restoring git commit: %s\n", gitCommit)
				stats, err = se.BatchRestoreByCommit(ctx, gitCommit, outputDir)

			default:
				fmt.Printf("🔍 Restoring %d file(s) by path\n", len(filePaths))
				stats, err = se.BatchRestore(ctx, filePaths, outputDir)
			}

			if err != nil {
				return fmt.Errorf("restore failed: %w", err)
			}

			// #336: BatchRestore is deliberately partially tolerant — it counts a
			// failure and carries on rather than aborting, so err == nil even
			// when nothing was restored. Reporting that as success is the worst
			// failure mode this tool has: restore is overwhelmingly scripted
			// (`cargoship restore ... && rm -rf ./local`), so a green exit code
			// on an empty restore is what deletes someone's only copy. The exit
			// code has to agree with the numbers printed beside it.
			if jsonOutput {
				fmt.Printf(`{"restored":%d,"failed":%d,"bytes":%d,"chunks_downloaded":%d,"retrieval_cost_usd":%.4f}`+"\n",
					stats.Restored, stats.Failed, stats.Bytes, stats.ChunksDownloaded, report.EstimatedCostUSD)
				return restoreOutcomeError(stats)
			}

			// No success banner when nothing came back — a total failure should
			// not read as a completed restore.
			if stats.Restored > 0 {
				fmt.Printf("✅ Restore complete!\n")
			} else {
				fmt.Printf("❌ Restore failed: no files were restored\n")
			}
			fmt.Printf("   Files restored:    %d\n", stats.Restored)
			if stats.Failed > 0 {
				fmt.Printf("   Files failed:      %d\n", stats.Failed)
			}
			fmt.Printf("   Data written:      %s\n", humanize.Bytes(uint64(stats.Bytes)))
			fmt.Printf("   Chunks downloaded: %d\n", stats.ChunksDownloaded)
			if report.EstimatedCostUSD > 0 {
				fmt.Printf("   Retrieval cost:    $%.4f USD\n", report.EstimatedCostUSD)
			}
			fmt.Printf("   Output directory:  %s\n", outputDir)

			return restoreOutcomeError(stats)
		},
	}

	cmd.Flags().StringVar(&hash, "hash", "", "MD5 content hash of the file to restore")
	cmd.Flags().StringArrayVar(&filePaths, "file", nil, "Exact file path(s) to restore (repeatable)")
	cmd.Flags().StringVar(&gitCommit, "git-commit", "", "Restore all files from this git commit SHA")
	cmd.Flags().StringVar(&dvcStage, "dvc-stage", "", "Restore all files produced by this DVC pipeline stage")
	cmd.Flags().BoolVar(&all, "all", false, "Restore every file recorded in the upload's manifest (disaster recovery); cannot be combined with --hash/--file/--git-commit/--dvc-stage")
	cmd.Flags().StringVarP(&region, "region", "r", "us-east-1", "AWS region")
	cmd.Flags().Int64Var(&cacheGB, "cache-gb", 10, "LRU chunk cache size in GB (0 = default 10 GB)")
	cmd.Flags().BoolVar(&jsonOutput, "json", false, "Output restore statistics as JSON")
	cmd.Flags().StringVar(&tier, "tier", "", "Glacier retrieval tier: expedited, standard (default), bulk")
	cmd.Flags().BoolVar(&wait, "wait", false, "Block until Glacier restoration completes before downloading")
	cmd.Flags().BoolVar(&dryRun, "dry-run", false, "Show what would be restored without downloading")
	cmd.Flags().Float64Var(&maxRestoreCost, "max-restore-cost", 0, "Abort if estimated retrieval cost exceeds this USD amount")
	cmd.Flags().Int32Var(&restoreDays, "restore-days", 7, "Days to keep Glacier restored copy available")
	cmd.Flags().BoolVar(&noVerify, "no-verify", false, "Skip restore-time checksum verification (faster, but won't detect corrupted stored data)")
	cmd.Flags().BoolVar(&flatten, "flatten", false, "Write restored files by basename into the output dir instead of recreating their directory structure")
	cmd.Flags().StringVar(&datasetID, "dataset-id", "", "Restore a version of this dataset (S3_URL is then the bucket/prefix); see 'cargoship dataset list'")
	cmd.Flags().IntVar(&version, "version", 0, "With --dataset-id: the version ordinal to restore (default: latest)")
	cmd.Flags().StringVar(&asOfStr, "as-of", "", "With --dataset-id: restore the newest version at or before this date (YYYY-MM-DD)")

	cmd.AddCommand(newRestoreJobsCmd())
	return cmd
}

// allFilePaths returns every file path recorded in the manifest (the stored paths
// `restore --file` expects). Used by `restore --all` for whole-upload disaster
// recovery. SelectiveExtractor.restorePath strips SourcePath for the on-disk layout.
func allFilePaths(m *manifest.Manifest) []string {
	paths := make([]string, 0, len(m.Files))
	for _, f := range m.Files {
		paths = append(paths, f.Path)
	}
	return paths
}

// loadEffectiveManifest downloads the manifest for uploadID and, when it is an
// incremental-sync delta (PreviousManifestID set), resolves the chain into the
// full effective dataset (newest-wins per path + deletion tombstones), so the
// caller sees every current file — not just that version's delta (#552).
// Shared by `restore` and `browse` so they can't diverge on chain handling.
// `verify` deliberately uses ResolveChain directly (it validates each version),
// so it does not use this.
func loadEffectiveManifest(ctx context.Context, s3Client *s3.Client, kmsClient *kms.Client, bucket, actualPrefix, uploadID string) (*manifest.Manifest, error) {
	fmt.Printf("📥 Loading manifest: s3://%s/%s/uploads/%s\n", bucket, actualPrefix, uploadID)
	m, err := manifest.DownloadFromS3WithDecryption(ctx, s3Client, kmsClient, bucket, actualPrefix, uploadID)
	if err != nil {
		return nil, fmt.Errorf("failed to load manifest: %w", err)
	}
	fmt.Printf("✅ Manifest loaded: %d files, %d chunks\n\n", m.TotalFiles, m.TotalChunks)

	if m.PreviousManifestID != "" {
		fetch := func(ctx context.Context, prevID string) (*manifest.Manifest, error) {
			return manifest.DownloadFromS3WithDecryption(ctx, s3Client, kmsClient, bucket, actualPrefix, prevID)
		}
		merged, err := manifest.ResolveEffective(ctx, m, fetch)
		if err != nil {
			return nil, fmt.Errorf("failed to resolve incremental version chain: %w", err)
		}
		fmt.Printf("🔗 Resolved incremental version chain: %d files across the full dataset\n\n", merged.TotalFiles)
		m = merged
	}
	return m, nil
}

// restoreOutcomeError converts restore statistics into the command's error
// result, so the exit code reflects what actually happened. BatchRestore counts
// per-file failures and continues rather than returning an error, so without
// this a restore that produced nothing exits 0. (#336)
func restoreOutcomeError(stats *manifest.RestoreStats) error {
	if stats.Failed == 0 {
		return nil
	}
	if stats.Restored == 0 {
		return fmt.Errorf("restore failed: 0 of %d file(s) restored", stats.Failed)
	}
	return fmt.Errorf("restore incomplete: %d of %d file(s) failed",
		stats.Failed, stats.Restored+stats.Failed)
}
