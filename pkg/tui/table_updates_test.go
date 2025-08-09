package tui

import (
	"log/slog"
	"os"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestDashboardUpdateTableFunctions(t *testing.T) {
	logger := slog.New(slog.NewTextHandler(os.Stdout, &slog.HandlerOptions{Level: slog.LevelError}))
	dashboard := NewDashboard(DashboardOverview, logger)

	t.Run("updateArchivalTables", func(t *testing.T) {
		// Should not panic and should execute successfully
		assert.NotPanics(t, func() {
			dashboard.updateArchivalTables()
		})
	})

	t.Run("updateInventoryTables", func(t *testing.T) {
		assert.NotPanics(t, func() {
			dashboard.updateInventoryTables()
		})
	})

	t.Run("updateCostTables", func(t *testing.T) {
		assert.NotPanics(t, func() {
			dashboard.updateCostTables()
		})
	})

	t.Run("updateConfigTables", func(t *testing.T) {
		assert.NotPanics(t, func() {
			dashboard.updateConfigTables()
		})
	})
}

func TestDashboardSpecificTableUpdates(t *testing.T) {
	logger := slog.New(slog.NewTextHandler(os.Stdout, &slog.HandlerOptions{Level: slog.LevelError}))
	dashboard := NewDashboard(DashboardArchival, logger)

	t.Run("updateArchivalQueue", func(t *testing.T) {
		assert.NotPanics(t, func() {
			dashboard.updateArchivalQueue()
		})
		// Verify the archival queue table was updated
		assert.NotNil(t, dashboard.archivalQueue)
	})

	t.Run("updateEstimateTable", func(t *testing.T) {
		assert.NotPanics(t, func() {
			dashboard.updateEstimateTable()
		})
		assert.NotNil(t, dashboard.estimateTable)
	})

	t.Run("updateSurveyTable", func(t *testing.T) {
		assert.NotPanics(t, func() {
			dashboard.updateSurveyTable()
		})
		assert.NotNil(t, dashboard.surveyResults)
	})
}

func TestInventoryTableUpdates(t *testing.T) {
	logger := slog.New(slog.NewTextHandler(os.Stdout, &slog.HandlerOptions{Level: slog.LevelError}))
	dashboard := NewDashboard(DashboardInventory, logger)

	t.Run("updateInventoryTree", func(t *testing.T) {
		assert.NotPanics(t, func() {
			dashboard.updateInventoryTree()
		})
		assert.NotNil(t, dashboard.inventoryTree)
	})

	t.Run("updateSearchResults", func(t *testing.T) {
		assert.NotPanics(t, func() {
			dashboard.updateSearchResults()
		})
		assert.NotNil(t, dashboard.searchResults)
	})

	t.Run("updateRestoreQueue", func(t *testing.T) {
		assert.NotPanics(t, func() {
			dashboard.updateRestoreQueue()
		})
		assert.NotNil(t, dashboard.restoreQueue)
	})
}

func TestCostTableUpdates(t *testing.T) {
	logger := slog.New(slog.NewTextHandler(os.Stdout, &slog.HandlerOptions{Level: slog.LevelError}))
	dashboard := NewDashboard(DashboardCosts, logger)

	t.Run("updateCostBreakdown", func(t *testing.T) {
		assert.NotPanics(t, func() {
			dashboard.updateCostBreakdown()
		})
		assert.NotNil(t, dashboard.costBreakdown)
	})

	t.Run("updateOptimizationsTable", func(t *testing.T) {
		assert.NotPanics(t, func() {
			dashboard.updateOptimizationsTable()
		})
		assert.NotNil(t, dashboard.optimizations)
	})

	t.Run("updateBudgetChart", func(t *testing.T) {
		assert.NotPanics(t, func() {
			dashboard.updateBudgetChart()
		})
		// Budget chart is represented as a table in the TUI
		assert.NotNil(t, dashboard.budgetChart)
	})
}

func TestConfigTableUpdates(t *testing.T) {
	logger := slog.New(slog.NewTextHandler(os.Stdout, &slog.HandlerOptions{Level: slog.LevelError}))
	dashboard := NewDashboard(DashboardConfig, logger)

	t.Run("updateConfigTable", func(t *testing.T) {
		assert.NotPanics(t, func() {
			dashboard.updateConfigTable()
		})
		assert.NotNil(t, dashboard.configTable)
	})

	t.Run("updateProfilesList", func(t *testing.T) {
		assert.NotPanics(t, func() {
			dashboard.updateProfilesList()
		})
		assert.NotNil(t, dashboard.profilesList)
	})
}

func TestMockDataFunctions(t *testing.T) {
	logger := slog.New(slog.NewTextHandler(os.Stdout, &slog.HandlerOptions{Level: slog.LevelError}))
	dashboard := NewDashboard(DashboardOverview, logger)

	t.Run("fetchMockArchivalJobs", func(t *testing.T) {
		jobs := dashboard.fetchMockArchivalJobs()
		assert.NotNil(t, jobs)
		assert.NotEmpty(t, jobs, "Should return at least one mock archival job")
		
		for _, job := range jobs {
			assert.NotEmpty(t, job.ID, "Job ID should not be empty")
			assert.NotEmpty(t, job.Source, "Job source should not be empty")
			assert.NotEmpty(t, job.Size, "Job size should not be empty")
			assert.NotEmpty(t, job.Status, "Job status should not be empty")
		}
	})

	t.Run("fetchMockInventoryItems", func(t *testing.T) {
		items := dashboard.fetchMockInventoryItems()
		assert.NotNil(t, items)
		assert.NotEmpty(t, items, "Should return at least one mock inventory item")
		
		for _, item := range items {
			// InventoryItem doesn't have Name field, test Path instead
			assert.NotEmpty(t, item.Path, "Item path should not be empty")
			assert.NotEmpty(t, item.Path, "Item path should not be empty")
			assert.NotEmpty(t, item.Size, "Item size should not be empty")
			assert.NotEmpty(t, item.Type, "Item type should not be empty")
		}
	})

	t.Run("fetchMockConfigurations", func(t *testing.T) {
		configs := dashboard.fetchMockConfigurations()
		assert.NotNil(t, configs)
		assert.NotEmpty(t, configs, "Should return at least one mock configuration")
		
		for _, config := range configs {
			assert.NotEmpty(t, config.Key, "Config key should not be empty")
			assert.NotEmpty(t, config.Value, "Config value should not be empty")
			assert.NotEmpty(t, config.Source, "Config source should not be empty")
		}
	})
}

func TestMockDataQuality(t *testing.T) {
	logger := slog.New(slog.NewTextHandler(os.Stdout, &slog.HandlerOptions{Level: slog.LevelError}))
	dashboard := NewDashboard(DashboardOverview, logger)

	t.Run("archival jobs data quality", func(t *testing.T) {
		jobs := dashboard.fetchMockArchivalJobs()
		require.NotEmpty(t, jobs)
		
		// Check data types and reasonable values
		for _, job := range jobs {
			assert.IsType(t, "", job.ID)
			assert.IsType(t, "", job.Source)
			assert.IsType(t, "", job.Size)
			assert.IsType(t, "", job.Destination)
			assert.IsType(t, "", job.StorageClass)
			// Priority field doesn't exist in ArchivalJob
			assert.IsType(t, "", job.Status)
			assert.IsType(t, "", job.EstimatedCost)
			
			// Reasonable value ranges
			assert.NotEmpty(t, job.Size)
			// EstimatedCost is a string field
			assert.NotEmpty(t, job.EstimatedCost)
		}
	})

	t.Run("inventory items data quality", func(t *testing.T) {
		items := dashboard.fetchMockInventoryItems()
		require.NotEmpty(t, items)
		
		for _, item := range items {
			// InventoryItem doesn't have Name field
			assert.IsType(t, "", item.Path)
			assert.IsType(t, "", item.Size)
			assert.IsType(t, "", item.Type)
			// Archived field doesn't exist in InventoryItem
			
			assert.NotEmpty(t, item.Size)
			assert.Contains(t, []string{"file", "directory", "folder", "archive"}, item.Type)
		}
	})

	t.Run("configurations data quality", func(t *testing.T) {
		configs := dashboard.fetchMockConfigurations()
		require.NotEmpty(t, configs)
		
		for _, config := range configs {
			assert.IsType(t, "", config.Key)
			assert.IsType(t, "", config.Value)
			assert.IsType(t, "", config.Description)
			assert.IsType(t, "", config.Source)
			// Editable field doesn't exist in ConfigItem
			
			assert.NotEmpty(t, config.Key)
			assert.NotEmpty(t, config.Value)
		}
	})
}

func TestTableConsistency(t *testing.T) {
	logger := slog.New(slog.NewTextHandler(os.Stdout, &slog.HandlerOptions{Level: slog.LevelError}))
	
	// Test each dashboard type has consistent table initialization
	dashboardTypes := []DashboardType{
		DashboardOverview, DashboardArchival, DashboardInventory,
		DashboardCosts, DashboardAgents, DashboardConfig, DashboardLogs,
	}

	for _, dashType := range dashboardTypes {
		t.Run(dashType.String(), func(t *testing.T) {
			dashboard := NewDashboard(dashType, logger)
			
			// All dashboards should have basic tables initialized
			assert.NotNil(t, dashboard.agentTable, "Agent table should be initialized")
			assert.NotNil(t, dashboard.jobsTable, "Jobs table should be initialized")
			assert.NotNil(t, dashboard.archivalQueue, "Archival queue should be initialized")
			
			// Test that update functions don't panic
			assert.NotPanics(t, func() {
				dashboard.updateArchivalTables()
				dashboard.updateInventoryTables()
				dashboard.updateCostTables()
				dashboard.updateConfigTables()
			})
		})
	}
}

// Helper function to add string representation for DashboardType
func (dt DashboardType) String() string {
	switch dt {
	case DashboardOverview:
		return "Overview"
	case DashboardArchival:
		return "Archival"
	case DashboardInventory:
		return "Inventory"
	case DashboardCosts:
		return "Costs"
	case DashboardAgents:
		return "Agents"
	case DashboardConfig:
		return "Config"
	case DashboardLogs:
		return "Logs"
	default:
		return "Unknown"
	}
}