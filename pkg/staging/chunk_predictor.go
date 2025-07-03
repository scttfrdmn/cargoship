package staging

import (
	"bytes"
	"crypto/md5"
	"fmt"
	"io"
	"math"
	"sync"
)

// NewChunkBoundaryPredictor creates a new chunk boundary predictor.
func NewChunkBoundaryPredictor(config *StagingConfig) *ChunkBoundaryPredictor {
	return &ChunkBoundaryPredictor{
		contentAnalyzer:   NewContentAnalyzer(config),
		boundaryDetector:  NewBoundaryDetector(config),
		compressionRatio:  NewCompressionRatioPredictor(config),
		historicalData:    &ChunkPerformanceHistory{},
		config:            config,
	}
}

// PredictBoundaries predicts optimal chunk boundaries for the given content.
func (cbp *ChunkBoundaryPredictor) PredictBoundaries(reader io.Reader, contentType string, expectedSize int64) ([]ChunkBoundary, error) {
	cbp.mu.Lock()
	defer cbp.mu.Unlock()
	
	// Analyze content characteristics
	contentProfile, err := cbp.contentAnalyzer.AnalyzeContent(reader, contentType)
	if err != nil {
		return nil, err
	}
	
	// Generate boundary candidates
	candidates := cbp.boundaryDetector.GenerateCandidates(contentProfile, expectedSize)
	
	// Score boundaries based on compression and alignment
	scoredBoundaries := make([]ChunkBoundary, 0, len(candidates))
	for _, candidate := range candidates {
		// Predict compression ratio for this boundary
		compressionScore := cbp.compressionRatio.PredictRatio(candidate, contentProfile)
		candidate.CompressionScore = compressionScore
		
		// Calculate alignment score
		alignmentScore := cbp.boundaryDetector.CalculateAlignmentScore(candidate, contentProfile)
		candidate.PredictedRatio = alignmentScore
		
		scoredBoundaries = append(scoredBoundaries, candidate)
	}
	
	// Sort by composite score
	cbp.rankBoundaries(scoredBoundaries)
	
	return scoredBoundaries, nil
}

// rankBoundaries ranks boundaries by their predicted performance.
func (cbp *ChunkBoundaryPredictor) rankBoundaries(boundaries []ChunkBoundary) {
	// Sort boundaries by composite score (compression + alignment + historical performance)
	for i := 0; i < len(boundaries); i++ {
		for j := i + 1; j < len(boundaries); j++ {
			scoreI := cbp.calculateCompositeScore(boundaries[i])
			scoreJ := cbp.calculateCompositeScore(boundaries[j])
			if scoreI < scoreJ {
				boundaries[i], boundaries[j] = boundaries[j], boundaries[i]
			}
		}
	}
}

// calculateCompositeScore calculates a composite score for a boundary.
func (cbp *ChunkBoundaryPredictor) calculateCompositeScore(boundary ChunkBoundary) float64 {
	compressionWeight := 0.4
	alignmentWeight := 0.3
	sizeWeight := 0.3
	
	// Normalize size score (prefer sizes close to target)
	targetSize := float64(cbp.config.TargetChunkSizeMB * 1024 * 1024)
	sizeDiff := math.Abs(float64(boundary.Size) - targetSize)
	sizeScore := 1.0 - (sizeDiff / targetSize)
	if sizeScore < 0 {
		sizeScore = 0
	}
	
	return (boundary.CompressionScore * compressionWeight) +
		(boundary.PredictedRatio * alignmentWeight) +
		(sizeScore * sizeWeight)
}

// ContentAnalyzer implementation

// NewContentAnalyzer creates a new content analyzer.
func NewContentAnalyzer(config *StagingConfig) *ContentAnalyzer {
	return &ContentAnalyzer{
		entropyCalculator: NewEntropyCalculator(),
		patternDetector:   NewContentPatternDetector(),
		typeClassifier:    NewContentTypeClassifier(),
		windowBuffer:      make([]byte, config.ContentAnalysisWindow*1024),
		analysisWindow:    config.ContentAnalysisWindow * 1024,
	}
}

// ContentProfile represents the analyzed characteristics of content.
type ContentProfile struct {
	ContentType      string
	Entropy          float64
	Patterns         []ContentPattern
	CompressionHints []CompressionHint
	FileAlignment    []FileAlignment
	EstimatedRatio   float64
	AnalysisQuality  float64
}

