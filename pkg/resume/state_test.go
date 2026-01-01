package resume

import (
	"os"
	"path/filepath"
	"testing"
	"time"
)

func TestSaveAndLoadState(t *testing.T) {
	// Create a test state
	state := &UploadState{
		UploadID:        "test-upload-123",
		StartTime:       time.Now(),
		LastSave:        time.Now(),
		SourceDir:       "/test/source",
		Bucket:          "test-bucket",
		Prefix:          "test-prefix",
		Region:          "us-west-2",
		StorageClass:    "STANDARD",
		TotalFiles:      100,
		TotalBytes:      1000000,
		CompletedFiles:  50,
		CompletedBytes:  500000,
		ShardCount:      8,
		Shards:          make([]ShardState, 8),
	}

	// Initialize shard states
	for i := 0; i < 8; i++ {
		state.Shards[i] = ShardState{
			ShardID:        i,
			CompletedFiles: 10,
			CompletedBytes: 100000,
		}
	}

	// Save state
	err := SaveState(state)
	if err != nil {
		t.Fatalf("Failed to save state: %v", err)
	}

	// Load state
	loaded, err := LoadState("test-upload-123")
	if err != nil {
		t.Fatalf("Failed to load state: %v", err)
	}

	// Verify basic fields
	if loaded.UploadID != state.UploadID {
		t.Errorf("UploadID mismatch: got %s, want %s", loaded.UploadID, state.UploadID)
	}
	if loaded.SourceDir != state.SourceDir {
		t.Errorf("SourceDir mismatch: got %s, want %s", loaded.SourceDir, state.SourceDir)
	}
	if loaded.Bucket != state.Bucket {
		t.Errorf("Bucket mismatch: got %s, want %s", loaded.Bucket, state.Bucket)
	}
	if loaded.TotalFiles != state.TotalFiles {
		t.Errorf("TotalFiles mismatch: got %d, want %d", loaded.TotalFiles, state.TotalFiles)
	}
	if loaded.CompletedFiles != state.CompletedFiles {
		t.Errorf("CompletedFiles mismatch: got %d, want %d", loaded.CompletedFiles, state.CompletedFiles)
	}

	// Verify shards
	if len(loaded.Shards) != len(state.Shards) {
		t.Errorf("Shard count mismatch: got %d, want %d", len(loaded.Shards), len(state.Shards))
	}

	// Clean up
	err = DeleteState("test-upload-123")
	if err != nil {
		t.Errorf("Failed to delete state: %v", err)
	}
}

func TestListStates(t *testing.T) {
	// Create multiple test states
	states := []*UploadState{
		{
			UploadID:  "test-1",
			StartTime: time.Now().Add(-2 * time.Hour),
			Bucket:    "bucket-1",
		},
		{
			UploadID:  "test-2",
			StartTime: time.Now().Add(-1 * time.Hour),
			Bucket:    "bucket-2",
		},
	}

	// Save states
	for _, state := range states {
		state.LastSave = time.Now()
		if err := SaveState(state); err != nil {
			t.Fatalf("Failed to save state %s: %v", state.UploadID, err)
		}
	}

	// List states
	listed, err := ListStates()
	if err != nil {
		t.Fatalf("Failed to list states: %v", err)
	}

	// Should have at least our 2 test states
	if len(listed) < 2 {
		t.Errorf("Expected at least 2 states, got %d", len(listed))
	}

	// Verify states are sorted by start time (newest first)
	if len(listed) >= 2 {
		if listed[0].StartTime.Before(listed[1].StartTime) {
			t.Error("States not sorted by start time (newest first)")
		}
	}

	// Clean up
	for _, state := range states {
		_ = DeleteState(state.UploadID)
	}
}

func TestStateProgress(t *testing.T) {
	state := &UploadState{
		TotalBytes:     1000,
		CompletedBytes: 450,
	}

	progress := state.Progress()
	expected := 45.0

	if progress != expected {
		t.Errorf("Progress mismatch: got %.1f%%, want %.1f%%", progress, expected)
	}

	// Test zero total bytes
	state.TotalBytes = 0
	progress = state.Progress()
	if progress != 0 {
		t.Errorf("Progress should be 0 when TotalBytes is 0, got %.1f%%", progress)
	}
}

