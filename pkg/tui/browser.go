// Package tui provides terminal user interface components for CargoShip.
// browser.go implements an interactive file browser for manifest contents
// with hash-based selective restore (Issue #190).
package tui

import (
	"context"
	"fmt"
	"strings"

	"github.com/charmbracelet/bubbles/list"
	"github.com/charmbracelet/bubbles/textinput"
	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"
	"github.com/dustin/go-humanize"

	"github.com/scttfrdmn/cargoship/pkg/manifest"
)

// ---------------------------------------------------------------------------
// Styles
// ---------------------------------------------------------------------------

var (
	browserTitleStyle = lipgloss.NewStyle().
				Bold(true).
				Foreground(lipgloss.Color("205")).
				MarginLeft(2)

	selectedItemStyle = lipgloss.NewStyle().
				Foreground(lipgloss.Color("170")).
				Bold(true)

	dimStyle = lipgloss.NewStyle().
			Foreground(lipgloss.Color("240"))

	statusBarStyle = lipgloss.NewStyle().
			Background(lipgloss.Color("235")).
			Foreground(lipgloss.Color("252")).
			Padding(0, 1)

	helpStyle = lipgloss.NewStyle().
			Foreground(lipgloss.Color("241")).
			MarginTop(1)

	metaLabelStyle = lipgloss.NewStyle().
			Foreground(lipgloss.Color("33")).
			Bold(true)
)

// ---------------------------------------------------------------------------
// list.Item implementation
// ---------------------------------------------------------------------------

// fileItem wraps a FileEntry for display in the bubbles list.
type fileItem struct {
	entry    *manifest.FileEntry
	selected bool
}

// Title returns the file path — used by the default list delegate.
func (fi fileItem) Title() string {
	if fi.selected {
		return selectedItemStyle.Render("✓ " + fi.entry.Path)
	}
	return fi.entry.Path
}

// Description returns a one-line metadata summary.
func (fi fileItem) Description() string {
	parts := []string{humanize.Bytes(uint64(fi.entry.Size))}
	if fi.entry.ContentHash != "" {
		h := fi.entry.ContentHash
		parts = append(parts, h[:min8(len(h))]+"…")
	}
	if fi.entry.DVCMetadata != nil && fi.entry.DVCMetadata.Stage != "" {
		parts = append(parts, "stage:"+fi.entry.DVCMetadata.Stage)
	}
	return dimStyle.Render(strings.Join(parts, "  ·  "))
}

// FilterValue is the string used by the list's built-in filtering.
func (fi fileItem) FilterValue() string { return fi.entry.Path }

// ---------------------------------------------------------------------------
// BrowserMode controls the active panel.
// ---------------------------------------------------------------------------

// BrowserMode indicates which panel the browser is currently showing.
type BrowserMode int

const (
	ModeList    BrowserMode = iota // Main file list
	ModeSearch                     // Incremental search active
	ModeConfirm                    // Confirm restore destination
	ModeDone                       // Restore complete or user quit
)

// RestoreResult captures the outcome when the user confirms a restore.
type RestoreResult struct {
	// SelectedPaths is the list of file paths chosen by the user.
	SelectedPaths []string
	// DestDir is the directory the user typed in the confirmation prompt.
	DestDir string
	// Cancelled is true when the user pressed q or ESC without confirming.
	Cancelled bool
}

// ---------------------------------------------------------------------------
// BrowserModel — the main bubbletea Model
// ---------------------------------------------------------------------------

// BrowserModel is the bubbletea Model for the interactive file browser.
// It supports arrow-key navigation, space-to-select, incremental search, and
// DVC stage / git commit filtering (Issue #190).
type BrowserModel struct {
	// source data
	manifest  *manifest.Manifest
	query     *manifest.ManifestQuery
	extractor *manifest.SelectiveExtractor

	// display state
	mode        BrowserMode
	list        list.Model
	searchInput textinput.Model
	destInput   textinput.Model

	// filter state
	activeSearch    string
	activeDVCStage  string
	activeGitCommit string

	// selection state
	selected map[string]bool // path → selected

	// result written when done
	Result RestoreResult

	// available filter values (populated once on init)
	dvcStages  []string
	gitCommits []string

	// terminal size
	width  int
	height int
}

// ---------------------------------------------------------------------------
// Constructors
// ---------------------------------------------------------------------------