// ContentPattern represents detected patterns in content.
type ContentPattern struct {
	Type        PatternType
	Offset      int64
	Length      int64
	Frequency   float64
	Compressibility float64
}

// PatternType represents different types of content patterns.
type PatternType int

const (
	PatternRepetitive PatternType = iota
	PatternRandom
	PatternStructured
	PatternBinary
	PatternText
)

// CompressionHint provides hints for optimal compression.
type CompressionHint struct {
	Algorithm    string
	WindowSize   int
	Dictionary   []byte
	EstimatedRatio float64
}

// FileAlignment represents file boundary information.
type FileAlignment struct {
	Offset   int64
	FileName string
	FileSize int64
	FileType string
}

// AnalyzeContent analyzes content characteristics for boundary prediction.
func (ca *ContentAnalyzer) AnalyzeContent(reader io.Reader, contentType string) (*ContentProfile, error) {
	profile := &ContentProfile{
		ContentType: contentType,
		Patterns:    make([]ContentPattern, 0),
		CompressionHints: make([]CompressionHint, 0),
		FileAlignment: make([]FileAlignment, 0),
	}
	
	// Analyze content in windows
	offset := int64(0)
	buffer := make([]byte, ca.analysisWindow)
	
	for {
		n, err := reader.Read(buffer)
		if err == io.EOF {
			break
		}
		if err != nil {
			return nil, err
		}
		
		// Analyze this window
		windowProfile := ca.analyzeWindow(buffer[:n], offset)
		ca.mergeWindowProfile(profile, windowProfile)
		
		offset += int64(n)
		
		// Limit analysis to prevent excessive processing
		if offset > 10*1024*1024 { // 10MB analysis limit
			break
		}
	}
	
	// Calculate overall metrics
	profile.Entropy = ca.entropyCalculator.CalculateOverallEntropy(profile.Patterns)
	profile.EstimatedRatio = ca.estimateCompressionRatio(profile)
	profile.AnalysisQuality = ca.calculateAnalysisQuality(profile, offset)
	
	return profile, nil
}

// analyzeWindow analyzes a single content window.
func (ca *ContentAnalyzer) analyzeWindow(data []byte, offset int64) *ContentProfile {
	window := &ContentProfile{
		Patterns: make([]ContentPattern, 0),
		CompressionHints: make([]CompressionHint, 0),
		FileAlignment: make([]FileAlignment, 0),
	}
	
	// Calculate entropy for this window
	entropy := ca.entropyCalculator.CalculateEntropy(data)
	
	// Detect patterns
	patterns := ca.patternDetector.DetectPatterns(data, offset)
	window.Patterns = append(window.Patterns, patterns...)
	
	// Classify content type
	classifiedType := ca.typeClassifier.ClassifyContent(data)
	window.ContentType = classifiedType
	
	// Detect file boundaries (for tar archives)
	alignments := ca.detectFileAlignments(data, offset)
	window.FileAlignment = append(window.FileAlignment, alignments...)
	
	window.Entropy = entropy
	
	return window
}

// mergeWindowProfile merges a window profile into the overall profile.
func (ca *ContentAnalyzer) mergeWindowProfile(overall, window *ContentProfile) {
	overall.Patterns = append(overall.Patterns, window.Patterns...)
	overall.CompressionHints = append(overall.CompressionHints, window.CompressionHints...)
	overall.FileAlignment = append(overall.FileAlignment, window.FileAlignment...)
}

// detectFileAlignments detects file boundaries within tar archives.
func (ca *ContentAnalyzer) detectFileAlignments(data []byte, offset int64) []FileAlignment {
	alignments := make([]FileAlignment, 0)
	
	// Look for tar header signatures
	for i := 0; i < len(data)-512; i += 512 {
		if ca.isTarHeader(data[i:i+512]) {
			// Extract file information from tar header
			alignment := ca.parseTarHeader(data[i:i+512], offset+int64(i))
			if alignment.FileName != "" {
				alignments = append(alignments, alignment)
			}
		}
	}
	
	return alignments
}

// isTarHeader checks if the given data looks like a tar header.
func (ca *ContentAnalyzer) isTarHeader(data []byte) bool {
	if len(data) < 512 {
		return false
	}
	
	// Check tar magic number at offset 257
	magic := string(data[257:262])
	return magic == "ustar"
}

