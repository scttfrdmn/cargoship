package tui

import (
	"context"
	"io"
	"log/slog"
	"strings"
	"testing"
	"time"

	tea "github.com/charmbracelet/bubbletea"
)

// bubbles v1.0.0 is a MAJOR bump, and this package drives list, table and
// textinput. Compilation proves the signatures still line up; it proves nothing
// about rendering. A TUI that compiles and draws nothing is a silent regression
// nobody notices until they open the dashboard, so drive the model headlessly and
// assert it produces output.
func TestDashboardRendersAfterBubblesV1(t *testing.T) {
	d := NewDashboard(
		context.Background(),
		nil, nil, nil, nil, // providers are nil-able by design
		"s3://example/prefix",
		DashboardType(0),
		time.Minute,
		slog.New(slog.NewTextHandler(io.Discard, nil)),
	)

	// A window size is what makes bubbles' table and list compute their viewport.
	// Without it they legitimately render empty, so this is the minimum realistic
	// input rather than a contrivance.
	m, _ := d.Update(tea.WindowSizeMsg{Width: 120, Height: 40})
	out := m.View()
	if strings.TrimSpace(out) == "" {
		t.Fatal("dashboard rendered nothing after a WindowSizeMsg; bubbles/bubbletea rendering regressed")
	}
	if lines := strings.Count(out, "\n") + 1; lines < 5 {
		t.Errorf("dashboard rendered only %d line(s); expected a full-screen view", lines)
	}
	t.Logf("dashboard rendered %d bytes across %d lines", len(out), strings.Count(out, "\n")+1)
}
