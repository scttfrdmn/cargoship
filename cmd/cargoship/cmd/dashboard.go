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

Passing an S3 target adds two bucket-backed views:
- 🗂️  Inventory: completed uploads, read from the manifests under the prefix
- 🔎 Analyze:   an on-demand bucket cost/savings scan (press 'a'; not automatic)

Local data comes from the cost ledger (see 'cargoship cost') and upload state
(see 'cargoship resume'). To browse and restore archived data, use
'cargoship browse'.

Navigation:
  Tab / ← →   Switch view      1-5   Jump to a view
  ↑ ↓         Move in a table   a     Analyze (with an S3 target)
  R           Refresh now       Q / Ctrl+C   Quit`,
		Example: `  cargoship dashboard
  cargoship dashboard --view costs
  cargoship dashboard s3://my-bucket/backups   # adds Inventory + Analyze`,
		Args: cobra.MaximumNArgs(1),
		RunE: runDashboard,
	}

	cmd.Flags().String("view", "overview", "Initial view (overview, costs, uploads, inventory, analyze)")
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

	// #449: an optional S3 target adds the Inventory + Analyze views.
	var inv tui.InventoryProvider
	var an tui.AnalyzeProvider
	var target string
	if len(args) == 1 {
		i, a, t, err := s3Providers(ctx, cmd, args[0])
		if err != nil {
			return err
		}
		inv, an, target = i, a, t
	}

	if err := tui.NewDashboard(ctx, costProvider, inv, an, target, initial, refresh, logger).Run(); err != nil {
		return fmt.Errorf("dashboard failed: %w", err)
	}
	return nil
}

// s3Providers builds the Inventory + Analyze providers for an S3 target, resolving
// region + credentials the same way 'cargoship analyze' does.
func s3Providers(ctx context.Context, cmd *cobra.Command, url string) (tui.InventoryProvider, tui.AnalyzeProvider, string, error) {
	bucket, prefix, err := s3pkg.ParseS3URL(url)
	if err != nil {
		return nil, nil, "", fmt.Errorf("invalid S3 target %q: %w", url, err)
	}
	profile, _ := cmd.Flags().GetString("profile")
	region, _ := cmd.Flags().GetString("region")

	awsCfg, err := loadAWSConfig(ctx, profile, region)
	if err != nil {
		return nil, nil, "", fmt.Errorf("failed to load AWS config: %w", err)
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
		target, nil
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
	out := make([]tui.ManifestSummary, 0, len(ms))
	for _, m := range ms {
		out = append(out, tui.ManifestSummary{
			UploadID:    m.UploadID,
			Source:      m.SourcePath,
			Destination: fmt.Sprintf("s3://%s/%s", m.Bucket, m.Prefix),
			Files:       m.TotalFiles,
			Bytes:       m.TotalBytes,
			Created:     m.CreatedAt,
			Completed:   !m.CompletedAt.IsZero(),
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
