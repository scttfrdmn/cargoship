/*
Package s3 scheduler implements intelligent transfer scheduling for cross-prefix coordination.

This module provides sophisticated scheduling algorithms that optimize transfer performance
across multiple S3 prefixes with adaptive load balancing and congestion awareness.
*/
package s3

import (
	"context"
	"fmt"
	"math"
	"sort"
	"time"
)

// Start begins the transfer scheduler operation.
func (ts *TransferScheduler) Start(ctx context.Context) {
	go ts.schedulingLoop(ctx)
	go ts.metricsCollectionLoop(ctx)
	go ts.adaptiveOptimizationLoop(ctx)
}

// RegisterPrefix registers a new S3 prefix with the scheduler.
func (ts *TransferScheduler) RegisterPrefix(prefixID string, capacity float64) {
	ts.mu.Lock()
	defer ts.mu.Unlock()
	
	ts.prefixMetrics[prefixID] = &PrefixPerformanceMetrics{
		PrefixID:               prefixID,
		ActiveUploads:          0,
		ThroughputMBps:         0,
		LatencyMs:              50, // Default baseline
		ErrorRate:              0,
		CongestionWindow:       ts.config.GlobalCongestionWindow / 4, // Start conservative
		LastUpdate:             time.Now(),
		BandwidthUtilization:   0,
		ThroughputHistory:      make([]float64, 0, 20),
		LatencyHistory:         make([]float64, 0, 20),
		ErrorHistory:           make([]float64, 0, 20),
		QueueLength:            0,
		ProcessingCapacity:     capacity,
	}
	
	ts.globalState.ActivePrefixes[prefixID] = true
	ts.loadBalancer.RegisterPrefix(prefixID, capacity)
}

// SelectOptimalPrefix selects the best prefix for a given upload based on current performance.
func (ts *TransferScheduler) SelectOptimalPrefix(upload *ScheduledUpload) (string, error) {
	ts.mu.RLock()
	defer ts.mu.RUnlock()
	
	if len(ts.prefixMetrics) == 0 {
		return "", fmt.Errorf("no prefixes registered")
	}
	
	switch ts.config.Strategy {
	case "tcp_like":
		return ts.selectPrefixTCPLike(upload)
	case "fair_share":
		return ts.selectPrefixFairShare(upload)
	case "adaptive":
		return ts.selectPrefixAdaptive(upload)
	default:
		return ts.selectPrefixAdaptive(upload)
	}
}

// selectPrefixTCPLike implements TCP-like prefix selection with congestion awareness.
func (ts *TransferScheduler) selectPrefixTCPLike(upload *ScheduledUpload) (string, error) {
	type prefixScore struct {
		prefixID string
		score    float64
		metrics  *PrefixPerformanceMetrics
	}
	
	var candidates []prefixScore
	
	for prefixID, metrics := range ts.prefixMetrics {
		// Calculate TCP-like score based on congestion window and performance
		congestionFactor := float64(metrics.CongestionWindow) / float64(ts.config.GlobalCongestionWindow)
		throughputFactor := metrics.ThroughputMBps / ts.networkProfile.EstimatedBandwidthMBps
		latencyFactor := 1.0 / (1.0 + metrics.LatencyMs/100.0) // Lower latency = higher score
		errorFactor := 1.0 - metrics.ErrorRate
		loadFactor := 1.0 - (float64(metrics.QueueLength) / (metrics.ProcessingCapacity + 1))
		
		// Combine factors with weights
		score := (congestionFactor * 0.3) + 
				(throughputFactor * 0.25) + 
				(latencyFactor * 0.2) + 
				(errorFactor * 0.15) + 
				(loadFactor * 0.1)
		
		candidates = append(candidates, prefixScore{
			prefixID: prefixID,
			score:    score,
			metrics:  metrics,
		})
	}
	
	// Sort by score (highest first)
	sort.Slice(candidates, func(i, j int) bool {
		return candidates[i].score > candidates[j].score
	})
	
	// Apply some randomization to avoid thundering herd
	if len(candidates) > 1 && candidates[0].score-candidates[1].score < 0.1 {
		// Scores are close, add some randomization
		if time.Now().UnixNano()%2 == 0 {
			return candidates[1].prefixID, nil
		}
	}
	
	return candidates[0].prefixID, nil
}

