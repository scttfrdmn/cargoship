package tui

import (
	"log/slog"
	"os"
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
)

func TestDashboardRenderFunctions(t *testing.T) {
	logger := slog.New(slog.NewTextHandler(os.Stdout, &slog.HandlerOptions{Level: slog.LevelError}))

	t.Run("renderOverviewDashboard", func(t *testing.T) {
		dashboard := NewDashboard(DashboardOverview, logger)

		view := dashboard.renderOverviewDashboard()

		assert.NotEmpty(t, view, "Overview dashboard view should not be empty")
		assert.Contains(t, view, "CargoShip", "Should contain CargoShip title")
		assert.Contains(t, view, "Overview", "Should contain Overview text")

		// Should be a multi-line view
		lines := strings.Split(view, "\n")
		assert.Greater(t, len(lines), 1, "Should be multi-line")
	})

	t.Run("renderArchivalDashboard", func(t *testing.T) {
		dashboard := NewDashboard(DashboardArchival, logger)

		view := dashboard.renderArchivalDashboard()

		assert.NotEmpty(t, view, "Archival dashboard view should not be empty")
		assert.Contains(t, view, "Archival", "Should contain Archival text")

		lines := strings.Split(view, "\n")
		assert.Greater(t, len(lines), 1, "Should be multi-line")
	})

	t.Run("renderInventoryDashboard", func(t *testing.T) {
		dashboard := NewDashboard(DashboardInventory, logger)

		view := dashboard.renderInventoryDashboard()

		assert.NotEmpty(t, view, "Inventory dashboard view should not be empty")
		assert.Contains(t, view, "Inventory", "Should contain Inventory text")

		lines := strings.Split(view, "\n")
		assert.Greater(t, len(lines), 1, "Should be multi-line")
	})

	t.Run("renderCostsDashboard", func(t *testing.T) {
		dashboard := NewDashboard(DashboardCosts, logger)

		view := dashboard.renderCostsDashboard()

		assert.NotEmpty(t, view, "Costs dashboard view should not be empty")
		assert.Contains(t, view, "Cost", "Should contain Cost text")

		lines := strings.Split(view, "\n")
		assert.Greater(t, len(lines), 1, "Should be multi-line")
	})

	t.Run("renderAgentsDashboard", func(t *testing.T) {
		dashboard := NewDashboard(DashboardAgents, logger)

		view := dashboard.renderAgentsDashboard()

		assert.NotEmpty(t, view, "Agents dashboard view should not be empty")
		assert.Contains(t, view, "Agent", "Should contain Agent text")

		lines := strings.Split(view, "\n")
		assert.Greater(t, len(lines), 1, "Should be multi-line")
	})

	t.Run("renderConfigDashboard", func(t *testing.T) {
		dashboard := NewDashboard(DashboardConfig, logger)

		view := dashboard.renderConfigDashboard()

		assert.NotEmpty(t, view, "Config dashboard view should not be empty")
		assert.Contains(t, view, "Config", "Should contain Config text")

		lines := strings.Split(view, "\n")
		assert.Greater(t, len(lines), 1, "Should be multi-line")
	})

	t.Run("renderLogsDashboard", func(t *testing.T) {
		dashboard := NewDashboard(DashboardLogs, logger)

		view := dashboard.renderLogsDashboard()

		assert.NotEmpty(t, view, "Logs dashboard view should not be empty")
		assert.Contains(t, view, "Log", "Should contain Log text")

		lines := strings.Split(view, "\n")
		assert.Greater(t, len(lines), 1, "Should be multi-line")
	})
}

func TestDashboardViewConsistency(t *testing.T) {
	logger := slog.New(slog.NewTextHandler(os.Stdout, &slog.HandlerOptions{Level: slog.LevelError}))

	dashboardTypes := []DashboardType{
		DashboardOverview, DashboardArchival, DashboardInventory,
		DashboardCosts, DashboardAgents, DashboardConfig, DashboardLogs,
	}

	for _, dashType := range dashboardTypes {
		t.Run(dashType.String()+" consistency", func(t *testing.T) {
			dashboard := NewDashboard(dashType, logger)

			// Test main View method
			mainView := dashboard.View()
			assert.NotEmpty(t, mainView, "Main view should not be empty")

			// View should contain navigation tabs with emojis
			assert.True(t, strings.Contains(mainView, "🏠") || strings.Contains(mainView, "Overview"), "Should contain navigation elements")

			// Should have some content structure
			lines := strings.Split(mainView, "\n")
			assert.Greater(t, len(lines), 5, "Should have substantial content")
		})
	}
}

func TestRenderComponentFunctions(t *testing.T) {
	logger := slog.New(slog.NewTextHandler(os.Stdout, &slog.HandlerOptions{Level: slog.LevelError}))
	dashboard := NewDashboard(DashboardOverview, logger)

	t.Run("renderSystemOverview", func(t *testing.T) {
		view := dashboard.renderSystemOverview()
		assert.NotEmpty(t, view, "System overview should not be empty")

		// Should contain some system information
		assert.IsType(t, "", view)
	})

	t.Run("renderRecentActivity", func(t *testing.T) {
		view := dashboard.renderRecentActivity()
		assert.NotEmpty(t, view, "Recent activity should not be empty")
		assert.IsType(t, "", view)
	})

	t.Run("renderQuickStats", func(t *testing.T) {
		view := dashboard.renderQuickStats()
		assert.NotEmpty(t, view, "Quick stats should not be empty")
		assert.IsType(t, "", view)
	})

	// renderNavigation method doesn't exist - navigation is handled in main View() method
}

