package staging

import (
	"bytes"
	"strings"
	"testing"
)

func TestNewChunkBoundaryPredictor(t *testing.T) {
	config := &StagingConfig{
		TargetChunkSizeMB:     32,
		ContentAnalysisWindow: 16,
	}
	
	predictor := NewChunkBoundaryPredictor(config)
	
	if predictor == nil {
		t.Fatal("Expected non-nil ChunkBoundaryPredictor")
	}
	
	if predictor.config != config {
		t.Error("Expected config to be set correctly")
	}
	
	if predictor.contentAnalyzer == nil {
		t.Error("Expected contentAnalyzer to be initialized")
	}
	
	if predictor.boundaryDetector == nil {
		t.Error("Expected boundaryDetector to be initialized")
	}
	
	if predictor.compressionRatio == nil {
		t.Error("Expected compressionRatio to be initialized")
	}
	
	if predictor.historicalData == nil {
		t.Error("Expected historicalData to be initialized")
	}
}

func TestChunkBoundaryPredictor_PredictBoundaries(t *testing.T) {
	config := &StagingConfig{
		TargetChunkSizeMB:     32,
		ContentAnalysisWindow: 16,
	}
	
	predictor := NewChunkBoundaryPredictor(config)
	
	// Test with text content
	textContent := strings.Repeat("Hello, world! ", 1000)
	reader := strings.NewReader(textContent)
	
	boundaries, err := predictor.PredictBoundaries(reader, "text/plain", int64(len(textContent)))
	if err != nil {
		t.Fatalf("Failed to predict boundaries: %v", err)
	}
	
	if len(boundaries) == 0 {
		t.Error("Expected at least one boundary to be predicted")
	}
	
	// Verify boundaries are sorted by score
	for i := 1; i < len(boundaries); i++ {
		score1 := predictor.calculateCompositeScore(boundaries[i-1])
		score2 := predictor.calculateCompositeScore(boundaries[i])
		if score1 < score2 {
			t.Error("Expected boundaries to be sorted by composite score (descending)")
		}
	}
}

func TestChunkBoundaryPredictor_PredictBoundariesEmptyContent(t *testing.T) {
	config := &StagingConfig{
		TargetChunkSizeMB:     32,
		ContentAnalysisWindow: 16,
	}
	
	predictor := NewChunkBoundaryPredictor(config)
	
	// Test with empty content
	reader := strings.NewReader("")
	
	boundaries, err := predictor.PredictBoundaries(reader, "text/plain", 0)
	if err != nil {
		t.Fatalf("Failed to predict boundaries for empty content: %v", err)
	}
	
	// Should handle empty content gracefully
	if boundaries == nil {
		t.Error("Expected non-nil boundaries slice even for empty content")
	}
}

func TestChunkBoundaryPredictor_calculateCompositeScore(t *testing.T) {
	config := &StagingConfig{
		TargetChunkSizeMB:     32,
		ContentAnalysisWindow: 16,
	}
	
	predictor := NewChunkBoundaryPredictor(config)
	
	boundary := ChunkBoundary{
		StartOffset:      0,
		EndOffset:        32 * 1024 * 1024,
		Size:             32 * 1024 * 1024, // Target size
		CompressionScore: 0.8,
		PredictedRatio:   0.7,
	}
	
	score := predictor.calculateCompositeScore(boundary)
	
	// Score should be between 0 and 1
	if score < 0 || score > 1 {
		t.Errorf("Expected composite score between 0 and 1, got %f", score)
	}
	
	// Target size should get good score
	if score < 0.5 {
		t.Errorf("Expected good score for target-sized boundary, got %f", score)
	}
}

func TestNewContentAnalyzer(t *testing.T) {
	config := &StagingConfig{
		ContentAnalysisWindow: 16,
	}
	
	analyzer := NewContentAnalyzer(config)
	
	if analyzer == nil {
		t.Fatal("Expected non-nil ContentAnalyzer")
	}
	
	if analyzer.entropyCalculator == nil {
		t.Error("Expected entropyCalculator to be initialized")
	}
	
	if analyzer.patternDetector == nil {
		t.Error("Expected patternDetector to be initialized")
	}
	
	if analyzer.typeClassifier == nil {
		t.Error("Expected typeClassifier to be initialized")
	}
	
	if analyzer.windowBuffer == nil {
		t.Error("Expected windowBuffer to be initialized")
	}
	
	if analyzer.analysisWindow != config.ContentAnalysisWindow*1024 {
		t.Error("Expected analysisWindow to be set from config")
	}
}

