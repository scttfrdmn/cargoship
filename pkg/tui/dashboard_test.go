package tui

import (
	"context"
	"strings"
	"testing"
	"time"

	tea "github.com/charmbracelet/bubbletea"

	"github.com/scttfrdmn/cargoship/pkg/aws/cost"
	"github.com/scttfrdmn/cargoship/pkg/resume"
)

func newTestDashboard() *Dashboard {
	return NewDashboard(context.Background(), nil, DashboardOverview, time.Second, nil)
}

// TestDashboard_UpdateSwitchesViews checks the real key handling: number keys and
// tab move between the three views.
func TestDashboard_UpdateSwitchesViews(t *testing.T) {
	d := newTestDashboard()
	key := func(s string) tea.KeyMsg { return tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune(s)} }

	if m, _ := d.Update(key("2")); m.(*Dashboard).currentView != DashboardCosts {
		t.Errorf("key 2 should select Costs, got %v", m.(*Dashboard).currentView)
	}
	if m, _ := d.Update(key("3")); m.(*Dashboard).currentView != DashboardUploads {
		t.Errorf("key 3 should select Uploads, got %v", m.(*Dashboard).currentView)
	}
	// tab wraps Uploads → Overview.
	if m, _ := d.Update(tea.KeyMsg{Type: tea.KeyTab}); m.(*Dashboard).currentView != DashboardOverview {
		t.Errorf("tab from Uploads should wrap to Overview, got %v", m.(*Dashboard).currentView)
	}
	// q quits.
	if _, cmd := d.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune("q")}); cmd == nil {
		t.Error("q should return a quit command")
	}
}

// TestDashboard_UpdateDataMsg checks that a data snapshot populates the tables via
// the real mappers.
func TestDashboard_UpdateDataMsg(t *testing.T) {
	d := newTestDashboard()
	m, _ := d.Update(dataMsg{
		summary: &cost.CostSummary{TotalCost: 5, ByStorageClass: map[string]float64{"STANDARD": 5}},
		uploads: []*resume.UploadState{{UploadID: "u1", Bucket: "b", Prefix: "p", TotalBytes: 10, CompletedBytes: 5, StartTime: time.Now()}},
	})
	d = m.(*Dashboard)
	if got := len(d.costTable.Rows()); got != 2 { // STANDARD + TOTAL
		t.Errorf("cost rows = %d, want 2", got)
	}
	if got := len(d.uploadTable.Rows()); got != 1 {
		t.Errorf("upload rows = %d, want 1", got)
	}
}

// TestDashboard_ViewHonestWhenEmpty confirms the View surfaces honest empty/
// unavailable states rather than fabricated numbers (nil cost provider, no data).
func TestDashboard_ViewHonestWhenEmpty(t *testing.T) {
	d := newTestDashboard()
	d.Update(dataMsg{}) // no data
	d.currentView = DashboardCosts
	if out := d.View(); !strings.Contains(out, "unavailable") {
		t.Errorf("Costs view with nil provider should say unavailable, got:\n%s", out)
	}
	d.currentView = DashboardUploads
	if out := d.View(); !strings.Contains(out, "no in-progress uploads") {
		t.Errorf("Uploads view with no data should say none, got:\n%s", out)
	}
}

// TestCostRows_Real maps a real cost summary into rows (per storage class + total).
func TestCostRows_Real(t *testing.T) {
	rows := costRows(&cost.CostSummary{
		TotalCost:      12.50,
		ByStorageClass: map[string]float64{"GLACIER": 2.50, "STANDARD": 10.00},
	})
	// 2 classes (sorted) + TOTAL.
	if len(rows) != 3 {
		t.Fatalf("got %d rows, want 3", len(rows))
	}
	if rows[0][0] != "GLACIER" || rows[1][0] != "STANDARD" {
		t.Errorf("classes not sorted: %v, %v", rows[0][0], rows[1][0])
	}
	if rows[2][0] != "TOTAL" || rows[2][1] != "$12.50" {
		t.Errorf("total row = %v, want [TOTAL $12.50]", rows[2])
	}
}

// TestCostRows_EmptyIsHonest guards the anti-theater property: no data yields a
// truthful "no data" row, never a fabricated figure.
func TestCostRows_EmptyIsHonest(t *testing.T) {
	for _, s := range []*cost.CostSummary{nil, {ByStorageClass: map[string]float64{}}} {
		rows := costRows(s)
		if len(rows) != 1 {
			t.Fatalf("empty summary should yield 1 row, got %d", len(rows))
		}
		if rows[0][0] == "" || rows[0][1] != "" {
			t.Errorf("empty row should be an honest label with no cost, got %v", rows[0])
		}
	}
}

// TestUploadRows_Real maps in-progress upload states into rows with real fields.
func TestUploadRows_Real(t *testing.T) {
	states := []*resume.UploadState{{
		UploadID:       "20260101-abc",
		SourceDir:      "/data/src",
		Bucket:         "bucket",
		Prefix:         "prefix",
		TotalBytes:     1000,
		CompletedBytes: 250,
		StartTime:      time.Now().Add(-90 * time.Second),
	}}
	rows := uploadRows(states)
	if len(rows) != 1 {
		t.Fatalf("got %d rows, want 1", len(rows))
	}
	r := rows[0]
	if r[0] != "20260101-abc" || r[1] != "/data/src" || r[2] != "s3://bucket/prefix" {
		t.Errorf("fields not mapped: %v", r)
	}
	if r[3] != "25%" {
		t.Errorf("progress = %q, want 25%%", r[3])
	}
}

// TestUploadRows_EmptyIsHonest: no uploads → a truthful "none" row.
func TestUploadRows_EmptyIsHonest(t *testing.T) {
	rows := uploadRows(nil)
	if len(rows) != 1 || rows[0][0] == "" {
		t.Fatalf("empty uploads should yield one honest row, got %v", rows)
	}
}

// TestBudgetFloat exercises the budget-status accessor's presence/type handling.
func TestBudgetFloat(t *testing.T) {
	m := map[string]interface{}{"current_spend": 4.5, "period_type": "monthly"}
	if v, ok := budgetFloat(m, "current_spend"); !ok || v != 4.5 {
		t.Errorf("current_spend = %v,%v want 4.5,true", v, ok)
	}
	if _, ok := budgetFloat(m, "period_type"); ok {
		t.Error("non-float value should report ok=false")
	}
	if _, ok := budgetFloat(nil, "x"); ok {
		t.Error("nil map should report ok=false")
	}
}
