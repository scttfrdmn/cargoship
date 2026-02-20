package cmd

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strings"

	awsconfig "github.com/aws/aws-sdk-go-v2/config"
	"github.com/aws/aws-sdk-go-v2/service/s3"
	"github.com/spf13/cobra"

	"github.com/scttfrdmn/cargoship/pkg/archivefs"
	"github.com/scttfrdmn/cargoship/pkg/manifest"
)

// NewDVCCmd returns the 'dvc' parent command with subcommands for interacting
// with DVC stage metadata embedded in CargoShip manifests (v0.13.0).
func NewDVCCmd() *cobra.Command {
	cmd := &cobra.Command{
		Use:   "dvc",
		Short: "Inspect and export DVC stage metadata from a CargoShip archive",
		Long: `Commands for working with DVC (Data Version Control) stage metadata
embedded in CargoShip manifests.

Use 'cargoship upload --dvc-auto' to annotate uploads with per-file stage
information from dvc.yaml before using these commands.`,
	}

	cmd.AddCommand(newDVCStagesCmd())
	cmd.AddCommand(newDVCStatusCmd())
	cmd.AddCommand(newDVCExportCmd())

	return cmd
}

// ---------------------------------------------------------------------------
// dvc stages
// ---------------------------------------------------------------------------

// newDVCStagesCmd returns the 'dvc stages' subcommand.
func newDVCStagesCmd() *cobra.Command {
	var (
		region     string
		jsonOutput bool
	)

	cmd := &cobra.Command{
		Use:   "stages S3_URL",
		Short: "List DVC pipeline stages and their file counts from a manifest",
		Args:  cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			ctx := context.Background()

			m, err := loadManifestFromS3URL(ctx, args[0], region)
			if err != nil {
				return err
			}

			vfs := archivefs.New(m)
			stages := vfs.Stages()

			if jsonOutput {
				return json.NewEncoder(cmd.OutOrStdout()).Encode(stages)
			}

			if len(stages) == 0 {
				_, _ = fmt.Fprintln(cmd.OutOrStdout(), "(no DVC stage metadata)")
				return nil
			}

			// Sort stage names for deterministic output.
			names := make([]string, 0, len(stages))
			for n := range stages {
				names = append(names, n)
			}
			sort.Strings(names)

			for _, name := range names {
				_, _ = fmt.Fprintf(cmd.OutOrStdout(), "  %-20s %d file(s)\n", name, stages[name])
			}
			return nil
		},
	}

	cmd.Flags().StringVar(&region, "region", "us-east-1", "AWS region")
	cmd.Flags().BoolVar(&jsonOutput, "json", false, "Output as JSON")
	return cmd
}

// ---------------------------------------------------------------------------
// dvc status
// ---------------------------------------------------------------------------

// newDVCStatusCmd returns the 'dvc status' subcommand.
func newDVCStatusCmd() *cobra.Command {
	var (
		region      string
		stageFilter string
		jsonOutput  bool
	)

	cmd := &cobra.Command{
		Use:   "status LOCAL_PATH S3_URL",
		Short: "Compare local files against a manifest (unchanged / modified / missing)",
		Args:  cobra.ExactArgs(2),
		RunE: func(cmd *cobra.Command, args []string) error {
			ctx := context.Background()
			localPath := args[0]
			s3URL := args[1]

			absLocal, err := filepath.Abs(localPath)
			if err != nil {
				return fmt.Errorf("resolve local path: %w", err)
			}

			m, err := loadManifestFromS3URL(ctx, s3URL, region)
			if err != nil {
				return err
			}

			q := manifest.NewManifestQuery(m)
			files := q.ListFiles("")
			if stageFilter != "" {
				files = filterByStage(files, stageFilter)
				if len(files) == 0 {
					return fmt.Errorf("no files found for DVC stage %q", stageFilter)
				}
			}

			type fileStatus struct {
				Path   string `json:"path"`
				Status string `json:"status"`
			}
			results := make([]fileStatus, 0, len(files))

			for _, fe := range files {
				localFile := filepath.Join(absLocal, filepath.FromSlash(fe.Path))
				status := computeFileStatus(localFile, fe.ContentHash)
				results = append(results, fileStatus{Path: fe.Path, Status: status})
			}

			if jsonOutput {
				return json.NewEncoder(cmd.OutOrStdout()).Encode(results)
			}

			for _, r := range results {
				_, _ = fmt.Fprintf(cmd.OutOrStdout(), "  %-12s %s\n", r.Status, r.Path)
			}
			return nil
		},
	}

	cmd.Flags().StringVar(&region, "region", "us-east-1", "AWS region")
	cmd.Flags().StringVar(&stageFilter, "stage", "", "Filter to files from a specific DVC stage")
	cmd.Flags().BoolVar(&jsonOutput, "json", false, "Output as JSON")
	return cmd
}

