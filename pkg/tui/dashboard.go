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
	DashboardOverview    DashboardType = iota // Main overview dashboard
	DashboardArchival                         // Data archival operations
	DashboardInventory                        // Inventory management
	DashboardCosts                            // Cost analysis and optimization
	DashboardAgents                           // Agent management
	DashboardConfig                           // Configuration management
	DashboardLogs                             // Logs and monitoring
	DashboardMultiRegion                      // Multi-region monitoring and management
)

// Dashboard represents a TUI dashboard
type Dashboard struct {
	dashboardType  DashboardType
	contextManager *contextpkg.Manager
	logger         *slog.Logger

	// Navigation
	currentView    DashboardType
	navigationTabs []string
	selectedTab    int

	// UI components - Agent Management
	agentTable table.Model
	jobsTable  table.Model

	// UI components - Archival Operations
	archivalQueue table.Model
	estimateTable table.Model
	surveyResults table.Model

	// UI components - Inventory Management
	inventoryTree table.Model
	searchResults table.Model
	restoreQueue  table.Model

	// UI components - Cost Analysis
	costBreakdown table.Model
	optimizations table.Model
	budgetChart   table.Model

	// UI components - Configuration
	configTable  table.Model
	profilesList table.Model

	// UI components - Multi-region
	regionOverviewTable table.Model
	regionHealthTable   table.Model
	regionMetricsTable  table.Model
	failoverStatusTable table.Model

	// Data
	agents         []AgentInfo
	jobs           []JobInfo
	metrics        SystemMetrics
	archivalJobs   []ArchivalJob
	inventoryItems []InventoryItem
	// costData        CostAnalysis // TODO: Implement cost analysis integration
	configurations []ConfigItem
	
	// Multi-region data
	regionStatus         map[string]RegionStatusInfo
	globalMetrics        GlobalMetricsInfo
	failoverOperations   []FailoverOperation
	lastRegionUpdate     time.Time
	regionUpdateInterval time.Duration

	// State
	focused        int
	lastUpdate     time.Time
	updateInterval time.Duration

	// Styling
	baseStyle      lipgloss.Style
	focusedStyle   lipgloss.Style
	titleStyle     lipgloss.Style
	tabStyle       lipgloss.Style
	activeTabStyle lipgloss.Style
}

// AgentInfo represents agent status information for display
type AgentInfo struct {
	ID         string
	Name       string
	Status     string
	Endpoint   string
	Jobs       int
	Throughput string
	LastSeen   time.Time
	Progress   float64
}

// JobInfo represents job information for display
type JobInfo struct {
	ID        string
	AgentID   string
	Type      string
	Path      string
	Status    string
	Progress  float64
	StartTime time.Time
	Size      string
	Rate      string
}

// SystemMetrics represents system performance metrics
type SystemMetrics struct {
	TotalAgents     int
	ActiveJobs      int
	CompletedJobs   int64
	FailedJobs      int64
	TotalThroughput string
	Uptime          time.Duration
	MemoryUsage     string
	CPUUsage        float64
	StorageUsed     string
	MonthlySpend    string
	ProjectedCost   string
}

// ArchivalJob represents an archival operation
type ArchivalJob struct {
	ID            string
	Source        string
	Destination   string
	Status        string
	Progress      float64
	StartTime     time.Time
	EstimatedCost string
	StorageClass  string
	Size          string
	Rate          string
}

// InventoryItem represents an inventory item
type InventoryItem struct {
	Path         string
	Type         string
	Size         string
	LastModified time.Time
	StorageClass string
	Cost         string
	Metadata     map[string]string
}

// CostAnalysis represents cost analysis data
type CostAnalysis struct {
	CurrentSpend   string
	ProjectedSpend string
	Optimizations  []CostOptimization
	Breakdown      []CostBreakdownItem
	Budget         BudgetInfo
}