// selectPrefixFairShare implements fair-share prefix selection.
func (ts *TransferScheduler) selectPrefixFairShare(upload *ScheduledUpload) (string, error) {
	var bestPrefix string
	lowestUtilization := math.Inf(1)
	
	for prefixID, metrics := range ts.prefixMetrics {
		utilization := metrics.BandwidthUtilization
		if metrics.ProcessingCapacity > 0 {
			utilization += float64(metrics.QueueLength) / metrics.ProcessingCapacity
		}
		
		if utilization < lowestUtilization {
			lowestUtilization = utilization
			bestPrefix = prefixID
		}
	}
	
	if bestPrefix == "" {
		// Fallback to first available prefix
		for prefixID := range ts.prefixMetrics {
			return prefixID, nil
		}
		return "", fmt.Errorf("no available prefixes")
	}
	
	return bestPrefix, nil
}

// selectPrefixAdaptive implements adaptive prefix selection with machine learning.
func (ts *TransferScheduler) selectPrefixAdaptive(upload *ScheduledUpload) (string, error) {
	// Start with TCP-like selection
	tcpPrefix, err := ts.selectPrefixTCPLike(upload)
	if err != nil {
		return "", err
	}
	
	// Apply adaptive adjustments based on historical performance
	adaptivePrefix := ts.applyAdaptiveAdjustments(tcpPrefix, upload)
	
	// Use network profile learning to refine selection
	finalPrefix := ts.applyNetworkProfileLearning(adaptivePrefix, upload)
	
	return finalPrefix, nil
}

// applyAdaptiveAdjustments applies adaptive learning to prefix selection.
func (ts *TransferScheduler) applyAdaptiveAdjustments(selectedPrefix string, upload *ScheduledUpload) string {
	metrics := ts.prefixMetrics[selectedPrefix]
	
	// Check if this prefix has been consistently underperforming
	if len(metrics.ThroughputHistory) >= 5 {
		recentAvg := ts.calculateRecentAverage(metrics.ThroughputHistory, 5)
		overallAvg := ts.calculateOverallAverage(metrics.ThroughputHistory)
		
		// If recent performance is significantly worse, consider alternatives
		if recentAvg < overallAvg*0.7 {
			// Look for a better alternative
			for prefixID, altMetrics := range ts.prefixMetrics {
				if prefixID != selectedPrefix && len(altMetrics.ThroughputHistory) >= 3 {
					altRecentAvg := ts.calculateRecentAverage(altMetrics.ThroughputHistory, 3)
					if altRecentAvg > recentAvg*1.2 && altMetrics.QueueLength < int(altMetrics.ProcessingCapacity*0.8) {
						return prefixID
					}
				}
			}
		}
	}
	
	return selectedPrefix
}

// applyNetworkProfileLearning applies network profile learning to selection.
func (ts *TransferScheduler) applyNetworkProfileLearning(selectedPrefix string, upload *ScheduledUpload) string {
	// Use network profile to predict optimal prefix based on upload characteristics
	if ts.networkProfile.LearningConfidence > 0.7 {
		// High confidence in network profile, apply learned optimizations
		
		// For large uploads, prefer prefixes with higher bandwidth trends
		if upload.EstimatedSize > 1024*1024*1024 { // > 1GB
			if ts.networkProfile.BandwidthTrend == TrendIncreasing {
				// Look for prefixes with recent bandwidth increases
				bestPrefix := ts.findPrefixWithBandwidthTrend(TrendIncreasing)
				if bestPrefix != "" {
					return bestPrefix
				}
			}
		}
		
		// For time-sensitive uploads, prefer low-latency prefixes
		if !upload.Deadline.IsZero() && time.Until(upload.Deadline) < time.Hour {
			lowLatencyPrefix := ts.findLowestLatencyPrefix()
			if lowLatencyPrefix != "" {
				return lowLatencyPrefix
			}
		}
	}
	
	return selectedPrefix
}

