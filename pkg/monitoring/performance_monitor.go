// Package monitoring provides comprehensive performance monitoring and alerting for CargoShip
package monitoring

import (
	"context"
	"fmt"
	"sync"
	"time"
)

// PerformanceMonitor provides comprehensive monitoring of system performance.
type PerformanceMonitor struct {
	// Core components
	metricsCollector  *MetricsCollector
	alertManager      *AlertManager
	thresholdManager  *ThresholdManager
	analyticsEngine   *AnalyticsEngine
	dashboardRenderer *DashboardRenderer

	// Configuration
	config *MonitoringConfig

	// State management
	isRunning bool
	mu        sync.RWMutex
	ctx       context.Context
	cancel    context.CancelFunc
	wg        sync.WaitGroup

	// Subsystem monitors
	transferMonitor *TransferPerformanceMonitor
	systemMonitor   *SystemResourceMonitor
	networkMonitor  *NetworkPerformanceMonitor
	s3Monitor       *S3PerformanceMonitor
	stagingMonitor  *StagingPerformanceMonitor
}

// MonitoringConfig configures the performance monitoring system.
type MonitoringConfig struct {
	// Collection intervals
	MetricsInterval    time.Duration `yaml:"metrics_interval" json:"metrics_interval"`
	AlertCheckInterval time.Duration `yaml:"alert_check_interval" json:"alert_check_interval"`
	DashboardInterval  time.Duration `yaml:"dashboard_interval" json:"dashboard_interval"`

	// Storage and retention
	MetricsRetention   time.Duration `yaml:"metrics_retention" json:"metrics_retention"`
	AlertRetention     time.Duration `yaml:"alert_retention" json:"alert_retention"`
	MaxMetricsInMemory int           `yaml:"max_metrics_in_memory" json:"max_metrics_in_memory"`

	// Features
	EnableRealTimeAlerts  bool `yaml:"enable_realtime_alerts" json:"enable_realtime_alerts"`
	EnablePredictive      bool `yaml:"enable_predictive" json:"enable_predictive"`
	EnableAutoRemediation bool `yaml:"enable_auto_remediation" json:"enable_auto_remediation"`
	EnableCloudWatch      bool `yaml:"enable_cloudwatch" json:"enable_cloudwatch"`

	// Thresholds (will be overridden by dynamic thresholds)
	DefaultThresholds *DefaultThresholds `yaml:"default_thresholds" json:"default_thresholds"`

	// External integrations
	CloudWatchConfig *CloudWatchConfig `yaml:"cloudwatch" json:"cloudwatch,omitempty"`
	WebhookConfig    *WebhookConfig    `yaml:"webhook" json:"webhook,omitempty"`
}

// DefaultThresholds defines default alert thresholds.
type DefaultThresholds struct {
	// Transfer performance
	MinThroughputMBps float64       `yaml:"min_throughput_mbps" json:"min_throughput_mbps"`
	MaxTransferTime   time.Duration `yaml:"max_transfer_time" json:"max_transfer_time"`
	MaxErrorRate      float64       `yaml:"max_error_rate" json:"max_error_rate"`

	// System resources
	MaxCPUUsage    float64 `yaml:"max_cpu_usage" json:"max_cpu_usage"`
	MaxMemoryUsage float64 `yaml:"max_memory_usage" json:"max_memory_usage"`
	MaxDiskUsage   float64 `yaml:"max_disk_usage" json:"max_disk_usage"`

	// Network performance
	MaxLatencyMs   float64 `yaml:"max_latency_ms" json:"max_latency_ms"`
	MinReliability float64 `yaml:"min_reliability" json:"min_reliability"`
	MaxPacketLoss  float64 `yaml:"max_packet_loss" json:"max_packet_loss"`

	// S3 specific
	MaxS3Errors       int     `yaml:"max_s3_errors" json:"max_s3_errors"`
	MinS3Availability float64 `yaml:"min_s3_availability" json:"min_s3_availability"`
}

// CloudWatchConfig configures CloudWatch integration.
type CloudWatchConfig struct {
	Enabled       bool          `yaml:"enabled" json:"enabled"`
	Region        string        `yaml:"region" json:"region"`
	Namespace     string        `yaml:"namespace" json:"namespace"`
	FlushInterval time.Duration `yaml:"flush_interval" json:"flush_interval"`
}

// WebhookConfig configures webhook alerting.
type WebhookConfig struct {
	Enabled    bool              `yaml:"enabled" json:"enabled"`
	URL        string            `yaml:"url" json:"url"`
	Headers    map[string]string `yaml:"headers" json:"headers,omitempty"`
	Timeout    time.Duration     `yaml:"timeout" json:"timeout"`
	RetryCount int               `yaml:"retry_count" json:"retry_count"`
}

