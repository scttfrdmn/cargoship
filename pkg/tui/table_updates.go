package tui

import (
	"fmt"
	"time"

	"github.com/charmbracelet/bubbles/table"
)

// updateArchivalTables updates archival-related tables
func (d *Dashboard) updateArchivalTables() {
	d.updateArchivalQueue()
	d.updateEstimateTable()
	d.updateSurveyTable()
}

// updateInventoryTables updates inventory-related tables
func (d *Dashboard) updateInventoryTables() {
	d.updateInventoryTree()
	d.updateSearchResults()
	d.updateRestoreQueue()
}

// updateCostTables updates cost-related tables
func (d *Dashboard) updateCostTables() {
	d.updateCostBreakdown()
	d.updateOptimizationsTable()
	d.updateBudgetChart()
}

// updateConfigTables updates configuration-related tables
func (d *Dashboard) updateConfigTables() {
	d.updateConfigTable()
	d.updateProfilesList()
}

// updateArchivalQueue updates the archival queue table
func (d *Dashboard) updateArchivalQueue() {
	var rows []table.Row
	for _, job := range d.archivalJobs {
		progress := fmt.Sprintf("%.1f%%", job.Progress)
		rows = append(rows, table.Row{
			job.ID[:8] + "...",
			job.Source,
			job.Destination,
			job.Status,
			progress,
			job.EstimatedCost,
		})
	}
	d.archivalQueue.SetRows(rows)
}

// updateEstimateTable updates the cost estimate table
func (d *Dashboard) updateEstimateTable() {
	// Mock data for estimates
	rows := []table.Row{
		{"/data/research-1", "2.5GB", "Standard", "$5.75", "$69.00"},
		{"/data/analysis", "1.2TB", "Glacier", "$61.44", "$737.28"},
		{"/data/backup", "500GB", "Deep Archive", "$5.00", "$60.00"},
		{"/data/logs", "100GB", "IA", "$2.50", "$30.00"},
	}
	d.estimateTable.SetRows(rows)
}

// updateSurveyTable updates the survey results table
func (d *Dashboard) updateSurveyTable() {
	// Mock data for survey results
	rows := []table.Row{
		{"/research/genomics", "1,245", "2.5TB", "2024-06-15", "Deep Archive"},
		{"/analysis/stats", "89", "150GB", "2024-07-01", "Glacier"},
		{"/backup/daily", "3,421", "890GB", "2024-07-12", "Standard"},
		{"/logs/system", "12,567", "45GB", "2024-07-13", "IA"},
	}
	d.surveyResults.SetRows(rows)
}

// updateInventoryTree updates the inventory tree table
func (d *Dashboard) updateInventoryTree() {
	var rows []table.Row
	for _, item := range d.inventoryItems {
		rows = append(rows, table.Row{
			item.Path,
			item.Type,
			item.Size,
			item.StorageClass,
			item.Cost,
		})
	}

	// Add some mock data if no real data
	if len(rows) == 0 {
		rows = []table.Row{
			{"s3://research-bucket/genomics/", "folder", "2.5TB", "Deep Archive", "$12.29/mo"},
			{"s3://research-bucket/analysis.tar.gz", "archive", "150GB", "Glacier", "$9.60/mo"},
			{"s3://research-bucket/backup/", "folder", "890GB", "Standard", "$20.48/mo"},
			{"s3://research-bucket/logs.zip", "archive", "45GB", "IA", "$2.88/mo"},
		}
	}

	d.inventoryTree.SetRows(rows)
}

// updateSearchResults updates the search results table
func (d *Dashboard) updateSearchResults() {
	// Mock search results
	rows := []table.Row{
		{"analysis.csv", "s3://bucket/data/", "25MB", "2024-07-10"},
		{"results.json", "s3://bucket/output/", "5MB", "2024-07-12"},
		{"genome.fasta", "s3://bucket/raw/", "1.2GB", "2024-06-20"},
		{"metadata.xml", "s3://bucket/meta/", "2MB", "2024-07-13"},
	}
	d.searchResults.SetRows(rows)
}

// updateRestoreQueue updates the restore queue table
func (d *Dashboard) updateRestoreQueue() {
	// Mock restore operations
	rows := []table.Row{
		{"analysis.tar.gz", "In Progress", "75%", "5 min", "$2.50"},
		{"backup.zip", "Queued", "0%", "2 hours", "$5.00"},
		{"logs.tar.gz", "Completed", "100%", "0 min", "$1.25"},
	}
	d.restoreQueue.SetRows(rows)
}