func TestStateIsComplete(t *testing.T) {
	tests := []struct {
		name           string
		totalFiles     int64
		completedFiles int64
		want           bool
	}{
		{"Complete", 100, 100, true},
		{"Incomplete", 100, 50, false},
		{"Zero files", 0, 0, false},
		{"Exceeded (edge case)", 100, 101, true},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			state := &UploadState{
				TotalFiles:     tt.totalFiles,
				CompletedFiles: tt.completedFiles,
			}
			got := state.IsComplete()
			if got != tt.want {
				t.Errorf("IsComplete() = %v, want %v", got, tt.want)
			}
		})
	}
}

func TestCleanupOldStates(t *testing.T) {
	// Create an old state (>1 hour ago)
	oldState := &UploadState{
		UploadID:  "old-state",
		StartTime: time.Now().Add(-2 * time.Hour),
		LastSave:  time.Now(),
		Bucket:    "test-bucket",
	}

	// Create a recent state
	recentState := &UploadState{
		UploadID:  "recent-state",
		StartTime: time.Now().Add(-30 * time.Minute),
		LastSave:  time.Now(),
		Bucket:    "test-bucket",
	}

	// Save both states
	if err := SaveState(oldState); err != nil {
		t.Fatalf("Failed to save old state: %v", err)
	}
	if err := SaveState(recentState); err != nil {
		t.Fatalf("Failed to save recent state: %v", err)
	}

	// Cleanup states older than 1 hour
	count, err := CleanupOldStates(1 * time.Hour)
	if err != nil {
		t.Fatalf("CleanupOldStates failed: %v", err)
	}

	// Should have deleted at least 1 state (the old one)
	if count < 1 {
		t.Errorf("Expected to delete at least 1 state, deleted %d", count)
	}

	// Verify old state was deleted
	_, err = LoadState("old-state")
	if err == nil {
		t.Error("Old state should have been deleted")
	}

	// Verify recent state still exists
	_, err = LoadState("recent-state")
	if err != nil {
		t.Error("Recent state should still exist")
	}

	// Clean up
	_ = DeleteState("recent-state")
}

func TestFindStateBySource(t *testing.T) {
	// Create test state
	state := &UploadState{
		UploadID:  "source-test",
		StartTime: time.Now(),
		LastSave:  time.Now(),
		SourceDir: "/test/source/data",
		Bucket:    "test-bucket",
		Prefix:    "test/prefix",
	}

	// Save state
	if err := SaveState(state); err != nil {
		t.Fatalf("Failed to save state: %v", err)
	}

	// Find state by source
	found, err := FindStateBySource("/test/source/data", "test-bucket", "test/prefix")
	if err != nil {
		t.Fatalf("FindStateBySource failed: %v", err)
	}

	if found == nil {
		t.Fatal("State should have been found")
	}

	if found.UploadID != state.UploadID {
		t.Errorf("Found wrong state: got %s, want %s", found.UploadID, state.UploadID)
	}

	// Try finding with non-matching source
	found, err = FindStateBySource("/different/source", "test-bucket", "test/prefix")
	if err != nil {
		t.Fatalf("FindStateBySource failed: %v", err)
	}

	if found != nil {
		t.Error("Should not have found state with different source")
	}

	// Clean up
	_ = DeleteState("source-test")
}

func TestStateFileAtomicWrite(t *testing.T) {
	state := &UploadState{
		UploadID:  "atomic-test",
		StartTime: time.Now(),
		LastSave:  time.Now(),
		Bucket:    "test-bucket",
	}

	// Save state
	if err := SaveState(state); err != nil {
		t.Fatalf("Failed to save state: %v", err)
	}

	// Verify temp file doesn't exist
	stateDir, _ := GetStateDir()
	tempFile := filepath.Join(stateDir, "atomic-test"+StateTempExtension)
	if _, err := os.Stat(tempFile); err == nil {
		t.Error("Temp file should not exist after atomic save")
	}

	// Verify final file exists
	finalFile := filepath.Join(stateDir, "atomic-test"+StateFileExtension)
	if _, err := os.Stat(finalFile); err != nil {
		t.Error("Final state file should exist")
	}

	// Clean up
	_ = DeleteState("atomic-test")
}

// TestUploadState_Age tests the Age method
func TestUploadState_Age(t *testing.T) {
	now := time.Now()
	state := &UploadState{
		StartTime: now.Add(-1 * time.Hour), // Started 1 hour ago
	}

	age := state.Age()

	// Should be approximately 1 hour (allow 1 second tolerance)
	expectedMin := 59 * time.Minute
	expectedMax := 61 * time.Minute

	if age < expectedMin || age > expectedMax {
		t.Errorf("Age() = %v, want approximately 1 hour", age)
	}
}

