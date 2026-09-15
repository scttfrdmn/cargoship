package cmd

import (
	"fmt"
	"os"
	"strings"

	"github.com/spf13/cobra"
	"gopkg.in/yaml.v3"

	"github.com/scttfrdmn/cargoship/pkg/fleet"
	"github.com/scttfrdmn/cargoship/pkg/launch"
	"github.com/scttfrdmn/cargoship/pkg/pipeline"
)

// NewGhostshipCmd creates the 'ghostship' command group for running CargoShip as
// a fleet of unattended, writer-isolated backup agents (#604).
func NewGhostshipCmd() *cobra.Command {
	cmd := &cobra.Command{
		Use:   "ghostship",
		Short: "Fleet mode: run CargoShip as unattended, writer-isolated backup agents",
		Long: `Ghostship groups the commands for running CargoShip as a fleet of unattended
backup agents that share one S3 bucket without colliding (writer isolation, #520).

Subcommands:
  iam-policy        Emit the least-privilege IAM policy for one writer
  validate-config   Check a ghostship config (and optionally its scope) before deploy`,
	}
	cmd.AddCommand(newGhostshipIAMPolicyCmd(), newGhostshipValidateConfigCmd())
	return cmd
}

func newGhostshipIAMPolicyCmd() *cobra.Command {
	var (
		writerID  string
		kmsKeyARN string
		output    string
	)
	cmd := &cobra.Command{
		Use:   "iam-policy S3_URL",
		Short: "Emit a least-privilege, delete-free IAM policy for a fleet writer",
		Long: `Emit the least-privilege AWS IAM policy for one ghostship writer backing up to
s3://BUCKET/PREFIX. Attach the emitted policy to the IAM user or role that your
writer runs as — a ghostship agent, or a cron'd 'cargoship sync --writer-id'.

The policy grants only what a writer needs: PutObject/GetObject/AbortMultipartUpload
scoped to writers/<id>/, plus ListBucket gated to that prefix. It is deliberately
delete-free and cannot reach another writer's data — a compromised writer can only
append to its own subtree, never delete or overwrite backups. With --kms-key-arn it
also grants kms:GenerateDataKey and kms:Decrypt on that one key (AWS requires both to
multipart-upload SSE-KMS objects).

Examples:
  cargoship ghostship iam-policy s3://my-bucket/backups --writer-id lab-nas-1
  cargoship ghostship iam-policy s3://my-bucket/backups --writer-id auto \
    --kms-key-arn arn:aws:kms:us-west-2:123456789012:key/abcd-1234 -o policy.json`,
		Args: cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			bucket, prefix, err := parseS3URL(args[0])
			if err != nil {
				return fmt.Errorf("invalid S3 URL: %w", err)
			}
			resolvedWriterID, err := pipeline.ResolveWriterID(writerID)
			if err != nil {
				return fmt.Errorf("invalid --writer-id: %w", err)
			}
			eff := pipeline.WriterPrefix(prefix, resolvedWriterID)

			policy, err := fleet.WriterIAMPolicy(bucket, eff, kmsKeyARN)
			if err != nil {
				return err
			}

			if output != "" {
				if err := os.WriteFile(output, []byte(policy+"\n"), 0o644); err != nil {
					return fmt.Errorf("write policy file: %w", err)
				}
				_, _ = fmt.Fprintf(cmd.ErrOrStderr(), "Wrote IAM policy to %s\n", output)
			} else {
				_, _ = fmt.Fprintln(cmd.OutOrStdout(), policy)
			}

			who := resolvedWriterID
			if who == "" {
				who = "(single-writer / base prefix)"
			}
			_, _ = fmt.Fprintf(cmd.ErrOrStderr(),
				"\nAttach this policy to the IAM user/role writer %q runs as. It is delete-free and scoped to writers/<id>/ — a compromised writer can only append to its own prefix.\n", who)
			return nil
		},
	}
	cmd.Flags().StringVar(&writerID, "writer-id", "", "Writer identity to scope the policy to (writers/<id>/). 'auto' derives a stable per-host id; empty scopes to the base prefix")
	cmd.Flags().StringVar(&kmsKeyARN, "kms-key-arn", "", "KMS key ARN for encrypted uploads; adds kms:GenerateDataKey and kms:Decrypt scoped to that key")
	cmd.Flags().StringVarP(&output, "output", "o", "", "Write the policy JSON to this file instead of stdout")
	return cmd
}

func newGhostshipValidateConfigCmd() *cobra.Command {
	var (
		baseline string
		strict   bool
	)
	cmd := &cobra.Command{
		Use:   "validate-config CONFIG_FILE",
		Short: "Validate a ghostship config, and optionally check it doesn't widen scope",
		Long: `Validate a ghostship config file before deploying it. Reports errors (an
invalid config) and warnings (dangerous-but-permitted combinations, e.g.
delete_after_archive with a broad matcher).

With --baseline, also reports how CONFIG_FILE widens what a writer reads or
deletes relative to the currently-deployed config (new/broadened watch paths,
recursive flips, added includes, removed excludes, newly-enabled source deletion)
— the check the fleet's config-over-S3 pull will enforce.

Read-only: no AWS calls, no network.

Examples:
  cargoship ghostship validate-config ghost_ship.yaml
  cargoship ghostship validate-config new.yaml --baseline deployed.yaml --strict`,
		Args: cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			cfg, err := loadGhostshipConfigFile(args[0])
			if err != nil {
				return err
			}
			out := cmd.OutOrStdout()

			var errCount, warnCount int
			for _, is := range launch.ValidateConfig(cfg) {
				if is.Severity == launch.SeverityError {
					errCount++
				} else {
					warnCount++
				}
				_, _ = fmt.Fprintf(out, "%s: %s: %s\n", strings.ToUpper(string(is.Severity)), is.Field, is.Message)
			}

			var widenings []string
			if baseline != "" {
				base, err := loadGhostshipConfigFile(baseline)
				if err != nil {
					return fmt.Errorf("baseline: %w", err)
				}
				widenings = launch.WatchScopeWidenings(base, cfg)
				for _, wd := range widenings {
					_, _ = fmt.Fprintf(out, "WIDENS-SCOPE: %s\n", wd)
				}
			}

			if errCount == 0 && warnCount == 0 && len(widenings) == 0 {
				_, _ = fmt.Fprintln(out, "OK: config is valid")
			}
			if errCount > 0 {
				return fmt.Errorf("%d validation error(s)", errCount)
			}
			if strict && (warnCount > 0 || len(widenings) > 0) {
				return fmt.Errorf("--strict: %d warning(s), %d scope-widening(s)", warnCount, len(widenings))
			}
			return nil
		},
	}
	cmd.Flags().StringVar(&baseline, "baseline", "", "Path to the currently-deployed config; report how CONFIG_FILE widens watch scope vs it")
	cmd.Flags().BoolVar(&strict, "strict", false, "Treat warnings and scope-widenings as failures (non-zero exit)")
	return cmd
}

// loadGhostshipConfigFile reads and YAML-decodes a ghostship config file.
func loadGhostshipConfigFile(path string) (*launch.GhostShipConfig, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return nil, fmt.Errorf("read config %s: %w", path, err)
	}
	var cfg launch.GhostShipConfig
	if err := yaml.Unmarshal(data, &cfg); err != nil {
		return nil, fmt.Errorf("parse config %s: %w", path, err)
	}
	return &cfg, nil
}