// CostOptimization represents a cost optimization suggestion
type CostOptimization struct {
	Type            string
	Description     string
	PotentialSaving string
	Impact          string
	Effort          string
}

// CostBreakdownItem represents a cost breakdown item
type CostBreakdownItem struct {
	Category   string
	Amount     string
	Percentage float64
	Trend      string
}

// BudgetInfo represents budget information
type BudgetInfo struct {
	Monthly   string
	Used      string
	Remaining string
	DaysLeft  int
}

// ConfigItem represents a configuration item
type ConfigItem struct {
	Key         string
	Value       string
	Type        string
	Description string
	Default     string
	Source      string
}

// RegionStatusInfo represents real-time region status information
type RegionStatusInfo struct {
	Name              string
	Status            string // healthy, degraded, unhealthy, offline
	Priority          int
	Weight            int
	LastChecked       time.Time
	Health            RegionHealthInfo
	Metrics           RegionMetricsInfo
	FailoverTarget    string
	InFailover        bool
}

// RegionHealthInfo represents health check information for a region  
type RegionHealthInfo struct {
	OverallHealthy        bool
	SuccessRate          float64
	ConsecutiveSuccesses int64
	ConsecutiveFailures  int64
	LastHealthCheck      time.Time
	HealthCheckLatency   time.Duration
	FailureReasons       []string
}

// RegionMetricsInfo represents operational metrics for a region
type RegionMetricsInfo struct {
	AverageLatency     time.Duration
	Throughput         string
	ErrorRate          float64
	ActiveUploads      int64
	SuccessfulUploads  int64
	FailedUploads      int64
	CPUUtilization     float64
	MemoryUtilization  float64
	StorageUtilization float64
	BandwidthUsage     string
	LastUpdated        time.Time
}

// GlobalMetricsInfo represents system-wide multi-region metrics
type GlobalMetricsInfo struct {
	TotalRegions         int
	HealthyRegions       int
	RegionAvailability   float64
	GlobalThroughput     string
	AverageLatency       time.Duration
	TotalUploads         int64
	GlobalErrorRate      float64
	SystemHealthScore    float64
	TotalCost            string
	EstimatedMonthlyCost string
	LastUpdated          time.Time
}

// FailoverOperation represents an active or recent failover operation
type FailoverOperation struct {
	ID            string
	FromRegion    string
	ToRegion      string
	Strategy      string // immediate, graceful, manual
	Status        string // initiated, in_progress, completed, failed
	StartTime     time.Time
	CompletedTime time.Time
	Duration      time.Duration
	Reason        string
	TriggerType   string // automatic, manual
	Success       bool
	ErrorMessage  string
}

