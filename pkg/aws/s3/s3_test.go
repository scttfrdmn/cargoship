package s3

import (
	"strconv"
	"testing"
	"time"

	"github.com/aws/aws-sdk-go-v2/service/s3/types"
	awsconfig "github.com/scttfrdmn/cargoship/pkg/aws/config"
)

func TestTransporterConstructor(t *testing.T) {
	config := awsconfig.S3Config{
		Bucket:             "test-bucket",
		StorageClass:       awsconfig.StorageClassStandard,
		MultipartChunkSize: 10 * 1024 * 1024,
		Concurrency:        4,
	}

	transporter := NewTransporter(nil, config)

	if transporter == nil {
		t.Fatal("NewTransporter returned nil")
	}

	if transporter.config.Bucket != config.Bucket {
		t.Errorf("Expected bucket %s, got %s", config.Bucket, transporter.config.Bucket)
	}

	if transporter.uploader == nil {
		t.Error("Expected uploader to be initialized")
	}
}

func TestOptimizeStorageClass(t *testing.T) {
	config := awsconfig.S3Config{
		StorageClass: awsconfig.StorageClassStandard,
	}
	
	transporter := &Transporter{config: config}

	tests := []struct {
		name            string
		archive         Archive
		expectedStorage types.StorageClass
	}{
		{
			name: "default storage class",
			archive: Archive{
				AccessPattern: "",
				RetentionDays: 0,
			},
			expectedStorage: types.StorageClassStandard,
		},
		{
			name: "deep archive for long-term archival",
			archive: Archive{
				AccessPattern: "archive",
				RetentionDays: 400,
			},
			expectedStorage: types.StorageClassDeepArchive,
		},
		{
			name: "glacier for long-term storage",
			archive: Archive{
				AccessPattern: "rare",
				RetentionDays: 100,
			},
			expectedStorage: types.StorageClassGlacier,
		},
		{
			name: "standard-ia for infrequent access",
			archive: Archive{
				AccessPattern: "infrequent",
				RetentionDays: 30,
			},
			expectedStorage: types.StorageClassStandardIa,
		},
		{
			name: "intelligent tiering for unknown pattern",
			archive: Archive{
				AccessPattern: "unknown",
				RetentionDays: 50,
			},
			expectedStorage: types.StorageClassIntelligentTiering,
		},
		{
			name: "standard for frequent access",
			archive: Archive{
				AccessPattern: "frequent",
				RetentionDays: 30,
			},
			expectedStorage: types.StorageClassStandard,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := transporter.optimizeStorageClass(tt.archive)
			if result != tt.expectedStorage {
				t.Errorf("Expected %s, got %s", tt.expectedStorage, result)
			}
		})
	}
}

func TestBuildMetadata(t *testing.T) {
	transporter := &Transporter{}
	
	archive := Archive{
		OriginalSize:    1024 * 1024 * 1024, // 1GB
		CompressionType: "gzip",
		AccessPattern:   "rare",
		RetentionDays:   365,
		Metadata: map[string]string{
			"custom-key": "custom-value",
		},
	}

	metadata := transporter.buildMetadata(archive)

	// Check custom metadata is preserved
	if metadata["custom-key"] != "custom-value" {
		t.Error("Custom metadata not preserved")
	}

	// Check CargoShip metadata is added
	expectedKeys := []string{
		"cargoship-original-size",
		"cargoship-compression-type", 
		"cargoship-created-by",
		"cargoship-upload-time",
		"cargoship-access-pattern",
		"cargoship-retention-days",
	}

	for _, key := range expectedKeys {
		if _, exists := metadata[key]; !exists {
			t.Errorf("Expected metadata key %s not found", key)
		}
	}

	// Verify specific values
	if metadata["cargoship-original-size"] != strconv.FormatInt(archive.OriginalSize, 10) {
		t.Error("Original size metadata incorrect")
	}

	if metadata["cargoship-compression-type"] != archive.CompressionType {
		t.Error("Compression type metadata incorrect")
	}

	if metadata["cargoship-access-pattern"] != archive.AccessPattern {
		t.Error("Access pattern metadata incorrect")
	}

	if metadata["cargoship-retention-days"] != strconv.Itoa(archive.RetentionDays) {
		t.Error("Retention days metadata incorrect")
	}

	if metadata["cargoship-created-by"] != "cargoship" {
		t.Error("Created-by metadata incorrect")
	}

	// Check upload time format
	uploadTime := metadata["cargoship-upload-time"]
	if uploadTime == "" {
		t.Error("Upload time not set")
	}

	// Verify it's a valid RFC3339 timestamp
	_, err := time.Parse(time.RFC3339, uploadTime)
	if err != nil {
		t.Errorf("Upload time not in RFC3339 format: %v", err)
	}
}

