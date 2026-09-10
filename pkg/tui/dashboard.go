// Package tui provides terminal user interface components for CargoShip.
package tui

import (
	"context"
	"fmt"
	"log/slog"
	"sort"
	"time"

	"github.com/charmbracelet/bubbles/table"
	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"

	"github.com/scttfrdmn/cargoship/pkg/aws/cost"
	"github.com/scttfrdmn/cargoship/pkg/resume"
)

// DashboardType selects which view the dashboard opens on.
type DashboardType int

const (
	// DashboardOverview is a real summary of recorded spend and in-progress uploads.
	DashboardOverview DashboardType = iota
	// DashboardCosts shows the recorded cost breakdown and budget status.
	DashboardCosts
	// DashboardUploads lists in-progress / resumable uploads.
	DashboardUploads
)

// CostProvider is the subset of *cost.Manager the dashboard reads. It is an
// interface so the dashboard degrades gracefully when the cost manager cannot be
// built, and so the view logic can be exercised with a fake in tests.
type CostProvider interface {
	GetReporter() *cost.CostReporter
	GetBudgetStatus() map[string]interface{}
}

// Dashboard is a read-only Bubble Tea dashboard over CargoShip's real local
// data: the recorded cost ledger (pkg/aws/cost) and in-progress upload state
// (pkg/resume). It fabricates nothing and performs no live bucket scans — when a
// source has no data it says so rather than showing invented numbers.
type Dashboard struct {
	ctx     context.Context
	cost    CostProvider // nil when the cost manager is unavailable
	logger  *slog.Logger
	refresh time.Duration

	tabs        []string
	currentView DashboardType

	// Fetched data.
	summary    *cost.CostSummary
	budget     map[string]interface{}
	uploads    []*resume.UploadState
	fetchErr   error
	lastUpdate time.Time

	// Widgets.
	costTable   table.Model
	uploadTable table.Model

	// Styles.
	titleStyle     lipgloss.Style
	tabStyle       lipgloss.Style
	activeTabStyle lipgloss.Style
	helpStyle      lipgloss.Style
	errStyle       lipgloss.Style
}

// NewDashboard builds a dashboard. costProvider may be nil (the Costs/Overview
// spend figures then report as unavailable while Uploads still works). initial
// selects the opening view; refresh is the data refresh cadence (a sane default
// is applied when non-positive).
func NewDashboard(ctx context.Context, costProvider CostProvider, initial DashboardType, refresh time.Duration, logger *slog.Logger) *Dashboard {
	if logger == nil {
		logger = slog.Default()
	}
	if refresh <= 0 {
		refresh = 5 * time.Second
	}

	costTable := table.New(
		table.WithColumns([]table.Column{
			{Title: "Storage Class", Width: 40},
			{Title: "Cost (USD)", Width: 16},
		}),
		table.WithFocused(true),
		table.WithHeight(12),
	)
	uploadTable := table.New(
		table.WithColumns([]table.Column{
			{Title: "Upload ID", Width: 26},
			{Title: "Source", Width: 26},
			{Title: "Destination", Width: 30},
			{Title: "Progress", Width: 10},
			{Title: "Age", Width: 12},
		}),
		table.WithFocused(true),
		table.WithHeight(12),
	)

	return &Dashboard{
		ctx:            ctx,
		cost:           costProvider,
		logger:         logger.With("component", "tui-dashboard"),
		refresh:        refresh,
		currentView:    initial,
		tabs:           []string{"🏠 Overview", "💰 Costs", "📦 Uploads"},
		costTable:      costTable,
		uploadTable:    uploadTable,
		titleStyle:     lipgloss.NewStyle().Foreground(lipgloss.Color("86")).Background(lipgloss.Color("235")).Padding(0, 1),
		tabStyle:       lipgloss.NewStyle().Padding(0, 1).Foreground(lipgloss.Color("252")).Background(lipgloss.Color("238")),
		activeTabStyle: lipgloss.NewStyle().Padding(0, 1).Foreground(lipgloss.Color("15")).Background(lipgloss.Color("69")).Bold(true),
		helpStyle:      lipgloss.NewStyle().Foreground(lipgloss.Color("241")).Padding(1, 0, 0, 0),
		errStyle:       lipgloss.NewStyle().Foreground(lipgloss.Color("203")),
	}
}