// NewPerformanceMonitor creates a comprehensive performance monitoring system.
func NewPerformanceMonitor(config *MonitoringConfig) *PerformanceMonitor {
	if config == nil {
		config = DefaultMonitoringConfig()
	}

	ctx, cancel := context.WithCancel(context.Background())

	pm := &PerformanceMonitor{
		config: config,
		ctx:    ctx,
		cancel: cancel,
	}

	// Initialize components
	pm.metricsCollector = NewMetricsCollector(config)
	pm.alertManager = NewAlertManager(config)
	pm.thresholdManager = NewThresholdManager(config)
	pm.analyticsEngine = NewAnalyticsEngine(config)
	pm.dashboardRenderer = NewDashboardRenderer(config)

	// Initialize subsystem monitors
	pm.transferMonitor = NewTransferPerformanceMonitor(config)
	pm.systemMonitor = NewSystemResourceMonitor(config)
	pm.networkMonitor = NewNetworkPerformanceMonitor(config)
	pm.s3Monitor = NewS3PerformanceMonitor(config)
	pm.stagingMonitor = NewStagingPerformanceMonitor(config)

	return pm
}

// DefaultMonitoringConfig returns sensible defaults for monitoring.
func DefaultMonitoringConfig() *MonitoringConfig {
	return &MonitoringConfig{
		MetricsInterval:       time.Second * 10,
		AlertCheckInterval:    time.Second * 5,
		DashboardInterval:     time.Second,
		MetricsRetention:      time.Hour * 24,
		AlertRetention:        time.Hour * 72,
		MaxMetricsInMemory:    10000,
		EnableRealTimeAlerts:  true,
		EnablePredictive:      true,
		EnableAutoRemediation: false, // Conservative default
		EnableCloudWatch:      false, // Requires AWS setup
		DefaultThresholds: &DefaultThresholds{
			MinThroughputMBps: 1.0,
			MaxTransferTime:   time.Hour,
			MaxErrorRate:      0.05,
			MaxCPUUsage:       0.8,
			MaxMemoryUsage:    0.85,
			MaxDiskUsage:      0.9,
			MaxLatencyMs:      1000,
			MinReliability:    0.95,
			MaxPacketLoss:     0.01,
			MaxS3Errors:       10,
			MinS3Availability: 0.99,
		},
		CloudWatchConfig: &CloudWatchConfig{
			Enabled:       false,
			Namespace:     "CargoShip/Performance",
			FlushInterval: time.Minute,
		},
		WebhookConfig: &WebhookConfig{
			Enabled:    false,
			Timeout:    time.Second * 10,
			RetryCount: 3,
		},
	}
}

// Start begins comprehensive performance monitoring.
func (pm *PerformanceMonitor) Start() error {
	pm.mu.Lock()
	defer pm.mu.Unlock()

	if pm.isRunning {
		return fmt.Errorf("performance monitor is already running")
	}

	// Start all components
	if err := pm.metricsCollector.Start(pm.ctx); err != nil {
		return fmt.Errorf("failed to start metrics collector: %w", err)
	}

	if err := pm.alertManager.Start(pm.ctx); err != nil {
		return fmt.Errorf("failed to start alert manager: %w", err)
	}

	if err := pm.thresholdManager.Start(pm.ctx); err != nil {
		return fmt.Errorf("failed to start threshold manager: %w", err)
	}

	if err := pm.analyticsEngine.Start(pm.ctx); err != nil {
		return fmt.Errorf("failed to start analytics engine: %w", err)
	}

	if err := pm.dashboardRenderer.Start(pm.ctx); err != nil {
		return fmt.Errorf("failed to start dashboard renderer: %w", err)
	}

	// Start subsystem monitors
	if err := pm.transferMonitor.Start(pm.ctx); err != nil {
		return fmt.Errorf("failed to start transfer monitor: %w", err)
	}

	if err := pm.systemMonitor.Start(pm.ctx); err != nil {
		return fmt.Errorf("failed to start system monitor: %w", err)
	}

	if err := pm.networkMonitor.Start(pm.ctx); err != nil {
		return fmt.Errorf("failed to start network monitor: %w", err)
	}

	if err := pm.s3Monitor.Start(pm.ctx); err != nil {
		return fmt.Errorf("failed to start S3 monitor: %w", err)
	}

	if err := pm.stagingMonitor.Start(pm.ctx); err != nil {
		return fmt.Errorf("failed to start staging monitor: %w", err)
	}

	// Start main monitoring loops
	pm.wg.Add(3)
	go pm.monitoringLoop()
	go pm.alertingLoop()
	go pm.analyticsLoop()

	pm.isRunning = true
	return nil
}