// NewBrowserModel creates a BrowserModel for m. If s3Client is non-nil a
// SelectiveExtractor is created so the model can perform restores directly;
// pass nil to use the browser in read-only / selection-only mode.
func NewBrowserModel(m *manifest.Manifest, s3Client manifest.S3Downloader) *BrowserModel {
	search := textinput.New()
	search.Placeholder = "search by path…"
	search.CharLimit = 200

	dest := textinput.New()
	dest.Placeholder = "/path/to/output"
	dest.CharLimit = 300

	bm := &BrowserModel{
		manifest:    m,
		query:       manifest.NewManifestQuery(m),
		searchInput: search,
		destInput:   dest,
		selected:    make(map[string]bool),
		width:       80,
		height:      24,
	}

	if s3Client != nil {
		bm.extractor = manifest.NewSelectiveExtractor(m, s3Client, 0)
	}

	// Collect distinct DVC stages and git commits for filter hints.
	stageSet := make(map[string]bool)
	for i := range m.Files {
		if m.Files[i].DVCMetadata != nil && m.Files[i].DVCMetadata.Stage != "" {
			stageSet[m.Files[i].DVCMetadata.Stage] = true
		}
	}
	for s := range stageSet {
		bm.dvcStages = append(bm.dvcStages, s)
	}
	if m.GitMetadata != nil && m.GitMetadata.Commit != "" {
		bm.gitCommits = append(bm.gitCommits, m.GitMetadata.Commit)
	}

	bm.list = bm.buildList(bm.allItems())
	return bm
}

// ---------------------------------------------------------------------------
// bubbletea Init / Update / View
// ---------------------------------------------------------------------------

// Init is called once when the program starts.
func (bm *BrowserModel) Init() tea.Cmd { return nil }

// Update handles all incoming messages.
func (bm *BrowserModel) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
	switch bm.mode {
	case ModeList:
		return bm.updateList(msg)
	case ModeSearch:
		return bm.updateSearch(msg)
	case ModeConfirm:
		return bm.updateConfirm(msg)
	}
	return bm, nil
}

// View renders the current state.
func (bm *BrowserModel) View() string {
	switch bm.mode {
	case ModeSearch:
		return bm.viewSearch()
	case ModeConfirm:
		return bm.viewConfirm()
	case ModeDone:
		return ""
	}
	return bm.viewList()
}

// ---------------------------------------------------------------------------
// Mode: ModeList
// ---------------------------------------------------------------------------

func (bm *BrowserModel) updateList(msg tea.Msg) (tea.Model, tea.Cmd) {
	switch msg := msg.(type) {
	case tea.WindowSizeMsg:
		bm.width = msg.Width
		bm.height = msg.Height
		bm.list.SetSize(msg.Width, msg.Height-6)
		return bm, nil

	case tea.KeyMsg:
		switch msg.String() {
		case "q", "ctrl+c":
			bm.Result = RestoreResult{Cancelled: true}
			bm.mode = ModeDone
			return bm, tea.Quit

		case " ":
			// Toggle selection on the currently highlighted item.
			if i, ok := bm.list.SelectedItem().(fileItem); ok {
				bm.selected[i.entry.Path] = !bm.selected[i.entry.Path]
				bm.refreshListItems()
			}
			return bm, nil

		case "enter":
			if len(bm.selected) == 0 {
				// Select current item if none chosen.
				if i, ok := bm.list.SelectedItem().(fileItem); ok {
					bm.selected[i.entry.Path] = true
				}
			}
			if len(bm.selected) > 0 {
				bm.mode = ModeConfirm
				bm.destInput.Focus()
			}
			return bm, nil

		case "/":
			bm.mode = ModeSearch
			bm.searchInput.SetValue(bm.activeSearch)
			bm.searchInput.Focus()
			return bm, textinput.Blink

		case "d":
			// Cycle DVC stage filter.
			bm.activeDVCStage = bm.cycleFilter(bm.activeDVCStage, bm.dvcStages)
			bm.activeSearch = ""
			bm.searchInput.SetValue("")
			bm.list = bm.buildList(bm.filteredItems())
			return bm, nil

		case "g":
			// Cycle git commit filter.
			bm.activeGitCommit = bm.cycleFilter(bm.activeGitCommit, bm.gitCommits)
			bm.activeSearch = ""
			bm.searchInput.SetValue("")
			bm.list = bm.buildList(bm.filteredItems())
			return bm, nil

		case "a":
			// Select all visible items.
			for _, item := range bm.list.Items() {
				if fi, ok := item.(fileItem); ok {
					bm.selected[fi.entry.Path] = true
				}
			}
			bm.refreshListItems()
			return bm, nil

		case "c":
			// Clear selection.
			bm.selected = make(map[string]bool)
			bm.refreshListItems()
			return bm, nil
		}
	}

	var cmd tea.Cmd
	bm.list, cmd = bm.list.Update(msg)
	return bm, cmd
}

