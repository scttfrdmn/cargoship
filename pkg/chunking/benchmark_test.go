package chunking

import (
	"fmt"
	"testing"
	"time"
)

// Benchmark calculator performance
func BenchmarkChunkSizeCalculator_SmallFiles(b *testing.B) {
	config := &ChunkingConfig{
		Workers:         8,
		AvailableMemory: 4 * 1024 * 1024 * 1024,
	}

	calc := NewChunkSizeCalculator(config)

	totalSize := int64(185 * 1024 * 1024) // 185 MB
	fileCount := 10000
	costTarget := float64(1000)

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		_, _ = calc.CalculateOptimalChunkSize(
			totalSize,
			fileCount,
			config.AvailableMemory,
			costTarget,
		)
	}
}

func BenchmarkChunkSizeCalculator_LargeFiles(b *testing.B) {
	config := &ChunkingConfig{
		Workers:         8,
		AvailableMemory: 4 * 1024 * 1024 * 1024,
	}

	calc := NewChunkSizeCalculator(config)

	totalSize := int64(56 * 1024 * 1024 * 1024) // 56 GB
	fileCount := 100
	costTarget := float64(100)

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		_, _ = calc.CalculateOptimalChunkSize(
			totalSize,
			fileCount,
			config.AvailableMemory,
			costTarget,
		)
	}
}

// Benchmark file grouping performance - TARGET: >10,000 files/sec
func BenchmarkGroupFilesIntoChunks_10k_Files(b *testing.B) {
	config := &ChunkingConfig{
		GroupingStrategy: "size",
	}

	strategy := NewSizeBasedChunkingStrategy(config)

	// Create 10,000 test files
	files := make([]File, 10000)
	for i := range files {
		files[i] = File{
			Path: fmt.Sprintf("/file%d.txt", i),
			Size: 18 * 1024, // 18 KB average
		}
	}

	chunkSize := int64(20 * 1024 * 1024) // 20 MB chunks

	b.ResetTimer()
	start := time.Now()

	for i := 0; i < b.N; i++ {
		_, err := strategy.GroupFilesIntoChunks(files, chunkSize)
		if err != nil {
			b.Fatal(err)
		}
	}

	elapsed := time.Since(start)
	filesPerSec := float64(len(files)*b.N) / elapsed.Seconds()

	b.ReportMetric(filesPerSec, "files/sec")
	b.ReportMetric(float64(len(files)), "files")
}

func BenchmarkGroupFilesIntoChunks_100k_Files(b *testing.B) {
	config := &ChunkingConfig{
		GroupingStrategy: "size",
	}

	strategy := NewSizeBasedChunkingStrategy(config)

	// Create 100,000 test files
	files := make([]File, 100000)
	for i := range files {
		files[i] = File{
			Path: fmt.Sprintf("/file%d.txt", i),
			Size: 18 * 1024, // 18 KB average
		}
	}

	chunkSize := int64(20 * 1024 * 1024) // 20 MB chunks

	b.ResetTimer()
	start := time.Now()

	for i := 0; i < b.N; i++ {
		_, err := strategy.GroupFilesIntoChunks(files, chunkSize)
		if err != nil {
			b.Fatal(err)
		}
	}

	elapsed := time.Since(start)
	filesPerSec := float64(len(files)*b.N) / elapsed.Seconds()

	b.ReportMetric(filesPerSec, "files/sec")
	b.ReportMetric(float64(len(files)), "files")
}

