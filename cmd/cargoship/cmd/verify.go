package cmd

import (
	"context"
	"fmt"
	"io"
	"os"

	"github.com/aws/aws-sdk-go-v2/aws"
	"github.com/aws/aws-sdk-go-v2/config"
	"github.com/aws/aws-sdk-go-v2/service/s3"
	"github.com/dustin/go-humanize"
	"github.com/spf13/cobra"

	"github.com/scttfrdmn/cargoship/pkg/manifest"
)

// NewVerifyCmd creates the 'verify' command for dataset integrity verification
func NewVerifyCmd() *cobra.Command {
	var (
		bucket   string
		prefix   string
		uploadID string
		region   string
		quick    bool
		verbose  bool
	)

	cmd := &cobra.Command{
		Use:   "verify [s3://bucket/prefix/uploads/upload-id]",
		Short: "Verify dataset integrity using manifest checksums",
		Long: `Verify the integrity of a CargoShip upload by validating the manifest and checksums.

The verify command performs comprehensive integrity checks:
- Downloads and validates manifest structure
- Verifies manifest consistency (shard counts, file counts, size totals)
- Checks for missing or corrupted metadata
- Validates checksum coverage (if present)

This is useful for:
- Ensuring upload completed successfully
- Detecting corrupted or incomplete uploads
- Validating data integrity before restore
- Compliance and audit requirements

Examples:
  # Verify using S3 URL
  cargoship verify s3://my-bucket/cargoship/uploads/20231208-123456-abcd1234

  # Verify using flags
  cargoship verify --bucket my-bucket --prefix cargoship --upload-id 20231208-123456-abcd1234

  # Quick validation (metadata only, fast)
  cargoship verify s3://my-bucket/cargoship/uploads/20231208-123456-abcd1234 --quick

  # Verbose output with detailed errors
  cargoship verify s3://my-bucket/cargoship/uploads/20231208-123456-abcd1234 --verbose

Exit Codes:
  0 - All checks passed
  1 - Verification failed (errors found)
`,
		RunE: func(cmd *cobra.Command, args []string) error {
			ctx := context.Background()

			// Parse S3 URL from positional argument if provided (Issue #98)
			if len(args) > 0 {
				s3URL := args[0]
				parsedBucket, parsedPath, err := manifest.ParseS3URL(s3URL)
				if err != nil {
					return fmt.Errorf("invalid S3 URL: %w", err)
				}

				// Extract prefix and upload ID from path
				parsedPrefix, parsedUploadID, err := parseUploadPath(parsedPath)
				if err != nil {
					return fmt.Errorf("invalid upload path in S3 URL: %w", err)
				}

				// Override flags with parsed values (only if not already set)
				if bucket == "" {
					bucket = parsedBucket
				}
				if prefix == "" {
					prefix = parsedPrefix
				}
				if uploadID == "" {
					uploadID = parsedUploadID
				}
			}

			// Validate required parameters
			if bucket == "" {
				return fmt.Errorf("bucket is required (provide S3 URL or use --bucket flag)")
			}
			if uploadID == "" {
				return fmt.Errorf("upload-id is required (provide S3 URL or use --upload-id flag)")
			}

			// Load AWS config
			cfg, err := config.LoadDefaultConfig(ctx, config.WithRegion(region))
			if err != nil {
				return fmt.Errorf("failed to load AWS config: %w", err)
			}

			s3Client := s3.NewFromConfig(cfg)

			// Download manifest from S3
			manifestKey := fmt.Sprintf("%s/uploads/%s/manifest.json.gz", prefix, uploadID)
			fmt.Printf("📥 Downloading manifest: s3://%s/%s\n", bucket, manifestKey)

			getObjectInput := &s3.GetObjectInput{
				Bucket: aws.String(bucket),
				Key:    aws.String(manifestKey),
			}

			result, err := s3Client.GetObject(ctx, getObjectInput)
			if err != nil {
				return fmt.Errorf("failed to download manifest from S3: %w", err)
			}
			defer func() {
				if closeErr := result.Body.Close(); closeErr != nil {
					fmt.Fprintf(os.Stderr, "Warning: failed to close S3 response body: %v\n", closeErr)
				}
			}()

			// Read manifest bytes
			manifestBytes, err := io.ReadAll(result.Body)
			if err != nil {
				return fmt.Errorf("failed to read manifest from S3: %w", err)
			}

			// Decompress and deserialize manifest
			m, err := manifest.FromJSONCompressed(manifestBytes)
			if err != nil {
				return fmt.Errorf("failed to deserialize manifest: %w", err)
			}

			fmt.Printf("✅ Manifest downloaded successfully\n\n")

			// Validate manifest using the validation framework (Issue #91)
			fmt.Printf("🔍 Validating manifest integrity...\n")
			validator := manifest.NewValidator(m)

			var validationResult *manifest.ValidationResult
			if quick {
				fmt.Printf("   Mode: Quick validation (metadata only)\n\n")
				validationResult = validator.ValidateQuick()
			} else {
				fmt.Printf("   Mode: Full validation (all checks)\n\n")
				validationResult = validator.Validate()
			}

			// Display validation results
			if validationResult.Valid {
				fmt.Printf("✅ Validation: PASS\n\n")

				// Display summary
				fmt.Printf("📊 Dataset Summary:\n")
				fmt.Printf("   Upload ID:        %s\n", m.UploadID)
				fmt.Printf("   Total Files:      %s files\n", humanize.Comma(m.TotalFiles))
				fmt.Printf("   Total Size:       %s\n", humanize.Bytes(uint64(m.TotalBytes)))
				fmt.Printf("   Total Shards:     %d\n", m.ShardCount)
				fmt.Printf("   Total Chunks:     %d\n", m.TotalChunks)
				fmt.Println()

				// Show warnings if any
				if validationResult.HasWarnings() {
					fmt.Printf("⚠️  Warnings: %d\n", len(validationResult.Warnings))
					if verbose {
						fmt.Println()
						for _, warning := range validationResult.Warnings {
							fmt.Printf("   • %s\n", warning.Message)
						}
						fmt.Println()
					} else {
						fmt.Printf("   💡 Use --verbose to see detailed warnings\n\n")
					}
				}

				// Show validation checks passed
				if verbose {
					fmt.Printf("✅ Checks Passed:\n")
					for check, passed := range validationResult.Checks {
						if passed {
							fmt.Printf("   ✓ %s\n", check)
						}
					}
					fmt.Println()
				}

				fmt.Printf("✅ All %s files verified successfully\n", humanize.Comma(m.TotalFiles))
				return nil
			}

			// Validation failed
			fmt.Printf("❌ Validation: FAIL\n\n")

			// Display errors
			fmt.Printf("❌ Errors: %d\n\n", len(validationResult.Errors))
			for _, err := range validationResult.Errors {
				fmt.Printf("   • %s\n", err.Message)
				if verbose {
					fmt.Printf("     Field:    %s\n", err.Field)
					fmt.Printf("     Expected: %s\n", err.Expected)
					fmt.Printf("     Actual:   %s\n", err.Actual)
					fmt.Println()
				}
			}

			// Display warnings if any
			if validationResult.HasWarnings() {
				fmt.Printf("\n⚠️  Warnings: %d\n\n", len(validationResult.Warnings))
				for _, warning := range validationResult.Warnings {
					fmt.Printf("   • %s\n", warning.Message)
					if verbose {
						fmt.Printf("     Field:    %s\n", warning.Field)
						fmt.Printf("     Expected: %s\n", warning.Expected)
						fmt.Printf("     Actual:   %s\n", warning.Actual)
						fmt.Println()
					}
				}
			}

			// Show which checks failed
			fmt.Printf("\n❌ Failed Checks:\n")
			for check, passed := range validationResult.Checks {
				if !passed {
					fmt.Printf("   ✗ %s\n", check)
				}
			}
			fmt.Println()

			// Exit with error code
			os.Exit(1)
			return nil // Never reached, but required for compilation
		},
	}

	cmd.Flags().StringVarP(&bucket, "bucket", "b", "", "S3 bucket name (or provide S3 URL as argument)")
	cmd.Flags().StringVarP(&prefix, "prefix", "p", "", "S3 prefix for upload (default: empty)")
	cmd.Flags().StringVarP(&uploadID, "upload-id", "u", "", "Upload ID to verify (or provide S3 URL as argument)")
	cmd.Flags().StringVarP(&region, "region", "r", "us-west-2", "AWS region")
	cmd.Flags().BoolVar(&quick, "quick", false, "Quick validation (metadata only, fast)")
	cmd.Flags().BoolVar(&verbose, "verbose", false, "Show detailed error and warning information")

	return cmd
}