// updateCostBreakdown updates the cost breakdown table
func (d *Dashboard) updateCostBreakdown() {
	// Mock cost breakdown data
	rows := []table.Row{
		{"Storage", "$89.50", "65%", "↑ 5%"},
		{"Transfer", "$24.30", "18%", "↓ 2%"},
		{"Requests", "$12.20", "9%", "→ 0%"},
		{"Lifecycle", "$8.75", "6%", "↓ 10%"},
		{"Other", "$3.25", "2%", "↑ 15%"},
	}
	d.costBreakdown.SetRows(rows)
}

// updateOptimizationsTable updates the optimization suggestions table
func (d *Dashboard) updateOptimizationsTable() {
	// Mock optimization suggestions
	rows := []table.Row{
		{"Lifecycle Policies", "$50/month", "High", "Low"},
		{"Storage Class Optimization", "$35/month", "Medium", "Medium"},
		{"Transfer Optimization", "$20/month", "Low", "High"},
		{"Request Optimization", "$15/month", "Low", "Low"},
	}
	d.optimizations.SetRows(rows)
}

// updateBudgetChart updates the budget tracking table
func (d *Dashboard) updateBudgetChart() {
	// Mock budget data
	rows := []table.Row{
		{"Monthly Storage", "$150.00", "$89.50", "$60.50", "18"},
		{"Transfer Budget", "$50.00", "$24.30", "$25.70", "18"},
		{"Request Budget", "$25.00", "$12.20", "$12.80", "18"},
		{"Total Budget", "$225.00", "$126.00", "$99.00", "18"},
	}
	d.budgetChart.SetRows(rows)
}

// updateConfigTable updates the configuration table
func (d *Dashboard) updateConfigTable() {
	var rows []table.Row
	for _, config := range d.configurations {
		rows = append(rows, table.Row{
			config.Key,
			config.Value,
			config.Source,
			config.Description,
		})
	}

	// Add some mock data if no real data
	if len(rows) == 0 {
		rows = []table.Row{
			{"default_storage_class", "GLACIER", "config", "Default storage class for archives"},
			{"max_file_size", "5GB", "env", "Maximum file size for single upload"},
			{"aws_region", "us-east-1", "aws", "Default AWS region"},
			{"compression", "zstd", "config", "Default compression algorithm"},
			{"encryption", "AES256", "config", "Default encryption method"},
		}
	}

	d.configTable.SetRows(rows)
}

// updateProfilesList updates the profiles list table
func (d *Dashboard) updateProfilesList() {
	// Mock profiles data
	rows := []table.Row{
		{"production", "✓", "Production environment settings", "2024-07-13"},
		{"development", "", "Development environment settings", "2024-07-10"},
		{"research", "", "Research-optimized settings", "2024-07-08"},
		{"backup", "", "Backup-specific configuration", "2024-07-05"},
	}
	d.profilesList.SetRows(rows)
}

// Mock data functions for testing

func (d *Dashboard) fetchMockArchivalJobs() []ArchivalJob {
	return []ArchivalJob{
		{
			ID:            "arch-001",
			Source:        "/data/research/genomics",
			Destination:   "s3://research-bucket/genomics",
			Status:        "running",
			Progress:      75.5,
			StartTime:     time.Now().Add(-2 * time.Hour),
			EstimatedCost: "$12.50",
			StorageClass:  "GLACIER",
			Size:          "2.5TB",
			Rate:          "150MB/s",
		},
		{
			ID:            "arch-002",
			Source:        "/data/backup/daily",
			Destination:   "s3://backup-bucket/daily",
			Status:        "pending",
			Progress:      0,
			StartTime:     time.Now(),
			EstimatedCost: "$5.75",
			StorageClass:  "DEEP_ARCHIVE",
			Size:          "890GB",
			Rate:          "0MB/s",
		},
	}
}

func (d *Dashboard) fetchMockInventoryItems() []InventoryItem {
	return []InventoryItem{
		{
			Path:         "s3://research-bucket/genomics/",
			Type:         "folder",
			Size:         "2.5TB",
			LastModified: time.Now().Add(-24 * time.Hour),
			StorageClass: "DEEP_ARCHIVE",
			Cost:         "$12.29/month",
			Metadata:     map[string]string{"project": "genomics-2024"},
		},
		{
			Path:         "s3://research-bucket/analysis.tar.gz",
			Type:         "archive",
			Size:         "150GB",
			LastModified: time.Now().Add(-12 * time.Hour),
			StorageClass: "GLACIER",
			Cost:         "$9.60/month",
			Metadata:     map[string]string{"created": "cargoship"},
		},
	}
}

func (d *Dashboard) fetchMockConfigurations() []ConfigItem {
	return []ConfigItem{
		{
			Key:         "default_storage_class",
			Value:       "GLACIER",
			Type:        "string",
			Description: "Default storage class for archives",
			Default:     "STANDARD",
			Source:      "config",
		},
		{
			Key:         "max_file_size",
			Value:       "5GB",
			Type:        "size",
			Description: "Maximum file size for single upload",
			Default:     "5GB",
			Source:      "env",
		},
	}
}

