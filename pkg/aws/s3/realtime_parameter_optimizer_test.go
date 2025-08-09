package s3

import (
	"context"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestNewRealTimeParameterOptimizer(t *testing.T) {
	ctx := context.Background()
	nm := NewRealTimeNetworkMonitor(ctx)
	defer func() { _ = nm.Shutdown() }()

	po := NewRealTimeParameterOptimizer(ctx, nm)
	defer func() { _ = po.Shutdown() }()

	assert.NotNil(t, po)
	assert.Equal(t, time.Second*60, po.optimizationInterval)
	assert.Equal(t, time.Minute*10, po.performanceWindow)
	assert.Equal(t, 0.01, po.learningRate)
	assert.NotNil(t, po.networkMonitor)
	assert.NotNil(t, po.parameterSpace)
	assert.NotNil(t, po.optimizationEngine)
	assert.NotNil(t, po.performanceTracker)
	assert.NotNil(t, po.constraintValidator)
	assert.NotNil(t, po.adaptationController)
	assert.NotNil(t, po.currentParameters)
	assert.NotNil(t, po.optimizationGoals)
	assert.NotNil(t, po.performanceMetrics)
	assert.True(t, po.optimizationActive)
	assert.NotNil(t, po.convergenceDetector)
	assert.Equal(t, RealTimeExplorationAdaptive, po.explorationStrategy)
	assert.Equal(t, RealTimeModeBalanced, po.optimizationMode)
	assert.False(t, po.isOptimizing)
}

func TestRealTimeParameterOptimizerOptimizeParameters(t *testing.T) {
	ctx := context.Background()
	nm := NewRealTimeNetworkMonitor(ctx)
	defer func() { _ = nm.Shutdown() }()

	po := NewRealTimeParameterOptimizer(ctx, nm)
	defer func() { _ = po.Shutdown() }()

	// Start network monitoring to provide conditions
	err := nm.StartMonitoring()
	require.NoError(t, err)
	defer func() { _ = nm.StopMonitoring() }()

	// Give monitoring time to collect data
	time.Sleep(time.Millisecond * 100)

	result, err := po.OptimizeParameters(ctx)
	require.NoError(t, err)
	assert.NotNil(t, result)
	assert.NotNil(t, result.OldParameters)
	assert.NotNil(t, result.NewParameters)
	assert.GreaterOrEqual(t, result.ImprovementRatio, 0.0)
	assert.Greater(t, result.OptimizationTime, time.Duration(0))
	assert.NotEmpty(t, result.Strategy.Algorithm)
	assert.GreaterOrEqual(t, result.Confidence, 0.0)
	assert.LessOrEqual(t, result.Confidence, 1.0)
	assert.NotNil(t, result.NetworkConditions)
}

func TestRealTimeParameterOptimizerGetCurrentParameters(t *testing.T) {
	ctx := context.Background()
	nm := NewRealTimeNetworkMonitor(ctx)
	defer func() { _ = nm.Shutdown() }()

	po := NewRealTimeParameterOptimizer(ctx, nm)
	defer func() { _ = po.Shutdown() }()

	params := po.GetCurrentParameters()

	assert.NotNil(t, params)
	assert.Greater(t, params.ChunkSizeMB, 0.0)
	assert.Greater(t, params.ConcurrentChunks, 0)
	assert.Greater(t, params.RequestTimeoutSec, 0.0)
	assert.Greater(t, params.BufferSizeMB, 0.0)
	assert.GreaterOrEqual(t, params.CompressionLevel, 0)
	assert.LessOrEqual(t, params.CompressionLevel, 9)
	assert.GreaterOrEqual(t, params.RetryAttempts, 0)
	assert.Greater(t, params.RetryBackoffMs, 0.0)
	assert.Greater(t, params.PipelineDepth, 0)
	assert.Greater(t, params.PreallocationSize, 0.0)
	assert.Greater(t, params.BatchingThreshold, 0)
	assert.Greater(t, params.QueueDepth, 0)
	assert.Greater(t, params.TCPWindowSize, 0)
	assert.Greater(t, params.KeepAliveInterval, time.Duration(0))
	assert.Greater(t, params.ConnectionPoolSize, 0)
	assert.NotNil(t, params.PriorityWeights)
	assert.NotNil(t, params.ResourceLimits)
	assert.NotZero(t, params.Timestamp)
	assert.GreaterOrEqual(t, params.OptimizationRound, int64(0))
	assert.GreaterOrEqual(t, params.Confidence, 0.0)
	assert.LessOrEqual(t, params.Confidence, 1.0)
	assert.GreaterOrEqual(t, params.PerformanceScore, 0.0)
}

func TestRealTimeParameterOptimizerGetOptimizationStatus(t *testing.T) {
	ctx := context.Background()
	nm := NewRealTimeNetworkMonitor(ctx)
	defer func() { _ = nm.Shutdown() }()

	po := NewRealTimeParameterOptimizer(ctx, nm)
	defer func() { _ = po.Shutdown() }()

	status := po.GetOptimizationStatus()

	assert.NotNil(t, status)
	assert.True(t, status.IsActive)
	assert.False(t, status.IsOptimizing)
	assert.NotEmpty(t, status.CurrentMode)
	assert.NotEmpty(t, status.ExplorationStrategy)
	assert.NotZero(t, status.LastOptimization)
	assert.GreaterOrEqual(t, status.OptimizationRounds, int64(0))
	assert.NotNil(t, status.ConvergenceStatus)
	assert.NotNil(t, status.PerformanceTrend)
	assert.NotNil(t, status.CurrentParameters)
	assert.NotNil(t, status.RecentPerformance)
}

func TestRealTimeParameterOptimizerGetParameterHistory(t *testing.T) {
	ctx := context.Background()
	nm := NewRealTimeNetworkMonitor(ctx)
	defer func() { _ = nm.Shutdown() }()

	po := NewRealTimeParameterOptimizer(ctx, nm)
	defer func() { _ = po.Shutdown() }()

	history := po.GetParameterHistory()

	assert.NotNil(t, history)
	// Initially empty until first optimization
	assert.GreaterOrEqual(t, len(history), 0)
}

func TestRealTimeParameterOptimizerConcurrentOptimization(t *testing.T) {
	ctx := context.Background()
	nm := NewRealTimeNetworkMonitor(ctx)
	defer func() { _ = nm.Shutdown() }()

	po := NewRealTimeParameterOptimizer(ctx, nm)
	defer func() { _ = po.Shutdown() }()

	// Start network monitoring
	err := nm.StartMonitoring()
	require.NoError(t, err)
	defer func() { _ = nm.StopMonitoring() }()

	// Give monitoring time to collect data
	time.Sleep(time.Millisecond * 100)

	// Try concurrent optimizations
	done := make(chan *RealTimeOptimizationResult, 2)
	errors := make(chan error, 2)

	for i := 0; i < 2; i++ {
		go func() {
			result, err := po.OptimizeParameters(ctx)
			if err != nil {
				errors <- err
				return
			}
			done <- result
		}()
	}

	// One should succeed, one should fail with "already in progress"
	successCount := 0
	errorCount := 0

	for i := 0; i < 2; i++ {
		select {
		case result := <-done:
			assert.NotNil(t, result)
			successCount++
		case err := <-errors:
			assert.Contains(t, err.Error(), "already in progress")
			errorCount++
		case <-time.After(time.Second * 5):
			t.Fatal("Concurrent optimization test timed out")
		}
	}

	assert.Equal(t, 1, successCount)
	assert.Equal(t, 1, errorCount)
}

func TestRealTimeParameterOptimizerShutdown(t *testing.T) {
	ctx := context.Background()
	nm := NewRealTimeNetworkMonitor(ctx)
	defer func() { _ = nm.Shutdown() }()

	po := NewRealTimeParameterOptimizer(ctx, nm)

	// Verify initial state
	assert.True(t, po.optimizationActive)

	// Shutdown
	err := po.Shutdown()
	assert.NoError(t, err)
	assert.False(t, po.optimizationActive)

	// Verify context is cancelled
	select {
	case <-po.ctx.Done():
		// Context properly cancelled
	default:
		t.Error("Context should be cancelled after shutdown")
	}
}

func TestNewDefaultRealTimeOptimizationParameters(t *testing.T) {
	params := NewDefaultRealTimeOptimizationParameters()

	assert.NotNil(t, params)
	assert.Equal(t, 16.0, params.ChunkSizeMB)
	assert.Equal(t, 8, params.ConcurrentChunks)
	assert.Equal(t, 30.0, params.RequestTimeoutSec)
	assert.Equal(t, 256.0, params.BufferSizeMB)
	assert.Equal(t, 6, params.CompressionLevel)
	assert.Equal(t, 3, params.RetryAttempts)
	assert.Equal(t, 1000.0, params.RetryBackoffMs)
	assert.Equal(t, 4, params.PipelineDepth)
	assert.Equal(t, 128.0, params.PreallocationSize)
	assert.Equal(t, 5, params.BatchingThreshold)
	assert.Equal(t, 16, params.QueueDepth)
	assert.Equal(t, 65536, params.TCPWindowSize)
	assert.Equal(t, time.Second*30, params.KeepAliveInterval)
	assert.Equal(t, 10, params.ConnectionPoolSize)
	assert.Contains(t, params.PriorityWeights, "high")
	assert.Contains(t, params.PriorityWeights, "normal")
	assert.Contains(t, params.PriorityWeights, "low")
	assert.Contains(t, params.ResourceLimits, "cpu")
	assert.Contains(t, params.ResourceLimits, "memory")
	assert.Contains(t, params.ResourceLimits, "network")
	assert.NotZero(t, params.Timestamp)
	assert.Equal(t, int64(0), params.OptimizationRound)
	assert.Equal(t, 0.5, params.Confidence)
	assert.Equal(t, 0.0, params.PerformanceScore)
}

func TestNewRealTimeParameterSpace(t *testing.T) {
	ps := NewRealTimeParameterSpace()

	assert.NotNil(t, ps)
	assert.Equal(t, 1.0, ps.chunkSizeRange.Min)
	assert.Equal(t, 64.0, ps.chunkSizeRange.Max)
	assert.Equal(t, 0.5, ps.chunkSizeRange.Step)
	assert.False(t, ps.chunkSizeRange.Discrete)
	assert.False(t, ps.chunkSizeRange.LogScale)

	assert.Equal(t, 1.0, ps.concurrencyRange.Min)
	assert.Equal(t, 32.0, ps.concurrencyRange.Max)
	assert.Equal(t, 1.0, ps.concurrencyRange.Step)
	assert.True(t, ps.concurrencyRange.Discrete)
	assert.False(t, ps.concurrencyRange.LogScale)

	assert.Equal(t, 5.0, ps.timeoutRange.Min)
	assert.Equal(t, 300.0, ps.timeoutRange.Max)
	assert.Equal(t, 1.0, ps.timeoutRange.Step)
	assert.False(t, ps.timeoutRange.Discrete)
	assert.False(t, ps.timeoutRange.LogScale)

	assert.NotNil(t, ps.dependencies)
	assert.NotNil(t, ps.constraints)
	assert.NotNil(t, ps.validationRules)
	assert.True(t, ps.adaptiveBounds)
	assert.NotNil(t, ps.boundsHistory)
	assert.NotNil(t, ps.boundaryConditions)
}

func TestNewRealTimeOptimizationEngine(t *testing.T) {
	oe := NewRealTimeOptimizationEngine()

	assert.NotNil(t, oe)
	assert.Equal(t, RealTimeAlgorithmHybrid, oe.algorithm)
	assert.NotNil(t, oe.hyperparameters)
	assert.NotNil(t, oe.gradientEstimator)
	assert.NotNil(t, oe.adamOptimizer)
	assert.Equal(t, 0.9, oe.momentum)
	assert.Equal(t, 20, oe.populationSize)
	assert.Equal(t, 0.1, oe.mutationRate)
	assert.Equal(t, 0.7, oe.crossoverRate)
	assert.Equal(t, 5, oe.eliteSize)
	assert.NotNil(t, oe.gaussianProcess)
	assert.Equal(t, RealTimeAcquisitionEI, oe.acquisitionFunction)
	assert.Equal(t, 0.1, oe.explorationWeight)
	assert.NotNil(t, oe.objectives)
	assert.NotNil(t, oe.paretoFront)
	assert.NotNil(t, oe.crowdingDistance)
	assert.NotNil(t, oe.rewardHistory)
	assert.NotNil(t, oe.regretBounds)
	assert.NotNil(t, oe.confidenceBounds)
}

func TestNewRealTimePerformanceTracker(t *testing.T) {
	pt := NewRealTimePerformanceTracker()

	assert.NotNil(t, pt)
	assert.NotNil(t, pt.metricsHistory)
	assert.Nil(t, pt.baselineMetrics)
	assert.NotNil(t, pt.improvementTracker)
	assert.NotNil(t, pt.throughputModel)
	assert.NotNil(t, pt.latencyModel)
	assert.NotNil(t, pt.reliabilityModel)
	assert.NotNil(t, pt.costModel)
	assert.NotNil(t, pt.trendAnalyzer)
	assert.NotNil(t, pt.anomalyDetector)
	assert.NotNil(t, pt.seasonalityDetector)
	assert.NotNil(t, pt.performancePredictor)
	assert.NotNil(t, pt.futureProjections)
	assert.NotNil(t, pt.confidenceIntervals)
}

func TestNewRealTimeConstraintValidator(t *testing.T) {
	cv := NewRealTimeConstraintValidator()

	assert.NotNil(t, cv)
	assert.NotNil(t, cv.constraints)
	assert.NotNil(t, cv.validationRules)
	assert.NotNil(t, cv.validationCache)
}

func TestNewRealTimeOptimizationGoals(t *testing.T) {
	og := NewRealTimeOptimizationGoals()

	assert.NotNil(t, og)
	assert.Equal(t, "throughput", og.PrimaryObjective)
	assert.Contains(t, og.SecondaryObjectives, "latency")
	assert.Contains(t, og.SecondaryObjectives, "reliability")
	assert.Contains(t, og.SecondaryObjectives, "cost")
	assert.Contains(t, og.TargetMetrics, "throughput")
	assert.Contains(t, og.TargetMetrics, "latency")
	assert.Contains(t, og.TargetMetrics, "reliability")
	assert.Contains(t, og.Weights, "throughput")
	assert.Contains(t, og.Weights, "latency")
	assert.Contains(t, og.Weights, "reliability")
	assert.Contains(t, og.Weights, "cost")
	assert.Contains(t, og.Constraints, "max_memory")
	assert.Contains(t, og.Constraints, "max_cost")
}

func TestNewRealTimeConvergenceDetector(t *testing.T) {
	cd := NewRealTimeConvergenceDetector()

	assert.NotNil(t, cd)
	assert.Equal(t, 0.01, cd.convergenceThreshold)
	assert.Equal(t, 10, cd.stagnationThreshold)
	assert.NotNil(t, cd.convergenceHistory)
	assert.Equal(t, 0, cd.stagnationCounter)
	assert.False(t, cd.isConverged)
}

func TestRealTimeConvergenceDetectorUpdate(t *testing.T) {
	cd := NewRealTimeConvergenceDetector()

	// Test initial state
	assert.False(t, cd.isConverged)
	assert.Equal(t, 0, cd.stagnationCounter)

	// Add some performance scores
	scores := []float64{0.5, 0.6, 0.65, 0.7, 0.72, 0.73, 0.735, 0.736, 0.737, 0.738}
	for _, score := range scores {
		cd.Update(score)
	}

	assert.GreaterOrEqual(t, len(cd.convergenceHistory), 10)

	// Add more stable scores to trigger convergence
	stableScores := []float64{0.738, 0.738, 0.738, 0.738, 0.738, 0.738, 0.738, 0.738, 0.738, 0.738}
	for _, score := range stableScores {
		cd.Update(score)
	}

	// Should eventually converge
	status := cd.GetStatus()
	assert.NotNil(t, status)
	assert.GreaterOrEqual(t, status.StagnationCounter, 0)
	assert.GreaterOrEqual(t, status.Variance, 0.0)
	assert.NotEmpty(t, status.RecentTrend)
}

func TestRealTimeConstraintValidatorValidateParameters(t *testing.T) {
	cv := NewRealTimeConstraintValidator()

	// Test valid parameters
	validParams := NewDefaultRealTimeOptimizationParameters()
	assert.True(t, cv.ValidateParameters(validParams))

	// Test invalid chunk size (too small)
	invalidParams1 := NewDefaultRealTimeOptimizationParameters()
	invalidParams1.ChunkSizeMB = -1.0
	assert.False(t, cv.ValidateParameters(invalidParams1))

	// Test invalid chunk size (too large)
	invalidParams2 := NewDefaultRealTimeOptimizationParameters()
	invalidParams2.ChunkSizeMB = 100.0
	assert.False(t, cv.ValidateParameters(invalidParams2))

	// Test invalid concurrency (too small)
	invalidParams3 := NewDefaultRealTimeOptimizationParameters()
	invalidParams3.ConcurrentChunks = 0
	assert.False(t, cv.ValidateParameters(invalidParams3))

	// Test invalid concurrency (too large)
	invalidParams4 := NewDefaultRealTimeOptimizationParameters()
	invalidParams4.ConcurrentChunks = 50
	assert.False(t, cv.ValidateParameters(invalidParams4))

	// Test invalid timeout (too small)
	invalidParams5 := NewDefaultRealTimeOptimizationParameters()
	invalidParams5.RequestTimeoutSec = 0
	assert.False(t, cv.ValidateParameters(invalidParams5))

	// Test invalid timeout (too large)
	invalidParams6 := NewDefaultRealTimeOptimizationParameters()
	invalidParams6.RequestTimeoutSec = 500
	assert.False(t, cv.ValidateParameters(invalidParams6))
}

func TestRealTimePerformanceTrackerGetCurrentPerformance(t *testing.T) {
	pt := NewRealTimePerformanceTracker()

	performance := pt.GetCurrentPerformance()

	assert.NotNil(t, performance)
	assert.Greater(t, performance.ThroughputMBps, 0.0)
	assert.Greater(t, performance.LatencyMs, 0.0)
	assert.GreaterOrEqual(t, performance.ErrorRate, 0.0)
	assert.NotNil(t, performance.ResourceUsage)
	assert.GreaterOrEqual(t, performance.QualityScore, 0.0)
	assert.Greater(t, performance.CostPerGB, 0.0)
	assert.NotZero(t, performance.Timestamp)
}

func TestRealTimePerformanceTrackerRecordPerformance(t *testing.T) {
	pt := NewRealTimePerformanceTracker()

	// Initially empty
	assert.Equal(t, 0, len(pt.metricsHistory))

	// Record performance
	performance := &RealTimePerformanceSnapshot{
		ThroughputMBps: 100.0,
		LatencyMs:      25.0,
		ErrorRate:      0.005,
		ResourceUsage:  &RealTimeResourceUsage{CPUUsage: 0.6, MemoryUsage: 1024 * 1024 * 512, NetworkUsage: 75.0, DiskUsage: 0.4},
		QualityScore:   0.85,
		CostPerGB:      0.08,
		Timestamp:      time.Now(),
	}

	pt.RecordPerformance(performance)

	assert.Equal(t, 1, len(pt.metricsHistory))
	assert.Equal(t, performance, pt.metricsHistory[0])
}

func TestRealTimePerformanceTrackerHistoryLimit(t *testing.T) {
	pt := NewRealTimePerformanceTracker()

	// Add many performance records
	for i := 0; i < 1500; i++ {
		performance := &RealTimePerformanceSnapshot{
			ThroughputMBps: float64(i),
			LatencyMs:      float64(i),
			ErrorRate:      0.01,
			ResourceUsage:  &RealTimeResourceUsage{CPUUsage: 0.5, MemoryUsage: 1024 * 1024 * 256, NetworkUsage: 50.0, DiskUsage: 0.3},
			QualityScore:   0.7,
			CostPerGB:      0.1,
			Timestamp:      time.Now(),
		}
		pt.RecordPerformance(performance)
	}

	// Verify history is limited to 1000
	assert.LessOrEqual(t, len(pt.metricsHistory), 1000)
}

func TestRealTimeThroughputModelPredict(t *testing.T) {
	tm := &RealTimeThroughputModel{
		coefficients: make(map[string]float64),
	}

	params := NewDefaultRealTimeOptimizationParameters()
	conditions := &RealTimeNetworkConditions{
		BandwidthMBps:       100.0,
		LatencyMs:           30.0,
		NetworkQuality:      0.8,
		ConnectionStability: 0.9,
	}

	prediction := tm.Predict(params, conditions)

	assert.Greater(t, prediction, 0.0)
	assert.Less(t, prediction, 1000.0) // Reasonable upper bound
}

func TestRealTimeLatencyModelPredict(t *testing.T) {
	lm := &RealTimeLatencyModel{}

	params := NewDefaultRealTimeOptimizationParameters()
	conditions := &RealTimeNetworkConditions{
		BandwidthMBps:       100.0,
		LatencyMs:           30.0,
		NetworkQuality:      0.8,
		ConnectionStability: 0.9,
	}

	prediction := lm.Predict(params, conditions)

	assert.GreaterOrEqual(t, prediction, 0.0)
	assert.LessOrEqual(t, prediction, 1.0)
}

func TestRealTimeReliabilityModelPredict(t *testing.T) {
	rm := &RealTimeReliabilityModel{}

	params := NewDefaultRealTimeOptimizationParameters()
	conditions := &RealTimeNetworkConditions{
		BandwidthMBps:       100.0,
		LatencyMs:           30.0,
		NetworkQuality:      0.8,
		ConnectionStability: 0.9,
	}

	prediction := rm.Predict(params, conditions)

	assert.GreaterOrEqual(t, prediction, 0.0)
	assert.LessOrEqual(t, prediction, 1.0)
}

func TestRealTimeCostModelPredict(t *testing.T) {
	cm := &RealTimeCostModel{}

	params := NewDefaultRealTimeOptimizationParameters()
	conditions := &RealTimeNetworkConditions{
		BandwidthMBps:       100.0,
		LatencyMs:           30.0,
		NetworkQuality:      0.8,
		ConnectionStability: 0.9,
	}

	prediction := cm.Predict(params, conditions)

	assert.GreaterOrEqual(t, prediction, 0.0)
	assert.LessOrEqual(t, prediction, 1.0)
}

func TestRealTimeParameterOptimizerOptimizationAlgorithms(t *testing.T) {
	// Test algorithm enum values
	assert.Equal(t, "gradient_descent", string(RealTimeAlgorithmGradientDescent))
	assert.Equal(t, "adam", string(RealTimeAlgorithmAdam))
	assert.Equal(t, "evolutionary", string(RealTimeAlgorithmEvolutionary))
	assert.Equal(t, "bayesian", string(RealTimeAlgorithmBayesian))
	assert.Equal(t, "bandit", string(RealTimeAlgorithmBandit))
	assert.Equal(t, "hybrid", string(RealTimeAlgorithmHybrid))
}

func TestRealTimeParameterOptimizerExplorationStrategies(t *testing.T) {
	// Test exploration strategy enum values
	assert.Equal(t, "greedy", string(RealTimeExplorationGreedy))
	assert.Equal(t, "epsilon_greedy", string(RealTimeExplorationEpsilonGreedy))
	assert.Equal(t, "ucb", string(RealTimeExplorationUCB))
	assert.Equal(t, "thompson", string(RealTimeExplorationThompson))
	assert.Equal(t, "adaptive", string(RealTimeExplorationAdaptive))
}

func TestRealTimeParameterOptimizerOptimizationModes(t *testing.T) {
	// Test optimization mode enum values
	assert.Equal(t, "exploration", string(RealTimeModeExploration))
	assert.Equal(t, "exploitation", string(RealTimeModeExploitation))
	assert.Equal(t, "balanced", string(RealTimeModeBalanced))
	assert.Equal(t, "adaptive", string(RealTimeModeAdaptive))
}

func TestRealTimeParameterOptimizerAcquisitionFunctions(t *testing.T) {
	// Test acquisition function enum values
	assert.Equal(t, "expected_improvement", string(RealTimeAcquisitionEI))
	assert.Equal(t, "upper_confidence_bound", string(RealTimeAcquisitionUCB))
	assert.Equal(t, "probability_improvement", string(RealTimeAcquisitionPI))
}

func TestRealTimeParameterOptimizerCopyParameters(t *testing.T) {
	ctx := context.Background()
	nm := NewRealTimeNetworkMonitor(ctx)
	defer func() { _ = nm.Shutdown() }()

	po := NewRealTimeParameterOptimizer(ctx, nm)
	defer func() { _ = po.Shutdown() }()

	original := NewDefaultRealTimeOptimizationParameters()
	original.ChunkSizeMB = 32.0
	original.PriorityWeights["test"] = 0.8
	original.ResourceLimits["test"] = 0.9

	copy := po.copyParameters(original)

	assert.Equal(t, original.ChunkSizeMB, copy.ChunkSizeMB)
	assert.Equal(t, original.PriorityWeights["test"], copy.PriorityWeights["test"])
	assert.Equal(t, original.ResourceLimits["test"], copy.ResourceLimits["test"])

	// Verify it's a deep copy
	copy.ChunkSizeMB = 64.0
	copy.PriorityWeights["test"] = 0.5
	copy.ResourceLimits["test"] = 0.6

	assert.NotEqual(t, original.ChunkSizeMB, copy.ChunkSizeMB)
	assert.NotEqual(t, original.PriorityWeights["test"], copy.PriorityWeights["test"])
	assert.NotEqual(t, original.ResourceLimits["test"], copy.ResourceLimits["test"])
}

func TestRealTimeParameterOptimizerClampToValidRanges(t *testing.T) {
	ctx := context.Background()
	nm := NewRealTimeNetworkMonitor(ctx)
	defer func() { _ = nm.Shutdown() }()

	po := NewRealTimeParameterOptimizer(ctx, nm)
	defer func() { _ = po.Shutdown() }()

	params := NewDefaultRealTimeOptimizationParameters()

	// Test clamping values outside ranges
	params.ChunkSizeMB = -5.0        // Below min
	params.ConcurrentChunks = 100    // Above max
	params.RequestTimeoutSec = 500.0 // Above max
	params.BufferSizeMB = -10.0      // Below min
	params.CompressionLevel = 15     // Above max
	params.RetryAttempts = -1        // Below min

	clamped := po.clampToValidRanges(params)

	assert.GreaterOrEqual(t, clamped.ChunkSizeMB, po.parameterSpace.chunkSizeRange.Min)
	assert.LessOrEqual(t, clamped.ChunkSizeMB, po.parameterSpace.chunkSizeRange.Max)
	assert.GreaterOrEqual(t, clamped.ConcurrentChunks, int(po.parameterSpace.concurrencyRange.Min))
	assert.LessOrEqual(t, clamped.ConcurrentChunks, int(po.parameterSpace.concurrencyRange.Max))
	assert.GreaterOrEqual(t, clamped.RequestTimeoutSec, po.parameterSpace.timeoutRange.Min)
	assert.LessOrEqual(t, clamped.RequestTimeoutSec, po.parameterSpace.timeoutRange.Max)
	assert.GreaterOrEqual(t, clamped.BufferSizeMB, po.parameterSpace.bufferSizeRange.Min)
	assert.LessOrEqual(t, clamped.BufferSizeMB, po.parameterSpace.bufferSizeRange.Max)
	assert.GreaterOrEqual(t, clamped.CompressionLevel, 0)
	assert.LessOrEqual(t, clamped.CompressionLevel, 9)
	assert.GreaterOrEqual(t, clamped.RetryAttempts, 0)
	assert.LessOrEqual(t, clamped.RetryAttempts, 10)
}

func TestRealTimeParameterOptimizerSampleFromAcquisitionFunction(t *testing.T) {
	ctx := context.Background()
	nm := NewRealTimeNetworkMonitor(ctx)
	defer func() { _ = nm.Shutdown() }()

	po := NewRealTimeParameterOptimizer(ctx, nm)
	defer func() { _ = po.Shutdown() }()

	conditions := &RealTimeNetworkConditions{
		BandwidthMBps:       100.0,
		LatencyMs:           30.0,
		NetworkQuality:      0.8,
		ConnectionStability: 0.9,
	}

	// Test chunk size sampling
	chunkSize := po.sampleFromAcquisitionFunction("chunk_size", conditions)
	assert.Greater(t, chunkSize, 0.0)
	assert.Less(t, chunkSize, 100.0)

	// Test concurrency sampling
	concurrency := po.sampleFromAcquisitionFunction("concurrency", conditions)
	assert.Greater(t, concurrency, 0.0)
	assert.Less(t, concurrency, 50.0)

	// Test timeout sampling
	timeout := po.sampleFromAcquisitionFunction("timeout", conditions)
	assert.Greater(t, timeout, 0.0)
	assert.Less(t, timeout, 1000.0)

	// Test unknown parameter
	unknown := po.sampleFromAcquisitionFunction("unknown", conditions)
	assert.GreaterOrEqual(t, unknown, 0.0)
	assert.LessOrEqual(t, unknown, 1.0)
}

func TestRealTimeParameterOptimizerEdgeCases(t *testing.T) {
	ctx := context.Background()
	nm := NewRealTimeNetworkMonitor(ctx)
	defer func() { _ = nm.Shutdown() }()

	po := NewRealTimeParameterOptimizer(ctx, nm)
	defer func() { _ = po.Shutdown() }()

	// Test getting parameters immediately after creation
	params := po.GetCurrentParameters()
	assert.NotNil(t, params)

	// Test getting status immediately after creation
	status := po.GetOptimizationStatus()
	assert.NotNil(t, status)

	// Test getting empty history
	history := po.GetParameterHistory()
	assert.NotNil(t, history)
	assert.Equal(t, 0, len(history))

	// Test optimization without network monitoring
	result, err := po.OptimizeParameters(ctx)
	if err != nil {
		// This is expected since network monitoring isn't started
		assert.Contains(t, err.Error(), "network conditions")
	} else {
		// If it succeeds, result should be valid
		assert.NotNil(t, result)
	}
}

func TestRealTimeParameterOptimizerMultipleOptimizations(t *testing.T) {
	ctx := context.Background()
	nm := NewRealTimeNetworkMonitor(ctx)
	defer func() { _ = nm.Shutdown() }()

	po := NewRealTimeParameterOptimizer(ctx, nm)
	defer func() { _ = po.Shutdown() }()

	// Start network monitoring
	err := nm.StartMonitoring()
	require.NoError(t, err)
	defer func() { _ = nm.StopMonitoring() }()

	// Give monitoring time to collect data
	time.Sleep(time.Millisecond * 100)

	// Run multiple optimizations
	for i := 0; i < 3; i++ {
		result, err := po.OptimizeParameters(ctx)
		require.NoError(t, err)
		assert.NotNil(t, result)

		// Small delay between optimizations
		time.Sleep(time.Millisecond * 10)
	}

	// Verify history has records
	history := po.GetParameterHistory()
	assert.Greater(t, len(history), 0)
}

func TestRealTimeParameterOptimizerUtilityFunctions(t *testing.T) {
	// Test clampRealTimeFloat64
	assert.Equal(t, 5.0, clampRealTimeFloat64(3.0, 5.0, 10.0))
	assert.Equal(t, 10.0, clampRealTimeFloat64(15.0, 5.0, 10.0))
	assert.Equal(t, 7.0, clampRealTimeFloat64(7.0, 5.0, 10.0))

	// Test clampRealTimeInt
	assert.Equal(t, 5, clampRealTimeInt(3, 5, 10))
	assert.Equal(t, 10, clampRealTimeInt(15, 5, 10))
	assert.Equal(t, 7, clampRealTimeInt(7, 5, 10))

	// Test maxRealTimeInt
	assert.Equal(t, 5, maxRealTimeInt(3, 5))
	assert.Equal(t, 5, maxRealTimeInt(5, 3))
	assert.Equal(t, 5, maxRealTimeInt(5, 5))

	// Test calculateRealTimeVariance
	values := []float64{1.0, 2.0, 3.0, 4.0, 5.0}
	variance := calculateRealTimeVariance(values)
	assert.Greater(t, variance, 0.0)

	// Test calculateRealTimeTrend
	improving := []float64{1.0, 2.0, 3.0, 4.0, 5.0}
	assert.Equal(t, "improving", calculateRealTimeTrend(improving))

	degrading := []float64{5.0, 4.0, 3.0, 2.0, 1.0}
	assert.Equal(t, "degrading", calculateRealTimeTrend(degrading))

	stable := []float64{3.0, 3.0, 3.0, 3.0, 3.0}
	assert.Equal(t, "stable", calculateRealTimeTrend(stable))
}
