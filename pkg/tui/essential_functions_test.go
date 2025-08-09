package tui

import (
	"log/slog"
	"os"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestTickMsg(t *testing.T) {
	now := time.Now()
	msg := tickMsg(now)

	assert.Equal(t, now, time.Time(msg))
	assert.IsType(t, tickMsg(time.Time{}), msg)
}

// TestDataUpdateMsg removed - duplicate of test in dashboard_test.go

func TestDashboardTickCommand(t *testing.T) {
	logger := slog.New(slog.NewTextHandler(os.Stdout, &slog.HandlerOptions{Level: slog.LevelError}))
	dashboard := NewDashboard(DashboardOverview, logger)

	t.Run("tick command execution", func(t *testing.T) {
		cmd := dashboard.tickCmd()
		require.NotNil(t, cmd)

		// Execute command
		msg := cmd()
		require.NotNil(t, msg)

		// Should return tickMsg
		tickMessage, ok := msg.(tickMsg)
		assert.True(t, ok, "Should return tickMsg type")

		// Time should be recent (within last second)
		msgTime := time.Time(tickMessage)
		assert.True(t, time.Since(msgTime) < time.Second)
	})

	t.Run("tick interval consistency", func(t *testing.T) {
		cmd1 := dashboard.tickCmd()
		msg1 := cmd1()

		// Small delay
		time.Sleep(time.Millisecond * 10)

		cmd2 := dashboard.tickCmd()
		msg2 := cmd2()

		tick1 := time.Time(msg1.(tickMsg))
		tick2 := time.Time(msg2.(tickMsg))

		assert.True(t, tick2.After(tick1) || tick2.Equal(tick1))
	})
}

func TestDashboardFetchDataCommand(t *testing.T) {
	logger := slog.New(slog.NewTextHandler(os.Stdout, &slog.HandlerOptions{Level: slog.LevelError}))
	dashboard := NewDashboard(DashboardOverview, logger)

	t.Run("fetch data command execution", func(t *testing.T) {
		cmd := dashboard.fetchDataCmd()
		require.NotNil(t, cmd)

		// Execute command
		msg := cmd()
		require.NotNil(t, msg)

		// Should return dataUpdateMsg
		dataMsg, ok := msg.(dataUpdateMsg)
		assert.True(t, ok, "Should return dataUpdateMsg type")
		assert.Equal(t, "data_refresh", dataMsg.Type)
	})

	t.Run("fetch data completeness", func(t *testing.T) {
		cmd := dashboard.fetchDataCmd()
		msg := cmd()
		dataMsg := msg.(dataUpdateMsg)

		// Check that all data types are fetched
		assert.NotNil(t, dataMsg.agents, "Agents should be fetched")
		assert.NotNil(t, dataMsg.jobs, "Jobs should be fetched")
		assert.NotNil(t, dataMsg.archivalJobs, "Archival jobs should be fetched")
		assert.NotNil(t, dataMsg.inventoryItems, "Inventory items should be fetched")
		assert.NotNil(t, dataMsg.configurations, "Configurations should be fetched")

		// Data should not be empty for mock data
		assert.NotEmpty(t, dataMsg.agents, "Should have mock agents")
		assert.NotEmpty(t, dataMsg.jobs, "Should have mock jobs")
		assert.NotEmpty(t, dataMsg.archivalJobs, "Should have mock archival jobs")
		assert.NotEmpty(t, dataMsg.inventoryItems, "Should have mock inventory items")
		assert.NotEmpty(t, dataMsg.configurations, "Should have mock configurations")
	})
}

// TestDashboardUpdateData removed - duplicate of test in dashboard_test.go

func TestDashboardMockDataFunctions(t *testing.T) {
	logger := slog.New(slog.NewTextHandler(os.Stdout, &slog.HandlerOptions{Level: slog.LevelError}))
	dashboard := NewDashboard(DashboardOverview, logger)

	t.Run("fetchMockAgents", func(t *testing.T) {
		agents := dashboard.fetchMockAgents()
		require.NotEmpty(t, agents, "Should return mock agents")

		for _, agent := range agents {
			assert.NotEmpty(t, agent.ID, "Agent ID should not be empty")
			assert.NotEmpty(t, agent.Name, "Agent name should not be empty")
			assert.NotEmpty(t, agent.Status, "Agent status should not be empty")
			assert.GreaterOrEqual(t, agent.Jobs, 0, "Jobs should be non-negative")
			assert.NotEmpty(t, agent.Endpoint, "Agent endpoint should not be empty")
		}
	})

	t.Run("fetchMockJobs", func(t *testing.T) {
		jobs := dashboard.fetchMockJobs()
		require.NotEmpty(t, jobs, "Should return mock jobs")

		for _, job := range jobs {
			assert.NotEmpty(t, job.ID, "Job ID should not be empty")
			assert.NotEmpty(t, job.Type, "Job type should not be empty")
			assert.NotEmpty(t, job.Path, "Job path should not be empty")
			assert.NotEmpty(t, job.Status, "Job status should not be empty")
			assert.GreaterOrEqual(t, job.Progress, 0.0, "Progress should be non-negative")
			assert.LessOrEqual(t, job.Progress, 100.0, "Progress should be <= 100")
		}
	})

	t.Run("fetchMockMetrics", func(t *testing.T) {
		metrics := dashboard.fetchMockMetrics()

		assert.GreaterOrEqual(t, metrics.TotalAgents, 0, "Total agents should be non-negative")
		assert.GreaterOrEqual(t, metrics.ActiveJobs, 0, "Active jobs should be non-negative")
		assert.GreaterOrEqual(t, metrics.CompletedJobs, int64(0), "Completed jobs should be non-negative")
		assert.GreaterOrEqual(t, metrics.FailedJobs, int64(0), "Failed jobs should be non-negative")
		assert.NotEmpty(t, metrics.TotalThroughput, "Throughput should be populated")
		assert.NotEmpty(t, metrics.MemoryUsage, "Memory usage should be populated")
		assert.GreaterOrEqual(t, metrics.CPUUsage, 0.0, "CPU usage should be non-negative")
	})
}

func TestDataUpdateMessageHandling(t *testing.T) {
	logger := slog.New(slog.NewTextHandler(os.Stdout, &slog.HandlerOptions{Level: slog.LevelError}))
	dashboard := NewDashboard(DashboardOverview, logger)

	t.Run("handle tick message", func(t *testing.T) {
		tickMessage := tickMsg(time.Now())

		updatedModel, cmd := dashboard.Update(tickMessage)

		assert.NotNil(t, updatedModel)
		assert.NotNil(t, cmd)

		// Should return the same dashboard
		updatedDashboard, ok := updatedModel.(*Dashboard)
		assert.True(t, ok)
		assert.Equal(t, dashboard, updatedDashboard)
	})

	t.Run("handle data update message", func(t *testing.T) {
		dataMsg := dataUpdateMsg{
			Type:   "test",
			agents: []AgentInfo{{ID: "test-agent"}},
		}

		updatedModel, cmd := dashboard.Update(dataMsg)

		assert.NotNil(t, updatedModel)
		// cmd might be nil for data updates
		_ = cmd

		updatedDashboard, ok := updatedModel.(*Dashboard)
		assert.True(t, ok)
		assert.Equal(t, dashboard, updatedDashboard)
	})

	t.Run("handle unknown message", func(t *testing.T) {
		unknownMsg := "unknown message"

		updatedModel, cmd := dashboard.Update(unknownMsg)

		assert.NotNil(t, updatedModel)
		// cmd might be nil for unknown messages
		_ = cmd // Use the variable to avoid "declared and not used" error

		updatedDashboard, ok := updatedModel.(*Dashboard)
		assert.True(t, ok)
		assert.Equal(t, dashboard, updatedDashboard)
	})
}

func TestCommandChaining(t *testing.T) {
	logger := slog.New(slog.NewTextHandler(os.Stdout, &slog.HandlerOptions{Level: slog.LevelError}))
	dashboard := NewDashboard(DashboardOverview, logger)

	t.Run("init command chain", func(t *testing.T) {
		initCmd := dashboard.Init()
		assert.NotNil(t, initCmd)

		// Execute init command
		msg := initCmd()
		assert.NotNil(t, msg)

		// Should be a batch command or specific message type
		// The exact type depends on implementation but should not be nil
	})

	t.Run("update command chain", func(t *testing.T) {
		// Simulate a tick message update
		tickMessage := tickMsg(time.Now())
		updatedModel, cmd := dashboard.Update(tickMessage)

		assert.NotNil(t, updatedModel)
		assert.NotNil(t, cmd)

		// Execute the returned command
		if cmd != nil {
			nextMsg := cmd()
			assert.NotNil(t, nextMsg)
		}
	})
}
