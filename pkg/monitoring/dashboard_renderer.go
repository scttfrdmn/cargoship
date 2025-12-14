package monitoring

import (
	"context"
	"fmt"
	"strings"
	"sync"
	"time"
)

// DashboardRenderer renders performance monitoring dashboards.
type DashboardRenderer struct {
	config      *MonitoringConfig
	currentView *DashboardView
	templates   map[string]*DashboardTemplate
	mu          sync.RWMutex
	isRunning   bool
}

// NewDashboardRenderer creates a new dashboard renderer.
func NewDashboardRenderer(config *MonitoringConfig) *DashboardRenderer {
	dr := &DashboardRenderer{
		config:    config,
		templates: make(map[string]*DashboardTemplate),
	}

	dr.initializeTemplates()
	return dr
}

// Start begins dashboard rendering.
func (dr *DashboardRenderer) Start(ctx context.Context) error {
	dr.mu.Lock()
	defer dr.mu.Unlock()

	if dr.isRunning {
		return nil
	}

	dr.isRunning = true
	go dr.renderingLoop(ctx)
	return nil
}

// Stop stops dashboard rendering.
func (dr *DashboardRenderer) Stop() {
	dr.mu.Lock()
	defer dr.mu.Unlock()
	dr.isRunning = false
}

// RenderDashboard renders a dashboard for the given metrics.
func (dr *DashboardRenderer) RenderDashboard(metrics *PerformanceMetrics, alerts []*Alert) *DashboardView {
	dr.mu.Lock()
	defer dr.mu.Unlock()

	view := &DashboardView{
		Title:       "CargoShip Performance Monitor",
		GeneratedAt: time.Now(),
		Sections:    make([]*DashboardSection, 0),
	}

	// Render overview section
	view.Sections = append(view.Sections, dr.renderOverviewSection(metrics, alerts))

	// Render system resources section
	if metrics.SystemMetrics != nil {
		view.Sections = append(view.Sections, dr.renderSystemSection(metrics.SystemMetrics))
	}

	// Render transfer performance section
	if metrics.TransferMetrics != nil {
		view.Sections = append(view.Sections, dr.renderTransferSection(metrics.TransferMetrics))
	}

	// Render network performance section
	if metrics.NetworkMetrics != nil {
		view.Sections = append(view.Sections, dr.renderNetworkSection(metrics.NetworkMetrics))
	}

	// Render S3 performance section
	if metrics.S3Metrics != nil {
		view.Sections = append(view.Sections, dr.renderS3Section(metrics.S3Metrics))
	}

	// Render staging performance section
	if metrics.StagingMetrics != nil {
		view.Sections = append(view.Sections, dr.renderStagingSection(metrics.StagingMetrics))
	}

	// Render alerts section
	if len(alerts) > 0 {
		view.Sections = append(view.Sections, dr.renderAlertsSection(alerts))
	}

	dr.currentView = view
	return view
}

// GetCurrentView returns the current dashboard view.
func (dr *DashboardRenderer) GetCurrentView() *DashboardView {
	dr.mu.RLock()
	defer dr.mu.RUnlock()

	if dr.currentView == nil {
		return nil
	}

	// Return a copy
	view := *dr.currentView
	return &view
}

// renderingLoop runs the dashboard rendering loop.
func (dr *DashboardRenderer) renderingLoop(ctx context.Context) {
	ticker := time.NewTicker(dr.config.DashboardInterval)
	defer ticker.Stop()

	for {
		select {
		case <-ctx.Done():
			return
		case <-ticker.C:
			// Dashboard rendering would be triggered by external calls
			// This loop can handle periodic cleanup or updates
		}
	}
}

// initializeTemplates initializes dashboard templates.
func (dr *DashboardRenderer) initializeTemplates() {
	dr.templates["overview"] = &DashboardTemplate{
		Name:        "overview",
		Title:       "System Overview",
		Description: "Overall system health and performance summary",
	}

	dr.templates["system"] = &DashboardTemplate{
		Name:        "system",
		Title:       "System Resources",
		Description: "CPU, memory, and disk usage",
	}

	dr.templates["transfer"] = &DashboardTemplate{
		Name:        "transfer",
		Title:       "Transfer Performance",
		Description: "Data transfer throughput and latency metrics",
	}

	dr.templates["network"] = &DashboardTemplate{
		Name:        "network",
		Title:       "Network Performance",
		Description: "Network bandwidth, latency, and reliability",
	}
}

