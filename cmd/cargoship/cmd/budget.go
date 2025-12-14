package cmd

import (
	"context"
	"encoding/json"
	"fmt"

	"github.com/aws/aws-sdk-go-v2/config"
	"github.com/spf13/cobra"

	cargoconfig "github.com/scttfrdmn/cargoship/pkg/aws/config"
	"github.com/scttfrdmn/cargoship/pkg/aws/cost"
)

// NewBudgetCmd creates the 'budget' command for budget management
func NewBudgetCmd() *cobra.Command {
	cmd := &cobra.Command{
		Use:   "budget",
		Short: "Manage project budgets and volume quotas",
		Long: `Manage project-specific budgets and volume quotas.

Project budgets allow you to set cost and volume limits per project (manifest upload).
This enables granular cost control where operations can be blocked if they would exceed
EITHER cost budgets OR volume quotas.

Features:
- Set cost budgets per project (e.g., $1000/month)
- Set volume quotas per project (e.g., 500GB/month)
- Separate alert thresholds for cost vs volume
- Hierarchical enforcement (project limits, then global limits)
- Real-time budget status with burn rate tracking

Examples:
  # Show budget status for a project
  cargoship budget status project1

  # Set project budget (cost only)
  cargoship budget set project1 --cost 1000

  # Set project with both cost and volume limits
  cargoship budget set project1 --cost 1000 --volume 500

  # Set project with custom alert thresholds
  cargoship budget set project1 --cost 1000 --volume 500 --cost-alert 0.85 --volume-alert 0.75

  # List all project budgets
  cargoship budget list

  # Remove project budget
  cargoship budget remove project1
`,
		RunE: func(cmd *cobra.Command, args []string) error {
			// If no subcommand, show help
			return cmd.Help()
		},
	}

	// Add subcommands
	cmd.AddCommand(newBudgetStatusCmd())
	cmd.AddCommand(newBudgetSetCmd())
	cmd.AddCommand(newBudgetListCmd())
	cmd.AddCommand(newBudgetRemoveCmd())

	return cmd
}

func newBudgetStatusCmd() *cobra.Command {
	var jsonOutput bool

	cmd := &cobra.Command{
		Use:   "status <project-id>",
		Short: "Show budget status for a project",
		Long: `Display detailed budget status for a specific project.

Shows:
- Maximum cost budget and current spending
- Maximum volume quota and current usage
- Remaining budget/quota
- Usage percentages
- Daily burn rates
- Projected end-of-period usage
- Alert status

Examples:
  # Show budget status
  cargoship budget status project1

  # Status as JSON
  cargoship budget status project1 --json
`,
		Args: cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			projectID := args[0]
			return runBudgetStatus(cmd.Context(), projectID, jsonOutput)
		},
	}

	cmd.Flags().BoolVar(&jsonOutput, "json", false, "Output as JSON")

	return cmd
}

func newBudgetSetCmd() *cobra.Command {
	var (
		costBudget           float64
		volumeQuota          float64
		costAlertThreshold   float64
		volumeAlertThreshold float64
	)

	cmd := &cobra.Command{
		Use:   "set <project-id>",
		Short: "Set budget and quota for a project",
		Long: `Set cost budget and/or volume quota for a specific project.

Budget values:
- Cost budget in USD (e.g., 1000 = $1000)
- Volume quota in GB (e.g., 500 = 500GB)
- Alert thresholds as percentages (0.0-1.0, e.g., 0.8 = 80%)
- Use 0 for unlimited

Examples:
  # Set cost budget only
  cargoship budget set project1 --cost 1000

  # Set volume quota only
  cargoship budget set project1 --volume 500

  # Set both cost and volume limits
  cargoship budget set project1 --cost 1000 --volume 500

  # Set with custom alert thresholds
  cargoship budget set project1 --cost 1000 --volume 500 \\
    --cost-alert 0.85 --volume-alert 0.75

  # Set unlimited
  cargoship budget set project1 --cost 0 --volume 0
`,
		Args: cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			projectID := args[0]
			return runBudgetSet(cmd.Context(), projectID, costBudget, volumeQuota, costAlertThreshold, volumeAlertThreshold)
		},
	}

	cmd.Flags().Float64Var(&costBudget, "cost", 0, "Maximum cost budget in USD (0 = unlimited)")
	cmd.Flags().Float64Var(&volumeQuota, "volume", 0, "Maximum volume quota in GB (0 = unlimited)")
	cmd.Flags().Float64Var(&costAlertThreshold, "cost-alert", 0.8, "Cost alert threshold (0.0-1.0, default 0.8)")
	cmd.Flags().Float64Var(&volumeAlertThreshold, "volume-alert", 0.75, "Volume alert threshold (0.0-1.0, default 0.75)")

	return cmd
}

