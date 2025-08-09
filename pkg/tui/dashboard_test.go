package tui

import (
	"log/slog"
	"os"
	"testing"
	"time"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestNewDashboard(t *testing.T) {
	logger := slog.New(slog.NewTextHandler(os.Stdout, &slog.HandlerOptions{Level: slog.LevelError}))

	testCases := []struct {
		name          string
		dashboardType DashboardType
	}{
		{"Overview Dashboard", DashboardOverview},
		{"Archival Dashboard", DashboardArchival},
		{"Inventory Dashboard", DashboardInventory},
		{"Costs Dashboard", DashboardCosts},
		{"Agents Dashboard", DashboardAgents},
		{"Config Dashboard", DashboardConfig},
		{"Logs Dashboard", DashboardLogs},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			dashboard := NewDashboard(tc.dashboardType, logger)
			require.NotNil(t, dashboard)
			
			assert.Equal(t, tc.dashboardType, dashboard.dashboardType)
			assert.Equal(t, DashboardOverview, dashboard.currentView) // Always starts at Overview
			assert.NotNil(t, dashboard.logger)
			assert.NotNil(t, dashboard.navigationTabs)
			assert.GreaterOrEqual(t, dashboard.selectedTab, 0)
			
			// Verify navigation tabs are set
			assert.NotEmpty(t, dashboard.navigationTabs)
			
			// Verify tables are initialized
			assert.NotNil(t, dashboard.agentTable)
			assert.NotNil(t, dashboard.jobsTable)
			assert.NotNil(t, dashboard.archivalQueue)
		})
	}
}

func TestDashboardTypes(t *testing.T) {
	// Test dashboard type constants
	assert.Equal(t, DashboardType(0), DashboardOverview)
	assert.Equal(t, DashboardType(1), DashboardArchival)
	assert.Equal(t, DashboardType(2), DashboardInventory)
	assert.Equal(t, DashboardType(3), DashboardCosts)
	assert.Equal(t, DashboardType(4), DashboardAgents)
	assert.Equal(t, DashboardType(5), DashboardConfig)
	assert.Equal(t, DashboardType(6), DashboardLogs)
}

func TestDashboardDataStructures(t *testing.T) {
	t.Run("AgentInfo structure", func(t *testing.T) {
		agent := AgentInfo{
			ID:         "agent-1",
			Name:       "Test Agent",
			Status:     "active",
			Endpoint:   "http://agent1:8080",
			Jobs:       2,
			Throughput: "100 MB/s",
			LastSeen:   time.Now(),
			Progress:   75.5,
		}
		
		assert.Equal(t, "agent-1", agent.ID)
		assert.Equal(t, "Test Agent", agent.Name)
		assert.Equal(t, "active", agent.Status)
		assert.Equal(t, "http://agent1:8080", agent.Endpoint)
		assert.Equal(t, 2, agent.Jobs)
		assert.Equal(t, "100 MB/s", agent.Throughput)
		assert.Equal(t, 75.5, agent.Progress)
	})

	t.Run("JobInfo structure", func(t *testing.T) {
		job := JobInfo{
			ID:        "job-1",
			AgentID:   "agent-1",
			Type:      "archive",
			Path:      "/data/test",
			Status:    "running",
			Progress:  50.5,
			StartTime: time.Now(),
			Size:      "1GB",
			Rate:      "10 MB/s",
		}
		
		assert.Equal(t, "job-1", job.ID)
		assert.Equal(t, "agent-1", job.AgentID)
		assert.Equal(t, "archive", job.Type)
		assert.Equal(t, "/data/test", job.Path)
		assert.Equal(t, "running", job.Status)
		assert.Equal(t, 50.5, job.Progress)
		assert.Equal(t, "1GB", job.Size)
		assert.Equal(t, "10 MB/s", job.Rate)
	})

	t.Run("SystemMetrics structure", func(t *testing.T) {
		metrics := SystemMetrics{
			TotalAgents:     5,
			ActiveJobs:      2,
			CompletedJobs:   100,
			FailedJobs:      1,
			TotalThroughput: "500 MB/s",
			Uptime:          time.Hour * 24 * 30, // 30 days
			MemoryUsage:     "8GB",
			CPUUsage:        75.5,
			StorageUsed:     "10TB",
			MonthlySpend:    "$1500.75",
			ProjectedCost:   "$1800.00",
		}
		
		assert.Equal(t, 5, metrics.TotalAgents)
		assert.Equal(t, 2, metrics.ActiveJobs)
		assert.Equal(t, int64(100), metrics.CompletedJobs)
		assert.Equal(t, int64(1), metrics.FailedJobs)
		assert.Equal(t, "500 MB/s", metrics.TotalThroughput)
		assert.Equal(t, "8GB", metrics.MemoryUsage)
		assert.Equal(t, 75.5, metrics.CPUUsage)
		assert.Equal(t, "$1500.75", metrics.MonthlySpend)
	})

	t.Run("ArchivalJob structure", func(t *testing.T) {
		job := ArchivalJob{
			ID:            "arch-1",
			Source:        "/data/genomics",
			Destination:   "s3://genomics-archive/",
			Status:        "queued",
			Progress:      0.0,
			StartTime:     time.Now(),
			EstimatedCost: "$15.50",
			StorageClass:  "DEEP_ARCHIVE",
			Size:          "500MB",
			Rate:          "10 MB/s",
		}
		
		assert.Equal(t, "arch-1", job.ID)
		assert.Equal(t, "/data/genomics", job.Source)
		assert.Equal(t, "s3://genomics-archive/", job.Destination)
		assert.Equal(t, "DEEP_ARCHIVE", job.StorageClass)
		assert.Equal(t, "queued", job.Status)
		assert.Equal(t, "$15.50", job.EstimatedCost)
		assert.Equal(t, "500MB", job.Size)
	})
}

