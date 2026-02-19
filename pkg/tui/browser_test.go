package tui

import (
	"fmt"
	"testing"
	"time"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/scttfrdmn/cargoship/pkg/manifest"
)

// ---------------------------------------------------------------------------
// Fixtures
// ---------------------------------------------------------------------------

func buildBrowserManifest() *manifest.Manifest {
	m := &manifest.Manifest{
		Version:         "2.0",
		UploadID:        "browser-test-001",
		TotalFiles:      6,
		CompressionType: "zstd",
		GitMetadata:     &manifest.GitMetadata{Commit: "abc1234"},
		Files: []manifest.FileEntry{
			{Path: "data/train.csv", Size: 1024, ModTime: time.Now(), ContentHash: "aaa", DVCMetadata: &manifest.DVCMetadata{Stage: "preprocess"}},
			{Path: "data/test.csv", Size: 512, ModTime: time.Now(), ContentHash: "bbb", DVCMetadata: &manifest.DVCMetadata{Stage: "preprocess"}},
			{Path: "models/model.pkl", Size: 2048, ModTime: time.Now(), ContentHash: "ccc", DVCMetadata: &manifest.DVCMetadata{Stage: "train"}},
			{Path: "reports/metrics.json", Size: 256, ModTime: time.Now(), ContentHash: "ddd", DVCMetadata: &manifest.DVCMetadata{Stage: "evaluate"}},
			{Path: "README.md", Size: 512, ModTime: time.Now()},
			{Path: "scripts/run.sh", Size: 128, ModTime: time.Now()},
		},
	}
	return m
}

// ---------------------------------------------------------------------------
// NewBrowserModel
// ---------------------------------------------------------------------------

func TestNewBrowserModel_BasicInit(t *testing.T) {
	m := buildBrowserManifest()
	bm := NewBrowserModel(m, nil)

	require.NotNil(t, bm)
	assert.Equal(t, ModeList, bm.mode)
	assert.NotNil(t, bm.query)
	assert.NotNil(t, bm.selected)
	assert.Nil(t, bm.extractor) // no S3 client provided
}

func TestNewBrowserModel_DVCStagesCollected(t *testing.T) {
	m := buildBrowserManifest()
	bm := NewBrowserModel(m, nil)

	assert.ElementsMatch(t, []string{"preprocess", "train", "evaluate"}, bm.dvcStages)
}

func TestNewBrowserModel_GitCommitsCollected(t *testing.T) {
	m := buildBrowserManifest()
	bm := NewBrowserModel(m, nil)

	require.Len(t, bm.gitCommits, 1)
	assert.Equal(t, "abc1234", bm.gitCommits[0])
}

func TestNewBrowserModel_NoGitMetadata(t *testing.T) {
	m := buildBrowserManifest()
	m.GitMetadata = nil
	bm := NewBrowserModel(m, nil)
	assert.Empty(t, bm.gitCommits)
}

// ---------------------------------------------------------------------------
// fileItem
// ---------------------------------------------------------------------------

func TestFileItem_Title_Unselected(t *testing.T) {
	fe := &manifest.FileEntry{Path: "data/train.csv", Size: 1024}
	item := fileItem{entry: fe, selected: false}
	assert.Equal(t, "data/train.csv", item.Title())
}

func TestFileItem_Title_Selected(t *testing.T) {
	fe := &manifest.FileEntry{Path: "data/train.csv", Size: 1024}
	item := fileItem{entry: fe, selected: true}
	assert.Contains(t, item.Title(), "✓")
	assert.Contains(t, item.Title(), "data/train.csv")
}

func TestFileItem_Description_WithHash(t *testing.T) {
	fe := &manifest.FileEntry{
		Path:        "models/model.pkl",
		Size:        2048,
		ContentHash: "abcdef1234567890",
		DVCMetadata: &manifest.DVCMetadata{Stage: "train"},
	}
	item := fileItem{entry: fe}
	desc := item.Description()
	assert.Contains(t, desc, "abcdef12")
	assert.Contains(t, desc, "stage:train")
}

func TestFileItem_FilterValue(t *testing.T) {
	fe := &manifest.FileEntry{Path: "data/train.csv"}
	item := fileItem{entry: fe}
	assert.Equal(t, "data/train.csv", item.FilterValue())
}

