package staging

import (
	"context"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
)

func TestNewPerformancePredictor(t *testing.T) {
	config := DefaultStagingConfig()
	predictor := NewPerformancePredictor(config)

	assert.NotNil(t, predictor)
	assert.NotNil(t, predictor.performanceModel)
	assert.NotNil(t, predictor.historicalData)
	assert.NotNil(t, predictor.networkIntegrator)
	assert.NotNil(t, predictor.contentAnalyzer)
	assert.NotNil(t, predictor.predictionCache)
	assert.Equal(t, time.Minute*5, predictor.cacheExpiry)
}

func TestPerformancePredictor_Start(t *testing.T) {
	config := DefaultStagingConfig()
	predictor := NewPerformancePredictor(config)

	ctx, cancel := context.WithTimeout(context.Background(), time.Millisecond*100)
	defer cancel()

	// Start should not block
	done := make(chan struct{})
	go func() {
		predictor.Start(ctx)
		close(done)
	}()

	// Wait for context to cancel and goroutines to complete
	select {
	case <-done:
		// Good, Start() returned when context was cancelled
	case <-time.After(time.Millisecond * 200):
		t.Fatal("Start() did not return when context was cancelled")
	}
}

func TestPerformancePredictor_PredictPerformance(t *testing.T) {
	config := DefaultStagingConfig()
	predictor := NewPerformancePredictor(config)

	boundary := ChunkBoundary{
		Size:             1024 * 1024 * 10, // 10MB
		CompressionScore: 0.7,
	}

	networkCondition := &NetworkCondition{
		BandwidthMBps:   50.0,
		LatencyMs:       20.0,
		CongestionLevel: 0.1,
		Reliability:     0.95,
	}

	prediction, err := predictor.PredictPerformance(boundary, networkCondition)

	assert.NoError(t, err)
	assert.NotNil(t, prediction)
	assert.Greater(t, prediction.EstimatedUploadTime, time.Duration(0))
	assert.Greater(t, prediction.PredictedThroughput, 0.0)
	assert.GreaterOrEqual(t, prediction.SuccessProbability, 0.1)
	assert.LessOrEqual(t, prediction.SuccessProbability, 0.99)
	assert.Greater(t, prediction.OptimalChunkSize, int64(0))
	assert.NotEmpty(t, prediction.RecommendedCompression)
	assert.GreaterOrEqual(t, prediction.NetworkSuitability, 0.0)
	assert.LessOrEqual(t, prediction.NetworkSuitability, 1.0)
	assert.GreaterOrEqual(t, prediction.Confidence, 0.1)
	assert.LessOrEqual(t, prediction.Confidence, 0.95)
}

func TestPerformancePredictor_PredictPerformanceWithCache(t *testing.T) {
	config := DefaultStagingConfig()
	predictor := NewPerformancePredictor(config)

	boundary := ChunkBoundary{
		Size:             1024 * 1024 * 5, // 5MB
		CompressionScore: 0.5,
	}

	networkCondition := &NetworkCondition{
		BandwidthMBps:   100.0,
		LatencyMs:       10.0,
		CongestionLevel: 0.05,
		Reliability:     0.98,
	}

	// First prediction - should generate new
	prediction1, err1 := predictor.PredictPerformance(boundary, networkCondition)
	assert.NoError(t, err1)
	assert.NotNil(t, prediction1)

	// Second prediction - should use cache
	prediction2, err2 := predictor.PredictPerformance(boundary, networkCondition)
	assert.NoError(t, err2)
	assert.NotNil(t, prediction2)

	// Should be the same cached result
	assert.Equal(t, prediction1.GeneratedAt, prediction2.GeneratedAt)
}