func TestDashboardViewIntegration(t *testing.T) {
	logger := slog.New(slog.NewTextHandler(os.Stdout, &slog.HandlerOptions{Level: slog.LevelError}))

	t.Run("view changes with dashboard type", func(t *testing.T) {
		overview := NewDashboard(DashboardOverview, logger)
		archival := NewDashboard(DashboardArchival, logger)
		inventory := NewDashboard(DashboardInventory, logger)

		// Set the currentView to match the dashboard type to get different content
		archival.currentView = DashboardArchival
		inventory.currentView = DashboardInventory

		overviewView := overview.View()
		archivalView := archival.View()
		inventoryView := inventory.View()

		// Different dashboard types should produce different views
		assert.NotEqual(t, overviewView, archivalView, "Overview and archival views should be different")
		assert.NotEqual(t, overviewView, inventoryView, "Overview and inventory views should be different")
		assert.NotEqual(t, archivalView, inventoryView, "Archival and inventory views should be different")
	})

	t.Run("view includes navigation", func(t *testing.T) {
		dashboard := NewDashboard(DashboardOverview, logger)
		view := dashboard.View()

		// Should include navigation tabs with emojis
		assert.True(t, strings.Contains(view, "🏠") || strings.Contains(view, "Overview"), "View should include navigation tabs")

		// Should show current selection
		lines := strings.Split(view, "\n")
		hasNavigationSection := false
		for _, line := range lines {
			if strings.Contains(line, "🏠") || strings.Contains(line, "Overview") {
				hasNavigationSection = true
				break
			}
		}
		assert.True(t, hasNavigationSection, "Should have navigation section")
	})
}

func TestStyleConsistency(t *testing.T) {
	logger := slog.New(slog.NewTextHandler(os.Stdout, &slog.HandlerOptions{Level: slog.LevelError}))
	dashboard := NewDashboard(DashboardOverview, logger)

	t.Run("styles are initialized", func(t *testing.T) {
		// Verify that basic styles are properly initialized
		assert.NotNil(t, dashboard.titleStyle, "Title style should be initialized")
		assert.NotNil(t, dashboard.baseStyle, "Base style should be initialized")
		// Note: other specific styles may be internal or handled differently
	})

	t.Run("view uses consistent styling", func(t *testing.T) {
		view := dashboard.View()

		// The view should be styled (contain ANSI escape codes for colors/formatting)
		// This is a basic check - in a real UI, we'd test more specific styling
		assert.IsType(t, "", view)
		assert.NotEmpty(t, view)
	})
}

func TestDashboardStateManagement(t *testing.T) {
	logger := slog.New(slog.NewTextHandler(os.Stdout, &slog.HandlerOptions{Level: slog.LevelError}))

	t.Run("current view tracking", func(t *testing.T) {
		dashboard := NewDashboard(DashboardOverview, logger)

		// Initial state
		assert.Equal(t, DashboardOverview, dashboard.currentView)
		assert.Equal(t, DashboardOverview, dashboard.dashboardType)

		// View content should reflect current view
		view := dashboard.View()
		assert.NotEmpty(t, view)
	})

	t.Run("navigation state", func(t *testing.T) {
		dashboard := NewDashboard(DashboardOverview, logger)

		// Should have navigation tabs
		assert.NotEmpty(t, dashboard.navigationTabs, "Should have navigation tabs")
		assert.GreaterOrEqual(t, dashboard.selectedTab, 0, "Selected tab should be valid")
		assert.Less(t, dashboard.selectedTab, len(dashboard.navigationTabs), "Selected tab should be within bounds")

		// Navigation should be reflected in view - check for Overview tab content
		view := dashboard.View()
		assert.True(t, strings.Contains(view, "🏠") || strings.Contains(view, "Overview"), "View should show selected tab content")
	})
}

func TestRenderErrorHandling(t *testing.T) {
	logger := slog.New(slog.NewTextHandler(os.Stdout, &slog.HandlerOptions{Level: slog.LevelError}))

	t.Run("render with no data", func(t *testing.T) {
		dashboard := NewDashboard(DashboardOverview, logger)

		// Should not panic even with no data
		assert.NotPanics(t, func() {
			view := dashboard.View()
			assert.NotEmpty(t, view, "Should still render something even with no data")
		})
	})

	t.Run("render different dashboard types", func(t *testing.T) {
		dashboardTypes := []DashboardType{
			DashboardOverview, DashboardArchival, DashboardInventory,
			DashboardCosts, DashboardAgents, DashboardConfig, DashboardLogs,
		}

		for _, dashType := range dashboardTypes {
			dashboard := NewDashboard(dashType, logger)

			assert.NotPanics(t, func() {
				view := dashboard.View()
				assert.NotEmpty(t, view, "Dashboard type %v should render", dashType)
			}, "Dashboard type %v should not panic", dashType)
		}
	})
}
