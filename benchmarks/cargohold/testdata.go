package main

import (
	"crypto/rand"
	"fmt"
	"log"
	"math"
	mrand "math/rand"
	"os"
	"path/filepath"
	"sync"
)

// ensureTestData generates or verifies test data for a scenario
func ensureTestData(dataDir string, spec ScenarioSpec) error {
	// Check if data already exists
	if exists, err := testDataExists(dataDir, spec); err != nil {
		return err
	} else if exists {
		log.Printf("   ✅ Test data already exists in %s", dataDir)
		return nil
	}

	log.Printf("   📁 Generating test data in %s...", dataDir)
	if err := os.MkdirAll(dataDir, 0755); err != nil {
		return fmt.Errorf("failed to create data directory: %w", err)
	}

	return generateTestData(dataDir, spec)
}

// testDataExists checks if test data directory contains expected files
func testDataExists(dataDir string, spec ScenarioSpec) (bool, error) {
	info, err := os.Stat(dataDir)
	if os.IsNotExist(err) {
		return false, nil
	}
	if err != nil {
		return false, err
	}

	if !info.IsDir() {
		return false, fmt.Errorf("%s is not a directory", dataDir)
	}

	// Count files
	count := 0
	err = filepath.Walk(dataDir, func(path string, info os.FileInfo, err error) error {
		if err != nil {
			return err
		}
		if !info.IsDir() {
			count++
		}
		return nil
	})

	if err != nil {
		return false, err
	}

	// Consider data complete if we have at least 90% of expected files
	threshold := int(float64(spec.FileCount) * 0.9)
	return count >= threshold, nil
}

// generateTestData creates realistic test files for benchmarking
func generateTestData(dataDir string, spec ScenarioSpec) error {
	// Create subdirectories for content types
	contentTypes := []string{"code", "documents", "images", "data", "misc"}
	for _, ct := range contentTypes {
		if err := os.MkdirAll(filepath.Join(dataDir, ct), 0755); err != nil {
			return err
		}
	}

	// Calculate files per content type (distribute evenly)
	filesPerType := spec.FileCount / len(contentTypes)

	// Use worker pool for parallel generation
	numWorkers := 10
	jobs := make(chan fileJob, numWorkers*2)
	errors := make(chan error, numWorkers)
	var wg sync.WaitGroup

	// Start workers
	for i := 0; i < numWorkers; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			for job := range jobs {
				if err := generateFile(job); err != nil {
					errors <- err
					return
				}
			}
		}()
	}

	// Generate jobs
	go func() {
		fileID := 0
		source := mrand.NewSource(12345) // Fixed seed for reproducibility
		rng := mrand.New(source)

		for i, contentType := range contentTypes {
			count := filesPerType
			if i == len(contentTypes)-1 {
				// Last type gets remaining files
				count = spec.FileCount - fileID
			}

			for j := 0; j < count; j++ {
				// Random file size within spec range
				size := spec.MinFileSize + int64(rng.Float64()*float64(spec.MaxFileSize-spec.MinFileSize))

				jobs <- fileJob{
					path:        filepath.Join(dataDir, contentType, fmt.Sprintf("file_%08d%s", fileID, getExtension(contentType))),
					size:        size,
					contentType: contentType,
					seed:        int64(fileID),
				}
				fileID++
			}
		}
		close(jobs)
	}()

	// Wait for completion
	go func() {
		wg.Wait()
		close(errors)
	}()

	// Check for errors
	for err := range errors {
		return err
	}

	log.Printf("   ✅ Generated %d files (~%s)", spec.FileCount, formatBytes(spec.TotalSize))
	return nil
}

type fileJob struct {
	path        string
	size        int64
	contentType string
	seed        int64
}

func generateFile(job fileJob) error {
	f, err := os.Create(job.path)
	if err != nil {
		return fmt.Errorf("failed to create file %s: %w", job.path, err)
	}
	defer f.Close()

	// Generate content based on type
	var data []byte
	switch job.contentType {
	case "code":
		data = generateCodeData(job.size, job.seed)
	case "documents":
		data = generateDocumentData(job.size, job.seed)
	case "images":
		data = generateImageData(job.size, job.seed)
	case "data":
		data = generateDataFileData(job.size, job.seed)
	default:
		data = generateMixedData(job.size, job.seed)
	}

	if _, err := f.Write(data); err != nil {
		return fmt.Errorf("failed to write file %s: %w", job.path, err)
	}

	return nil
}

func getExtension(contentType string) string {
	switch contentType {
	case "code":
		extensions := []string{".go", ".py", ".js", ".java", ".cpp"}
		return extensions[mrand.Intn(len(extensions))]
	case "documents":
		extensions := []string{".txt", ".md", ".pdf", ".docx"}
		return extensions[mrand.Intn(len(extensions))]
	case "images":
		extensions := []string{".jpg", ".png", ".bmp"}
		return extensions[mrand.Intn(len(extensions))]
	case "data":
		extensions := []string{".json", ".csv", ".xml", ".yaml"}
		return extensions[mrand.Intn(len(extensions))]
	default:
		return ".dat"
	}
}

