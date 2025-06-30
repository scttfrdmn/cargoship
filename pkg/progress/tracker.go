// Package progress provides upload progress tracking with ETA for CargoShip
package progress

import (
	"fmt"
	"sync"
	"time"
)

// Tracker provides real-time upload progress tracking with ETA calculations
type Tracker struct {
	startTime       time.Time
	totalBytes      int64
	uploadedBytes   int64
	totalFiles      int
	uploadedFiles   int
	currentFile     string
	errors          []error
	mutex           sync.RWMutex
	listeners       []ProgressListener
	lastUpdate      time.Time
	updateInterval  time.Duration
}

// ProgressUpdate contains current progress information
type ProgressUpdate struct {
	// Overall progress
	TotalBytes      int64         `json:"total_bytes"`
	UploadedBytes   int64         `json:"uploaded_bytes"`
	PercentComplete float64       `json:"percent_complete"`
	
	// File progress
	TotalFiles      int           `json:"total_files"`
	UploadedFiles   int           `json:"uploaded_files"`
	CurrentFile     string        `json:"current_file"`
	
	// Time estimates
	ElapsedTime     time.Duration `json:"elapsed_time"`
	EstimatedTotal  time.Duration `json:"estimated_total"`
	ETA             time.Duration `json:"eta"`
	
	// Performance metrics
	AverageSpeed    float64       `json:"average_speed_mbps"`
	CurrentSpeed    float64       `json:"current_speed_mbps"`
	
	// Error tracking
	ErrorCount      int           `json:"error_count"`
	LastError       string        `json:"last_error,omitempty"`
	
	// Metadata
	StartTime       time.Time     `json:"start_time"`
	LastUpdate      time.Time     `json:"last_update"`
}

// ProgressListener receives progress updates
type ProgressListener interface {
	OnProgress(update ProgressUpdate)
	OnComplete(update ProgressUpdate)
	OnError(err error, update ProgressUpdate)
}

// ConsoleProgressListener provides console output for progress
type ConsoleProgressListener struct {
	showDetails bool
	lastPrint   time.Time
	printInterval time.Duration
}

// NewTracker creates a new progress tracker
func NewTracker(totalBytes int64, totalFiles int) *Tracker {
	return &Tracker{
		startTime:      time.Now(),
		totalBytes:     totalBytes,
		totalFiles:     totalFiles,
		updateInterval: 500 * time.Millisecond, // Update every 500ms
		listeners:      make([]ProgressListener, 0),
		errors:         make([]error, 0),
	}
}

// NewConsoleProgressListener creates a console progress listener
func NewConsoleProgressListener(showDetails bool) *ConsoleProgressListener {
	return &ConsoleProgressListener{
		showDetails:   showDetails,
		printInterval: 1 * time.Second, // Print every second
	}
}

// AddListener adds a progress listener
func (t *Tracker) AddListener(listener ProgressListener) {
	t.mutex.Lock()
	defer t.mutex.Unlock()
	t.listeners = append(t.listeners, listener)
}

// SetCurrentFile updates the currently uploading file
func (t *Tracker) SetCurrentFile(filename string) {
	t.mutex.Lock()
	defer t.mutex.Unlock()
	t.currentFile = filename
	t.notifyListeners()
}

// AddBytes increments the uploaded bytes counter
func (t *Tracker) AddBytes(bytes int64) {
	t.mutex.Lock()
	defer t.mutex.Unlock()
	t.uploadedBytes += bytes
	t.notifyListeners()
}

// CompleteFile marks a file as completed
func (t *Tracker) CompleteFile() {
	t.mutex.Lock()
	defer t.mutex.Unlock()
	t.uploadedFiles++
	t.currentFile = ""
	t.notifyListeners()
}

// AddError records an error
func (t *Tracker) AddError(err error) {
	t.mutex.Lock()
	defer t.mutex.Unlock()
	t.errors = append(t.errors, err)
	
	// Notify listeners of error
	update := t.buildProgressUpdate()
	for _, listener := range t.listeners {
		listener.OnError(err, update)
	}
}

// GetProgress returns current progress information
func (t *Tracker) GetProgress() ProgressUpdate {
	t.mutex.RLock()
	defer t.mutex.RUnlock()
	return t.buildProgressUpdate()
}

// Complete marks the upload as completed
func (t *Tracker) Complete() {
	t.mutex.Lock()
	defer t.mutex.Unlock()
	
	// Ensure all counters are at maximum
	t.uploadedFiles = t.totalFiles
	t.uploadedBytes = t.totalBytes
	t.currentFile = ""
	
	update := t.buildProgressUpdate()
	for _, listener := range t.listeners {
		listener.OnComplete(update)
	}
}

// buildProgressUpdate creates a progress update (must be called with lock held)
func (t *Tracker) buildProgressUpdate() ProgressUpdate {
	now := time.Now()
	elapsed := now.Sub(t.startTime)
	
	var percentComplete float64
	if t.totalBytes > 0 {
		percentComplete = float64(t.uploadedBytes) / float64(t.totalBytes) * 100
	}
	
	// Calculate speeds
	var averageSpeed, currentSpeed float64
	if elapsed.Seconds() > 0 {
		averageSpeed = float64(t.uploadedBytes) / elapsed.Seconds() / (1024 * 1024) // MB/s
	}
	
	// Current speed based on recent progress (simplified)
	if now.Sub(t.lastUpdate).Seconds() > 0 {
		currentSpeed = averageSpeed // Simplified - would need sliding window for accuracy
	}
	
	// Calculate ETA
	var estimatedTotal, eta time.Duration
	if percentComplete > 0 && percentComplete < 100 {
		estimatedTotal = time.Duration(float64(elapsed) / (percentComplete / 100))
		eta = estimatedTotal - elapsed
	}
	
	// Get last error
	var lastError string
	if len(t.errors) > 0 {
		lastError = t.errors[len(t.errors)-1].Error()
	}
	
	return ProgressUpdate{
		TotalBytes:      t.totalBytes,
		UploadedBytes:   t.uploadedBytes,
		PercentComplete: percentComplete,
		TotalFiles:      t.totalFiles,
		UploadedFiles:   t.uploadedFiles,
		CurrentFile:     t.currentFile,
		ElapsedTime:     elapsed,
		EstimatedTotal:  estimatedTotal,
		ETA:             eta,
		AverageSpeed:    averageSpeed,
		CurrentSpeed:    currentSpeed,
		ErrorCount:      len(t.errors),
		LastError:       lastError,
		StartTime:       t.startTime,
		LastUpdate:      now,
	}
}