// renderOverviewSection renders the system overview section.
func (dr *DashboardRenderer) renderOverviewSection(metrics *PerformanceMetrics, alerts []*Alert) *DashboardSection {
	section := &DashboardSection{
		Title:   "System Overview",
		Type:    "overview",
		Widgets: make([]*DashboardWidget, 0),
	}

	// System health widget
	healthWidget := &DashboardWidget{
		Type:    "health_status",
		Title:   "System Health",
		Content: dr.renderSystemHealth(metrics, alerts),
	}
	section.Widgets = append(section.Widgets, healthWidget)

	// Key metrics widget
	metricsWidget := &DashboardWidget{
		Type:    "key_metrics",
		Title:   "Key Metrics",
		Content: dr.renderKeyMetrics(metrics),
	}
	section.Widgets = append(section.Widgets, metricsWidget)

	// Active alerts widget
	if len(alerts) > 0 {
		alertsWidget := &DashboardWidget{
			Type:    "active_alerts",
			Title:   "Active Alerts",
			Content: dr.renderActiveAlertsWidget(alerts),
		}
		section.Widgets = append(section.Widgets, alertsWidget)
	}

	return section
}

// renderSystemSection renders the system resources section.
func (dr *DashboardRenderer) renderSystemSection(metrics *SystemMetrics) *DashboardSection {
	section := &DashboardSection{
		Title:   "System Resources",
		Type:    "system",
		Widgets: make([]*DashboardWidget, 0),
	}

	// CPU usage widget
	cpuWidget := &DashboardWidget{
		Type:    "gauge",
		Title:   "CPU Usage",
		Content: dr.renderGauge("CPU", metrics.CPUUsagePercent, "%", 80, 90),
	}
	section.Widgets = append(section.Widgets, cpuWidget)

	// Memory usage widget
	memoryWidget := &DashboardWidget{
		Type:    "gauge",
		Title:   "Memory Usage",
		Content: dr.renderGauge("Memory", metrics.MemoryUsagePercent, "%", 85, 90),
	}
	section.Widgets = append(section.Widgets, memoryWidget)

	// Disk usage widget
	diskWidget := &DashboardWidget{
		Type:    "gauge",
		Title:   "Disk Usage",
		Content: dr.renderGauge("Disk", metrics.DiskUsagePercent, "%", 80, 90),
	}
	section.Widgets = append(section.Widgets, diskWidget)

	// Goroutines widget
	goroutineWidget := &DashboardWidget{
		Type:    "metric",
		Title:   "Active Goroutines",
		Content: fmt.Sprintf("%d", metrics.ActiveGoroutines),
	}
	section.Widgets = append(section.Widgets, goroutineWidget)

	return section
}

// renderTransferSection renders the transfer performance section.
func (dr *DashboardRenderer) renderTransferSection(metrics *TransferMetrics) *DashboardSection {
	section := &DashboardSection{
		Title:   "Transfer Performance",
		Type:    "transfer",
		Widgets: make([]*DashboardWidget, 0),
	}

	// Throughput widget
	throughputWidget := &DashboardWidget{
		Type:    "metric",
		Title:   "Total Throughput",
		Content: fmt.Sprintf("%.2f MB/s", metrics.TotalThroughputMBps),
	}
	section.Widgets = append(section.Widgets, throughputWidget)

	// Active transfers widget
	transfersWidget := &DashboardWidget{
		Type:    "metric",
		Title:   "Active Transfers",
		Content: fmt.Sprintf("%d", metrics.ActiveTransfers),
	}
	section.Widgets = append(section.Widgets, transfersWidget)

	// Success rate widget
	successWidget := &DashboardWidget{
		Type:    "gauge",
		Title:   "Success Rate",
		Content: dr.renderGauge("Success", metrics.SuccessRate*100, "%", 95, 98),
	}
	section.Widgets = append(section.Widgets, successWidget)

	// Average latency widget
	latencyWidget := &DashboardWidget{
		Type:    "metric",
		Title:   "Average Latency",
		Content: fmt.Sprintf("%.2f ms", metrics.AverageLatencyMs),
	}
	section.Widgets = append(section.Widgets, latencyWidget)

	return section
}