// UpdatePrefixMetrics updates performance metrics for a specific prefix.
func (ts *TransferScheduler) UpdatePrefixMetrics(prefixID string, metrics *PrefixPerformanceMetrics) {
	ts.mu.Lock()
	defer ts.mu.Unlock()
	
	existing, exists := ts.prefixMetrics[prefixID]
	if !exists {
		return
	}
	
	// Update current metrics
	existing.ThroughputMBps = metrics.ThroughputMBps
	existing.LatencyMs = metrics.LatencyMs
	existing.ErrorRate = metrics.ErrorRate
	existing.ActiveUploads = metrics.ActiveUploads
	existing.BandwidthUtilization = metrics.BandwidthUtilization
	existing.QueueLength = metrics.QueueLength
	existing.LastUpdate = time.Now()
	
	// Update historical data
	ts.updateHistoricalMetrics(existing, metrics)
	
	// Update global state
	ts.updateGlobalState()
	
	// Trigger network profile learning
	ts.updateNetworkProfile(metrics)
}

// updateHistoricalMetrics updates historical performance data.
func (ts *TransferScheduler) updateHistoricalMetrics(existing, new *PrefixPerformanceMetrics) {
	maxHistory := 20
	
	// Update throughput history
	existing.ThroughputHistory = append(existing.ThroughputHistory, new.ThroughputMBps)
	if len(existing.ThroughputHistory) > maxHistory {
		existing.ThroughputHistory = existing.ThroughputHistory[1:]
	}
	
	// Update latency history
	existing.LatencyHistory = append(existing.LatencyHistory, new.LatencyMs)
	if len(existing.LatencyHistory) > maxHistory {
		existing.LatencyHistory = existing.LatencyHistory[1:]
	}
	
	// Update error history
	existing.ErrorHistory = append(existing.ErrorHistory, new.ErrorRate)
	if len(existing.ErrorHistory) > maxHistory {
		existing.ErrorHistory = existing.ErrorHistory[1:]
	}
}

// updateGlobalState updates the global transfer state based on current metrics.
func (ts *TransferScheduler) updateGlobalState() {
	totalThroughput := 0.0
	totalUploads := 0
	totalErrors := 0.0
	activeCount := 0
	
	for _, metrics := range ts.prefixMetrics {
		totalThroughput += metrics.ThroughputMBps
		totalUploads += metrics.ActiveUploads
		totalErrors += metrics.ErrorRate
		if metrics.ActiveUploads > 0 {
			activeCount++
		}
	}
	
	ts.globalState.TotalActiveUploads = totalUploads
	ts.globalState.GlobalThroughputMBps = totalThroughput
	if activeCount > 0 {
		ts.globalState.GlobalErrorRate = totalErrors / float64(activeCount)
	}
	ts.globalState.LastUpdate = time.Now()
}

// updateNetworkProfile updates the network profile based on new metrics.
func (ts *TransferScheduler) updateNetworkProfile(metrics *PrefixPerformanceMetrics) {
	// Update estimated bandwidth based on observed performance
	if metrics.ThroughputMBps > ts.networkProfile.EstimatedBandwidthMBps {
		// New peak observed, update estimate
		learningRate := 0.1
		ts.networkProfile.EstimatedBandwidthMBps = 
			(1-learningRate)*ts.networkProfile.EstimatedBandwidthMBps + 
			learningRate*metrics.ThroughputMBps
	}
	
	// Update baseline RTT
	if metrics.LatencyMs > 0 {
		newRTT := time.Duration(metrics.LatencyMs) * time.Millisecond
		if newRTT < ts.networkProfile.BaselineRTT || ts.networkProfile.BaselineRTT == 0 {
			ts.networkProfile.BaselineRTT = newRTT
		}
	}
	
	// Update trends
	ts.updatePerformanceTrends()
	
	// Increase learning confidence over time
	ts.networkProfile.LearningConfidence = math.Min(
		ts.networkProfile.LearningConfidence + 0.01, 
		1.0,
	)
	
	ts.networkProfile.LastProfileUpdate = time.Now()
}

