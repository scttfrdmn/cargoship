package staging

import (
	"context"
	"fmt"
	"math"
	"sync"
	"time"
)

// NewPerformancePredictor creates a new performance predictor.
func NewPerformancePredictor(config *StagingConfig) *PerformancePredictor {
	return &PerformancePredictor{
		performanceModel:  NewPerformanceModel(config),
		historicalData:    NewPerformanceHistory(config),
		networkIntegrator: NewNetworkPerformanceIntegrator(),
		contentAnalyzer:   NewContentPerformanceAnalyzer(),
		predictionCache:   make(map[string]*PerformancePrediction),
		cacheExpiry:       time.Minute * 5, // 5-minute cache expiry
	}
}

// Start begins the performance predictor.
func (pp *PerformancePredictor) Start(ctx context.Context) {
	// Start cache cleanup routine
	go pp.cacheCleanupLoop(ctx)
	
	// Start model update routine
	go pp.modelUpdateLoop(ctx)
}

// PredictPerformance predicts upload performance for a chunk boundary.
func (pp *PerformancePredictor) PredictPerformance(boundary ChunkBoundary, networkCondition *NetworkCondition) (*PerformancePrediction, error) {
	pp.mu.Lock()
	defer pp.mu.Unlock()
	
	// Check cache first
	cacheKey := pp.generateCacheKey(boundary, networkCondition)
	if cached, exists := pp.predictionCache[cacheKey]; exists {
		if time.Since(cached.GeneratedAt) < pp.cacheExpiry {
			return cached, nil
		}
		// Remove expired cache entry
		delete(pp.predictionCache, cacheKey)
	}
	
	// Generate new prediction
	prediction := pp.generatePrediction(boundary, networkCondition)
	
	// Cache the prediction
	pp.predictionCache[cacheKey] = prediction
	
	return prediction, nil
}

// UpdateHistory updates performance history with actual results.
func (pp *PerformancePredictor) UpdateHistory(chunkID string, record *ChunkPerformanceRecord) {
	pp.historicalData.AddRecord(chunkID, record)
	
	// Update models based on new data
	pp.performanceModel.UpdateModel(record)
}

// GetAccuracy returns current prediction accuracy.
func (pp *PerformancePredictor) GetAccuracy() float64 {
	return pp.historicalData.GetPredictionAccuracy()
}

// UpdateModels triggers model updates based on recent performance data.
func (pp *PerformancePredictor) UpdateModels() {
	pp.performanceModel.Retrain()
}

// generatePrediction generates a new performance prediction.
func (pp *PerformancePredictor) generatePrediction(boundary ChunkBoundary, networkCondition *NetworkCondition) *PerformancePrediction {
	// Predict upload time
	uploadTime := pp.predictUploadTime(boundary, networkCondition)
	
	// Predict throughput
	throughput := pp.predictThroughput(boundary, networkCondition)
	
	// Predict success probability
	successProb := pp.predictSuccessProbability(boundary, networkCondition)
	
	// Determine optimal chunk size
	optimalSize := pp.determineOptimalChunkSize(boundary, networkCondition)
	
	// Recommend compression
	compression := pp.recommendCompression(boundary, networkCondition)
	
	// Calculate network suitability
	networkSuitability := pp.calculateNetworkSuitability(boundary, networkCondition)
	
	// Calculate overall confidence
	confidence := pp.calculatePredictionConfidence(boundary, networkCondition)
	
	return &PerformancePrediction{
		EstimatedUploadTime:    uploadTime,
		PredictedThroughput:    throughput,
		SuccessProbability:     successProb,
		OptimalChunkSize:       optimalSize,
		RecommendedCompression: compression,
		NetworkSuitability:     networkSuitability,
		Confidence:             confidence,
		GeneratedAt:            time.Now(),
	}
}

// predictUploadTime predicts the time required to upload a chunk.
func (pp *PerformancePredictor) predictUploadTime(boundary ChunkBoundary, networkCondition *NetworkCondition) time.Duration {
	// Base calculation: size / bandwidth
	sizeGB := float64(boundary.Size) / (1024 * 1024 * 1024)
	throughputGBps := networkCondition.BandwidthMBps / 1024
	
	if throughputGBps <= 0 {
		return time.Hour // Very conservative estimate for poor network
	}
	
	baseTime := sizeGB / throughputGBps
	
	// Adjust for compression (reduces transfer time)
	compressionFactor := boundary.CompressionScore
	if compressionFactor > 0 {
		baseTime *= (1.0 - compressionFactor*0.5) // Up to 50% reduction
	}
	
	// Adjust for network conditions
	latencyPenalty := networkCondition.LatencyMs / 1000.0 // Convert to seconds
	congestionPenalty := networkCondition.CongestionLevel * baseTime * 0.5
	reliabilityPenalty := (1.0 - networkCondition.Reliability) * baseTime * 0.3
	
	totalTime := baseTime + latencyPenalty + congestionPenalty + reliabilityPenalty
	
	// Add overhead for chunk staging and processing
	overhead := time.Second * 2 // 2-second overhead per chunk
	
	return time.Duration(totalTime*float64(time.Second)) + overhead
}