// renderNetworkSection renders the network performance section.
func (dr *DashboardRenderer) renderNetworkSection(metrics *NetworkMetrics) *DashboardSection {
	section := &DashboardSection{
		Title:   "Network Performance",
		Type:    "network",
		Widgets: make([]*DashboardWidget, 0),
	}

	// Bandwidth widget
	bandwidthWidget := &DashboardWidget{
		Type:    "metric",
		Title:   "Bandwidth",
		Content: fmt.Sprintf("%.2f MB/s", metrics.BandwidthMBps),
	}
	section.Widgets = append(section.Widgets, bandwidthWidget)

	// Latency widget
	latencyWidget := &DashboardWidget{
		Type:    "metric",
		Title:   "Network Latency",
		Content: fmt.Sprintf("%.2f ms", metrics.LatencyMs),
	}
	section.Widgets = append(section.Widgets, latencyWidget)

	// Packet loss widget
	packetLossWidget := &DashboardWidget{
		Type:    "gauge",
		Title:   "Packet Loss",
		Content: dr.renderGauge("Loss", metrics.PacketLossPercent*100, "%", 1, 5),
	}
	section.Widgets = append(section.Widgets, packetLossWidget)

	// Reliability widget
	reliabilityWidget := &DashboardWidget{
		Type:    "gauge",
		Title:   "Reliability",
		Content: dr.renderGauge("Reliability", metrics.ReliabilityScore*100, "%", 95, 98),
	}
	section.Widgets = append(section.Widgets, reliabilityWidget)

	return section
}

// renderS3Section renders the S3 performance section.
func (dr *DashboardRenderer) renderS3Section(metrics *S3Metrics) *DashboardSection {
	section := &DashboardSection{
		Title:   "S3 Performance",
		Type:    "s3",
		Widgets: make([]*DashboardWidget, 0),
	}

	// Request latency widget
	latencyWidget := &DashboardWidget{
		Type:    "metric",
		Title:   "Request Latency",
		Content: fmt.Sprintf("%.2f ms", metrics.RequestLatencyMs),
	}
	section.Widgets = append(section.Widgets, latencyWidget)

	// Success rate widget
	totalRequests := metrics.SuccessfulRequests + metrics.FailedRequests
	successRate := 0.0
	if totalRequests > 0 {
		successRate = float64(metrics.SuccessfulRequests) / float64(totalRequests) * 100
	}
	successWidget := &DashboardWidget{
		Type:    "gauge",
		Title:   "Success Rate",
		Content: dr.renderGauge("Success", successRate, "%", 95, 98),
	}
	section.Widgets = append(section.Widgets, successWidget)

	// Throughput widget
	throughputWidget := &DashboardWidget{
		Type:    "metric",
		Title:   "S3 Throughput",
		Content: fmt.Sprintf("%.2f MB/s", metrics.ThroughputMBps),
	}
	section.Widgets = append(section.Widgets, throughputWidget)

	// Error rate widget
	errorWidget := &DashboardWidget{
		Type:    "gauge",
		Title:   "Error Rate",
		Content: dr.renderGauge("Errors", metrics.ErrorRate*100, "%", 5, 10),
	}
	section.Widgets = append(section.Widgets, errorWidget)

	return section
}

