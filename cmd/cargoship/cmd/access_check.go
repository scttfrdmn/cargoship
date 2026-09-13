package cmd

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
	"strings"

	"github.com/aws/aws-sdk-go-v2/service/kms"
	"github.com/aws/aws-sdk-go-v2/service/s3"
	"github.com/aws/aws-sdk-go-v2/service/sts"
	"github.com/spf13/cobra"

	"github.com/scttfrdmn/cargoship/pkg/aws/access"
	s3pkg "github.com/scttfrdmn/cargoship/pkg/aws/s3"
)

// NewAccessCheckCmd creates the 'access-check' command: a read-only report on
// the access-control posture of a target bucket (Issue #529).
func NewAccessCheckCmd() *cobra.Command {
	var (
		bucket   string
		prefix   string
		region   string
		profile  string
		kmsKeyID string
		format   string
	)

	cmd := &cobra.Command{
		Use:   "access-check [s3://bucket/prefix]",
		Short: "Report the access-control posture of a target bucket",
		Long: `Report whether the bucket CargoShip writes to (or reads from) is exposed:
public access, wildcard-principal grants in the bucket policy or ACL, object
ownership, and the scoping of the default SSE-KMS key policy.

This is a read-only posture report — it makes no changes and always exits 0 when
it can reach the bucket (findings are informational). It is deliberately scoped
to user/group access controls, not a general security audit.

Each check degrades gracefully: if the calling principal lacks permission for a
check, that check reports "unknown" with the IAM action it needs, and the rest
of the report still runs. Checks are bucket-level (a prefix is recorded for
context but S3 policy/ACL/BPA/ownership apply to the whole bucket).

Examples:
  # Report on a bucket/prefix
  cargoship access-check s3://my-bucket/archives

  # Using flags, with a specific profile and region
  cargoship access-check --bucket my-bucket --region us-west-2 --profile prod

  # Machine-readable output
  cargoship access-check s3://my-bucket/archives --format json

  # Inspect a specific KMS key's policy instead of the bucket default
  cargoship access-check s3://my-bucket/archives --kms-key-id alias/cargoship

Exit Codes:
  0 - Report produced (regardless of findings)
  1 - Could not reach the bucket (missing/invalid credentials, no such bucket)
  2 - Usage error
`,
		RunE: func(cmd *cobra.Command, args []string) error {
			ctx := context.Background()

			if len(args) > 0 {
				parsedBucket, parsedPrefix, err := s3pkg.ParseS3URL(args[0])
				if err != nil {
					return fmt.Errorf("invalid S3 URL: %w", err)
				}
				if bucket == "" {
					bucket = parsedBucket
				}
				if prefix == "" {
					prefix = parsedPrefix
				}
			}
			if bucket == "" {
				return fmt.Errorf("bucket is required (provide an s3:// URL or --bucket)")
			}
			if format != "table" && format != "json" {
				return fmt.Errorf("unsupported --format %q (want table or json)", format)
			}

			cfg, err := loadAWSConfig(ctx, profile, region)
			if err != nil {
				return fmt.Errorf("failed to load AWS config: %w", err)
			}
			s3Client := s3.NewFromConfig(cfg)

			if region == "" {
				detected, err := s3pkg.GetBucketRegion(ctx, s3Client, bucket)
				if err != nil {
					return fmt.Errorf("could not reach bucket %q (check the name, region, and credentials): %w", bucket, err)
				}
				region = detected
				cfg.Region = region
				s3Client = s3.NewFromConfig(cfg)
			}

			checker := &access.Checker{
				S3:       s3Client,
				STS:      sts.NewFromConfig(cfg),
				KMS:      kms.NewFromConfig(cfg),
				KMSKeyID: kmsKeyID,
			}
			report, err := checker.Check(ctx, bucket, prefix)
			if err != nil {
				return fmt.Errorf("access check failed: %w", err)
			}
			report.Region = region

			if format == "json" {
				enc := json.NewEncoder(os.Stdout)
				enc.SetIndent("", "  ")
				return enc.Encode(report)
			}
			printAccessReport(report)
			return nil
		},
	}

	cmd.Flags().StringVarP(&bucket, "bucket", "b", "", "S3 bucket name")
	cmd.Flags().StringVarP(&prefix, "prefix", "p", "", "S3 prefix (for context; checks are bucket-level)")
	cmd.Flags().StringVarP(&region, "region", "r", "", "AWS region (auto-detected if omitted)")
	cmd.Flags().StringVar(&profile, "profile", "", "AWS profile")
	cmd.Flags().StringVar(&kmsKeyID, "kms-key-id", "", "KMS key ID/ARN/alias to inspect (default: the bucket's SSE-KMS key)")
	cmd.Flags().StringVar(&format, "format", "table", "output format: table or json")

	return cmd
}