// updatePerformanceTrends analyzes trends in network performance.
func (ts *TransferScheduler) updatePerformanceTrends() {
	// Analyze bandwidth trend across all prefixes
	recentBandwidth := 0.0
	historicalBandwidth := 0.0
	prefixCount := 0
	
	for _, metrics := range ts.prefixMetrics {
		if len(metrics.ThroughputHistory) >= 5 {
			recent := ts.calculateRecentAverage(metrics.ThroughputHistory, 3)
			historical := ts.calculateOverallAverage(metrics.ThroughputHistory)
			
			recentBandwidth += recent
			historicalBandwidth += historical
			prefixCount++
		}
	}
	
	if prefixCount > 0 {
		avgRecent := recentBandwidth / float64(prefixCount)
		avgHistorical := historicalBandwidth / float64(prefixCount)
		
		diff := (avgRecent - avgHistorical) / avgHistorical
		
		if diff > 0.1 {
			ts.networkProfile.BandwidthTrend = TrendIncreasing
		} else if diff < -0.1 {
			ts.networkProfile.BandwidthTrend = TrendDecreasing
		} else {
			ts.networkProfile.BandwidthTrend = TrendStable
		}
	}
}

// GetMetrics returns comprehensive scheduler metrics.
func (ts *TransferScheduler) GetMetrics() *SchedulerMetrics {
	ts.mu.RLock()
	defer ts.mu.RUnlock()
	
	return &SchedulerMetrics{
		GlobalThroughputMBps:  ts.globalState.GlobalThroughputMBps,
		LoadBalanceEfficiency: ts.calculateLoadBalanceEfficiency(),
		ActivePrefixes:        len(ts.globalState.ActivePrefixes),
		AverageQueueLength:    ts.calculateAverageQueueLength(),
		NetworkUtilization:    ts.calculateNetworkUtilization(),
		AdaptationRate:        ts.networkProfile.LearningConfidence,
		LastUpdate:            time.Now(),
	}
}

// SchedulerMetrics provides comprehensive scheduler performance metrics.
type SchedulerMetrics struct {
	GlobalThroughputMBps  float64
	LoadBalanceEfficiency float64
	ActivePrefixes        int
	AverageQueueLength    float64
	NetworkUtilization    float64
	AdaptationRate        float64
	LastUpdate            time.Time
}

// Helper methods

func (ts *TransferScheduler) calculateRecentAverage(values []float64, count int) float64 {
	if len(values) == 0 {
		return 0
	}
	
	start := len(values) - count
	if start < 0 {
		start = 0
	}
	
	sum := 0.0
	for i := start; i < len(values); i++ {
		sum += values[i]
	}
	
	return sum / float64(len(values)-start)
}

func (ts *TransferScheduler) calculateOverallAverage(values []float64) float64 {
	if len(values) == 0 {
		return 0
	}
	
	sum := 0.0
	for _, v := range values {
		sum += v
	}
	
	return sum / float64(len(values))
}

func (ts *TransferScheduler) findPrefixWithBandwidthTrend(trend TrendDirection) string {
	bestPrefix := ""
	bestTrend := 0.0
	
	for prefixID, metrics := range ts.prefixMetrics {
		if len(metrics.ThroughputHistory) >= 5 {
			recent := ts.calculateRecentAverage(metrics.ThroughputHistory, 3)
			historical := ts.calculateOverallAverage(metrics.ThroughputHistory)
			trendValue := (recent - historical) / historical
			
			if trend == TrendIncreasing && trendValue > bestTrend {
				bestTrend = trendValue
				bestPrefix = prefixID
			}
		}
	}
	
	return bestPrefix
}

func (ts *TransferScheduler) findLowestLatencyPrefix() string {
	bestPrefix := ""
	lowestLatency := math.Inf(1)
	
	for prefixID, metrics := range ts.prefixMetrics {
		if metrics.LatencyMs < lowestLatency && metrics.QueueLength < int(metrics.ProcessingCapacity*0.8) {
			lowestLatency = metrics.LatencyMs
			bestPrefix = prefixID
		}
	}
	
	return bestPrefix
}

