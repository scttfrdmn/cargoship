package monitoring

import (
	"context"
	"fmt"
	"sync"
	"time"
)

// AlertManager manages performance alerts and notifications.
type AlertManager struct {
	config       *MonitoringConfig
	activeAlerts []*Alert
	alertHistory []*Alert
	notifiers    []Notifier
	mu           sync.RWMutex
	isRunning    bool
}

// NewAlertManager creates a new alert manager.
func NewAlertManager(config *MonitoringConfig) *AlertManager {
	am := &AlertManager{
		config:       config,
		activeAlerts: make([]*Alert, 0),
		alertHistory: make([]*Alert, 0),
		notifiers:    make([]Notifier, 0),
	}
	
	// Initialize notifiers based on config
	if config.WebhookConfig != nil && config.WebhookConfig.Enabled {
		am.notifiers = append(am.notifiers, NewWebhookNotifier(config.WebhookConfig))
	}
	
	return am
}

// Start begins alert management.
func (am *AlertManager) Start(ctx context.Context) error {
	am.mu.Lock()
	defer am.mu.Unlock()
	
	if am.isRunning {
		return nil
	}
	
	am.isRunning = true
	return nil
}

// Stop stops alert management.
func (am *AlertManager) Stop() {
	am.mu.Lock()
	defer am.mu.Unlock()
	am.isRunning = false
}

// GetActiveAlerts returns current active alerts.
func (am *AlertManager) GetActiveAlerts() []*Alert {
	am.mu.RLock()
	defer am.mu.RUnlock()
	
	// Return copies
	alerts := make([]*Alert, len(am.activeAlerts))
	for i, alert := range am.activeAlerts {
		alertCopy := *alert
		alerts[i] = &alertCopy
	}
	return alerts
}

// AddAlert adds a new alert.
func (am *AlertManager) AddAlert(alert *Alert) {
	am.mu.Lock()
	defer am.mu.Unlock()
	
	// Check if similar alert already exists
	for _, existing := range am.activeAlerts {
		if am.isSimilarAlert(existing, alert) {
			existing.LastSeen = time.Now()
			existing.Count++
			return
		}
	}
	
	// Add new alert
	alert.ID = am.generateAlertID()
	alert.Count = 1
	alert.LastSeen = alert.Timestamp
	am.activeAlerts = append(am.activeAlerts, alert)
	
	// Notify
	am.notifyAlert(alert)
}

// EvaluateMetrics evaluates metrics against thresholds and returns new alerts.
func (am *AlertManager) EvaluateMetrics(metrics *PerformanceMetrics, thresholds *DynamicThresholds) []*Alert {
	am.mu.Lock()
	defer am.mu.Unlock()
	
	var newAlerts []*Alert
	
	// Evaluate transfer metrics
	if metrics.TransferMetrics != nil {
		if alerts := am.evaluateTransferMetrics(metrics.TransferMetrics, thresholds); len(alerts) > 0 {
			newAlerts = append(newAlerts, alerts...)
		}
	}
	
	// Evaluate system metrics
	if metrics.SystemMetrics != nil {
		if alerts := am.evaluateSystemMetrics(metrics.SystemMetrics, thresholds); len(alerts) > 0 {
			newAlerts = append(newAlerts, alerts...)
		}
	}
	
	// Evaluate network metrics
	if metrics.NetworkMetrics != nil {
		if alerts := am.evaluateNetworkMetrics(metrics.NetworkMetrics, thresholds); len(alerts) > 0 {
			newAlerts = append(newAlerts, alerts...)
		}
	}
	
	// Evaluate S3 metrics
	if metrics.S3Metrics != nil {
		if alerts := am.evaluateS3Metrics(metrics.S3Metrics, thresholds); len(alerts) > 0 {
			newAlerts = append(newAlerts, alerts...)
		}
	}
	
	// Add all new alerts
	for _, alert := range newAlerts {
		am.AddAlert(alert)
	}
	
	return newAlerts
}