func TestDashboardTickCmd(t *testing.T) {
	logger := slog.New(slog.NewTextHandler(os.Stdout, &slog.HandlerOptions{Level: slog.LevelError}))
	dashboard := NewDashboard(DashboardOverview, logger)

	cmd := dashboard.tickCmd()
	assert.NotNil(t, cmd)
	
	// Execute the command to get a message
	msg := cmd()
	assert.NotNil(t, msg)
	
	// Should return a tickMsg
	_, ok := msg.(tickMsg)
	assert.True(t, ok, "Command should return a tickMsg")
}

func TestDashboardFetchDataCmd(t *testing.T) {
	logger := slog.New(slog.NewTextHandler(os.Stdout, &slog.HandlerOptions{Level: slog.LevelError}))
	dashboard := NewDashboard(DashboardOverview, logger)

	cmd := dashboard.fetchDataCmd()
	assert.NotNil(t, cmd)
	
	// Execute the command to get a message
	msg := cmd()
	assert.NotNil(t, msg)
	
	// Should return a dataUpdateMsg
	dataMsg, ok := msg.(dataUpdateMsg)
	assert.True(t, ok, "Command should return a dataUpdateMsg")
	assert.Equal(t, "data_refresh", dataMsg.Type)
}

func TestDashboardUpdateData(t *testing.T) {
	logger := slog.New(slog.NewTextHandler(os.Stdout, &slog.HandlerOptions{Level: slog.LevelError}))
	dashboard := NewDashboard(DashboardOverview, logger)

	// Create test data
	testAgents := []AgentInfo{
		{ID: "agent-1", Name: "Agent 1", Status: "active"},
		{ID: "agent-2", Name: "Agent 2", Status: "idle"},
	}
	
	testJobs := []JobInfo{
		{ID: "job-1", Type: "archive", Status: "running"},
		{ID: "job-2", Type: "restore", Status: "queued"},
	}

	testMetrics := SystemMetrics{
		TotalAgents:     2,
		ActiveJobs:      1,
		CompletedJobs:   10,
		FailedJobs:      1,
		TotalThroughput: "100 MB/s",
		MemoryUsage:     "4GB",
		CPUUsage:        50.0,
	}

	// Test update
	updateMsg := dataUpdateMsg{
		Type:    "test_update",
		agents:  testAgents,
		jobs:    testJobs,
		metrics: testMetrics,
	}

	dashboard.updateData(updateMsg)

	// Verify data was updated (the function modifies internal state)
	assert.NotNil(t, dashboard.agents)
	assert.NotNil(t, dashboard.jobs)
	assert.NotNil(t, dashboard.metrics)
}

func TestDashboardBubbleTeaInterface(t *testing.T) {
	logger := slog.New(slog.NewTextHandler(os.Stdout, &slog.HandlerOptions{Level: slog.LevelError}))
	dashboard := NewDashboard(DashboardOverview, logger)

	// Test that Dashboard implements tea.Model interface
	var model tea.Model = dashboard
	assert.NotNil(t, model)

	// Test Init method
	initCmd := dashboard.Init()
	assert.NotNil(t, initCmd)

	// Test Update method with tick message
	tickMessage := tickMsg(time.Now())
	updatedModel, cmd := dashboard.Update(tickMessage)
	assert.NotNil(t, updatedModel)
	assert.NotNil(t, cmd)
	
	// Verify it returns the same dashboard
	updatedDashboard, ok := updatedModel.(*Dashboard)
	assert.True(t, ok)
	assert.Equal(t, dashboard, updatedDashboard)

	// Test View method
	view := dashboard.View()
	assert.NotEmpty(t, view)
	assert.IsType(t, "", view) // Should return a string
}

