package pipeline

import (
	"context"
	"fmt"
	"sync"
	"time"

	"github.com/charmbracelet/bubbles/progress"
	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"
	"github.com/dustin/go-humanize"
)

// ShardProgressRenderer tracks upload progress and renders a beautiful TUI
type ShardProgressRenderer struct {
	coordinator   *ShardCoordinator
	ctx           context.Context
	cancel        context.CancelFunc
	wg            sync.WaitGroup
	program       *tea.Program
	updateTicker  *time.Ticker
	startTime     time.Time
	estimatedSize int64 // Estimated total size for ETA calculation
}

// NewShardProgressRenderer creates a new progress renderer for a shard coordinator
func NewShardProgressRenderer(ctx context.Context, coordinator *ShardCoordinator, estimatedSize int64) *ShardProgressRenderer {
	ctx, cancel := context.WithCancel(ctx)
	return &ShardProgressRenderer{
		coordinator:   coordinator,
		ctx:           ctx,
		cancel:        cancel,
		startTime:     time.Now(),
		estimatedSize: estimatedSize,
	}
}

// Start begins rendering progress updates
func (pt *ShardProgressRenderer) Start() error {
	// Create bubbletea program
	model := newProgressModel(pt)
	pt.program = tea.NewProgram(model)

	// Start update ticker (every 100ms for smooth animation)
	pt.updateTicker = time.NewTicker(100 * time.Millisecond)

	// Start update goroutine
	pt.wg.Add(1)
	go func() {
		defer pt.wg.Done()
		defer pt.updateTicker.Stop()

		for {
			select {
			case <-pt.ctx.Done():
				return
			case <-pt.updateTicker.C:
				if pt.program != nil {
					pt.program.Send(tickMsg{})
				}
			}
		}
	}()

	// Run the TUI (blocks until completion)
	if _, err := pt.program.Run(); err != nil {
		return fmt.Errorf("failed to run progress TUI: %w", err)
	}

	return nil
}

// Stop stops the progress renderer
func (pt *ShardProgressRenderer) Stop() {
	if pt.cancel != nil {
		pt.cancel()
	}
	if pt.program != nil {
		pt.program.Quit()
	}
	pt.wg.Wait()
}

// tickMsg is sent periodically to update the UI
type tickMsg struct{}

// progressModel is the bubbletea model for progress rendering
type progressModel struct {
	tracker        *ShardProgressRenderer
	shardProgress  []progress.Model
	width          int
	height         int
	lastStats      ShardCoordinatorStats
	lastUpdateTime time.Time
}

func newProgressModel(tracker *ShardProgressRenderer) progressModel {
	stats := tracker.coordinator.GetStats()
	shardProgress := make([]progress.Model, stats.ShardCount)
	for i := 0; i < stats.ShardCount; i++ {
		p := progress.New(progress.WithDefaultGradient())
		p.Width = 40
		shardProgress[i] = p
	}

	return progressModel{
		tracker:        tracker,
		shardProgress:  shardProgress,
		width:          80,
		height:         25,
		lastUpdateTime: time.Now(),
	}
}

func (m progressModel) Init() tea.Cmd {
	return nil
}

func (m progressModel) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
	switch msg := msg.(type) {
	case tea.KeyMsg:
		switch msg.String() {
		case "ctrl+c", "q":
			return m, tea.Quit
		case "p", "P":
			// Pause all shard pipelines (Issue #112)
			if !m.tracker.coordinator.IsPaused() {
				m.tracker.coordinator.Pause()
			}
		case "r", "R":
			// Resume all shard pipelines (Issue #112)
			if m.tracker.coordinator.IsPaused() {
				m.tracker.coordinator.Resume()
			}
		}

	case tea.WindowSizeMsg:
		m.width = msg.Width
		m.height = msg.Height
		// Update progress bar widths
		progressWidth := minInt(60, m.width-40)
		for i := range m.shardProgress {
			m.shardProgress[i].Width = progressWidth
		}

	case tickMsg:
		// Update statistics
		m.lastStats = m.tracker.coordinator.GetStats()
		m.lastUpdateTime = time.Now()

		// Check if complete
		if m.lastStats.IsComplete() {
			// Wait a moment to show final state, then quit
			return m, tea.Sequence(
				tea.Tick(500*time.Millisecond, func(time.Time) tea.Msg {
					return tea.Quit()
				}),
			)
		}
	}

	return m, nil
}