// Stop gracefully shuts down the performance monitoring system.
func (pm *PerformanceMonitor) Stop() error {
	pm.mu.Lock()
	if !pm.isRunning {
		pm.mu.Unlock()
		return nil
	}

	pm.cancel() // Signal all goroutines to stop
	pm.isRunning = false
	pm.mu.Unlock()

	// Wait for all goroutines to finish
	pm.wg.Wait()
	return nil
}

// GetSystemHealth returns overall system health status.
func (pm *PerformanceMonitor) GetSystemHealth() *SystemHealthStatus {
	pm.mu.RLock()
	defer pm.mu.RUnlock()

	if !pm.isRunning {
		return &SystemHealthStatus{
			Status:    HealthUnknown,
			Message:   "Performance monitor is not running",
			Timestamp: time.Now(),
		}
	}

	// Collect health from all subsystems
	transferHealth := pm.transferMonitor.GetHealth()
	systemHealth := pm.systemMonitor.GetHealth()
	networkHealth := pm.networkMonitor.GetHealth()
	s3Health := pm.s3Monitor.GetHealth()
	stagingHealth := pm.stagingMonitor.GetHealth()

	// Determine overall health
	overallStatus := pm.calculateOverallHealth(
		transferHealth, systemHealth, networkHealth, s3Health, stagingHealth,
	)

	return &SystemHealthStatus{
		Status:             overallStatus.Status,
		Message:            overallStatus.Message,
		Timestamp:          time.Now(),
		SubsystemHealth:    overallStatus.Details,
		ActiveAlerts:       pm.alertManager.GetActiveAlerts(),
		PerformanceMetrics: pm.getLatestMetrics(),
	}
}

// GetMetrics returns current performance metrics.
func (pm *PerformanceMonitor) GetMetrics() *PerformanceMetrics {
	return pm.metricsCollector.GetCurrentMetrics()
}

// GetAlerts returns current alert status.
func (pm *PerformanceMonitor) GetAlerts() []*Alert {
	return pm.alertManager.GetActiveAlerts()
}

// GetPredictions returns performance predictions.
func (pm *PerformanceMonitor) GetPredictions() *PerformancePredictions {
	if !pm.config.EnablePredictive {
		return nil
	}
	return pm.analyticsEngine.GetPredictions()
}

// RegisterMetric registers a custom metric for monitoring.
func (pm *PerformanceMonitor) RegisterMetric(metric *CustomMetric) error {
	return pm.metricsCollector.RegisterMetric(metric)
}

// RecordMetric records a metric value.
func (pm *PerformanceMonitor) RecordMetric(name string, value float64, labels map[string]string) {
	pm.metricsCollector.RecordMetric(name, value, labels, time.Now())
}

// SetThreshold dynamically sets an alert threshold.
func (pm *PerformanceMonitor) SetThreshold(metric string, threshold *AlertThreshold) error {
	return pm.thresholdManager.SetThreshold(metric, threshold)
}

// monitoringLoop runs the main monitoring collection loop.
func (pm *PerformanceMonitor) monitoringLoop() {
	defer pm.wg.Done()
	ticker := time.NewTicker(pm.config.MetricsInterval)
	defer ticker.Stop()

	for {
		select {
		case <-pm.ctx.Done():
			return
		case <-ticker.C:
			pm.collectMetrics()
		}
	}
}

// alertingLoop runs the alert checking loop.
func (pm *PerformanceMonitor) alertingLoop() {
	defer pm.wg.Done()
	ticker := time.NewTicker(pm.config.AlertCheckInterval)
	defer ticker.Stop()

	for {
		select {
		case <-pm.ctx.Done():
			return
		case <-ticker.C:
			pm.checkAlerts()
		}
	}
}

// analyticsLoop runs predictive analytics.
func (pm *PerformanceMonitor) analyticsLoop() {
	defer pm.wg.Done()
	if !pm.config.EnablePredictive {
		return
	}

	ticker := time.NewTicker(time.Minute) // Run analytics every minute
	defer ticker.Stop()

	for {
		select {
		case <-pm.ctx.Done():
			return
		case <-ticker.C:
			pm.runAnalytics()
		}
	}
}

// collectMetrics collects metrics from all subsystems.
func (pm *PerformanceMonitor) collectMetrics() {
	// Collect from all subsystem monitors
	pm.metricsCollector.CollectFrom(pm.transferMonitor)
	pm.metricsCollector.CollectFrom(pm.systemMonitor)
	pm.metricsCollector.CollectFrom(pm.networkMonitor)
	pm.metricsCollector.CollectFrom(pm.s3Monitor)
	pm.metricsCollector.CollectFrom(pm.stagingMonitor)

	// Update analytics engine with new data
	if pm.config.EnablePredictive {
		pm.analyticsEngine.ProcessNewMetrics(pm.metricsCollector.GetCurrentMetrics())
	}
}

