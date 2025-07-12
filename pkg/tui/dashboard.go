// Package tui provides terminal user interface components for CargoShip
package tui

import (
	"fmt"
	"log/slog"
	"time"

	"github.com/charmbracelet/bubbles/table"
	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"

	contextpkg "github.com/scttfrdmn/cargoship/pkg/context"
)

// DashboardType represents different dashboard interfaces
type DashboardType int

const (
	DashboardLocal      DashboardType = iota // Local filesystem monitoring
	DashboardAgent                           // Single agent monitoring
	DashboardController                      // Multi-agent controller dashboard
)

// Dashboard represents a TUI dashboard
type Dashboard struct {
	dashboardType   DashboardType
	contextManager  *contextpkg.Manager
	logger          *slog.Logger
	
	// UI components
	agentTable      table.Model
	jobsTable       table.Model
	metricsTable    table.Model
	
	// Data
	agents          []AgentInfo
	jobs           []JobInfo
	metrics        SystemMetrics
	
	// State
	focused         int
	lastUpdate     time.Time
	updateInterval time.Duration
	
	// Styling
	baseStyle      lipgloss.Style
	focusedStyle   lipgloss.Style
	titleStyle     lipgloss.Style
}

// AgentInfo represents agent status information for display
type AgentInfo struct {
	ID          string
	Name        string
	Status      string
	Endpoint    string
	Jobs        int
	Throughput  string
	LastSeen    time.Time
	Progress    float64
}

// JobInfo represents job information for display
type JobInfo struct {
	ID          string
	AgentID     string
	Type        string
	Path        string
	Status      string
	Progress    float64
	StartTime   time.Time
	Size        string
	Rate        string
}

// SystemMetrics represents system performance metrics
type SystemMetrics struct {
	TotalAgents    int
	ActiveJobs     int
	CompletedJobs  int64
	FailedJobs     int64
	TotalThroughput string
	Uptime         time.Duration
	MemoryUsage    string
	CPUUsage       float64
}

// NewDashboard creates a new TUI dashboard
func NewDashboard(dashboardType DashboardType, logger *slog.Logger) *Dashboard {
	contextManager := contextpkg.NewManager(logger)
	
	// Initialize styling
	baseStyle := lipgloss.NewStyle().
		BorderStyle(lipgloss.NormalBorder()).
		BorderForeground(lipgloss.Color("240"))
		
	focusedStyle := lipgloss.NewStyle().
		BorderStyle(lipgloss.NormalBorder()).
		BorderForeground(lipgloss.Color("69"))
		
	titleStyle := lipgloss.NewStyle().
		Foreground(lipgloss.Color("86")).
		Background(lipgloss.Color("235")).
		Padding(0, 1)
	
	// Initialize tables
	agentTable := createAgentTable()
	jobsTable := createJobsTable()
	metricsTable := createMetricsTable()
	
	return &Dashboard{
		dashboardType:   dashboardType,
		contextManager:  contextManager,
		logger:          logger.With("component", "tui-dashboard"),
		agentTable:      agentTable,
		jobsTable:       jobsTable,
		metricsTable:    metricsTable,
		updateInterval:  time.Second * 2,
		baseStyle:       baseStyle,
		focusedStyle:    focusedStyle,
		titleStyle:      titleStyle,
	}
}

// Run starts the TUI dashboard
func (d *Dashboard) Run() error {
	d.logger.Info("Starting TUI dashboard", "type", d.dashboardType)
	
	// Create Bubble Tea program
	p := tea.NewProgram(d, tea.WithAltScreen())
	
	// Run the program
	_, err := p.Run()
	if err != nil {
		return fmt.Errorf("failed to run TUI dashboard: %w", err)
	}
	
	d.logger.Info("TUI dashboard stopped")
	return nil
}

// Init implements tea.Model
func (d *Dashboard) Init() tea.Cmd {
	return tea.Batch(
		d.tickCmd(),
		d.fetchDataCmd(),
	)
}