func BenchmarkGroupFilesIntoChunks_MixedSizes(b *testing.B) {
	config := &ChunkingConfig{
		GroupingStrategy: "mixed",
	}

	strategy := NewAdaptiveChunkingStrategy(config)

	// Create 10,000 files with mixed sizes
	files := make([]File, 10000)
	for i := range files {
		// Mix of small (90%), medium (9%), and large (1%) files
		var size int64
		switch {
		case i < 9000:
			size = 10 * 1024 // 10 KB (small)
		case i < 9900:
			size = 1 * 1024 * 1024 // 1 MB (medium)
		default:
			size = 100 * 1024 * 1024 // 100 MB (large)
		}

		files[i] = File{
			Path:      fmt.Sprintf("/file%d.txt", i),
			Size:      size,
			Directory: fmt.Sprintf("/dir%d", i%10),
		}
	}

	chunkSize := int64(20 * 1024 * 1024) // 20 MB chunks

	b.ResetTimer()
	start := time.Now()

	for i := 0; i < b.N; i++ {
		_, err := strategy.GroupFilesIntoChunks(files, chunkSize)
		if err != nil {
			b.Fatal(err)
		}
	}

	elapsed := time.Since(start)
	filesPerSec := float64(len(files)*b.N) / elapsed.Seconds()

	b.ReportMetric(filesPerSec, "files/sec")
	b.ReportMetric(float64(len(files)), "files")
}

func BenchmarkGroupByDirectory(b *testing.B) {
	config := &ChunkingConfig{
		GroupingStrategy: "directory",
	}

	strategy := NewAdaptiveChunkingStrategy(config)

	// Create 10,000 files across 100 directories
	files := make([]File, 10000)
	for i := range files {
		files[i] = File{
			Path:      fmt.Sprintf("/dir%d/file%d.txt", i%100, i),
			Directory: fmt.Sprintf("/dir%d", i%100),
			Size:      18 * 1024, // 18 KB average
		}
	}

	chunkSize := int64(20 * 1024 * 1024) // 20 MB chunks

	b.ResetTimer()
	start := time.Now()

	for i := 0; i < b.N; i++ {
		_, err := strategy.GroupFilesIntoChunks(files, chunkSize)
		if err != nil {
			b.Fatal(err)
		}
	}

	elapsed := time.Since(start)
	filesPerSec := float64(len(files)*b.N) / elapsed.Seconds()

	b.ReportMetric(filesPerSec, "files/sec")
	b.ReportMetric(float64(len(files)), "files")
}

// Benchmark memory allocation patterns
func BenchmarkChunkAllocation(b *testing.B) {
	files := make([]File, 1000)
	for i := range files {
		files[i] = File{
			Path: fmt.Sprintf("/file%d.txt", i),
			Size: 10 * 1024, // 10 KB
		}
	}

	chunkSize := int64(1 * 1024 * 1024) // 1 MB chunks

	b.ResetTimer()
	b.ReportAllocs()

	for i := 0; i < b.N; i++ {
		chunks := []Chunk{}
		currentChunk := Chunk{ID: 0}

		for j := range files {
			file := files[j]

			if currentChunk.TotalSize+file.Size > chunkSize && len(currentChunk.Files) > 0 {
				chunks = append(chunks, currentChunk)
				currentChunk = Chunk{ID: len(chunks)}
			}

			currentChunk.Files = append(currentChunk.Files, file)
			currentChunk.TotalSize += file.Size
			currentChunk.FileCount++
		}

		if len(currentChunk.Files) > 0 {
			chunks = append(chunks, currentChunk)
		}

		// Prevent compiler optimization
		_ = chunks
	}
}