func TestPerformancePredictor_UpdateHistory(t *testing.T) {
	config := DefaultStagingConfig()
	predictor := NewPerformancePredictor(config)

	record := &ChunkPerformanceRecord{
		ChunkID:          "test-chunk-1",
		Size:             1024 * 1024 * 20, // 20MB
		CompressionRatio: 0.6,
		UploadTime:       time.Second * 5,
		ThroughputMBps:   40.0,
		NetworkCondition: &NetworkCondition{
			BandwidthMBps: 50.0,
			LatencyMs:     25.0,
			Reliability:   0.9,
		},
		Success:   true,
		Timestamp: time.Now(),
	}

	// Should not panic
	predictor.UpdateHistory("test-chunk-1", record)

	// Verify accuracy is calculated
	accuracy := predictor.GetAccuracy()
	assert.GreaterOrEqual(t, accuracy, 0.0)
	assert.LessOrEqual(t, accuracy, 1.0)
}

func TestPerformancePredictor_GetAccuracy(t *testing.T) {
	config := DefaultStagingConfig()
	predictor := NewPerformancePredictor(config)

	// Initially should have default accuracy
	accuracy := predictor.GetAccuracy()
	assert.Equal(t, 0.5, accuracy)

	// Add some successful records
	for i := 0; i < 5; i++ {
		record := &ChunkPerformanceRecord{
			ChunkID:        "test-chunk",
			Size:           1024 * 1024 * 10,
			UploadTime:     time.Second * 3,
			ThroughputMBps: 30.0,
			NetworkCondition: &NetworkCondition{
				BandwidthMBps: 40.0,
				LatencyMs:     20.0,
				Reliability:   0.9,
			},
			Success:   true,
			Timestamp: time.Now(),
		}
		predictor.UpdateHistory("test-chunk", record)
	}

	accuracy = predictor.GetAccuracy()
	assert.Equal(t, 1.0, accuracy) // All successful
}

func TestPerformancePredictor_UpdateModels(t *testing.T) {
	config := DefaultStagingConfig()
	predictor := NewPerformancePredictor(config)

	// Should not panic
	predictor.UpdateModels()
}

func TestPerformancePredictor_PredictUploadTime(t *testing.T) {
	config := DefaultStagingConfig()
	predictor := NewPerformancePredictor(config)

	testCases := []struct {
		name             string
		boundary         ChunkBoundary
		networkCondition *NetworkCondition
		expectLong       bool
	}{
		{
			name: "fast network small file",
			boundary: ChunkBoundary{
				Size:             1024 * 1024 * 5, // 5MB
				CompressionScore: 0.8,
			},
			networkCondition: &NetworkCondition{
				BandwidthMBps:   200.0,
				LatencyMs:       5.0,
				CongestionLevel: 0.0,
				Reliability:     0.99,
			},
			expectLong: false,
		},
		{
			name: "slow network large file",
			boundary: ChunkBoundary{
				Size:             1024 * 1024 * 100, // 100MB
				CompressionScore: 0.2,
			},
			networkCondition: &NetworkCondition{
				BandwidthMBps:   5.0,
				LatencyMs:       200.0,
				CongestionLevel: 0.8,
				Reliability:     0.7,
			},
			expectLong: true,
		},
		{
			name: "zero bandwidth",
			boundary: ChunkBoundary{
				Size:             1024 * 1024 * 10,
				CompressionScore: 0.5,
			},
			networkCondition: &NetworkCondition{
				BandwidthMBps:   0.0,
				LatencyMs:       50.0,
				CongestionLevel: 0.5,
				Reliability:     0.8,
			},
			expectLong: true,
		},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			duration := predictor.predictUploadTime(tc.boundary, tc.networkCondition)
			assert.Greater(t, duration, time.Duration(0))

			if tc.expectLong {
				assert.Greater(t, duration, time.Second*10)
			} else {
				assert.Less(t, duration, time.Second*30)
			}
		})
	}
}