// Update implements tea.Model
func (d *Dashboard) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
	var cmds []tea.Cmd
	
	switch msg := msg.(type) {
	case tea.KeyMsg:
		switch msg.String() {
		case "ctrl+c", "q":
			return d, tea.Quit
			
		case "tab":
			d.focused = (d.focused + 1) % 3
			
		case "shift+tab":
			d.focused = (d.focused - 1 + 3) % 3
			
		case "r":
			// Force refresh
			cmds = append(cmds, d.fetchDataCmd())
		}
		
	case tickMsg:
		cmds = append(cmds, d.tickCmd())
		if time.Since(d.lastUpdate) >= d.updateInterval {
			cmds = append(cmds, d.fetchDataCmd())
		}
		
	case dataUpdateMsg:
		d.updateData(msg)
		d.lastUpdate = time.Now()
	}
	
	return d, tea.Batch(cmds...)
}

// View implements tea.Model
func (d *Dashboard) View() string {
	switch d.dashboardType {
	case DashboardController:
		return d.renderControllerDashboard()
	case DashboardAgent:
		return d.renderAgentDashboard()
	case DashboardLocal:
		return d.renderLocalDashboard()
	default:
		return d.renderControllerDashboard()
	}
}

// renderControllerDashboard renders the multi-agent controller dashboard
func (d *Dashboard) renderControllerDashboard() string {
	title := d.titleStyle.Render("🚢 CargoShip Controller Dashboard")
	
	// Header with system metrics
	header := d.renderSystemMetrics()
	
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
	jobsTitle := "Active Jobs"
	if d.focused == 1 {
		jobsTitle = d.focusedStyle.Render(jobsTitle)
	}
	jobsSection := lipgloss.JoinVertical(lipgloss.Left,
		jobsTitle,
		d.jobsTable.View(),
	)
	
	// Recent activity
	activityTitle := "Recent Activity"
	if d.focused == 2 {
		activityTitle = d.focusedStyle.Render(activityTitle)
	}
	activitySection := lipgloss.JoinVertical(lipgloss.Left,
		activityTitle,
		d.renderRecentActivity(),
	)
	
	// Footer
	footer := d.renderFooter()
	
	// Combine all sections
	return lipgloss.JoinVertical(lipgloss.Left,
		title,
		"",
		header,
		"",
		agentSection,
		"",
		jobsSection,
		"",
		activitySection,
		"",
		footer,
	)
}

// renderAgentDashboard renders a single agent monitoring dashboard
func (d *Dashboard) renderAgentDashboard() string {
	title := d.titleStyle.Render("🤖 CargoShip Agent Dashboard")
	
	// Agent status
	status := d.renderAgentStatus()
	
	// Job queue
	jobs := d.renderAgentJobs()
	
	// Performance metrics
	metrics := d.renderAgentMetrics()
	
	// Footer
	footer := d.renderFooter()
	
	return lipgloss.JoinVertical(lipgloss.Left,
		title,
		"",
		status,
		"",
		jobs,
		"",
		metrics,
		"",
		footer,
	)
}

// renderLocalDashboard renders a local filesystem monitoring dashboard
func (d *Dashboard) renderLocalDashboard() string {
	title := d.titleStyle.Render("🏠 CargoShip Local Dashboard")
	
	// Filesystem status
	filesystem := d.renderFilesystemStatus()
	
	// Archive queue
	archives := d.renderArchiveQueue()
	
	// System resources
	resources := d.renderSystemResources()
	
	// Footer
	footer := d.renderFooter()
	
	return lipgloss.JoinVertical(lipgloss.Left,
		title,
		"",
		filesystem,
		"",
		archives,
		"",
		resources,
		"",
		footer,
	)
}

// renderSystemMetrics renders the system metrics header
func (d *Dashboard) renderSystemMetrics() string {
	return fmt.Sprintf(
		"Agents: %d active | Jobs: %d running, %d completed, %d failed | Throughput: %s | Uptime: %s",
		d.metrics.TotalAgents,
		d.metrics.ActiveJobs,
		d.metrics.CompletedJobs,
		d.metrics.FailedJobs,
		d.metrics.TotalThroughput,
		d.formatDuration(d.metrics.Uptime),
	)
}

// renderFooter renders the footer with controls
func (d *Dashboard) renderFooter() string {
	return "Press q to quit • Tab to switch focus • r to refresh • Last updated: " + 
		d.lastUpdate.Format("15:04:05")
}

