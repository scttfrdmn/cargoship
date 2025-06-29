package s3

import (
	"testing"
	"time"
)

func TestNewAdaptiveUploader(t *testing.T) {
	config := AdaptiveConfig{
		MinChunkSize:   5 * 1024 * 1024,
		MaxChunkSize:   50 * 1024 * 1024,
		MaxConcurrency: 5,
	}
	
	uploader := NewAdaptiveUploader(nil, config)
	
	if uploader == nil {
		t.Fatal("NewAdaptiveUploader returned nil")
	}
	
	if uploader.config.MinChunkSize != config.MinChunkSize {
		t.Errorf("Expected MinChunkSize %d, got %d", config.MinChunkSize, uploader.config.MinChunkSize)
	}
}

func TestCalculateOptimalChunkSize(t *testing.T) {
	config := AdaptiveConfig{}
	uploader := NewAdaptiveUploader(nil, config)
	
	tests := []struct {
		name        string
		fileSize    int64
		contentType string
		wantMin     int64
		wantMax     int64
	}{
		{
			name:        "small file",
			fileSize:    50 * 1024 * 1024, // 50MB
			contentType: "application/octet-stream",
			wantMin:     5 * 1024 * 1024,  // 5MB min
			wantMax:     20 * 1024 * 1024, // 20MB max
		},
		{
			name:        "large video file",
			fileSize:    5 * 1024 * 1024 * 1024, // 5GB
			contentType: "video/mp4",
			wantMin:     30 * 1024 * 1024, // Should be larger for video
			wantMax:     100 * 1024 * 1024,
		},
		{
			name:        "compressed archive",
			fileSize:    1 * 1024 * 1024 * 1024, // 1GB
			contentType: "application/zip",
			wantMin:     15 * 1024 * 1024, // Larger chunks for compressed data
			wantMax:     50 * 1024 * 1024,
		},
	}
	
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			chunkSize := uploader.CalculateOptimalChunkSize(tt.fileSize, tt.contentType)
			
			if chunkSize < tt.wantMin || chunkSize > tt.wantMax {
				t.Errorf("CalculateOptimalChunkSize() = %d, want between %d and %d", 
					chunkSize, tt.wantMin, tt.wantMax)
			}
		})
	}
}

func TestNetworkSample(t *testing.T) {
	uploader := NewAdaptiveUploader(nil, AdaptiveConfig{})
	
	// Test recording network samples
	samples := []NetworkSample{
		{
			Timestamp: time.Now(),
			Bandwidth: 10.0, // 10 MB/s
			Latency:   50 * time.Millisecond,
			Success:   true,
		},
		{
			Timestamp: time.Now(),
			Bandwidth: 15.0, // 15 MB/s
			Latency:   40 * time.Millisecond,
			Success:   true,
		},
		{
			Timestamp: time.Now(),
			Bandwidth: 5.0, // 5 MB/s
			Latency:   100 * time.Millisecond,
			Success:   true,
		},
	}
	
	for _, sample := range samples {
		uploader.RecordNetworkSample(sample)
	}
	
	// Check that network conditions are being tracked
	avgBandwidth := uploader.networkMonitor.GetAverageBandwidth()
	if avgBandwidth < 5.0 || avgBandwidth > 15.0 {
		t.Errorf("Average bandwidth %f should be between 5.0 and 15.0", avgBandwidth)
	}
	
	condition := uploader.GetNetworkCondition()
	if condition == "" {
		t.Error("Network condition should not be empty")
	}
}

func TestCalculateOptimalConcurrency(t *testing.T) {
	uploader := NewAdaptiveUploader(nil, AdaptiveConfig{MaxConcurrency: 10})
	
	// Simulate good network conditions
	sample := NetworkSample{
		Timestamp: time.Now(),
		Bandwidth: 20.0, // 20 MB/s - good connection
		Latency:   30 * time.Millisecond,
		Success:   true,
	}
	uploader.RecordNetworkSample(sample)
	
	tests := []struct {
		name      string
		fileSize  int64
		chunkSize int64
		wantMin   int
		wantMax   int
	}{
		{
			name:      "small file",
			fileSize:  100 * 1024 * 1024, // 100MB
			chunkSize: 10 * 1024 * 1024,  // 10MB chunks = 10 chunks
			wantMin:   2,
			wantMax:   10, // Can't exceed number of chunks
		},
		{
			name:      "large file",
			fileSize:  10 * 1024 * 1024 * 1024, // 10GB
			chunkSize: 50 * 1024 * 1024,        // 50MB chunks = 200 chunks
			wantMin:   4,
			wantMax:   10, // Should use max concurrency
		},
	}
	
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			concurrency := uploader.CalculateOptimalConcurrency(tt.fileSize, tt.chunkSize)
			
			if concurrency < tt.wantMin || concurrency > tt.wantMax {
				t.Errorf("CalculateOptimalConcurrency() = %d, want between %d and %d", 
					concurrency, tt.wantMin, tt.wantMax)
			}
		})
	}
}