// updateMultiRegionTables updates all multi-region related tables
func (d *Dashboard) updateMultiRegionTables() {
	d.updateRegionOverviewTable()
	d.updateRegionHealthTable()
	d.updateRegionMetricsTable()
	d.updateFailoverStatusTable()
}

// updateRegionOverviewTable updates the region overview table
func (d *Dashboard) updateRegionOverviewTable() {
	var rows []table.Row

	// Use mock data for now - in production this would come from the multi-region coordinator
	mockData := d.fetchMockRegionStatus()

	for _, region := range mockData {
		healthScore := fmt.Sprintf("%.1f%%", region.Health.SuccessRate*100)
		errorRate := fmt.Sprintf("%.2f%%", region.Metrics.ErrorRate)
		lastCheck := region.LastChecked.Format("15:04:05")

		rows = append(rows, table.Row{
			region.Name,
			region.Status,
			fmt.Sprintf("%d", region.Priority),
			fmt.Sprintf("%d", region.Weight),
			healthScore,
			region.Metrics.Throughput,
			errorRate,
			lastCheck,
		})
	}

	d.regionOverviewTable.SetRows(rows)
}

// updateRegionHealthTable updates the region health monitoring table
func (d *Dashboard) updateRegionHealthTable() {
	var rows []table.Row

	mockData := d.fetchMockRegionStatus()

	for _, region := range mockData {
		healthStatus := "Healthy"
		if !region.Health.OverallHealthy {
			healthStatus = "Issues"
		}

		successRate := fmt.Sprintf("%.1f%%", region.Health.SuccessRate*100)
		latency := region.Health.HealthCheckLatency.String()
		issues := ""
		if len(region.Health.FailureReasons) > 0 {
			issues = region.Health.FailureReasons[0] // Show first issue
		}

		rows = append(rows, table.Row{
			region.Name,
			healthStatus,
			successRate,
			fmt.Sprintf("%d", region.Health.ConsecutiveSuccesses),
			fmt.Sprintf("%d", region.Health.ConsecutiveFailures),
			latency,
			issues,
		})
	}

	d.regionHealthTable.SetRows(rows)
}

// updateRegionMetricsTable updates the region metrics table
func (d *Dashboard) updateRegionMetricsTable() {
	var rows []table.Row

	mockData := d.fetchMockRegionStatus()

	for _, region := range mockData {
		avgLatency := region.Metrics.AverageLatency.String()
		cpuUsage := fmt.Sprintf("%.1f%%", region.Metrics.CPUUtilization)
		memUsage := fmt.Sprintf("%.1f%%", region.Metrics.MemoryUtilization)
		storageUsage := fmt.Sprintf("%.1f%%", region.Metrics.StorageUtilization)

		rows = append(rows, table.Row{
			region.Name,
			avgLatency,
			region.Metrics.Throughput,
			fmt.Sprintf("%d", region.Metrics.ActiveUploads),
			cpuUsage,
			memUsage,
			storageUsage,
			region.Metrics.BandwidthUsage,
		})
	}

	d.regionMetricsTable.SetRows(rows)
}

// updateFailoverStatusTable updates the failover operations table
func (d *Dashboard) updateFailoverStatusTable() {
	var rows []table.Row

	mockData := d.fetchMockFailoverOperations()

	for _, failover := range mockData {
		duration := ""
		if !failover.CompletedTime.IsZero() {
			duration = failover.Duration.String()
		} else if failover.Status == "in_progress" {
			duration = time.Since(failover.StartTime).String()
		}

		// Truncate long operation IDs and reasons for display
		opID := failover.ID
		if len(opID) > 14 {
			opID = opID[:14] + ".."
		}

		reason := failover.Reason
		if len(reason) > 18 {
			reason = reason[:18] + ".."
		}

		rows = append(rows, table.Row{
			opID,
			failover.FromRegion,
			failover.ToRegion,
			failover.Strategy,
			failover.Status,
			duration,
			failover.TriggerType,
			reason,
		})
	}

	d.failoverStatusTable.SetRows(rows)
}