func TestPerformancePredictor_PredictThroughput(t *testing.T) {
	config := DefaultStagingConfig()
	predictor := NewPerformancePredictor(config)

	boundary := ChunkBoundary{
		Size:             1024 * 1024 * 15, // 15MB
		CompressionScore: 0.6,
	}

	networkCondition := &NetworkCondition{
		BandwidthMBps:   60.0,
		LatencyMs:       30.0,
		CongestionLevel: 0.2,
		Reliability:     0.85,
	}

	throughput := predictor.predictThroughput(boundary, networkCondition)

	assert.Greater(t, throughput, 0.0)
	// Should be at least 10% of base throughput
	assert.GreaterOrEqual(t, throughput, networkCondition.BandwidthMBps*0.1)
	// Should not exceed base throughput significantly
	assert.LessOrEqual(t, throughput, networkCondition.BandwidthMBps*1.5)
}

func TestPerformancePredictor_PredictSuccessProbability(t *testing.T) {
	config := DefaultStagingConfig()
	predictor := NewPerformancePredictor(config)

	testCases := []struct {
		name             string
		boundary         ChunkBoundary
		networkCondition *NetworkCondition
		expectHigh       bool
	}{
		{
			name: "small chunk reliable network",
			boundary: ChunkBoundary{
				Size: 1024 * 1024 * 10, // 10MB
			},
			networkCondition: &NetworkCondition{
				BandwidthMBps:   100.0,
				LatencyMs:       10.0,
				CongestionLevel: 0.05,
				Reliability:     0.98,
			},
			expectHigh: true,
		},
		{
			name: "large chunk unreliable network",
			boundary: ChunkBoundary{
				Size: 1024 * 1024 * 150, // 150MB
			},
			networkCondition: &NetworkCondition{
				BandwidthMBps:   20.0,
				LatencyMs:       500.0,
				CongestionLevel: 0.7,
				Reliability:     0.6,
			},
			expectHigh: false,
		},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			prob := predictor.predictSuccessProbability(tc.boundary, tc.networkCondition)

			assert.GreaterOrEqual(t, prob, 0.1)
			assert.LessOrEqual(t, prob, 0.99)

			if tc.expectHigh {
				assert.Greater(t, prob, 0.8)
			} else {
				assert.Less(t, prob, 0.8)
			}
		})
	}
}

func TestPerformancePredictor_DetermineOptimalChunkSize(t *testing.T) {
	config := DefaultStagingConfig()
	predictor := NewPerformancePredictor(config)

	testCases := []struct {
		name             string
		boundary         ChunkBoundary
		networkCondition *NetworkCondition
		expectSmaller    bool
	}{
		{
			name: "high bandwidth reliable network",
			boundary: ChunkBoundary{
				Size: 1024 * 1024 * 50, // 50MB
			},
			networkCondition: &NetworkCondition{
				BandwidthMBps:   200.0,
				LatencyMs:       5.0,
				CongestionLevel: 0.0,
				Reliability:     0.99,
			},
			expectSmaller: false,
		},
		{
			name: "unreliable congested network",
			boundary: ChunkBoundary{
				Size: 1024 * 1024 * 50, // 50MB
			},
			networkCondition: &NetworkCondition{
				BandwidthMBps:   10.0,
				LatencyMs:       50.0,
				CongestionLevel: 0.8,
				Reliability:     0.6,
			},
			expectSmaller: true,
		},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			optimalSize := predictor.determineOptimalChunkSize(tc.boundary, tc.networkCondition)

			// Should be within bounds
			assert.GreaterOrEqual(t, optimalSize, int64(5*1024*1024)) // 5MB min
			assert.LessOrEqual(t, optimalSize, int64(100*1024*1024))  // 100MB max

			if tc.expectSmaller {
				assert.Less(t, optimalSize, tc.boundary.Size)
			}
		})
	}
}