// Benchmark against Issue #52 targets
func BenchmarkIssue52_SmallFiles_Target(b *testing.B) {
	// Issue #52 Target: 10,000 files @ 185MB in ≤10s
	// Chunking should be < 1s of that budget

	config := &ChunkingConfig{
		Workers:           8,
		AvailableMemory:   4 * 1024 * 1024 * 1024,
		GroupingStrategy:  "size",
		CostSavingsTarget: 1000,
	}

	strategy := NewSizeBasedChunkingStrategy(config)
	calc := strategy.calculator

	// Create 10,000 files totaling 185MB
	files := make([]File, 10000)
	totalSize := int64(185 * 1024 * 1024)
	avgSize := totalSize / int64(len(files))

	for i := range files {
		files[i] = File{
			Path: fmt.Sprintf("/file%d.txt", i),
			Size: avgSize,
		}
	}

	b.ResetTimer()
	start := time.Now()

	for i := 0; i < b.N; i++ {
		// Calculate optimal chunk size
		chunkSize, stats := calc.CalculateOptimalChunkSize(
			totalSize,
			len(files),
			config.AvailableMemory,
			config.CostSavingsTarget,
		)

		// Group files into chunks
		chunks, err := strategy.GroupFilesIntoChunks(files, chunkSize)
		if err != nil {
			b.Fatal(err)
		}

		// Validate targets
		if i == 0 {
			b.Logf("Small files scenario:")
			b.Logf("  Chunk size: %d MB", chunkSize/(1024*1024))
			b.Logf("  Chunks created: %d", len(chunks))
			b.Logf("  Estimated ops: %d (vs %d direct)", stats.EstimatedOps, len(files))
			b.Logf("  Cost savings: %.1fx", stats.CostSavings)
			b.Logf("  Memory required: %d MB", stats.MemoryRequired/(1024*1024))
			b.Logf("  Target: ≤500 MB memory, ≤50 operations")
		}
	}

	elapsed := time.Since(start)
	perIteration := elapsed / time.Duration(b.N)

	b.ReportMetric(float64(perIteration.Milliseconds()), "ms/op")

	// TARGET: Should complete in < 1 second for chunking phase
	if perIteration > time.Second {
		b.Errorf("Chunking too slow: %v (target <1s)", perIteration)
	}
}

func BenchmarkIssue52_LargeFiles_Target(b *testing.B) {
	// Issue #52 Target: 100 files @ 56GB in ≤200s
	// Must not OOM, memory ≤4GB

	config := &ChunkingConfig{
		Workers:           8,
		AvailableMemory:   4 * 1024 * 1024 * 1024,
		GroupingStrategy:  "mixed",
		CostSavingsTarget: 100,
	}

	strategy := NewAdaptiveChunkingStrategy(config)
	calc := strategy.calculator

	// Create 100 files totaling 56GB
	files := make([]File, 100)
	totalSize := int64(56 * 1024 * 1024 * 1024)
	avgSize := totalSize / int64(len(files))

	for i := range files {
		files[i] = File{
			Path: fmt.Sprintf("/file%d.dat", i),
			Size: avgSize,
		}
	}

	b.ResetTimer()
	start := time.Now()

	for i := 0; i < b.N; i++ {
		// Calculate optimal chunk size
		chunkSize, stats := calc.CalculateOptimalChunkSize(
			totalSize,
			len(files),
			config.AvailableMemory,
			config.CostSavingsTarget,
		)

		// Group files into chunks
		chunks, err := strategy.GroupFilesIntoChunks(files, chunkSize)
		if err != nil {
			b.Fatal(err)
		}

		// Validate memory constraint
		if stats.MemoryRequired > config.AvailableMemory {
			b.Errorf("Memory exceeds limit: %d > %d",
				stats.MemoryRequired, config.AvailableMemory)
		}

		if i == 0 {
			b.Logf("Large files scenario:")
			b.Logf("  Chunk size: %d MB", chunkSize/(1024*1024))
			b.Logf("  Chunks created: %d", len(chunks))
			b.Logf("  Estimated ops: %d (vs %d direct)", stats.EstimatedOps, len(files))
			b.Logf("  Cost savings: %.1fx", stats.CostSavings)
			b.Logf("  Memory required: %d MB (limit: %d MB)",
				stats.MemoryRequired/(1024*1024),
				config.AvailableMemory/(1024*1024))
			b.Logf("  Target: ≤4GB memory, ≤200 operations")
		}
	}

	elapsed := time.Since(start)
	perIteration := elapsed / time.Duration(b.N)

	b.ReportMetric(float64(perIteration.Milliseconds()), "ms/op")
}