// evaluateTransferMetrics evaluates transfer metrics against thresholds.
func (am *AlertManager) evaluateTransferMetrics(metrics *TransferMetrics, thresholds *DynamicThresholds) []*Alert {
	var alerts []*Alert
	
	// Low throughput alert
	if thresholds.MinThroughputMBps > 0 && metrics.TotalThroughputMBps < thresholds.MinThroughputMBps {
		alerts = append(alerts, &Alert{
			Type:        AlertTypeLowThroughput,
			Severity:    am.calculateSeverity(metrics.TotalThroughputMBps, thresholds.MinThroughputMBps, false),
			Title:       "Low Transfer Throughput",
			Description: fmt.Sprintf("Transfer throughput %.2f MB/s is below threshold %.2f MB/s", 
				metrics.TotalThroughputMBps, thresholds.MinThroughputMBps),
			Timestamp:   time.Now(),
			Source:      "TransferMonitor",
			Metadata: map[string]interface{}{
				"current_throughput": metrics.TotalThroughputMBps,
				"threshold":          thresholds.MinThroughputMBps,
				"active_transfers":   metrics.ActiveTransfers,
			},
		})
	}
	
	// High latency alert
	if thresholds.MaxLatencyMs > 0 && metrics.AverageLatencyMs > thresholds.MaxLatencyMs {
		alerts = append(alerts, &Alert{
			Type:        AlertTypeHighLatency,
			Severity:    am.calculateSeverity(metrics.AverageLatencyMs, thresholds.MaxLatencyMs, true),
			Title:       "High Transfer Latency",
			Description: fmt.Sprintf("Average latency %.2f ms exceeds threshold %.2f ms", 
				metrics.AverageLatencyMs, thresholds.MaxLatencyMs),
			Timestamp:   time.Now(),
			Source:      "TransferMonitor",
			Metadata: map[string]interface{}{
				"current_latency": metrics.AverageLatencyMs,
				"threshold":       thresholds.MaxLatencyMs,
			},
		})
	}
	
	// High error rate alert
	if thresholds.MaxErrorRate > 0 && (1.0-metrics.SuccessRate) > thresholds.MaxErrorRate {
		errorRate := 1.0 - metrics.SuccessRate
		alerts = append(alerts, &Alert{
			Type:        AlertTypeHighErrorRate,
			Severity:    am.calculateSeverity(errorRate, thresholds.MaxErrorRate, true),
			Title:       "High Transfer Error Rate",
			Description: fmt.Sprintf("Error rate %.2f%% exceeds threshold %.2f%%", 
				errorRate*100, thresholds.MaxErrorRate*100),
			Timestamp:   time.Now(),
			Source:      "TransferMonitor",
			Metadata: map[string]interface{}{
				"error_rate":   errorRate,
				"threshold":    thresholds.MaxErrorRate,
				"error_count":  metrics.ErrorCount,
				"success_rate": metrics.SuccessRate,
			},
		})
	}
	
	return alerts
}

// evaluateSystemMetrics evaluates system metrics against thresholds.
func (am *AlertManager) evaluateSystemMetrics(metrics *SystemMetrics, thresholds *DynamicThresholds) []*Alert {
	var alerts []*Alert
	
	// High CPU usage alert
	if thresholds.MaxCPUUsage > 0 && metrics.CPUUsagePercent > thresholds.MaxCPUUsage*100 {
		alerts = append(alerts, &Alert{
			Type:        AlertTypeHighCPUUsage,
			Severity:    am.calculateSeverity(metrics.CPUUsagePercent/100, thresholds.MaxCPUUsage, true),
			Title:       "High CPU Usage",
			Description: fmt.Sprintf("CPU usage %.1f%% exceeds threshold %.1f%%", 
				metrics.CPUUsagePercent, thresholds.MaxCPUUsage*100),
			Timestamp:   time.Now(),
			Source:      "SystemMonitor",
			Metadata: map[string]interface{}{
				"cpu_usage":  metrics.CPUUsagePercent,
				"threshold":  thresholds.MaxCPUUsage * 100,
			},
		})
	}
	
	// High memory usage alert
	if thresholds.MaxMemoryUsage > 0 && metrics.MemoryUsagePercent > thresholds.MaxMemoryUsage*100 {
		alerts = append(alerts, &Alert{
			Type:        AlertTypeHighMemoryUsage,
			Severity:    am.calculateSeverity(metrics.MemoryUsagePercent/100, thresholds.MaxMemoryUsage, true),
			Title:       "High Memory Usage",
			Description: fmt.Sprintf("Memory usage %.1f%% exceeds threshold %.1f%%", 
				metrics.MemoryUsagePercent, thresholds.MaxMemoryUsage*100),
			Timestamp:   time.Now(),
			Source:      "SystemMonitor",
			Metadata: map[string]interface{}{
				"memory_usage_percent": metrics.MemoryUsagePercent,
				"memory_usage_mb":      metrics.MemoryUsageMB,
				"threshold":            thresholds.MaxMemoryUsage * 100,
			},
		})
	}
	
	return alerts
}

// evaluateNetworkMetrics evaluates network metrics against thresholds.
func (am *AlertManager) evaluateNetworkMetrics(metrics *NetworkMetrics, thresholds *DynamicThresholds) []*Alert {
	var alerts []*Alert
	
	// High network latency
	if thresholds.MaxLatencyMs > 0 && metrics.LatencyMs > thresholds.MaxLatencyMs {
		alerts = append(alerts, &Alert{
			Type:        AlertTypeHighLatency,
			Severity:    am.calculateSeverity(metrics.LatencyMs, thresholds.MaxLatencyMs, true),
			Title:       "High Network Latency",
			Description: fmt.Sprintf("Network latency %.2f ms exceeds threshold %.2f ms", 
				metrics.LatencyMs, thresholds.MaxLatencyMs),
			Timestamp:   time.Now(),
			Source:      "NetworkMonitor",
			Metadata: map[string]interface{}{
				"latency":   metrics.LatencyMs,
				"threshold": thresholds.MaxLatencyMs,
			},
		})
	}
	
	// High packet loss
	if thresholds.MaxPacketLoss > 0 && metrics.PacketLossPercent > thresholds.MaxPacketLoss {
		alerts = append(alerts, &Alert{
			Type:        AlertTypeHighPacketLoss,
			Severity:    am.calculateSeverity(metrics.PacketLossPercent, thresholds.MaxPacketLoss, true),
			Title:       "High Packet Loss",
			Description: fmt.Sprintf("Packet loss %.2f%% exceeds threshold %.2f%%", 
				metrics.PacketLossPercent*100, thresholds.MaxPacketLoss*100),
			Timestamp:   time.Now(),
			Source:      "NetworkMonitor",
			Metadata: map[string]interface{}{
				"packet_loss": metrics.PacketLossPercent,
				"threshold":   thresholds.MaxPacketLoss,
			},
		})
	}
	
	return alerts
}

