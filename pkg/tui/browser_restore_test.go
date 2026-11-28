package tui

import (
	"os"
	"testing"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// TestBrowserModel_ConfirmOnlyCollectsTheSelection pins #972. Confirming in the
// TUI ran a restore of its own — from the bucket recorded in the manifest, with
// no existing-file policy or Glacier pre-flight — and discarded its stats and
// error; browse then restored the same files again, which the default refuse
// policy always failed. The model now has no S3 client at all and only reports
// what was chosen and where.
func TestBrowserModel_ConfirmOnlyCollectsTheSelection(t *testing.T) {
	bm := NewBrowserModel(buildBrowserManifest())
	bm.selected["data/train.csv"] = true
	bm.mode = ModeConfirm
	dest := t.TempDir()
	bm.destInput.SetValue(dest)

	_, cmd := bm.Update(tea.KeyMsg{Type: tea.KeyEnter})
	require.NotNil(t, cmd, "confirming quits the browser")

	assert.Equal(t, RestoreResult{SelectedPaths: []string{"data/train.csv"}, DestDir: dest}, bm.Result)
	assert.Equal(t, ModeDone, bm.mode)
	entries, err := os.ReadDir(dest)
	require.NoError(t, err)
	assert.Empty(t, entries, "nothing may be written before browse's own restore")
}

// TestBrowserModel_SetDestination: browse's OUTPUT_DIR argument pre-fills the
// prompt (it was accepted and ignored), and confirming takes it as typed.
func TestBrowserModel_SetDestination(t *testing.T) {
	bm := NewBrowserModel(buildBrowserManifest())
	bm.SetDestination("/restore/here")
	bm.selected["README.md"] = true
	bm.mode = ModeConfirm
	_, _ = bm.Update(tea.KeyMsg{Type: tea.KeyEnter})
	assert.Equal(t, "/restore/here", bm.Result.DestDir)
	assert.Equal(t, []string{"README.md"}, bm.Result.SelectedPaths)
}

// TestBrowserModel_ConfirmNeedsADestination: enter on an empty prompt does
// nothing, so the caller never receives a selection without a destination.
func TestBrowserModel_ConfirmNeedsADestination(t *testing.T) {
	bm := NewBrowserModel(buildBrowserManifest())
	bm.selected["README.md"] = true
	bm.mode = ModeConfirm
	_, cmd := bm.Update(tea.KeyMsg{Type: tea.KeyEnter})
	assert.Nil(t, cmd)
	assert.Equal(t, ModeConfirm, bm.mode)
	assert.Empty(t, bm.Result.SelectedPaths)
}
