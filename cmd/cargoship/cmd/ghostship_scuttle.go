package cmd

import (
	"context"
	"fmt"
	"strings"

	"github.com/aws/aws-sdk-go-v2/aws"
	"github.com/aws/aws-sdk-go-v2/service/iam"
	"github.com/aws/aws-sdk-go-v2/service/s3"
	"github.com/spf13/cobra"

	"github.com/scttfrdmn/cargoship/pkg/fleet"
	"github.com/scttfrdmn/cargoship/pkg/pipeline"
)

func newGhostshipScuttleCmd() *cobra.Command {
	var (
		region    string
		profile   string
		purgeData string
		yes       bool
	)
	cmd := &cobra.Command{
		Use:   "scuttle WRITER_ID",
		Short: "Revoke a minted fleet-writer IAM identity (#613)",
		Long: `Decommission one fleet writer by deleting the IAM identity that
'ghostship init --mint' created for it: its access keys, the attached write-only
policy, and the user. Idempotent — a writer that is already gone is a no-op, and
only the cargoship-managed policy is deleted (any other attached policy is detached
but left intact).

With --purge-data it ALSO deletes that writer's S3 data (<base>/writers/<id>/) and
config (<base>/fleet/<id>/) under the given fleet base — irreversible, so it requires
--yes. The base is required, so one fleet's purge can't reach another's data. Data an
agent wrote from a signed config, at writers/<id>/ in the bucket root, is shared by
every fleet with that writer name; it is reported, not deleted.

This is the fleet-writer counterpart to 'cargoship scuttle' (which wipes bucket data).

Examples:
  cargoship ghostship scuttle lab-nas-1
  cargoship ghostship scuttle lab-nas-1 --purge-data s3://backups/nas --yes`,
		Args: cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			id, err := pipeline.SanitizeWriterID(args[0])
			if err != nil {
				return fmt.Errorf("invalid writer id: %w", err)
			}
			// Fail fast on the irreversible-delete confirmation, before any AWS call.
			if purgeData != "" && !yes {
				return fmt.Errorf("--purge-data deletes writer %q's backups irreversibly; pass --yes to confirm", id)
			}
			var purgeBucket, purgeBase string
			if purgeData != "" {
				// #920: the purge must be scoped to ONE fleet. Without a base it
				// targeted writers/<id>/ at the bucket root, which every fleet in the
				// bucket with a writer of that name shares, so purging fleet A's
				// writer deleted fleet B's backups.
				b, base, perr := parseS3URL(purgeData)
				if perr != nil {
					return fmt.Errorf("invalid --purge-data URL: %w", perr)
				}
				if strings.Trim(base, "/") == "" {
					return fmt.Errorf("--purge-data needs the fleet base, s3://BUCKET/BASE (the same base given to 'ghostship init'), " +
						"so only this fleet's objects are deleted")
				}
				purgeBucket, purgeBase = b, strings.Trim(base, "/")
			}
			awsCfg, err := loadAWSConfig(cmd.Context(), profile, region)
			if err != nil {
				return fmt.Errorf("failed to load AWS config: %w", err)
			}

			if err := fleet.ScuttleWriterIAM(cmd.Context(), iam.NewFromConfig(awsCfg), id); err != nil {
				return fmt.Errorf("revoke IAM identity: %w", err)
			}
			_, _ = fmt.Fprintf(cmd.ErrOrStderr(), "Revoked IAM identity %s.\n", fleet.WriterIdentityName(id))

			if purgeData != "" {
				s3Client := s3.NewFromConfig(awsCfg)
				total := 0
				for _, pfx := range fleetPurgePrefixes(purgeBase, id) {
					n, derr := purgeS3Prefix(cmd.Context(), s3Client, purgeBucket, pfx)
					if derr != nil {
						return fmt.Errorf("purge %s: %w", pfx, derr)
					}
					total += n
				}
				_, _ = fmt.Fprintf(cmd.ErrOrStderr(), "Purged %d object(s) for writer %q under s3://%s/%s (data + config).\n",
					total, id, purgeBucket, purgeBase)

				// Agents started from a signed config write to writers/<id>/ at the
				// bucket ROOT, which isn't attributable to one fleet. Say so rather
				// than delete it or stay silent.
				shared := pipeline.WriterPrefix("", id)
				if n, cerr := countS3Prefix(cmd.Context(), s3Client, purgeBucket, shared); cerr == nil && n != "" {
					_, _ = fmt.Fprintf(cmd.ErrOrStderr(),
						"⚠️  %s object(s) remain under s3://%s/%s/ (written by agents running from a signed config). That path is shared "+
							"by every fleet in this bucket with a writer named %q, so it was NOT deleted. After checking no other fleet "+
							"uses it: cargoship scuttle --bucket %s --prefix %s\n", n, purgeBucket, shared, id, purgeBucket, shared)
				}
			}
			return nil
		},
	}
	cmd.Flags().StringVarP(&region, "region", "r", "us-west-2", "AWS region")
	cmd.Flags().StringVar(&profile, "profile", "", "AWS profile to use")
	cmd.Flags().StringVar(&purgeData, "purge-data", "", "Also delete this writer's S3 data + config under s3://BUCKET/BASE (irreversible; needs --yes)")
	cmd.Flags().BoolVar(&yes, "yes", false, "Confirm the irreversible --purge-data deletion")
	return cmd
}

// purgeS3Prefix lists and deletes every object under bucket/<prefix>/ and returns the
// count deleted.
func purgeS3Prefix(ctx context.Context, client *s3.Client, bucket, prefix string) (int, error) {
	listPrefix := strings.TrimSuffix(prefix, "/") + "/"
	var keys []string
	pager := s3.NewListObjectsV2Paginator(client, &s3.ListObjectsV2Input{
		Bucket: aws.String(bucket),
		Prefix: aws.String(listPrefix),
	})
	for pager.HasMorePages() {
		page, err := pager.NextPage(ctx)
		if err != nil {
			return 0, fmt.Errorf("list %s: %w", listPrefix, err)
		}
		for _, o := range page.Contents {
			keys = append(keys, aws.ToString(o.Key))
		}
	}
	if len(keys) == 0 {
		return 0, nil
	}
	return deleteS3Objects(ctx, client, bucket, keys)
}

// fleetPurgePrefixes are the prefixes a fleet writer's purge deletes: its data
// and its config under the fleet base (#920).
func fleetPurgePrefixes(base, id string) []string {
	return []string{pipeline.WriterPrefix(base, id), fleet.ControlPrefix(base, id)}
}

// countS3Prefix reports how many objects sit under bucket/<prefix>/, as a
// string capped at "1000+" (one listing page).
func countS3Prefix(ctx context.Context, client *s3.Client, bucket, prefix string) (string, error) {
	out, err := client.ListObjectsV2(ctx, &s3.ListObjectsV2Input{
		Bucket: aws.String(bucket),
		Prefix: aws.String(strings.TrimSuffix(prefix, "/") + "/"),
	})
	if err != nil {
		return "", err
	}
	n := aws.ToInt32(out.KeyCount)
	if n == 0 {
		return "", nil
	}
	if aws.ToBool(out.IsTruncated) {
		return "1000+", nil
	}
	return fmt.Sprintf("%d", n), nil
}
