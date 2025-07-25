package s3

import (
	"context"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestNewRealTimeLoadBalancer(t *testing.T) {
	rtlb := NewRealTimeLoadBalancer(LoadBalanceAdaptive)
	
	assert.NotNil(t, rtlb)
	assert.Equal(t, LoadBalanceAdaptive, rtlb.strategy)
	assert.NotNil(t, rtlb.prefixWeights)
	assert.NotNil(t, rtlb.prefixCapacities)
	assert.NotNil(t, rtlb.performanceHistory)
	assert.NotNil(t, rtlb.predictor)
	assert.NotNil(t, rtlb.optimizer)
	assert.Equal(t, 0.15, rtlb.rebalanceThreshold)
	assert.Equal(t, time.Second*10, rtlb.rebalanceInterval)
}

func TestRealTimeLoadBalancerRegisterPrefix(t *testing.T) {
	rtlb := NewRealTimeLoadBalancer(LoadBalanceAdaptive)
	
	// Register test prefix
	rtlb.RegisterPrefix("test-prefix", 100.0)
	
	// Verify prefix weight was created
	weight, exists := rtlb.prefixWeights["test-prefix"]
	require.True(t, exists)
	assert.Equal(t, "test-prefix", weight.PrefixID)
	assert.Equal(t, 1.0, weight.CurrentWeight)
	assert.Equal(t, 1.0, weight.BaseWeight)
	assert.True(t, weight.IsHealthy)
	assert.False(t, weight.IsOverloaded)
	
	// Verify capacity was set
	capacity, exists := rtlb.prefixCapacities["test-prefix"]
	require.True(t, exists)
	assert.Equal(t, 100.0, capacity)
	
	// Verify performance history was created
	history, exists := rtlb.performanceHistory["test-prefix"]
	require.True(t, exists)
	assert.Equal(t, "test-prefix", history.PrefixID)
	assert.Equal(t, 1000, history.MaxHistorySize)
	assert.NotNil(t, history.ThroughputHistory)
	assert.NotNil(t, history.LatencyHistory)
}

func TestRealTimeLoadBalancerUpdatePrefixMetrics(t *testing.T) {
	rtlb := NewRealTimeLoadBalancer(LoadBalanceAdaptive)
	rtlb.RegisterPrefix("test-prefix", 100.0)
	
	// Update with good performance metrics
	metrics := &PrefixPerformanceMetrics{
		PrefixID:             "test-prefix",
		ActiveUploads:        5,
		ThroughputMBps:       80.0,
		LatencyMs:            100.0,
		ErrorRate:            0.01,
		BandwidthUtilization: 0.6,
	}
	
	rtlb.UpdatePrefixMetrics("test-prefix", metrics)
	
	// Verify weight was updated
	weight := rtlb.prefixWeights["test-prefix"]
	assert.Greater(t, weight.ThroughputScore, 0.0)
	assert.Greater(t, weight.LatencyScore, 0.0)
	assert.Greater(t, weight.ReliabilityScore, 0.0)
	assert.Greater(t, weight.CapacityScore, 0.0)
	assert.True(t, weight.IsHealthy)
	assert.False(t, weight.IsOverloaded)
	
	// Verify performance history was updated
	history := rtlb.performanceHistory["test-prefix"]
	assert.Len(t, history.ThroughputHistory, 1)
	assert.Len(t, history.LatencyHistory, 1)
	assert.Len(t, history.ErrorRateHistory, 1)
	assert.Len(t, history.LoadHistory, 1)
	assert.Equal(t, 80.0, history.ThroughputHistory[0].Value)
	assert.Equal(t, 100.0, history.LatencyHistory[0].Value)
}

func TestRealTimeLoadBalancerUpdatePrefixMetricsOverloaded(t *testing.T) {
	rtlb := NewRealTimeLoadBalancer(LoadBalanceAdaptive)
	rtlb.RegisterPrefix("test-prefix", 100.0)
	
	// Update with overloaded metrics
	metrics := &PrefixPerformanceMetrics{
		PrefixID:             "test-prefix",
		ActiveUploads:        10,
		ThroughputMBps:       20.0,
		LatencyMs:            2500.0,
		ErrorRate:            0.15,
		BandwidthUtilization: 0.95,
	}
	
	rtlb.UpdatePrefixMetrics("test-prefix", metrics)
	
	// Verify overload detection
	weight := rtlb.prefixWeights["test-prefix"]
	assert.False(t, weight.IsHealthy)
	assert.True(t, weight.IsOverloaded)
	assert.Less(t, weight.LatencyScore, 0.5)
	assert.Less(t, weight.ReliabilityScore, 0.5)
	assert.Less(t, weight.CapacityScore, 0.2)
}

func TestRealTimeLoadBalancerSelectOptimalPrefixes(t *testing.T) {
	rtlb := NewRealTimeLoadBalancer(LoadBalanceAdaptive)
	
	// Register multiple prefixes with different performance
	prefixes := []struct {
		id         string
		capacity   float64
		throughput float64
		latency    float64
		errorRate  float64
		utilization float64
	}{
		{"high-perf", 100.0, 90.0, 50.0, 0.001, 0.5},
		{"med-perf", 100.0, 60.0, 100.0, 0.01, 0.7},
		{"low-perf", 100.0, 30.0, 200.0, 0.05, 0.9},
	}
	
	for _, prefix := range prefixes {
		rtlb.RegisterPrefix(prefix.id, prefix.capacity)
		
		metrics := &PrefixPerformanceMetrics{
			PrefixID:             prefix.id,
			ThroughputMBps:       prefix.throughput,
			LatencyMs:            prefix.latency,
			ErrorRate:            prefix.errorRate,
			BandwidthUtilization: prefix.utilization,
		}
		
		rtlb.UpdatePrefixMetrics(prefix.id, metrics)
	}
	
	// Create test upload
	upload := &ScheduledUpload{
		ArchivePath:   "/test/file.tar",
		Priority:      3,
		EstimatedSize: 1024 * 1024 * 100, // 100MB
	}
	
	// Select optimal prefixes
	selected, err := rtlb.SelectOptimalPrefixes(upload, 2)
	require.NoError(t, err)
	require.Len(t, selected, 2)
	
	// High performance prefix should be selected first
	assert.Equal(t, "high-perf", selected[0])
	assert.Equal(t, "med-perf", selected[1])
}

func TestRealTimeLoadBalancerSelectOptimalPrefixesHighPriority(t *testing.T) {
	rtlb := NewRealTimeLoadBalancer(LoadBalanceAdaptive)
	
	// Register prefixes with different reliability
	rtlb.RegisterPrefix("reliable", 100.0)
	rtlb.RegisterPrefix("unreliable", 100.0)
	
	// Update with metrics favoring different aspects
	reliableMetrics := &PrefixPerformanceMetrics{
		PrefixID:             "reliable",
		ThroughputMBps:       60.0,
		LatencyMs:            100.0,
		ErrorRate:            0.001, // Very reliable
		BandwidthUtilization: 0.6,
	}
	
	unreliableMetrics := &PrefixPerformanceMetrics{
		PrefixID:             "unreliable",
		ThroughputMBps:       90.0, // Higher throughput
		LatencyMs:            80.0,
		ErrorRate:            0.1, // Less reliable
		BandwidthUtilization: 0.5,
	}
	
	rtlb.UpdatePrefixMetrics("reliable", reliableMetrics)
	rtlb.UpdatePrefixMetrics("unreliable", unreliableMetrics)
	
	// High priority upload should prefer reliability
	highPriorityUpload := &ScheduledUpload{
		Priority: 5, // High priority
		EstimatedSize: 1024 * 1024,
	}
	
	selected, err := rtlb.SelectOptimalPrefixes(highPriorityUpload, 1)
	require.NoError(t, err)
	require.Len(t, selected, 1)
	
	// Should select reliable prefix despite lower throughput
	assert.Equal(t, "reliable", selected[0])
}

func TestRealTimeLoadBalancerOptimizeWeights(t *testing.T) {
	rtlb := NewRealTimeLoadBalancer(LoadBalanceAdaptive)
	
	// Register test prefixes
	rtlb.RegisterPrefix("prefix-1", 100.0)
	rtlb.RegisterPrefix("prefix-2", 100.0)
	
	// Add some performance history
	for _, prefixID := range []string{"prefix-1", "prefix-2"} {
		metrics := &PrefixPerformanceMetrics{
			PrefixID:             prefixID,
			ThroughputMBps:       50.0,
			LatencyMs:            100.0,
			ErrorRate:            0.01,
			BandwidthUtilization: 0.5,
		}
		rtlb.UpdatePrefixMetrics(prefixID, metrics)
	}
	
	// Optimize weights
	solution := rtlb.OptimizeWeights()
	
	assert.NotNil(t, solution)
	assert.Contains(t, solution.Weights, "prefix-1")
	assert.Contains(t, solution.Weights, "prefix-2")
	assert.Greater(t, solution.TotalScore, 0.0)
	assert.NotZero(t, solution.Timestamp)
}

func TestRealTimeLoadBalancerGetRealTimeMetrics(t *testing.T) {
	rtlb := NewRealTimeLoadBalancer(LoadBalanceAdaptive)
	
	// Register and update prefixes
	rtlb.RegisterPrefix("healthy", 100.0)
	rtlb.RegisterPrefix("overloaded", 100.0)
	
	healthyMetrics := &PrefixPerformanceMetrics{
		PrefixID:             "healthy",
		ThroughputMBps:       80.0,
		LatencyMs:            100.0,
		ErrorRate:            0.01,
		BandwidthUtilization: 0.6,
	}
	
	overloadedMetrics := &PrefixPerformanceMetrics{
		PrefixID:             "overloaded",
		ThroughputMBps:       20.0,
		LatencyMs:            1500.0,
		ErrorRate:            0.1,
		BandwidthUtilization: 0.95,
	}
	
	rtlb.UpdatePrefixMetrics("healthy", healthyMetrics)
	rtlb.UpdatePrefixMetrics("overloaded", overloadedMetrics)
	
	// Get real-time metrics
	metrics := rtlb.GetRealTimeMetrics()
	
	assert.NotNil(t, metrics)
	assert.Equal(t, 1, metrics.HealthyPrefixes)
	assert.Equal(t, 1, metrics.OverloadedPrefixes)
	assert.Greater(t, metrics.AverageThroughput, 0.0)
	assert.Greater(t, metrics.AverageLatency, 0.0)
	assert.NotZero(t, metrics.LastUpdate)
}

func TestRealTimeLoadBalancerRealTimeMonitoring(t *testing.T) {
	ctx, cancel := context.WithTimeout(context.Background(), time.Second*2)
	defer cancel()
	
	rtlb := NewRealTimeLoadBalancer(LoadBalanceAdaptive)
	rtlb.rebalanceInterval = time.Millisecond * 100 // Fast rebalancing for test
	
	// Start real-time monitoring
	rtlb.Start(ctx)
	
	// Register prefix and simulate load
	rtlb.RegisterPrefix("test-prefix", 100.0)
	
	metrics := &PrefixPerformanceMetrics{
		PrefixID:             "test-prefix",
		ThroughputMBps:       50.0,
		LatencyMs:            100.0,
		ErrorRate:            0.01,
		BandwidthUtilization: 0.5,
	}
	
	rtlb.UpdatePrefixMetrics("test-prefix", metrics)
	
	// Wait for monitoring loops to run
	time.Sleep(time.Millisecond * 200)
	
	// Verify system load metrics were updated
	assert.Greater(t, rtlb.systemLoad.TotalCapacity, 0.0)
	assert.NotZero(t, rtlb.systemLoad.LastUpdate)
}

func TestPerformanceHistoryManagement(t *testing.T) {
	rtlb := NewRealTimeLoadBalancer(LoadBalanceAdaptive)
	rtlb.RegisterPrefix("test-prefix", 100.0)
	
	// Set small history size for testing
	rtlb.performanceHistory["test-prefix"].MaxHistorySize = 3
	
	// Add multiple metrics to test history trimming
	for i := 0; i < 5; i++ {
		metrics := &PrefixPerformanceMetrics{
			PrefixID:             "test-prefix",
			ThroughputMBps:       float64(50 + i*10),
			LatencyMs:            100.0,
			ErrorRate:            0.01,
			BandwidthUtilization: 0.5,
		}
		
		rtlb.UpdatePrefixMetrics("test-prefix", metrics)
		time.Sleep(time.Millisecond) // Ensure different timestamps
	}
	
	// Verify history was trimmed to max size
	history := rtlb.performanceHistory["test-prefix"]
	assert.Len(t, history.ThroughputHistory, 3)
	assert.Len(t, history.LatencyHistory, 3)
	
	// Verify latest values are preserved
	assert.Equal(t, 90.0, history.ThroughputHistory[2].Value) // Last value
}

func TestWeightAdaptation(t *testing.T) {
	rtlb := NewRealTimeLoadBalancer(LoadBalanceAdaptive)
	rtlb.adaptationRate = 1.0 // Full adaptation for testing
	rtlb.RegisterPrefix("test-prefix", 100.0)
	
	initialWeight := rtlb.prefixWeights["test-prefix"].CurrentWeight
	
	// Update with poor performance to trigger adaptation
	poorMetrics := &PrefixPerformanceMetrics{
		PrefixID:             "test-prefix",
		ThroughputMBps:       10.0, // Poor throughput
		LatencyMs:            2000.0, // High latency
		ErrorRate:            0.2, // High error rate
		BandwidthUtilization: 0.5,
	}
	
	rtlb.UpdatePrefixMetrics("test-prefix", poorMetrics)
	
	// Weight should be reduced due to poor performance
	newWeight := rtlb.prefixWeights["test-prefix"].CurrentWeight
	assert.Less(t, newWeight, initialWeight)
	
	// Verify weight history was recorded
	weight := rtlb.prefixWeights["test-prefix"]
	assert.Len(t, weight.WeightHistory, 1)
	assert.Equal(t, newWeight, weight.WeightHistory[0].Weight)
}

func TestLoadImbalanceCalculation(t *testing.T) {
	rtlb := NewRealTimeLoadBalancer(LoadBalanceAdaptive)
	
	// Test with balanced load
	balancedDistribution := map[string]float64{
		"prefix-1": 50.0,
		"prefix-2": 50.0,
		"prefix-3": 50.0,
	}
	
	imbalance := rtlb.calculateLoadImbalance(balancedDistribution)
	assert.InDelta(t, 0.0, imbalance, 0.01) // Should be very low
	
	// Test with imbalanced load
	imbalancedDistribution := map[string]float64{
		"prefix-1": 10.0,
		"prefix-2": 50.0,
		"prefix-3": 90.0,
	}
	
	imbalance = rtlb.calculateLoadImbalance(imbalancedDistribution)
	assert.Greater(t, imbalance, 0.3) // Should be significant
}

func TestNewPerformancePredictor(t *testing.T) {
	predictor := NewPerformancePredictor()
	
	assert.NotNil(t, predictor)
	assert.NotNil(t, predictor.throughputModel)
	assert.NotNil(t, predictor.latencyModel)
	assert.NotNil(t, predictor.capacityModel)
	assert.NotNil(t, predictor.featureExtractor)
	assert.Equal(t, time.Hour, predictor.lookbackWindow)
	assert.Equal(t, time.Minute*10, predictor.predictionHorizon)
}

func TestPerformancePrediction(t *testing.T) {
	predictor := NewPerformancePredictor()
	
	// Create test performance history
	history := map[string]*PerformanceHistory{
		"test-prefix": {
			PrefixID: "test-prefix",
			ThroughputHistory: []TimeSeriesPoint{
				{Timestamp: time.Now().Add(-time.Minute), Value: 50.0},
				{Timestamp: time.Now().Add(-time.Second * 30), Value: 60.0},
				{Timestamp: time.Now(), Value: 70.0},
			},
			LatencyHistory: []TimeSeriesPoint{
				{Timestamp: time.Now().Add(-time.Minute), Value: 150.0},
				{Timestamp: time.Now().Add(-time.Second * 30), Value: 120.0},
				{Timestamp: time.Now(), Value: 100.0},
			},
			ErrorRateHistory: []TimeSeriesPoint{
				{Timestamp: time.Now().Add(-time.Minute), Value: 0.02},
				{Timestamp: time.Now().Add(-time.Second * 30), Value: 0.015},
				{Timestamp: time.Now(), Value: 0.01},
			},
		},
	}
	
	predictions := predictor.PredictPerformance(history, time.Minute*2)
	
	assert.Len(t, predictions, 1)
	prediction := predictions["test-prefix"]
	assert.NotNil(t, prediction)
	assert.Equal(t, "test-prefix", prediction.PrefixID)
	assert.Greater(t, prediction.ExpectedThroughput, 50.0)
	assert.Less(t, prediction.ExpectedLatency, 150.0)
	assert.Less(t, prediction.ExpectedErrorRate, 0.02)
	assert.Greater(t, prediction.ExpectedPerformance, 0.0)
	assert.Greater(t, prediction.Confidence, 0.0)
}

func TestNewLoadBalanceOptimizer(t *testing.T) {
	optimizer := NewLoadBalanceOptimizer()
	
	assert.NotNil(t, optimizer)
	assert.Equal(t, OptimizationGradientDescent, optimizer.strategy)
	assert.Equal(t, 0.4, optimizer.throughputObjective)
	assert.Equal(t, 0.3, optimizer.latencyObjective)
	assert.Equal(t, 0.2, optimizer.fairnessObjective)
	assert.Equal(t, 0.1, optimizer.stabilityObjective)
	assert.Equal(t, 0.5, optimizer.maxWeightChange)
	assert.NotNil(t, optimizer.currentSolution)
}

func TestLoadBalanceOptimization(t *testing.T) {
	optimizer := NewLoadBalanceOptimizer()
	
	// Test weights
	weights := map[string]float64{
		"prefix-1": 1.0,
		"prefix-2": 1.5,
		"prefix-3": 0.8,
	}
	
	// Test predictions
	predictions := map[string]*PerformancePrediction{
		"prefix-1": {ExpectedPerformance: 1.2, ExpectedThroughput: 60.0, ExpectedLatency: 100.0},
		"prefix-2": {ExpectedPerformance: 0.8, ExpectedThroughput: 40.0, ExpectedLatency: 150.0},
		"prefix-3": {ExpectedPerformance: 1.5, ExpectedThroughput: 80.0, ExpectedLatency: 80.0},
	}
	
	systemLoad := &SystemLoadMetrics{
		TotalThroughput:  180.0,
		TotalCapacity:    300.0,
		UtilizationRatio: 0.6,
	}
	
	solution := optimizer.Optimize(weights, predictions, systemLoad)
	
	assert.NotNil(t, solution)
	assert.Len(t, solution.Weights, 3)
	assert.Greater(t, solution.TotalScore, 0.0)
	
	// High-performing prefix should get higher weight
	assert.Greater(t, solution.Weights["prefix-3"], solution.Weights["prefix-2"])
}

func TestNewLoadBalanceAlertSystem(t *testing.T) {
	alertSystem := NewLoadBalanceAlertSystem()
	
	assert.NotNil(t, alertSystem)
	assert.NotNil(t, alertSystem.alerts)
	assert.NotNil(t, alertSystem.alertThresholds)
	assert.NotNil(t, alertSystem.alertHistory)
	assert.NotNil(t, alertSystem.notificationChannel)
	assert.Equal(t, 0.9, alertSystem.alertThresholds.OverloadThreshold)
}

func TestLoadBalanceAlertGeneration(t *testing.T) {
	alertSystem := NewLoadBalanceAlertSystem()
	
	// Test overload alert
	overloadedMetrics := &PrefixPerformanceMetrics{
		PrefixID:             "test-prefix",
		BandwidthUtilization: 0.95, // Above threshold
		LatencyMs:            100.0,
		ErrorRate:            0.01,
	}
	
	alertSystem.CheckAlerts("test-prefix", overloadedMetrics)
	
	// Check if alert was generated
	select {
	case alert := <-alertSystem.notificationChannel:
		assert.Equal(t, AlertTypeOverload, alert.Type)
		assert.Equal(t, "test-prefix", alert.PrefixID)
		assert.Equal(t, 0.95, alert.Value)
		assert.Equal(t, AlertSeverityHigh, alert.Severity)
	case <-time.After(time.Millisecond * 100):
		t.Fatal("Expected alert was not generated")
	}
}

func TestPrefixScoreCalculation(t *testing.T) {
	rtlb := NewRealTimeLoadBalancer(LoadBalanceAdaptive)
	
	// Set up test prefixes with different characteristics
	rtlb.RegisterPrefix("fast", 100.0)
	rtlb.RegisterPrefix("reliable", 100.0)
	rtlb.RegisterPrefix("overloaded", 100.0)
	
	// Update weights with different performance characteristics
	rtlb.prefixWeights["fast"].ThroughputScore = 2.0
	rtlb.prefixWeights["fast"].LatencyScore = 0.9
	rtlb.prefixWeights["fast"].ReliabilityScore = 0.8
	
	rtlb.prefixWeights["reliable"].ThroughputScore = 1.0
	rtlb.prefixWeights["reliable"].LatencyScore = 0.8
	rtlb.prefixWeights["reliable"].ReliabilityScore = 1.5
	
	rtlb.prefixWeights["overloaded"].IsOverloaded = true
	rtlb.prefixWeights["overloaded"].ThroughputScore = 1.5
	
	// Test normal priority upload
	normalUpload := &ScheduledUpload{
		Priority:      3,
		EstimatedSize: 1024 * 1024 * 10, // 10MB
	}
	
	predictions := map[string]*PerformancePrediction{
		"fast":       {ExpectedPerformance: 1.2},
		"reliable":   {ExpectedPerformance: 1.1},
		"overloaded": {ExpectedPerformance: 0.8},
	}
	
	scores := rtlb.calculatePrefixScores(normalUpload, predictions)
	
	// Fast prefix should score highest for normal uploads
	assert.Greater(t, scores["fast"], scores["reliable"])
	assert.Greater(t, scores["reliable"], scores["overloaded"])
	
	// Overloaded prefix should be penalized
	assert.Less(t, scores["overloaded"], 1.0)
}

func TestHighPriorityPrefixSelection(t *testing.T) {
	rtlb := NewRealTimeLoadBalancer(LoadBalanceAdaptive)
	
	rtlb.RegisterPrefix("fast", 100.0)
	rtlb.RegisterPrefix("reliable", 100.0)
	
	// Fast but less reliable
	rtlb.prefixWeights["fast"].ThroughputScore = 2.0
	rtlb.prefixWeights["fast"].ReliabilityScore = 0.5
	
	// Slower but more reliable
	rtlb.prefixWeights["reliable"].ThroughputScore = 1.0
	rtlb.prefixWeights["reliable"].ReliabilityScore = 1.5
	
	// High priority upload should prefer reliability
	highPriorityUpload := &ScheduledUpload{
		Priority:      5, // High priority
		EstimatedSize: 1024 * 1024,
	}
	
	predictions := map[string]*PerformancePrediction{
		"fast":     {ExpectedPerformance: 1.0},
		"reliable": {ExpectedPerformance: 1.0},
	}
	
	scores := rtlb.calculatePrefixScores(highPriorityUpload, predictions)
	
	// For high priority, reliable should score higher
	assert.Greater(t, scores["reliable"], scores["fast"])
}

func TestLargeUploadPrefixSelection(t *testing.T) {
	rtlb := NewRealTimeLoadBalancer(LoadBalanceAdaptive)
	
	rtlb.RegisterPrefix("fast", 100.0)
	rtlb.RegisterPrefix("stable", 100.0)
	
	// Fast throughput
	rtlb.prefixWeights["fast"].ThroughputScore = 2.0
	rtlb.prefixWeights["fast"].LatencyScore = 0.8
	
	// Stable but slower
	rtlb.prefixWeights["stable"].ThroughputScore = 1.0
	rtlb.prefixWeights["stable"].LatencyScore = 1.2
	
	// Large upload should prefer throughput
	largeUpload := &ScheduledUpload{
		Priority:      3,
		EstimatedSize: 1024 * 1024 * 1024 * 2, // 2GB
	}
	
	predictions := map[string]*PerformancePrediction{
		"fast":   {ExpectedPerformance: 1.0},
		"stable": {ExpectedPerformance: 1.0},
	}
	
	scores := rtlb.calculatePrefixScores(largeUpload, predictions)
	
	// For large uploads, fast should score higher
	assert.Greater(t, scores["fast"], scores["stable"])
}

func TestSystemLoadMetricsUpdate(t *testing.T) {
	rtlb := NewRealTimeLoadBalancer(LoadBalanceAdaptive)
	
	// Register prefixes with different capacities and utilizations
	rtlb.RegisterPrefix("prefix-1", 100.0)
	rtlb.RegisterPrefix("prefix-2", 150.0)
	
	// Set different capacity scores (inverse of utilization)
	rtlb.prefixWeights["prefix-1"].CapacityScore = 0.6 // 40% utilized
	rtlb.prefixWeights["prefix-2"].CapacityScore = 0.3 // 70% utilized
	
	rtlb.updateSystemLoadMetrics()
	
	// Verify calculations
	expectedTotalCapacity := 250.0
	expectedTotalThroughput := 100.0*0.4 + 150.0*0.7 // 40 + 105 = 145
	expectedUtilization := expectedTotalThroughput / expectedTotalCapacity
	
	assert.Equal(t, expectedTotalCapacity, rtlb.systemLoad.TotalCapacity)
	assert.InDelta(t, expectedTotalThroughput, rtlb.systemLoad.TotalThroughput, 0.1)
	assert.InDelta(t, expectedUtilization, rtlb.systemLoad.UtilizationRatio, 0.01)
	assert.Greater(t, rtlb.systemLoad.Imbalance, 0.0) // Should detect imbalance
	assert.NotZero(t, rtlb.systemLoad.LastUpdate)
}

func TestRealTimeAdaptationTriggers(t *testing.T) {
	rtlb := NewRealTimeLoadBalancer(LoadBalanceAdaptive)
	
	// Test high error rate trigger
	highErrorMetrics := &PrefixPerformanceMetrics{
		ErrorRate:            0.15, // Above 0.1 threshold
		LatencyMs:            100.0,
		BandwidthUtilization: 0.5,
	}
	assert.True(t, rtlb.shouldAdaptWeights(highErrorMetrics))
	
	// Test high latency trigger
	highLatencyMetrics := &PrefixPerformanceMetrics{
		ErrorRate:            0.01,
		LatencyMs:            1500.0, // Above 1000ms threshold
		BandwidthUtilization: 0.5,
	}
	assert.True(t, rtlb.shouldAdaptWeights(highLatencyMetrics))
	
	// Test overload trigger
	overloadMetrics := &PrefixPerformanceMetrics{
		ErrorRate:            0.01,
		LatencyMs:            100.0,
		BandwidthUtilization: 0.98, // Above 0.95 threshold
	}
	assert.True(t, rtlb.shouldAdaptWeights(overloadMetrics))
	
	// Test underload trigger
	underloadMetrics := &PrefixPerformanceMetrics{
		ErrorRate:            0.01,
		LatencyMs:            100.0,
		BandwidthUtilization: 0.05, // Below 0.1 threshold
	}
	assert.True(t, rtlb.shouldAdaptWeights(underloadMetrics))
	
	// Test normal metrics
	normalMetrics := &PrefixPerformanceMetrics{
		ErrorRate:            0.01,
		LatencyMs:            100.0,
		BandwidthUtilization: 0.5,
	}
	assert.False(t, rtlb.shouldAdaptWeights(normalMetrics))
}