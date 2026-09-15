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
	"github.com/spf13/cobra"

	"github.com/scttfrdmn/cargoship/pkg/pipeline"
)

func newGhostshipRunCmd() *cobra.Command {
	var (
		interval      time.Duration
		once          bool
		writerID      string
		region        string
		storageClass  string
		shardCount    int
		shardStrategy string
		compression   int
		trackDeletes  bool
	)
	cmd := &cobra.Command{
		Use:   "run SOURCE_DIR S3_URL",
		Short: "Run an unattended, writer-isolated incremental backup on a schedule",
		Long: `Continuously back up SOURCE_DIR to s3://BUCKET/PREFIX on an interval, using
CargoShip's real incremental sync engine (chunked archives, manifests, dataset
versioning) — the same engine as 'cargoship sync', just scheduled and headless.

Each cycle uploads only what changed since the previous manifest. With --writer-id
the objects are isolated under writers/<id>/ so a fleet sharing one bucket never
collides. Outbound-only: no inbound port, no daemon socket. Stops cleanly on
SIGINT/SIGTERM.

Examples:
  cargoship ghostship run /volume1/Documents s3://backups/nas --writer-id lab-nas-1
  cargoship ghostship run ./data s3://backups/dev --writer-id auto --interval 30m
  cargoship ghostship run ./data s3://backups/dev --once   # one cycle then exit (cron)`,
		Args: cobra.ExactArgs(2),
		RunE: func(cmd *cobra.Command, args []string) error {
			sourceDir, s3URL := args[0], args[1]

			info, err := os.Stat(sourceDir)
			if err != nil {
				return fmt.Errorf("source path error: %w", err)
			}
			if !info.IsDir() {
				return fmt.Errorf("source path must be a directory: %s", sourceDir)
			}

			bucket, prefix, err := parseS3URL(s3URL)
			if err != nil {
				return fmt.Errorf("invalid S3 URL: %w", err)
			}
			resolvedWriterID, err := pipeline.ResolveWriterID(writerID)
			if err != nil {
				return fmt.Errorf("invalid --writer-id: %w", err)
			}
			prefix = pipeline.WriterPrefix(prefix, resolvedWriterID)

			if err := pipeline.ValidateShardStrategy(shardStrategy); err != nil {
				return err
			}

			cfg, err := config.LoadDefaultConfig(cmd.Context(), config.WithRegion(region))
			if err != nil {
				return fmt.Errorf("failed to load AWS config: %w", err)
			}
			s3Client := s3.NewFromConfig(cfg)

			params := syncRunParams{
				s3Client:         s3Client,
				bucket:           bucket,
				prefix:           prefix,
				sourcePath:       sourceDir,
				region:           region,
				writerID:         resolvedWriterID,
				storageClass:     storageClass,
				shardCount:       shardCount,
				shardStrategy:    shardStrategy,
				compressionLevel: compression,
				trackDeletes:     trackDeletes,
			}

			logger := slog.New(slog.NewTextHandler(cmd.ErrOrStderr(), nil)).With(
				"component", "ghostship", "writer_id", displayWriterID(resolvedWriterID),
				"source", sourceDir, "dest", fmt.Sprintf("s3://%s/%s", bucket, prefix))

			// Stop cleanly on SIGINT/SIGTERM (outbound-only daemon; no listeners).
			ctx, stop := signal.NotifyContext(cmd.Context(), syscall.SIGINT, syscall.SIGTERM)
			defer stop()

			cycle := func(ctx context.Context) error {
				res, err := runOneSync(ctx, params)
				if err != nil {
					logger.Error("sync cycle failed", "err", err)
					return err
				}
				switch {
				case res.NoChanges:
					logger.Info("no changes; nothing to back up")
				case res.Result != nil:
					logger.Info("backup cycle complete",
						"upload_id", res.Result.UploadID,
						"files", res.Result.TotalFiles,
						"bytes", res.Result.TotalBytes,
						"sync_type", res.SyncType)
				}
				return nil
			}

			if !once {
				logger.Info("ghostship started", "interval", interval.String())
			}
			return runLoop(ctx, interval, once, cycle)
		},
	}
	cmd.Flags().DurationVar(&interval, "interval", time.Hour, "How often to run a backup cycle")
	cmd.Flags().BoolVar(&once, "once", false, "Run a single cycle and exit (for cron / testing)")
	cmd.Flags().StringVar(&writerID, "writer-id", "", "Writer identity for fleet isolation (writers/<id>/). 'auto' derives a stable per-host id")
	cmd.Flags().StringVarP(&region, "region", "r", "us-west-2", "AWS region")
	cmd.Flags().StringVar(&storageClass, "storage-class", "STANDARD", "S3 storage class (STANDARD, GLACIER_IR, DEEP_ARCHIVE)")
	cmd.Flags().IntVar(&shardCount, "shard-count", 10, "Number of shards for parallel uploads (1-100)")
	cmd.Flags().StringVar(&shardStrategy, "shard-strategy", pipeline.ShardStrategyRoundRobin,
		"Shard distribution strategy (round-robin, hash, size, type, directory)")
	cmd.Flags().IntVar(&compression, "compression-level", 0, "Fixed zstd level (1-22); 0 = content-aware per-chunk selection")
	cmd.Flags().BoolVar(&trackDeletes, "track-deletes", false, "Record files deleted since the last backup in the manifest")
	return cmd
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
