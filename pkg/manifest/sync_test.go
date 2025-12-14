package manifest

import (
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// TestComputeDelta_NewFiles tests detecting new files not in previous manifest (Issue #148)
func TestComputeDelta_NewFiles(t *testing.T) {
	now := time.Now()

	// Previous manifest with 2 files
	previousManifest := &Manifest{
		Files: []FileEntry{
			{Path: "file1.txt", Size: 100, ModTime: now.Add(-1 * time.Hour)},
			{Path: "file2.txt", Size: 200, ModTime: now.Add(-1 * time.Hour)},
		},
	}

	// Local filesystem has 3 files (file3.txt is new)
	localFiles := []FileInfo{
		{Path: "file1.txt", Size: 100, ModTime: now.Add(-1 * time.Hour), IsDir: false},
		{Path: "file2.txt", Size: 200, ModTime: now.Add(-1 * time.Hour), IsDir: false},
		{Path: "file3.txt", Size: 300, ModTime: now, IsDir: false}, // NEW
	}

	delta, err := ComputeDelta(localFiles, previousManifest, nil)
	require.NoError(t, err)

	assert.Equal(t, 1, len(delta.New), "Should detect 1 new file")
	assert.Equal(t, "file3.txt", delta.New[0].Path)
	assert.Equal(t, 2, len(delta.Same), "2 files unchanged")
	assert.Equal(t, 0, len(delta.Modified), "No modified files")
}

// TestComputeDelta_ModifiedFiles tests detecting modified files (Issue #148)
func TestComputeDelta_ModifiedFiles(t *testing.T) {
	now := time.Now()

	// Previous manifest
	previousManifest := &Manifest{
		Files: []FileEntry{
			{Path: "file1.txt", Size: 100, ModTime: now.Add(-2 * time.Hour)},
			{Path: "file2.txt", Size: 200, ModTime: now.Add(-2 * time.Hour)},
		},
	}

	// Local filesystem - file1 has newer mtime, file2 has different size
	localFiles := []FileInfo{
		{Path: "file1.txt", Size: 100, ModTime: now.Add(-1 * time.Hour), IsDir: false}, // MODIFIED (mtime)
		{Path: "file2.txt", Size: 250, ModTime: now.Add(-2 * time.Hour), IsDir: false}, // MODIFIED (size)
	}

	delta, err := ComputeDelta(localFiles, previousManifest, nil)
	require.NoError(t, err)

	assert.Equal(t, 2, len(delta.Modified), "Should detect 2 modified files")
	assert.Equal(t, 0, len(delta.New), "No new files")
	assert.Equal(t, 0, len(delta.Same), "No unchanged files")

	// Verify both files detected as modified
	modifiedPaths := []string{delta.Modified[0].Path, delta.Modified[1].Path}
	assert.Contains(t, modifiedPaths, "file1.txt")
	assert.Contains(t, modifiedPaths, "file2.txt")
}

// TestComputeDelta_DeletedFiles tests detecting deleted files (Issue #148)
func TestComputeDelta_DeletedFiles(t *testing.T) {
	now := time.Now()

	// Previous manifest with 3 files
	previousManifest := &Manifest{
		Files: []FileEntry{
			{Path: "file1.txt", Size: 100, ModTime: now},
			{Path: "file2.txt", Size: 200, ModTime: now},
			{Path: "file3.txt", Size: 300, ModTime: now},
		},
	}

	// Local filesystem only has 2 files (file3.txt deleted)
	localFiles := []FileInfo{
		{Path: "file1.txt", Size: 100, ModTime: now, IsDir: false},
		{Path: "file2.txt", Size: 200, ModTime: now, IsDir: false},
	}

	// Test with TrackDeletes enabled
	opts := &SyncOptions{TrackDeletes: true}
	delta, err := ComputeDelta(localFiles, previousManifest, opts)
	require.NoError(t, err)

	assert.Equal(t, 1, len(delta.Deleted), "Should detect 1 deleted file")
	assert.Equal(t, "file3.txt", delta.Deleted[0])
	assert.Equal(t, 2, len(delta.Same), "2 files unchanged")

	// Test with TrackDeletes disabled (default)
	delta, err = ComputeDelta(localFiles, previousManifest, nil)
	require.NoError(t, err)

	assert.Equal(t, 0, len(delta.Deleted), "Should not track deletes by default")
}

// TestComputeDelta_NoChanges tests when no files changed (Issue #148)
func TestComputeDelta_NoChanges(t *testing.T) {
	now := time.Now()

	// Previous manifest
	previousManifest := &Manifest{
		Files: []FileEntry{
			{Path: "file1.txt", Size: 100, ModTime: now},
			{Path: "file2.txt", Size: 200, ModTime: now},
		},
	}

	// Local filesystem identical to manifest
	localFiles := []FileInfo{
		{Path: "file1.txt", Size: 100, ModTime: now, IsDir: false},
		{Path: "file2.txt", Size: 200, ModTime: now, IsDir: false},
	}

	delta, err := ComputeDelta(localFiles, previousManifest, nil)
	require.NoError(t, err)

	assert.Equal(t, 0, len(delta.New), "No new files")
	assert.Equal(t, 0, len(delta.Modified), "No modified files")
	assert.Equal(t, 2, len(delta.Same), "All files unchanged")
	assert.False(t, delta.HasChanges(), "Should report no changes")
}

// TestComputeDelta_EmptyPreviousManifest tests first sync (no previous manifest) (Issue #148)
func TestComputeDelta_EmptyPreviousManifest(t *testing.T) {
	now := time.Now()

	// No previous manifest (first sync)
	localFiles := []FileInfo{
		{Path: "file1.txt", Size: 100, ModTime: now, IsDir: false},
		{Path: "file2.txt", Size: 200, ModTime: now, IsDir: false},
		{Path: "file3.txt", Size: 300, ModTime: now, IsDir: false},
	}

	delta, err := ComputeDelta(localFiles, nil, nil)
	require.NoError(t, err)

	assert.Equal(t, 3, len(delta.New), "All files should be new")
	assert.Equal(t, 0, len(delta.Modified), "No modified files")
	assert.Equal(t, 0, len(delta.Same), "No unchanged files")
	assert.True(t, delta.HasChanges(), "Should report changes")
}

// TestComputeDelta_SkipsDirectories tests that directories are ignored (Issue #148)
func TestComputeDelta_SkipsDirectories(t *testing.T) {
	now := time.Now()

	previousManifest := &Manifest{
		Files: []FileEntry{
			{Path: "file1.txt", Size: 100, ModTime: now},
		},
	}

	// Local filesystem includes directories (should be skipped)
	localFiles := []FileInfo{
		{Path: "dir1", Size: 0, ModTime: now, IsDir: true}, // Directory - skip
		{Path: "file1.txt", Size: 100, ModTime: now, IsDir: false},
		{Path: "dir2", Size: 0, ModTime: now, IsDir: true}, // Directory - skip
	}

	delta, err := ComputeDelta(localFiles, previousManifest, nil)
	require.NoError(t, err)

	assert.Equal(t, 0, len(delta.New), "Directories should be skipped")
	assert.Equal(t, 1, len(delta.Same), "Only file1.txt should be counted")
}

// TestHasChanged_SizeChange tests size change detection (Issue #148)
func TestHasChanged_SizeChange(t *testing.T) {
	now := time.Now()

	local := FileInfo{
		Path:    "file.txt",
		Size:    200, // Changed from 100
		ModTime: now,
	}

	manifest := FileEntry{
		Path:    "file.txt",
		Size:    100,
		ModTime: now,
	}

	changed := hasChanged(local, manifest, nil)
	assert.True(t, changed, "Size change should be detected")
}

// TestHasChanged_ModTimeChange tests modification time detection (Issue #148)
func TestHasChanged_ModTimeChange(t *testing.T) {
	now := time.Now()

	local := FileInfo{
		Path:    "file.txt",
		Size:    100,
		ModTime: now, // Newer than manifest
	}

	manifest := FileEntry{
		Path:    "file.txt",
		Size:    100,
		ModTime: now.Add(-1 * time.Hour), // Older
	}

	changed := hasChanged(local, manifest, nil)
	assert.True(t, changed, "ModTime change should be detected")
}

// TestHasChanged_NoChange tests when file hasn't changed (Issue #148)
func TestHasChanged_NoChange(t *testing.T) {
	now := time.Now()

	local := FileInfo{
		Path:    "file.txt",
		Size:    100,
		ModTime: now,
	}

	manifest := FileEntry{
		Path:    "file.txt",
		Size:    100,
		ModTime: now,
	}

	changed := hasChanged(local, manifest, nil)
	assert.False(t, changed, "Identical files should not be detected as changed")
}

// TestDeltaResult_SummaryString tests summary formatting (Issue #148)
func TestDeltaResult_SummaryString(t *testing.T) {
	delta := &DeltaResult{
		New:      []FileInfo{{Path: "new1.txt"}, {Path: "new2.txt"}},
		Modified: []FileInfo{{Path: "mod1.txt"}},
		Deleted:  []string{"del1.txt", "del2.txt", "del3.txt"},
		Same:     []FileInfo{{Path: "same1.txt"}},
	}

	summary := delta.SummaryString()
	assert.Equal(t, "New: 2, Modified: 1, Deleted: 3, Same: 1", summary)
}

// TestDeltaResult_TotalChanges tests change count (Issue #148)
func TestDeltaResult_TotalChanges(t *testing.T) {
	delta := &DeltaResult{
		New:      []FileInfo{{Path: "new1.txt"}, {Path: "new2.txt"}},
		Modified: []FileInfo{{Path: "mod1.txt"}},
		Same:     []FileInfo{{Path: "same1.txt"}},
	}

	assert.Equal(t, 3, delta.TotalChanges(), "Should count new + modified")
	assert.True(t, delta.HasChanges(), "Should report has changes")
}

// TestDeltaResult_GetChangedFiles tests retrieving files to upload (Issue #148)
func TestDeltaResult_GetChangedFiles(t *testing.T) {
	delta := &DeltaResult{
		New:      []FileInfo{{Path: "new1.txt"}, {Path: "new2.txt"}},
		Modified: []FileInfo{{Path: "mod1.txt"}},
		Same:     []FileInfo{{Path: "same1.txt"}},
	}

	changed := delta.GetChangedFiles()
	assert.Equal(t, 3, len(changed), "Should return new + modified files")

	// Verify all changed files are included
	paths := make([]string, len(changed))
	for i, f := range changed {
		paths[i] = f.Path
	}
	assert.Contains(t, paths, "new1.txt")
	assert.Contains(t, paths, "new2.txt")
	assert.Contains(t, paths, "mod1.txt")
	assert.NotContains(t, paths, "same1.txt")
}

// TestManifest_SyncFields tests new sync-related fields (Issue #148)
func TestManifest_SyncFields(t *testing.T) {
	m := &Manifest{
		UploadID:           "sync-001",
		SourcePath:         "/home/user/data",
		PreviousManifestID: "sync-000",
		SyncType:           SyncTypeIncremental,
	}

	assert.Equal(t, "sync-000", m.PreviousManifestID)
	assert.Equal(t, SyncTypeIncremental, m.SyncType)
	assert.Equal(t, "/home/user/data", m.SourcePath)
}

// TestSyncType_Constants tests sync type constants (Issue #148)
func TestSyncType_Constants(t *testing.T) {
	assert.Equal(t, "full", SyncTypeFull)
	assert.Equal(t, "incremental", SyncTypeIncremental)
}