// notifyListeners sends updates to all listeners (must be called with lock held)
func (t *Tracker) notifyListeners() {
	now := time.Now()
	
	// Rate limit updates
	if now.Sub(t.lastUpdate) < t.updateInterval {
		return
	}
	
	t.lastUpdate = now
	update := t.buildProgressUpdate()
	
	for _, listener := range t.listeners {
		listener.OnProgress(update)
	}
}

// OnProgress implements ProgressListener for console output
func (c *ConsoleProgressListener) OnProgress(update ProgressUpdate) {
	now := time.Now()
	
	// Rate limit console output
	if now.Sub(c.lastPrint) < c.printInterval {
		return
	}
	c.lastPrint = now
	
	// Create progress bar
	barWidth := 40
	filled := int(update.PercentComplete / 100 * float64(barWidth))
	bar := ""
	for i := 0; i < barWidth; i++ {
		if i < filled {
			bar += "█"
		} else {
			bar += "░"
		}
	}
	
	// Format output
	fmt.Printf("\r🚢 Upload Progress: [%s] %.1f%% ", bar, update.PercentComplete)
	
	if c.showDetails {
		fmt.Printf("| %s | %.1f MB/s | ETA: %s", 
			formatBytes(update.UploadedBytes),
			update.AverageSpeed,
			formatDuration(update.ETA))
		
		if update.CurrentFile != "" {
			fmt.Printf(" | %s", update.CurrentFile)
		}
		
		if update.ErrorCount > 0 {
			fmt.Printf(" | ⚠️ %d errors", update.ErrorCount)
		}
	}
	
	// Add newline if complete
	if update.PercentComplete >= 100 {
		fmt.Println()
	}
}

// OnComplete implements ProgressListener for console output
func (c *ConsoleProgressListener) OnComplete(update ProgressUpdate) {
	fmt.Printf("\n✅ Upload completed in %s\n", formatDuration(update.ElapsedTime))
	fmt.Printf("   📊 Total: %s (%d files)\n", formatBytes(update.TotalBytes), update.TotalFiles)
	fmt.Printf("   ⚡ Average speed: %.1f MB/s\n", update.AverageSpeed)
	
	if update.ErrorCount > 0 {
		fmt.Printf("   ⚠️ Errors encountered: %d\n", update.ErrorCount)
	}
}

// OnError implements ProgressListener for console output
func (c *ConsoleProgressListener) OnError(err error, update ProgressUpdate) {
	fmt.Printf("\n⚠️ Error: %v\n", err)
	
	// Continue progress display
	c.OnProgress(update)
}

// formatBytes formats bytes in human readable format
func formatBytes(bytes int64) string {
	const unit = 1024
	if bytes < unit {
		return fmt.Sprintf("%d B", bytes)
	}
	
	div, exp := int64(unit), 0
	for n := bytes / unit; n >= unit; n /= unit {
		div *= unit
		exp++
	}
	
	units := []string{"KB", "MB", "GB", "TB", "PB"}
	if exp >= len(units) {
		exp = len(units) - 1 // Cap at PB for very large values
	}
	return fmt.Sprintf("%.1f %s", float64(bytes)/float64(div), units[exp])
}

// formatDuration formats duration in human readable format
func formatDuration(d time.Duration) string {
	if d <= 0 {
		return "0s"
	}
	
	if d < time.Minute {
		return fmt.Sprintf("%.0fs", d.Seconds())
	}
	
	if d < time.Hour {
		minutes := int(d.Minutes())
		seconds := int(d.Seconds()) - (minutes * 60)
		return fmt.Sprintf("%dm %ds", minutes, seconds)
	}
	
	hours := int(d.Hours())
	minutes := int(d.Minutes()) - (hours * 60)
	return fmt.Sprintf("%dh %dm", hours, minutes)
}

// JSONProgressListener outputs progress as JSON for automation
type JSONProgressListener struct {
	outputFunc func(update ProgressUpdate)
}

// NewJSONProgressListener creates a JSON progress listener
func NewJSONProgressListener(outputFunc func(update ProgressUpdate)) *JSONProgressListener {
	return &JSONProgressListener{
		outputFunc: outputFunc,
	}
}

// OnProgress implements ProgressListener for JSON output
func (j *JSONProgressListener) OnProgress(update ProgressUpdate) {
	if j.outputFunc != nil {
		j.outputFunc(update)
	}
}

// OnComplete implements ProgressListener for JSON output
func (j *JSONProgressListener) OnComplete(update ProgressUpdate) {
	if j.outputFunc != nil {
		j.outputFunc(update)
	}
}

// OnError implements ProgressListener for JSON output
func (j *JSONProgressListener) OnError(err error, update ProgressUpdate) {
	if j.outputFunc != nil {
		j.outputFunc(update)
	}
}