// renderStagingSection renders the staging performance section.
func (dr *DashboardRenderer) renderStagingSection(metrics *StagingMetrics) *DashboardSection {
	section := &DashboardSection{
		Title:   "Staging Performance",
		Type:    "staging",
		Widgets: make([]*DashboardWidget, 0),
	}

	// Active chunks widget
	chunksWidget := &DashboardWidget{
		Type:    "metric",
		Title:   "Active Chunks",
		Content: fmt.Sprintf("%d", metrics.ActiveChunks),
	}
	section.Widgets = append(section.Widgets, chunksWidget)

	// Deduplication rate widget
	dedupeWidget := &DashboardWidget{
		Type:    "gauge",
		Title:   "Deduplication Rate",
		Content: dr.renderGauge("Dedupe", metrics.ChunkDeduplicationRate*100, "%", 10, 20),
	}
	section.Widgets = append(section.Widgets, dedupeWidget)

	// Compression efficiency widget
	compressionWidget := &DashboardWidget{
		Type:    "gauge",
		Title:   "Compression Efficiency",
		Content: dr.renderGauge("Compression", (1.0-metrics.CompressionEfficiency)*100, "%", 30, 50),
	}
	section.Widgets = append(section.Widgets, compressionWidget)

	// Prediction accuracy widget
	accuracyWidget := &DashboardWidget{
		Type:    "gauge",
		Title:   "Prediction Accuracy",
		Content: dr.renderGauge("Accuracy", metrics.PredictionAccuracy*100, "%", 80, 90),
	}
	section.Widgets = append(section.Widgets, accuracyWidget)

	return section
}

// renderAlertsSection renders the alerts section.
func (dr *DashboardRenderer) renderAlertsSection(alerts []*Alert) *DashboardSection {
	section := &DashboardSection{
		Title:   "Active Alerts",
		Type:    "alerts",
		Widgets: make([]*DashboardWidget, 0),
	}

	// Group alerts by severity
	critical := 0
	warning := 0
	info := 0

	for _, alert := range alerts {
		switch alert.Severity {
		case SeverityCritical:
			critical++
		case SeverityWarning:
			warning++
		case SeverityInfo:
			info++
		}
	}

	// Alert counts widget
	countsWidget := &DashboardWidget{
		Type:    "alert_counts",
		Title:   "Alert Summary",
		Content: fmt.Sprintf("Critical: %d, Warning: %d, Info: %d", critical, warning, info),
	}
	section.Widgets = append(section.Widgets, countsWidget)

	// Recent alerts widget
	recentAlertsWidget := &DashboardWidget{
		Type:    "alert_list",
		Title:   "Recent Alerts",
		Content: dr.renderRecentAlerts(alerts),
	}
	section.Widgets = append(section.Widgets, recentAlertsWidget)

	return section
}

// renderSystemHealth renders system health status.
func (dr *DashboardRenderer) renderSystemHealth(metrics *PerformanceMetrics, alerts []*Alert) string {
	healthScore := dr.calculateHealthScore(metrics, alerts)

	var status string
	var color string

	if healthScore >= 90 {
		status = "Healthy"
		color = "green"
	} else if healthScore >= 70 {
		status = "Warning"
		color = "yellow"
	} else {
		status = "Critical"
		color = "red"
	}

	return fmt.Sprintf("[%s] %s (Score: %.1f)", color, status, healthScore)
}

// renderKeyMetrics renders key system metrics.
func (dr *DashboardRenderer) renderKeyMetrics(metrics *PerformanceMetrics) string {
	var lines []string

	if metrics.TransferMetrics != nil {
		lines = append(lines, fmt.Sprintf("Throughput: %.2f MB/s", metrics.TransferMetrics.TotalThroughputMBps))
		lines = append(lines, fmt.Sprintf("Active Transfers: %d", metrics.TransferMetrics.ActiveTransfers))
	}

	if metrics.SystemMetrics != nil {
		lines = append(lines, fmt.Sprintf("CPU: %.1f%%", metrics.SystemMetrics.CPUUsagePercent))
		lines = append(lines, fmt.Sprintf("Memory: %.1f%%", metrics.SystemMetrics.MemoryUsagePercent))
	}

	if metrics.NetworkMetrics != nil {
		lines = append(lines, fmt.Sprintf("Network Latency: %.2f ms", metrics.NetworkMetrics.LatencyMs))
	}

	return strings.Join(lines, "\n")
}

// renderActiveAlertsWidget renders active alerts widget.
func (dr *DashboardRenderer) renderActiveAlertsWidget(alerts []*Alert) string {
	if len(alerts) == 0 {
		return "No active alerts"
	}

	var lines []string
	for i, alert := range alerts {
		if i >= 5 { // Show only first 5 alerts
			lines = append(lines, fmt.Sprintf("... and %d more", len(alerts)-5))
			break
		}

		severity := "INFO"
		switch alert.Severity {
		case SeverityCritical:
			severity = "CRIT"
		case SeverityWarning:
			severity = "WARN"
		}

		lines = append(lines, fmt.Sprintf("[%s] %s", severity, alert.Title))
	}

	return strings.Join(lines, "\n")
}