// fetchMockRegionStatus returns mock region status data
func (d *Dashboard) fetchMockRegionStatus() []RegionStatusInfo {
	return []RegionStatusInfo{
		{
			Name:        "us-east-1",
			Status:      "healthy",
			Priority:    1,
			Weight:      80,
			LastChecked: time.Now().Add(-30 * time.Second),
			Health: RegionHealthInfo{
				OverallHealthy:       true,
				SuccessRate:          0.98,
				ConsecutiveSuccesses: 45,
				ConsecutiveFailures:  0,
				LastHealthCheck:      time.Now().Add(-30 * time.Second),
				HealthCheckLatency:   25 * time.Millisecond,
				FailureReasons:       []string{},
			},
			Metrics: RegionMetricsInfo{
				AverageLatency:     80 * time.Millisecond,
				Throughput:         "125.3 MB/s",
				ErrorRate:          2.1,
				ActiveUploads:      23,
				SuccessfulUploads:  1847,
				FailedUploads:      39,
				CPUUtilization:     34.5,
				MemoryUtilization:  67.2,
				StorageUtilization: 45.8,
				BandwidthUsage:     "89.2 MB/s",
				LastUpdated:        time.Now().Add(-15 * time.Second),
			},
		},
		{
			Name:        "us-west-2",
			Status:      "healthy",
			Priority:    2,
			Weight:      70,
			LastChecked: time.Now().Add(-45 * time.Second),
			Health: RegionHealthInfo{
				OverallHealthy:       true,
				SuccessRate:          0.96,
				ConsecutiveSuccesses: 38,
				ConsecutiveFailures:  0,
				LastHealthCheck:      time.Now().Add(-45 * time.Second),
				HealthCheckLatency:   32 * time.Millisecond,
				FailureReasons:       []string{},
			},
			Metrics: RegionMetricsInfo{
				AverageLatency:     95 * time.Millisecond,
				Throughput:         "98.7 MB/s",
				ErrorRate:          3.4,
				ActiveUploads:      18,
				SuccessfulUploads:  1523,
				FailedUploads:      54,
				CPUUtilization:     28.1,
				MemoryUtilization:  59.8,
				StorageUtilization: 52.3,
				BandwidthUsage:     "73.4 MB/s",
				LastUpdated:        time.Now().Add(-20 * time.Second),
			},
		},
		{
			Name:        "eu-west-1",
			Status:      "degraded",
			Priority:    3,
			Weight:      50,
			LastChecked: time.Now().Add(-60 * time.Second),
			Health: RegionHealthInfo{
				OverallHealthy:       false,
				SuccessRate:          0.85,
				ConsecutiveSuccesses: 5,
				ConsecutiveFailures:  3,
				LastHealthCheck:      time.Now().Add(-60 * time.Second),
				HealthCheckLatency:   150 * time.Millisecond,
				FailureReasons:       []string{"high_latency", "intermittent_connectivity"},
			},
			Metrics: RegionMetricsInfo{
				AverageLatency:     180 * time.Millisecond,
				Throughput:         "45.2 MB/s",
				ErrorRate:          8.7,
				ActiveUploads:      8,
				SuccessfulUploads:  892,
				FailedUploads:      87,
				CPUUtilization:     52.3,
				MemoryUtilization:  78.9,
				StorageUtilization: 61.4,
				BandwidthUsage:     "31.8 MB/s",
				LastUpdated:        time.Now().Add(-35 * time.Second),
			},
		},
	}
}

// fetchMockFailoverOperations returns mock failover operations data
func (d *Dashboard) fetchMockFailoverOperations() []FailoverOperation {
	return []FailoverOperation{
		{
			ID:            "failover-20240101-001",
			FromRegion:    "eu-west-1",
			ToRegion:      "us-east-1",
			Strategy:      "graceful",
			Status:        "completed",
			StartTime:     time.Now().Add(-5 * time.Minute),
			CompletedTime: time.Now().Add(-3 * time.Minute),
			Duration:      2 * time.Minute,
			Reason:        "High error rate detected",
			TriggerType:   "automatic",
			Success:       true,
		},
		{
			ID:          "failover-20240101-002",
			FromRegion:  "us-west-2",
			ToRegion:    "us-east-1",
			Strategy:    "immediate",
			Status:      "in_progress",
			StartTime:   time.Now().Add(-30 * time.Second),
			Duration:    30 * time.Second,
			Reason:      "Connection timeout",
			TriggerType: "automatic",
			Success:     false,
		},
	}
}

// fetchMockGlobalMetrics returns mock global multi-region metrics
func (d *Dashboard) fetchMockGlobalMetrics() GlobalMetricsInfo {
	now := time.Now()
	return GlobalMetricsInfo{
		TotalRegions:         3,
		HealthyRegions:       2,
		RegionAvailability:   66.7,         // 2 out of 3 regions healthy
		GlobalThroughput:     "269.2 MB/s", // Combined throughput
		AverageLatency:       118 * time.Millisecond,
		TotalUploads:         4262,
		GlobalErrorRate:      4.7,
		SystemHealthScore:    85.3,
		TotalCost:            "$1,247.50/month",
		EstimatedMonthlyCost: "$1,500.00",
		LastUpdated:          now,
	}
}
