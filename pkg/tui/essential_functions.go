package tui

import (
	"fmt"
	"time"

	"github.com/charmbracelet/bubbles/table"
	tea "github.com/charmbracelet/bubbletea"
)

// Message types for Bubble Tea
type tickMsg time.Time
type dataUpdateMsg struct {
	Type           string
	agents         []AgentInfo
	jobs           []JobInfo
	metrics        SystemMetrics
	archivalJobs   []ArchivalJob
	inventoryItems []InventoryItem
	configurations []ConfigItem
}

// tickCmd returns a command that sends periodic tick messages
func (d *Dashboard) tickCmd() tea.Cmd {
	return tea.Tick(time.Second, func(t time.Time) tea.Msg {
		return tickMsg(t)
	})
}

// fetchDataCmd returns a command that fetches fresh data
func (d *Dashboard) fetchDataCmd() tea.Cmd {
	return func() tea.Msg {
		// TODO: Implement actual data fetching from controller/agent APIs
		// For now, return mock data
		return dataUpdateMsg{
			Type:           "data_refresh",
			agents:         d.fetchMockAgents(),
			jobs:           d.fetchMockJobs(),
			metrics:        d.fetchMockMetrics(),
			archivalJobs:   d.fetchMockArchivalJobs(),
			inventoryItems: d.fetchMockInventoryItems(),
			configurations: d.fetchMockConfigurations(),
		}
	}
}

// updateData updates the dashboard with fresh data
func (d *Dashboard) updateData(msg dataUpdateMsg) {
	if len(msg.agents) > 0 {
		d.agents = msg.agents
	}
	if len(msg.jobs) > 0 {
		d.jobs = msg.jobs
	}
	if msg.metrics.TotalAgents >= 0 {
		d.metrics = msg.metrics
	}
	if len(msg.archivalJobs) > 0 {
		d.archivalJobs = msg.archivalJobs
	}
	if len(msg.inventoryItems) > 0 {
		d.inventoryItems = msg.inventoryItems
	}
	if len(msg.configurations) > 0 {
		d.configurations = msg.configurations
	}

	// Update tables based on current view
	switch d.currentView {
	case DashboardAgents:
		d.updateAgentTable()
		d.updateJobsTable()
	case DashboardArchival:
		d.updateArchivalTables()
	case DashboardInventory:
		d.updateInventoryTables()
	case DashboardCosts:
		d.updateCostTables()
	case DashboardConfig:
		d.updateConfigTables()
	}
}

// Table creation functions
func createAgentTable() table.Model {
	columns := []table.Column{
		{Title: "ID", Width: 15},
		{Title: "Name", Width: 20},
		{Title: "Status", Width: 12},
		{Title: "Jobs", Width: 8},
		{Title: "Throughput", Width: 15},
		{Title: "Progress", Width: 12},
	}

	t := table.New(
		table.WithColumns(columns),
		table.WithFocused(true),
		table.WithHeight(10),
	)

	return t
}

func createJobsTable() table.Model {
	columns := []table.Column{
		{Title: "Job ID", Width: 12},
		{Title: "Agent", Width: 15},
		{Title: "Type", Width: 10},
		{Title: "Path", Width: 25},
		{Title: "Status", Width: 12},
		{Title: "Progress", Width: 10},
		{Title: "Size", Width: 12},
		{Title: "Rate", Width: 12},
	}

	t := table.New(
		table.WithColumns(columns),
		table.WithHeight(8),
	)

	return t
}

// Table update functions
func (d *Dashboard) updateAgentTable() {
	var rows []table.Row
	for _, agent := range d.agents {
		progress := fmt.Sprintf("%.1f%%", agent.Progress)
		rows = append(rows, table.Row{
			agent.ID,
			agent.Name,
			agent.Status,
			fmt.Sprintf("%d", agent.Jobs),
			agent.Throughput,
			progress,
		})
	}
	d.agentTable.SetRows(rows)
}

func (d *Dashboard) updateJobsTable() {
	var rows []table.Row
	for _, job := range d.jobs {
		progress := fmt.Sprintf("%.1f%%", job.Progress)
		rows = append(rows, table.Row{
			job.ID,
			job.AgentID,
			job.Type,
			job.Path,
			job.Status,
			progress,
			job.Size,
			job.Rate,
		})
	}
	d.jobsTable.SetRows(rows)
}

// Mock data functions
func (d *Dashboard) fetchMockAgents() []AgentInfo {
	return []AgentInfo{
		{
			ID:         "nas-lab-001",
			Name:       "Lab NAS Agent",
			Status:     "working",
			Endpoint:   "http://nas-lab-001:8080",
			Jobs:       2,
			Throughput: "45.2 MB/s",
			LastSeen:   time.Now(),
			Progress:   83.5,
		},
		{
			ID:         "storage-rack-02",
			Name:       "Backup Storage",
			Status:     "ready",
			Endpoint:   "http://storage-rack-02:8080",
			Jobs:       0,
			Throughput: "0 MB/s",
			LastSeen:   time.Now(),
			Progress:   0,
		},
	}
}

func (d *Dashboard) fetchMockJobs() []JobInfo {
	return []JobInfo{
		{
			ID:        "job-001",
			AgentID:   "nas-lab-001",
			Type:      "archive",
			Path:      "/data/genomics/samples",
			Status:    "running",
			Progress:  67.8,
			StartTime: time.Now(),
			Size:      "2.3 GB",
			Rate:      "34 MB/s",
		},
	}
}

func (d *Dashboard) fetchMockMetrics() SystemMetrics {
	return SystemMetrics{
		TotalAgents:     2,
		ActiveJobs:      1,
		CompletedJobs:   15,
		FailedJobs:      2,
		TotalThroughput: "45.2 MB/s",
		Uptime:          time.Hour*4 + time.Minute*23,
		MemoryUsage:     "2.3 GB",
		CPUUsage:        45.2,
		StorageUsed:     "3.2TB",
		MonthlySpend:    "$137.50",
		ProjectedCost:   "$1,650/year",
	}
}
