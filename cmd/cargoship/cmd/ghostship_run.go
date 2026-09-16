package cmd

import (
	"context"
	"fmt"
	"log/slog"
	"os"
	"os/signal"
	"syscall"
	"time"

	"github.com/aws/aws-sdk-go-v2/config"
	"github.com/aws/aws-sdk-go-v2/service/s3"
	"github.com/google/uuid"
	"github.com/spf13/cobra"

	versionpkg "github.com/scttfrdmn/cargoship/internal/version"
	"github.com/scttfrdmn/cargoship/pkg/aws/cost"
	"github.com/scttfrdmn/cargoship/pkg/fleet"
	"github.com/scttfrdmn/cargoship/pkg/launch"
	"github.com/scttfrdmn/cargoship/pkg/pipeline"
)

func newGhostshipRunCmd() *cobra.Command {
	var (
		configPath    string
		interval      time.Duration
		once          bool
		flagWriterID  string
		region        string
		storageClass  string
		shardCount    int
		shardStrategy string
		compression   int
		trackDeletes  bool
		ignoreBudget  bool
	)
	cmd := &cobra.Command{
		Use:   "run [SOURCE_DIR S3_URL]",
		Short: "Run an unattended, writer-isolated incremental backup on a schedule",
		Long: `Continuously back up on an interval using CargoShip's real incremental sync engine
(chunked archives, manifests, dataset versioning) — the same engine as 'cargoship
sync', just scheduled and headless. Each cycle uploads only what changed since the
previous manifest.

Two forms:
  Single source (flags):  cargoship ghostship run SOURCE_DIR s3://BUCKET/PREFIX --writer-id ID
  Config file (fleet):    cargoship ghostship run --config box.yaml

With a config file, each entry in 'watch_paths' is backed up as its own source under
one writer prefix (writers/<id>/). 'archival_rules' are ignored in sync mode (they
belong to the legacy per-file model). Outbound-only: no inbound port. Stops cleanly on
SIGINT/SIGTERM; --once runs a single cycle (for cron).

Examples:
  cargoship ghostship run /volume1/Documents s3://backups/nas --writer-id lab-nas-1
  cargoship ghostship run --config /etc/cargoship/box.yaml --interval 30m
  cargoship ghostship run --config box.yaml --once`,
		Args: cobra.MaximumNArgs(2),
		RunE: func(cmd *cobra.Command, args []string) error {
			if err := pipeline.ValidateShardStrategy(shardStrategy); err != nil {
				return err
			}
			d := runDefaults{
				region: region, storageClass: storageClass, shardCount: shardCount,
				shardStrategy: shardStrategy, compression: compression, trackDeletes: trackDeletes,
			}

			var (
				writerID      string
				sources       []syncRunParams
				effInterval   = interval
				configVersion int
				err           error
			)

			if configPath != "" {
				if len(args) != 0 {
					return fmt.Errorf("--config takes no positional args (got %d); use either --config or SOURCE_DIR S3_URL", len(args))
				}
				cfg, lerr := loadGhostshipConfigFile(configPath)
				if lerr != nil {
					return lerr
				}
				issues := launch.ValidateConfig(cfg)
				if hasConfigErrors(issues) {
					for _, is := range issues {
						if is.Severity == launch.SeverityError {
							_, _ = fmt.Fprintf(cmd.ErrOrStderr(), "config error: %s: %s\n", is.Field, is.Message)
						}
					}
					return fmt.Errorf("invalid config %s", configPath)
				}
				var warnings []string
				writerID, sources, warnings, err = buildRunPlan(cfg, flagWriterID, d)
				if err != nil {
					return err
				}
				configVersion = cfg.Version
				for _, w := range warnings {
					_, _ = fmt.Fprintf(cmd.ErrOrStderr(), "warning: %s\n", w)
				}
				if !cmd.Flags().Changed("interval") && cfg.ScanInterval > 0 {
					effInterval = cfg.ScanInterval
				}
			} else {
				if len(args) != 2 {
					return fmt.Errorf("require SOURCE_DIR and S3_URL, or use --config")
				}
				sourceDir, s3URL := args[0], args[1]
				info, serr := os.Stat(sourceDir)
				if serr != nil {
					return fmt.Errorf("source path error: %w", serr)
				}
				if !info.IsDir() {
					return fmt.Errorf("source path must be a directory: %s", sourceDir)
				}
				bucket, prefix, perr := parseS3URL(s3URL)
				if perr != nil {
					return fmt.Errorf("invalid S3 URL: %w", perr)
				}
				writerID, err = pipeline.ResolveWriterID(flagWriterID)
				if err != nil {
					return fmt.Errorf("invalid --writer-id: %w", err)
				}
				sources = []syncRunParams{{
					bucket: bucket, prefix: pipeline.WriterPrefix(prefix, writerID), sourcePath: sourceDir,
					region: region, writerID: writerID, projectID: writerID, storageClass: storageClass, shardCount: shardCount,
					shardStrategy: shardStrategy, compressionLevel: compression, trackDeletes: trackDeletes,
				}}
			}

			if len(sources) == 0 {
				return fmt.Errorf("no sources to back up (config has no watch_paths)")
			}

			cfg, cerr := config.LoadDefaultConfig(cmd.Context(), config.WithRegion(region))
			if cerr != nil {
				return fmt.Errorf("failed to load AWS config: %w", cerr)
			}
			s3Client := s3.NewFromConfig(cfg)

			// #629: per-writer cap gate. Build the cost manager once; nil = enforcement
			// off (either --ignore-budget or the manager couldn't load — fail-open, a
			// backup shouldn't be blocked because cost tracking is unavailable).
			var costMgr *cost.Manager
			if !ignoreBudget {
				if mgr, mErr := loadCostManager(cmd.Context()); mErr == nil {
					costMgr = mgr
				}
			}
			for i := range sources {
				sources[i].s3Client = s3Client
				sources[i].costMgr = costMgr
			}

			logger := slog.New(slog.NewTextHandler(cmd.ErrOrStderr(), nil)).With(
				"component", "ghostship", "writer_id", displayWriterID(writerID))

			// Outbound-only daemon; stop cleanly on SIGINT/SIGTERM.
			ctx, stop := signal.NotifyContext(cmd.Context(), syscall.SIGINT, syscall.SIGTERM)
			defer stop()

			// #615: heartbeat identity + in-memory per-source accumulator (so LastSuccess
			// persists across cycles without reading S3 — stays write-only). InstanceID is
			// per-boot so two live instances under one writer id (a cloned VM) are visible.
			instanceID := uuid.New().String()
			host, herr := os.Hostname()
			if herr != nil || host == "" {
				host = "unknown"
			}
			statusBySource := make(map[string]*fleet.SourceStatus, len(sources))
			for _, p := range sources {
				statusBySource[p.sourcePath] = &fleet.SourceStatus{Path: p.sourcePath}
			}

			cycle := func(ctx context.Context) error {
				var firstErr error
				for _, p := range sources {
					srcLog := logger.With("source", p.sourcePath,
						"dest", fmt.Sprintf("s3://%s/%s", p.bucket, p.prefix))
					ss := statusBySource[p.sourcePath]
					res, rerr := runOneSync(ctx, p)
					if rerr != nil {
						srcLog.Error("sync cycle failed", "err", rerr)
						ss.OK = false
						ss.LastError = rerr.Error()
						if firstErr == nil {
							firstErr = rerr
						}
						continue
					}
					ss.OK = true
					ss.LastError = ""
					ss.LastSuccess = time.Now()
					ss.NoChanges = res.NoChanges
					ss.SyncType = res.SyncType
					if res.Result != nil {
						ss.UploadID = res.Result.UploadID
						ss.Files = res.Result.TotalFiles
						ss.Bytes = res.Result.TotalBytes
					}
					switch {
					case res.NoChanges:
						srcLog.Info("no changes; nothing to back up")
					case res.Result != nil:
						srcLog.Info("backup cycle complete",
							"upload_id", res.Result.UploadID, "files", res.Result.TotalFiles,
							"bytes", res.Result.TotalBytes, "sync_type", res.SyncType)
					}
				}

				// #615: write one aggregated heartbeat for this writer (best-effort).
				sts := make([]fleet.SourceStatus, 0, len(sources))
				for _, p := range sources {
					sts = append(sts, *statusBySource[p.sourcePath])
				}
				st := fleet.WriterStatus{
					WriterID:         writerID,
					Hostname:         host,
					InstanceID:       instanceID,
					CargoshipVersion: versionpkg.Version,
					ConfigVersion:    configVersion,
					UpdatedAt:        time.Now(),
					Sources:          sts,
				}
				if werr := fleet.WriteStatus(ctx, sources[0].s3Client, sources[0].bucket, sources[0].prefix, st); werr != nil {
					logger.Warn("failed to write heartbeat", "err", werr)
				}
				return firstErr
			}

			if !once {
				logger.Info("ghostship started", "sources", len(sources), "interval", effInterval.String())
			}
			return runLoop(ctx, effInterval, once, cycle)
		},
	}
	cmd.Flags().StringVar(&configPath, "config", "", "Ghostship config file (fleet mode); backs up each watch_paths entry")
	cmd.Flags().DurationVar(&interval, "interval", time.Hour, "How often to run a backup cycle (overrides config scan_interval)")
	cmd.Flags().BoolVar(&once, "once", false, "Run a single cycle and exit (for cron / testing)")
	cmd.Flags().StringVar(&flagWriterID, "writer-id", "", "Writer identity for fleet isolation (writers/<id>/). 'auto' derives a stable per-host id; overrides the config")
	cmd.Flags().StringVarP(&region, "region", "r", "us-west-2", "AWS region")
	cmd.Flags().StringVar(&storageClass, "storage-class", "STANDARD", "Default S3 storage class (per-source storage_class in config overrides)")
	cmd.Flags().IntVar(&shardCount, "shard-count", 10, "Number of shards for parallel uploads (1-100)")
	cmd.Flags().StringVar(&shardStrategy, "shard-strategy", pipeline.ShardStrategyRoundRobin,
		"Shard distribution strategy (round-robin, hash, size, type, directory)")
	cmd.Flags().IntVar(&compression, "compression-level", 0, "Fixed zstd level (1-22); 0 = content-aware per-chunk selection")
	cmd.Flags().BoolVar(&trackDeletes, "track-deletes", false, "Record files deleted since the last backup in the manifest")
	cmd.Flags().BoolVar(&ignoreBudget, "ignore-budget", false, "Skip the per-writer budget/volume cap check (#629)")
	return cmd
}