// predictThroughput predicts effective throughput for the chunk.
func (pp *PerformancePredictor) predictThroughput(boundary ChunkBoundary, networkCondition *NetworkCondition) float64 {
	baseThroughput := networkCondition.BandwidthMBps
	
	// Adjust for compression benefits
	compressionBoost := boundary.CompressionScore * 0.3 // Up to 30% effective throughput increase
	
	// Adjust for network conditions
	congestionReduction := networkCondition.CongestionLevel * 0.4
	reliabilityReduction := (1.0 - networkCondition.Reliability) * 0.2
	
	effectiveThroughput := baseThroughput * (1.0 + compressionBoost - congestionReduction - reliabilityReduction)
	
	return math.Max(effectiveThroughput, baseThroughput*0.1) // At least 10% of base throughput
}

// predictSuccessProbability predicts the probability of successful upload.
func (pp *PerformancePredictor) predictSuccessProbability(boundary ChunkBoundary, networkCondition *NetworkCondition) float64 {
	baseProbability := networkCondition.Reliability
	
	// Adjust based on chunk size (very large chunks are riskier)
	sizePenalty := 0.0
	if boundary.Size > 100*1024*1024 { // > 100MB
		sizePenalty = 0.1
	} else if boundary.Size > 50*1024*1024 { // > 50MB
		sizePenalty = 0.05
	}
	
	// Adjust for network conditions
	congestionPenalty := networkCondition.CongestionLevel * 0.2
	latencyPenalty := math.Min(networkCondition.LatencyMs/1000.0, 0.1) // Max 10% penalty
	
	successProb := baseProbability - sizePenalty - congestionPenalty - latencyPenalty
	
	return math.Max(math.Min(successProb, 0.99), 0.1) // Clamp between 10% and 99%
}

// determineOptimalChunkSize determines the optimal chunk size for current conditions.
func (pp *PerformancePredictor) determineOptimalChunkSize(boundary ChunkBoundary, networkCondition *NetworkCondition) int64 {
	// Base optimal size based on network bandwidth
	baseOptimalMB := networkCondition.BandwidthMBps * 2 // 2 seconds worth of data
	
	// Adjust for network reliability
	reliabilityFactor := networkCondition.Reliability
	if reliabilityFactor < 0.8 {
		baseOptimalMB *= 0.5 // Smaller chunks for unreliable networks
	}
	
	// Adjust for latency
	if networkCondition.LatencyMs > 100 {
		baseOptimalMB *= 1.5 // Larger chunks to amortize latency
	}
	
	// Adjust for congestion
	if networkCondition.CongestionLevel > 0.5 {
		baseOptimalMB *= 0.7 // Smaller chunks during congestion
	}
	
	// Convert to bytes and apply bounds
	optimalBytes := int64(baseOptimalMB * 1024 * 1024)
	minSize := int64(5 * 1024 * 1024)   // 5MB minimum
	maxSize := int64(100 * 1024 * 1024) // 100MB maximum
	
	return int64(math.Max(float64(minSize), math.Min(float64(maxSize), float64(optimalBytes))))
}

// recommendCompression recommends the best compression algorithm.
func (pp *PerformancePredictor) recommendCompression(boundary ChunkBoundary, networkCondition *NetworkCondition) string {
	// High compression for slow networks
	if networkCondition.BandwidthMBps < 10 {
		return "zstd-max"
	}
	
	// Fast compression for fast networks
	if networkCondition.BandwidthMBps > 100 {
		return "zstd-fast"
	}
	
	// Balanced compression for most cases
	if boundary.CompressionScore > 0.5 {
		return "zstd"
	}
	
	return "zstd-fast"
}

