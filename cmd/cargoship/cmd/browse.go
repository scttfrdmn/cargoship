package cmd

import (
	"context"
	"fmt"
	"strings"

	"github.com/aws/aws-sdk-go-v2/config"
	"github.com/aws/aws-sdk-go-v2/service/kms"
	"github.com/aws/aws-sdk-go-v2/service/s3"
	"github.com/spf13/cobra"

	s3pkg "github.com/scttfrdmn/cargoship/pkg/aws/s3"
	"github.com/scttfrdmn/cargoship/pkg/manifest"
	"github.com/scttfrdmn/cargoship/pkg/tui"
)

// NewBrowseCmd creates the 'browse' command for interactive TUI file browsing
// and selective restore (Issue #190, #200, #201).
func NewBrowseCmd() *cobra.Command {
	var (
		region         string
		cacheGB        int64
		tier           string
		wait           bool
		maxRestoreCost float64
		restoreDays    int32
	)

	cmd := &cobra.Command{
		Use:   "browse S3_URL [OUTPUT_DIR]",
		Short: "Interactively browse and restore files from a CargoShip archive",
		Long: `Open an interactive terminal UI to browse manifest contents and select files for restore.

Navigation:
  ↑/↓          Navigate file list
  space        Toggle selection on highlighted file
  enter        Confirm restore of selected files
  /            Enter incremental search mode
  d            Cycle DVC stage filter
  g            Cycle git commit filter
  a            Select all visible files
  c            Clear selection
  q / ctrl+c   Quit without restoring

Glacier/Deep Archive:
  --tier       Retrieval tier if files are archived: expedited, standard (default), bulk
  --wait       Block until Glacier restoration completes before downloading
  --max-restore-cost  Abort restore if estimated retrieval cost exceeds this USD limit

Examples:
  # Open the interactive browser
  cargoship browse s3://my-bucket/uploads/20240101-abc123 ./restored

  # Use a larger cache for big datasets
  cargoship browse s3://my-bucket/uploads/20240101-abc123 ./restored --cache-gb 20

  # Restore from Glacier with standard tier, wait for completion
  cargoship browse s3://my-bucket/uploads/20240101-abc123 --tier standard --wait
`,
		Args: cobra.RangeArgs(1, 2),
		RunE: func(cmd *cobra.Command, args []string) error {
			ctx := context.Background()
			s3URL := args[0]

			// OUTPUT_DIR can be provided as arg or supplied interactively in the TUI.
			var outputDirHint string
			if len(args) == 2 {
				outputDirHint = args[1]
			}

			bucket, prefix, err := parseS3URL(s3URL)
			if err != nil {
				return fmt.Errorf("invalid S3 URL: %w", err)
			}

			cfg, err := config.LoadDefaultConfig(ctx, config.WithRegion(region))
			if err != nil {
				return fmt.Errorf("failed to load AWS config: %w", err)
			}

			s3Client := s3.NewFromConfig(cfg)
			kmsClient := kms.NewFromConfig(cfg)

			// Parse upload ID from prefix.
			var actualPrefix, uploadID string
			if idx := strings.Index(prefix, "/uploads/"); idx != -1 {
				actualPrefix = prefix[:idx]
				uploadID = prefix[idx+9:]
			} else {
				uploadID = prefix
			}

			fmt.Printf("📥 Loading manifest: s3://%s/%s\n", bucket, prefix)
			m, err := manifest.DownloadFromS3WithDecryption(ctx, s3Client, kmsClient, bucket, actualPrefix, uploadID)
			if err != nil {
				return fmt.Errorf("failed to load manifest: %w", err)
			}
			fmt.Printf("✅ Manifest loaded: %d files, %d chunks\n\n", m.TotalFiles, m.TotalChunks)

			_ = outputDirHint // pre-populated in the TUI destination field if provided

			result, err := tui.RunBrowser(m, s3Client)
			if err != nil {
				return fmt.Errorf("browser error: %w", err)
			}

			if result.Cancelled || len(result.SelectedPaths) == 0 {
				fmt.Println("No files selected.")
				return nil
			}

			destDir := result.DestDir
			if destDir == "" {
				if outputDirHint != "" {
					destDir = outputDirHint
				} else {
					return fmt.Errorf("no output directory specified")
				}
			}

			maxCacheBytes := cacheGB * 1024 * 1024 * 1024
			se := manifest.NewSelectiveExtractor(m, s3Client, maxCacheBytes)

			// Glacier pre-flight check on the chunks needed for selected files.
			restoreTier := s3pkg.RestoreTier(tier)
			if restoreTier == "" {
				restoreTier = s3pkg.DefaultRestoreTier
			}
			chunkKeys := se.ChunkKeysForPaths(result.SelectedPaths)

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

			if summary := s3pkg.FormatAccessibilityReport(report, restoreTier); summary != "" {
				fmt.Print(summary)
			}

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
					fmt.Printf("\n⚠️  %d chunk(s) are not yet accessible.\n", len(report.Frozen)+len(report.InProgress))
					fmt.Println("   Re-run with --wait to block until restoration completes.")
					return nil
				}
			}

			fmt.Printf("🚀 Restoring %d file(s) to %s…\n", len(result.SelectedPaths), destDir)
			stats, err := se.BatchRestore(ctx, result.SelectedPaths, destDir)
			if err != nil {
				return fmt.Errorf("restore failed: %w", err)
			}

			fmt.Printf("✅ Restore complete!\n")
			fmt.Printf("   Files restored:    %d\n", stats.Restored)
			if stats.Failed > 0 {
				fmt.Printf("   Files failed:      %d\n", stats.Failed)
			}
			fmt.Printf("   Chunks downloaded: %d\n", stats.ChunksDownloaded)
			if report.EstimatedCostUSD > 0 {
				fmt.Printf("   Retrieval cost:    $%.4f USD\n", report.EstimatedCostUSD)
			}
			fmt.Printf("   Output directory:  %s\n", destDir)
			return nil
		},
	}

	cmd.Flags().StringVarP(&region, "region", "r", "us-east-1", "AWS region")
	cmd.Flags().Int64Var(&cacheGB, "cache-gb", 10, "LRU chunk cache size in GB")
	cmd.Flags().StringVar(&tier, "tier", "", "Glacier retrieval tier: expedited, standard (default), bulk")
	cmd.Flags().BoolVar(&wait, "wait", false, "Block until Glacier restoration completes before downloading")
	cmd.Flags().Float64Var(&maxRestoreCost, "max-restore-cost", 0, "Abort if estimated retrieval cost exceeds this USD amount")
	cmd.Flags().Int32Var(&restoreDays, "restore-days", 7, "Days to keep Glacier restored copy available")

	return cmd
}