// checkAlerts evaluates current metrics against thresholds.
func (pm *PerformanceMonitor) checkAlerts() {
	currentMetrics := pm.metricsCollector.GetCurrentMetrics()
	thresholds := pm.thresholdManager.GetCurrentThresholds()

	// Check for new alerts
	newAlerts := pm.alertManager.EvaluateMetrics(currentMetrics, thresholds)

	// Handle auto-remediation if enabled
	if pm.config.EnableAutoRemediation {
		for _, alert := range newAlerts {
			if alert.Severity == SeverityCritical {
				pm.attemptAutoRemediation(alert)
			}
		}
	}
}

// runAnalytics performs predictive analytics.
func (pm *PerformanceMonitor) runAnalytics() {
	pm.analyticsEngine.RunPredictiveAnalysis()

	// Check for predicted issues
	predictions := pm.analyticsEngine.GetPredictions()
	if predictions != nil {
		pm.handlePredictedIssues(predictions)
	}
}

// calculateOverallHealth determines overall system health from subsystem health.
func (pm *PerformanceMonitor) calculateOverallHealth(healthStatuses ...*HealthStatus) *SystemHealthStatus {
	criticalCount := 0
	warningCount := 0

	details := make(map[string]*HealthStatus)

	for i, health := range healthStatuses {
		subsystemName := []string{"transfer", "system", "network", "s3", "staging"}[i]
		details[subsystemName] = health

		switch health.Status {
		case HealthCritical:
			criticalCount++
		case HealthWarning:
			warningCount++
		}
	}

	var overallStatus HealthStatus
	if criticalCount > 0 {
		overallStatus = HealthStatus{
			Status:  HealthCritical,
			Message: fmt.Sprintf("%d subsystems in critical state", criticalCount),
		}
	} else if warningCount > 0 {
		overallStatus = HealthStatus{
			Status:  HealthWarning,
			Message: fmt.Sprintf("%d subsystems in warning state", warningCount),
		}
	} else {
		overallStatus = HealthStatus{
			Status:  HealthHealthy,
			Message: "All subsystems healthy",
		}
	}

	return &SystemHealthStatus{
		Status:    overallStatus.Status,
		Message:   overallStatus.Message,
		Details:   details,
		Timestamp: time.Now(),
	}
}

// getLatestMetrics gets the most recent metrics for health status.
func (pm *PerformanceMonitor) getLatestMetrics() *PerformanceMetrics {
	return pm.metricsCollector.GetCurrentMetrics()
}

// attemptAutoRemediation attempts automatic remediation for critical alerts.
func (pm *PerformanceMonitor) attemptAutoRemediation(alert *Alert) {
	// Implement auto-remediation strategies based on alert type
	// This is a simplified implementation - in practice would be more sophisticated
	switch alert.Type {
	case AlertTypeHighMemoryUsage:
		// Could trigger garbage collection, reduce buffer sizes, etc.
		pm.systemMonitor.TriggerMemoryCleanup()
	case AlertTypeHighLatency:
		// Could switch to different S3 endpoints, adjust concurrency, etc.
		pm.networkMonitor.OptimizeNetworkSettings()
	case AlertTypeS3Errors:
		// Could implement retry logic, failover to different regions, etc.
		pm.s3Monitor.TriggerFailoverLogic()
	}
}

// handlePredictedIssues handles predicted performance issues.
func (pm *PerformanceMonitor) handlePredictedIssues(predictions *PerformancePredictions) {
	for _, prediction := range predictions.Predictions {
		if prediction.Confidence > 0.8 && prediction.TimeToIssue < time.Minute*30 {
			// Create predictive alert
			alert := &Alert{
				ID:          generateAlertID(),
				Type:        AlertTypePredictive,
				Severity:    SeverityWarning,
				Title:       fmt.Sprintf("Predicted %s issue", prediction.Type),
				Description: prediction.Description,
				Timestamp:   time.Now(),
				Source:      "PredictiveAnalytics",
				Metadata: map[string]interface{}{
					"confidence":     prediction.Confidence,
					"time_to_issue":  prediction.TimeToIssue,
					"predicted_type": prediction.Type,
				},
			}

			pm.alertManager.AddAlert(alert)
		}
	}
}

// generateAlertID generates a unique alert ID.
func generateAlertID() string {
	return fmt.Sprintf("alert_%d", time.Now().UnixNano())
}
