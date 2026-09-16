package cmd

import (
	"context"
	"encoding/json"
	"fmt"
	"log/slog"
	"os/signal"
	"sort"
	"syscall"
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
	cmd.AddCommand(newFleetMonitorCmd())
	cmd.AddCommand(newFleetLockStatusCmd())
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

func newFleetMonitorCmd() *cobra.Command {
	var (
		threshold time.Duration
		interval  time.Duration
		once      bool
	)
	cmd := &cobra.Command{
		Use:   "monitor S3_URL",
		Short: "Watch a fleet and alert when a writer goes stale (#630)",
		Long: `Periodically read every writer's heartbeat under a fleet prefix and fire a
stale-writer alert for any writer that has not checked in within the freshness
threshold — a writer that is down, stuck, or offline can't report on itself, so this
runs on the control side. Each pass also evaluates budgets, so both fleet-freshness
and budget alerts go out through the same configured channels (see 'cargoship alerts').

Alerts are de-duplicated per writer by the alert cooldown; a writer alerts at most
once per cooldown window. With no alert channel configured, this still logs the fleet
state on every pass. Runs on an interval; --once does a single pass (for cron).

Examples:
  cargoship fleet monitor s3://backups/nas --threshold 2h --interval 15m
  cargoship fleet monitor s3://backups/nas --once`,
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
					region = r
					awsCfg.Region = r
					client = s3.NewFromConfig(awsCfg)
				}
			}

			// The Manager owns the notifier (built from the persisted alert config,
			// #634); alerting is its whole purpose here, so a load failure is fatal.
			mgr, mErr := loadCostManagerWithRegion(cmd.Context(), region)
			if mErr != nil {
				return fmt.Errorf("load cost manager (needed for alerting): %w", mErr)
			}

			logger := slog.New(slog.NewTextHandler(cmd.ErrOrStderr(), nil)).With("component", "fleet-monitor")

			ctx, stop := signal.NotifyContext(cmd.Context(), syscall.SIGINT, syscall.SIGTERM)
			defer stop()

			check := func(ctx context.Context) error {
				statuses, lErr := fleet.ListWriterStatuses(ctx, client, bucket, prefix)
				if lErr != nil {
					logger.Error("failed to list writer statuses", "err", lErr)
					return lErr
				}
				now := time.Now()
				stale := 0
				for _, st := range statuses {
					age := now.Sub(st.UpdatedAt)
					if age > threshold {
						stale++
						logger.Warn("writer stale", "writer_id", st.WriterID, "host", st.Hostname, "age", humanizeAge(age))
						if aErr := mgr.NotifyStaleWriter(ctx, st.WriterID, age, threshold); aErr != nil {
							logger.Error("failed to send stale-writer alert", "writer_id", st.WriterID, "err", aErr)
						}
						continue
					}
					logger.Info("writer ok", "writer_id", st.WriterID, "age", humanizeAge(age), "healthy", st.Healthy())
				}
				logger.Info("fleet check complete", "writers", len(statuses), "stale", stale)
				// Evaluate budgets through the same notifier so one monitor loop covers
				// both fleet-freshness and budget alerting.
				if bErr := mgr.MonitorBudgets(ctx); bErr != nil {
					logger.Error("budget monitoring failed", "err", bErr)
				}
				return nil
			}

			if !once {
				logger.Info("fleet monitor started", "threshold", threshold.String(), "interval", interval.String())
			}
			return runLoop(ctx, interval, once, check)
		},
	}
	cmd.Flags().DurationVar(&threshold, "threshold", 2*time.Hour, "Age since last check-in after which a writer is considered stale")
	cmd.Flags().DurationVar(&interval, "interval", 15*time.Minute, "How often to check the fleet")
	cmd.Flags().BoolVar(&once, "once", false, "Run a single check and exit (for cron / testing)")
	cmd.Flags().String("region", "", "AWS region for the S3 target (auto-detected if empty)")
	cmd.Flags().String("profile", "", "AWS profile for the S3 target")
	return cmd
}

func newFleetLockStatusCmd() *cobra.Command {
	var asJSON bool
	cmd := &cobra.Command{
		Use:   "lock-status S3_URL",
		Short: "Audit a fleet bucket's immutability posture (versioning, Object Lock, lifecycle hygiene)",
		Long: `Report whether a fleet bucket can resist a stolen delete-capable credential and
whether failed-upload hygiene is in place — read-only, it never changes the bucket.

Because the ghostship agent is delete-free by design, the ransomware backstop lives in
the bucket itself: S3 Versioning + Object Lock protect history even if a delete-capable
credential is compromised, and a lifecycle AbortIncompleteMultipartUpload rule cleans up
failed uploads the agent won't. This command checks all three and suggests remediation.

Object Lock, versioning, and lifecycle are bucket-level, so any prefix in the S3 URL is
ignored — the posture is reported for the whole bucket.

Examples:
  cargoship fleet lock-status s3://backups/nas
  cargoship fleet lock-status s3://backups/nas --json`,
		Args: cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			bucket, _, err := s3pkg.ParseS3URL(args[0])
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

			report, err := fleet.AuditBucketImmutability(cmd.Context(), client, bucket)
			if err != nil {
				return fmt.Errorf("audit bucket immutability: %w", err)
			}

			if asJSON {
				enc := json.NewEncoder(cmd.OutOrStdout())
				enc.SetIndent("", "  ")
				return enc.Encode(report)
			}
			renderLockStatus(cmd, report)
			return nil
		},
	}
	cmd.Flags().String("region", "", "AWS region for the S3 target (auto-detected if empty)")
	cmd.Flags().String("profile", "", "AWS profile for the S3 target")
	cmd.Flags().BoolVar(&asJSON, "json", false, "Emit the posture report as JSON")
	return cmd
}

// renderLockStatus prints the posture table, an overall rollup, and remediation for
// any non-OK finding.
func renderLockStatus(cmd *cobra.Command, report fleet.ImmutabilityReport) {
	out := cmd.OutOrStdout()
	tw := tabwriter.NewWriter(out, 0, 2, 2, ' ', 0)
	_, _ = fmt.Fprintln(tw, "CHECK\tSTATUS\tDETAIL")
	for _, f := range report.Findings {
		detail := f.Detail
		if detail == "" {
			detail = f.Summary
		} else {
			detail = f.Summary + " — " + detail
		}
		_, _ = fmt.Fprintf(tw, "%s\t%s\t%s\n", f.Check, f.Severity, detail)
	}
	_ = tw.Flush()
	_, _ = fmt.Fprintf(out, "\nOverall: %s\n", report.Worst())

	var remediations []fleet.ImmutabilityFinding
	for _, f := range report.Findings {
		if f.Severity != fleet.SevOK && f.Remediation != "" {
			remediations = append(remediations, f)
		}
	}
	if len(remediations) > 0 {
		_, _ = fmt.Fprintln(out, "\nRemediation:")
		for _, f := range remediations {
			_, _ = fmt.Fprintf(out, "  %s: %s\n", f.Check, f.Remediation)
		}
	}
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