func (m progressModel) View() string {
	stats := m.lastStats

	// Header
	titleStyle := lipgloss.NewStyle().
		Bold(true).
		Foreground(lipgloss.Color("86")). // Cyan
		MarginBottom(1)

	var header string
	if stats.IsComplete() {
		if stats.HasErrors() {
			header = titleStyle.Render("✗ Upload completed with errors")
		} else {
			header = titleStyle.Render("✓ Upload completed successfully")
		}
	} else {
		// Check if paused (Issue #112)
		if m.tracker.coordinator.IsPaused() {
			pausedStyle := lipgloss.NewStyle().
				Bold(true).
				Foreground(lipgloss.Color("226")). // Yellow
				MarginBottom(1)
			header = pausedStyle.Render("⏸  PAUSED - Press [R] to resume")
		} else {
			header = titleStyle.Render("🚢 Uploading to S3...")
		}
	}

	// Per-shard progress bars
	shardStyle := lipgloss.NewStyle().
		Foreground(lipgloss.Color("240"))
	valueStyle := lipgloss.NewStyle().
		Foreground(lipgloss.Color("255")).
		Bold(true)

	var shardViews []string
	for i, shardStat := range stats.ShardStats {
		// Calculate progress percentage
		var progressPct float64
		if m.tracker.estimatedSize > 0 {
			// Use estimated size for more accurate progress
			expectedBytesPerShard := m.tracker.estimatedSize / int64(stats.ShardCount)
			if expectedBytesPerShard > 0 {
				progressPct = float64(shardStat.BytesProcessed) / float64(expectedBytesPerShard)
			}
		} else if shardStat.FilesAdded > 0 {
			// Fallback: assume progress based on completion status
			if shardStat.Completed {
				progressPct = 1.0
			} else {
				progressPct = 0.5 // Assume 50% if in progress
			}
		}

		// Clamp to [0, 1]
		progressPct = maxFloat(0.0, minFloat(1.0, progressPct))

		// Render progress bar
		progressBar := m.shardProgress[i].ViewAs(progressPct)

		// Shard stats
		filesStr := humanize.Comma(shardStat.FilesAdded)
		bytesStr := humanize.IBytes(uint64(shardStat.BytesProcessed))

		// Calculate throughput (MB/s)
		var throughputStr string
		if shardStat.Duration.Seconds() > 0 {
			throughputMBps := float64(shardStat.BytesProcessed) / (1 << 20) / shardStat.Duration.Seconds()
			throughputStr = fmt.Sprintf("%.1f MB/s", throughputMBps)
		} else {
			throughputStr = "-- MB/s"
		}

		// Status indicator
		statusIcon := "●"
		statusColor := "226" // Yellow (in progress)
		if shardStat.Completed {
			if shardStat.Error != nil {
				statusIcon = "✗"
				statusColor = "196" // Red (error)
			} else {
				statusIcon = "✓"
				statusColor = "82" // Green (complete)
			}
		}
		statusStyle := lipgloss.NewStyle().Foreground(lipgloss.Color(statusColor))

		// Worker count display (Issue #112)
		workerStr := fmt.Sprintf("(%dw)", shardStat.WorkerCount)
		if shardStat.WorkerCount == 0 {
			workerStr = "(--)"
		}

		shardLine := fmt.Sprintf("%s %s %s %s %s %s %s %s",
			statusStyle.Render(statusIcon),
			shardStyle.Render(fmt.Sprintf("Shard %d:", i)),
			progressBar,
			valueStyle.Render(filesStr),
			shardStyle.Render("files"),
			valueStyle.Render(bytesStr),
			shardStyle.Render(throughputStr),
			valueStyle.Render(workerStr),
		)
		shardViews = append(shardViews, shardLine)
	}

	// Aggregate statistics
	aggregateStyle := lipgloss.NewStyle().
		Foreground(lipgloss.Color("255")).
		Bold(true).
		MarginTop(1)

	totalFiles := humanize.Comma(stats.FilesAdded)
	totalBytes := humanize.IBytes(uint64(stats.BytesProcessed))
	uploadSize := humanize.IBytes(uint64(stats.TotalUploadSize))
	compressionRatio := stats.CompressionRatio()
	compressionPct := (1.0 - compressionRatio) * 100

	var aggregateLine string
	if stats.Duration.Seconds() > 0 {
		processingThroughput := stats.ThroughputMBps()
		networkThroughput := stats.NetworkThroughputMBps()

		aggregateLine = aggregateStyle.Render(fmt.Sprintf(
			"Total: %s files | %s → %s (%.1f%% compression) | %.1f MB/s processing | %.1f MB/s network",
			totalFiles,
			totalBytes,
			uploadSize,
			compressionPct,
			processingThroughput,
			networkThroughput,
		))
	} else {
		aggregateLine = aggregateStyle.Render(fmt.Sprintf(
			"Total: %s files | %s",
			totalFiles,
			totalBytes,
		))
	}

	// ETA calculation
	var etaLine string
	if !stats.IsComplete() && stats.BytesProcessed > 0 && m.tracker.estimatedSize > 0 {
		progressPct := float64(stats.BytesProcessed) / float64(m.tracker.estimatedSize)
		if progressPct > 0 && progressPct < 1.0 {
			elapsedSeconds := stats.Duration.Seconds()
			totalEstimatedSeconds := elapsedSeconds / progressPct
			remainingSeconds := totalEstimatedSeconds - elapsedSeconds

			etaDuration := time.Duration(remainingSeconds) * time.Second
			etaStyle := lipgloss.NewStyle().
				Foreground(lipgloss.Color("240"))
			etaLine = etaStyle.Render(fmt.Sprintf("ETA: %s", etaDuration.Round(time.Second)))
		}
	}

	// Combine all sections
	var sections []string
	sections = append(sections, header)
	sections = append(sections, shardViews...)
	sections = append(sections, aggregateLine)
	if etaLine != "" {
		sections = append(sections, etaLine)
	}

	// Add footer with instructions (only if not complete)
	if !stats.IsComplete() {
		footerStyle := lipgloss.NewStyle().
			Foreground(lipgloss.Color("240")).
			MarginTop(1)
		footer := footerStyle.Render("[P]ause [R]esume [Q]uit")
		sections = append(sections, footer)
	}

	return lipgloss.JoinVertical(lipgloss.Left, sections...)
}

// RenderOnce renders a single snapshot of progress (non-interactive)
func RenderOnce(coordinator *ShardCoordinator, estimatedSize int64, startTime time.Time) string {
	tracker := &ShardProgressRenderer{
		coordinator:   coordinator,
		startTime:     startTime,
		estimatedSize: estimatedSize,
	}
	model := newProgressModel(tracker)
	model.lastStats = coordinator.GetStats()
	return model.View()
}

func minInt(a, b int) int {
	if a < b {
		return a
	}
	return b
}

func minFloat(a, b float64) float64 {
	if a < b {
		return a
	}
	return b
}

func maxFloat(a, b float64) float64 {
	if a > b {
		return a
	}
	return b
}