// evaluateS3Metrics evaluates S3 metrics against thresholds.
func (am *AlertManager) evaluateS3Metrics(metrics *S3Metrics, thresholds *DynamicThresholds) []*Alert {
	var alerts []*Alert
	
	// High S3 error rate
	if thresholds.MaxS3ErrorRate > 0 && metrics.ErrorRate > thresholds.MaxS3ErrorRate {
		alerts = append(alerts, &Alert{
			Type:        AlertTypeS3Errors,
			Severity:    am.calculateSeverity(metrics.ErrorRate, thresholds.MaxS3ErrorRate, true),
			Title:       "High S3 Error Rate",
			Description: fmt.Sprintf("S3 error rate %.2f%% exceeds threshold %.2f%%", 
				metrics.ErrorRate*100, thresholds.MaxS3ErrorRate*100),
			Timestamp:   time.Now(),
			Source:      "S3Monitor",
			Metadata: map[string]interface{}{
				"error_rate":         metrics.ErrorRate,
				"threshold":          thresholds.MaxS3ErrorRate,
				"failed_requests":    metrics.FailedRequests,
				"successful_requests": metrics.SuccessfulRequests,
			},
		})
	}
	
	return alerts
}

// calculateSeverity calculates alert severity based on threshold breach.
func (am *AlertManager) calculateSeverity(current, threshold float64, isHigher bool) AlertSeverity {
	var ratio float64
	if isHigher {
		ratio = current / threshold
	} else {
		ratio = threshold / current
	}
	
	if ratio >= 2.0 {
		return SeverityCritical
	} else if ratio >= 1.5 {
		return SeverityWarning
	}
	return SeverityInfo
}

// isSimilarAlert checks if two alerts are similar.
func (am *AlertManager) isSimilarAlert(a1, a2 *Alert) bool {
	return a1.Type == a2.Type && a1.Source == a2.Source
}

// generateAlertID generates a unique alert ID.
func (am *AlertManager) generateAlertID() string {
	return fmt.Sprintf("alert_%d", time.Now().UnixNano())
}

// notifyAlert sends notifications for an alert.
func (am *AlertManager) notifyAlert(alert *Alert) {
	for _, notifier := range am.notifiers {
		go func(n Notifier) {
			if err := n.Notify(alert); err != nil {
				// Log error but don't fail
				// In a real implementation, would log to a proper logger
				_ = err // Acknowledge the error for static analysis
			}
		}(notifier)
	}
}

// Alert represents a performance alert.
type Alert struct {
	ID          string                 `json:"id"`
	Type        AlertType             `json:"type"`
	Severity    AlertSeverity         `json:"severity"`
	Title       string                `json:"title"`
	Description string                `json:"description"`
	Timestamp   time.Time             `json:"timestamp"`
	LastSeen    time.Time             `json:"last_seen"`
	Source      string                `json:"source"`
	Count       int                   `json:"count"`
	Resolved    bool                  `json:"resolved"`
	Metadata    map[string]interface{} `json:"metadata"`
}

// AlertType defines types of performance alerts.
type AlertType int

const (
	AlertTypeLowThroughput AlertType = iota
	AlertTypeHighLatency
	AlertTypeHighErrorRate
	AlertTypeHighCPUUsage
	AlertTypeHighMemoryUsage
	AlertTypeHighDiskUsage
	AlertTypeHighPacketLoss
	AlertTypeS3Errors
	AlertTypePredictive
)

// AlertSeverity defines alert severity levels.
type AlertSeverity int

const (
	SeverityInfo AlertSeverity = iota
	SeverityWarning
	SeverityCritical
)

// Notifier interface for alert notifications.
type Notifier interface {
	Notify(alert *Alert) error
}

// WebhookNotifier sends alerts via webhook.
type WebhookNotifier struct {
	config *WebhookConfig
}

// NewWebhookNotifier creates a new webhook notifier.
func NewWebhookNotifier(config *WebhookConfig) *WebhookNotifier {
	return &WebhookNotifier{
		config: config,
	}
}

// Notify sends an alert notification via webhook.
func (wn *WebhookNotifier) Notify(alert *Alert) error {
	// Implementation would send HTTP POST to webhook URL
	// For now, just return nil
	return nil
}