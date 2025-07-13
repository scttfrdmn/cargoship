package tui

import (
	"fmt"
	"strings"

	"github.com/charmbracelet/bubbles/table"
	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"
)

// renderOverviewDashboard renders the main overview dashboard
func (d *Dashboard) renderOverviewDashboard() string {
	title := d.titleStyle.Render("🚢 CargoShip Overview Dashboard")
	
	// System overview metrics
	overview := d.renderSystemOverview()
	
	// Recent activity
	recentActivity := d.renderRecentActivity()
	
	// Quick stats
	quickStats := d.renderQuickStats()
	
	content := lipgloss.JoinVertical(lipgloss.Left,
		title,
		overview,
		quickStats,
		recentActivity,
	)
	
	return content
}

// renderArchivalDashboard renders the archival operations dashboard
func (d *Dashboard) renderArchivalDashboard() string {
	title := d.titleStyle.Render("📦 Data Archival Operations")
	
	// Active archival jobs
	activeJobs := d.baseStyle.Render("Active Archival Jobs\n" + d.archivalQueue.View())
	
	// Cost estimates
	estimates := d.baseStyle.Render("Cost Estimates\n" + d.estimateTable.View())
	
	// Survey results
	surveys := d.baseStyle.Render("Survey Results\n" + d.surveyResults.View())
	
	content := lipgloss.JoinVertical(lipgloss.Left,
		title,
		lipgloss.JoinHorizontal(lipgloss.Top,
			activeJobs,
			lipgloss.JoinVertical(lipgloss.Left, estimates, surveys),
		),
	)
	
	return content
}

// renderInventoryDashboard renders the inventory management dashboard
func (d *Dashboard) renderInventoryDashboard() string {
	title := d.titleStyle.Render("📋 Inventory Management")
	
	// Inventory tree
	inventory := d.baseStyle.Render("Inventory Browser\n" + d.inventoryTree.View())
	
	// Search results
	search := d.baseStyle.Render("Search Results\n" + d.searchResults.View())
	
	// Restore queue
	restore := d.baseStyle.Render("Restore Queue\n" + d.restoreQueue.View())
	
	content := lipgloss.JoinVertical(lipgloss.Left,
		title,
		inventory,
		lipgloss.JoinHorizontal(lipgloss.Top, search, restore),
	)
	
	return content
}

// renderCostsDashboard renders the cost analysis dashboard
func (d *Dashboard) renderCostsDashboard() string {
	title := d.titleStyle.Render("💰 Cost Analysis & Optimization")
	
	// Cost breakdown
	breakdown := d.baseStyle.Render("Cost Breakdown\n" + d.costBreakdown.View())
	
	// Optimization suggestions
	optimizations := d.baseStyle.Render("Optimization Suggestions\n" + d.optimizations.View())
	
	// Budget tracking
	budget := d.baseStyle.Render("Budget Tracking\n" + d.budgetChart.View())
	
	content := lipgloss.JoinVertical(lipgloss.Left,
		title,
		lipgloss.JoinHorizontal(lipgloss.Top, breakdown, budget),
		optimizations,
	)
	
	return content
}

// renderAgentsDashboard renders the agents management dashboard
func (d *Dashboard) renderAgentsDashboard() string {
	title := d.titleStyle.Render("🤖 Launch Agents Management")
	
	// Agent table
	agentTitle := "Connected Agents"
	if d.focused == 0 {
		agentTitle = d.focusedStyle.Render(agentTitle)
	}
	agentSection := lipgloss.JoinVertical(lipgloss.Left,
		agentTitle,
		d.agentTable.View(),
	)
	
	// Jobs table
	jobTitle := "Agent Jobs"
	if d.focused == 1 {
		jobTitle = d.focusedStyle.Render(jobTitle)
	}
	jobSection := lipgloss.JoinVertical(lipgloss.Left,
		jobTitle,
		d.jobsTable.View(),
	)
	
	content := lipgloss.JoinVertical(lipgloss.Left,
		title,
		agentSection,
		jobSection,
	)
	
	return content
}