// parseTarHeader extracts file information from a tar header.
func (ca *ContentAnalyzer) parseTarHeader(data []byte, offset int64) FileAlignment {
	if len(data) < 512 {
		return FileAlignment{}
	}
	
	// Extract filename (first 100 bytes)
	nameBytes := data[0:100]
	nameEnd := bytes.IndexByte(nameBytes, 0)
	if nameEnd == -1 {
		nameEnd = 100
	}
	fileName := string(nameBytes[:nameEnd])
	
	// Extract file size (12 bytes starting at offset 124)
	sizeBytes := data[124:136]
	sizeEnd := bytes.IndexByte(sizeBytes, 0)
	if sizeEnd == -1 {
		sizeEnd = 12
	}
	sizeStr := string(sizeBytes[:sizeEnd])
	
	// Parse octal size
	var fileSize int64
	for _, b := range sizeStr {
		if b >= '0' && b <= '7' {
			fileSize = fileSize*8 + int64(b-'0')
		}
	}
	
	return FileAlignment{
		Offset:   offset,
		FileName: fileName,
		FileSize: fileSize,
		FileType: ca.classifyFileType(fileName),
	}
}

// classifyFileType classifies file type based on extension.
func (ca *ContentAnalyzer) classifyFileType(fileName string) string {
	if fileName == "" {
		return "unknown"
	}
	
	// Simple file type classification
	lastDot := -1
	for i := len(fileName) - 1; i >= 0; i-- {
		if fileName[i] == '.' {
			lastDot = i
			break
		}
	}
	
	if lastDot == -1 {
		return "no_extension"
	}
	
	ext := fileName[lastDot+1:]
	switch ext {
	case "txt", "log", "csv", "json", "xml", "yaml", "yml":
		return "text"
	case "jpg", "jpeg", "png", "gif", "bmp", "tiff":
		return "image"
	case "mp4", "avi", "mov", "wmv", "flv":
		return "video"
	case "mp3", "wav", "flac", "ogg":
		return "audio"
	case "zip", "gz", "bz2", "xz", "7z":
		return "compressed"
	case "pdf", "doc", "docx", "ppt", "pptx":
		return "document"
	case "exe", "dll", "so", "dylib":
		return "binary"
	default:
		return "other"
	}
}

// estimateCompressionRatio estimates overall compression ratio.
func (ca *ContentAnalyzer) estimateCompressionRatio(profile *ContentProfile) float64 {
	if profile.Entropy == 0 {
		return 1.0 // No compression possible
	}
	
	// Base ratio from entropy (higher entropy = lower compression)
	baseRatio := 1.0 - (profile.Entropy / 8.0) // Entropy is 0-8 bits
	
	// Adjust based on content type
	switch profile.ContentType {
	case "text":
		baseRatio *= 0.3 // Text compresses very well
	case "image":
		baseRatio *= 0.8 // Images already compressed
	case "compressed":
		baseRatio *= 0.95 // Already compressed
	case "binary":
		baseRatio *= 0.6 // Moderate compression
	default:
		baseRatio *= 0.5 // Average compression
	}
	
	// Ensure ratio is reasonable
	if baseRatio < 0.1 {
		baseRatio = 0.1
	}
	if baseRatio > 0.9 {
		baseRatio = 0.9
	}
	
	return baseRatio
}

// calculateAnalysisQuality calculates the quality of the analysis.
func (ca *ContentAnalyzer) calculateAnalysisQuality(profile *ContentProfile, analyzedBytes int64) float64 {
	// Quality based on amount of data analyzed
	minBytes := int64(1024 * 1024) // 1MB minimum for good quality
	if analyzedBytes >= minBytes {
		return 1.0
	}
	
	return float64(analyzedBytes) / float64(minBytes)
}

// EntropyCalculator calculates content entropy for compressibility estimation.
type EntropyCalculator struct {
}

// NewEntropyCalculator creates a new entropy calculator.
func NewEntropyCalculator() *EntropyCalculator {
	return &EntropyCalculator{}
}

// CalculateEntropy calculates Shannon entropy of the given data.
func (ec *EntropyCalculator) CalculateEntropy(data []byte) float64 {
	if len(data) == 0 {
		return 0
	}
	
	// Count byte frequencies
	freq := make(map[byte]int)
	for _, b := range data {
		freq[b]++
	}
	
	// Calculate entropy
	dataLen := float64(len(data))
	entropy := 0.0
	
	for _, count := range freq {
		if count == 0 {
			continue
		}
		p := float64(count) / dataLen
		entropy -= p * math.Log2(p)
	}
	
	return entropy
}

