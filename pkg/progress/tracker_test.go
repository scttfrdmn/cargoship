package progress

import (
	"errors"
	"strings"
	"testing"
	"time"
)

func TestNewTracker(t *testing.T) {
	totalBytes := int64(1000)
	totalFiles := 10

	tracker := NewTracker(totalBytes, totalFiles)

	if tracker.totalBytes != totalBytes {
		t.Errorf("NewTracker() totalBytes = %v, want %v", tracker.totalBytes, totalBytes)
	}
	if tracker.totalFiles != totalFiles {
		t.Errorf("NewTracker() totalFiles = %v, want %v", tracker.totalFiles, totalFiles)
	}
	if tracker.uploadedBytes != 0 {
		t.Errorf("NewTracker() uploadedBytes = %v, want 0", tracker.uploadedBytes)
	}
	if tracker.uploadedFiles != 0 {
		t.Errorf("NewTracker() uploadedFiles = %v, want 0", tracker.uploadedFiles)
	}
	if tracker.updateInterval != 500*time.Millisecond {
		t.Errorf("NewTracker() updateInterval = %v, want %v", tracker.updateInterval, 500*time.Millisecond)
	}
	if len(tracker.listeners) != 0 {
		t.Errorf("NewTracker() listeners length = %v, want 0", len(tracker.listeners))
	}
	if len(tracker.errors) != 0 {
		t.Errorf("NewTracker() errors length = %v, want 0", len(tracker.errors))
	}
}

func TestNewConsoleProgressListener(t *testing.T) {
	listener := NewConsoleProgressListener(true)

	if !listener.showDetails {
		t.Errorf("NewConsoleProgressListener(true) showDetails = false, want true")
	}
	if listener.printInterval != 1*time.Second {
		t.Errorf("NewConsoleProgressListener() printInterval = %v, want %v", listener.printInterval, 1*time.Second)
	}

	listener2 := NewConsoleProgressListener(false)
	if listener2.showDetails {
		t.Errorf("NewConsoleProgressListener(false) showDetails = true, want false")
	}
}

func TestTracker_AddListener(t *testing.T) {
	tracker := NewTracker(1000, 10)
	listener := &mockProgressListener{}

	tracker.AddListener(listener)

	if len(tracker.listeners) != 1 {
		t.Errorf("AddListener() listeners length = %v, want 1", len(tracker.listeners))
	}
}

func TestTracker_SetCurrentFile(t *testing.T) {
	tracker := NewTracker(1000, 10)
	listener := &mockProgressListener{}
	tracker.AddListener(listener)

	filename := "test.txt"
	tracker.SetCurrentFile(filename)

	if tracker.currentFile != filename {
		t.Errorf("SetCurrentFile() currentFile = %v, want %v", tracker.currentFile, filename)
	}

	// Check that listeners were notified
	if len(listener.progressUpdates) == 0 {
		t.Errorf("SetCurrentFile() should notify listeners")
	}
}

func TestTracker_AddBytes(t *testing.T) {
	tracker := NewTracker(1000, 10)
	listener := &mockProgressListener{}
	tracker.AddListener(listener)

	bytes := int64(100)
	tracker.AddBytes(bytes)

	if tracker.uploadedBytes != bytes {
		t.Errorf("AddBytes() uploadedBytes = %v, want %v", tracker.uploadedBytes, bytes)
	}

	// Add more bytes
	tracker.AddBytes(bytes)
	if tracker.uploadedBytes != 2*bytes {
		t.Errorf("AddBytes() uploadedBytes = %v, want %v", tracker.uploadedBytes, 2*bytes)
	}

	// Check that listeners were notified
	if len(listener.progressUpdates) == 0 {
		t.Errorf("AddBytes() should notify listeners")
	}
}

