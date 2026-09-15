package cmd

import (
	"fmt"
	"os"

	"github.com/spf13/cobra"

	"github.com/scttfrdmn/cargoship/pkg/fleet"
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
  iam-policy   Emit the least-privilege IAM policy for one writer`,
	}
	cmd.AddCommand(newGhostshipIAMPolicyCmd())
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