func TestBuildMetadataEmptyFields(t *testing.T) {
	transporter := &Transporter{}
	
	archive := Archive{
		OriginalSize:    100,
		CompressionType: "none",
		// Empty AccessPattern and RetentionDays
	}

	metadata := transporter.buildMetadata(archive)

	// These fields should not be present when empty/zero
	if _, exists := metadata["cargoship-access-pattern"]; exists {
		t.Error("Access pattern should not be set when empty")
	}

	if _, exists := metadata["cargoship-retention-days"]; exists {
		t.Error("Retention days should not be set when zero")
	}

	// These should always be present
	if metadata["cargoship-created-by"] != "cargoship" {
		t.Error("Created-by should always be set")
	}

	if metadata["cargoship-original-size"] != "100" {
		t.Error("Original size should be set")
	}
}

func TestArchiveFields(t *testing.T) {
	// Test Archive struct creation and field access
	archive := Archive{
		Key:              "test/archive.tar.gz",
		Size:             1024,
		StorageClass:     awsconfig.StorageClassStandard,
		OriginalSize:     2048,
		CompressionType:  "gzip",
		AccessPattern:    "infrequent",
		RetentionDays:    90,
		Metadata: map[string]string{
			"project": "test-project",
		},
	}

	// Verify all fields are accessible and correctly set
	if archive.Key != "test/archive.tar.gz" {
		t.Error("Key field not set correctly")
	}
	if archive.Size != 1024 {
		t.Error("Size field not set correctly")
	}
	if archive.StorageClass != awsconfig.StorageClassStandard {
		t.Error("StorageClass field not set correctly")
	}
	if archive.OriginalSize != 2048 {
		t.Error("OriginalSize field not set correctly")
	}
	if archive.CompressionType != "gzip" {
		t.Error("CompressionType field not set correctly")
	}
	if archive.AccessPattern != "infrequent" {
		t.Error("AccessPattern field not set correctly")
	}
	if archive.RetentionDays != 90 {
		t.Error("RetentionDays field not set correctly")
	}
	if archive.Metadata["project"] != "test-project" {
		t.Error("Metadata field not set correctly")
	}
}

func TestUploadResultFields(t *testing.T) {
	// Test UploadResult struct creation and field access
	result := UploadResult{
		Location:     "https://bucket.s3.amazonaws.com/key",
		Key:          "test-key",
		ETag:         "etag-value",
		UploadID:     "upload-id",
		Duration:     5 * time.Second,
		Throughput:   10.5,
		StorageClass: types.StorageClassStandard,
	}

	// Verify all fields are accessible and correctly set
	if result.Location != "https://bucket.s3.amazonaws.com/key" {
		t.Error("Location field not set correctly")
	}
	if result.Key != "test-key" {
		t.Error("Key field not set correctly")
	}
	if result.ETag != "etag-value" {
		t.Error("ETag field not set correctly")
	}
	if result.UploadID != "upload-id" {
		t.Error("UploadID field not set correctly")
	}
	if result.Duration != 5*time.Second {
		t.Error("Duration field not set correctly")
	}
	if result.Throughput != 10.5 {
		t.Error("Throughput field not set correctly")
	}
	if result.StorageClass != types.StorageClassStandard {
		t.Error("StorageClass field not set correctly")
	}
}

func TestParallelUploaderConstructor(t *testing.T) {
	config := ParallelConfig{
		MaxPrefixes:          6,
		MaxConcurrentUploads: 4,
		PrefixPattern:        "hash",
		LoadBalancing:        "round_robin",
		PrefixOptimization:   true,
	}

	uploader := NewParallelUploader(nil, config)

	if uploader == nil {
		t.Fatal("NewParallelUploader returned nil")
	}

	if uploader.config.MaxPrefixes != 6 {
		t.Errorf("Expected MaxPrefixes=6, got %d", uploader.config.MaxPrefixes)
	}

	if uploader.config.MaxConcurrentUploads != 4 {
		t.Errorf("Expected MaxConcurrentUploads=4, got %d", uploader.config.MaxConcurrentUploads)
	}

	if uploader.config.PrefixPattern != "hash" {
		t.Errorf("Expected PrefixPattern=hash, got %s", uploader.config.PrefixPattern)
	}

	if uploader.metrics == nil {
		t.Error("Expected metrics to be initialized")
	}

	if uploader.metrics.PrefixStats == nil {
		t.Error("Expected PrefixStats to be initialized")
	}
}