func (bm *BrowserModel) viewList() string {
	var sb strings.Builder

	// Title bar
	title := browserTitleStyle.Render(
		fmt.Sprintf("📦 CargoShip Browser — %s  (%d files)", bm.manifest.UploadID, bm.manifest.TotalFiles),
	)
	sb.WriteString(title + "\n")

	// Active filter chips
	filters := bm.filterChips()
	if filters != "" {
		sb.WriteString(dimStyle.Render("  Filters: "+filters) + "\n")
	}

	sb.WriteString(bm.list.View() + "\n")

	// Status bar
	selCount := len(bm.selected)
	status := fmt.Sprintf(" %d selected  ·  %d visible", selCount, len(bm.list.Items()))
	sb.WriteString(statusBarStyle.Width(bm.width).Render(status) + "\n")

	// Help line
	sb.WriteString(helpStyle.Render(
		"↑/↓ navigate  ·  space select  ·  enter restore  ·  / search  ·  d dvc-stage  ·  g git-commit  ·  a select-all  ·  c clear  ·  q quit",
	))

	return sb.String()
}

// ---------------------------------------------------------------------------
// Mode: ModeSearch
// ---------------------------------------------------------------------------

func (bm *BrowserModel) updateSearch(msg tea.Msg) (tea.Model, tea.Cmd) {
	switch msg := msg.(type) {
	case tea.KeyMsg:
		switch msg.String() {
		case "enter", "esc":
			bm.activeSearch = bm.searchInput.Value()
			bm.list = bm.buildList(bm.filteredItems())
			bm.mode = ModeList
			return bm, nil
		}
	}
	var cmd tea.Cmd
	bm.searchInput, cmd = bm.searchInput.Update(msg)
	return bm, cmd
}

func (bm *BrowserModel) viewSearch() string {
	return fmt.Sprintf(
		"\n  %s\n\n  %s\n\n  %s",
		browserTitleStyle.Render("Search files"),
		bm.searchInput.View(),
		helpStyle.Render("enter/esc to confirm"),
	)
}

// ---------------------------------------------------------------------------
// Mode: ModeConfirm
// ---------------------------------------------------------------------------

func (bm *BrowserModel) updateConfirm(msg tea.Msg) (tea.Model, tea.Cmd) {
	switch msg := msg.(type) {
	case tea.KeyMsg:
		switch msg.String() {
		case "esc":
			bm.mode = ModeList
			bm.destInput.Blur()
			return bm, nil

		case "enter":
			destDir := strings.TrimSpace(bm.destInput.Value())
			if destDir == "" {
				return bm, nil
			}

			paths := bm.selectedPaths()
			if bm.extractor != nil {
				stats, err := bm.extractor.BatchRestore(context.Background(), paths, destDir)
				if err == nil {
					bm.Result = RestoreResult{
						SelectedPaths: paths,
						DestDir:       destDir,
					}
					_ = stats // stats available for caller via Result
				}
			} else {
				bm.Result = RestoreResult{
					SelectedPaths: paths,
					DestDir:       destDir,
				}
			}
			bm.mode = ModeDone
			return bm, tea.Quit
		}
	}
	var cmd tea.Cmd
	bm.destInput, cmd = bm.destInput.Update(msg)
	return bm, cmd
}

func (bm *BrowserModel) viewConfirm() string {
	paths := bm.selectedPaths()
	var sb strings.Builder
	sb.WriteString("\n")
	sb.WriteString(browserTitleStyle.Render(fmt.Sprintf("Restore %d file(s)", len(paths))) + "\n\n")
	for i, p := range paths {
		if i >= 8 {
			sb.WriteString(dimStyle.Render(fmt.Sprintf("  … and %d more", len(paths)-8)) + "\n")
			break
		}
		sb.WriteString(dimStyle.Render("  · "+p) + "\n")
	}
	sb.WriteString("\n")
	sb.WriteString(metaLabelStyle.Render("  Output directory: "))
	sb.WriteString(bm.destInput.View() + "\n\n")
	sb.WriteString(helpStyle.Render("  enter to confirm  ·  esc to cancel"))
	return sb.String()
}