// Data generators with realistic entropy characteristics

func generateCodeData(size int64, seed int64) []byte {
	// Source code: high compressibility due to repeated keywords
	data := make([]byte, size)
	keywords := [][]byte{
		[]byte("func "), []byte("var "), []byte("if "), []byte("for "),
		[]byte("return "), []byte("import "), []byte("const "),
	}

	source := mrand.NewSource(seed)
	rng := mrand.New(source)

	pos := int64(0)
	for pos < size {
		keyword := keywords[rng.Intn(len(keywords))]
		n := copy(data[pos:], keyword)
		pos += int64(n)

		// Add some variable names
		nameLen := 5 + rng.Intn(10)
		for i := 0; i < nameLen && pos < size; i++ {
			data[pos] = byte('a' + rng.Intn(26))
			pos++
		}

		// Add newline
		if pos < size {
			data[pos] = '\n'
			pos++
		}
	}

	return data
}

func generateDocumentData(size int64, seed int64) []byte {
	// Text documents: high compressibility
	data := make([]byte, size)
	lorem := []byte("Lorem ipsum dolor sit amet, consectetur adipiscing elit. ")

	for i := int64(0); i < size; i++ {
		data[i] = lorem[i%int64(len(lorem))]
	}

	return data
}

func generateImageData(size int64, seed int64) []byte {
	// Images: low compressibility (simulated JPEG data)
	data := make([]byte, size)

	// Mix of high-entropy (80%) and structured data (20%)
	source := mrand.NewSource(seed)
	rng := mrand.New(source)

	// Add JPEG header
	header := []byte{0xFF, 0xD8, 0xFF, 0xE0}
	copy(data, header)

	// Fill with pseudo-random data (high entropy)
	for i := int64(len(header)); i < size; i++ {
		data[i] = byte(rng.Intn(256))
	}

	return data
}

func generateDataFileData(size int64, seed int64) []byte {
	// Structured data files (JSON, CSV): moderate compressibility
	data := make([]byte, size)

	source := mrand.NewSource(seed)
	rng := mrand.New(source)

	// Simulate CSV-like data
	pos := int64(0)
	for pos < size {
		// Field values
		for j := 0; j < 5 && pos < size; j++ {
			if j > 0 {
				data[pos] = ','
				pos++
			}

			// Generate numeric or text value
			if rng.Float64() < 0.5 {
				// Numeric
				val := fmt.Sprintf("%d", rng.Intn(10000))
				n := copy(data[pos:], []byte(val))
				pos += int64(n)
			} else {
				// Text
				length := 5 + rng.Intn(10)
				for k := 0; k < length && pos < size; k++ {
					data[pos] = byte('a' + rng.Intn(26))
					pos++
				}
			}
		}

		if pos < size {
			data[pos] = '\n'
			pos++
		}
	}

	return data
}

func generateMixedData(size int64, seed int64) []byte {
	// Mixed content: average compressibility
	data := make([]byte, size)

	source := mrand.NewSource(seed)
	rng := mrand.New(source)

	// 50% structured, 50% random
	for i := int64(0); i < size; i++ {
		if i%2 == 0 {
			// Structured pattern
			data[i] = byte('A' + (i % 26))
		} else {
			// Random
			data[i] = byte(rng.Intn(256))
		}
	}

	return data
}

// generateEntropyData creates data with specified entropy level (0.0-1.0)
func generateEntropyData(size int64, entropy float64) []byte {
	data := make([]byte, size)

	if entropy < 0.1 {
		// Very low entropy: highly repetitive
		pattern := []byte("AAAAAAAAAA")
		for i := int64(0); i < size; i++ {
			data[i] = pattern[i%int64(len(pattern))]
		}
	} else if entropy > 0.9 {
		// Very high entropy: cryptographically random
		if _, err := rand.Read(data); err != nil {
			// Fallback to math/rand
			for i := range data {
				data[i] = byte(mrand.Intn(256))
			}
		}
	} else {
		// Medium entropy: mix of pattern and randomness
		patternSize := int(float64(size) * (1.0 - entropy))
		pattern := make([]byte, patternSize)
		for i := range pattern {
			pattern[i] = byte('A' + (i % 26))
		}

		for i := int64(0); i < size; i++ {
			if i < int64(patternSize) {
				data[i] = pattern[i]
			} else {
				data[i] = byte(mrand.Intn(256))
			}
		}
	}

	return data
}

// calculateActualEntropy computes Shannon entropy of data
func calculateActualEntropy(data []byte) float64 {
	if len(data) == 0 {
		return 0.0
	}

	// Count byte frequencies
	freq := make(map[byte]int)
	for _, b := range data {
		freq[b]++
	}

	// Calculate Shannon entropy
	var entropy float64
	total := float64(len(data))
	for _, count := range freq {
		if count > 0 {
			p := float64(count) / total
			entropy -= p * math.Log2(p)
		}
	}

	// Normalize to 0-1 range (max entropy is 8 bits for bytes)
	return entropy / 8.0
}