func TestParallelUploaderDefaults(t *testing.T) {
	config := ParallelConfig{} // Empty config to test defaults

	uploader := NewParallelUploader(nil, config)

	if uploader.config.MaxPrefixes != 4 {
		t.Errorf("Expected default MaxPrefixes=4, got %d", uploader.config.MaxPrefixes)
	}

	if uploader.config.MaxConcurrentUploads != 3 {
		t.Errorf("Expected default MaxConcurrentUploads=3, got %d", uploader.config.MaxConcurrentUploads)
	}

	if uploader.config.PrefixPattern != "hash" {
		t.Errorf("Expected default PrefixPattern=hash, got %s", uploader.config.PrefixPattern)
	}

	if uploader.config.LoadBalancing != "least_loaded" {
		t.Errorf("Expected default LoadBalancing=least_loaded, got %s", uploader.config.LoadBalancing)
	}
}

func TestGeneratePrefixes(t *testing.T) {
	tests := []struct {
		name         string
		pattern      string
		customPrefixes []string
		archiveCount int
		wantCount    int
	}{
		{"hash pattern", "hash", nil, 10, 4},
		{"date pattern", "date", nil, 20, 4},
		{"sequential pattern", "sequential", nil, 5, 4},
		{"custom pattern with prefixes", "custom", []string{"custom/prefix1/", "custom/prefix2/"}, 8, 2},
		{"unknown pattern defaults to hash", "unknown", nil, 15, 4},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			config := ParallelConfig{
				MaxPrefixes:     4,
				PrefixPattern:   tt.pattern,
				CustomPrefixes:  tt.customPrefixes,
			}

			uploader := &ParallelUploader{config: config}
			prefixes := uploader.generatePrefixes(tt.archiveCount)

			if len(prefixes) != tt.wantCount {
				t.Errorf("Expected %d prefixes, got %d", tt.wantCount, len(prefixes))
			}

			// Verify prefixes are not empty
			for i, prefix := range prefixes {
				if prefix == "" {
					t.Errorf("Prefix %d is empty", i)
				}
			}
		})
	}
}

func TestGenerateSequentialPrefixes(t *testing.T) {
	uploader := &ParallelUploader{
		config: ParallelConfig{MaxPrefixes: 4},
	}

	prefixes := uploader.generateSequentialPrefixes()

	if len(prefixes) != 4 {
		t.Errorf("Expected 4 prefixes, got %d", len(prefixes))
	}

	expectedPrefixes := []string{
		"archives/batch-0000/",
		"archives/batch-0001/",
		"archives/batch-0002/",
		"archives/batch-0003/",
	}

	for i, expected := range expectedPrefixes {
		if prefixes[i] != expected {
			t.Errorf("Expected prefix %s, got %s", expected, prefixes[i])
		}
	}
}

func TestGenerateHashPrefixes(t *testing.T) {
	uploader := &ParallelUploader{
		config: ParallelConfig{MaxPrefixes: 4},
	}

	prefixes := uploader.generateHashPrefixes()

	if len(prefixes) != 4 {
		t.Errorf("Expected 4 prefixes, got %d", len(prefixes))
	}

	// Verify prefixes are unique and have the expected format
	seen := make(map[string]bool)
	for _, prefix := range prefixes {
		if seen[prefix] {
			t.Errorf("Duplicate prefix found: %s", prefix)
		}
		seen[prefix] = true

		if len(prefix) < 10 { // Should be like "archives/00/"
			t.Errorf("Prefix %s seems too short", prefix)
		}
	}
}

func TestHashArchiveKey(t *testing.T) {
	uploader := &ParallelUploader{}

	key1 := "test/archive1.tar.gz"
	key2 := "test/archive2.tar.gz"

	hash1 := uploader.hashArchiveKey(key1)
	hash2 := uploader.hashArchiveKey(key2)

	// Same key should produce same hash
	hash1Again := uploader.hashArchiveKey(key1)
	if hash1 != hash1Again {
		t.Error("Hash function not deterministic")
	}

	// Different keys should produce different hashes (usually)
	if hash1 == hash2 {
		t.Error("Different keys produced same hash - potential collision")
	}
}