func (ts *TransferScheduler) calculateLoadBalanceEfficiency() float64 {
	if len(ts.prefixMetrics) == 0 {
		return 1.0
	}
	
	utilizationSum := 0.0
	utilizationVariance := 0.0
	
	// Calculate average utilization
	for _, metrics := range ts.prefixMetrics {
		utilization := metrics.BandwidthUtilization
		utilizationSum += utilization
	}
	avgUtilization := utilizationSum / float64(len(ts.prefixMetrics))
	
	// Calculate variance
	for _, metrics := range ts.prefixMetrics {
		diff := metrics.BandwidthUtilization - avgUtilization
		utilizationVariance += diff * diff
	}
	utilizationVariance /= float64(len(ts.prefixMetrics))
	
	// Efficiency is inversely related to variance (lower variance = better balance)
	efficiency := 1.0 / (1.0 + utilizationVariance)
	return math.Min(efficiency, 1.0)
}

func (ts *TransferScheduler) calculateAverageQueueLength() float64 {
	if len(ts.prefixMetrics) == 0 {
		return 0
	}
	
	totalQueue := 0
	for _, metrics := range ts.prefixMetrics {
		totalQueue += metrics.QueueLength
	}
	
	return float64(totalQueue) / float64(len(ts.prefixMetrics))
}

func (ts *TransferScheduler) calculateNetworkUtilization() float64 {
	if ts.networkProfile.EstimatedBandwidthMBps == 0 {
		return 0
	}
	
	return math.Min(ts.globalState.GlobalThroughputMBps / ts.networkProfile.EstimatedBandwidthMBps, 1.0)
}

// schedulingLoop runs the main scheduling coordination loop.
func (ts *TransferScheduler) schedulingLoop(ctx context.Context) {
	ticker := time.NewTicker(time.Second * 2)
	defer ticker.Stop()
	
	for {
		select {
		case <-ctx.Done():
			return
		case <-ticker.C:
			ts.performSchedulingOptimizations()
		}
	}
}

// metricsCollectionLoop runs periodic metrics collection.
func (ts *TransferScheduler) metricsCollectionLoop(ctx context.Context) {
	ticker := time.NewTicker(time.Second * 5)
	defer ticker.Stop()
	
	for {
		select {
		case <-ctx.Done():
			return
		case <-ticker.C:
			ts.collectAndAnalyzeMetrics()
		}
	}
}

// adaptiveOptimizationLoop runs adaptive optimization algorithms.
func (ts *TransferScheduler) adaptiveOptimizationLoop(ctx context.Context) {
	ticker := time.NewTicker(time.Second * 10)
	defer ticker.Stop()
	
	for {
		select {
		case <-ctx.Done():
			return
		case <-ticker.C:
			ts.performAdaptiveOptimizations()
		}
	}
}

func (ts *TransferScheduler) performSchedulingOptimizations() {
	ts.mu.Lock()
	defer ts.mu.Unlock()
	
	// Rebalance load if needed
	ts.loadBalancer.RebalanceIfNeeded(ts.prefixMetrics)
	
	// Adjust congestion windows based on performance
	ts.adjustCongestionWindows()
	
	// Update network profile confidence
	ts.updateLearningConfidence()
}

func (ts *TransferScheduler) collectAndAnalyzeMetrics() {
	ts.mu.Lock()
	defer ts.mu.Unlock()
	
	// Analyze performance patterns
	ts.analyzePerformancePatterns()
	
	// Detect anomalies
	ts.detectPerformanceAnomalies()
	
	// Update predictions
	ts.updatePerformancePredictions()
}

func (ts *TransferScheduler) performAdaptiveOptimizations() {
	ts.mu.Lock()
	defer ts.mu.Unlock()
	
	// Apply machine learning optimizations
	ts.applyMLOptimizations()
	
	// Optimize prefix allocation
	ts.optimizePrefixAllocation()
	
	// Update adaptive parameters
	ts.updateAdaptiveParameters()
}

func (ts *TransferScheduler) adjustCongestionWindows() {
	// Implement adaptive congestion window adjustment
	for prefixID, metrics := range ts.prefixMetrics {
		if metrics.ErrorRate > 0.05 { // 5% error rate threshold
			// Reduce congestion window
			metrics.CongestionWindow = int(float64(metrics.CongestionWindow) * 0.8)
			if metrics.CongestionWindow < 1 {
				metrics.CongestionWindow = 1
			}
		} else if metrics.ErrorRate < 0.01 && metrics.BandwidthUtilization < 0.8 {
			// Increase congestion window
			metrics.CongestionWindow = int(float64(metrics.CongestionWindow) * 1.1)
			if metrics.CongestionWindow > ts.config.GlobalCongestionWindow {
				metrics.CongestionWindow = ts.config.GlobalCongestionWindow
			}
		}
		
		ts.prefixMetrics[prefixID] = metrics
	}
}