func TestContentAnalyzer_AnalyzeContent(t *testing.T) {
	config := &StagingConfig{
		ContentAnalysisWindow: 16,
	}
	
	analyzer := NewContentAnalyzer(config)
	
	// Test with text content
	textContent := "Hello, world! This is a test of content analysis. " + strings.Repeat("test ", 100)
	reader := strings.NewReader(textContent)
	
	profile, err := analyzer.AnalyzeContent(reader, "text/plain")
	if err != nil {
		t.Fatalf("Failed to analyze content: %v", err)
	}
	
	if profile == nil {
		t.Fatal("Expected non-nil content profile")
	}
	
	if profile.ContentType != "text/plain" {
		t.Error("Expected content type to be preserved")
	}
	
	if profile.Entropy <= 0 {
		t.Error("Expected positive entropy")
	}
	
	if profile.EstimatedRatio <= 0 || profile.EstimatedRatio > 1 {
		t.Error("Expected estimated ratio between 0 and 1")
	}
	
	if profile.AnalysisQuality < 0 || profile.AnalysisQuality > 1 {
		t.Error("Expected analysis quality between 0 and 1")
	}
}

func TestContentAnalyzer_AnalyzeContentEmpty(t *testing.T) {
	config := &StagingConfig{
		ContentAnalysisWindow: 16,
	}
	
	analyzer := NewContentAnalyzer(config)
	
	// Test with empty content
	reader := strings.NewReader("")
	
	profile, err := analyzer.AnalyzeContent(reader, "text/plain")
	if err != nil {
		t.Fatalf("Failed to analyze empty content: %v", err)
	}
	
	if profile == nil {
		t.Fatal("Expected non-nil content profile for empty content")
	}
	
	// Should handle empty content gracefully
	if len(profile.Patterns) != 0 {
		t.Error("Expected no patterns for empty content")
	}
}

func TestContentAnalyzer_isTarHeader(t *testing.T) {
	config := &StagingConfig{
		ContentAnalysisWindow: 16,
	}
	
	analyzer := NewContentAnalyzer(config)
	
	// Create a mock tar header
	tarHeader := make([]byte, 512)
	copy(tarHeader[257:262], "ustar")
	
	if !analyzer.isTarHeader(tarHeader) {
		t.Error("Expected valid tar header to be detected")
	}
	
	// Test invalid header
	invalidHeader := make([]byte, 512)
	copy(invalidHeader[257:262], "invalid")
	
	if analyzer.isTarHeader(invalidHeader) {
		t.Error("Expected invalid tar header to be rejected")
	}
	
	// Test short header
	shortHeader := make([]byte, 100)
	if analyzer.isTarHeader(shortHeader) {
		t.Error("Expected short header to be rejected")
	}
}

func TestContentAnalyzer_parseTarHeader(t *testing.T) {
	config := &StagingConfig{
		ContentAnalysisWindow: 16,
	}
	
	analyzer := NewContentAnalyzer(config)
	
	// Create a mock tar header
	tarHeader := make([]byte, 512)
	copy(tarHeader[0:], "test.txt\x00")    // filename
	copy(tarHeader[124:], "777\x00")       // size in octal
	copy(tarHeader[257:262], "ustar")      // magic
	
	alignment := analyzer.parseTarHeader(tarHeader, 0)
	
	if alignment.FileName != "test.txt" {
		t.Errorf("Expected filename 'test.txt', got '%s'", alignment.FileName)
	}
	
	if alignment.FileSize != 511 { // 777 octal = 511 decimal
		t.Errorf("Expected file size 511, got %d", alignment.FileSize)
	}
	
	if alignment.FileType == "" {
		t.Error("Expected file type to be classified")
	}
}

func TestContentAnalyzer_classifyFileType(t *testing.T) {
	config := &StagingConfig{
		ContentAnalysisWindow: 16,
	}
	
	analyzer := NewContentAnalyzer(config)
	
	tests := []struct {
		filename     string
		expectedType string
	}{
		{"test.txt", "text"},
		{"data.json", "text"},
		{"image.jpg", "image"},
		{"video.mp4", "video"},
		{"audio.mp3", "audio"},
		{"archive.zip", "compressed"},
		{"document.pdf", "document"},
		{"program.exe", "binary"},
		{"noextension", "no_extension"},
		{"unknown.xyz", "other"},
		{"", "unknown"},
	}
	
	for _, test := range tests {
		result := analyzer.classifyFileType(test.filename)
		if result != test.expectedType {
			t.Errorf("Expected file type '%s' for '%s', got '%s'",
				test.expectedType, test.filename, result)
		}
	}
}