func TestGetOptimalPrefixCount(t *testing.T) {
	tests := []struct {
		name         string
		totalSize    int64
		archiveCount int
		expected     int
	}{
		{"small dataset", 500 * 1024 * 1024, 10, 1}, // 500MB
		{"medium dataset", 5 * 1024 * 1024 * 1024, 50, 2}, // 5GB  
		{"large dataset", 50 * 1024 * 1024 * 1024, 100, 4}, // 50GB
		{"very large dataset", 500 * 1024 * 1024 * 1024, 1000, 8}, // 500GB
		{"massive dataset", 5000 * 1024 * 1024 * 1024, 10000, 16}, // 5TB
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := GetOptimalPrefixCount(tt.totalSize, tt.archiveCount)
			if result != tt.expected {
				t.Errorf("Expected %d prefixes, got %d", tt.expected, result)
			}
		})
	}
}

func TestDistributeArchivesRoundRobin(t *testing.T) {
	uploader := &ParallelUploader{
		config: ParallelConfig{LoadBalancing: "round_robin"},
	}

	archives := []Archive{
		{Key: "archive1", Size: 100},
		{Key: "archive2", Size: 200},
		{Key: "archive3", Size: 150},
		{Key: "archive4", Size: 250},
	}

	prefixes := []string{"prefix1/", "prefix2/"}
	batches := uploader.distributeArchives(archives, prefixes)

	if len(batches) != 2 {
		t.Errorf("Expected 2 batches, got %d", len(batches))
	}

	// Check round-robin distribution
	if len(batches[0].Archives) != 2 {
		t.Errorf("Expected 2 archives in first batch, got %d", len(batches[0].Archives))
	}

	if len(batches[1].Archives) != 2 {
		t.Errorf("Expected 2 archives in second batch, got %d", len(batches[1].Archives))
	}

	// Verify the keys are distributed correctly
	if batches[0].Archives[0].Key != "archive1" || batches[0].Archives[1].Key != "archive3" {
		t.Error("Round-robin distribution not working correctly for batch 0")
	}

	if batches[1].Archives[0].Key != "archive2" || batches[1].Archives[1].Key != "archive4" {
		t.Error("Round-robin distribution not working correctly for batch 1")
	}
}

func TestSelectOptimalPattern(t *testing.T) {
	uploader := &ParallelUploader{}

	tests := []struct {
		name         string
		totalSize    int64
		archiveCount int
		expected     string
	}{
		{"small archive count", 1024*1024*1024, 50, "sequential"},
		{"large dataset", 200*1024*1024*1024, 500, "hash"},
		{"medium dataset", 10*1024*1024*1024, 200, "date"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := uploader.selectOptimalPattern(tt.totalSize, tt.archiveCount)
			if result != tt.expected {
				t.Errorf("Expected pattern %s, got %s", tt.expected, result)
			}
		})
	}
}

func TestOptimizePrefixDistribution(t *testing.T) {
	uploader := &ParallelUploader{
		config: ParallelConfig{MaxPrefixes: 4},
	}

	archives := []Archive{
		{Size: 100 * 1024 * 1024}, // 100MB
		{Size: 200 * 1024 * 1024}, // 200MB
		{Size: 150 * 1024 * 1024}, // 150MB
	}

	optimization := uploader.OptimizePrefixDistribution(archives)

	if optimization == nil {
		t.Fatal("Expected optimization result, got nil")
	}

	if optimization.TotalSize != 450*1024*1024 {
		t.Errorf("Expected total size 450MB, got %d", optimization.TotalSize)
	}

	if optimization.ArchiveCount != 3 {
		t.Errorf("Expected archive count 3, got %d", optimization.ArchiveCount)
	}

	if optimization.RecommendedPrefixes <= 0 {
		t.Error("Expected positive recommended prefixes")
	}

	if optimization.RecommendedConcurrency <= 0 {
		t.Error("Expected positive recommended concurrency") 
	}

	if optimization.OptimalPattern == "" {
		t.Error("Expected optimal pattern to be set")
	}

	if optimization.SizeVariation < 0 || optimization.SizeVariation > 1 {
		t.Errorf("Size variation should be between 0 and 1, got %f", optimization.SizeVariation)
	}
}

func TestPrefixBatch(t *testing.T) {
	// Test PrefixBatch struct functionality
	batch := PrefixBatch{
		Prefix: "test/prefix/",
		Archives: []Archive{
			{Key: "archive1", Size: 100},
			{Key: "archive2", Size: 200},
		},
		Priority: 5,
	}

	if batch.Prefix != "test/prefix/" {
		t.Error("Prefix not set correctly")
	}

	if len(batch.Archives) != 2 {
		t.Error("Archives not set correctly")
	}

	if batch.Priority != 5 {
		t.Error("Priority not set correctly")
	}
}