func TestGetRecommendations(t *testing.T) {
	uploader := NewAdaptiveUploader(nil, AdaptiveConfig{})
	
	// Add some network history
	sample := NetworkSample{
		Timestamp: time.Now(),
		Bandwidth: 8.0, // 8 MB/s - fair connection
		Latency:   80 * time.Millisecond,
		Success:   true,
	}
	uploader.RecordNetworkSample(sample)
	
	fileSize := int64(500 * 1024 * 1024) // 500MB
	contentType := "application/zip"
	
	recommendations := uploader.GetRecommendations(fileSize, contentType)
	
	if recommendations == nil {
		t.Fatal("GetRecommendations returned nil")
	}
	
	if recommendations.OptimalChunkSize <= 0 {
		t.Error("OptimalChunkSize should be greater than 0")
	}
	
	if recommendations.OptimalConcurrency <= 0 {
		t.Error("OptimalConcurrency should be greater than 0")
	}
	
	if recommendations.NetworkCondition == "" {
		t.Error("NetworkCondition should not be empty")
	}
	
	if recommendations.EstimatedDuration <= 0 {
		t.Error("EstimatedDuration should be greater than 0")
	}
	
	if recommendations.Reasoning == "" {
		t.Error("Reasoning should not be empty")
	}
	
	// Confidence should be between 0 and 1
	if recommendations.ConfidenceLevel < 0.0 || recommendations.ConfidenceLevel > 1.0 {
		t.Errorf("ConfidenceLevel %f should be between 0.0 and 1.0", recommendations.ConfidenceLevel)
	}
}

func TestUploadSessionRecording(t *testing.T) {
	uploader := NewAdaptiveUploader(nil, AdaptiveConfig{})
	
	session := UploadSession{
		StartTime:   time.Now().Add(-5 * time.Minute),
		EndTime:     time.Now(),
		TotalSize:   1024 * 1024 * 1024, // 1GB
		ChunkSizes:  []int64{16 * 1024 * 1024, 16 * 1024 * 1024, 32 * 1024 * 1024},
		Throughputs: []float64{8.0, 10.0, 12.0}, // MB/s
		Concurrency: 4,
		Success:     true,
		ContentType: "application/octet-stream",
	}
	
	uploader.RecordUploadSession(session)
	
	// Check that history was recorded
	if len(uploader.uploadHistory.sessions) != 1 {
		t.Errorf("Expected 1 session in history, got %d", len(uploader.uploadHistory.sessions))
	}
	
	recordedSession := uploader.uploadHistory.sessions[0]
	if recordedSession.OptimalChunk <= 0 {
		t.Error("OptimalChunk should be calculated and greater than 0")
	}
	
	if recordedSession.NetworkCondition == "" {
		t.Error("NetworkCondition should be classified")
	}
}

func BenchmarkCalculateOptimalChunkSize(b *testing.B) {
	uploader := NewAdaptiveUploader(nil, AdaptiveConfig{})
	
	fileSize := int64(1024 * 1024 * 1024) // 1GB
	contentType := "application/octet-stream"
	
	b.ResetTimer()
	
	for i := 0; i < b.N; i++ {
		_ = uploader.CalculateOptimalChunkSize(fileSize, contentType)
	}
}

func BenchmarkGetRecommendations(b *testing.B) {
	uploader := NewAdaptiveUploader(nil, AdaptiveConfig{})
	
	// Add some network history for realistic benchmarking
	sample := NetworkSample{
		Timestamp: time.Now(),
		Bandwidth: 10.0,
		Latency:   50 * time.Millisecond,
		Success:   true,
	}
	uploader.RecordNetworkSample(sample)
	
	fileSize := int64(1024 * 1024 * 1024) // 1GB
	contentType := "application/octet-stream"
	
	b.ResetTimer()
	
	for i := 0; i < b.N; i++ {
		_ = uploader.GetRecommendations(fileSize, contentType)
	}
}