// renderConfigDashboard renders the configuration dashboard
func (d *Dashboard) renderConfigDashboard() string {
	title := d.titleStyle.Render("⚙️ Configuration Management")
	
	// Configuration table
	config := d.baseStyle.Render("Current Configuration\n" + d.configTable.View())
	
	// Profiles list
	profiles := d.baseStyle.Render("Configuration Profiles\n" + d.profilesList.View())
	
	content := lipgloss.JoinVertical(lipgloss.Left,
		title,
		lipgloss.JoinHorizontal(lipgloss.Top, config, profiles),
	)
	
	return content
}

// renderLogsDashboard renders the logs dashboard
func (d *Dashboard) renderLogsDashboard() string {
	title := d.titleStyle.Render("📝 Logs & Monitoring")
	
	// TODO: Implement log viewer
	logContent := d.baseStyle.Render("Log viewer coming soon...")
	
	content := lipgloss.JoinVertical(lipgloss.Left,
		title,
		logContent,
	)
	
	return content
}

// renderSystemOverview renders system overview metrics
func (d *Dashboard) renderSystemOverview() string {
	metrics := []string{
		fmt.Sprintf("Storage Used: %s", d.metrics.StorageUsed),
		fmt.Sprintf("Monthly Spend: %s", d.metrics.MonthlySpend),
		fmt.Sprintf("Projected Cost: %s", d.metrics.ProjectedCost),
		fmt.Sprintf("Active Agents: %d", d.metrics.TotalAgents),
		fmt.Sprintf("Active Jobs: %d", d.metrics.ActiveJobs),
		fmt.Sprintf("Completed: %d", d.metrics.CompletedJobs),
	}
	
	return d.baseStyle.Render("System Overview\n" + strings.Join(metrics, "\n"))
}

// renderRecentActivity renders recent activity
func (d *Dashboard) renderRecentActivity() string {
	// TODO: Implement recent activity tracking
	return d.baseStyle.Render("Recent Activity\n• No recent activity")
}

// renderQuickStats renders quick statistics
func (d *Dashboard) renderQuickStats() string {
	stats := []string{
		"📊 Data Archived: 1.2TB",
		"💰 Cost Savings: $2,150/month",
		"⚡ Throughput: 150MB/s",
		"🔄 Uptime: 99.9%",
	}
	
	return d.baseStyle.Render("Quick Stats\n" + strings.Join(stats, "\n"))
}

// renderHelpText renders help text
func (d *Dashboard) renderHelpText() string {
	helpStyle := lipgloss.NewStyle().
		Foreground(lipgloss.Color("244")).
		Italic(true)
	
	help := []string{
		"Navigation: Tab/→ ← | 1-7 (quick switch) | ↑↓ (focus) | Enter (select) | R (refresh) | Q (quit)",
	}
	
	return helpStyle.Render(strings.Join(help, " | "))
}

// Table creation functions

func createArchivalTable() table.Model {
	columns := []table.Column{
		{Title: "Job ID", Width: 10},
		{Title: "Source", Width: 25},
		{Title: "Destination", Width: 25},
		{Title: "Status", Width: 12},
		{Title: "Progress", Width: 10},
		{Title: "Cost", Width: 12},
	}
	
	t := table.New(
		table.WithColumns(columns),
		table.WithFocused(true),
		table.WithHeight(10),
	)
	
	return t
}

func createEstimateTable() table.Model {
	columns := []table.Column{
		{Title: "Path", Width: 30},
		{Title: "Size", Width: 12},
		{Title: "Storage Class", Width: 15},
		{Title: "Monthly Cost", Width: 12},
		{Title: "Annual Cost", Width: 12},
	}
	
	t := table.New(
		table.WithColumns(columns),
		table.WithHeight(8),
	)
	
	return t
}

