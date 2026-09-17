package cmd

import (
	"context"
	"fmt"
	"log/slog"

	"github.com/aws/aws-sdk-go-v2/service/s3"
	"github.com/spf13/cobra"

	"github.com/scttfrdmn/cargoship/pkg/aws/cost"
	"github.com/scttfrdmn/cargoship/pkg/aws/costs"
	s3pkg "github.com/scttfrdmn/cargoship/pkg/aws/s3"
	"github.com/scttfrdmn/cargoship/pkg/fleet"
	"github.com/scttfrdmn/cargoship/pkg/manifest"
	"github.com/scttfrdmn/cargoship/pkg/tui"
)

// NewDashboardCmd creates the TUI dashboard command.
func NewDashboardCmd() *cobra.Command {
	cmd := &cobra.Command{
		Use:   "dashboard [s3://bucket[/prefix]]",
		Short: "Launch the CargoShip TUI dashboard",
		Long: `Launch the CargoShip terminal dashboard — a read-only view over your real
data. It fabricates nothing; when a source has no data it says so.

Views (local, always available):
- 🏠 Overview: this month's recorded spend, budget used, in-progress uploads
- 💰 Costs:    recorded spend this month by storage class, plus budget status
- 📦 Uploads:  in-progress / resumable uploads and their progress

Passing an S3 target adds three bucket-backed views:
- 🗂️  Inventory: completed uploads, read from the manifests under the prefix
- 🔎 Analyze:   an on-demand bucket cost/savings scan (press 'a'; not automatic)
- 🚢 Fleet:     ghostship writers reporting under the prefix (heartbeat, #615)

Local data comes from the cost ledger (see 'cargoship cost') and upload state
(see 'cargoship resume'). To browse and restore archived data, use
'cargoship browse'.

Navigation:
  Tab / ← →   Switch view      1-6   Jump to a view
  ↑ ↓         Move in a table   a     Analyze (with an S3 target)
  R           Refresh now       Q / Ctrl+C   Quit`,
		Example: `  cargoship dashboard
  cargoship dashboard --view costs
  cargoship dashboard s3://my-bucket/backups   # adds Inventory + Analyze + Fleet`,
		Args: cobra.MaximumNArgs(1),
		RunE: runDashboard,
	}

	cmd.Flags().String("view", "overview", "Initial view (overview, costs, uploads, inventory, analyze, fleet)")
	cmd.Flags().Duration("refresh", 0, "Data refresh interval (e.g. 5s, 1m; default 5s)")
	cmd.Flags().String("region", "", "AWS region for the S3 target (auto-detected if empty)")
	cmd.Flags().String("profile", "", "AWS profile for the S3 target")

	return cmd
}

// runDashboard starts the TUI dashboard.
func runDashboard(cmd *cobra.Command, args []string) error {
	ctx := context.Background()
	logger := slog.Default()

	viewFlag, _ := cmd.Flags().GetString("view")
	var initial tui.DashboardType
	switch viewFlag {
	case "costs", "cost":
		initial = tui.DashboardCosts
	case "uploads", "upload":
		initial = tui.DashboardUploads
	case "inventory":
		initial = tui.DashboardInventory
	case "analyze":
		initial = tui.DashboardAnalyze
	case "fleet":
		initial = tui.DashboardFleet
	default:
		initial = tui.DashboardOverview
	}
	refresh, _ := cmd.Flags().GetDuration("refresh")

	// Build the cost manager for the real cost/budget views. If it can't be built
	// (e.g. no AWS config), the dashboard still runs and shows the Uploads view;
	// the cost views report the manager as unavailable rather than faking data.
	var costProvider tui.CostProvider
	if awsCfg, cargoConfig, err := loadAWSConfigForCost(ctx, ""); err != nil {
		logger.Warn("cost views unavailable: could not load AWS config", "error", err)
	} else if mgr, err := cost.NewManager(&cargoConfig.CostControl, awsCfg, logger); err != nil {
		logger.Warn("cost views unavailable: could not create cost manager", "error", err)
	} else {
		costProvider = mgr
	}

	// #449/#615: an optional S3 target adds the Inventory + Analyze + Fleet views.
	var inv tui.InventoryProvider
	var an tui.AnalyzeProvider
	var fleetProvider tui.FleetProvider
	var target string
	if len(args) == 1 {
		i, a, f, t, err := s3Providers(ctx, cmd, args[0])
		if err != nil {
			return err
		}
		inv, an, fleetProvider, target = i, a, f, t
	}

	if err := tui.NewDashboard(ctx, costProvider, inv, an, fleetProvider, target, initial, refresh, logger).Run(); err != nil {
		return fmt.Errorf("dashboard failed: %w", err)
	}
	return nil
}