func newBudgetListCmd() *cobra.Command {
	var jsonOutput bool

	cmd := &cobra.Command{
		Use:   "list",
		Short: "List all project budgets",
		Long: `List all configured project budgets and their current status.

Shows for each project:
- Project ID
- Cost budget and current spending
- Volume quota and current usage
- Alert status

Examples:
  # List all project budgets
  cargoship budget list

  # List as JSON
  cargoship budget list --json
`,
		RunE: func(cmd *cobra.Command, args []string) error {
			return runBudgetList(cmd.Context(), jsonOutput)
		},
	}

	cmd.Flags().BoolVar(&jsonOutput, "json", false, "Output as JSON")

	return cmd
}

func newBudgetRemoveCmd() *cobra.Command {
	cmd := &cobra.Command{
		Use:   "remove <project-id>",
		Short: "Remove budget for a project",
		Long: `Remove cost budget and volume quota for a specific project.

After removal, the project will use the global budget and quota settings.

Examples:
  # Remove project budget
  cargoship budget remove project1
`,
		Args: cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			projectID := args[0]
			return runBudgetRemove(cmd.Context(), projectID)
		},
	}

	return cmd
}

// Implementation functions

func runBudgetStatus(ctx context.Context, projectID string, jsonOutput bool) error {
	// Load cost manager
	manager, err := loadCostManager(ctx)
	if err != nil {
		return fmt.Errorf("failed to load cost manager: %w", err)
	}

	// Get budget status
	status, err := manager.GetProjectBudgetStatus(projectID)
	if err != nil {
		return fmt.Errorf("failed to get project budget status: %w", err)
	}

	if jsonOutput {
		data, err := json.MarshalIndent(status, "", "  ")
		if err != nil {
			return fmt.Errorf("failed to marshal JSON: %w", err)
		}
		fmt.Println(string(data))
		return nil
	}

	// Format output
	fmt.Printf("Budget Status for Project: %s\n\n", projectID)

	// Cost budget section
	fmt.Println("=== Cost Budget ===")
	if status.MaxBudget > 0 {
		fmt.Printf("  Maximum Budget:    $%.2f\n", status.MaxBudget)
		fmt.Printf("  Current Spending:  $%.2f\n", status.CurrentSpend)
		fmt.Printf("  Remaining:         $%.2f\n", status.BudgetRemaining)
		fmt.Printf("  Usage:             %.1f%%\n", status.BudgetUsed*100)
		fmt.Printf("  Daily Burn Rate:   $%.2f/day\n", status.DailyBurnRate)
		fmt.Printf("  Projected Total:   $%.2f\n", status.ProjectedEOPSpend)

		if status.OverBudget {
			fmt.Println("  Status:            ⚠️  OVER BUDGET")
		} else if status.AlertTriggered {
			fmt.Println("  Status:            ⚠️  ALERT TRIGGERED")
		} else if status.WillExceedBudget {
			fmt.Println("  Status:            ⚠️  WILL EXCEED")
		} else {
			fmt.Println("  Status:            ✅ OK")
		}
	} else {
		fmt.Println("  No cost budget set (unlimited)")
	}

	fmt.Println()

	// Volume quota section
	fmt.Println("=== Volume Quota ===")
	if status.MaxVolumeGB > 0 {
		fmt.Printf("  Maximum Volume:    %.2f GB\n", status.MaxVolumeGB)
		fmt.Printf("  Current Volume:    %.2f GB\n", status.CurrentVolumeGB)
		fmt.Printf("  Remaining:         %.2f GB\n", status.VolumeRemaining)
		fmt.Printf("  Usage:             %.1f%%\n", status.VolumeUsed*100)
		fmt.Printf("  Daily Rate:        %.2f GB/day\n", status.DailyVolumeBurnRate)
		fmt.Printf("  Projected Total:   %.2f GB\n", status.ProjectedEOPVolume)

		if status.OverVolume {
			fmt.Println("  Status:            ⚠️  OVER QUOTA")
		} else if status.VolumeAlertTriggered {
			fmt.Println("  Status:            ⚠️  ALERT TRIGGERED")
		} else if status.WillExceedVolume {
			fmt.Println("  Status:            ⚠️  WILL EXCEED")
		} else {
			fmt.Println("  Status:            ✅ OK")
		}
	} else {
		fmt.Println("  No volume quota set (unlimited)")
	}

	return nil
}