func TestPerformancePredictor_RecommendCompression(t *testing.T) {
	config := DefaultStagingConfig()
	predictor := NewPerformancePredictor(config)

	testCases := []struct {
		name             string
		boundary         ChunkBoundary
		networkCondition *NetworkCondition
		expectedAlgo     string
	}{
		{
			name: "slow network",
			boundary: ChunkBoundary{
				CompressionScore: 0.7,
			},
			networkCondition: &NetworkCondition{
				BandwidthMBps: 5.0,
			},
			expectedAlgo: "zstd-max",
		},
		{
			name: "fast network",
			boundary: ChunkBoundary{
				CompressionScore: 0.3,
			},
			networkCondition: &NetworkCondition{
				BandwidthMBps: 150.0,
			},
			expectedAlgo: "zstd-fast",
		},
		{
			name: "medium network high compression score",
			boundary: ChunkBoundary{
				CompressionScore: 0.8,
			},
			networkCondition: &NetworkCondition{
				BandwidthMBps: 50.0,
			},
			expectedAlgo: "zstd",
		},
		{
			name: "medium network low compression score",
			boundary: ChunkBoundary{
				CompressionScore: 0.2,
			},
			networkCondition: &NetworkCondition{
				BandwidthMBps: 50.0,
			},
			expectedAlgo: "zstd-fast",
		},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			algo := predictor.recommendCompression(tc.boundary, tc.networkCondition)
			assert.Equal(t, tc.expectedAlgo, algo)
		})
	}
}

func TestPerformancePredictor_CalculateNetworkSuitability(t *testing.T) {
	config := DefaultStagingConfig()
	predictor := NewPerformancePredictor(config)

	testCases := []struct {
		name             string
		boundary         ChunkBoundary
		networkCondition *NetworkCondition
		expectHigh       bool
	}{
		{
			name: "fast reliable network small chunk",
			boundary: ChunkBoundary{
				Size: 1024 * 1024 * 5, // 5MB
			},
			networkCondition: &NetworkCondition{
				BandwidthMBps:   200.0,
				LatencyMs:       10.0,
				CongestionLevel: 0.0,
				Reliability:     0.99,
			},
			expectHigh: true,
		},
		{
			name: "slow unreliable network large chunk",
			boundary: ChunkBoundary{
				Size: 1024 * 1024 * 200, // 200MB
			},
			networkCondition: &NetworkCondition{
				BandwidthMBps:   5.0,
				LatencyMs:       200.0,
				CongestionLevel: 0.8,
				Reliability:     0.6,
			},
			expectHigh: false,
		},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			suitability := predictor.calculateNetworkSuitability(tc.boundary, tc.networkCondition)

			assert.GreaterOrEqual(t, suitability, 0.0)
			assert.LessOrEqual(t, suitability, 1.0)

			if tc.expectHigh {
				assert.Greater(t, suitability, 0.7)
			} else {
				assert.Less(t, suitability, 0.5)
			}
		})
	}
}

func TestPerformancePredictor_CalculatePredictionConfidence(t *testing.T) {
	config := DefaultStagingConfig()
	predictor := NewPerformancePredictor(config)

	boundary := ChunkBoundary{
		Size: 1024 * 1024 * 25, // 25MB
	}

	networkCondition := &NetworkCondition{
		BandwidthMBps:   75.0,
		LatencyMs:       40.0,
		CongestionLevel: 0.3,
		Reliability:     0.8,
	}

	confidence := predictor.calculatePredictionConfidence(boundary, networkCondition)

	assert.GreaterOrEqual(t, confidence, 0.1)
	assert.LessOrEqual(t, confidence, 0.95)
}

func TestPerformancePredictor_GenerateCacheKey(t *testing.T) {
	config := DefaultStagingConfig()
	predictor := NewPerformancePredictor(config)

	boundary := ChunkBoundary{
		Size:             1024 * 1024 * 10, // 10MB
		CompressionScore: 0.75,
	}

	networkCondition := &NetworkCondition{
		BandwidthMBps:   50.5,
		LatencyMs:       25.3,
		CongestionLevel: 0.123,
	}

	key := predictor.generateCacheKey(boundary, networkCondition)

	assert.NotEmpty(t, key)
	assert.Contains(t, key, "boundary_")
	assert.Contains(t, key, "network_")

	// Same inputs should generate same key
	key2 := predictor.generateCacheKey(boundary, networkCondition)
	assert.Equal(t, key, key2)

	// Different inputs should generate different key
	boundary.Size = 1024 * 1024 * 20
	key3 := predictor.generateCacheKey(boundary, networkCondition)
	assert.NotEqual(t, key, key3)
}