// Run starts the dashboard program.
func (d *Dashboard) Run() error {
	if _, err := tea.NewProgram(d, tea.WithAltScreen()).Run(); err != nil {
		return fmt.Errorf("failed to run TUI dashboard: %w", err)
	}
	return nil
}

// dataMsg carries a refreshed snapshot of the real data sources.
type dataMsg struct {
	summary *cost.CostSummary
	budget  map[string]interface{}
	uploads []*resume.UploadState
	err     error
}

type tickMsg time.Time

// Init implements tea.Model.
func (d *Dashboard) Init() tea.Cmd {
	return tea.Batch(d.fetch(), d.tick())
}

func (d *Dashboard) tick() tea.Cmd {
	return tea.Tick(d.refresh, func(t time.Time) tea.Msg { return tickMsg(t) })
}

// fetch reads the real data sources off the UI goroutine.
func (d *Dashboard) fetch() tea.Cmd {
	ctx, cp := d.ctx, d.cost
	return func() tea.Msg {
		msg := dataMsg{}
		// In-progress / resumable uploads (local, no AWS calls).
		if states, err := resume.ListStates(); err == nil {
			msg.uploads = states
		} else {
			msg.err = err
		}
		// Recorded cost ledger + budget (local store; no live pricing calls).
		if cp != nil {
			msg.budget = cp.GetBudgetStatus()
			if summary, err := cp.GetReporter().GenerateReport(ctx, "month"); err == nil {
				msg.summary = summary
			}
			// A report error (e.g. no records yet) is not fatal — Costs shows an
			// honest empty state.
		}
		return msg
	}
}

// Update implements tea.Model.
func (d *Dashboard) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
	switch msg := msg.(type) {
	case tea.KeyMsg:
		switch msg.String() {
		case "q", "ctrl+c", "esc":
			return d, tea.Quit
		case "tab", "right", "l":
			d.currentView = (d.currentView + 1) % DashboardType(len(d.tabs))
		case "shift+tab", "left", "h":
			d.currentView = (d.currentView - 1 + DashboardType(len(d.tabs))) % DashboardType(len(d.tabs))
		case "1":
			d.currentView = DashboardOverview
		case "2":
			d.currentView = DashboardCosts
		case "3":
			d.currentView = DashboardUploads
		case "r":
			return d, d.fetch()
		}
	case tickMsg:
		return d, tea.Batch(d.fetch(), d.tick())
	case dataMsg:
		d.summary = msg.summary
		d.budget = msg.budget
		d.uploads = msg.uploads
		d.fetchErr = msg.err
		d.lastUpdate = time.Now()
		d.costTable.SetRows(costRows(msg.summary))
		d.uploadTable.SetRows(uploadRows(msg.uploads))
		return d, nil
	}

	// Route table navigation to the focused view's table.
	var cmd tea.Cmd
	switch d.currentView {
	case DashboardCosts:
		d.costTable, cmd = d.costTable.Update(msg)
	case DashboardUploads:
		d.uploadTable, cmd = d.uploadTable.Update(msg)
	}
	return d, cmd
}

// View implements tea.Model.
func (d *Dashboard) View() string {
	var tabs string
	for i, name := range d.tabs {
		if DashboardType(i) == d.currentView {
			tabs += d.activeTabStyle.Render(name)
		} else {
			tabs += d.tabStyle.Render(name)
		}
	}
	header := d.titleStyle.Render("CargoShip Dashboard")

	var body string
	switch d.currentView {
	case DashboardOverview:
		body = d.renderOverview()
	case DashboardCosts:
		body = d.renderCosts()
	case DashboardUploads:
		body = d.renderUploads()
	}

	help := d.helpStyle.Render("tab/1-3 switch · r refresh · q quit")
	if !d.lastUpdate.IsZero() {
		help = d.helpStyle.Render(fmt.Sprintf("tab/1-3 switch · r refresh · q quit · updated %s ago", time.Since(d.lastUpdate).Round(time.Second)))
	}
	return fmt.Sprintf("%s\n%s\n\n%s\n%s", header, tabs, body, help)
}