func TestContentAnalyzer_estimateCompressionRatio(t *testing.T) {
	config := &StagingConfig{
		ContentAnalysisWindow: 16,
	}
	
	analyzer := NewContentAnalyzer(config)
	
	// Test text content (should compress well)
	textProfile := &ContentProfile{
		ContentType: "text",
		Entropy:     2.0, // Low entropy
	}
	
	textRatio := analyzer.estimateCompressionRatio(textProfile)
	if textRatio <= 0 || textRatio >= 1 {
		t.Error("Expected text compression ratio between 0 and 1")
	}
	
	// Text should have good compression
	if textRatio > 0.5 {
		t.Error("Expected text to have good compression ratio (< 0.5)")
	}
	
	// Test compressed content (should not compress much)
	compressedProfile := &ContentProfile{
		ContentType: "compressed",
		Entropy:     7.5, // High entropy
	}
	
	compressedRatio := analyzer.estimateCompressionRatio(compressedProfile)
	if compressedRatio <= 0 || compressedRatio >= 1 {
		t.Error("Expected compressed compression ratio between 0 and 1")
	}
	
	// Compressed content should have poor compression (low ratio)
	if compressedRatio > 0.2 {
		t.Error("Expected compressed content to have poor compression ratio (< 0.2)")
	}
}

func TestContentAnalyzer_calculateAnalysisQuality(t *testing.T) {
	config := &StagingConfig{
		ContentAnalysisWindow: 16,
	}
	
	analyzer := NewContentAnalyzer(config)
	
	profile := &ContentProfile{}
	
	// Test with sufficient data
	sufficientBytes := int64(2 * 1024 * 1024) // 2MB
	quality := analyzer.calculateAnalysisQuality(profile, sufficientBytes)
	if quality != 1.0 {
		t.Errorf("Expected quality 1.0 for sufficient data, got %f", quality)
	}
	
	// Test with insufficient data
	insufficientBytes := int64(512 * 1024) // 512KB
	quality = analyzer.calculateAnalysisQuality(profile, insufficientBytes)
	if quality <= 0 || quality >= 1 {
		t.Errorf("Expected quality between 0 and 1 for insufficient data, got %f", quality)
	}
}

func TestNewEntropyCalculator(t *testing.T) {
	calculator := NewEntropyCalculator()
	
	if calculator == nil {
		t.Fatal("Expected non-nil EntropyCalculator")
	}
}

func TestEntropyCalculator_CalculateEntropy(t *testing.T) {
	calculator := NewEntropyCalculator()
	
	// Test with uniform data (high entropy)
	uniformData := make([]byte, 256)
	for i := 0; i < 256; i++ {
		uniformData[i] = byte(i)
	}
	
	uniformEntropy := calculator.CalculateEntropy(uniformData)
	if uniformEntropy <= 0 {
		t.Error("Expected positive entropy for uniform data")
	}
	
	// Should be close to maximum entropy (8 bits)
	if uniformEntropy < 7.0 {
		t.Error("Expected high entropy for uniform data")
	}
	
	// Test with repetitive data (low entropy)
	repetitiveData := bytes.Repeat([]byte("A"), 1000)
	
	repetitiveEntropy := calculator.CalculateEntropy(repetitiveData)
	if repetitiveEntropy != 0 {
		t.Error("Expected zero entropy for repetitive data")
	}
	
	// Test with empty data
	emptyEntropy := calculator.CalculateEntropy([]byte{})
	if emptyEntropy != 0 {
		t.Error("Expected zero entropy for empty data")
	}
}

func TestEntropyCalculator_CalculateOverallEntropy(t *testing.T) {
	calculator := NewEntropyCalculator()
	
	patterns := []ContentPattern{
		{Type: PatternText, Length: 1000, Frequency: 0.8},
		{Type: PatternRandom, Length: 500, Frequency: 0.2},
	}
	
	entropy := calculator.CalculateOverallEntropy(patterns)
	if entropy <= 0 {
		t.Error("Expected positive overall entropy")
	}
	
	// Test with empty patterns
	emptyEntropy := calculator.CalculateOverallEntropy([]ContentPattern{})
	if emptyEntropy != 4.0 {
		t.Error("Expected default entropy of 4.0 for empty patterns")
	}
}

