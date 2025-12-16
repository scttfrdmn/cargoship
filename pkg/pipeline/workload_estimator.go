package pipeline

import (
	"context"
	"io/fs"
	"path/filepath"
	"runtime"
	"strings"
)

// WorkerCounts holds the calculated worker counts for each pipeline stage
type WorkerCounts struct {
	Scanner  int
	Archiver int
	Uploader int
}

// EstimateWorkload performs a fast metadata-only scan to count files and estimate total size.
// It skips hidden files (starting with .) to match scanner behavior.
// Returns file count and total size in bytes.
func EstimateWorkload(ctx context.Context, sourcePath string) (fileCount int64, totalSize int64, err error) {
	var count, size int64

	err = filepath.WalkDir(sourcePath, func(path string, d fs.DirEntry, err error) error {
		// Check for context cancellation
		select {
		case <-ctx.Done():
			return ctx.Err()
		default:
		}

		if err != nil {
			// If this is the root path, propagate the error
			if path == sourcePath {
				return err
			}
			// For individual files, log but continue - don't fail entire scan
			return nil
		}

		// Skip directories
		if d.IsDir() {
			return nil
		}

		// Skip hidden files and system files (matches scanner behavior)
		name := d.Name()
		if strings.HasPrefix(name, ".") {
			return nil
		}

		// Get file info for size
		info, err := d.Info()
		if err != nil {
			// Log but continue - don't fail entire scan
			return nil
		}

		count++
		size += info.Size()

		return nil
	})

	return count, size, err
}

// CalculateOptimalWorkers determines optimal worker counts based on workload size and CPU cores.
// The strategy is:
// - Small workloads (<100 files): Minimal workers to reduce overhead
// - Light workloads (100-1000 files): Low worker counts
// - Medium workloads (1000-10000 files): Current defaults
// - Large workloads (10k+ files): Scale with CPU cores up to maximum limits
func CalculateOptimalWorkers(fileCount int64, totalSize int64) WorkerCounts {
	cores := runtime.NumCPU()

	// Small workload: < 100 files
	// Use minimal workers to reduce goroutine overhead
	if fileCount < 100 {
		return WorkerCounts{
			Scanner:  1,
			Archiver: 2,
			Uploader: 2,
		}
	}

	// Light workload: 100-1000 files
	// Use low worker counts
	if fileCount < 1000 {
		return WorkerCounts{
			Scanner:  2,
			Archiver: 4,
			Uploader: 2,
		}
	}

	// Medium workload: 1000-10000 files
	// Use current defaults (baseline performance)
	if fileCount < 10000 {
		return WorkerCounts{
			Scanner:  4,
			Archiver: 8,
			Uploader: 4,
		}
	}

	// Large workload: 10k+ files
	// Scale with CPU cores, respecting maximums for each stage type:
	// - Scanner: I/O bound, diminishing returns beyond 8 workers
	// - Archiver: CPU bound, benefits from parallelism up to 16 workers
	// - Uploader: Network bound, limited by S3 rate limits at 8 workers
	return WorkerCounts{
		Scanner:  min(cores/2, 8), // I/O bound
		Archiver: min(cores, 16),  // CPU bound
		Uploader: min(cores/2, 8), // Network bound
	}
}

// min returns the minimum of two integers
func min(a, b int) int {
	if a < b {
		return a
	}
	return b
}