func createSurveyTable() table.Model {
	columns := []table.Column{
		{Title: "Directory", Width: 25},
		{Title: "Files", Width: 8},
		{Title: "Size", Width: 12},
		{Title: "Last Modified", Width: 12},
		{Title: "Recommendation", Width: 15},
	}
	
	t := table.New(
		table.WithColumns(columns),
		table.WithHeight(8),
	)
	
	return t
}

func createInventoryTable() table.Model {
	columns := []table.Column{
		{Title: "Path", Width: 40},
		{Title: "Type", Width: 10},
		{Title: "Size", Width: 12},
		{Title: "Storage Class", Width: 15},
		{Title: "Cost", Width: 12},
	}
	
	t := table.New(
		table.WithColumns(columns),
		table.WithFocused(true),
		table.WithHeight(12),
	)
	
	return t
}

func createSearchTable() table.Model {
	columns := []table.Column{
		{Title: "Match", Width: 30},
		{Title: "Location", Width: 25},
		{Title: "Size", Width: 12},
		{Title: "Modified", Width: 12},
	}
	
	t := table.New(
		table.WithColumns(columns),
		table.WithHeight(8),
	)
	
	return t
}

func createRestoreTable() table.Model {
	columns := []table.Column{
		{Title: "Item", Width: 25},
		{Title: "Status", Width: 12},
		{Title: "Progress", Width: 10},
		{Title: "ETA", Width: 12},
		{Title: "Cost", Width: 12},
	}
	
	t := table.New(
		table.WithColumns(columns),
		table.WithHeight(8),
	)
	
	return t
}

func createCostBreakdownTable() table.Model {
	columns := []table.Column{
		{Title: "Category", Width: 20},
		{Title: "Amount", Width: 12},
		{Title: "Percentage", Width: 10},
		{Title: "Trend", Width: 10},
	}
	
	t := table.New(
		table.WithColumns(columns),
		table.WithHeight(10),
	)
	
	return t
}

func createOptimizationTable() table.Model {
	columns := []table.Column{
		{Title: "Optimization", Width: 25},
		{Title: "Potential Saving", Width: 15},
		{Title: "Impact", Width: 10},
		{Title: "Effort", Width: 10},
	}
	
	t := table.New(
		table.WithColumns(columns),
		table.WithHeight(8),
	)
	
	return t
}

func createBudgetTable() table.Model {
	columns := []table.Column{
		{Title: "Budget Item", Width: 20},
		{Title: "Allocated", Width: 12},
		{Title: "Used", Width: 12},
		{Title: "Remaining", Width: 12},
		{Title: "Days Left", Width: 10},
	}
	
	t := table.New(
		table.WithColumns(columns),
		table.WithHeight(8),
	)
	
	return t
}

func createConfigTable() table.Model {
	columns := []table.Column{
		{Title: "Setting", Width: 25},
		{Title: "Value", Width: 20},
		{Title: "Source", Width: 15},
		{Title: "Description", Width: 30},
	}
	
	t := table.New(
		table.WithColumns(columns),
		table.WithFocused(true),
		table.WithHeight(12),
	)
	
	return t
}

func createProfilesTable() table.Model {
	columns := []table.Column{
		{Title: "Profile", Width: 20},
		{Title: "Active", Width: 8},
		{Title: "Description", Width: 30},
		{Title: "Last Used", Width: 12},
	}
	
	t := table.New(
		table.WithColumns(columns),
		table.WithHeight(8),
	)
	
	return t
}

// handleEnterCmd handles enter key press
func (d *Dashboard) handleEnterCmd() tea.Cmd {
	switch d.currentView {
	case DashboardArchival:
		// Handle archival operations
		return tea.Cmd(func() tea.Msg {
			return dataUpdateMsg{Type: "archival_action"}
		})
	case DashboardInventory:
		// Handle inventory operations
		return tea.Cmd(func() tea.Msg {
			return dataUpdateMsg{Type: "inventory_action"}
		})
	case DashboardCosts:
		// Handle cost operations
		return tea.Cmd(func() tea.Msg {
			return dataUpdateMsg{Type: "cost_action"}
		})
	default:
		return nil
	}
}