func TestParallelUploadResultCalculateMetrics(t *testing.T) {
	result := &ParallelUploadResult{
		StartTime: time.Now().Add(-5 * time.Second),
		Duration:  5 * time.Second,
		PrefixStats: map[string]*PrefixMetrics{
			"prefix1": {TotalBytes: 1024 * 1024},     // 1MB
			"prefix2": {TotalBytes: 2 * 1024 * 1024}, // 2MB
		},
	}

	result.CalculateMetrics()

	expectedBytes := int64(3 * 1024 * 1024) // 3MB total
	if result.TotalBytes != expectedBytes {
		t.Errorf("Expected total bytes %d, got %d", expectedBytes, result.TotalBytes)
	}

	// Should calculate throughput (3MB in 5 seconds = 0.6 MB/s)
	expectedThroughput := 0.6
	if result.AverageThroughputMBps < expectedThroughput-0.1 || result.AverageThroughputMBps > expectedThroughput+0.1 {
		t.Errorf("Expected throughput around %.1f MB/s, got %.2f", expectedThroughput, result.AverageThroughputMBps)
	}
}

func BenchmarkOptimizeStorageClass(b *testing.B) {
	config := awsconfig.S3Config{StorageClass: awsconfig.StorageClassStandard}
	transporter := &Transporter{config: config}
	
	archive := Archive{
		AccessPattern: "infrequent",
		RetentionDays: 90,
	}

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		_ = transporter.optimizeStorageClass(archive)
	}
}

func BenchmarkBuildMetadata(b *testing.B) {
	transporter := &Transporter{}
	
	archive := Archive{
		OriginalSize:    1024 * 1024,
		CompressionType: "gzip",
		AccessPattern:   "frequent",
		RetentionDays:   30,
		Metadata: map[string]string{
			"key1": "value1",
			"key2": "value2",
		},
	}

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		_ = transporter.buildMetadata(archive)
	}
}

func TestDistributeBySize(t *testing.T) {
	uploader := &ParallelUploader{}

	archives := []Archive{
		{Key: "small", Size: 100},
		{Key: "large", Size: 1000},
		{Key: "medium", Size: 500},
		{Key: "tiny", Size: 50},
	}

	// Initialize empty batches
	batches := []PrefixBatch{
		{Prefix: "prefix1/", Archives: make([]Archive, 0)},
		{Prefix: "prefix2/", Archives: make([]Archive, 0)},
	}

	uploader.distributeBySize(archives, batches)

	// Calculate total size per batch
	batch1Size := int64(0)
	batch2Size := int64(0)
	
	for _, archive := range batches[0].Archives {
		batch1Size += archive.Size
	}
	
	for _, archive := range batches[1].Archives {
		batch2Size += archive.Size
	}

	// The algorithm should balance the sizes reasonably
	totalSize := int64(1650) // 100 + 1000 + 500 + 50
	diff := batch1Size - batch2Size
	if diff < 0 {
		diff = -diff
	}

	// Allow for some imbalance but not too much
	if diff > totalSize/2 {
		t.Errorf("Size distribution too unbalanced: batch1=%d, batch2=%d", batch1Size, batch2Size)
	}

	// Verify all archives are distributed
	totalDistributed := len(batches[0].Archives) + len(batches[1].Archives)
	if totalDistributed != len(archives) {
		t.Errorf("Expected %d archives distributed, got %d", len(archives), totalDistributed)
	}
}

func TestDistributeArchivesHashBased(t *testing.T) {
	uploader := &ParallelUploader{
		config: ParallelConfig{LoadBalancing: "hash_based"},
	}

	archives := []Archive{
		{Key: "archive1", Size: 100},
		{Key: "archive2", Size: 200},
		{Key: "archive3", Size: 150},
	}

	prefixes := []string{"prefix1/", "prefix2/"}
	batches := uploader.distributeArchives(archives, prefixes)

	// Verify total archives are preserved
	totalArchives := 0
	for _, batch := range batches {
		totalArchives += len(batch.Archives)
	}

	if totalArchives != len(archives) {
		t.Errorf("Expected %d total archives, got %d", len(archives), totalArchives)
	}

	// The same key should always map to the same batch
	firstRun := uploader.distributeArchives(archives, prefixes)
	secondRun := uploader.distributeArchives(archives, prefixes)

	for i := range firstRun {
		if len(firstRun[i].Archives) != len(secondRun[i].Archives) {
			t.Errorf("Hash-based distribution is not consistent for batch %d", i)
		}
	}
}