// CalculateOverallEntropy calculates entropy from pattern analysis.
func (ec *EntropyCalculator) CalculateOverallEntropy(patterns []ContentPattern) float64 {
	if len(patterns) == 0 {
		return 4.0 // Average entropy
	}
	
	// Weight entropy by pattern frequency and length
	totalEntropy := 0.0
	totalWeight := 0.0
	
	for _, pattern := range patterns {
		weight := float64(pattern.Length) * pattern.Frequency
		entropy := ec.patternTypeEntropy(pattern.Type)
		
		totalEntropy += entropy * weight
		totalWeight += weight
	}
	
	if totalWeight == 0 {
		return 4.0
	}
	
	return totalEntropy / totalWeight
}

// patternTypeEntropy returns typical entropy for different pattern types.
func (ec *EntropyCalculator) patternTypeEntropy(pType PatternType) float64 {
	switch pType {
	case PatternRepetitive:
		return 1.0 // Low entropy
	case PatternRandom:
		return 8.0 // High entropy
	case PatternStructured:
		return 3.0 // Medium-low entropy
	case PatternBinary:
		return 6.0 // Medium-high entropy
	case PatternText:
		return 4.0 // Medium entropy
	default:
		return 4.0 // Default medium entropy
	}
}

// ContentPatternDetector detects patterns in content for boundary optimization.
type ContentPatternDetector struct {
	patternCache map[string][]ContentPattern
	cacheSize    int
	mu           sync.RWMutex
}

// NewContentPatternDetector creates a new pattern detector.
func NewContentPatternDetector() *ContentPatternDetector {
	return &ContentPatternDetector{
		patternCache: make(map[string][]ContentPattern),
		cacheSize:    100, // Cache up to 100 pattern analyses
	}
}

// DetectPatterns detects content patterns in the given data.
func (cpd *ContentPatternDetector) DetectPatterns(data []byte, offset int64) []ContentPattern {
	cpd.mu.Lock()
	defer cpd.mu.Unlock()
	
	// Create cache key
	hasher := md5.New()
	hasher.Write(data[:min(len(data), 1024)]) // Hash first 1KB for cache key
	cacheKey := fmt.Sprintf("%x", hasher.Sum(nil))
	
	// Check cache
	if patterns, exists := cpd.patternCache[cacheKey]; exists {
		// Adjust offsets for cached patterns
		adjustedPatterns := make([]ContentPattern, len(patterns))
		for i, pattern := range patterns {
			adjustedPatterns[i] = pattern
			adjustedPatterns[i].Offset = offset + pattern.Offset
		}
		return adjustedPatterns
	}
	
	// Detect patterns
	patterns := cpd.detectPatternsInternal(data, offset)
	
	// Cache results (with relative offsets)
	if len(cpd.patternCache) < cpd.cacheSize {
		cachedPatterns := make([]ContentPattern, len(patterns))
		for i, pattern := range patterns {
			cachedPatterns[i] = pattern
			cachedPatterns[i].Offset = pattern.Offset - offset // Store relative offset
		}
		cpd.patternCache[cacheKey] = cachedPatterns
	}
	
	return patterns
}

// detectPatternsInternal performs the actual pattern detection.
func (cpd *ContentPatternDetector) detectPatternsInternal(data []byte, offset int64) []ContentPattern {
	patterns := make([]ContentPattern, 0)
	
	// Detect repetitive patterns
	repetitivePatterns := cpd.detectRepetitivePatterns(data, offset)
	patterns = append(patterns, repetitivePatterns...)
	
	// Detect structured patterns
	structuredPatterns := cpd.detectStructuredPatterns(data, offset)
	patterns = append(patterns, structuredPatterns...)
	
	// Detect random regions
	randomPatterns := cpd.detectRandomPatterns(data, offset)
	patterns = append(patterns, randomPatterns...)
	
	return patterns
}

