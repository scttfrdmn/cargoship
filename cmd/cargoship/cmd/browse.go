package cmd

import (
	"context"
	"fmt"
	"strings"

	"github.com/aws/aws-sdk-go-v2/config"
	"github.com/aws/aws-sdk-go-v2/service/kms"
	"github.com/aws/aws-sdk-go-v2/service/s3"
	"github.com/spf13/cobra"

	"github.com/scttfrdmn/cargoship/pkg/manifest"
	"github.com/scttfrdmn/cargoship/pkg/tui"
)

// NewBrowseCmd creates the 'browse' command for interactive TUI file browsing
// and selective restore (Issue #190).
func NewBrowseCmd() *cobra.Command {
	var (
		region  string
		cacheGB int64
	)

	cmd := &cobra.Command{
		Use:   "browse S3_URL OUTPUT_DIR",
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

After pressing enter, you will be prompted for an output directory.
The restore uses the SelectiveExtractor which groups files by chunk to
minimise S3 downloads (one download per distinct chunk).

Examples:
  # Open the interactive browser
  cargoship browse s3://my-bucket/uploads/20240101-abc123 ./restored

  # Use a larger cache for big datasets
  cargoship browse s3://my-bucket/uploads/20240101-abc123 ./restored --cache-gb 20
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
			_ = cacheGB       // passed to SelectiveExtractor inside RunBrowser

			result, err := tui.RunBrowser(m, s3Client)
			if err != nil {
				return fmt.Errorf("browser error: %w", err)
			}

			if result.Cancelled || len(result.SelectedPaths) == 0 {
				fmt.Println("No files selected.")
				return nil
			}

			// Perform the restore using SelectiveExtractor.
			maxCacheBytes := cacheGB * 1024 * 1024 * 1024
			se := manifest.NewSelectiveExtractor(m, s3Client, maxCacheBytes)
			destDir := result.DestDir
			if destDir == "" {
				if outputDirHint != "" {
					destDir = outputDirHint
				} else {
					return fmt.Errorf("no output directory specified")
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
			fmt.Printf("   Output directory:  %s\n", destDir)
			return nil
		},
	}

	cmd.Flags().StringVarP(&region, "region", "r", "us-east-1", "AWS region")
	cmd.Flags().Int64Var(&cacheGB, "cache-gb", 10, "LRU chunk cache size in GB")

	return cmd
}