func TestDistributeArchivesLeastLoaded(t *testing.T) {
	uploader := &ParallelUploader{
		config: ParallelConfig{LoadBalancing: "least_loaded"},
	}

	archives := []Archive{
		{Key: "small", Size: 100},
		{Key: "large", Size: 1000},
		{Key: "medium", Size: 500},
		{Key: "tiny", Size: 50},
	}

	prefixes := []string{"prefix1/", "prefix2/"}
	batches := uploader.distributeArchives(archives, prefixes)

	// Verify total archives are preserved
	totalArchives := 0
	for _, batch := range batches {
		totalArchives += len(batch.Archives)
		
		// Check that priorities are set based on total size
		totalSize := int64(0)
		for _, archive := range batch.Archives {
			totalSize += archive.Size
		}
		expectedPriority := int(totalSize / (1024 * 1024)) // Priority based on MB
		if batch.Priority != expectedPriority {
			t.Errorf("Expected priority %d, got %d", expectedPriority, batch.Priority)
		}
	}

	if totalArchives != len(archives) {
		t.Errorf("Expected %d total archives, got %d", len(archives), totalArchives)
	}
}

func TestAdjustForContentType(t *testing.T) {
	uploader := &AdaptiveUploader{
		config: AdaptiveConfig{EnableContentTypeOptimization: true},
	}

	baseSize := int64(16 * 1024 * 1024) // 16MB

	tests := []struct {
		contentType string
		expectLarger bool
		expectSmaller bool
	}{
		{"application/zip", true, false},           // Should increase
		{"application/x-tar", true, false},        // Should increase
		{"video/mp4", true, false},                // Should increase
		{"video/mov", true, false},                // Should increase
		{"image/jpeg", true, false},               // Should increase slightly
		{"text/plain", false, true},               // Should decrease
		{"application/json", false, true},         // Should decrease
		{"application/octet-stream", false, false}, // Should stay same
	}

	for _, tt := range tests {
		t.Run(tt.contentType, func(t *testing.T) {
			adjusted := uploader.adjustForContentType(baseSize, tt.contentType)
			
			if tt.expectLarger && adjusted <= baseSize {
				t.Errorf("Expected %s to increase chunk size, got %d vs base %d", tt.contentType, adjusted, baseSize)
			}
			if tt.expectSmaller && adjusted >= baseSize {
				t.Errorf("Expected %s to decrease chunk size, got %d vs base %d", tt.contentType, adjusted, baseSize)
			}
			if !tt.expectLarger && !tt.expectSmaller && adjusted != baseSize {
				t.Errorf("Expected %s to keep same chunk size, got %d vs base %d", tt.contentType, adjusted, baseSize)
			}
		})
	}
}

func TestAdjustForHistoryWithSessions(t *testing.T) {
	uploader := &AdaptiveUploader{
		uploadHistory: &UploadHistory{
			sessions: []UploadSession{
				{
					ContentType:   "application/zip",
					Success:       true,
					OptimalChunk:  32 * 1024 * 1024, // 32MB
				},
				{
					ContentType:   "application/zip",
					Success:       true,
					OptimalChunk:  24 * 1024 * 1024, // 24MB
				},
				{
					ContentType:   "text/plain",
					Success:       true,
					OptimalChunk:  8 * 1024 * 1024, // 8MB (different content type)
				},
			},
		},
	}

	baseSize := int64(16 * 1024 * 1024) // 16MB

	// Test with matching content type
	adjusted := uploader.adjustForHistory(baseSize, "application/zip")
	
	// Should blend with historical average: (32+24)/2 = 28MB
	// 70% current (16MB) + 30% historical (28MB) = 0.7*16 + 0.3*28 = 11.2 + 8.4 = 19.6MB
	expected := int64(0.7*float64(baseSize) + 0.3*28*1024*1024)
	if adjusted < expected-1024*1024 || adjusted > expected+1024*1024 { // 1MB tolerance
		t.Errorf("Expected historical adjustment around %d, got %d", expected, adjusted)
	}

	// Test with no matching content type
	adjustedNoMatch := uploader.adjustForHistory(baseSize, "video/mp4")
	if adjustedNoMatch != baseSize {
		t.Errorf("Expected no adjustment for unmatched content type, got %d vs %d", adjustedNoMatch, baseSize)
	}
}