// ---------------------------------------------------------------------------
// dvc export
// ---------------------------------------------------------------------------

// newDVCExportCmd returns the 'dvc export' subcommand.
func newDVCExportCmd() *cobra.Command {
	var (
		region    string
		cacheDir  string
		outputDir string
	)

	cmd := &cobra.Command{
		Use:   "export S3_URL [OUTPUT_DIR]",
		Short: "Download a manifest and generate .dvc sidecar files",
		Args:  cobra.RangeArgs(1, 2),
		RunE: func(cmd *cobra.Command, args []string) error {
			ctx := context.Background()

			out := outputDir
			if len(args) == 2 {
				out = args[1]
			}
			if out == "" {
				out = "dvc-files"
			}

			m, err := loadManifestFromS3URL(ctx, args[0], region)
			if err != nil {
				return err
			}

			opts := &manifest.DVCGenerateOptions{CacheDir: cacheDir}
			n, err := m.GenerateDVCFiles(out, opts)
			if err != nil {
				return fmt.Errorf("generate DVC files: %w", err)
			}

			_, _ = fmt.Fprintf(cmd.OutOrStdout(), "📄 %d .dvc file(s) written to %s\n", n, out)
			return nil
		},
	}

	cmd.Flags().StringVar(&region, "region", "us-east-1", "AWS region")
	cmd.Flags().StringVar(&cacheDir, "cache-dir", ".dvc/cache", "Local DVC cache directory (recorded in manifest)")
	cmd.Flags().StringVar(&outputDir, "output-dir", "", "Directory to write .dvc files (default: dvc-files)")
	return cmd
}

// ---------------------------------------------------------------------------
// Helpers
// ---------------------------------------------------------------------------

// loadManifestFromS3URL parses an s3://bucket/prefix/uploads/<id> URL, creates
// an S3 client, and downloads the manifest.
func loadManifestFromS3URL(ctx context.Context, s3URL, region string) (*manifest.Manifest, error) {
	bucket, prefix, err := parseS3URL(s3URL)
	if err != nil {
		return nil, fmt.Errorf("invalid S3 URL: %w", err)
	}

	cfg, err := awsconfig.LoadDefaultConfig(ctx, awsconfig.WithRegion(region))
	if err != nil {
		return nil, fmt.Errorf("load AWS config: %w", err)
	}
	s3Client := s3.NewFromConfig(cfg)

	var actualPrefix, uploadID string
	if idx := strings.Index(prefix, "/uploads/"); idx != -1 {
		actualPrefix = prefix[:idx]
		uploadID = prefix[idx+9:]
	} else {
		uploadID = prefix
	}

	m, err := manifest.DownloadFromS3(ctx, s3Client, bucket, actualPrefix, uploadID)
	if err != nil {
		return nil, fmt.Errorf("download manifest: %w", err)
	}
	return m, nil
}

// filterByStage returns only files whose DVCMetadata.Stage matches stage.
func filterByStage(files []manifest.FileEntry, stage string) []manifest.FileEntry {
	out := make([]manifest.FileEntry, 0, len(files))
	for _, f := range files {
		if f.DVCMetadata != nil && f.DVCMetadata.Stage == stage {
			out = append(out, f)
		}
	}
	return out
}

// computeFileStatus returns "unchanged", "modified", or "missing" for the
// local file at localPath compared to the manifest's ContentHash.
func computeFileStatus(localPath, contentHash string) string {
	if _, err := os.Stat(localPath); err != nil {
		return "missing"
	}
	if contentHash == "" {
		// No hash in manifest — can only confirm presence.
		return "unchanged"
	}
	h, err := manifest.ComputeContentHash(localPath)
	if err != nil {
		return "missing"
	}
	if h == contentHash {
		return "unchanged"
	}
	return "modified"
}