// runDefaults are the box-wide sync settings from flags, applied to every source a
// config-driven run backs up (per-source config values may override some).
type runDefaults struct {
	region        string
	storageClass  string
	shardCount    int
	shardStrategy string
	compression   int
	trackDeletes  bool
}

// buildRunPlan turns a ghostship config into the per-source sync params for the
// daemon loop (#604), pure and S3-free. Writer-id precedence: flagWriterID >
// cfg.WriterID > cfg.ID > CARGOSHIP_WRITER_ID env. Each watch path becomes one
// writer-scoped source under writers/<id>/ at the bucket root; per-source
// storage_class overrides the run default. Returns a warning when archival_rules
// are present (ignored in sync mode).
func buildRunPlan(cfg *launch.GhostShipConfig, flagWriterID string, d runDefaults) (writerID string, sources []syncRunParams, warnings []string, err error) {
	switch {
	case flagWriterID != "":
		writerID, err = pipeline.ResolveWriterID(flagWriterID)
	case cfg.WriterID != "":
		writerID, err = pipeline.SanitizeWriterID(cfg.WriterID)
	case cfg.ID != "":
		writerID, err = pipeline.SanitizeWriterID(cfg.ID)
	default:
		writerID, err = pipeline.ResolveWriterID("") // env, else ""
	}
	if err != nil {
		return "", nil, nil, fmt.Errorf("resolve writer id: %w", err)
	}

	prefix := pipeline.WriterPrefix("", writerID)
	for _, wp := range cfg.WatchPaths {
		sc := d.storageClass
		if wp.StorageClass != "" {
			sc = wp.StorageClass
		}
		sources = append(sources, syncRunParams{
			bucket:           cfg.S3Config.Bucket,
			prefix:           prefix,
			sourcePath:       wp.Path,
			region:           d.region,
			writerID:         writerID,
			projectID:        writerID, // #629: per-writer cap key
			storageClass:     sc,
			shardCount:       d.shardCount,
			shardStrategy:    d.shardStrategy,
			compressionLevel: d.compression,
			trackDeletes:     d.trackDeletes,
		})
	}
	if len(cfg.ArchivalRules) > 0 {
		warnings = append(warnings, fmt.Sprintf(
			"%d archival_rules present; ignored in sync mode (ghostship run does directory sync, not per-file rule archival)",
			len(cfg.ArchivalRules)))
	}
	return writerID, sources, warnings, nil
}

func hasConfigErrors(issues []launch.ConfigIssue) bool {
	for _, is := range issues {
		if is.Severity == launch.SeverityError {
			return true
		}
	}
	return false
}

// runLoop runs fn immediately, then (unless once) every interval until the context
// is canceled. When once is true it returns fn's error; in loop mode per-cycle
// errors are handled by fn (logged) and the loop keeps running — an unattended
// backup should survive a transient failure and try again next interval.
func runLoop(ctx context.Context, interval time.Duration, once bool, fn func(context.Context) error) error {
	err := fn(ctx)
	if once {
		return err
	}
	t := time.NewTicker(interval)
	defer t.Stop()
	for {
		select {
		case <-ctx.Done():
			return nil
		case <-t.C:
			_ = fn(ctx)
		}
	}
}

func displayWriterID(id string) string {
	if id == "" {
		return "(single-writer)"
	}
	return id
}
