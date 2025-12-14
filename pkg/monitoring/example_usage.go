package monitoring

import (
	"context"
	"fmt"
	"strings"
	"time"
)

// ExamplePerformanceMonitor demonstrates how to use the performance monitoring system.
func ExamplePerformanceMonitor() {
	// Create configuration with custom settings
	config := &MonitoringConfig{
		MetricsInterval:       time.Second * 5,
		AlertCheckInterval:    time.Second * 2,
		DashboardInterval:     time.Second * 1,
		MetricsRetention:      time.Hour * 12,
		AlertRetention:        time.Hour * 48,
		MaxMetricsInMemory:    5000,
		EnableRealTimeAlerts:  true,
		EnablePredictive:      true,
		EnableAutoRemediation: false, // Disable for safety in example
		EnableCloudWatch:      false,
		DefaultThresholds: &DefaultThresholds{
			MinThroughputMBps: 2.0,
			MaxTransferTime:   time.Minute * 30,
			MaxErrorRate:      0.03,
			MaxCPUUsage:       0.85,
			MaxMemoryUsage:    0.90,
			MaxDiskUsage:      0.95,
			MaxLatencyMs:      800,
			MinReliability:    0.96,
			MaxPacketLoss:     0.005,
			MaxS3Errors:       5,
			MinS3Availability: 0.995,
		},
	}

	// Create and start the performance monitor
	monitor := NewPerformanceMonitor(config)

	if err := monitor.Start(); err != nil {
		fmt.Printf("Failed to start performance monitor: %v\n", err)
		return
	}
	defer func() { _ = monitor.Stop() }()

	// Register custom metrics
	customMetric := &CustomMetric{
		Name:        "custom_upload_count",
		Description: "Number of custom uploads processed",
		Type:        MetricTypeCounter,
		Unit:        "count",
		DataPoints:  make([]*MetricDataPoint, 0),
		Labels:      map[string]string{"service": "upload_processor"},
		CreatedAt:   time.Now(),
	}

	if err := monitor.RegisterMetric(customMetric); err != nil {
		fmt.Printf("Failed to register custom metric: %v\n", err)
	}

	// Simulate some activity and metric recording
	for i := 0; i < 10; i++ {
		// Record custom metric
		monitor.RecordMetric("custom_upload_count", float64(i+1), map[string]string{
			"batch": fmt.Sprintf("batch_%d", i/3),
		})

		// Get current metrics
		metrics := monitor.GetMetrics()
		fmt.Printf("Iteration %d - Active transfers: %d, Throughput: %.2f MB/s\n",
			i+1, metrics.TransferMetrics.ActiveTransfers, metrics.TransferMetrics.TotalThroughputMBps)

		// Check system health
		health := monitor.GetSystemHealth()
		fmt.Printf("System health: %s - %s\n", getHealthString(health.Status), health.Message)

		// Check for alerts
		alerts := monitor.GetAlerts()
		if len(alerts) > 0 {
			fmt.Printf("Active alerts: %d\n", len(alerts))
			for _, alert := range alerts {
				fmt.Printf("  - %s: %s (Severity: %s)\n",
					alert.Title, alert.Description, getSeverityString(alert.Severity))
			}
		}

		// Get predictions if available
		if predictions := monitor.GetPredictions(); predictions != nil {
			fmt.Printf("Performance predictions available (Confidence: %.2f)\n", predictions.Confidence)
			for _, prediction := range predictions.Predictions {
				if prediction.TimeToIssue > 0 {
					fmt.Printf("  - Predicted %s issue in %v (Value: %.2f, Confidence: %.2f)\n",
						prediction.Type, prediction.TimeToIssue, prediction.Value, prediction.Confidence)
				}
			}
		}

		time.Sleep(time.Second)
	}

	// Demonstrate threshold management
	fmt.Println("\nDemonstrating threshold management...")

	// Set custom threshold
	customThreshold := &AlertThreshold{
		Value:       5.0, // 5 MB/s minimum throughput
		Enabled:     true,
		LastUpdated: time.Now(),
		Source:      "example",
	}

	if err := monitor.SetThreshold("throughput", customThreshold); err != nil {
		fmt.Printf("Failed to set custom threshold: %v\n", err)
	} else {
		fmt.Println("Custom throughput threshold set to 5.0 MB/s")
	}

	// Wait a bit to see if the new threshold triggers any alerts
	time.Sleep(time.Second * 5)

	// Final system status
	fmt.Println("\nFinal system status:")
	finalHealth := monitor.GetSystemHealth()
	fmt.Printf("Overall health: %s - %s\n", getHealthString(finalHealth.Status), finalHealth.Message)

	if finalHealth.SubsystemHealth != nil {
		fmt.Println("Subsystem health:")
		for subsystem, health := range finalHealth.SubsystemHealth {
			fmt.Printf("  %s: %s - %s\n", subsystem, getHealthString(health.Status), health.Message)
		}
	}
}

// ExampleCloudWatchIntegration demonstrates CloudWatch integration.
func ExampleCloudWatchIntegration() {
	// Configuration with CloudWatch enabled
	config := DefaultMonitoringConfig()
	config.EnableCloudWatch = true
	config.CloudWatchConfig = &CloudWatchConfig{
		Enabled:       true,
		Region:        "us-east-1",
		Namespace:     "CargoShip/Performance",
		FlushInterval: time.Minute,
	}

	_ = NewPerformanceMonitor(config)

	// Note: In a real implementation, you would need to provide actual CloudWatch credentials
	// and client configuration
	fmt.Println("CloudWatch integration configured (requires AWS credentials)")
	fmt.Println("Metrics would be published to:", config.CloudWatchConfig.Namespace)
}