func TestEntropyCalculator_patternTypeEntropy(t *testing.T) {
	calculator := NewEntropyCalculator()
	
	tests := []struct {
		patternType     PatternType
		expectedRange   [2]float64 // [min, max]
	}{
		{PatternRepetitive, [2]float64{0.5, 1.5}},
		{PatternRandom, [2]float64{7.5, 8.5}},
		{PatternStructured, [2]float64{2.5, 3.5}},
		{PatternBinary, [2]float64{5.5, 6.5}},
		{PatternText, [2]float64{3.5, 4.5}},
	}
	
	for _, test := range tests {
		entropy := calculator.patternTypeEntropy(test.patternType)
		if entropy < test.expectedRange[0] || entropy > test.expectedRange[1] {
			t.Errorf("Expected entropy for %v to be in range [%f, %f], got %f",
				test.patternType, test.expectedRange[0], test.expectedRange[1], entropy)
		}
	}
}

func TestNewContentPatternDetector(t *testing.T) {
	detector := NewContentPatternDetector()
	
	if detector == nil {
		t.Fatal("Expected non-nil ContentPatternDetector")
	}
	
	if detector.patternCache == nil {
		t.Error("Expected patternCache to be initialized")
	}
	
	if detector.cacheSize <= 0 {
		t.Error("Expected positive cache size")
	}
}

func TestContentPatternDetector_DetectPatterns(t *testing.T) {
	detector := NewContentPatternDetector()
	
	// Test with repetitive data
	repetitiveData := bytes.Repeat([]byte("ABCDEFGHIJKLMNOP"), 100)
	
	patterns := detector.DetectPatterns(repetitiveData, 0)
	if len(patterns) == 0 {
		t.Error("Expected patterns to be detected in repetitive data")
	}
	
	// Should detect repetitive patterns
	hasRepetitive := false
	for _, pattern := range patterns {
		if pattern.Type == PatternRepetitive {
			hasRepetitive = true
			break
		}
	}
	if !hasRepetitive {
		t.Error("Expected repetitive pattern to be detected")
	}
}

func TestContentPatternDetector_DetectPatternsWithCache(t *testing.T) {
	detector := NewContentPatternDetector()
	
	data := []byte("test data for caching")
	
	// First call should populate cache
	patterns1 := detector.DetectPatterns(data, 0)
	
	// Second call should use cache
	patterns2 := detector.DetectPatterns(data, 100)
	
	if len(patterns1) != len(patterns2) {
		t.Error("Expected cached results to have same pattern count")
	}
	
	// Offsets should be adjusted for cached patterns
	if len(patterns2) > 0 {
		if patterns2[0].Offset != patterns1[0].Offset+100 {
			t.Error("Expected cached pattern offset to be adjusted")
		}
	}
}

func TestContentPatternDetector_detectRepetitivePatterns(t *testing.T) {
	detector := NewContentPatternDetector()
	
	// Create data with clear repetitive pattern
	pattern := []byte("0123456789ABCDEF") // 16 bytes
	data := bytes.Repeat(pattern, 10)     // Repeat 10 times
	
	patterns := detector.detectRepetitivePatterns(data, 0)
	
	if len(patterns) == 0 {
		t.Error("Expected repetitive patterns to be detected")
	}
	
	// Check if detected pattern is reasonable
	found := false
	for _, p := range patterns {
		if p.Type == PatternRepetitive && p.Compressibility > 0.9 {
			found = true
			break
		}
	}
	if !found {
		t.Error("Expected high compressibility repetitive pattern")
	}
}

func TestContentPatternDetector_detectStructuredPatterns(t *testing.T) {
	detector := NewContentPatternDetector()
	
	// Create JSON-like data
	jsonData := []byte(`{"key": "value", "array": [1, 2, 3], "nested": {"inner": true}}`)
	
	patterns := detector.detectStructuredPatterns(jsonData, 0)
	
	// Should detect structured pattern due to JSON characters
	hasStructured := false
	for _, pattern := range patterns {
		if pattern.Type == PatternStructured {
			hasStructured = true
			break
		}
	}
	if !hasStructured {
		t.Error("Expected structured pattern to be detected in JSON data")
	}
}

func TestContentPatternDetector_detectRandomPatterns(t *testing.T) {
	detector := NewContentPatternDetector()
	
	// Create high-entropy data
	randomData := make([]byte, 2048)
	for i := range randomData {
		randomData[i] = byte(i * 137 % 256) // Pseudo-random
	}
	
	patterns := detector.detectRandomPatterns(randomData, 0)
	
	// Should detect some random patterns
	hasRandom := false
	for _, pattern := range patterns {
		if pattern.Type == PatternRandom {
			hasRandom = true
			break
		}
	}
	if !hasRandom {
		t.Error("Expected random pattern to be detected in high-entropy data")
	}
}

