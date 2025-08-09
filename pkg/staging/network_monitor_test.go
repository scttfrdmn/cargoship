package staging

import (
	"context"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
)

func TestNetworkConditionMonitor_UpdatePredictions(t *testing.T) {
	config := DefaultStagingConfig()
	monitor := NewNetworkConditionMonitor(config)

	// Add some history
	monitor.addToHistory(&NetworkCondition{
		Timestamp:       time.Now().Add(-time.Minute),
		BandwidthMBps:   50.0,
		LatencyMs:       20.0,
		PacketLoss:      0.01,
		Jitter:          2.0,
		CongestionLevel: 0.1,
		Reliability:     0.95,
	})

	monitor.addToHistory(&NetworkCondition{
		Timestamp:       time.Now(),
		BandwidthMBps:   55.0,
		LatencyMs:       18.0,
		PacketLoss:      0.005,
		Jitter:          1.8,
		CongestionLevel: 0.08,
		Reliability:     0.97,
	})

	// Update predictions should not panic
	monitor.UpdatePredictions()

	// Verify current condition was updated
	condition := monitor.GetCurrentCondition()
	assert.NotNil(t, condition)
	assert.True(t, condition.Timestamp.After(time.Time{}))
}

func TestNetworkConditionMonitor_RecordTransferMetrics(t *testing.T) {
	config := DefaultStagingConfig()
	monitor := NewNetworkConditionMonitor(config)

	// Record some metrics
	monitor.RecordTransferMetrics(100.0, 20.0) // 100 MB/s, 20ms latency

	condition := monitor.GetCurrentCondition()
	assert.NotNil(t, condition)
	assert.Equal(t, 100.0, condition.BandwidthMBps)
	assert.Equal(t, 20.0, condition.LatencyMs)

	// Record different metrics
	monitor.RecordTransferMetrics(50.0, 30.0) // 50 MB/s, 30ms latency

	condition = monitor.GetCurrentCondition()
	assert.NotNil(t, condition)
	assert.Equal(t, 50.0, condition.BandwidthMBps)
	assert.Equal(t, 30.0, condition.LatencyMs)
}

func TestNetworkConditionMonitor_UpdateConditions(t *testing.T) {
	config := DefaultStagingConfig()
	monitor := NewNetworkConditionMonitor(config)

	// Should not panic
	monitor.updateConditions()

	condition := monitor.GetCurrentCondition()
	assert.NotNil(t, condition)
	assert.True(t, condition.Timestamp.After(time.Time{}))
}

func TestNetworkConditionMonitor_MeasureNetworkCondition(t *testing.T) {
	config := DefaultStagingConfig()
	monitor := NewNetworkConditionMonitor(config)

	condition := monitor.measureNetworkCondition()

	assert.NotNil(t, condition)
	assert.True(t, condition.Timestamp.After(time.Time{}))
	assert.GreaterOrEqual(t, condition.BandwidthMBps, 0.0)
	assert.GreaterOrEqual(t, condition.LatencyMs, 0.0)
	assert.GreaterOrEqual(t, condition.PacketLoss, 0.0)
	assert.GreaterOrEqual(t, condition.Jitter, 0.0)
	assert.GreaterOrEqual(t, condition.CongestionLevel, 0.0)
	assert.GreaterOrEqual(t, condition.Reliability, 0.0)
	assert.LessOrEqual(t, condition.Reliability, 1.0)
}

func TestNetworkConditionMonitor_EstimateBandwidth(t *testing.T) {
	config := DefaultStagingConfig()
	monitor := NewNetworkConditionMonitor(config)

	bandwidth := monitor.estimateBandwidth()

	assert.GreaterOrEqual(t, bandwidth, 0.0)
	assert.LessOrEqual(t, bandwidth, 10000.0) // Should be reasonable upper bound
}