// renderGauge renders a gauge widget.
func (dr *DashboardRenderer) renderGauge(name string, value float64, unit string, warningThreshold, criticalThreshold float64) string {
	var status string
	var bar string

	if value >= criticalThreshold {
		status = "CRITICAL"
		bar = "████████████████████" // Full red bar
	} else if value >= warningThreshold {
		status = "WARNING"
		bar = "████████████▒▒▒▒▒▒▒▒" // Partial yellow bar
	} else {
		status = "NORMAL"
		barLength := int(value / 100 * 20)
		if barLength > 20 {
			barLength = 20
		}
		bar = strings.Repeat("█", barLength) + strings.Repeat("▒", 20-barLength)
	}

	return fmt.Sprintf("%.1f%s [%s] %s", value, unit, bar, status)
}

// renderRecentAlerts renders recent alerts list.
func (dr *DashboardRenderer) renderRecentAlerts(alerts []*Alert) string {
	if len(alerts) == 0 {
		return "No recent alerts"
	}

	var lines []string
	for i, alert := range alerts {
		if i >= 10 { // Show only 10 most recent
			break
		}

		age := time.Since(alert.Timestamp)
		ageStr := dr.formatDuration(age)

		lines = append(lines, fmt.Sprintf("%s - %s (%s ago)", alert.Title, alert.Source, ageStr))
	}

	return strings.Join(lines, "\n")
}

// calculateHealthScore calculates overall system health score.
func (dr *DashboardRenderer) calculateHealthScore(metrics *PerformanceMetrics, alerts []*Alert) float64 {
	score := 100.0

	// Deduct points for active alerts
	for _, alert := range alerts {
		switch alert.Severity {
		case SeverityCritical:
			score -= 15
		case SeverityWarning:
			score -= 5
		case SeverityInfo:
			score -= 1
		}
	}

	// Deduct points for poor performance
	if metrics.TransferMetrics != nil {
		if metrics.TransferMetrics.SuccessRate < 0.95 {
			score -= (0.95 - metrics.TransferMetrics.SuccessRate) * 100
		}
	}

	if metrics.SystemMetrics != nil {
		if metrics.SystemMetrics.CPUUsagePercent > 80 {
			score -= (metrics.SystemMetrics.CPUUsagePercent - 80) / 2
		}
		if metrics.SystemMetrics.MemoryUsagePercent > 85 {
			score -= (metrics.SystemMetrics.MemoryUsagePercent - 85) / 2
		}
	}

	if score < 0 {
		score = 0
	}

	return score
}

// formatDuration formats a duration in human-readable format.
func (dr *DashboardRenderer) formatDuration(d time.Duration) string {
	if d < time.Minute {
		return fmt.Sprintf("%ds", int(d.Seconds()))
	} else if d < time.Hour {
		return fmt.Sprintf("%dm", int(d.Minutes()))
	} else if d < time.Hour*24 {
		return fmt.Sprintf("%dh", int(d.Hours()))
	}
	return fmt.Sprintf("%dd", int(d.Hours()/24))
}

// DashboardView represents a rendered dashboard view.
type DashboardView struct {
	Title       string              `json:"title"`
	GeneratedAt time.Time           `json:"generated_at"`
	Sections    []*DashboardSection `json:"sections"`
}

// DashboardSection represents a section of the dashboard.
type DashboardSection struct {
	Title   string             `json:"title"`
	Type    string             `json:"type"`
	Widgets []*DashboardWidget `json:"widgets"`
}

// DashboardWidget represents a widget in the dashboard.
type DashboardWidget struct {
	Type    string `json:"type"`
	Title   string `json:"title"`
	Content string `json:"content"`
}

// DashboardTemplate represents a dashboard template.
type DashboardTemplate struct {
	Name        string `json:"name"`
	Title       string `json:"title"`
	Description string `json:"description"`
}