func TestPerformancePredictor_CleanupExpiredCache(t *testing.T) {
	config := DefaultStagingConfig()
	predictor := NewPerformancePredictor(config)

	// Add some cache entries
	boundary := ChunkBoundary{Size: 1024 * 1024 * 10}
	networkCondition := &NetworkCondition{BandwidthMBps: 50.0}

	prediction1, _ := predictor.PredictPerformance(boundary, networkCondition)
	assert.NotNil(t, prediction1)

	// Manually expire cache by setting very short expiry
	predictor.cacheExpiry = time.Nanosecond

	// Wait a bit to ensure expiry
	time.Sleep(time.Millisecond)

	// Cleanup should remove expired entries
	predictor.cleanupExpiredCache()

	// Next prediction should generate new one (different timestamp)
	prediction2, _ := predictor.PredictPerformance(boundary, networkCondition)
	assert.NotNil(t, prediction2)
	assert.NotEqual(t, prediction1.GeneratedAt, prediction2.GeneratedAt)
}

func TestNewPerformanceModel(t *testing.T) {
	config := DefaultStagingConfig()
	model := NewPerformanceModel(config)

	assert.NotNil(t, model)
	assert.Equal(t, config, model.config)
	assert.NotNil(t, model.modelWeights)
	assert.NotNil(t, model.trainingData)

	// Check default weights
	assert.Contains(t, model.modelWeights, "size_factor")
	assert.Contains(t, model.modelWeights, "bandwidth_factor")
	assert.Equal(t, 0.4, model.modelWeights["size_factor"])
	assert.Equal(t, 0.3, model.modelWeights["bandwidth_factor"])
}

func TestPerformanceModel_UpdateModel(t *testing.T) {
	config := DefaultStagingConfig()
	model := NewPerformanceModel(config)

	record := &ChunkPerformanceRecord{
		ChunkID:          "test-chunk",
		Size:             1024 * 1024 * 15,
		CompressionRatio: 0.7,
		UploadTime:       time.Second * 4,
		ThroughputMBps:   25.0,
		NetworkCondition: &NetworkCondition{
			BandwidthMBps: 30.0,
			LatencyMs:     20.0,
			Reliability:   0.9,
		},
		Success:   true,
		Timestamp: time.Now(),
	}

	initialLength := len(model.trainingData)
	model.UpdateModel(record)

	assert.Equal(t, initialLength+1, len(model.trainingData))

	// Check that training data was properly converted
	trainingData := model.trainingData[len(model.trainingData)-1]
	assert.Equal(t, record.Size, trainingData.ChunkSize)
	assert.Equal(t, record.CompressionRatio, trainingData.CompressionRatio)
	assert.Equal(t, record.NetworkCondition.BandwidthMBps, trainingData.NetworkBandwidth)
	assert.Equal(t, record.ThroughputMBps, trainingData.ActualThroughput)
	assert.Equal(t, record.Success, trainingData.Success)
}

func TestPerformanceModel_UpdateModelSizeLimit(t *testing.T) {
	config := DefaultStagingConfig()
	model := NewPerformanceModel(config)

	// Add more than 1000 records
	for i := 0; i < 1050; i++ {
		record := &ChunkPerformanceRecord{
			ChunkID:        "test-chunk",
			Size:           1024 * 1024 * 10,
			UploadTime:     time.Second * 3,
			ThroughputMBps: 20.0,
			NetworkCondition: &NetworkCondition{
				BandwidthMBps: 25.0,
				LatencyMs:     20.0,
				Reliability:   0.9,
			},
			Success:   true,
			Timestamp: time.Now(),
		}
		model.UpdateModel(record)
	}

	// Should be limited to 1000
	assert.Equal(t, 1000, len(model.trainingData))
}