func (d *Dashboard) renderOverview() string {
	spend, haveSpend := budgetFloat(d.budget, "current_spend")
	if !haveSpend && d.summary != nil {
		spend, haveSpend = d.summary.TotalCost, true
	}
	used, haveUsed := budgetFloat(d.budget, "budget_used")

	spendLine := "  This month's spend:  (no cost data recorded yet — run an upload)"
	if haveSpend {
		spendLine = fmt.Sprintf("  This month's spend:  $%.2f", spend)
	}
	budgetLine := "  Budget used:         (no budget configured)"
	if haveUsed {
		budgetLine = fmt.Sprintf("  Budget used:         %.1f%%", used)
	}
	uploadsLine := fmt.Sprintf("  In-progress uploads: %d", len(d.uploads))

	out := "Overview\n\n" + spendLine + "\n" + budgetLine + "\n" + uploadsLine
	if d.cost == nil {
		out += "\n\n  " + d.errStyle.Render("cost manager unavailable (check AWS config) — Uploads still works")
	}
	return out
}

func (d *Dashboard) renderCosts() string {
	if d.cost == nil {
		return "Costs\n\n  " + d.errStyle.Render("cost manager unavailable (check AWS config)")
	}
	out := "Costs — recorded spend this month, by storage class\n\n" + d.costTable.View()
	if max, ok := budgetFloat(d.budget, "max_budget"); ok && max > 0 {
		spend, _ := budgetFloat(d.budget, "current_spend")
		rem, _ := budgetFloat(d.budget, "budget_remaining")
		out += fmt.Sprintf("\n\n  Budget: $%.2f spent of $%.2f (remaining $%.2f)", spend, max, rem)
	}
	return out
}

func (d *Dashboard) renderUploads() string {
	body := "Uploads — in-progress / resumable (resume with: cargoship resume <id>)\n\n" + d.uploadTable.View()
	if d.fetchErr != nil {
		body += "\n\n  " + d.errStyle.Render("could not read upload state: "+d.fetchErr.Error())
	}
	return body
}

// costRows maps a cost summary's per-storage-class spend into table rows. A nil
// or empty summary yields a single honest "no data" row rather than fabricated
// figures.
func costRows(summary *cost.CostSummary) []table.Row {
	if summary == nil || len(summary.ByStorageClass) == 0 {
		return []table.Row{{"(no cost data recorded yet — run an upload)", ""}}
	}
	classes := make([]string, 0, len(summary.ByStorageClass))
	for class := range summary.ByStorageClass {
		classes = append(classes, class)
	}
	sort.Strings(classes)

	rows := make([]table.Row, 0, len(classes)+1)
	for _, class := range classes {
		rows = append(rows, table.Row{class, fmt.Sprintf("$%.2f", summary.ByStorageClass[class])})
	}
	rows = append(rows, table.Row{"TOTAL", fmt.Sprintf("$%.2f", summary.TotalCost)})
	return rows
}

// uploadRows maps in-progress upload states into table rows. An empty slice
// yields a single honest "none" row.
func uploadRows(states []*resume.UploadState) []table.Row {
	if len(states) == 0 {
		return []table.Row{{"(no in-progress uploads)", "", "", "", ""}}
	}
	rows := make([]table.Row, 0, len(states))
	for _, s := range states {
		dest := fmt.Sprintf("s3://%s/%s", s.Bucket, s.Prefix)
		rows = append(rows, table.Row{
			s.UploadID,
			s.SourceDir,
			dest,
			fmt.Sprintf("%.0f%%", s.Progress()),
			s.Age().Round(time.Second).String(),
		})
	}
	return rows
}

// budgetFloat reads a float64 value from the budget-status map, reporting whether
// it was present and of the expected type.
func budgetFloat(m map[string]interface{}, key string) (float64, bool) {
	if m == nil {
		return 0, false
	}
	v, ok := m[key]
	if !ok {
		return 0, false
	}
	f, ok := v.(float64)
	return f, ok
}