// NewDashboard creates a new TUI dashboard
func NewDashboard(dashboardType DashboardType, logger *slog.Logger) *Dashboard {
	contextManager := contextpkg.NewManager(logger)

	// Initialize navigation tabs
	navigationTabs := []string{
		"🏠 Overview", "📦 Archive", "📋 Inventory",
		"💰 Costs", "🤖 Agents", "⚙️ Config", "📝 Logs", "🌐 Multi-Region",
	}

	// Initialize styling
	baseStyle := lipgloss.NewStyle().
		BorderStyle(lipgloss.NormalBorder()).
		BorderForeground(lipgloss.Color("240"))

	focusedStyle := lipgloss.NewStyle().
		BorderStyle(lipgloss.NormalBorder()).
		BorderForeground(lipgloss.Color("69"))

	tabStyle := lipgloss.NewStyle().
		Padding(0, 1).
		Foreground(lipgloss.Color("252")).
		Background(lipgloss.Color("238"))

	activeTabStyle := lipgloss.NewStyle().
		Padding(0, 1).
		Foreground(lipgloss.Color("15")).
		Background(lipgloss.Color("69")).
		Bold(true)

	titleStyle := lipgloss.NewStyle().
		Foreground(lipgloss.Color("86")).
		Background(lipgloss.Color("235")).
		Padding(0, 1)

	// Initialize tables
	agentTable := createAgentTable()
	jobsTable := createJobsTable()

	return &Dashboard{
		dashboardType:  dashboardType,
		currentView:    DashboardOverview,
		navigationTabs: navigationTabs,
		selectedTab:    0,
		contextManager: contextManager,
		logger:         logger.With("component", "tui-dashboard"),
		agentTable:     agentTable,
		jobsTable:      jobsTable,
		archivalQueue:  createArchivalTable(),
		estimateTable:  createEstimateTable(),
		surveyResults:  createSurveyTable(),
		inventoryTree:  createInventoryTable(),
		searchResults:  createSearchTable(),
		restoreQueue:   createRestoreTable(),
		costBreakdown:  createCostBreakdownTable(),
		optimizations:  createOptimizationTable(),
		budgetChart:    createBudgetTable(),
		configTable:    createConfigTable(),
		profilesList:   createProfilesTable(),
		regionOverviewTable: createRegionOverviewTable(),
		regionHealthTable:   createRegionHealthTable(),
		regionMetricsTable:  createRegionMetricsTable(),
		failoverStatusTable: createFailoverStatusTable(),
		regionStatus:         make(map[string]RegionStatusInfo),
		globalMetrics:        GlobalMetricsInfo{}, // Will be populated by fetchMockGlobalMetrics
		updateInterval:       time.Second * 2,
		regionUpdateInterval: time.Second * 5,
		baseStyle:            baseStyle,
		focusedStyle:         focusedStyle,
		titleStyle:           titleStyle,
		tabStyle:             tabStyle,
		activeTabStyle:       activeTabStyle,
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

		case "tab", "right":
			d.selectedTab = (d.selectedTab + 1) % len(d.navigationTabs)
			d.currentView = DashboardType(d.selectedTab)

		case "shift+tab", "left":
			d.selectedTab = (d.selectedTab - 1 + len(d.navigationTabs)) % len(d.navigationTabs)
			d.currentView = DashboardType(d.selectedTab)

		case "up":
			if d.focused > 0 {
				d.focused--
			}

		case "down":
			d.focused = (d.focused + 1) % 3

		case "r":
			// Force refresh
			cmds = append(cmds, d.fetchDataCmd())

		case "enter":
			// Handle enter key for current focused item
			cmds = append(cmds, d.handleEnterCmd())

		case "1", "2", "3", "4", "5", "6", "7":
			// Quick navigation to tabs
			if tabIndex := int(msg.String()[0] - '1'); tabIndex < len(d.navigationTabs) {
				d.selectedTab = tabIndex
				d.currentView = DashboardType(tabIndex)
			}
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
	// Render navigation tabs
	var tabs []string
	for i, tab := range d.navigationTabs {
		if i == d.selectedTab {
			tabs = append(tabs, d.activeTabStyle.Render(tab))
		} else {
			tabs = append(tabs, d.tabStyle.Render(tab))
		}
	}

	header := lipgloss.JoinHorizontal(lipgloss.Top, tabs...)

	// Render content based on current view
	var content string
	switch d.currentView {
	case DashboardOverview:
		content = d.renderOverviewDashboard()
	case DashboardArchival:
		content = d.renderArchivalDashboard()
	case DashboardInventory:
		content = d.renderInventoryDashboard()
	case DashboardCosts:
		content = d.renderCostsDashboard()
	case DashboardAgents:
		content = d.renderAgentsDashboard()
	case DashboardConfig:
		content = d.renderConfigDashboard()
	case DashboardLogs:
		content = d.renderLogsDashboard()
	case DashboardMultiRegion:
		content = d.renderMultiRegionDashboard()
	default:
		content = d.renderOverviewDashboard()
	}

	// Add help text
	helpText := d.renderHelpText()

	return lipgloss.JoinVertical(lipgloss.Left, header, content, helpText)
}