func TestPerformanceModel_Retrain(t *testing.T) {
	config := DefaultStagingConfig()
	model := NewPerformanceModel(config)

	// Should not retrain with insufficient data
	model.Retrain()
	assert.True(t, model.lastTraining.IsZero())

	// Add sufficient training data
	for i := 0; i < 15; i++ {
		record := &ChunkPerformanceRecord{
			Size:           1024 * 1024 * 10,
			ThroughputMBps: 20.0 + float64(i),
			NetworkCondition: &NetworkCondition{
				BandwidthMBps: 25.0 + float64(i),
				LatencyMs:     20.0,
				Reliability:   0.9,
			},
			Success: true,
		}
		model.UpdateModel(record)
	}

	// Should retrain now
	model.Retrain()
	assert.False(t, model.lastTraining.IsZero())
}

func TestPerformanceModel_CalculateCorrelation(t *testing.T) {
	config := DefaultStagingConfig()
	model := NewPerformanceModel(config)

	// Test with simple data
	data := []*ModelTrainingData{
		{ChunkSize: 10, ActualThroughput: 20},
		{ChunkSize: 20, ActualThroughput: 40},
		{ChunkSize: 30, ActualThroughput: 60},
		{ChunkSize: 40, ActualThroughput: 80},
	}

	// Perfect positive correlation
	correlation := model.calculateCorrelation(data,
		func(d *ModelTrainingData) float64 { return float64(d.ChunkSize) },
		func(d *ModelTrainingData) float64 { return d.ActualThroughput })

	assert.InDelta(t, 1.0, correlation, 0.01)

	// Test with insufficient data
	smallData := []*ModelTrainingData{
		{ChunkSize: 10, ActualThroughput: 20},
	}

	correlation = model.calculateCorrelation(smallData,
		func(d *ModelTrainingData) float64 { return float64(d.ChunkSize) },
		func(d *ModelTrainingData) float64 { return d.ActualThroughput })

	assert.Equal(t, 0.0, correlation)

	// Test with zero variance
	constantData := []*ModelTrainingData{
		{ChunkSize: 10, ActualThroughput: 20},
		{ChunkSize: 10, ActualThroughput: 20},
	}

	correlation = model.calculateCorrelation(constantData,
		func(d *ModelTrainingData) float64 { return float64(d.ChunkSize) },
		func(d *ModelTrainingData) float64 { return d.ActualThroughput })

	assert.Equal(t, 0.0, correlation)
}

func TestNewPerformanceHistory(t *testing.T) {
	config := DefaultStagingConfig()
	history := NewPerformanceHistory(config)

	assert.NotNil(t, history)
	assert.NotNil(t, history.performanceRecords)
	assert.NotNil(t, history.aggregatedStats)
	assert.Equal(t, 1000, history.maxHistorySize)
	assert.True(t, history.learningEnabled)
}

func TestPerformanceHistory_AddRecord(t *testing.T) {
	config := DefaultStagingConfig()
	history := NewPerformanceHistory(config)

	record := &ChunkPerformanceRecord{
		ChunkID:        "test-chunk-1",
		Size:           1024 * 1024 * 20,
		UploadTime:     time.Second * 5,
		ThroughputMBps: 15.0,
		Success:        true,
		Timestamp:      time.Now(),
	}

	history.AddRecord("test-chunk-1", record)

	assert.Contains(t, history.performanceRecords, "test-chunk-1")
	assert.Len(t, history.performanceRecords["test-chunk-1"], 1)
	assert.Equal(t, record, history.performanceRecords["test-chunk-1"][0])
}