func runBudgetSet(ctx context.Context, projectID string, costBudget, volumeQuota, costAlert, volumeAlert float64) error {
	// Load cost manager
	manager, err := loadCostManager(ctx)
	if err != nil {
		return fmt.Errorf("failed to load cost manager: %w", err)
	}

	// Set project budget
	err = manager.SetProjectBudget(projectID, costBudget, volumeQuota, costAlert, volumeAlert)
	if err != nil {
		return fmt.Errorf("failed to set project budget: %w", err)
	}

	fmt.Printf("✅ Budget set for project: %s\n", projectID)
	if costBudget > 0 {
		fmt.Printf("   Cost Budget:      $%.2f (alert at %.0f%%)\n", costBudget, costAlert*100)
	} else {
		fmt.Println("   Cost Budget:      unlimited")
	}
	if volumeQuota > 0 {
		fmt.Printf("   Volume Quota:     %.2f GB (alert at %.0f%%)\n", volumeQuota, volumeAlert*100)
	} else {
		fmt.Println("   Volume Quota:     unlimited")
	}

	return nil
}

func runBudgetList(ctx context.Context, jsonOutput bool) error {
	// Load cost manager
	manager, err := loadCostManager(ctx)
	if err != nil {
		return fmt.Errorf("failed to load cost manager: %w", err)
	}

	// Get all project budgets
	budgets := manager.ListProjectBudgets()

	if jsonOutput {
		data, err := json.MarshalIndent(budgets, "", "  ")
		if err != nil {
			return fmt.Errorf("failed to marshal JSON: %w", err)
		}
		fmt.Println(string(data))
		return nil
	}

	// Format output
	if len(budgets) == 0 {
		fmt.Println("No project budgets configured")
		return nil
	}

	fmt.Printf("Project Budgets (%d total)\n\n", len(budgets))

	for _, budget := range budgets {
		fmt.Printf("Project: %s\n", budget.ProjectID)
		if budget.MaxBudget > 0 {
			fmt.Printf("  Cost Budget:    $%.2f\n", budget.MaxBudget)
		} else {
			fmt.Println("  Cost Budget:    unlimited")
		}
		if budget.MaxVolumeGB > 0 {
			fmt.Printf("  Volume Quota:   %.2f GB\n", budget.MaxVolumeGB)
		} else {
			fmt.Println("  Volume Quota:   unlimited")
		}
		fmt.Println()
	}

	return nil
}

func runBudgetRemove(ctx context.Context, projectID string) error {
	// Load cost manager
	manager, err := loadCostManager(ctx)
	if err != nil {
		return fmt.Errorf("failed to load cost manager: %w", err)
	}

	// Remove project budget
	err = manager.RemoveProjectBudget(projectID)
	if err != nil {
		return fmt.Errorf("failed to remove project budget: %w", err)
	}

	fmt.Printf("✅ Budget removed for project: %s\n", projectID)
	fmt.Println("   Project will now use global budget and quota settings")

	return nil
}

// Helper function to load cost manager
func loadCostManager(ctx context.Context) (*cost.Manager, error) {
	// Load AWS config
	awsCfg, err := config.LoadDefaultConfig(ctx)
	if err != nil {
		return nil, fmt.Errorf("failed to load AWS config: %w", err)
	}

	// Load CargoShip config (use default for now)
	cargoConfig := cargoconfig.DefaultAWSConfig()

	// Create cost manager
	manager, err := cost.NewManager(&cargoConfig.CostControl, awsCfg, nil)
	if err != nil {
		return nil, fmt.Errorf("failed to create cost manager: %w", err)
	}

	return manager, nil
}