func TestNetworkConditionMonitor_EstimateLatency(t *testing.T) {
	config := DefaultStagingConfig()
	monitor := NewNetworkConditionMonitor(config)

	latency := monitor.estimateLatency()

	assert.GreaterOrEqual(t, latency, 0.0)
	assert.LessOrEqual(t, latency, 10000.0) // Should be reasonable upper bound
}

func TestNetworkConditionMonitor_EstimatePacketLoss(t *testing.T) {
	config := DefaultStagingConfig()
	monitor := NewNetworkConditionMonitor(config)

	packetLoss := monitor.estimatePacketLoss()

	assert.GreaterOrEqual(t, packetLoss, 0.0)
	assert.LessOrEqual(t, packetLoss, 1.0)
}

func TestNetworkConditionMonitor_EstimateJitter(t *testing.T) {
	config := DefaultStagingConfig()
	monitor := NewNetworkConditionMonitor(config)

	jitter := monitor.estimateJitter()

	assert.GreaterOrEqual(t, jitter, 0.0)
}

func TestNetworkConditionMonitor_EstimateCongestion(t *testing.T) {
	config := DefaultStagingConfig()
	monitor := NewNetworkConditionMonitor(config)

	congestion := monitor.estimateCongestion()

	assert.GreaterOrEqual(t, congestion, 0.0)
	assert.LessOrEqual(t, congestion, 1.0)
}

func TestNetworkConditionMonitor_EstimateReliability(t *testing.T) {
	config := DefaultStagingConfig()
	monitor := NewNetworkConditionMonitor(config)

	reliability := monitor.estimateReliability()

	assert.GreaterOrEqual(t, reliability, 0.0)
	assert.LessOrEqual(t, reliability, 1.0)
}

func TestNetworkConditionMonitor_AddToHistory(t *testing.T) {
	config := DefaultStagingConfig()
	monitor := NewNetworkConditionMonitor(config)

	condition := &NetworkCondition{
		Timestamp:       time.Now(),
		BandwidthMBps:   50.0,
		LatencyMs:       20.0,
		PacketLoss:      0.01,
		Jitter:          2.0,
		CongestionLevel: 0.1,
		Reliability:     0.95,
	}

	monitor.addToHistory(condition)

	// Verify history was updated
	assert.Greater(t, len(monitor.conditionHistory), 0)
}

func TestNetworkTrendAnalyzer_AnalyzeTrend(t *testing.T) {
	analyzer := NewNetworkTrendAnalyzer()

	history := []*NetworkCondition{
		{
			Timestamp:     time.Now().Add(-time.Minute * 3),
			BandwidthMBps: 40.0,
		},
		{
			Timestamp:     time.Now().Add(-time.Minute * 2),
			BandwidthMBps: 45.0,
		},
		{
			Timestamp:     time.Now().Add(-time.Minute),
			BandwidthMBps: 50.0,
		},
	}

	trend := analyzer.AnalyzeTrend(history)
	assert.NotEqual(t, NetworkTrend(0), trend) // Should return some trend
}

func TestNetworkTrendAnalyzer_CalculateMetricTrend(t *testing.T) {
	analyzer := NewNetworkTrendAnalyzer()

	conditions := []*NetworkCondition{
		{BandwidthMBps: 40.0},
		{BandwidthMBps: 45.0},
		{BandwidthMBps: 50.0},
		{BandwidthMBps: 55.0},
	}

	trend := analyzer.calculateMetricTrend(conditions, func(nc *NetworkCondition) float64 {
		return nc.BandwidthMBps
	})

	assert.GreaterOrEqual(t, trend, -1.0)
	assert.LessOrEqual(t, trend, 1.0)
}

func TestNetworkTrendAnalyzer_CalculateVolatility(t *testing.T) {
	analyzer := NewNetworkTrendAnalyzer()

	conditions := []*NetworkCondition{
		{BandwidthMBps: 40.0},
		{BandwidthMBps: 45.0},
		{BandwidthMBps: 50.0},
		{BandwidthMBps: 55.0},
	}

	volatility := analyzer.calculateVolatility(conditions)

	assert.GreaterOrEqual(t, volatility, 0.0)
}

