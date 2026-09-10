package cmd

import (
	"context"
	"fmt"
	"log/slog"

	"github.com/spf13/cobra"

	"github.com/scttfrdmn/cargoship/pkg/aws/cost"
	"github.com/scttfrdmn/cargoship/pkg/tui"
)

// NewDashboardCmd creates the TUI dashboard command.
func NewDashboardCmd() *cobra.Command {
	cmd := &cobra.Command{
		Use:   "dashboard",
		Short: "Launch the CargoShip TUI dashboard",
		Long: `Launch the CargoShip terminal dashboard — a read-only view over your real
local data. It fabricates nothing; when a source has no data it says so.

Views:
- 🏠 Overview: this month's recorded spend, budget used, in-progress uploads
- 💰 Costs:    recorded spend this month by storage class, plus budget status
- 📦 Uploads:  in-progress / resumable uploads and their progress

Data comes from the local cost ledger (see 'cargoship cost') and local upload
state (see 'cargoship resume'); the dashboard makes no live bucket scans. To
browse and restore archived data interactively, use 'cargoship browse'.

Navigation:
  Tab / ← →   Switch view      1-3   Jump to a view
  ↑ ↓         Move in a table   R     Refresh now      Q / Ctrl+C   Quit`,
		Example: `  cargoship dashboard
  cargoship dashboard --view costs
  cargoship dashboard --refresh 10s`,
		RunE: runDashboard,
	}

	cmd.Flags().String("view", "overview", "Initial view (overview, costs, uploads)")
	cmd.Flags().Duration("refresh", 0, "Data refresh interval (e.g. 5s, 1m; default 5s)")

	return cmd
}

// runDashboard starts the TUI dashboard.
func runDashboard(cmd *cobra.Command, _ []string) error {
	ctx := context.Background()
	logger := slog.Default()

	viewFlag, _ := cmd.Flags().GetString("view")
	var initial tui.DashboardType
	switch viewFlag {
	case "costs", "cost":
		initial = tui.DashboardCosts
	case "uploads", "upload":
		initial = tui.DashboardUploads
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

	if err := tui.NewDashboard(ctx, costProvider, initial, refresh, logger).Run(); err != nil {
		return fmt.Errorf("dashboard failed: %w", err)
	}
	return nil
}