func TestTracker_CompleteFile(t *testing.T) {
	tracker := NewTracker(1000, 10)
	listener := &mockProgressListener{}
	tracker.AddListener(listener)

	// Set current file first
	tracker.SetCurrentFile("test.txt")

	tracker.CompleteFile()

	if tracker.uploadedFiles != 1 {
		t.Errorf("CompleteFile() uploadedFiles = %v, want 1", tracker.uploadedFiles)
	}
	if tracker.currentFile != "" {
		t.Errorf("CompleteFile() currentFile = %v, want empty string", tracker.currentFile)
	}

	// Check that listeners were notified (may be rate limited, so just check we have some updates)
	if len(listener.progressUpdates) == 0 {
		t.Errorf("CompleteFile() should notify listeners")
	}
}

func TestTracker_AddError(t *testing.T) {
	tracker := NewTracker(1000, 10)
	listener := &mockProgressListener{}
	tracker.AddListener(listener)

	err := errors.New("test error")
	tracker.AddError(err)

	if len(tracker.errors) != 1 {
		t.Errorf("AddError() errors length = %v, want 1", len(tracker.errors))
	}
	if tracker.errors[0] != err {
		t.Errorf("AddError() error = %v, want %v", tracker.errors[0], err)
	}

	// Check that listeners were notified of error
	if len(listener.errorUpdates) != 1 {
		t.Errorf("AddError() should notify listeners of error")
	}
	if listener.errorUpdates[0].err != err {
		t.Errorf("AddError() error notification = %v, want %v", listener.errorUpdates[0].err, err)
	}
}

func TestTracker_GetProgress(t *testing.T) {
	tracker := NewTracker(1000, 10)
	
	// Initial progress
	progress := tracker.GetProgress()
	if progress.TotalBytes != 1000 {
		t.Errorf("GetProgress() TotalBytes = %v, want 1000", progress.TotalBytes)
	}
	if progress.UploadedBytes != 0 {
		t.Errorf("GetProgress() UploadedBytes = %v, want 0", progress.UploadedBytes)
	}
	if progress.PercentComplete != 0 {
		t.Errorf("GetProgress() PercentComplete = %v, want 0", progress.PercentComplete)
	}

	// Add some progress
	tracker.AddBytes(500)
	progress = tracker.GetProgress()
	if progress.UploadedBytes != 500 {
		t.Errorf("GetProgress() UploadedBytes = %v, want 500", progress.UploadedBytes)
	}
	if progress.PercentComplete != 50.0 {
		t.Errorf("GetProgress() PercentComplete = %v, want 50.0", progress.PercentComplete)
	}
}

func TestTracker_Complete(t *testing.T) {
	tracker := NewTracker(1000, 10)
	listener := &mockProgressListener{}
	tracker.AddListener(listener)

	// Partially complete
	tracker.AddBytes(500)
	tracker.uploadedFiles = 5

	tracker.Complete()

	if tracker.uploadedBytes != tracker.totalBytes {
		t.Errorf("Complete() uploadedBytes = %v, want %v", tracker.uploadedBytes, tracker.totalBytes)
	}
	if tracker.uploadedFiles != tracker.totalFiles {
		t.Errorf("Complete() uploadedFiles = %v, want %v", tracker.uploadedFiles, tracker.totalFiles)
	}
	if tracker.currentFile != "" {
		t.Errorf("Complete() currentFile = %v, want empty string", tracker.currentFile)
	}

	// Check that listeners were notified of completion
	if len(listener.completeUpdates) != 1 {
		t.Errorf("Complete() should notify listeners of completion")
	}
}

func TestTracker_ProgressCalculations(t *testing.T) {
	tracker := NewTracker(1000, 10)
	
	// Allow some time to pass for meaningful calculations
	time.Sleep(10 * time.Millisecond)
	
	tracker.AddBytes(500)
	progress := tracker.GetProgress()

	// Check percent calculation
	if progress.PercentComplete != 50.0 {
		t.Errorf("Progress PercentComplete = %v, want 50.0", progress.PercentComplete)
	}

	// Check that elapsed time is positive
	if progress.ElapsedTime <= 0 {
		t.Errorf("Progress ElapsedTime = %v, want positive duration", progress.ElapsedTime)
	}

	// Check that average speed is calculated
	if progress.AverageSpeed <= 0 {
		t.Errorf("Progress AverageSpeed = %v, want positive value", progress.AverageSpeed)
	}

	// Check that ETA is calculated for partial progress
	if progress.ETA <= 0 {
		t.Errorf("Progress ETA = %v, want positive duration", progress.ETA)
	}
}

