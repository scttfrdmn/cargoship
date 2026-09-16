package cmd

import (
	"encoding/json"
	"fmt"
	"sort"
	"text/tabwriter"
	"time"

	"github.com/aws/aws-sdk-go-v2/service/s3"
	"github.com/spf13/cobra"

	s3pkg "github.com/scttfrdmn/cargoship/pkg/aws/s3"
	"github.com/scttfrdmn/cargoship/pkg/fleet"
)

// NewFleetCmd is the control-side view over a ghostship fleet (#615). Agents write
// heartbeats (writers/<id>/status.json) on every backup cycle; these commands read
// them back. Read-only — it needs bucket read + list, never the agent's identity.
func NewFleetCmd() *cobra.Command {
	cmd := &cobra.Command{
		Use:   "fleet",
		Short: "Observe a ghostship fleet from the control side (read-only)",
		Long: `Inspect a fleet of unattended ghostship backup agents by reading the
heartbeats they write under writers/<id>/status.json.

These commands run on your control machine and only read the bucket — they do not
need any agent's write identity. See 'cargoship ghostship' for the agent side.`,
	}
	cmd.AddCommand(newFleetStatusCmd())
	return cmd
}

func newFleetStatusCmd() *cobra.Command {
	var asJSON bool
	cmd := &cobra.Command{
		Use:   "status S3_URL",
		Short: "Show the last-known status of every writer in a fleet",
		Long: `List every writer that has reported under the given fleet prefix, with how
long ago it last checked in, whether its most recent cycle was healthy, and its
per-source counts.

S3_URL is the fleet's base prefix (the same destination the agents back up to, not a
writers/<id>/ sub-prefix). Writers with no readable heartbeat are omitted.

Examples:
  cargoship fleet status s3://backups/nas
  cargoship fleet status s3://backups/nas --json`,
		Args: cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			bucket, prefix, err := s3pkg.ParseS3URL(args[0])
			if err != nil {
				return fmt.Errorf("invalid S3 target %q: %w", args[0], err)
			}
			profile, _ := cmd.Flags().GetString("profile")
			region, _ := cmd.Flags().GetString("region")

			awsCfg, err := loadAWSConfig(cmd.Context(), profile, region)
			if err != nil {
				return fmt.Errorf("failed to load AWS config: %w", err)
			}
			client := s3.NewFromConfig(awsCfg)
			if region == "" {
				if r, rErr := s3pkg.GetBucketRegion(cmd.Context(), client, bucket); rErr == nil {
					awsCfg.Region = r
					client = s3.NewFromConfig(awsCfg)
				}
			}

			statuses, err := fleet.ListWriterStatuses(cmd.Context(), client, bucket, prefix)
			if err != nil {
				return fmt.Errorf("list writer statuses: %w", err)
			}
			// Stable order: writer id.
			sort.Slice(statuses, func(i, j int) bool { return statuses[i].WriterID < statuses[j].WriterID })

			if asJSON {
				enc := json.NewEncoder(cmd.OutOrStdout())
				enc.SetIndent("", "  ")
				return enc.Encode(statuses)
			}
			return renderFleetTable(cmd, args[0], statuses, time.Now())
		},
	}
	cmd.Flags().String("region", "", "AWS region for the S3 target (auto-detected if empty)")
	cmd.Flags().String("profile", "", "AWS profile for the S3 target")
	cmd.Flags().BoolVar(&asJSON, "json", false, "Emit the fleet status as JSON")
	return cmd
}

// renderFleetTable prints a one-row-per-writer table. now is injectable so the age
// column is deterministic in tests.
func renderFleetTable(cmd *cobra.Command, target string, statuses []fleet.WriterStatus, now time.Time) error {
	out := cmd.OutOrStdout()
	if len(statuses) == 0 {
		_, _ = fmt.Fprintf(out, "No writers have reported under %s.\n", target)
		return nil
	}
	tw := tabwriter.NewWriter(out, 0, 2, 2, ' ', 0)
	_, _ = fmt.Fprintln(tw, "WRITER\tHOST\tAGE\tHEALTHY\tSOURCES\tLAST ERROR")
	for _, st := range statuses {
		_, _ = fmt.Fprintf(tw, "%s\t%s\t%s\t%s\t%d\t%s\n",
			orDash(st.WriterID), orDash(st.Hostname), humanizeAge(now.Sub(st.UpdatedAt)),
			healthLabel(st.Healthy()), len(st.Sources), orDash(firstSourceError(st.Sources)))
	}
	if err := tw.Flush(); err != nil {
		return fmt.Errorf("render fleet table: %w", err)
	}
	return nil
}

func healthLabel(ok bool) string {
	if ok {
		return "yes"
	}
	return "NO"
}

func orDash(s string) string {
	if s == "" {
		return "-"
	}
	return s
}

// firstSourceError returns the first non-OK source's error, for the summary column.
func firstSourceError(sources []fleet.SourceStatus) string {
	for _, s := range sources {
		if !s.OK && s.LastError != "" {
			return s.LastError
		}
	}
	return ""
}

// humanizeAge renders a heartbeat age compactly (2m, 3h, 5d). A negative age (clock
// skew: the writer's clock is ahead) collapses to "0s" rather than a confusing sign.
func humanizeAge(d time.Duration) string {
	if d < 0 {
		return "0s"
	}
	switch {
	case d < time.Minute:
		return fmt.Sprintf("%ds", int(d.Seconds()))
	case d < time.Hour:
		return fmt.Sprintf("%dm", int(d.Minutes()))
	case d < 24*time.Hour:
		return fmt.Sprintf("%dh", int(d.Hours()))
	default:
		return fmt.Sprintf("%dd", int(d.Hours()/24))
	}
}