// s3Providers builds the Inventory + Analyze + Fleet providers for an S3 target,
// resolving region + credentials the same way 'cargoship analyze' does.
func s3Providers(ctx context.Context, cmd *cobra.Command, url string) (tui.InventoryProvider, tui.AnalyzeProvider, tui.FleetProvider, string, error) {
	bucket, prefix, err := s3pkg.ParseS3URL(url)
	if err != nil {
		return nil, nil, nil, "", fmt.Errorf("invalid S3 target %q: %w", url, err)
	}
	profile, _ := cmd.Flags().GetString("profile")
	region, _ := cmd.Flags().GetString("region")

	awsCfg, err := loadAWSConfig(ctx, profile, region)
	if err != nil {
		return nil, nil, nil, "", fmt.Errorf("failed to load AWS config: %w", err)
	}
	client := s3.NewFromConfig(awsCfg)
	if region == "" {
		if r, err := s3pkg.GetBucketRegion(ctx, client, bucket); err == nil {
			region = r
			awsCfg.Region = r
			client = s3.NewFromConfig(awsCfg)
		}
	}

	target := fmt.Sprintf("s3://%s/%s", bucket, prefix)
	return dashInventory{client: client, bucket: bucket, prefix: prefix},
		dashAnalyzer{client: client, bucket: bucket, prefix: prefix, region: region},
		dashFleet{client: client, bucket: bucket, prefix: prefix},
		target, nil
}

// dashFleet adapts fleet.ListWriterStatuses to tui.FleetProvider (#615).
type dashFleet struct {
	client         *s3.Client
	bucket, prefix string
}

func (d dashFleet) ListWriters(ctx context.Context) ([]tui.WriterSummary, error) {
	statuses, err := fleet.ListWriterStatuses(ctx, d.client, d.bucket, d.prefix)
	if err != nil {
		return nil, err
	}
	out := make([]tui.WriterSummary, 0, len(statuses))
	for _, st := range statuses {
		out = append(out, tui.WriterSummary{
			WriterID:      st.WriterID,
			Hostname:      st.Hostname,
			Healthy:       st.Healthy(),
			Sources:       len(st.Sources),
			ConfigVersion: st.ConfigVersion,
			ConfigError:   st.ConfigError,
			LastError:     firstSourceError(st.Sources),
			UpdatedAt:     st.UpdatedAt,
		})
	}
	return out, nil
}

// dashInventory adapts manifest.ListAllManifests to tui.InventoryProvider.
type dashInventory struct {
	client         *s3.Client
	bucket, prefix string
}

func (d dashInventory) ListManifests(ctx context.Context) ([]tui.ManifestSummary, error) {
	ms, err := manifest.ListAllManifests(ctx, d.client, d.bucket, d.prefix)
	if err != nil {
		return nil, err
	}
	// #603: surface dataset-versioning identity. DatasetIDOf resolves the chain
	// root for legacy manifests via an in-memory fetcher over the listed set (no
	// extra network calls); a post-#521 manifest returns its recorded DatasetID
	// directly.
	fetch := inMemoryFetch(ms)
	out := make([]tui.ManifestSummary, 0, len(ms))
	for _, m := range ms {
		datasetID, derr := manifest.DatasetIDOf(ctx, m, fetch)
		if derr != nil {
			datasetID = m.UploadID
		}
		out = append(out, tui.ManifestSummary{
			UploadID:    m.UploadID,
			Source:      m.SourcePath,
			Destination: fmt.Sprintf("s3://%s/%s", m.Bucket, m.Prefix),
			Files:       m.TotalFiles,
			Bytes:       m.TotalBytes,
			Created:     m.CreatedAt,
			Completed:   !m.CompletedAt.IsZero(),
			DatasetID:   datasetID,
			Version:     m.VersionOrdinal,
		})
	}
	return out, nil
}

// dashAnalyzer adapts costs.S3Analyzer to tui.AnalyzeProvider.
type dashAnalyzer struct {
	client                 *s3.Client
	bucket, prefix, region string
}

func (d dashAnalyzer) Analyze(ctx context.Context) (*tui.AnalyzeResult, error) {
	scanner := s3pkg.NewBucketScanner(d.client, &s3pkg.BucketScanConfig{
		Bucket: d.bucket, Prefix: d.prefix, Concurrency: 1,
	})
	analyzer := costs.NewS3Analyzer(costs.NewCalculator(d.region), scanner, d.region)
	res, err := analyzer.Analyze(ctx)
	if err != nil {
		return nil, err
	}
	out := &tui.AnalyzeResult{}
	if res.BucketStats != nil {
		out.Objects = res.BucketStats.ObjectCount
		out.Bytes = res.BucketStats.TotalSize
	}
	if res.CurrentCosts != nil {
		out.CurrentMonthly = res.CurrentCosts.TotalMonthlyCost
	}
	if res.ProjectedCosts != nil {
		out.ProjectedMonthly = res.ProjectedCosts.TotalMonthlyCost
	}
	if res.Savings != nil {
		out.Savings = res.Savings.MonthlySavings
	}
	return out, nil
}
