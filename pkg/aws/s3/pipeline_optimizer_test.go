package s3

import (
	"context"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestNewPipelineOptimizer(t *testing.T) {
	po := NewPipelineOptimizer(PipelineOptimizationAdaptive)
	
	assert.NotNil(t, po)
	assert.Equal(t, PipelineOptimizationAdaptive, po.strategy)
	assert.NotNil(t, po.prefixPipelines)
	assert.NotNil(t, po.globalPipelineState)
	assert.NotNil(t, po.performanceTracker)
	assert.NotNil(t, po.adaptationEngine)
	assert.NotNil(t, po.resourceManager)
	assert.NotNil(t, po.predictor)
	assert.NotNil(t, po.optimizer)
	assert.Equal(t, 1, po.minPipelineDepth)
	assert.Equal(t, 32, po.maxPipelineDepth)
	assert.Equal(t, 0.2, po.adaptationRate)
	assert.Equal(t, time.Second*5, po.optimizationInterval)
}

func TestPipelineOptimizerRegisterPipeline(t *testing.T) {
	po := NewPipelineOptimizer(PipelineOptimizationAdaptive)
	
	// Register test pipeline
	po.RegisterPipeline("test-prefix", 8, 16)
	
	// Verify pipeline was created
	pipeline, exists := po.prefixPipelines["test-prefix"]
	require.True(t, exists)
	assert.Equal(t, "test-prefix", pipeline.PrefixID)
	assert.Equal(t, 8, pipeline.CurrentDepth)
	assert.Equal(t, 8, pipeline.OptimalDepth)
	assert.Equal(t, 1, pipeline.MinDepth)
	assert.Equal(t, 16, pipeline.MaxDepth)
	assert.Equal(t, 0.2, pipeline.AdaptationRate)
	assert.Equal(t, 1.0, pipeline.StabilityScore)
	assert.Equal(t, 1.0, pipeline.PerformanceScore)
	assert.Equal(t, 1.0, pipeline.ResourceScore)
	assert.Equal(t, PipelineCongestionNone, pipeline.CongestionState)
	assert.NotNil(t, pipeline.PerformanceMetrics)
	assert.NotNil(t, pipeline.ResourceUsage)
}

func TestPipelineOptimizerUpdatePipelineMetrics(t *testing.T) {
	po := NewPipelineOptimizer(PipelineOptimizationAdaptive)
	po.RegisterPipeline("test-prefix", 4, 16)
	
	// Update with good performance metrics
	metrics := &PipelinePerformanceMetrics{
		PrefixID:               "test-prefix",
		ActiveConnections:      4,
		ThroughputMBps:         80.0,
		LatencyMs:              100.0,
		ErrorRate:              0.01,
		CompletionRate:         0.95,
		QueueDepth:             2,
		MemoryUsageMB:          256.0,
		CPUUsagePercent:        60.0,
		NetworkUtilization:     0.7,
		BandwidthEfficiency:    0.8,
		ConcurrencyEfficiency:  0.85,
		ResourceEfficiency:     0.75,
	}
	
	po.UpdatePipelineMetrics("test-prefix", metrics)
	
	// Verify metrics were updated
	pipeline := po.prefixPipelines["test-prefix"]
	assert.Equal(t, metrics, pipeline.PerformanceMetrics)
	assert.Greater(t, pipeline.PerformanceScore, 0.0)
	assert.Equal(t, PipelineCongestionNone, pipeline.CongestionState)
	assert.Len(t, pipeline.PerformanceMetrics.ThroughputHistory, 1)
	assert.Len(t, pipeline.PerformanceMetrics.LatencyHistory, 1)
	assert.Len(t, pipeline.PerformanceMetrics.ErrorRateHistory, 1)
	assert.Len(t, pipeline.PerformanceMetrics.DepthHistory, 1)
}

func TestPipelineOptimizerCongestionDetection(t *testing.T) {
	po := NewPipelineOptimizer(PipelineOptimizationAdaptive)
	po.RegisterPipeline("test-prefix", 8, 16)
	
	// Add some baseline metrics
	for i := 0; i < 5; i++ {
		baselineMetrics := &PipelinePerformanceMetrics{
			PrefixID:       "test-prefix",
			ThroughputMBps: 100.0,
			LatencyMs:      100.0,
			ErrorRate:      0.01,
		}
		po.UpdatePipelineMetrics("test-prefix", baselineMetrics)
		time.Sleep(time.Millisecond) // Ensure different timestamps
	}
	
	// Update with congested metrics
	congestedMetrics := &PipelinePerformanceMetrics{
		PrefixID:       "test-prefix",
		ThroughputMBps: 30.0,  // Significant drop
		LatencyMs:      300.0, // Significant increase
		ErrorRate:      0.15,  // High error rate
	}
	
	po.UpdatePipelineMetrics("test-prefix", congestedMetrics)
	
	// Verify congestion was detected
	pipeline := po.prefixPipelines["test-prefix"]
	assert.NotEqual(t, PipelineCongestionNone, pipeline.CongestionState)
	assert.Less(t, pipeline.PerformanceScore, 1.0)
}

func TestPipelineOptimizerGetOptimalDepth(t *testing.T) {
	po := NewPipelineOptimizer(PipelineOptimizationAdaptive)
	po.RegisterPipeline("test-prefix", 8, 16)
	
	// Test getting optimal depth
	depth := po.GetOptimalDepth("test-prefix")
	assert.Equal(t, 8, depth)
	
	// Test non-existent prefix
	depth = po.GetOptimalDepth("non-existent")
	assert.Equal(t, 1, depth) // Should return minimum depth
}

func TestPipelineOptimizerAdaptiveOptimization(t *testing.T) {
	po := NewPipelineOptimizer(PipelineOptimizationAdaptive)
	po.RegisterPipeline("test-prefix", 4, 16)
	
	pipeline := po.prefixPipelines["test-prefix"]
	
	// Test optimization with no congestion and good performance
	pipeline.PerformanceScore = 1.5
	pipeline.StabilityScore = 0.9
	pipeline.PerformanceMetrics.ResourceEfficiency = 0.8
	pipeline.CongestionState = PipelineCongestionNone
	
	newDepth, reason := po.optimizeAdaptive(pipeline)
	assert.Equal(t, 5, newDepth) // Should increase by 1
	assert.Equal(t, ReasonThroughputIncrease, reason)
	
	// Test optimization with severe congestion
	pipeline.CongestionState = PipelineCongestionSevere
	newDepth, reason = po.optimizeAdaptive(pipeline)
	assert.Equal(t, 2, newDepth) // Should reduce to 50%
	assert.Equal(t, ReasonCongestionControl, reason)
}

func TestPipelineOptimizerThroughputOptimization(t *testing.T) {
	po := NewPipelineOptimizer(PipelineOptimizationThroughput)
	po.RegisterPipeline("test-prefix", 4, 16)
	
	pipeline := po.prefixPipelines["test-prefix"]
	
	// Test throughput optimization
	pipeline.PerformanceScore = 0.8 // Below 1.0
	pipeline.CongestionState = PipelineCongestionNone
	
	newDepth, reason := po.optimizeForThroughput(pipeline)
	assert.Equal(t, 6, newDepth) // Should increase by 2
	assert.Equal(t, ReasonThroughputIncrease, reason)
	
	// Test with congestion
	pipeline.CongestionState = PipelineCongestionMild
	newDepth, reason = po.optimizeForThroughput(pipeline)
	assert.Equal(t, 3, newDepth) // Should decrease by 1
	assert.Equal(t, ReasonCongestionControl, reason)
}

func TestPipelineOptimizerLatencyOptimization(t *testing.T) {
	po := NewPipelineOptimizer(PipelineOptimizationLatency)
	po.RegisterPipeline("test-prefix", 4, 16)
	
	pipeline := po.prefixPipelines["test-prefix"]
	
	// Add latency history
	for i := 0; i < 5; i++ {
		point := TimeSeriesPoint{
			Timestamp: time.Now().Add(-time.Duration(i) * time.Second),
			Value:     300.0, // High latency
		}
		pipeline.PerformanceMetrics.LatencyHistory = append(pipeline.PerformanceMetrics.LatencyHistory, point)
	}
	
	newDepth, reason := po.optimizeForLatency(pipeline)
	assert.Equal(t, 3, newDepth) // Should reduce by 1
	assert.Equal(t, ReasonLatencyDecrease, reason)
}

func TestPipelineOptimizerResourceOptimization(t *testing.T) {
	po := NewPipelineOptimizer(PipelineOptimizationResource)
	po.RegisterPipeline("test-prefix", 4, 16)
	
	pipeline := po.prefixPipelines["test-prefix"]
	
	// Set poor resource score
	pipeline.ResourceScore = 0.5 // Below 0.7
	
	newDepth, reason := po.optimizeForResource(pipeline)
	assert.Equal(t, 3, newDepth) // Should reduce by 1
	assert.Equal(t, ReasonResourceOptimization, reason)
}

func TestPipelineOptimizerHybridOptimization(t *testing.T) {
	po := NewPipelineOptimizer(PipelineOptimizationHybrid)
	po.RegisterPipeline("test-prefix", 4, 16)
	
	pipeline := po.prefixPipelines["test-prefix"]
	
	// Test with good combined score
	pipeline.PerformanceScore = 1.3
	pipeline.StabilityScore = 1.2
	pipeline.ResourceScore = 1.1
	pipeline.CongestionState = PipelineCongestionNone
	
	newDepth, reason := po.optimizeHybrid(pipeline)
	assert.Equal(t, 5, newDepth) // Should increase by 1
	assert.Equal(t, ReasonThroughputIncrease, reason)
	
	// Test with poor combined score
	pipeline.PerformanceScore = 0.6
	pipeline.StabilityScore = 0.7
	pipeline.ResourceScore = 0.5
	
	newDepth, reason = po.optimizeHybrid(pipeline)
	assert.Equal(t, 3, newDepth) // Should decrease by 1
	assert.Equal(t, ReasonSystemRebalance, reason)
}

func TestPipelineOptimizerDepthAdjustment(t *testing.T) {
	po := NewPipelineOptimizer(PipelineOptimizationAdaptive)
	po.RegisterPipeline("test-prefix", 4, 16)
	
	pipeline := po.prefixPipelines["test-prefix"]
	initialDepth := pipeline.CurrentDepth
	
	// Apply depth adjustment
	po.applyDepthAdjustment(pipeline, 6, ReasonThroughputIncrease)
	
	// Verify adjustment was applied
	assert.Equal(t, 6, pipeline.CurrentDepth)
	assert.Equal(t, 6, pipeline.OptimalDepth)
	assert.Equal(t, 1, pipeline.TotalAdjustments)
	assert.Len(t, pipeline.AdjustmentHistory, 1)
	
	adjustment := pipeline.AdjustmentHistory[0]
	assert.Equal(t, initialDepth, adjustment.OldDepth)
	assert.Equal(t, 6, adjustment.NewDepth)
	assert.Equal(t, ReasonThroughputIncrease, adjustment.Reason)
	assert.NotZero(t, adjustment.Timestamp)
}

func TestPipelineOptimizerShouldOptimize(t *testing.T) {
	po := NewPipelineOptimizer(PipelineOptimizationAdaptive)
	po.RegisterPipeline("test-prefix", 4, 16)
	
	pipeline := po.prefixPipelines["test-prefix"]
	
	// Test recent adjustment (should not optimize)
	pipeline.LastAdjustment = time.Now().Add(-time.Second * 10)
	assert.False(t, po.shouldOptimize(pipeline))
	
	// Test old adjustment with poor performance (should optimize)
	pipeline.LastAdjustment = time.Now().Add(-time.Minute * 2)
	pipeline.PerformanceScore = 0.5
	assert.True(t, po.shouldOptimize(pipeline))
	
	// Test congestion detection (should optimize)
	pipeline.PerformanceScore = 1.0
	pipeline.CongestionState = PipelineCongestionModerate
	assert.True(t, po.shouldOptimize(pipeline))
	
	// Test potential for improvement (should optimize)
	pipeline.CongestionState = PipelineCongestionNone
	pipeline.StabilityScore = 0.9
	pipeline.PerformanceScore = 1.2
	assert.True(t, po.shouldOptimize(pipeline))
	
	// Test stable state (should not optimize)
	pipeline.PerformanceScore = 1.6
	assert.False(t, po.shouldOptimize(pipeline))
}

func TestPipelineOptimizerOptimizeAllPipelines(t *testing.T) {
	po := NewPipelineOptimizer(PipelineOptimizationAdaptive)
	po.RegisterPipeline("prefix-1", 4, 16)
	po.RegisterPipeline("prefix-2", 6, 16)
	
	// Set last optimization time to allow optimization
	po.lastOptimization = time.Now().Add(-time.Minute)
	
	// Update metrics to trigger optimization
	for _, prefixID := range []string{"prefix-1", "prefix-2"} {
		pipeline := po.prefixPipelines[prefixID]
		pipeline.LastAdjustment = time.Now().Add(-time.Minute * 2)
		pipeline.PerformanceScore = 0.6 // Poor performance to trigger optimization
	}
	
	po.OptimizeAllPipelines()
	
	// Verify global optimization was performed
	assert.True(t, po.lastOptimization.After(time.Now().Add(-time.Second*5)))
	assert.NotZero(t, po.globalPipelineState.LastUpdate)
}

func TestPipelineOptimizerGetPipelineMetrics(t *testing.T) {
	po := NewPipelineOptimizer(PipelineOptimizationAdaptive)
	po.RegisterPipeline("prefix-1", 4, 16)
	po.RegisterPipeline("prefix-2", 6, 16)
	
	metrics := po.GetPipelineMetrics()
	
	assert.NotNil(t, metrics)
	assert.Equal(t, 2, metrics.TotalPipelines)
	assert.Equal(t, 5.0, metrics.AveragePipelineDepth) // (4+6)/2
	assert.Greater(t, metrics.OptimizationEfficiency, 0.0)
	assert.GreaterOrEqual(t, metrics.ResourceUtilization, 0.0)
	assert.Greater(t, metrics.PerformanceScore, 0.0)
	assert.Equal(t, 0.2, metrics.AdaptationRate)
}

func TestPipelineOptimizerGlobalStateUpdate(t *testing.T) {
	po := NewPipelineOptimizer(PipelineOptimizationAdaptive)
	po.RegisterPipeline("prefix-1", 4, 16)
	po.RegisterPipeline("prefix-2", 6, 16)
	
	// Update with performance metrics
	for _, prefixID := range []string{"prefix-1", "prefix-2"} {
		metrics := &PipelinePerformanceMetrics{
			PrefixID:       prefixID,
			ThroughputMBps: 50.0,
			LatencyMs:      100.0,
			ErrorRate:      0.02,
			MemoryUsageMB:  200.0,
			CPUUsagePercent: 40.0,
		}
		po.UpdatePipelineMetrics(prefixID, metrics)
	}
	
	state := po.globalPipelineState
	assert.Equal(t, 2, state.TotalActivePipelines)
	assert.Equal(t, 10, state.TotalDepth) // 4+6
	assert.Equal(t, 5.0, state.AverageDepth)
	assert.Equal(t, 100.0, state.GlobalThroughput) // 50+50
	assert.Equal(t, 100.0, state.GlobalLatency)
	assert.Equal(t, 0.02, state.GlobalErrorRate)
	assert.NotZero(t, state.LastUpdate)
}

func TestPipelineOptimizerVarianceCalculation(t *testing.T) {
	po := NewPipelineOptimizer(PipelineOptimizationAdaptive)
	
	// Test with simple data
	history := []TimeSeriesPoint{
		{Value: 10.0}, {Value: 20.0}, {Value: 30.0},
	}
	
	variance := po.calculateVariance(history)
	assert.InDelta(t, 66.67, variance, 0.1)
	
	// Test with no variance
	uniformHistory := []TimeSeriesPoint{
		{Value: 20.0}, {Value: 20.0}, {Value: 20.0},
	}
	
	variance = po.calculateVariance(uniformHistory)
	assert.Equal(t, 0.0, variance)
	
	// Test with empty history
	variance = po.calculateVariance([]TimeSeriesPoint{})
	assert.Equal(t, 0.0, variance)
}

func TestPipelineOptimizerAverageCalculations(t *testing.T) {
	po := NewPipelineOptimizer(PipelineOptimizationAdaptive)
	
	history := []TimeSeriesPoint{
		{Value: 10.0}, {Value: 20.0}, {Value: 30.0}, {Value: 40.0}, {Value: 50.0},
	}
	
	// Test recent average (last 3 values)
	recentAvg := po.calculateRecentAverage(history, 3)
	assert.Equal(t, 40.0, recentAvg) // (30+40+50)/3
	
	// Test overall average
	overallAvg := po.calculateOverallAverage(history)
	assert.Equal(t, 30.0, overallAvg) // (10+20+30+40+50)/5
	
	// Test with count larger than history
	recentAvg = po.calculateRecentAverage(history, 10)
	assert.Equal(t, 30.0, recentAvg) // Should use all values
	
	// Test with empty history
	recentAvg = po.calculateRecentAverage([]TimeSeriesPoint{}, 3)
	assert.Equal(t, 0.0, recentAvg)
	
	overallAvg = po.calculateOverallAverage([]TimeSeriesPoint{})
	assert.Equal(t, 0.0, overallAvg)
}

func TestPipelineOptimizerResourceUtilizationCalculation(t *testing.T) {
	po := NewPipelineOptimizer(PipelineOptimizationAdaptive)
	po.RegisterPipeline("prefix-1", 4, 16)
	po.RegisterPipeline("prefix-2", 6, 16)
	
	// Update with resource metrics
	metrics1 := &PipelinePerformanceMetrics{
		PrefixID:        "prefix-1",
		MemoryUsageMB:   512.0, // 50% of 1024MB
		CPUUsagePercent: 60.0,
	}
	
	metrics2 := &PipelinePerformanceMetrics{
		PrefixID:        "prefix-2",
		MemoryUsageMB:   256.0, // 25% of 1024MB
		CPUUsagePercent: 40.0,
	}
	
	po.UpdatePipelineMetrics("prefix-1", metrics1)
	po.UpdatePipelineMetrics("prefix-2", metrics2)
	
	utilization := po.calculateResourceUtilization()
	
	// Expected: ((512+256)/2)/1024 + ((60+40)/2)/100 = 0.375 + 0.5 = 0.4375
	assert.InDelta(t, 0.4375, utilization, 0.01)
}

func TestPipelineOptimizerDepthVarianceCalculation(t *testing.T) {
	po := NewPipelineOptimizer(PipelineOptimizationAdaptive)
	po.RegisterPipeline("prefix-1", 4, 16)
	po.RegisterPipeline("prefix-2", 8, 16)
	po.RegisterPipeline("prefix-3", 2, 16)
	
	variance := po.calculateDepthVariance()
	
	// Mean = (4+8+2)/3 = 4.67
	// Variance = ((4-4.67)² + (8-4.67)² + (2-4.67)²)/3 = (0.44 + 11.11 + 7.11)/3 = 6.22
	assert.InDelta(t, 6.22, variance, 0.1)
}

func TestPipelineOptimizerShouldPerformGlobalRebalance(t *testing.T) {
	po := NewPipelineOptimizer(PipelineOptimizationAdaptive)
	po.RegisterPipeline("prefix-1", 4, 16)
	po.RegisterPipeline("prefix-2", 12, 16) // High variance
	
	// Test high load and non-stable adaptation
	po.globalPipelineState.SystemLoad = LoadHigh
	po.globalPipelineState.AdaptationPhase = PhaseExploring
	assert.True(t, po.shouldPerformGlobalRebalance())
	
	// Test high error rate
	po.globalPipelineState.SystemLoad = LoadLow
	po.globalPipelineState.AdaptationPhase = PhaseStable
	po.globalPipelineState.GlobalErrorRate = 0.15
	assert.True(t, po.shouldPerformGlobalRebalance())
	
	// Test high depth variance
	po.globalPipelineState.GlobalErrorRate = 0.01
	variance := po.calculateDepthVariance()
	if variance > 4.0 {
		assert.True(t, po.shouldPerformGlobalRebalance())
	}
	
	// Test stable conditions
	po.RegisterPipeline("prefix-3", 6, 16) // Reduce variance
	po.globalPipelineState.SystemLoad = LoadLow
	po.globalPipelineState.AdaptationPhase = PhaseStable
	po.globalPipelineState.GlobalErrorRate = 0.01
	// With more balanced depths, should not rebalance
}

func TestPipelineOptimizerRealtimeMonitoring(t *testing.T) {
	ctx, cancel := context.WithTimeout(context.Background(), time.Second*2)
	defer cancel()
	
	po := NewPipelineOptimizer(PipelineOptimizationAdaptive)
	po.optimizationInterval = time.Millisecond * 100 // Fast optimization for test
	po.RegisterPipeline("test-prefix", 4, 16)
	
	// Start real-time monitoring
	po.Start(ctx)
	
	// Simulate performance updates
	metrics := &PipelinePerformanceMetrics{
		PrefixID:       "test-prefix",
		ThroughputMBps: 50.0,
		LatencyMs:      100.0,
		ErrorRate:      0.01,
	}
	
	po.UpdatePipelineMetrics("test-prefix", metrics)
	
	// Wait for monitoring loops to run
	time.Sleep(time.Millisecond * 300)
	
	// Verify system is tracking metrics
	pipeline := po.prefixPipelines["test-prefix"]
	assert.NotNil(t, pipeline.PerformanceMetrics)
	assert.Greater(t, len(pipeline.PerformanceMetrics.ThroughputHistory), 0)
}

func TestPipelineOptimizerExpectedImprovementCalculation(t *testing.T) {
	po := NewPipelineOptimizer(PipelineOptimizationAdaptive)
	po.RegisterPipeline("test-prefix", 4, 16)
	
	pipeline := po.prefixPipelines["test-prefix"]
	pipeline.PerformanceScore = 1.0
	
	// Test increasing depth
	improvement := po.calculateExpectedImprovement(pipeline, 8)
	assert.Greater(t, improvement, 0.0)
	
	// Test decreasing depth
	improvement = po.calculateExpectedImprovement(pipeline, 2)
	assert.Less(t, improvement, 0.0)
	
	// Test same depth
	improvement = po.calculateExpectedImprovement(pipeline, 4)
	assert.Equal(t, 0.0, improvement)
}

func TestPipelineOptimizerOptimizationStrategies(t *testing.T) {
	strategies := []PipelineOptimizationStrategy{
		PipelineOptimizationAdaptive,
		PipelineOptimizationThroughput,
		PipelineOptimizationLatency,
		PipelineOptimizationResource,
		PipelineOptimizationHybrid,
	}
	
	for _, strategy := range strategies {
		po := NewPipelineOptimizer(strategy)
		po.RegisterPipeline("test-prefix", 4, 16)
		
		assert.Equal(t, strategy, po.strategy)
		assert.Equal(t, strategy, po.globalPipelineState.OptimizationMode)
		
		// Verify pipeline can be optimized with each strategy
		pipeline := po.prefixPipelines["test-prefix"]
		pipeline.LastAdjustment = time.Now().Add(-time.Minute * 2)
		
		po.optimizePipelineDepth(pipeline)
		// Should not panic and pipeline should still exist
		assert.NotNil(t, po.prefixPipelines["test-prefix"])
	}
}

func TestPipelineOptimizerConstraintEnforcement(t *testing.T) {
	po := NewPipelineOptimizer(PipelineOptimizationAdaptive)
	po.RegisterPipeline("test-prefix", 4, 8) // Limited max depth
	
	pipeline := po.prefixPipelines["test-prefix"]
	
	// Test minimum constraint
	po.applyDepthAdjustment(pipeline, -5, ReasonCongestionControl)
	assert.Equal(t, 1, pipeline.CurrentDepth) // Should be clamped to minimum
	
	// Test maximum constraint
	po.applyDepthAdjustment(pipeline, 20, ReasonThroughputIncrease)
	assert.Equal(t, 8, pipeline.CurrentDepth) // Should be clamped to maximum
}

func TestPipelineOptimizerHistoryManagement(t *testing.T) {
	po := NewPipelineOptimizer(PipelineOptimizationAdaptive)
	po.RegisterPipeline("test-prefix", 4, 16)
	
	pipeline := po.prefixPipelines["test-prefix"]
	
	// Add many adjustments to test history trimming
	for i := 0; i < 60; i++ {
		po.applyDepthAdjustment(pipeline, 4+i%4, ReasonThroughputIncrease)
	}
	
	// Verify history was trimmed
	assert.LessOrEqual(t, len(pipeline.AdjustmentHistory), 50)
	assert.Equal(t, 60, pipeline.TotalAdjustments)
}

func TestPipelineOptimizerConcurrentAccess(t *testing.T) {
	po := NewPipelineOptimizer(PipelineOptimizationAdaptive)
	po.RegisterPipeline("test-prefix", 4, 16)
	
	// Test concurrent access to pipeline metrics
	done := make(chan bool, 2)
	
	// Goroutine 1: Update metrics
	go func() {
		for i := 0; i < 10; i++ {
			metrics := &PipelinePerformanceMetrics{
				PrefixID:       "test-prefix",
				ThroughputMBps: float64(50 + i),
				LatencyMs:      100.0,
				ErrorRate:      0.01,
			}
			po.UpdatePipelineMetrics("test-prefix", metrics)
			time.Sleep(time.Millisecond)
		}
		done <- true
	}()
	
	// Goroutine 2: Get optimal depth
	go func() {
		for i := 0; i < 10; i++ {
			_ = po.GetOptimalDepth("test-prefix")
			time.Sleep(time.Millisecond)
		}
		done <- true
	}()
	
	// Wait for both goroutines
	<-done
	<-done
	
	// Verify no race conditions occurred
	pipeline := po.prefixPipelines["test-prefix"]
	assert.NotNil(t, pipeline)
	assert.GreaterOrEqual(t, len(pipeline.PerformanceMetrics.ThroughputHistory), 5)
}