func TestNewContentTypeClassifier(t *testing.T) {
	classifier := NewContentTypeClassifier()
	
	if classifier == nil {
		t.Fatal("Expected non-nil ContentTypeClassifier")
	}
	
	if classifier.signatures == nil {
		t.Error("Expected signatures to be initialized")
	}
	
	if len(classifier.signatures) == 0 {
		t.Error("Expected signatures to contain known file types")
	}
}

func TestContentTypeClassifier_ClassifyContent(t *testing.T) {
	classifier := NewContentTypeClassifier()
	
	// Test empty content
	emptyResult := classifier.ClassifyContent([]byte{})
	if emptyResult != "empty" {
		t.Errorf("Expected 'empty' for empty content, got '%s'", emptyResult)
	}
	
	// Test PDF signature
	pdfData := []byte("%PDF-1.4 rest of pdf content...")
	pdfResult := classifier.ClassifyContent(pdfData)
	if pdfResult != "pdf" {
		t.Errorf("Expected 'pdf' for PDF content, got '%s'", pdfResult)
	}
	
	// Test ZIP signature
	zipData := []byte{0x50, 0x4B, 0x03, 0x04, 0x00, 0x00} // ZIP magic bytes
	zipResult := classifier.ClassifyContent(zipData)
	if zipResult != "zip" {
		t.Errorf("Expected 'zip' for ZIP content, got '%s'", zipResult)
	}
	
	// Test text content
	textData := []byte("Hello, this is plain text content with normal characters.")
	textResult := classifier.ClassifyContent(textData)
	if textResult != "text" {
		t.Errorf("Expected 'text' for text content, got '%s'", textResult)
	}
	
	// Test binary content
	binaryData := make([]byte, 100)
	for i := range binaryData {
		binaryData[i] = byte(i % 256)
	}
	binaryResult := classifier.ClassifyContent(binaryData)
	if binaryResult != "binary" {
		t.Errorf("Expected 'binary' for binary content, got '%s'", binaryResult)
	}
}

func TestMinIntFunction(t *testing.T) {
	tests := []struct {
		a, b, expected int
	}{
		{5, 3, 3},
		{2, 8, 2},
		{4, 4, 4},
		{0, 10, 0},
		{-5, -3, -5},
	}
	
	for _, test := range tests {
		result := minInt(test.a, test.b)
		if result != test.expected {
			t.Errorf("minInt(%d, %d) = %d, expected %d", test.a, test.b, result, test.expected)
		}
	}
}

func TestChunkBoundary_Fields(t *testing.T) {
	boundary := ChunkBoundary{
		StartOffset:      1024,
		EndOffset:        33 * 1024 * 1024,
		Size:             32 * 1024 * 1024,
		CompressionScore: 0.75,
		PredictedRatio:   0.65,
	}
	
	// Test that fields are accessible
	if boundary.StartOffset != 1024 {
		t.Error("Expected start offset to be set correctly")
	}
	
	if boundary.Size != 32*1024*1024 {
		t.Error("Expected size to be set correctly")
	}
	
	if boundary.CompressionScore != 0.75 {
		t.Error("Expected compression score to be set correctly")
	}
}

func TestContentProfile_Fields(t *testing.T) {
	profile := &ContentProfile{
		ContentType:     "text",
		Entropy:         4.5,
		EstimatedRatio:  0.3,
		AnalysisQuality: 0.9,
		Patterns: []ContentPattern{
			{Type: PatternText, Length: 1000, Frequency: 0.8},
		},
		CompressionHints: []CompressionHint{
			{Algorithm: "zstd", EstimatedRatio: 0.3},
		},
		FileAlignment: []FileAlignment{
			{Offset: 0, FileName: "test.txt", FileSize: 1000},
		},
	}
	
	// Test that fields are accessible and have expected values
	if profile.ContentType != "text" {
		t.Error("Expected content type to be set correctly")
	}
	
	if profile.Entropy != 4.5 {
		t.Error("Expected entropy to be set correctly")
	}
	
	if len(profile.Patterns) != 1 {
		t.Error("Expected one pattern")
	}
	
	if len(profile.CompressionHints) != 1 {
		t.Error("Expected one compression hint")
	}
	
	if len(profile.FileAlignment) != 1 {
		t.Error("Expected one file alignment")
	}
	
	// Test pattern access
	if profile.Patterns[0].Type != PatternText {
		t.Error("Expected pattern type to be text")
	}
}