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