// runAccessPreflight builds a checker from an already-constructed S3 client
// (plus STS/KMS clients from a config loaded for the region), runs the posture
// checks against bucket/prefix, prints the report, and enforces failOn. It is
// shared by the standalone `access-check` command and `upload --check-access`.
func runAccessPreflight(ctx context.Context, s3api access.S3PolicyAPI, region, profile, kmsKeyID, bucket, prefix, failOn string) error {
	cfg, err := loadAWSConfig(ctx, profile, region)
	if err != nil {
		return fmt.Errorf("failed to load AWS config for access check: %w", err)
	}
	checker := &access.Checker{
		S3:       s3api,
		STS:      sts.NewFromConfig(cfg),
		KMS:      kms.NewFromConfig(cfg),
		KMSKeyID: kmsKeyID,
	}
	report, err := checker.Check(ctx, bucket, prefix)
	if err != nil {
		return fmt.Errorf("access check failed: %w", err)
	}
	report.Region = region
	printAccessReport(report)
	return enforceAccessFailOn(report, failOn)
}

// enforceAccessFailOn returns a non-nil error when failOn names a severity and
// the report's worst finding is at least that severe. Accepts "none"/"" (never
// fails), "unknown", "warn", "critical".
func enforceAccessFailOn(r *access.Report, failOn string) error {
	var threshold access.Severity
	switch strings.ToLower(strings.TrimSpace(failOn)) {
	case "", "none":
		return nil
	case "unknown":
		threshold = access.SeverityUnknown
	case "warn", "warning":
		threshold = access.SeverityWarn
	case "critical":
		threshold = access.SeverityCritical
	default:
		return fmt.Errorf("invalid --fail-on %q (want none, unknown, warn, or critical)", failOn)
	}
	if worst := r.Worst(); worst.AtLeast(threshold) {
		return fmt.Errorf("access-control posture %q meets --fail-on threshold %q", worst, threshold)
	}
	return nil
}

// printAccessReport renders a posture report as a human-readable table.
func printAccessReport(r *access.Report) {
	fmt.Printf("🔐 Access-control posture — s3://%s", r.Bucket)
	if r.Prefix != "" {
		fmt.Printf("/%s", r.Prefix)
	}
	fmt.Println()
	if r.Region != "" {
		fmt.Printf("   region: %s\n", r.Region)
	}
	if r.CallerARN != "" {
		fmt.Printf("   caller: %s (account %s)\n", r.CallerARN, r.Account)
	}
	fmt.Println()

	for _, f := range r.Findings {
		fmt.Printf("%s  %-22s %s\n", severityIcon(f.Severity), f.Check, f.Summary)
		if f.Detail != "" {
			fmt.Printf("       %s\n", f.Detail)
		}
		if f.Remediation != "" {
			fmt.Printf("       ↳ %s\n", f.Remediation)
		}
	}

	fmt.Printf("\nOverall: %s %s\n", severityIcon(r.Worst()), strings.ToUpper(string(r.Worst())))
}

func severityIcon(s access.Severity) string {
	switch s {
	case access.SeverityCritical:
		return "🛑"
	case access.SeverityWarn:
		return "⚠️ "
	case access.SeverityUnknown:
		return "❓"
	case access.SeverityInfo:
		return "ℹ️ "
	default: // ok
		return "✅"
	}
}