// ---------------------------------------------------------------------------
// filteredItems
// ---------------------------------------------------------------------------

func TestBrowserModel_FilteredItems_NoFilter(t *testing.T) {
	m := buildBrowserManifest()
	bm := NewBrowserModel(m, nil)
	items := bm.filteredItems()
	assert.Len(t, items, len(m.Files))
}

func TestBrowserModel_FilteredItems_DVCStage(t *testing.T) {
	m := buildBrowserManifest()
	bm := NewBrowserModel(m, nil)
	bm.activeDVCStage = "preprocess"
	items := bm.filteredItems()
	// 2 files have stage=preprocess
	assert.Len(t, items, 2)
	for _, item := range items {
		fi := item.(fileItem)
		require.NotNil(t, fi.entry.DVCMetadata)
		assert.Equal(t, "preprocess", fi.entry.DVCMetadata.Stage)
	}
}

func TestBrowserModel_FilteredItems_Search(t *testing.T) {
	m := buildBrowserManifest()
	bm := NewBrowserModel(m, nil)
	bm.activeSearch = "csv"
	items := bm.filteredItems()
	assert.Len(t, items, 2) // train.csv + test.csv
}

func TestBrowserModel_FilteredItems_SearchCaseInsensitive(t *testing.T) {
	m := buildBrowserManifest()
	bm := NewBrowserModel(m, nil)
	bm.activeSearch = "README"
	items := bm.filteredItems()
	assert.Len(t, items, 1)
}

func TestBrowserModel_FilteredItems_SearchNoMatch(t *testing.T) {
	m := buildBrowserManifest()
	bm := NewBrowserModel(m, nil)
	bm.activeSearch = "zzznomatch"
	items := bm.filteredItems()
	assert.Len(t, items, 0)
}

// ---------------------------------------------------------------------------
// cycleFilter
// ---------------------------------------------------------------------------

func TestBrowserModel_CycleFilter_Empty(t *testing.T) {
	bm := &BrowserModel{}
	assert.Equal(t, "", bm.cycleFilter("", nil))
}

func TestBrowserModel_CycleFilter_Advances(t *testing.T) {
	bm := &BrowserModel{}
	values := []string{"a", "b", "c"}
	assert.Equal(t, "a", bm.cycleFilter("", values))
	assert.Equal(t, "b", bm.cycleFilter("a", values))
	assert.Equal(t, "c", bm.cycleFilter("b", values))
	assert.Equal(t, "", bm.cycleFilter("c", values)) // wraps to no filter
}

// ---------------------------------------------------------------------------
// Key event handling — model logic
// ---------------------------------------------------------------------------

func TestBrowserModel_SpaceTogglesSelection(t *testing.T) {
	m := buildBrowserManifest()
	bm := NewBrowserModel(m, nil)

	// Simulate space key on the first item.
	msg := tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{' '}}
	newModel, _ := bm.Update(msg)
	updated := newModel.(*BrowserModel)

	selectedCount := 0
	for _, sel := range updated.selected {
		if sel {
			selectedCount++
		}
	}
	assert.Equal(t, 1, selectedCount, "space should select one item")

	// Press space again to deselect.
	newModel2, _ := updated.Update(msg)
	updated2 := newModel2.(*BrowserModel)
	selectedCount2 := 0
	for _, sel := range updated2.selected {
		if sel {
			selectedCount2++
		}
	}
	assert.Equal(t, 0, selectedCount2, "second space should deselect")
}

func TestBrowserModel_QuitSetsMode(t *testing.T) {
	m := buildBrowserManifest()
	bm := NewBrowserModel(m, nil)

	msg := tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'q'}}
	newModel, _ := bm.Update(msg)
	updated := newModel.(*BrowserModel)

	assert.Equal(t, ModeDone, updated.mode)
	assert.True(t, updated.Result.Cancelled)
}

func TestBrowserModel_SlashEntersSearchMode(t *testing.T) {
	m := buildBrowserManifest()
	bm := NewBrowserModel(m, nil)

	msg := tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'/'}}
	newModel, _ := bm.Update(msg)
	updated := newModel.(*BrowserModel)

	assert.Equal(t, ModeSearch, updated.mode)
}