// calculateNetworkSuitability calculates how suitable the network is for the chunk.
func (pp *PerformancePredictor) calculateNetworkSuitability(boundary ChunkBoundary, networkCondition *NetworkCondition) float64 {
	suitability := 0.0
	
	// Bandwidth suitability
	chunkSizeMB := float64(boundary.Size) / (1024 * 1024)
	transferTime := chunkSizeMB / networkCondition.BandwidthMBps
	
	if transferTime < 10 { // Less than 10 seconds
		suitability += 0.4
	} else if transferTime < 30 { // Less than 30 seconds
		suitability += 0.2
	}
	
	// Reliability suitability
	suitability += networkCondition.Reliability * 0.3
	
	// Congestion suitability
	suitability += (1.0 - networkCondition.CongestionLevel) * 0.2
	
	// Latency suitability
	if networkCondition.LatencyMs < 50 {
		suitability += 0.1
	}
	
	return math.Min(suitability, 1.0)
}

// calculatePredictionConfidence calculates overall prediction confidence.
func (pp *PerformancePredictor) calculatePredictionConfidence(boundary ChunkBoundary, networkCondition *NetworkCondition) float64 {
	confidence := 0.5 // Base confidence
	
	// Increase confidence with network reliability
	confidence += networkCondition.Reliability * 0.3
	
	// Decrease confidence with high congestion
	confidence -= networkCondition.CongestionLevel * 0.2
	
	// Adjust based on historical data availability
	historicalConfidence := pp.historicalData.GetConfidenceForSize(boundary.Size)
	confidence = confidence*0.7 + historicalConfidence*0.3
	
	return math.Max(math.Min(confidence, 0.95), 0.1)
}

// generateCacheKey generates a cache key for the prediction.
func (pp *PerformancePredictor) generateCacheKey(boundary ChunkBoundary, networkCondition *NetworkCondition) string {
	return fmt.Sprintf("boundary_%d_%d_network_%.1f_%.1f_%.3f", 
		boundary.Size, 
		int(boundary.CompressionScore*100),
		networkCondition.BandwidthMBps,
		networkCondition.LatencyMs,
		networkCondition.CongestionLevel,
	)
}

// cacheCleanupLoop runs periodic cache cleanup.
func (pp *PerformancePredictor) cacheCleanupLoop(ctx context.Context) {
	ticker := time.NewTicker(time.Minute * 2)
	defer ticker.Stop()
	
	for {
		select {
		case <-ctx.Done():
			return
		case <-ticker.C:
			pp.cleanupExpiredCache()
		}
	}
}

// cleanupExpiredCache removes expired cache entries.
func (pp *PerformancePredictor) cleanupExpiredCache() {
	pp.mu.Lock()
	defer pp.mu.Unlock()
	
	now := time.Now()
	for key, prediction := range pp.predictionCache {
		if now.Sub(prediction.GeneratedAt) > pp.cacheExpiry {
			delete(pp.predictionCache, key)
		}
	}
}

// modelUpdateLoop runs periodic model updates.
func (pp *PerformancePredictor) modelUpdateLoop(ctx context.Context) {
	ticker := time.NewTicker(time.Minute * 10)
	defer ticker.Stop()
	
	for {
		select {
		case <-ctx.Done():
			return
		case <-ticker.C:
			pp.UpdateModels()
		}
	}
}

// PerformanceModel represents a machine learning model for performance prediction.
type PerformanceModel struct {
	config         *StagingConfig
	modelWeights   map[string]float64
	trainingData   []*ModelTrainingData
	lastTraining   time.Time
	mu             sync.RWMutex
}

// NewPerformanceModel creates a new performance model.
func NewPerformanceModel(config *StagingConfig) *PerformanceModel {
	return &PerformanceModel{
		config: config,
		modelWeights: map[string]float64{
			"size_factor":        0.4,
			"bandwidth_factor":   0.3,
			"latency_factor":     0.1,
			"compression_factor": 0.1,
			"reliability_factor": 0.1,
		},
		trainingData: make([]*ModelTrainingData, 0),
	}
}

// UpdateModel updates the model with new performance data.
func (pm *PerformanceModel) UpdateModel(record *ChunkPerformanceRecord) {
	pm.mu.Lock()
	defer pm.mu.Unlock()
	
	// Convert record to training data
	trainingData := &ModelTrainingData{
		ChunkSize:        record.Size,
		CompressionRatio: record.CompressionRatio,
		NetworkBandwidth: record.NetworkCondition.BandwidthMBps,
		NetworkLatency:   record.NetworkCondition.LatencyMs,
		NetworkReliability: record.NetworkCondition.Reliability,
		ActualThroughput: record.ThroughputMBps,
		ActualUploadTime: record.UploadTime,
		Success:          record.Success,
		Timestamp:        record.Timestamp,
	}
	
	pm.trainingData = append(pm.trainingData, trainingData)
	
	// Limit training data size
	maxTrainingData := 1000
	if len(pm.trainingData) > maxTrainingData {
		pm.trainingData = pm.trainingData[len(pm.trainingData)-maxTrainingData:]
	}
}

