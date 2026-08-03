package cmd

import (
	"fmt"
	"log/slog"

	"github.com/spf13/cobra"

	"github.com/scttfrdmn/cargoship/pkg/tui"
)

// NewDashboardCmd creates the TUI dashboard command
func NewDashboardCmd() *cobra.Command {
	cmd := &cobra.Command{
		Use:   "dashboard",
		Short: "Launch comprehensive CargoShip TUI dashboard",
		Long: `Launch the comprehensive CargoShip terminal user interface dashboard.

The TUI provides a full-featured interface with multiple views:
- 🏠 Overview: System status, storage usage, and quick stats
- 📦 Archive: Data archival operations, cost estimates, and survey results
- 📋 Inventory: Browse archived data, search, and restore operations
- 💰 Costs: Cost analysis, optimization suggestions, and budget tracking
- 🤖 Agents: Launch agent management and monitoring (when available)
- ⚙️ Config: Configuration management and profiles
- 📝 Logs: System logs and monitoring (coming soon)

Navigation:
  Tab/→ ←       - Switch between dashboard views
  1-7           - Quick jump to specific view
  ↑↓            - Focus different sections within view
  Enter         - Select/activate focused item
  R             - Force refresh data
  Q/Ctrl+C      - Exit dashboard

The dashboard adapts based on your current context and available features.`,
		Example: `  # Launch dashboard with overview
  cargoship dashboard

  # Launch with specific view
  cargoship dashboard --view archive
  cargoship dashboard --view costs

  # Launch with agent context
  cargoship --context=agent dashboard`,
		RunE: runDashboard,
	}

	// Dashboard-specific flags
	cmd.Flags().String("view", "overview", "Initial dashboard view (overview, archive, inventory, costs, agents, config, logs)")
	cmd.Flags().Duration("refresh", 0, "Override refresh interval (e.g., 5s, 1m)")
	cmd.Flags().Bool("mock-data", false, "Use mock data for testing (development only)")

	return cmd
}

// runDashboard starts the TUI dashboard
func runDashboard(cmd *cobra.Command, args []string) error {
	logger := slog.Default()

	// Get view preference from flag
	viewFlag, _ := cmd.Flags().GetString("view")

	// Parse dashboard type from view argument
	var dashboardType tui.DashboardType
	switch viewFlag {
	case "overview":
		dashboardType = tui.DashboardOverview
	case "archive", "archival":
		dashboardType = tui.DashboardArchival
	case "inventory":
		dashboardType = tui.DashboardInventory
	case "costs", "cost":
		dashboardType = tui.DashboardCosts
	case "agents", "agent":
		dashboardType = tui.DashboardAgents
	case "config", "configuration":
		dashboardType = tui.DashboardConfig
	case "logs", "log":
		dashboardType = tui.DashboardLogs
	default:
		// Default to overview
		dashboardType = tui.DashboardOverview
	}

	logger.Info("Starting TUI dashboard",
		"view", viewFlag,
		"dashboard_type", dashboardType)

	// Create and run dashboard
	dashboard := tui.NewDashboard(dashboardType, logger)

	// Handle refresh interval flag
	if refreshFlag, _ := cmd.Flags().GetDuration("refresh"); refreshFlag > 0 {
		// TODO: Set custom refresh interval
		logger.Info("Using custom refresh interval", "interval", refreshFlag)
	}

	// Handle mock data flag
	if mockData, _ := cmd.Flags().GetBool("mock-data"); mockData {
		logger.Info("Using mock data for dashboard")
		// TODO: Enable mock data mode
	}

	fmt.Printf("🚢 Starting CargoShip Dashboard (%s view)\n", viewFlag)
	fmt.Println("Use Tab/← → to navigate, 1-7 for quick switching, Enter to select, R to refresh, Q to quit")
	fmt.Println()

	if err := dashboard.Run(); err != nil {
		return fmt.Errorf("dashboard failed: %w", err)
	}

	logger.Info("TUI dashboard stopped")
	fmt.Println("Dashboard closed. Thanks for using CargoShip!")

	return nil
}

// getDashboardType maps execution context to dashboard type
// TODO: Re-enable when implementing context-aware dashboard selection
/*
func getDashboardType(ctx contextpkg.ExecutionContext) tui.DashboardType {
	switch ctx {
	case contextpkg.ContextLocal:
		return tui.DashboardOverview
	case contextpkg.ContextAgent:
		return tui.DashboardAgents
	case contextpkg.ContextREPL:
		return tui.DashboardOverview
	default:
		return tui.DashboardOverview
	}
}
*/