// detectRepetitivePatterns detects repetitive byte sequences.
func (cpd *ContentPatternDetector) detectRepetitivePatterns(data []byte, offset int64) []ContentPattern {
	patterns := make([]ContentPattern, 0)
	minPatternLength := 16
	minRepetitions := 3
	
	for patternLen := minPatternLength; patternLen <= 256 && patternLen < len(data)/minRepetitions; patternLen++ {
		for start := 0; start <= len(data)-patternLen*minRepetitions; start++ {
			pattern := data[start : start+patternLen]
			repetitions := 1
			
			// Count consecutive repetitions
			pos := start + patternLen
			for pos+patternLen <= len(data) {
				if bytes.Equal(data[pos:pos+patternLen], pattern) {
					repetitions++
					pos += patternLen
				} else {
					break
				}
			}
			
			if repetitions >= minRepetitions {
				totalLength := int64(repetitions * patternLen)
				patterns = append(patterns, ContentPattern{
					Type:      PatternRepetitive,
					Offset:    offset + int64(start),
					Length:    totalLength,
					Frequency: float64(repetitions),
					Compressibility: 0.95, // Highly compressible
				})
				
				// Skip past this pattern
				start = pos - 1
			}
		}
	}
	
	return patterns
}

// detectStructuredPatterns detects structured data patterns.
func (cpd *ContentPatternDetector) detectStructuredPatterns(data []byte, offset int64) []ContentPattern {
	patterns := make([]ContentPattern, 0)
	
	// Look for JSON/XML-like structures
	structureChars := []byte{'{', '}', '[', ']', '<', '>', '"', ':'}
	structureCount := 0
	
	for _, b := range data {
		for _, sc := range structureChars {
			if b == sc {
				structureCount++
				break
			}
		}
	}
	
	structureDensity := float64(structureCount) / float64(len(data))
	if structureDensity > 0.05 { // 5% structure characters
		patterns = append(patterns, ContentPattern{
			Type:      PatternStructured,
			Offset:    offset,
			Length:    int64(len(data)),
			Frequency: structureDensity,
			Compressibility: 0.7, // Good compressibility
		})
	}
	
	return patterns
}

// detectRandomPatterns detects regions of high entropy (random data).
func (cpd *ContentPatternDetector) detectRandomPatterns(data []byte, offset int64) []ContentPattern {
	patterns := make([]ContentPattern, 0)
	windowSize := 1024
	
	for start := 0; start < len(data); start += windowSize {
		end := start + windowSize
		if end > len(data) {
			end = len(data)
		}
		
		window := data[start:end]
		entropy := NewEntropyCalculator().CalculateEntropy(window)
		
		// High entropy indicates random data
		if entropy > 7.0 {
			patterns = append(patterns, ContentPattern{
				Type:      PatternRandom,
				Offset:    offset + int64(start),
				Length:    int64(end - start),
				Frequency: 1.0,
				Compressibility: 0.1, // Poor compressibility
			})
		}
	}
	
	return patterns
}

// ContentTypeClassifier classifies content type based on byte patterns.
type ContentTypeClassifier struct {
	signatures map[string][]byte
}

// NewContentTypeClassifier creates a new content type classifier.
func NewContentTypeClassifier() *ContentTypeClassifier {
	return &ContentTypeClassifier{
		signatures: map[string][]byte{
			"text":       []byte("text"),
			"json":       []byte("{"),
			"xml":        []byte("<?xml"),
			"binary":     []byte{0x00},
			"image_jpeg": []byte{0xFF, 0xD8, 0xFF},
			"image_png":  []byte{0x89, 0x50, 0x4E, 0x47},
			"pdf":        []byte("%PDF"),
			"zip":        []byte{0x50, 0x4B, 0x03, 0x04},
		},
	}
}

// ClassifyContent classifies the content type of the given data.
func (ctc *ContentTypeClassifier) ClassifyContent(data []byte) string {
	if len(data) == 0 {
		return "empty"
	}
	
	// Check for binary signatures first
	for contentType, signature := range ctc.signatures {
		if len(data) >= len(signature) && bytes.Equal(data[:len(signature)], signature) {
			return contentType
		}
	}
	
	// Check for text content (printable ASCII)
	printableCount := 0
	for _, b := range data[:min(len(data), 1024)] {
		if (b >= 32 && b <= 126) || b == 9 || b == 10 || b == 13 {
			printableCount++
		}
	}
	
	printableRatio := float64(printableCount) / float64(min(len(data), 1024))
	if printableRatio > 0.8 {
		return "text"
	}
	
	return "binary"
}

// Utility function
func min(a, b int) int {
	if a < b {
		return a
	}
	return b
}