// Retrain retrains the performance model.
func (pm *PerformanceModel) Retrain() {
	pm.mu.Lock()
	defer pm.mu.Unlock()
	
	if len(pm.trainingData) < 10 {
		return // Need at least 10 data points
	}
	
	// Simple linear regression to update weights
	pm.updateWeights()
	pm.lastTraining = time.Now()
}

// updateWeights updates model weights based on training data.
func (pm *PerformanceModel) updateWeights() {
	// This is a simplified weight update mechanism
	// In practice, you would use a more sophisticated ML algorithm
	
	successfulUploads := make([]*ModelTrainingData, 0)
	for _, data := range pm.trainingData {
		if data.Success {
			successfulUploads = append(successfulUploads, data)
		}
	}
	
	if len(successfulUploads) == 0 {
		return
	}
	
	// Calculate correlation between features and throughput
	correlations := make(map[string]float64)
	
	// Size correlation (negative - larger chunks may have lower effective throughput)
	correlations["size_factor"] = pm.calculateCorrelation(successfulUploads, 
		func(d *ModelTrainingData) float64 { return -float64(d.ChunkSize) },
		func(d *ModelTrainingData) float64 { return d.ActualThroughput })
	
	// Bandwidth correlation (positive)
	correlations["bandwidth_factor"] = pm.calculateCorrelation(successfulUploads,
		func(d *ModelTrainingData) float64 { return d.NetworkBandwidth },
		func(d *ModelTrainingData) float64 { return d.ActualThroughput })
	
	// Latency correlation (negative)
	correlations["latency_factor"] = pm.calculateCorrelation(successfulUploads,
		func(d *ModelTrainingData) float64 { return -d.NetworkLatency },
		func(d *ModelTrainingData) float64 { return d.ActualThroughput })
	
	// Update weights based on correlations
	totalCorrelation := 0.0
	for _, corr := range correlations {
		totalCorrelation += math.Abs(corr)
	}
	
	if totalCorrelation > 0 {
		for key, corr := range correlations {
			pm.modelWeights[key] = math.Abs(corr) / totalCorrelation
		}
	}
}

// calculateCorrelation calculates correlation between two variables.
func (pm *PerformanceModel) calculateCorrelation(data []*ModelTrainingData, 
	xFunc, yFunc func(*ModelTrainingData) float64) float64 {
	
	if len(data) < 2 {
		return 0.0
	}
	
	// Calculate means
	xMean := 0.0
	yMean := 0.0
	for _, d := range data {
		xMean += xFunc(d)
		yMean += yFunc(d)
	}
	xMean /= float64(len(data))
	yMean /= float64(len(data))
	
	// Calculate correlation
	numerator := 0.0
	xVar := 0.0
	yVar := 0.0
	
	for _, d := range data {
		x := xFunc(d)
		y := yFunc(d)
		
		xDiff := x - xMean
		yDiff := y - yMean
		
		numerator += xDiff * yDiff
		xVar += xDiff * xDiff
		yVar += yDiff * yDiff
	}
	
	denominator := math.Sqrt(xVar * yVar)
	if denominator == 0 {
		return 0.0
	}
	
	return numerator / denominator
}

// ModelTrainingData represents training data for the performance model.
type ModelTrainingData struct {
	ChunkSize          int64
	CompressionRatio   float64
	NetworkBandwidth   float64
	NetworkLatency     float64
	NetworkReliability float64
	ActualThroughput   float64
	ActualUploadTime   time.Duration
	Success            bool
	Timestamp          time.Time
}

// NewPerformanceHistory creates a new performance history tracker.
func NewPerformanceHistory(config *StagingConfig) *ChunkPerformanceHistory {
	return &ChunkPerformanceHistory{
		performanceRecords: make(map[string][]*ChunkPerformanceRecord),
		aggregatedStats:    make(map[string]*AggregatedStats),
		maxHistorySize:     1000,
		learningEnabled:    true,
	}
}

// AddRecord adds a performance record to the history.
func (cph *ChunkPerformanceHistory) AddRecord(chunkID string, record *ChunkPerformanceRecord) {
	cph.mu.Lock()
	defer cph.mu.Unlock()
	
	// Add to records
	if cph.performanceRecords[chunkID] == nil {
		cph.performanceRecords[chunkID] = make([]*ChunkPerformanceRecord, 0)
	}
	
	cph.performanceRecords[chunkID] = append(cph.performanceRecords[chunkID], record)
	
	// Update aggregated stats
	cph.updateAggregatedStats(record)
	
	// Limit history size
	if len(cph.performanceRecords[chunkID]) > cph.maxHistorySize {
		cph.performanceRecords[chunkID] = cph.performanceRecords[chunkID][1:]
	}
}