func TestDataUpdateMsg(t *testing.T) {
	msg := dataUpdateMsg{
		Type:            "test",
		agents:          []AgentInfo{{ID: "test-agent"}},
		jobs:            []JobInfo{{ID: "test-job"}},
		archivalJobs:    []ArchivalJob{{ID: "test-archival"}},
		inventoryItems:  []InventoryItem{{Path: "test-item", Type: "file"}},
		configurations:  []ConfigItem{{Key: "test-config"}},
	}

	assert.Equal(t, "test", msg.Type)
	assert.Len(t, msg.agents, 1)
	assert.Len(t, msg.jobs, 1)
	assert.Len(t, msg.archivalJobs, 1)
	assert.Len(t, msg.inventoryItems, 1)
	assert.Len(t, msg.configurations, 1)
}

func TestInventoryItemStructure(t *testing.T) {
	item := InventoryItem{
		Path:         "/data/genomics/dataset-1",
		Type:         "directory",
		Size:         "5GB",
		LastModified: time.Now(),
		StorageClass: "GLACIER",
		Cost:         "$25.50/month",
		Metadata:     map[string]string{"project": "genomics"},
	}

	assert.Equal(t, "/data/genomics/dataset-1", item.Path)
	assert.Equal(t, "directory", item.Type)
	assert.Equal(t, "5GB", item.Size)
	assert.Equal(t, "GLACIER", item.StorageClass)
	assert.Equal(t, "$25.50/month", item.Cost)
	assert.NotNil(t, item.Metadata)
	assert.Equal(t, "genomics", item.Metadata["project"])
}

func TestCostStructures(t *testing.T) {
	t.Run("CostAnalysis structure", func(t *testing.T) {
		analysis := CostAnalysis{
			CurrentSpend:   "$1500.75",
			ProjectedSpend: "$1800.90",
			Optimizations:  []CostOptimization{},
			Breakdown:      []CostBreakdownItem{},
			Budget:         BudgetInfo{Monthly: "$2000.00"},
		}

		assert.Equal(t, "$1500.75", analysis.CurrentSpend)
		assert.Equal(t, "$1800.90", analysis.ProjectedSpend)
		assert.NotNil(t, analysis.Optimizations)
		assert.NotNil(t, analysis.Breakdown)
		assert.Equal(t, "$2000.00", analysis.Budget.Monthly)
	})

	t.Run("CostOptimization structure", func(t *testing.T) {
		optimization := CostOptimization{
			Type:            "storage_class",
			Description:     "Move old data to Deep Archive",
			PotentialSaving: "$150.25",
			Impact:          "High",
			Effort:          "Low",
		}

		assert.Equal(t, "storage_class", optimization.Type)
		assert.Equal(t, "Move old data to Deep Archive", optimization.Description)
		assert.Equal(t, "$150.25", optimization.PotentialSaving)
		assert.Equal(t, "High", optimization.Impact)
		assert.Equal(t, "Low", optimization.Effort)
	})

	t.Run("BudgetInfo structure", func(t *testing.T) {
		budget := BudgetInfo{
			Monthly:   "$2000.00",
			Used:      "$1500.75",
			Remaining: "$499.25",
			DaysLeft:  15,
		}

		assert.Equal(t, "$2000.00", budget.Monthly)
		assert.Equal(t, "$1500.75", budget.Used)
		assert.Equal(t, "$499.25", budget.Remaining)
		assert.Equal(t, 15, budget.DaysLeft)
	})
}

func TestConfigItemStructure(t *testing.T) {
	config := ConfigItem{
		Key:         "aws.region",
		Value:       "us-west-2",
		Type:        "string",
		Description: "AWS region for operations",
		Default:     "us-east-1",
		Source:      "config.yaml",
	}

	assert.Equal(t, "aws.region", config.Key)
	assert.Equal(t, "us-west-2", config.Value)
	assert.Equal(t, "string", config.Type)
	assert.Equal(t, "AWS region for operations", config.Description)
	assert.Equal(t, "us-east-1", config.Default)
	assert.Equal(t, "config.yaml", config.Source)
}