func TestTracker_ZeroBytes(t *testing.T) {
	tracker := NewTracker(0, 5)
	progress := tracker.GetProgress()

	// With zero total bytes, percent should be 0
	if progress.PercentComplete != 0 {
		t.Errorf("Progress PercentComplete with zero bytes = %v, want 0", progress.PercentComplete)
	}
}

func TestTracker_ErrorHandling(t *testing.T) {
	tracker := NewTracker(1000, 10)
	
	err1 := errors.New("first error")
	err2 := errors.New("second error")
	
	tracker.AddError(err1)
	tracker.AddError(err2)
	
	progress := tracker.GetProgress()
	
	if progress.ErrorCount != 2 {
		t.Errorf("Progress ErrorCount = %v, want 2", progress.ErrorCount)
	}
	if progress.LastError != "second error" {
		t.Errorf("Progress LastError = %v, want 'second error'", progress.LastError)
	}
}

func TestConsoleProgressListener_OnProgress(t *testing.T) {
	listener := NewConsoleProgressListener(true)
	
	update := ProgressUpdate{
		TotalBytes:      1000,
		UploadedBytes:   500,
		PercentComplete: 50.0,
		CurrentFile:     "test.txt",
		AverageSpeed:    10.5,
		ETA:             30 * time.Second,
		ErrorCount:      1,
	}

	// This should not panic - we can't easily test console output
	listener.OnProgress(update)

	// Test with 100% completion
	update.PercentComplete = 100.0
	listener.OnProgress(update)
}

func TestConsoleProgressListener_OnComplete(t *testing.T) {
	listener := NewConsoleProgressListener(true)
	
	update := ProgressUpdate{
		TotalBytes:      1000,
		UploadedBytes:   1000,
		PercentComplete: 100.0,
		TotalFiles:      10,
		ElapsedTime:     2 * time.Minute,
		AverageSpeed:    8.3,
		ErrorCount:      2,
	}

	// This should not panic
	listener.OnComplete(update)
}

func TestConsoleProgressListener_OnError(t *testing.T) {
	listener := NewConsoleProgressListener(false)
	
	err := errors.New("test error")
	update := ProgressUpdate{
		PercentComplete: 25.0,
	}

	// This should not panic
	listener.OnError(err, update)
}

func TestConsoleProgressListener_RateLimit(t *testing.T) {
	listener := NewConsoleProgressListener(true)
	
	update := ProgressUpdate{
		PercentComplete: 50.0,
	}

	// First call should work
	listener.OnProgress(update)
	
	// Immediate second call should be rate limited (no output)
	listener.OnProgress(update)
}

func TestFormatBytes(t *testing.T) {
	tests := []struct {
		bytes    int64
		expected string
	}{
		{0, "0 B"},
		{500, "500 B"},
		{1024, "1.0 KB"},
		{1536, "1.5 KB"},
		{1048576, "1.0 MB"},
		{1073741824, "1.0 GB"},
		{1099511627776, "1.0 TB"},
		{1125899906842624, "1.0 PB"},
	}

	for _, tt := range tests {
		t.Run(tt.expected, func(t *testing.T) {
			result := formatBytes(tt.bytes)
			if result != tt.expected {
				t.Errorf("formatBytes(%v) = %v, want %v", tt.bytes, result, tt.expected)
			}
		})
	}
}

