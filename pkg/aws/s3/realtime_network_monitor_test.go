package s3

import (
	"context"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestNewRealTimeNetworkMonitor(t *testing.T) {
	ctx := context.Background()
	nm := NewRealTimeNetworkMonitor(ctx)
	defer func() { _ = nm.Shutdown() }()

	assert.NotNil(t, nm)
	assert.Equal(t, time.Second*10, nm.monitoringInterval)
	assert.Equal(t, time.Minute*5, nm.samplingWindow)
	assert.Equal(t, 0.95, nm.stabilityThreshold)
	assert.Equal(t, 0.8, nm.qualityThreshold)
	assert.NotNil(t, nm.bandwidthTracker)
	assert.NotNil(t, nm.latencyTracker)
	assert.NotNil(t, nm.stabilityAnalyzer)
	assert.NotNil(t, nm.qualityAssessor)
	assert.NotNil(t, nm.pathDetector)
	assert.NotNil(t, nm.pathManager)
	assert.NotNil(t, nm.currentConditions)
	assert.NotNil(t, nm.historicalData)
	assert.NotNil(t, nm.trendAnalyzer)
	assert.NotNil(t, nm.alertSystem)
	assert.False(t, nm.isMonitoring)
	assert.Len(t, nm.monitoringWorkers, 5)
}

func TestRealTimeNetworkMonitorStartStopMonitoring(t *testing.T) {
	ctx := context.Background()
	nm := NewRealTimeNetworkMonitor(ctx)
	defer func() { _ = nm.Shutdown() }()

	// Test start monitoring
	err := nm.StartMonitoring()
	require.NoError(t, err)
	assert.True(t, nm.isMonitoring)

	// Test starting already active monitoring
	err = nm.StartMonitoring()
	assert.Error(t, err)
	assert.Contains(t, err.Error(), "already active")

	// Give monitoring some time to run
	time.Sleep(time.Millisecond * 100)

	// Test stop monitoring
	err = nm.StopMonitoring()
	require.NoError(t, err)
	assert.False(t, nm.isMonitoring)

	// Test stopping inactive monitoring
	err = nm.StopMonitoring()
	assert.Error(t, err)
	assert.Contains(t, err.Error(), "not active")
}

func TestRealTimeNetworkMonitorGetCurrentConditions(t *testing.T) {
	ctx := context.Background()
	nm := NewRealTimeNetworkMonitor(ctx)
	defer func() { _ = nm.Shutdown() }()

	conditions := nm.GetCurrentConditions()

	assert.NotNil(t, conditions)
	assert.NotZero(t, conditions.Timestamp)
	assert.Greater(t, conditions.BandwidthMBps, 0.0)
	assert.Greater(t, conditions.LatencyMs, 0.0)
	assert.GreaterOrEqual(t, conditions.JitterMs, 0.0)
	assert.GreaterOrEqual(t, conditions.PacketLossRate, 0.0)
	assert.GreaterOrEqual(t, conditions.ConnectionStability, 0.0)
	assert.LessOrEqual(t, conditions.ConnectionStability, 1.0)
	assert.GreaterOrEqual(t, conditions.NetworkQuality, 0.0)
	assert.LessOrEqual(t, conditions.NetworkQuality, 1.0)
	assert.GreaterOrEqual(t, conditions.PathCount, 0)
	assert.GreaterOrEqual(t, conditions.Confidence, 0.0)
	assert.LessOrEqual(t, conditions.Confidence, 1.0)
}

func TestRealTimeNetworkMonitorGetNetworkTrends(t *testing.T) {
	ctx := context.Background()
	nm := NewRealTimeNetworkMonitor(ctx)
	defer func() { _ = nm.Shutdown() }()

	trends := nm.GetNetworkTrends()

	assert.NotNil(t, trends)
	assert.NotEmpty(t, trends.BandwidthTrend)
	assert.NotEmpty(t, trends.LatencyTrend)
	assert.NotEmpty(t, trends.StabilityTrend)
	assert.NotEmpty(t, trends.QualityTrend)
	assert.GreaterOrEqual(t, trends.PredictedQuality, 0.0)
	assert.LessOrEqual(t, trends.PredictedQuality, 1.0)
	assert.GreaterOrEqual(t, trends.TrendConfidence, 0.0)
	assert.LessOrEqual(t, trends.TrendConfidence, 1.0)
	assert.NotZero(t, trends.LastUpdate)
}

func TestRealTimeNetworkMonitorGetPathInformation(t *testing.T) {
	ctx := context.Background()
	nm := NewRealTimeNetworkMonitor(ctx)
	defer func() { _ = nm.Shutdown() }()

	// Trigger path detection
	_, _ = nm.pathDetector.DetectPaths()

	pathInfo := nm.GetPathInformation()

	assert.NotNil(t, pathInfo)
	assert.NotNil(t, pathInfo.AvailablePaths)
	assert.NotNil(t, pathInfo.ActivePaths)
	assert.NotNil(t, pathInfo.PathMetrics)
	assert.NotNil(t, pathInfo.TrafficDistribution)
}

func TestRealTimeNetworkMonitorWithRealMonitoring(t *testing.T) {
	ctx := context.Background()
	nm := NewRealTimeNetworkMonitor(ctx)
	defer func() { _ = nm.Shutdown() }()

	// Start monitoring
	err := nm.StartMonitoring()
	require.NoError(t, err)

	// Let it run for monitoring to execute (may take up to several seconds)
	time.Sleep(time.Millisecond * 2100)

	// Check that conditions are being updated
	conditions := nm.GetCurrentConditions()
	assert.NotNil(t, conditions)
	assert.NotZero(t, conditions.Timestamp)

	// Check worker statistics
	nm.mu.RLock()
	for _, worker := range nm.monitoringWorkers {
		assert.GreaterOrEqual(t, worker.ExecutionCount, int64(0))
		assert.NotZero(t, worker.LastExecution)
	}
	nm.mu.RUnlock()

	// Stop monitoring
	err = nm.StopMonitoring()
	require.NoError(t, err)
}

func TestRealTimeBandwidthTracker(t *testing.T) {
	bt := NewRealTimeBandwidthTracker()

	assert.NotNil(t, bt)
	assert.Equal(t, time.Second*30, bt.probeInterval)
	assert.Equal(t, int64(1024*1024), bt.probeSize)
	assert.Equal(t, 3, bt.maxConcurrentProbes)
	assert.True(t, bt.adaptiveProbing)
	assert.NotNil(t, bt.bandwidthHistory)
	assert.NotNil(t, bt.activeProbes)
	assert.Equal(t, 0.8, bt.confidence)

	// Test bandwidth measurement
	measurement, err := bt.MeasureBandwidth()
	require.NoError(t, err)
	assert.NotNil(t, measurement)
	assert.NotZero(t, measurement.Timestamp)
	assert.Greater(t, measurement.MeasuredBandwidth, 0.0)
	assert.Equal(t, RealTimeDirectionUpload, measurement.Direction)
	assert.Equal(t, RealTimeMethodActiveProbing, measurement.MeasurementMethod)
	assert.Greater(t, measurement.Accuracy, 0.0)
	assert.Greater(t, measurement.Duration, time.Duration(0))

	// Verify history is updated
	assert.Len(t, bt.bandwidthHistory, 1)
	assert.Equal(t, measurement.MeasuredBandwidth, bt.currentBandwidth)
}

func TestRealTimeLatencyTracker(t *testing.T) {
	lt := NewRealTimeLatencyTracker()

	assert.NotNil(t, lt)
	assert.Equal(t, time.Second*10, lt.pingInterval)
	assert.Equal(t, time.Second*5, lt.timeoutThreshold)
	assert.Equal(t, 10.0, lt.jitterThreshold)
	assert.Equal(t, 10, lt.sampleSize)
	assert.NotNil(t, lt.latencyHistory)
	assert.NotNil(t, lt.targetEndpoints)
	assert.Len(t, lt.targetEndpoints, 2)
	assert.NotNil(t, lt.activePings)
	assert.NotNil(t, lt.rttEstimator)

	// Test latency measurement
	measurement, err := lt.MeasureLatency()
	require.NoError(t, err)
	assert.NotNil(t, measurement)
	assert.NotZero(t, measurement.Timestamp)
	assert.Greater(t, measurement.Latency, time.Duration(0))
	assert.NotEmpty(t, measurement.Target)
	assert.Equal(t, RealTimeMeasurementICMP, measurement.MeasurementType)
	assert.Greater(t, measurement.PacketSize, 0)
	assert.True(t, measurement.Success)

	// Verify history is updated
	assert.Len(t, lt.latencyHistory, 1)
	assert.Equal(t, measurement.Latency, lt.currentLatency)
}

func TestRealTimeConnectionStabilityAnalyzer(t *testing.T) {
	csa := NewRealTimeConnectionStabilityAnalyzer()

	assert.NotNil(t, csa)
	assert.Equal(t, time.Minute*10, csa.stabilityWindow)
	assert.NotNil(t, csa.connectionEvents)
	assert.NotNil(t, csa.disconnectionEvents)
	assert.NotNil(t, csa.qualityDegradations)
	assert.NotNil(t, csa.eventCorrelator)
	assert.NotNil(t, csa.anomalyDetector)
	assert.NotNil(t, csa.stabilityPredictor)
	assert.NotNil(t, csa.failurePrediction)
	assert.Equal(t, 0.95, csa.stabilityScore)

	// Test stability analysis
	stability := csa.AnalyzeStability()
	assert.GreaterOrEqual(t, stability, 0.0)
	assert.LessOrEqual(t, stability, 1.0)

	// Test with connection events
	csa.connectionEvents = append(csa.connectionEvents, RealTimeConnectionEvent{
		Timestamp: time.Now(),
		EventType: RealTimeEventDisconnect,
		Duration:  time.Second,
		Quality:   0.5,
	})

	stability = csa.AnalyzeStability()
	assert.GreaterOrEqual(t, stability, 0.0)
	assert.LessOrEqual(t, stability, 1.0)
}

func TestRealTimeNetworkQualityAssessor(t *testing.T) {
	nqa := NewRealTimeNetworkQualityAssessor()

	assert.NotNil(t, nqa)
	assert.NotNil(t, nqa.qualityScorer)
	assert.Equal(t, RealTimeWeightingAdaptive, nqa.weightingAlgorithm)
	assert.NotNil(t, nqa.qualityThresholds)
	assert.NotNil(t, nqa.qualityHistory)
	assert.NotNil(t, nqa.qualityTrends)
	assert.True(t, nqa.adaptiveWeighting)
	assert.True(t, nqa.contextAwareness)
	assert.NotNil(t, nqa.applicationProfile)

	// Test quality assessment
	conditions := &RealTimeNetworkConditions{
		BandwidthMBps:       100.0,
		LatencyMs:           50.0,
		ConnectionStability: 0.95,
	}

	assessment := nqa.AssessQuality(conditions)
	assert.NotNil(t, assessment)
	assert.NotZero(t, assessment.Timestamp)
	assert.GreaterOrEqual(t, assessment.OverallScore, 0.0)
	assert.LessOrEqual(t, assessment.OverallScore, 1.0)
	assert.NotNil(t, assessment.ComponentScores)
	assert.Contains(t, assessment.ComponentScores, "bandwidth")
	assert.Contains(t, assessment.ComponentScores, "latency")
	assert.Contains(t, assessment.ComponentScores, "stability")
	assert.NotEmpty(t, assessment.QualityLevel)
	assert.GreaterOrEqual(t, assessment.Confidence, 0.0)
	assert.LessOrEqual(t, assessment.Confidence, 1.0)

	// Verify history is updated
	assert.Len(t, nqa.qualityHistory, 1)
}

func TestQualityLevelDetermination(t *testing.T) {
	nqa := NewRealTimeNetworkQualityAssessor()

	testCases := []struct {
		bandwidth       float64
		latency         float64
		stability       float64
		expectedQuality RealTimeQualityLevel
	}{
		{100.0, 10.0, 0.95, RealTimeQualityExcellent},
		{80.0, 50.0, 0.90, RealTimeQualityGood},
		{50.0, 100.0, 0.80, RealTimeQualityFair},
		{20.0, 500.0, 0.60, RealTimeQualityPoor},
	}

	for _, tc := range testCases {
		conditions := &RealTimeNetworkConditions{
			BandwidthMBps:       tc.bandwidth,
			LatencyMs:           tc.latency,
			ConnectionStability: tc.stability,
		}

		assessment := nqa.AssessQuality(conditions)
		assert.Equal(t, tc.expectedQuality, assessment.QualityLevel)
	}
}

func TestRealTimeMultiPathDetector(t *testing.T) {
	mpd := NewRealTimeMultiPathDetector()

	assert.NotNil(t, mpd)
	assert.Equal(t, RealTimeDetectionProbing, mpd.detectionAlgorithm)
	assert.True(t, mpd.activeDetection)
	assert.True(t, mpd.passiveDetection)
	assert.NotNil(t, mpd.availablePaths)
	assert.NotNil(t, mpd.activePaths)
	assert.NotNil(t, mpd.pathMetrics)
	assert.NotNil(t, mpd.pathQuality)
	assert.NotNil(t, mpd.pathStability)
	assert.NotNil(t, mpd.loadBalancer)
	assert.NotNil(t, mpd.trafficDistribution)

	// Test path detection
	paths, err := mpd.DetectPaths()
	require.NoError(t, err)
	assert.Greater(t, len(paths), 0)

	// Verify default path is created
	assert.NotNil(t, mpd.primaryPath)
	assert.Equal(t, "default", mpd.primaryPath.ID)
	assert.True(t, mpd.primaryPath.IsActive)
	assert.NotNil(t, mpd.primaryPath.Metrics)
	assert.Greater(t, mpd.primaryPath.Metrics.Bandwidth, 0.0)
	assert.Greater(t, mpd.primaryPath.Metrics.Latency, time.Duration(0))
	assert.GreaterOrEqual(t, mpd.primaryPath.Metrics.PacketLoss, 0.0)
	assert.GreaterOrEqual(t, mpd.primaryPath.Metrics.Reliability, 0.0)
	assert.LessOrEqual(t, mpd.primaryPath.Metrics.Reliability, 1.0)
	assert.GreaterOrEqual(t, mpd.primaryPath.Metrics.QualityScore, 0.0)
	assert.LessOrEqual(t, mpd.primaryPath.Metrics.QualityScore, 1.0)

	// Verify traffic distribution
	assert.Contains(t, mpd.trafficDistribution, "default")
	assert.Equal(t, 1.0, mpd.trafficDistribution["default"])
}

func TestRealTimeNetworkConditionsTypes(t *testing.T) {
	// Test WorkerType enum
	assert.Equal(t, "bandwidth_monitor", string(RealTimeWorkerBandwidthMonitor))
	assert.Equal(t, "latency_monitor", string(RealTimeWorkerLatencyMonitor))
	assert.Equal(t, "stability_monitor", string(RealTimeWorkerStabilityMonitor))
	assert.Equal(t, "quality_assessor", string(RealTimeWorkerQualityAssessor))
	assert.Equal(t, "path_detector", string(RealTimeWorkerPathDetector))

	// Test TrafficDirection enum
	assert.Equal(t, "upload", string(RealTimeDirectionUpload))
	assert.Equal(t, "download", string(RealTimeDirectionDownload))
	assert.Equal(t, "both", string(RealTimeDirectionBoth))

	// Test QualityLevel enum
	assert.Equal(t, "excellent", string(RealTimeQualityExcellent))
	assert.Equal(t, "good", string(RealTimeQualityGood))
	assert.Equal(t, "fair", string(RealTimeQualityFair))
	assert.Equal(t, "poor", string(RealTimeQualityPoor))

	// Test EventType enum
	assert.Equal(t, "connect", string(RealTimeEventConnect))
	assert.Equal(t, "disconnect", string(RealTimeEventDisconnect))
	assert.Equal(t, "degrade", string(RealTimeEventDegrade))
	assert.Equal(t, "recover", string(RealTimeEventRecover))
}

func TestRealTimeNetworkMonitoringWorkers(t *testing.T) {
	ctx := context.Background()
	nm := NewRealTimeNetworkMonitor(ctx)
	defer func() { _ = nm.Shutdown() }()

	// Verify workers are initialized
	assert.Len(t, nm.monitoringWorkers, 5)

	workerTypes := make(map[RealTimeWorkerType]bool)
	for _, worker := range nm.monitoringWorkers {
		assert.NotEmpty(t, worker.ID)
		assert.NotEmpty(t, worker.Type)
		assert.False(t, worker.IsActive)
		assert.Equal(t, int64(0), worker.ExecutionCount)
		assert.Equal(t, int64(0), worker.ErrorCount)

		workerTypes[worker.Type] = true
	}

	// Verify all expected worker types are present
	assert.True(t, workerTypes[RealTimeWorkerBandwidthMonitor])
	assert.True(t, workerTypes[RealTimeWorkerLatencyMonitor])
	assert.True(t, workerTypes[RealTimeWorkerStabilityMonitor])
	assert.True(t, workerTypes[RealTimeWorkerQualityAssessor])
	assert.True(t, workerTypes[RealTimeWorkerPathDetector])
}

func TestRealTimeNetworkMonitorConfidenceCalculation(t *testing.T) {
	ctx := context.Background()
	nm := NewRealTimeNetworkMonitor(ctx)
	defer func() { _ = nm.Shutdown() }()

	// Test confidence calculation
	confidence := nm.calculateConfidence()
	assert.GreaterOrEqual(t, confidence, 0.0)
	assert.LessOrEqual(t, confidence, 1.0)
}

func TestRealTimeNetworkMonitorExecuteWorkerTasks(t *testing.T) {
	ctx := context.Background()
	nm := NewRealTimeNetworkMonitor(ctx)
	defer func() { _ = nm.Shutdown() }()

	for i, worker := range nm.monitoringWorkers {
		err := nm.executeWorkerTask(&nm.monitoringWorkers[i])
		assert.NoError(t, err)
		assert.False(t, worker.IsActive) // Should be false after execution
	}
}

func TestRealTimeNetworkMonitorShutdown(t *testing.T) {
	ctx := context.Background()
	nm := NewRealTimeNetworkMonitor(ctx)

	// Start monitoring
	err := nm.StartMonitoring()
	require.NoError(t, err)
	assert.True(t, nm.isMonitoring)

	// Test shutdown
	err = nm.Shutdown()
	assert.NoError(t, err)
	assert.False(t, nm.isMonitoring)

	// Verify context is cancelled
	select {
	case <-nm.ctx.Done():
		// Context properly cancelled
	default:
		t.Error("Context should be cancelled after shutdown")
	}
}

func TestRealTimeUtilityFunctions(t *testing.T) {
	// Test math_max
	assert.Equal(t, 5.0, math_max(3.0, 5.0))
	assert.Equal(t, 5.0, math_max(5.0, 3.0))
	assert.Equal(t, 5.0, math_max(5.0, 5.0))

	// Test math_min
	assert.Equal(t, 3.0, math_min(3.0, 5.0))
	assert.Equal(t, 3.0, math_min(5.0, 3.0))
	assert.Equal(t, 3.0, math_min(3.0, 3.0))
}

func TestRealTimeNetworkEndpoint(t *testing.T) {
	endpoint := RealTimeNetworkEndpoint{
		Address:      "8.8.8.8",
		Port:         53,
		Protocol:     "icmp",
		Priority:     1,
		ResponseTime: time.Millisecond * 50,
		Availability: 0.99,
	}

	assert.Equal(t, "8.8.8.8", endpoint.Address)
	assert.Equal(t, 53, endpoint.Port)
	assert.Equal(t, "icmp", endpoint.Protocol)
	assert.Equal(t, 1, endpoint.Priority)
	assert.Equal(t, time.Millisecond*50, endpoint.ResponseTime)
	assert.Equal(t, 0.99, endpoint.Availability)
}

func TestRealTimeApplicationQualityProfile(t *testing.T) {
	profile := NewRealTimeApplicationQualityProfile("test")

	assert.Equal(t, "test", profile.Name)
	assert.Equal(t, 0.4, profile.BandwidthWeight)
	assert.Equal(t, 0.3, profile.LatencyWeight)
	assert.Equal(t, 0.2, profile.StabilityWeight)
	assert.Equal(t, 0.1, profile.ReliabilityWeight)
	assert.NotNil(t, profile.QualityRequirements)
}

func TestRealTimeNetworkMonitorConcurrency(t *testing.T) {
	ctx := context.Background()
	nm := NewRealTimeNetworkMonitor(ctx)
	defer func() { _ = nm.Shutdown() }()

	// Start monitoring
	err := nm.StartMonitoring()
	require.NoError(t, err)

	// Test concurrent access to current conditions
	done := make(chan bool, 10)

	for i := 0; i < 10; i++ {
		go func() {
			defer func() { done <- true }()

			for j := 0; j < 5; j++ {
				conditions := nm.GetCurrentConditions()
				assert.NotNil(t, conditions)

				trends := nm.GetNetworkTrends()
				assert.NotNil(t, trends)

				pathInfo := nm.GetPathInformation()
				assert.NotNil(t, pathInfo)

				time.Sleep(time.Millisecond * 10)
			}
		}()
	}

	// Wait for all goroutines to complete
	for i := 0; i < 10; i++ {
		select {
		case <-done:
			// Success
		case <-time.After(time.Second * 5):
			t.Fatal("Concurrent access test timed out")
		}
	}

	// Stop monitoring
	err = nm.StopMonitoring()
	require.NoError(t, err)
}

func TestRealTimeNetworkMonitorEdgeCases(t *testing.T) {
	ctx := context.Background()
	nm := NewRealTimeNetworkMonitor(ctx)
	defer func() { _ = nm.Shutdown() }()

	// Test getting conditions before starting monitoring
	conditions := nm.GetCurrentConditions()
	assert.NotNil(t, conditions)
	assert.NotZero(t, conditions.Timestamp)

	// Test getting trends before any monitoring
	trends := nm.GetNetworkTrends()
	assert.NotNil(t, trends)

	// Test path detection with no prior setup
	pathInfo := nm.GetPathInformation()
	assert.NotNil(t, pathInfo)
}

func TestRealTimeBandwidthTrackerHistoryLimits(t *testing.T) {
	bt := NewRealTimeBandwidthTracker()

	// Add many measurements to test history limit
	for i := 0; i < 1500; i++ {
		_, err := bt.MeasureBandwidth()
		require.NoError(t, err)
	}

	// Verify history is limited to 1000
	assert.LessOrEqual(t, len(bt.bandwidthHistory), 1000)
}

func TestRealTimeLatencyTrackerHistoryLimits(t *testing.T) {
	lt := NewRealTimeLatencyTracker()

	// Add many measurements to test history limit
	for i := 0; i < 1500; i++ {
		_, err := lt.MeasureLatency()
		require.NoError(t, err)
	}

	// Verify history is limited to 1000
	assert.LessOrEqual(t, len(lt.latencyHistory), 1000)
}

func TestRealTimeNetworkQualityAssessorHistoryLimits(t *testing.T) {
	nqa := NewRealTimeNetworkQualityAssessor()

	conditions := &RealTimeNetworkConditions{
		BandwidthMBps:       100.0,
		LatencyMs:           50.0,
		ConnectionStability: 0.95,
	}

	// Add many assessments to test history limit
	for i := 0; i < 1500; i++ {
		_ = nqa.AssessQuality(conditions)
	}

	// Verify history is limited to 1000
	assert.LessOrEqual(t, len(nqa.qualityHistory), 1000)
}

func TestRealTimeMeasurementMethods(t *testing.T) {
	// Test measurement method enum values
	assert.Equal(t, "active_probing", string(RealTimeMethodActiveProbing))
	assert.Equal(t, "passive_monitor", string(RealTimeMethodPassiveMonitor))
	assert.Equal(t, "estimation", string(RealTimeMethodEstimation))
}

func TestRealTimeMeasurementTypes(t *testing.T) {
	// Test measurement type enum values
	assert.Equal(t, "icmp", string(RealTimeMeasurementICMP))
	assert.Equal(t, "tcp", string(RealTimeMeasurementTCP))
	assert.Equal(t, "https", string(RealTimeMeasurementHTTPS))
}

func TestRealTimeWeightingAlgorithms(t *testing.T) {
	// Test weighting algorithm enum values
	assert.Equal(t, "fixed", string(RealTimeWeightingFixed))
	assert.Equal(t, "adaptive", string(RealTimeWeightingAdaptive))
	assert.Equal(t, "learning", string(RealTimeWeightingLearning))
}

func TestRealTimeDetectionAlgorithms(t *testing.T) {
	// Test detection algorithm enum values
	assert.Equal(t, "probing", string(RealTimeDetectionProbing))
	assert.Equal(t, "passive", string(RealTimeDetectionPassive))
	assert.Equal(t, "hybrid", string(RealTimeDetectionHybrid))
}

func TestRealTimeNetworkHistoryBuffer(t *testing.T) {
	buffer := NewRealTimeNetworkHistoryBuffer()

	assert.NotNil(t, buffer)
	assert.Equal(t, 1000, buffer.maxSize)
	assert.Equal(t, 0, len(buffer.history))

	// Add conditions
	conditions := &RealTimeNetworkConditions{
		Timestamp:     time.Now(),
		BandwidthMBps: 50.0,
		LatencyMs:     30.0,
	}

	buffer.AddConditions(conditions)
	assert.Equal(t, 1, len(buffer.history))

	history := buffer.GetHistory()
	assert.Equal(t, 1, len(history))
	assert.Equal(t, conditions.BandwidthMBps, history[0].BandwidthMBps)
}

func TestRealTimeNetworkTrendAnalyzer(t *testing.T) {
	analyzer := NewRealTimeNetworkTrendAnalyzer()

	assert.NotNil(t, analyzer)
	assert.NotNil(t, analyzer.trends)

	trends := analyzer.GetCurrentTrends()
	assert.NotNil(t, trends)
	assert.Equal(t, "stable", trends.BandwidthTrend)
	assert.Equal(t, "stable", trends.LatencyTrend)
	assert.Equal(t, "stable", trends.StabilityTrend)
	assert.Equal(t, "stable", trends.QualityTrend)
	assert.Equal(t, 0.8, trends.PredictedQuality)
	assert.Equal(t, 0.7, trends.TrendConfidence)

	// Test update
	conditions := &RealTimeNetworkConditions{
		Timestamp:     time.Now(),
		BandwidthMBps: 100.0,
	}

	analyzer.UpdateTrends(conditions)
	updatedTrends := analyzer.GetCurrentTrends()
	assert.True(t, updatedTrends.LastUpdate.After(trends.LastUpdate))
}