func TestClampChunkSizeBounds(t *testing.T) {
	uploader := &AdaptiveUploader{
		config: AdaptiveConfig{
			MinChunkSize: 5 * 1024 * 1024,  // 5MB
			MaxChunkSize: 50 * 1024 * 1024, // 50MB
		},
	}

	tests := []struct {
		name     string
		input    int64
		expected int64
	}{
		{"below minimum", 1 * 1024 * 1024, 5 * 1024 * 1024},   // Should clamp to min
		{"above maximum", 100 * 1024 * 1024, 50 * 1024 * 1024}, // Should clamp to max
		{"within bounds", 20 * 1024 * 1024, 20 * 1024 * 1024},  // Should remain unchanged
		{"exactly minimum", 5 * 1024 * 1024, 5 * 1024 * 1024},  // Should remain unchanged
		{"exactly maximum", 50 * 1024 * 1024, 50 * 1024 * 1024}, // Should remain unchanged
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := uploader.clampChunkSize(tt.input)
			if result != tt.expected {
				t.Errorf("Expected %d, got %d", tt.expected, result)
			}
		})
	}
}

func TestCalculateOptimalConcurrencyEdgeCases(t *testing.T) {
	uploader := &AdaptiveUploader{
		config: AdaptiveConfig{MaxConcurrency: 10},
		networkMonitor: &NetworkMonitor{bandwidth: 0}, // No network data
		uploadHistory: &UploadHistory{sessions: []UploadSession{}}, // Initialize upload history
	}

	// Test with small file that has fewer chunks than max concurrency
	fileSize := int64(50 * 1024 * 1024)  // 50MB
	chunkSize := int64(20 * 1024 * 1024) // 20MB chunks = 3 chunks total

	concurrency := uploader.CalculateOptimalConcurrency(fileSize, chunkSize)

	// Should not exceed number of chunks
	expectedMaxConcurrency := 3 // ceil(50/20) = 3 chunks
	if concurrency > expectedMaxConcurrency {
		t.Errorf("Concurrency %d should not exceed number of chunks %d", concurrency, expectedMaxConcurrency)
	}

	if concurrency <= 0 {
		t.Error("Concurrency should be positive")
	}
}

func TestGetOptimalConcurrencyFromHistoryWithData(t *testing.T) {
	now := time.Now()
	uploader := &AdaptiveUploader{
		uploadHistory: &UploadHistory{
			sessions: []UploadSession{
				{
					EndTime:              now.Add(-1 * time.Hour), // Recent
					Success:              true,
					OptimalConcurrency:   8,
				},
				{
					EndTime:              now.Add(-24 * time.Hour), // Older
					Success:              true,
					OptimalConcurrency:   4,
				},
				{
					EndTime:              now.Add(-1 * time.Hour),
					Success:              false, // Should be ignored
					OptimalConcurrency:   16,
				},
			},
		},
	}

	result := uploader.getOptimalConcurrencyFromHistory()

	// Should return weighted average favoring recent successful sessions
	if result <= 0 {
		t.Error("Should return positive concurrency from history")
	}

	// Recent session should have more weight, so result should be closer to 8 than 4
	if result < 4 || result > 8 {
		t.Errorf("Expected result between 4 and 8, got %d", result)
	}
}

func TestGetOptimalConcurrencyFromHistoryEmpty(t *testing.T) {
	uploader := &AdaptiveUploader{
		uploadHistory: &UploadHistory{
			sessions: []UploadSession{}, // Empty history
		},
	}

	result := uploader.getOptimalConcurrencyFromHistory()

	if result != 0 {
		t.Errorf("Expected 0 for empty history, got %d", result)
	}
}

func TestGetNetworkConditionVariations(t *testing.T) {
	tests := []struct {
		name      string
		bandwidth float64
		expected  string
	}{
		{"poor connection", 0.5, "poor"},
		{"fair connection", 3.0, "fair"},
		{"good connection", 15.0, "good"},
		{"excellent connection", 50.0, "excellent"},
		{"boundary case - poor", 1.0, "fair"}, // Exactly 1.0 should be fair
		{"boundary case - fair", 5.0, "good"}, // Exactly 5.0 should be good
		{"boundary case - good", 25.0, "excellent"}, // Exactly 25.0 should be excellent
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			uploader := &AdaptiveUploader{
				networkMonitor: &NetworkMonitor{bandwidth: tt.bandwidth},
			}

			result := uploader.GetNetworkCondition()
			if result != tt.expected {
				t.Errorf("Expected %s for bandwidth %.1f, got %s", tt.expected, tt.bandwidth, result)
			}
		})
	}
}