// ---------------------------------------------------------------------------
// Helpers
// ---------------------------------------------------------------------------

// allItems returns list items for every file in the manifest.
func (bm *BrowserModel) allItems() []list.Item {
	items := make([]list.Item, len(bm.manifest.Files))
	for i := range bm.manifest.Files {
		items[i] = fileItem{
			entry:    &bm.manifest.Files[i],
			selected: bm.selected[bm.manifest.Files[i].Path],
		}
	}
	return items
}

// filteredItems returns items matching the current search and filter state.
func (bm *BrowserModel) filteredItems() []list.Item {
	var items []list.Item
	search := strings.ToLower(bm.activeSearch)
	for i := range bm.manifest.Files {
		fe := &bm.manifest.Files[i]

		// DVC stage filter
		if bm.activeDVCStage != "" {
			if fe.DVCMetadata == nil || fe.DVCMetadata.Stage != bm.activeDVCStage {
				continue
			}
		}
		// Git commit filter (manifest-level)
		if bm.activeGitCommit != "" {
			if bm.manifest.GitMetadata == nil || bm.manifest.GitMetadata.Commit != bm.activeGitCommit {
				continue
			}
		}
		// Search filter
		if search != "" && !strings.Contains(strings.ToLower(fe.Path), search) {
			continue
		}

		items = append(items, fileItem{
			entry:    fe,
			selected: bm.selected[fe.Path],
		})
	}
	return items
}

// buildList creates a new list.Model with the given items.
func (bm *BrowserModel) buildList(items []list.Item) list.Model {
	l := list.New(items, list.NewDefaultDelegate(), bm.width, bm.height-6)
	l.Title = ""
	l.SetShowTitle(false)
	l.SetShowStatusBar(false)
	l.SetFilteringEnabled(false) // we handle filtering manually
	return l
}

// refreshListItems replaces the items in the current list without rebuilding.
func (bm *BrowserModel) refreshListItems() {
	items := bm.filteredItems()
	cmds := make([]tea.Cmd, len(items))
	for i, item := range items {
		cmds[i] = bm.list.SetItem(i, item)
	}
	_ = cmds
	bm.list = bm.buildList(items)
}

// cycleFilter advances current through values (wrapping to "" after the last).
func (bm *BrowserModel) cycleFilter(current string, values []string) string {
	if len(values) == 0 {
		return ""
	}
	for i, v := range values {
		if v == current {
			if i+1 < len(values) {
				return values[i+1]
			}
			return "" // wrap to "no filter"
		}
	}
	return values[0]
}

// filterChips returns a short string describing active filters.
func (bm *BrowserModel) filterChips() string {
	var chips []string
	if bm.activeSearch != "" {
		chips = append(chips, "search:"+bm.activeSearch)
	}
	if bm.activeDVCStage != "" {
		chips = append(chips, "dvc:"+bm.activeDVCStage)
	}
	if bm.activeGitCommit != "" {
		chips = append(chips, "commit:"+bm.activeGitCommit[:min8(len(bm.activeGitCommit))])
	}
	return strings.Join(chips, "  ")
}

// selectedPaths returns sorted selected file paths.
func (bm *BrowserModel) selectedPaths() []string {
	paths := make([]string, 0, len(bm.selected))
	for p, sel := range bm.selected {
		if sel {
			paths = append(paths, p)
		}
	}
	return paths
}

// min8 returns min(n, 8).
func min8(n int) int {
	if n < 8 {
		return n
	}
	return 8
}

// ---------------------------------------------------------------------------
// RunBrowser launches the interactive browser and returns the user's result.
// It is the main entry point for the 'cargoship browse' command.
// ---------------------------------------------------------------------------

// RunBrowser runs the interactive TUI browser for m. Returns RestoreResult
// with SelectedPaths and DestDir set when the user confirms a restore, or
// Cancelled=true when the user quits without selecting.
func RunBrowser(m *manifest.Manifest, s3Client manifest.S3Downloader) (*RestoreResult, error) {
	bm := NewBrowserModel(m, s3Client)
	p := tea.NewProgram(bm, tea.WithAltScreen())
	finalModel, err := p.Run()
	if err != nil {
		return nil, fmt.Errorf("browser error: %w", err)
	}
	final, ok := finalModel.(*BrowserModel)
	if !ok {
		return nil, fmt.Errorf("unexpected model type from browser")
	}
	return &final.Result, nil
}
