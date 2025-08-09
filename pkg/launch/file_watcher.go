package launch

import (
	"context"
	"fmt"
	"log/slog"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"time"

	"github.com/fsnotify/fsnotify"
)

// FileWatcher monitors filesystem changes for archival candidates
type FileWatcher struct {
	logger     *slog.Logger
	watchPaths []WatchPath
	watcher    *fsnotify.Watcher
	events     chan FileEvent
	mu         sync.RWMutex

	// State
	watching map[string]bool
	ctx      context.Context
	cancel   context.CancelFunc
	wg       sync.WaitGroup
}

// FileEvent represents a filesystem event
type FileEvent struct {
	Path      string    `json:"path"`
	Operation string    `json:"operation"`
	Timestamp time.Time `json:"timestamp"`
	Size      int64     `json:"size,omitempty"`
	ModTime   time.Time `json:"mod_time,omitempty"`
}

// NewFileWatcher creates a new file system watcher
func NewFileWatcher(watchPaths []WatchPath, logger *slog.Logger) (*FileWatcher, error) {
	if logger == nil {
		logger = slog.Default()
	}

	watcher, err := fsnotify.NewWatcher()
	if err != nil {
		return nil, fmt.Errorf("failed to create filesystem watcher: %w", err)
	}

	ctx, cancel := context.WithCancel(context.Background())

	fw := &FileWatcher{
		logger:     logger.With("component", "file-watcher"),
		watchPaths: watchPaths,
		watcher:    watcher,
		events:     make(chan FileEvent, 1000),
		watching:   make(map[string]bool),
		ctx:        ctx,
		cancel:     cancel,
	}

	return fw, nil
}

// Start begins watching configured paths
func (fw *FileWatcher) Start() error {
	fw.logger.Info("Starting file watcher", "paths", len(fw.watchPaths))

	// Add watch paths
	for _, watchPath := range fw.watchPaths {
		if err := fw.addWatchPath(watchPath); err != nil {
			fw.logger.Error("Failed to add watch path",
				"path", watchPath.Path,
				"error", err)
			continue
		}
	}

	// Start event processor
	fw.wg.Add(1)
	go fw.processEvents()

	fw.logger.Info("File watcher started successfully")
	return nil
}

// Stop gracefully stops the file watcher
func (fw *FileWatcher) Stop() error {
	fw.logger.Info("Stopping file watcher")

	fw.cancel()

	if fw.watcher != nil {
		if err := fw.watcher.Close(); err != nil {
			fw.logger.Error("Failed to close filesystem watcher", "error", err)
		}
	}

	// Wait for goroutines to finish
	done := make(chan struct{})
	go func() {
		fw.wg.Wait()
		close(done)
	}()

	select {
	case <-done:
		fw.logger.Info("File watcher stopped gracefully")
	case <-time.After(10 * time.Second):
		fw.logger.Warn("File watcher shutdown timed out")
	}

	return nil
}

// Events returns the channel for receiving file events
func (fw *FileWatcher) Events() <-chan FileEvent {
	return fw.events
}

// addWatchPath adds a path to the filesystem watcher
func (fw *FileWatcher) addWatchPath(watchPath WatchPath) error {
	// Check if path exists
	info, err := os.Stat(watchPath.Path)
	if err != nil {
		return fmt.Errorf("watch path does not exist: %w", err)
	}

	if !info.IsDir() {
		return fmt.Errorf("watch path is not a directory: %s", watchPath.Path)
	}

	// Add to watcher
	if err := fw.watcher.Add(watchPath.Path); err != nil {
		return fmt.Errorf("failed to add path to watcher: %w", err)
	}

	fw.mu.Lock()
	fw.watching[watchPath.Path] = true
	fw.mu.Unlock()

	// If recursive, add subdirectories
	if watchPath.Recursive {
		if err := fw.addSubdirectories(watchPath.Path); err != nil {
			fw.logger.Warn("Failed to add some subdirectories",
				"path", watchPath.Path,
				"error", err)
		}
	}

	fw.logger.Info("Added watch path",
		"path", watchPath.Path,
		"recursive", watchPath.Recursive)

	return nil
}

// addSubdirectories recursively adds subdirectories to watcher
func (fw *FileWatcher) addSubdirectories(root string) error {
	return filepath.Walk(root, func(path string, info os.FileInfo, err error) error {
		if err != nil {
			return nil // Skip errors, continue walking
		}

		if info.IsDir() && path != root {
			if err := fw.watcher.Add(path); err != nil {
				fw.logger.Debug("Failed to add subdirectory",
					"path", path,
					"error", err)
			} else {
				fw.mu.Lock()
				fw.watching[path] = true
				fw.mu.Unlock()
			}
		}

		return nil
	})
}

// processEvents processes filesystem events
func (fw *FileWatcher) processEvents() {
	defer fw.wg.Done()

	fw.logger.Info("File watcher event processor started")

	for {
		select {
		case <-fw.ctx.Done():
			return

		case event, ok := <-fw.watcher.Events:
			if !ok {
				return
			}
			fw.handleFsnotifyEvent(event)

		case err, ok := <-fw.watcher.Errors:
			if !ok {
				return
			}
			fw.logger.Error("Filesystem watcher error", "error", err)
		}
	}
}