// TestUploadState_TimeSinceLastSave tests the TimeSinceLastSave method
func TestUploadState_TimeSinceLastSave(t *testing.T) {
	now := time.Now()
	state := &UploadState{
		LastSave: now.Add(-30 * time.Minute), // Saved 30 minutes ago
	}

	timeSince := state.TimeSinceLastSave()

	// Should be approximately 30 minutes (allow 1 second tolerance)
	expectedMin := 29 * time.Minute
	expectedMax := 31 * time.Minute

	if timeSince < expectedMin || timeSince > expectedMax {
		t.Errorf("TimeSinceLastSave() = %v, want approximately 30 minutes", timeSince)
	}
}

// TestStateExists tests the StateExists function
func TestStateExists(t *testing.T) {
	// Test with empty upload ID
	if StateExists("") {
		t.Error("StateExists(\"\") should return false")
	}

	// Create a test state
	state := &UploadState{
		UploadID:  "test-exists-123",
		StartTime: time.Now(),
		LastSave:  time.Now(),
		SourceDir: "/test",
		Bucket:    "test-bucket",
		Prefix:    "test-prefix",
	}

	// Save the state
	err := SaveState(state)
	if err != nil {
		t.Fatalf("Failed to save state: %v", err)
	}
	defer func() { _ = DeleteState("test-exists-123") }()

	// Test that it exists
	if !StateExists("test-exists-123") {
		t.Error("StateExists() should return true for existing state")
	}

	// Test with non-existent ID
	if StateExists("non-existent-id") {
		t.Error("StateExists() should return false for non-existent state")
	}
}

// TestChangeDetectionResult_HasChanges tests the HasChanges method
func TestChangeDetectionResult_HasChanges(t *testing.T) {
	tests := []struct {
		name   string
		result *ChangeDetectionResult
		want   bool
	}{
		{
			name: "no changes",
			result: &ChangeDetectionResult{
				ModifiedFiles: []string{},
				DeletedFiles:  []string{},
				NewFiles:      []string{},
			},
			want: false,
		},
		{
			name: "has modified files",
			result: &ChangeDetectionResult{
				ModifiedFiles: []string{"file1.txt"},
				DeletedFiles:  []string{},
				NewFiles:      []string{},
			},
			want: true,
		},
		{
			name: "has deleted files",
			result: &ChangeDetectionResult{
				ModifiedFiles: []string{},
				DeletedFiles:  []string{"file2.txt"},
				NewFiles:      []string{},
			},
			want: true,
		},
		{
			name: "has new files",
			result: &ChangeDetectionResult{
				ModifiedFiles: []string{},
				DeletedFiles:  []string{},
				NewFiles:      []string{"file3.txt"},
			},
			want: true,
		},
		{
			name: "has all types of changes",
			result: &ChangeDetectionResult{
				ModifiedFiles: []string{"file1.txt"},
				DeletedFiles:  []string{"file2.txt"},
				NewFiles:      []string{"file3.txt"},
			},
			want: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := tt.result.HasChanges()
			if got != tt.want {
				t.Errorf("HasChanges() = %v, want %v", got, tt.want)
			}
		})
	}
}

// TestChangeDetectionResult_TotalChanges tests the TotalChanges method
func TestChangeDetectionResult_TotalChanges(t *testing.T) {
	tests := []struct {
		name   string
		result *ChangeDetectionResult
		want   int
	}{
		{
			name: "no changes",
			result: &ChangeDetectionResult{
				ModifiedFiles: []string{},
				DeletedFiles:  []string{},
				NewFiles:      []string{},
			},
			want: 0,
		},
		{
			name: "one modified file",
			result: &ChangeDetectionResult{
				ModifiedFiles: []string{"file1.txt"},
				DeletedFiles:  []string{},
				NewFiles:      []string{},
			},
			want: 1,
		},
		{
			name: "multiple changes",
			result: &ChangeDetectionResult{
				ModifiedFiles: []string{"file1.txt", "file2.txt"},
				DeletedFiles:  []string{"file3.txt"},
				NewFiles:      []string{"file4.txt", "file5.txt", "file6.txt"},
			},
			want: 6, // 2 modified + 1 deleted + 3 new
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := tt.result.TotalChanges()
			if got != tt.want {
				t.Errorf("TotalChanges() = %v, want %v", got, tt.want)
			}
		})
	}
}