func TestNetworkPredictor_PredictCondition(t *testing.T) {
	config := DefaultStagingConfig()
	predictor := NewNetworkPredictor(config)

	history := []*NetworkCondition{
		{
			Timestamp:     time.Now().Add(-time.Minute * 2),
			BandwidthMBps: 45.0,
			LatencyMs:     20.0,
		},
		{
			Timestamp:     time.Now().Add(-time.Minute),
			BandwidthMBps: 50.0,
			LatencyMs:     18.0,
		},
	}

	predicted := predictor.PredictCondition(history, time.Minute)

	assert.NotNil(t, predicted)
	assert.GreaterOrEqual(t, predicted.BandwidthMBps, 0.0)
	assert.GreaterOrEqual(t, predicted.LatencyMs, 0.0)
}

func TestPredictionModel_Predict(t *testing.T) {
	model := NewPredictionModel("bandwidth")

	history := []*NetworkCondition{
		{BandwidthMBps: 40.0},
		{BandwidthMBps: 45.0},
		{BandwidthMBps: 50.0},
		{BandwidthMBps: 55.0},
	}

	prediction := model.Predict(history, time.Minute)

	assert.NotNil(t, prediction)
}

func TestPredictionModel_ExtractMetricValues(t *testing.T) {
	model := NewPredictionModel("bandwidth")

	history := []*NetworkCondition{
		{BandwidthMBps: 40.0},
		{BandwidthMBps: 50.0},
		{BandwidthMBps: 60.0},
	}

	values := model.extractMetricValues(history)

	assert.Len(t, values, 3)
	assert.Equal(t, 40.0, values[0])
	assert.Equal(t, 50.0, values[1])
	assert.Equal(t, 60.0, values[2])
}

func TestPredictionModel_CalculateTrend(t *testing.T) {
	model := NewPredictionModel("bandwidth")

	// Increasing trend
	values := []float64{10.0, 20.0, 30.0, 40.0}
	trend := model.calculateTrend(values)
	assert.Greater(t, trend, 0.0)

	// Decreasing trend
	values = []float64{40.0, 30.0, 20.0, 10.0}
	trend = model.calculateTrend(values)
	assert.Less(t, trend, 0.0)

	// Stable trend
	values = []float64{30.0, 30.0, 30.0, 30.0}
	trend = model.calculateTrend(values)
	assert.InDelta(t, 0.0, trend, 0.1)
}

func TestPredictionModel_CalculateConfidence(t *testing.T) {
	model := NewPredictionModel("bandwidth")

	// Stable values should have high confidence
	values := []float64{50.0, 51.0, 49.0, 50.5}
	trend := model.calculateTrend(values)
	confidence := model.calculateConfidence(values, trend)
	assert.GreaterOrEqual(t, confidence, 0.0)
	assert.LessOrEqual(t, confidence, 1.0)

	// Highly variable values should have lower confidence
	values = []float64{10.0, 50.0, 5.0, 80.0}
	trend = model.calculateTrend(values)
	variableConfidence := model.calculateConfidence(values, trend)
	assert.GreaterOrEqual(t, variableConfidence, 0.0)
	assert.LessOrEqual(t, variableConfidence, 1.0)
}

func TestNetworkConditionMonitor_Start(t *testing.T) {
	config := DefaultStagingConfig()
	monitor := NewNetworkConditionMonitor(config)
	monitor.updateInterval = time.Millisecond * 10 // Speed up for testing

	ctx, cancel := context.WithTimeout(context.Background(), time.Millisecond*50)
	defer cancel()

	// Start should not block and should stop when context is cancelled
	done := make(chan struct{})
	go func() {
		monitor.Start(ctx)
		close(done)
	}()

	select {
	case <-done:
		// Good, Start() returned when context was cancelled
	case <-time.After(time.Millisecond * 100):
		t.Fatal("Start() did not return when context was cancelled")
	}
}