// handleFsnotifyEvent converts fsnotify events to FileEvent
func (fw *FileWatcher) handleFsnotifyEvent(event fsnotify.Event) {
	// Skip temporary files and hidden files
	if fw.shouldIgnoreFile(event.Name) {
		return
	}

	// Check if this path matches any watch criteria
	if !fw.matchesWatchCriteria(event.Name) {
		return
	}

	// Get file info
	var size int64
	var modTime time.Time
	if info, err := os.Stat(event.Name); err == nil {
		size = info.Size()
		modTime = info.ModTime()
	}

	// Convert to FileEvent
	fileEvent := FileEvent{
		Path:      event.Name,
		Operation: fw.fsnotifyOpToString(event.Op),
		Timestamp: time.Now(),
		Size:      size,
		ModTime:   modTime,
	}

	// Send event (non-blocking)
	select {
	case fw.events <- fileEvent:
		fw.logger.Debug("File event processed",
			"path", fileEvent.Path,
			"operation", fileEvent.Operation)
	default:
		fw.logger.Warn("File event channel full, dropping event",
			"path", fileEvent.Path)
	}
}

// shouldIgnoreFile checks if a file should be ignored
func (fw *FileWatcher) shouldIgnoreFile(path string) bool {
	basename := filepath.Base(path)

	// Ignore hidden files
	if strings.HasPrefix(basename, ".") {
		return true
	}

	// Ignore temporary files
	if strings.HasSuffix(basename, ".tmp") ||
		strings.HasSuffix(basename, ".temp") ||
		strings.HasPrefix(basename, "~") {
		return true
	}

	// Ignore common system files
	systemFiles := []string{
		"Thumbs.db", "Desktop.ini", ".DS_Store",
		"$RECYCLE.BIN", "System Volume Information",
	}

	for _, sysFile := range systemFiles {
		if strings.Contains(path, sysFile) {
			return true
		}
	}

	return false
}

// matchesWatchCriteria checks if a file matches watch criteria
func (fw *FileWatcher) matchesWatchCriteria(path string) bool {
	for _, watchPath := range fw.watchPaths {
		// Check if file is under this watch path
		if !strings.HasPrefix(path, watchPath.Path) {
			continue
		}

		// Apply include patterns
		if len(watchPath.IncludePatterns) > 0 {
			matched := false
			for _, pattern := range watchPath.IncludePatterns {
				if patternMatched, _ := filepath.Match(pattern, filepath.Base(path)); patternMatched {
					matched = true
					break
				}
			}
			if !matched {
				continue
			}
		}

		// Apply exclude patterns
		excluded := false
		for _, pattern := range watchPath.ExcludePatterns {
			if matched, _ := filepath.Match(pattern, filepath.Base(path)); matched {
				excluded = true
				break
			}
		}
		if excluded {
			continue
		}

		// Check minimum age if specified
		if watchPath.MinAge > 0 {
			if info, err := os.Stat(path); err == nil {
				if time.Since(info.ModTime()) < watchPath.MinAge {
					continue
				}
			}
		}

		return true
	}

	return false
}

// fsnotifyOpToString converts fsnotify operation to string
func (fw *FileWatcher) fsnotifyOpToString(op fsnotify.Op) string {
	switch {
	case op&fsnotify.Create == fsnotify.Create:
		return "create"
	case op&fsnotify.Write == fsnotify.Write:
		return "write"
	case op&fsnotify.Remove == fsnotify.Remove:
		return "remove"
	case op&fsnotify.Rename == fsnotify.Rename:
		return "rename"
	case op&fsnotify.Chmod == fsnotify.Chmod:
		return "chmod"
	default:
		return "unknown"
	}
}

// GetWatchedPaths returns currently watched paths
func (fw *FileWatcher) GetWatchedPaths() []string {
	fw.mu.RLock()
	defer fw.mu.RUnlock()

	paths := make([]string, 0, len(fw.watching))
	for path := range fw.watching {
		paths = append(paths, path)
	}

	return paths
}

// ScanDirectory performs a one-time scan of a directory for files
func (fw *FileWatcher) ScanDirectory(path string, patterns []string) ([]string, error) {
	var matches []string

	err := filepath.Walk(path, func(filePath string, info os.FileInfo, err error) error {
		if err != nil {
			return nil // Skip errors, continue walking
		}

		if info.IsDir() {
			return nil
		}

		// Skip ignored files
		if fw.shouldIgnoreFile(filePath) {
			return nil
		}

		// Check patterns
		if len(patterns) == 0 {
			matches = append(matches, filePath)
			return nil
		}

		for _, pattern := range patterns {
			if matched, _ := filepath.Match(pattern, filepath.Base(filePath)); matched {
				matches = append(matches, filePath)
				break
			}
		}

		return nil
	})

	return matches, err
}
