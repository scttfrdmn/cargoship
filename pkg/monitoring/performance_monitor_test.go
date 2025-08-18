package monitoring

import (
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestPerformanceMonitor_Creation(t *testing.T) {
	config := DefaultMonitoringConfig()
	pm := NewPerformanceMonitor(config)
	
	assert.NotNil(t, pm)
	assert.NotNil(t, pm.metricsCollector)
	assert.NotNil(t, pm.alertManager)
	assert.NotNil(t, pm.thresholdManager)
	assert.NotNil(t, pm.analyticsEngine)
	assert.NotNil(t, pm.dashboardRenderer)
	assert.False(t, pm.isRunning)
}

func TestPerformanceMonitor_StartStop(t *testing.T) {
	config := DefaultMonitoringConfig()
	pm := NewPerformanceMonitor(config)
	
	// Test start
	err := pm.Start()
	require.NoError(t, err)
	assert.True(t, pm.isRunning)
	
	// Test double start (should not error)
	err = pm.Start()
	assert.Error(t, err)
	assert.Contains(t, err.Error(), "already running")
	
	// Test stop
	err = pm.Stop()
	require.NoError(t, err)
	assert.False(t, pm.isRunning)
}

func TestPerformanceMonitor_SystemHealth(t *testing.T) {
	config := DefaultMonitoringConfig()
	pm := NewPerformanceMonitor(config)
	
	err := pm.Start()
	require.NoError(t, err)
	defer func() { _ = pm.Stop() }()
	
	// Wait a moment for initialization
	time.Sleep(100 * time.Millisecond)
	
	health := pm.GetSystemHealth()
	assert.NotNil(t, health)
	assert.NotEqual(t, HealthUnknown, health.Status)
	assert.NotEmpty(t, health.Message)
	assert.WithinDuration(t, time.Now(), health.Timestamp, time.Second)
}

func TestPerformanceMonitor_Metrics(t *testing.T) {
	config := DefaultMonitoringConfig()
	pm := NewPerformanceMonitor(config)
	
	err := pm.Start()
	require.NoError(t, err)
	defer func() { _ = pm.Stop() }()
	
	metrics := pm.GetMetrics()
	assert.NotNil(t, metrics)
	assert.NotNil(t, metrics.TransferMetrics)
	assert.NotNil(t, metrics.SystemMetrics)
	assert.NotNil(t, metrics.NetworkMetrics)
	assert.NotNil(t, metrics.S3Metrics)
	assert.NotNil(t, metrics.StagingMetrics)
}

func TestPerformanceMonitor_Alerts(t *testing.T) {
	config := DefaultMonitoringConfig()
	pm := NewPerformanceMonitor(config)
	
	err := pm.Start()
	require.NoError(t, err)
	defer func() { _ = pm.Stop() }()
	
	alerts := pm.GetAlerts()
	assert.NotNil(t, alerts)
	// Initially should have no alerts
}

func TestPerformanceMonitor_Predictions(t *testing.T) {
	config := DefaultMonitoringConfig()
	config.EnablePredictive = true
	pm := NewPerformanceMonitor(config)
	
	err := pm.Start()
	require.NoError(t, err)
	defer func() { _ = pm.Stop() }()
	
	predictions := pm.GetPredictions()
	// Initially might be nil due to insufficient data
	if predictions != nil {
		assert.WithinDuration(t, time.Now(), predictions.GeneratedAt, time.Minute)
		assert.True(t, predictions.Confidence >= 0 && predictions.Confidence <= 1)
	}
}

func TestPerformanceMonitor_CustomMetrics(t *testing.T) {
	config := DefaultMonitoringConfig()
	pm := NewPerformanceMonitor(config)
	
	// Register custom metric
	metric := &CustomMetric{
		Name:        "test_metric",
		Description: "Test metric for unit tests",
		Type:        MetricTypeCounter,
		Unit:        "count",
		DataPoints:  make([]*MetricDataPoint, 0),
		Labels:      map[string]string{"test": "true"},
		CreatedAt:   time.Now(),
	}
	
	err := pm.RegisterMetric(metric)
	assert.NoError(t, err)
	
	// Record metric value
	pm.RecordMetric("test_metric", 42.0, map[string]string{"instance": "test"})
	
	// Verify metric was recorded (would need access to internal state)
	metrics := pm.GetMetrics()
	assert.NotNil(t, metrics)
}

func TestPerformanceMonitor_ThresholdManagement(t *testing.T) {
	config := DefaultMonitoringConfig()
	pm := NewPerformanceMonitor(config)
	
	// Set custom threshold
	threshold := &AlertThreshold{
		Value:       100.0,
		Enabled:     true,
		LastUpdated: time.Now(),
		Source:      "manual",
	}
	
	err := pm.SetThreshold("latency", threshold)
	assert.NoError(t, err)
}

func TestDefaultMonitoringConfig(t *testing.T) {
	config := DefaultMonitoringConfig()
	
	assert.NotNil(t, config)
	assert.Equal(t, time.Second*10, config.MetricsInterval)
	assert.Equal(t, time.Second*5, config.AlertCheckInterval)
	assert.Equal(t, time.Second, config.DashboardInterval)
	assert.True(t, config.EnableRealTimeAlerts)
	assert.True(t, config.EnablePredictive)
	assert.False(t, config.EnableAutoRemediation)
	assert.NotNil(t, config.DefaultThresholds)
}

func TestPerformanceMonitor_Integration(t *testing.T) {
	config := DefaultMonitoringConfig()
	config.MetricsInterval = time.Millisecond * 100
	config.AlertCheckInterval = time.Millisecond * 50
	config.EnablePredictive = true
	
	pm := NewPerformanceMonitor(config)
	
	err := pm.Start()
	require.NoError(t, err)
	defer func() { _ = pm.Stop() }()
	
	// Wait for some monitoring cycles
	time.Sleep(time.Millisecond * 500)
	
	// Verify system is collecting metrics
	metrics := pm.GetMetrics()
	assert.NotNil(t, metrics)
	assert.WithinDuration(t, time.Now(), metrics.LastUpdated, time.Second)
	
	// Check system health
	health := pm.GetSystemHealth()
	assert.NotNil(t, health)
	assert.NotEqual(t, HealthUnknown, health.Status)
	
	// Verify subsystem health is reported
	if health.SubsystemHealth != nil {
		assert.Contains(t, health.SubsystemHealth, "transfer")
		assert.Contains(t, health.SubsystemHealth, "system")
		assert.Contains(t, health.SubsystemHealth, "network")
		assert.Contains(t, health.SubsystemHealth, "s3")
		assert.Contains(t, health.SubsystemHealth, "staging")
	}
}

func TestPerformanceMonitor_StressConditions(t *testing.T) {
	config := DefaultMonitoringConfig()
	config.MetricsInterval = time.Millisecond * 10 // Very fast updates
	config.AlertCheckInterval = time.Millisecond * 5
	
	pm := NewPerformanceMonitor(config)
	
	err := pm.Start()
	require.NoError(t, err)
	defer func() { _ = pm.Stop() }()
	
	// Generate rapid metric requests
	for i := 0; i < 100; i++ {
		go func() {
			pm.GetMetrics()
			pm.GetSystemHealth()
			pm.GetAlerts()
		}()
	}
	
	// Wait for operations to complete
	time.Sleep(time.Millisecond * 100)
	
	// System should still be responsive
	health := pm.GetSystemHealth()
	assert.NotNil(t, health)
	assert.NotEqual(t, HealthUnknown, health.Status)
}

func TestPerformanceMonitor_AutoRemediation(t *testing.T) {
	config := DefaultMonitoringConfig()
	config.EnableAutoRemediation = true
	config.AlertCheckInterval = time.Millisecond * 50
	
	pm := NewPerformanceMonitor(config)
	
	err := pm.Start()
	require.NoError(t, err)
	defer func() { _ = pm.Stop() }()
	
	// Simulate critical alert that should trigger auto-remediation
	alert := &Alert{
		Type:        AlertTypeHighMemoryUsage,
		Severity:    SeverityCritical,
		Title:       "Critical Memory Usage",
		Description: "Memory usage exceeded critical threshold",
		Timestamp:   time.Now(),
		Source:      "TestMonitor",
	}
	
	pm.alertManager.AddAlert(alert)
	
	// Wait for auto-remediation to potentially trigger
	time.Sleep(time.Millisecond * 200)
	
	// Verify system is still responsive
	health := pm.GetSystemHealth()
	assert.NotNil(t, health)
}

// Benchmark tests
func BenchmarkPerformanceMonitor_GetMetrics(b *testing.B) {
	config := DefaultMonitoringConfig()
	pm := NewPerformanceMonitor(config)
	
	err := pm.Start()
	require.NoError(b, err)
	defer func() { _ = pm.Stop() }()
	
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		pm.GetMetrics()
	}
}

func BenchmarkPerformanceMonitor_GetSystemHealth(b *testing.B) {
	config := DefaultMonitoringConfig()
	pm := NewPerformanceMonitor(config)
	
	err := pm.Start()
	require.NoError(b, err)
	defer func() { _ = pm.Stop() }()
	
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		pm.GetSystemHealth()
	}
}

func BenchmarkPerformanceMonitor_RecordMetric(b *testing.B) {
	config := DefaultMonitoringConfig()
	pm := NewPerformanceMonitor(config)
	
	metric := &CustomMetric{
		Name:        "benchmark_metric",
		Description: "Benchmark metric",
		Type:        MetricTypeCounter,
		Unit:        "count",
		DataPoints:  make([]*MetricDataPoint, 0),
		Labels:      make(map[string]string),
		CreatedAt:   time.Now(),
	}
	
	_ = pm.RegisterMetric(metric)
	
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		pm.RecordMetric("benchmark_metric", float64(i), map[string]string{"iteration": "benchmark"})
	}
}