func TestMaxHelper(t *testing.T) {
	tests := []struct {
		a, b, expected int
	}{
		{5, 3, 5},
		{2, 8, 8},
		{7, 7, 7},
		{0, 1, 1},
		{-1, -5, -1},
	}

	for _, tt := range tests {
		result := max(tt.a, tt.b)
		if result != tt.expected {
			t.Errorf("max(%d, %d) = %d, expected %d", tt.a, tt.b, result, tt.expected)
		}
	}
}

func TestMinHelper(t *testing.T) {
	tests := []struct {
		a, b, expected int
	}{
		{5, 3, 3},
		{2, 8, 2},
		{7, 7, 7},
		{0, 1, 0},
		{-1, -5, -5},
	}

	for _, tt := range tests {
		result := min(tt.a, tt.b)
		if result != tt.expected {
			t.Errorf("min(%d, %d) = %d, expected %d", tt.a, tt.b, result, tt.expected)
		}
	}
}

func TestEstimateUploadDurationWithZeroBandwidth(t *testing.T) {
	uploader := &AdaptiveUploader{
		networkMonitor: &NetworkMonitor{bandwidth: 0}, // No bandwidth data
	}

	fileSize := int64(100 * 1024 * 1024) // 100MB
	chunkSize := int64(10 * 1024 * 1024) // 10MB
	concurrency := 4

	duration := uploader.estimateUploadDuration(fileSize, chunkSize, concurrency)

	if duration <= 0 {
		t.Error("Duration should be positive even with zero bandwidth")
	}

	// Should use default bandwidth of 5 MB/s
	expectedSeconds := float64(fileSize) / (1024 * 1024) / (5.0 * (0.7 + 0.3/float64(concurrency)))
	expectedDuration := time.Duration(expectedSeconds * float64(time.Second))

	// Allow for some variance in the calculation
	if duration < expectedDuration/2 || duration > expectedDuration*2 {
		t.Errorf("Duration %v seems unreasonable for 100MB file", duration)
	}
}

func TestClassifyNetworkConditionEdgeCases(t *testing.T) {
	uploader := &AdaptiveUploader{}

	// Test with empty throughputs
	session := UploadSession{
		Throughputs: []float64{},
	}

	condition := uploader.classifyNetworkCondition(session)
	if condition != "unknown" {
		t.Errorf("Expected 'unknown' for empty throughputs, got %s", condition)
	}

	// Test with various throughput values
	tests := []struct {
		throughputs []float64
		expected    string
	}{
		{[]float64{0.5, 0.8}, "poor"},
		{[]float64{2.0, 3.0}, "fair"},
		{[]float64{10.0, 15.0}, "good"},
		{[]float64{30.0, 40.0}, "excellent"},
		{[]float64{1.0}, "fair"}, // Exactly 1.0 average
	}

	for _, tt := range tests {
		session := UploadSession{Throughputs: tt.throughputs}
		condition := uploader.classifyNetworkCondition(session)
		if condition != tt.expected {
			t.Errorf("Expected %s for throughputs %v, got %s", tt.expected, tt.throughputs, condition)
		}
	}
}

func TestCalculateSessionOptimalConcurrencyFailedSession(t *testing.T) {
	uploader := &AdaptiveUploader{}

	// Test with failed session
	session := UploadSession{
		Success:     false,
		Concurrency: 8,
	}

	optimal := uploader.calculateSessionOptimalConcurrency(session)
	expected := max(1, session.Concurrency-1) // Should reduce concurrency
	if optimal != expected {
		t.Errorf("Expected %d for failed session, got %d", expected, optimal)
	}

	// Test with successful session
	session.Success = true
	optimal = uploader.calculateSessionOptimalConcurrency(session)
	if optimal != session.Concurrency {
		t.Errorf("Expected %d for successful session, got %d", session.Concurrency, optimal)
	}
}

func BenchmarkDistributeArchives(b *testing.B) {
	uploader := &ParallelUploader{
		config: ParallelConfig{LoadBalancing: "least_loaded"},
	}

	// Create test archives
	archives := make([]Archive, 100)
	for i := range archives {
		archives[i] = Archive{
			Key:  "archive" + string(rune(i)),
			Size: int64(100 + i*10),
		}
	}

	prefixes := []string{"prefix1/", "prefix2/", "prefix3/", "prefix4/"}

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		_ = uploader.distributeArchives(archives, prefixes)
	}
}

func BenchmarkHashArchiveKey(b *testing.B) {
	uploader := &ParallelUploader{}
	key := "test/archive/very/long/path/to/archive.tar.gz"

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		_ = uploader.hashArchiveKey(key)
	}
}