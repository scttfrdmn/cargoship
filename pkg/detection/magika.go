// Package detection provides AI-powered file type detection using Magika (Issue #30)
package detection

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
	"os/exec"
	"sync"
	"time"

	"github.com/scttfrdmn/cargoship/pkg/config"
)

// MagikaDetector wraps the Magika CLI tool for AI-powered file type detection
type MagikaDetector struct {
	config     config.MagikaConfig
	binaryPath string
	cache      map[string]*MagikaResult
	cacheMu    sync.RWMutex
	available  bool
}

// MagikaResult represents Magika's detection output for a single file
type MagikaResult struct {
	Path   string              `json:"path"`
	Result MagikaResultWrapper `json:"result,omitempty"`
}

// MagikaResultWrapper wraps the result with status
type MagikaResultWrapper struct {
	Status string             `json:"status"`
	Value  MagikaValueWrapper `json:"value,omitempty"`
}

// MagikaValueWrapper contains the output value
type MagikaValueWrapper struct {
	Output MagikaOutput `json:"output"`
}

// MagikaOutput contains the detected content type information
type MagikaOutput struct {
	CTLabel     string  `json:"label"`
	MimeType    string  `json:"mime_type"`
	Score       float64 `json:"score"`
	Description string  `json:"description,omitempty"`
}

// NewMagikaDetector creates a new Magika detector
func NewMagikaDetector(cfg config.MagikaConfig) (*MagikaDetector, error) {
	if !cfg.Enabled {
		return &MagikaDetector{
			config:    cfg,
			available: false,
		}, nil
	}

	// Discover binary path
	binaryPath := cfg.BinaryPath
	if binaryPath == "" {
		path, err := exec.LookPath("magika")
		if err != nil {
			return nil, fmt.Errorf("magika not found in PATH: %w", err)
		}
		binaryPath = path
	}

	// Verify binary exists and is executable
	if err := verifyBinary(binaryPath); err != nil {
		return nil, fmt.Errorf("magika binary validation failed: %w", err)
	}

	detector := &MagikaDetector{
		config:     cfg,
		binaryPath: binaryPath,
		available:  true,
	}

	if cfg.EnableCache {
		detector.cache = make(map[string]*MagikaResult)
	}

	return detector, nil
}

// DetectBatch detects file types for multiple files in batch mode
func (md *MagikaDetector) DetectBatch(ctx context.Context, paths []string) (map[string]*MagikaResult, error) {
	if !md.available || len(paths) == 0 {
		return make(map[string]*MagikaResult), nil
	}

	// Check cache first
	if md.config.EnableCache {
		cached, uncached := md.getCachedResults(paths)
		if len(uncached) == 0 {
			// All results cached
			return cached, nil
		}
		// Detect uncached files
		if len(uncached) > 0 {
			uncachedResults, err := md.detectUncached(ctx, uncached)
			if err != nil {
				return cached, err
			}
			// Merge cached and uncached
			for k, v := range uncachedResults {
				cached[k] = v
			}
			return cached, nil
		}
	}

	// No cache, detect all
	return md.detectUncached(ctx, paths)
}

// detectUncached runs Magika on uncached files
func (md *MagikaDetector) detectUncached(ctx context.Context, paths []string) (map[string]*MagikaResult, error) {
	// Build magika command
	// Note: --json already includes MIME type and score, so no need for extra flags
	args := []string{"--json"}
	args = append(args, paths...)

	// Execute with timeout
	timeout, _ := time.ParseDuration(md.config.Timeout)
	cmdCtx, cancel := context.WithTimeout(ctx, timeout)
	defer cancel()

	cmd := exec.CommandContext(cmdCtx, md.binaryPath, args...)
	output, err := cmd.Output()
	if err != nil {
		// Check if it's a context timeout
		if cmdCtx.Err() == context.DeadlineExceeded {
			return nil, fmt.Errorf("magika execution timed out after %s", md.config.Timeout)
		}
		return nil, fmt.Errorf("magika execution failed: %w", err)
	}

	// Parse JSON output
	var results []MagikaResult
	if err := json.Unmarshal(output, &results); err != nil {
		return nil, fmt.Errorf("failed to parse magika output: %w", err)
	}

	// Convert to map and cache
	resultMap := make(map[string]*MagikaResult, len(results))
	for i := range results {
		resultMap[results[i].Path] = &results[i]
		if md.config.EnableCache {
			md.cacheResult(&results[i])
		}
	}

	return resultMap, nil
}

// DetectSingle detects file type for a single file
func (md *MagikaDetector) DetectSingle(ctx context.Context, path string) (*MagikaResult, error) {
	results, err := md.DetectBatch(ctx, []string{path})
	if err != nil {
		return nil, err
	}
	if result, ok := results[path]; ok {
		return result, nil
	}
	return nil, fmt.Errorf("no result for path: %s", path)
}

// IsAvailable returns whether Magika is available for use
func (md *MagikaDetector) IsAvailable() bool {
	return md.available
}

// getCachedResults retrieves cached results for paths, returning cached and uncached paths
func (md *MagikaDetector) getCachedResults(paths []string) (map[string]*MagikaResult, []string) {
	md.cacheMu.RLock()
	defer md.cacheMu.RUnlock()

	cached := make(map[string]*MagikaResult)
	uncached := make([]string, 0)

	for _, path := range paths {
		if result, exists := md.cache[path]; exists {
			cached[path] = result
		} else {
			uncached = append(uncached, path)
		}
	}

	return cached, uncached
}

// cacheResult stores a result in cache
func (md *MagikaDetector) cacheResult(result *MagikaResult) {
	md.cacheMu.Lock()
	defer md.cacheMu.Unlock()
	md.cache[result.Path] = result
}

// ClearCache clears the detection cache
func (md *MagikaDetector) ClearCache() {
	md.cacheMu.Lock()
	defer md.cacheMu.Unlock()
	md.cache = make(map[string]*MagikaResult)
}

// GetCacheStats returns cache statistics for debugging
func (md *MagikaDetector) GetCacheStats() map[string]interface{} {
	md.cacheMu.RLock()
	defer md.cacheMu.RUnlock()

	return map[string]interface{}{
		"enabled":       md.config.EnableCache,
		"cached_files":  len(md.cache),
		"available":     md.available,
		"binary_path":   md.binaryPath,
		"batch_size":    md.config.BatchSize,
		"use_mime_type": md.config.UseMimeType,
	}
}

// verifyBinary checks if the binary exists and is executable
func verifyBinary(path string) error {
	info, err := os.Stat(path)
	if err != nil {
		return err
	}
	if info.IsDir() {
		return fmt.Errorf("path is a directory, not a file")
	}
	// Check executable bit (Unix-like systems)
	// On Windows, this check is less relevant but won't fail
	if info.Mode()&0111 == 0 {
		// Not executable, but on Windows this might be expected
		// Only return error if we can verify it's truly not executable
		if _, err := exec.LookPath(path); err != nil {
			return fmt.Errorf("binary is not executable")
		}
	}
	return nil
}