// ExampleWebhookAlerts demonstrates webhook alert configuration.
func ExampleWebhookAlerts() {
	config := DefaultMonitoringConfig()
	config.WebhookConfig = &WebhookConfig{
		Enabled: true,
		URL:     "https://hooks.slack.com/services/YOUR/SLACK/WEBHOOK",
		Headers: map[string]string{
			"Content-Type": "application/json",
		},
		Timeout:    time.Second * 10,
		RetryCount: 3,
	}

	_ = NewPerformanceMonitor(config)

	fmt.Println("Webhook alerts configured for URL:", config.WebhookConfig.URL)
	fmt.Println("Alerts will be sent via HTTP POST with JSON payload")
}

// ExampleAdvancedConfiguration demonstrates advanced configuration options.
func ExampleAdvancedConfiguration() {
	config := &MonitoringConfig{
		// Aggressive monitoring for high-throughput scenarios
		MetricsInterval:    time.Millisecond * 500,
		AlertCheckInterval: time.Millisecond * 250,
		DashboardInterval:  time.Millisecond * 100,

		// Extended retention for analysis
		MetricsRetention:   time.Hour * 72,
		AlertRetention:     time.Hour * 168, // 1 week
		MaxMetricsInMemory: 50000,

		// Enable all advanced features
		EnableRealTimeAlerts:  true,
		EnablePredictive:      true,
		EnableAutoRemediation: true, // Enable for automatic issue resolution
		EnableCloudWatch:      true,

		// Strict thresholds for production environment
		DefaultThresholds: &DefaultThresholds{
			MinThroughputMBps: 10.0, // Minimum 10 MB/s
			MaxTransferTime:   time.Minute * 15,
			MaxErrorRate:      0.01,  // 1% max error rate
			MaxCPUUsage:       0.75,  // 75% max CPU
			MaxMemoryUsage:    0.80,  // 80% max memory
			MaxDiskUsage:      0.85,  // 85% max disk
			MaxLatencyMs:      500,   // 500ms max latency
			MinReliability:    0.98,  // 98% min reliability
			MaxPacketLoss:     0.001, // 0.1% max packet loss
			MaxS3Errors:       3,     // Max 3 S3 errors
			MinS3Availability: 0.999, // 99.9% S3 availability
		},

		CloudWatchConfig: &CloudWatchConfig{
			Enabled:       true,
			Region:        "us-east-1",
			Namespace:     "Production/CargoShip",
			FlushInterval: time.Second * 30,
		},

		WebhookConfig: &WebhookConfig{
			Enabled: true,
			URL:     "https://monitoring.company.com/webhooks/cargoship",
			Headers: map[string]string{
				"Content-Type":  "application/json",
				"Authorization": "Bearer YOUR_API_TOKEN",
			},
			Timeout:    time.Second * 5,
			RetryCount: 5,
		},
	}

	_ = NewPerformanceMonitor(config)
	fmt.Println("Advanced monitoring configuration created for production environment")
	fmt.Printf("Metrics interval: %v, Alert interval: %v\n",
		config.MetricsInterval, config.AlertCheckInterval)
	fmt.Printf("Auto-remediation: %v, Predictive: %v\n",
		config.EnableAutoRemediation, config.EnablePredictive)
}

// Helper functions for string representation
func getHealthString(status HealthStatusType) string {
	switch status {
	case HealthHealthy:
		return "Healthy"
	case HealthWarning:
		return "Warning"
	case HealthCritical:
		return "Critical"
	default:
		return "Unknown"
	}
}

func getSeverityString(severity AlertSeverity) string {
	switch severity {
	case SeverityInfo:
		return "Info"
	case SeverityWarning:
		return "Warning"
	case SeverityCritical:
		return "Critical"
	default:
		return "Unknown"
	}
}

// ExampleDashboardUsage demonstrates how to use the dashboard renderer.
func ExampleDashboardUsage() {
	config := DefaultMonitoringConfig()
	monitor := NewPerformanceMonitor(config)

	if err := monitor.Start(); err != nil {
		fmt.Printf("Failed to start monitor: %v\n", err)
		return
	}
	defer func() { _ = monitor.Stop() }()

	// Wait for some metrics to be collected
	time.Sleep(time.Second * 2)

	// Get current metrics and alerts
	metrics := monitor.GetMetrics()
	alerts := monitor.GetAlerts()

	// Create dashboard renderer
	renderer := NewDashboardRenderer(config)
	_ = renderer.Start(context.Background())
	defer renderer.Stop()

	// Render dashboard
	dashboard := renderer.RenderDashboard(metrics, alerts)

	fmt.Println("Dashboard Title:", dashboard.Title)
	fmt.Println("Generated at:", dashboard.GeneratedAt.Format(time.RFC3339))
	fmt.Printf("Sections: %d\n", len(dashboard.Sections))

	for _, section := range dashboard.Sections {
		fmt.Printf("\nSection: %s (%s)\n", section.Title, section.Type)
		for _, widget := range section.Widgets {
			fmt.Printf("  Widget: %s (%s)\n", widget.Title, widget.Type)
			// Print first few lines of content
			lines := strings.Split(widget.Content, "\n")
			for i, line := range lines {
				if i >= 3 { // Limit output
					if len(lines) > 3 {
						fmt.Printf("    ... (%d more lines)\n", len(lines)-3)
					}
					break
				}
				fmt.Printf("    %s\n", line)
			}
		}
	}
}