// Message types for Bubble Tea
type tickMsg time.Time
type dataUpdateMsg struct {
	agents  []AgentInfo
	jobs    []JobInfo
	metrics SystemMetrics
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
			agents:  d.fetchMockAgents(),
			jobs:    d.fetchMockJobs(),
			metrics: d.fetchMockMetrics(),
		}
	}
}

// updateData updates the dashboard with fresh data
func (d *Dashboard) updateData(msg dataUpdateMsg) {
	d.agents = msg.agents
	d.jobs = msg.jobs
	d.metrics = msg.metrics
	
	// Update table data
	d.updateAgentTable()
	d.updateJobsTable()
}

// Helper methods for rendering different dashboard sections
func (d *Dashboard) renderRecentActivity() string {
	// TODO: Implement recent activity display
	return "Recent job completions and system events will appear here..."
}

func (d *Dashboard) renderAgentStatus() string {
	// TODO: Implement agent status display
	return "Agent connection status, health, and configuration..."
}

func (d *Dashboard) renderAgentJobs() string {
	// TODO: Implement agent job queue display
	return "Current job queue and execution status..."
}

func (d *Dashboard) renderAgentMetrics() string {
	// TODO: Implement agent performance metrics
	return "Performance metrics, throughput, and resource usage..."
}

func (d *Dashboard) renderFilesystemStatus() string {
	// TODO: Implement filesystem monitoring
	return "Filesystem usage, watched directories, and file detection..."
}

func (d *Dashboard) renderArchiveQueue() string {
	// TODO: Implement archive queue display
	return "Pending archives, compression status, and upload queue..."
}

func (d *Dashboard) renderSystemResources() string {
	// TODO: Implement system resource monitoring
	return "CPU, memory, disk usage, and network statistics..."
}

// Utility methods
func (d *Dashboard) formatDuration(duration time.Duration) string {
	hours := int(duration.Hours())
	minutes := int(duration.Minutes()) % 60
	return fmt.Sprintf("%dh %dm", hours, minutes)
}

// Table creation helpers
func createAgentTable() table.Model {
	columns := []table.Column{
		{Title: "ID", Width: 15},
		{Title: "Name", Width: 20},
		{Title: "Status", Width: 10},
		{Title: "Jobs", Width: 8},
		{Title: "Throughput", Width: 12},
		{Title: "Progress", Width: 10},
	}
	
	t := table.New(
		table.WithColumns(columns),
		table.WithFocused(true),
		table.WithHeight(7),
	)
	
	return t
}

func createJobsTable() table.Model {
	columns := []table.Column{
		{Title: "Job ID", Width: 12},
		{Title: "Agent", Width: 15},
		{Title: "Type", Width: 10},
		{Title: "Path", Width: 25},
		{Title: "Status", Width: 10},
		{Title: "Progress", Width: 10},
	}
	
	t := table.New(
		table.WithColumns(columns),
		table.WithHeight(7),
	)
	
	return t
}

func createMetricsTable() table.Model {
	columns := []table.Column{
		{Title: "Metric", Width: 20},
		{Title: "Value", Width: 15},
		{Title: "Unit", Width: 10},
	}
	
	t := table.New(
		table.WithColumns(columns),
		table.WithHeight(5),
	)
	
	return t
}

// updateAgentTable updates the agent table with fresh data
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

// updateJobsTable updates the jobs table with fresh data
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
		})
	}
	d.jobsTable.SetRows(rows)
}

// Mock data generators (TODO: Replace with real data sources)
func (d *Dashboard) fetchMockAgents() []AgentInfo {
	return []AgentInfo{
		{
			ID:         "nas-lab-001",
			Name:       "Lab NAS Agent",
			Status:     "working",
			Jobs:       2,
			Throughput: "45.2 MB/s",
			Progress:   83.5,
		},
		{
			ID:         "storage-rack-02",
			Name:       "Backup Storage",
			Status:     "ready",
			Jobs:       0,
			Throughput: "0 MB/s",
			Progress:   0,
		},
	}
}

func (d *Dashboard) fetchMockJobs() []JobInfo {
	return []JobInfo{
		{
			ID:       "job-001",
			AgentID:  "nas-lab-001",
			Type:     "archive",
			Path:     "/data/genomics/samples",
			Status:   "running",
			Progress: 67.8,
			Size:     "2.3 GB",
			Rate:     "34 MB/s",
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
	}
}