func TestPerformanceHistory_AddRecordSizeLimit(t *testing.T) {
	config := DefaultStagingConfig()
	history := NewPerformanceHistory(config)
	history.maxHistorySize = 5 // Small limit for testing

	// Add more records than the limit
	for i := 0; i < 10; i++ {
		record := &ChunkPerformanceRecord{
			ChunkID:        "test-chunk",
			Size:           1024 * 1024 * 10,
			ThroughputMBps: 20.0,
			Success:        true,
		}
		history.AddRecord("test-chunk", record)
	}

	// Should be limited to maxHistorySize
	assert.LessOrEqual(t, len(history.performanceRecords["test-chunk"]), history.maxHistorySize)
}

func TestPerformanceHistory_GetPredictionAccuracy(t *testing.T) {
	config := DefaultStagingConfig()
	history := NewPerformanceHistory(config)

	// Initially should return default
	accuracy := history.GetPredictionAccuracy()
	assert.Equal(t, 0.5, accuracy)

	// Add some records
	for i := 0; i < 8; i++ {
		record := &ChunkPerformanceRecord{
			ChunkID: "test-chunk",
			Size:    1024 * 1024 * 10,
			NetworkCondition: &NetworkCondition{
				BandwidthMBps: 30.0,
				LatencyMs:     20.0,
				Reliability:   0.9,
			},
			Success: true,
		}
		history.AddRecord("test-chunk", record)
	}

	// Add some failures
	for i := 0; i < 2; i++ {
		record := &ChunkPerformanceRecord{
			ChunkID: "test-chunk",
			Size:    1024 * 1024 * 10,
			NetworkCondition: &NetworkCondition{
				BandwidthMBps: 30.0,
				LatencyMs:     20.0,
				Reliability:   0.9,
			},
			Success: false,
		}
		history.AddRecord("test-chunk", record)
	}

	accuracy = history.GetPredictionAccuracy()
	assert.Equal(t, 0.8, accuracy) // 8 successful out of 10
}

func TestPerformanceHistory_GetConfidenceForSize(t *testing.T) {
	config := DefaultStagingConfig()
	history := NewPerformanceHistory(config)

	testSize := int64(1024 * 1024 * 50) // 50MB

	// Initially should return default
	confidence := history.GetConfidenceForSize(testSize)
	assert.Equal(t, 0.5, confidence)

	// Add records for similar sized chunks
	for i := 0; i < 15; i++ {
		record := &ChunkPerformanceRecord{
			ChunkID: "test-chunk",
			Size:    testSize + int64(i*1024*1024), // Vary size slightly
			Success: i < 12,                        // 12 successful, 3 failed
		}
		history.AddRecord("test-chunk", record)
	}

	confidence = history.GetConfidenceForSize(testSize)

	// Should be higher than default due to success rate and data boost
	assert.Greater(t, confidence, 0.5)
	assert.LessOrEqual(t, confidence, 0.95)
}

func TestPerformanceHistory_GetSizeCategory(t *testing.T) {
	config := DefaultStagingConfig()
	history := NewPerformanceHistory(config)

	testCases := []struct {
		size     int64
		expected string
	}{
		{1024 * 1024 * 5, "small"},    // 5MB
		{1024 * 1024 * 25, "medium"},  // 25MB
		{1024 * 1024 * 75, "large"},   // 75MB
		{1024 * 1024 * 150, "xlarge"}, // 150MB
	}

	for _, tc := range testCases {
		category := history.getSizeCategory(tc.size)
		assert.Equal(t, tc.expected, category)
	}
}

func TestNewNetworkPerformanceIntegrator(t *testing.T) {
	integrator := NewNetworkPerformanceIntegrator()
	assert.NotNil(t, integrator)
}

func TestNewContentPerformanceAnalyzer(t *testing.T) {
	analyzer := NewContentPerformanceAnalyzer()
	assert.NotNil(t, analyzer)
}