func TestFormatDuration(t *testing.T) {
	tests := []struct {
		duration time.Duration
		expected string
	}{
		{0, "0s"},
		{-1 * time.Second, "0s"},
		{30 * time.Second, "30s"},
		{90 * time.Second, "1m 30s"},
		{3661 * time.Second, "1h 1m"},
		{7200 * time.Second, "2h 0m"},
	}

	for _, tt := range tests {
		t.Run(tt.expected, func(t *testing.T) {
			result := formatDuration(tt.duration)
			if result != tt.expected {
				t.Errorf("formatDuration(%v) = %v, want %v", tt.duration, result, tt.expected)
			}
		})
	}
}

func TestNewJSONProgressListener(t *testing.T) {
	var capturedUpdate ProgressUpdate
	outputFunc := func(update ProgressUpdate) {
		capturedUpdate = update
	}

	listener := NewJSONProgressListener(outputFunc)

	if listener.outputFunc == nil {
		t.Errorf("NewJSONProgressListener() outputFunc is nil")
	}

	// Test the listener methods
	update := ProgressUpdate{
		PercentComplete: 75.0,
		CurrentFile:     "test.json",
	}

	listener.OnProgress(update)
	if capturedUpdate.PercentComplete != 75.0 {
		t.Errorf("JSONProgressListener OnProgress() captured wrong update")
	}

	listener.OnComplete(update)
	if capturedUpdate.CurrentFile != "test.json" {
		t.Errorf("JSONProgressListener OnComplete() captured wrong update")
	}

	err := errors.New("json error")
	listener.OnError(err, update)
	// Should still capture the update
	if capturedUpdate.PercentComplete != 75.0 {
		t.Errorf("JSONProgressListener OnError() captured wrong update")
	}
}

func TestJSONProgressListener_NilOutputFunc(t *testing.T) {
	listener := NewJSONProgressListener(nil)
	
	update := ProgressUpdate{PercentComplete: 50.0}
	err := errors.New("test error")

	// These should not panic with nil outputFunc
	listener.OnProgress(update)
	listener.OnComplete(update)
	listener.OnError(err, update)
}

func TestProgressUpdate_Fields(t *testing.T) {
	now := time.Now()
	update := ProgressUpdate{
		TotalBytes:      1000,
		UploadedBytes:   750,
		PercentComplete: 75.0,
		TotalFiles:      10,
		UploadedFiles:   7,
		CurrentFile:     "current.txt",
		ElapsedTime:     2 * time.Minute,
		EstimatedTotal:  3 * time.Minute,
		ETA:             1 * time.Minute,
		AverageSpeed:    5.5,
		CurrentSpeed:    6.0,
		ErrorCount:      1,
		LastError:       "test error",
		StartTime:       now,
		LastUpdate:      now,
	}

	if update.TotalBytes != 1000 {
		t.Errorf("ProgressUpdate.TotalBytes = %v, want 1000", update.TotalBytes)
	}
	if update.UploadedBytes != 750 {
		t.Errorf("ProgressUpdate.UploadedBytes = %v, want 750", update.UploadedBytes)
	}
	if update.PercentComplete != 75.0 {
		t.Errorf("ProgressUpdate.PercentComplete = %v, want 75.0", update.PercentComplete)
	}
	if update.CurrentFile != "current.txt" {
		t.Errorf("ProgressUpdate.CurrentFile = %v, want 'current.txt'", update.CurrentFile)
	}
	if update.ErrorCount != 1 {
		t.Errorf("ProgressUpdate.ErrorCount = %v, want 1", update.ErrorCount)
	}
	if update.LastError != "test error" {
		t.Errorf("ProgressUpdate.LastError = %v, want 'test error'", update.LastError)
	}
}