// GetPredictionAccuracy calculates overall prediction accuracy.
func (cph *ChunkPerformanceHistory) GetPredictionAccuracy() float64 {
	cph.mu.RLock()
	defer cph.mu.RUnlock()
	
	totalRecords := 0
	successfulPredictions := 0
	
	for _, records := range cph.performanceRecords {
		for _, record := range records {
			totalRecords++
			if record.Success {
				successfulPredictions++
			}
		}
	}
	
	if totalRecords == 0 {
		return 0.5 // Default accuracy
	}
	
	return float64(successfulPredictions) / float64(totalRecords)
}

// GetConfidenceForSize returns confidence level for chunks of a given size.
func (cph *ChunkPerformanceHistory) GetConfidenceForSize(size int64) float64 {
	cph.mu.RLock()
	defer cph.mu.RUnlock()
	
	// Find records for similar sized chunks
	tolerance := size / 10 // 10% tolerance
	matchingRecords := 0
	successfulRecords := 0
	
	for _, records := range cph.performanceRecords {
		for _, record := range records {
			if math.Abs(float64(record.Size-size)) <= float64(tolerance) {
				matchingRecords++
				if record.Success {
					successfulRecords++
				}
			}
		}
	}
	
	if matchingRecords == 0 {
		return 0.5 // Default confidence
	}
	
	confidence := float64(successfulRecords) / float64(matchingRecords)
	
	// Boost confidence with more data points
	dataBoost := math.Min(float64(matchingRecords)/100.0, 0.2) // Up to 20% boost
	
	return math.Min(confidence+dataBoost, 0.95)
}

// updateAggregatedStats updates aggregated statistics.
func (cph *ChunkPerformanceHistory) updateAggregatedStats(record *ChunkPerformanceRecord) {
	sizeCategory := cph.getSizeCategory(record.Size)
	
	if cph.aggregatedStats[sizeCategory] == nil {
		cph.aggregatedStats[sizeCategory] = &AggregatedStats{
			TotalRecords:     0,
			SuccessfulRecords: 0,
			AverageThroughput: 0,
			AverageUploadTime: 0,
		}
	}
	
	stats := cph.aggregatedStats[sizeCategory]
	
	// Update counts
	stats.TotalRecords++
	if record.Success {
		stats.SuccessfulRecords++
	}
	
	// Update averages (running average)
	stats.AverageThroughput = (stats.AverageThroughput*float64(stats.TotalRecords-1) + record.ThroughputMBps) / float64(stats.TotalRecords)
	
	uploadTimeSeconds := record.UploadTime.Seconds()
	avgUploadTimeSeconds := stats.AverageUploadTime.Seconds()
	newAvgSeconds := (avgUploadTimeSeconds*float64(stats.TotalRecords-1) + uploadTimeSeconds) / float64(stats.TotalRecords)
	stats.AverageUploadTime = time.Duration(newAvgSeconds * float64(time.Second))
}

// getSizeCategory categorizes chunk size for aggregated statistics.
func (cph *ChunkPerformanceHistory) getSizeCategory(size int64) string {
	sizeMB := size / (1024 * 1024)
	
	if sizeMB < 10 {
		return "small"
	} else if sizeMB < 50 {
		return "medium"
	} else if sizeMB < 100 {
		return "large"
	}
	
	return "xlarge"
}

// AggregatedStats represents aggregated performance statistics.
type AggregatedStats struct {
	TotalRecords      int
	SuccessfulRecords int
	AverageThroughput float64
	AverageUploadTime time.Duration
}

// NewNetworkPerformanceIntegrator creates a network performance integrator.
func NewNetworkPerformanceIntegrator() *NetworkPerformanceIntegrator {
	return &NetworkPerformanceIntegrator{}
}

// NetworkPerformanceIntegrator integrates network conditions with performance predictions.
type NetworkPerformanceIntegrator struct {
}

// NewContentPerformanceAnalyzer creates a content performance analyzer.
func NewContentPerformanceAnalyzer() *ContentPerformanceAnalyzer {
	return &ContentPerformanceAnalyzer{}
}

// ContentPerformanceAnalyzer analyzes content characteristics for performance prediction.
type ContentPerformanceAnalyzer struct {
}