func (ts *TransferScheduler) updateLearningConfidence() {
	// Update confidence based on prediction accuracy
	// This is a simplified implementation - in production, you'd track prediction accuracy
	if ts.networkProfile.LearningConfidence < 0.9 {
		ts.networkProfile.LearningConfidence += 0.005
	}
}

func (ts *TransferScheduler) analyzePerformancePatterns() {
	// Analyze patterns in performance data for predictive optimization
	// Implementation would include time series analysis, pattern recognition, etc.
}

func (ts *TransferScheduler) detectPerformanceAnomalies() {
	// Detect unusual performance patterns that might indicate issues
	// Implementation would include statistical anomaly detection
}

func (ts *TransferScheduler) updatePerformancePredictions() {
	// Update predictions for future performance based on historical data
	// Implementation would include predictive modeling
}

func (ts *TransferScheduler) applyMLOptimizations() {
	// Apply machine learning-based optimizations
	// Implementation would include neural networks, decision trees, etc.
}

func (ts *TransferScheduler) optimizePrefixAllocation() {
	// Optimize the allocation of uploads to prefixes
	// Implementation would include optimization algorithms
}

func (ts *TransferScheduler) updateAdaptiveParameters() {
	// Update parameters for adaptive algorithms based on performance
	// Implementation would include parameter tuning based on results
}

// RegisterPrefix registers a new prefix with the load balancer.
func (plb *PrefixLoadBalancer) RegisterPrefix(prefixID string, capacity float64) {
	plb.mu.Lock()
	defer plb.mu.Unlock()
	
	plb.prefixWeights[prefixID] = 1.0 // Start with equal weight
	plb.prefixCapacities[prefixID] = capacity
}

// RebalanceIfNeeded checks if rebalancing is needed and performs it.
func (plb *PrefixLoadBalancer) RebalanceIfNeeded(prefixMetrics map[string]*PrefixPerformanceMetrics) {
	plb.mu.Lock()
	defer plb.mu.Unlock()
	
	if time.Since(plb.lastRebalance) < plb.rebalanceInterval {
		return
	}
	
	// Check if rebalancing is needed
	if plb.shouldRebalance(prefixMetrics) {
		plb.performRebalance(prefixMetrics)
		plb.lastRebalance = time.Now()
	}
}

func (plb *PrefixLoadBalancer) shouldRebalance(prefixMetrics map[string]*PrefixPerformanceMetrics) bool {
	if len(prefixMetrics) < 2 {
		return false
	}
	
	var utilizationValues []float64
	for _, metrics := range prefixMetrics {
		utilizationValues = append(utilizationValues, metrics.BandwidthUtilization)
	}
	
	// Calculate coefficient of variation
	mean := 0.0
	for _, v := range utilizationValues {
		mean += v
	}
	mean /= float64(len(utilizationValues))
	
	variance := 0.0
	for _, v := range utilizationValues {
		diff := v - mean
		variance += diff * diff
	}
	variance /= float64(len(utilizationValues))
	
	cv := math.Sqrt(variance) / mean
	return cv > plb.rebalanceThreshold
}

func (plb *PrefixLoadBalancer) performRebalance(prefixMetrics map[string]*PrefixPerformanceMetrics) {
	// Adjust weights based on performance
	for prefixID, metrics := range prefixMetrics {
		if weight, exists := plb.prefixWeights[prefixID]; exists {
			// Increase weight for well-performing prefixes
			performanceScore := metrics.ThroughputMBps / (1.0 + metrics.ErrorRate + metrics.LatencyMs/100.0)
			newWeight := weight * (1.0 + performanceScore*0.1)
			
			// Normalize to prevent unbounded growth
			newWeight = math.Min(newWeight, 2.0)
			newWeight = math.Max(newWeight, 0.1)
			
			plb.prefixWeights[prefixID] = newWeight
		}
	}
}