func TestBrowserModel_SelectAllAndClear(t *testing.T) {
	m := buildBrowserManifest()
	bm := NewBrowserModel(m, nil)

	// 'a' selects all.
	newModel, _ := bm.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'a'}})
	updated := newModel.(*BrowserModel)

	count := 0
	for _, sel := range updated.selected {
		if sel {
			count++
		}
	}
	assert.Equal(t, len(m.Files), count, "all files should be selected")

	// 'c' clears.
	newModel2, _ := updated.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'c'}})
	updated2 := newModel2.(*BrowserModel)
	for _, sel := range updated2.selected {
		assert.False(t, sel)
	}
}

func TestBrowserModel_DVCStageFilterKey(t *testing.T) {
	m := buildBrowserManifest()
	bm := NewBrowserModel(m, nil)

	// 'd' cycles the DVC stage filter.
	newModel, _ := bm.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'d'}})
	updated := newModel.(*BrowserModel)
	assert.NotEmpty(t, updated.activeDVCStage)
	assert.Less(t, len(updated.list.Items()), len(m.Files))
}

// ---------------------------------------------------------------------------
// View
// ---------------------------------------------------------------------------

func TestBrowserModel_ViewContainsUploadID(t *testing.T) {
	m := buildBrowserManifest()
	bm := NewBrowserModel(m, nil)
	view := bm.View()
	assert.Contains(t, view, "browser-test-001")
}

func TestBrowserModel_ViewConfirm(t *testing.T) {
	m := buildBrowserManifest()
	bm := NewBrowserModel(m, nil)
	bm.mode = ModeConfirm
	bm.selected["data/train.csv"] = true
	view := bm.View()
	assert.Contains(t, view, "Restore")
	assert.Contains(t, view, "data/train.csv")
}

// ---------------------------------------------------------------------------
// allItems / selectedPaths
// ---------------------------------------------------------------------------

func TestBrowserModel_AllItemsCount(t *testing.T) {
	m := buildBrowserManifest()
	bm := NewBrowserModel(m, nil)
	items := bm.allItems()
	assert.Len(t, items, len(m.Files))
}

func TestBrowserModel_SelectedPaths(t *testing.T) {
	m := buildBrowserManifest()
	bm := NewBrowserModel(m, nil)
	bm.selected["data/train.csv"] = true
	bm.selected["README.md"] = true
	bm.selected["models/model.pkl"] = false // explicitly not selected

	paths := bm.selectedPaths()
	assert.Len(t, paths, 2)
	assert.ElementsMatch(t, []string{"data/train.csv", "README.md"}, paths)
}

// ---------------------------------------------------------------------------
// filterChips
// ---------------------------------------------------------------------------

func TestBrowserModel_FilterChips_None(t *testing.T) {
	bm := &BrowserModel{}
	assert.Empty(t, bm.filterChips())
}

func TestBrowserModel_FilterChips_Combined(t *testing.T) {
	bm := &BrowserModel{
		activeSearch:    "csv",
		activeDVCStage:  "preprocess",
		activeGitCommit: "abc1234567890",
	}
	chips := bm.filterChips()
	assert.Contains(t, chips, "search:csv")
	assert.Contains(t, chips, "dvc:preprocess")
	assert.Contains(t, chips, "commit:abc12345")
}

// ---------------------------------------------------------------------------
// min8
// ---------------------------------------------------------------------------

func TestMin8(t *testing.T) {
	assert.Equal(t, 0, min8(0))
	assert.Equal(t, 5, min8(5))
	assert.Equal(t, 8, min8(8))
	assert.Equal(t, 8, min8(100))
}

// ---------------------------------------------------------------------------
// Large manifest benchmark
// ---------------------------------------------------------------------------

func BenchmarkNewBrowserModel_10kFiles(b *testing.B) {
	files := make([]manifest.FileEntry, 10_000)
	for i := range files {
		files[i] = manifest.FileEntry{
			Path:        fmt.Sprintf("data/file-%05d.bin", i),
			Size:        int64(i * 100),
			ContentHash: fmt.Sprintf("%032x", i),
		}
		if i%10 == 0 {
			files[i].DVCMetadata = &manifest.DVCMetadata{Stage: "stage"}
		}
	}
	m := &manifest.Manifest{
		UploadID:   "bench",
		TotalFiles: int64(len(files)),
		Files:      files,
	}
	b.ResetTimer()
	for range b.N {
		_ = NewBrowserModel(m, nil)
	}
}