func TestTracker_NotifyListenersRateLimit(t *testing.T) {
	tracker := NewTracker(1000, 10)
	listener := &mockProgressListener{}
	tracker.AddListener(listener)

	// Set a very short update interval for testing
	tracker.updateInterval = 100 * time.Millisecond

	// Multiple rapid updates should be rate limited
	tracker.AddBytes(100)
	tracker.AddBytes(100)
	tracker.AddBytes(100)

	// Should have fewer notifications than update calls due to rate limiting
	// (exact count depends on timing, but should be limited)
	if len(listener.progressUpdates) >= 3 {
		t.Errorf("Rate limiting not working: got %v notifications for 3 rapid updates", len(listener.progressUpdates))
	}

	// Wait for rate limit to expire
	time.Sleep(150 * time.Millisecond)
	
	// This update should go through
	tracker.AddBytes(100)
	if len(listener.progressUpdates) == 0 {
		t.Errorf("Should have at least one notification after rate limit expires")
	}
}

func TestTracker_FullWorkflow(t *testing.T) {
	tracker := NewTracker(1000, 3)
	listener := &mockProgressListener{}
	tracker.AddListener(listener)

	// Simulate uploading files
	tracker.SetCurrentFile("file1.txt")
	tracker.AddBytes(300)
	tracker.CompleteFile()

	tracker.SetCurrentFile("file2.txt")
	tracker.AddBytes(400)
	tracker.CompleteFile()

	tracker.SetCurrentFile("file3.txt")
	tracker.AddBytes(300)
	tracker.CompleteFile()

	// Complete the upload
	tracker.Complete()

	// Verify final state
	progress := tracker.GetProgress()
	if progress.UploadedBytes != 1000 {
		t.Errorf("Final uploadedBytes = %v, want 1000", progress.UploadedBytes)
	}
	if progress.UploadedFiles != 3 {
		t.Errorf("Final uploadedFiles = %v, want 3", progress.UploadedFiles)
	}
	if progress.PercentComplete != 100.0 {
		t.Errorf("Final PercentComplete = %v, want 100.0", progress.PercentComplete)
	}

	// Should have received completion notification
	if len(listener.completeUpdates) != 1 {
		t.Errorf("Should have received exactly one completion notification")
	}
}

func TestConsoleProgressListener_ProgressBar(t *testing.T) {
	listener := NewConsoleProgressListener(false)
	
	// Test different progress percentages
	tests := []float64{0, 25, 50, 75, 100}
	
	for _, percent := range tests {
		update := ProgressUpdate{
			PercentComplete: percent,
		}
		
		// Should not panic with any percentage
		listener.OnProgress(update)
	}
}

func TestFormatBytes_EdgeCases(t *testing.T) {
	// Test very large numbers
	result := formatBytes(1125899906842624 * 1024) // Beyond PB
	if !strings.Contains(result, "PB") {
		t.Errorf("formatBytes for very large number should still use PB: %v", result)
	}

	// Test exact unit boundaries
	result = formatBytes(1024 * 1024) // Exactly 1 MB
	if result != "1.0 MB" {
		t.Errorf("formatBytes(1048576) = %v, want '1.0 MB'", result)
	}
}

func TestFormatDuration_EdgeCases(t *testing.T) {
	// Test exactly 1 minute
	result := formatDuration(60 * time.Second)
	if result != "1m 0s" {
		t.Errorf("formatDuration(60s) = %v, want '1m 0s'", result)
	}

	// Test exactly 1 hour
	result = formatDuration(3600 * time.Second)
	if result != "1h 0m" {
		t.Errorf("formatDuration(3600s) = %v, want '1h 0m'", result)
	}
}

// mockProgressListener for testing
type mockProgressListener struct {
	progressUpdates  []ProgressUpdate
	completeUpdates  []ProgressUpdate
	errorUpdates     []errorUpdate
}

type errorUpdate struct {
	err    error
	update ProgressUpdate
}

func (m *mockProgressListener) OnProgress(update ProgressUpdate) {
	m.progressUpdates = append(m.progressUpdates, update)
}

func (m *mockProgressListener) OnComplete(update ProgressUpdate) {
	m.completeUpdates = append(m.completeUpdates, update)
}

func (m *mockProgressListener) OnError(err error, update ProgressUpdate) {
	m.errorUpdates = append(m.errorUpdates, errorUpdate{err: err, update: update})
}