// Package cost provides cost reporting functionality
package cost

import (
	"context"
	"encoding/csv"
	"encoding/json"
	"fmt"
	"log/slog"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"sync"
	"time"

	"github.com/aws/aws-sdk-go-v2/aws"
	"github.com/aws/aws-sdk-go-v2/service/s3"
	"github.com/scttfrdmn/cargoship/pkg/aws/config"
)

// CostReporter handles cost reporting and analytics
type CostReporter struct {
	config     *config.CostReportingConfig
	pricingMgr *PricingManager
	s3Client   *s3.Client
	logger     *slog.Logger
	costs      []CostRecord
	mu         sync.RWMutex
}

// CostRecord represents a single cost record
type CostRecord struct {
	Timestamp       time.Time         `json:"timestamp"`
	Operation       string            `json:"operation"` // "upload", "download", "storage"
	Service         string            `json:"service"`   // "s3", "ec2", etc.
	Region          string            `json:"region"`
	StorageClass    string            `json:"storage_class,omitempty"`
	SizeBytes       int64             `json:"size_bytes"`
	SizeGB          float64           `json:"size_gb"`
	Cost            float64           `json:"cost"`
	OriginalCost    float64           `json:"original_cost"`
	DiscountApplied float64           `json:"discount_applied"`
	Currency        string            `json:"currency"`
	FileName        string            `json:"file_name,omitempty"`
	JobID           string            `json:"job_id,omitempty"`
	ProjectID       string            `json:"project_id,omitempty"` // Manifest UploadID (Issue #147 Phase 2)
	Tags            map[string]string `json:"tags,omitempty"`
}

// CostSummary provides cost summary statistics
type CostSummary struct {
	Period          string               `json:"period"`
	TotalCost       float64              `json:"total_cost"`
	TotalSavings    float64              `json:"total_savings"`
	Currency        string               `json:"currency"`
	ByService       map[string]float64   `json:"by_service"`
	ByRegion        map[string]float64   `json:"by_region"`
	ByStorageClass  map[string]float64   `json:"by_storage_class"`
	ByOperation     map[string]float64   `json:"by_operation"`
	ByProject       map[string]float64   `json:"by_project,omitempty"` // Project-based cost tracking (Issue #147 Phase 2)
	TopFiles        []CostRecord         `json:"top_files"`
	DailyCosts      map[string]float64   `json:"daily_costs"`
	Trends          CostTrends           `json:"trends"`
	Recommendations []CostRecommendation `json:"recommendations"`
}

// CostTrends shows cost trends and projections
type CostTrends struct {
	DailyAverage      float64 `json:"daily_average"`
	WeeklyAverage     float64 `json:"weekly_average"`
	MonthlyProjection float64 `json:"monthly_projection"`
	GrowthRate        float64 `json:"growth_rate"`        // Percentage
	SeasonalVariation float64 `json:"seasonal_variation"` // Percentage
	CostPerGB         float64 `json:"cost_per_gb"`
	CostPerOperation  float64 `json:"cost_per_operation"`
}

// CostRecommendation provides cost optimization recommendations
type CostRecommendation struct {
	Type            string  `json:"type"`     // "storage_class", "region", "lifecycle"
	Priority        string  `json:"priority"` // "high", "medium", "low"
	Description     string  `json:"description"`
	PotentialSaving float64 `json:"potential_saving"`
	Implementation  string  `json:"implementation"`
	Impact          string  `json:"impact"`
}

// NewCostReporter creates a new cost reporter
func NewCostReporter(cfg *config.CostReportingConfig, pricingMgr *PricingManager, s3Client *s3.Client, logger *slog.Logger) *CostReporter {
	if logger == nil {
		logger = slog.Default()
	}

	return &CostReporter{
		config:     cfg,
		pricingMgr: pricingMgr,
		s3Client:   s3Client,
		logger:     logger.With("component", "cost-reporter"),
		costs:      make([]CostRecord, 0),
	}
}

// RecordCost records a cost entry
func (cr *CostReporter) RecordCost(record CostRecord) {
	cr.mu.Lock()
	defer cr.mu.Unlock()

	if record.Timestamp.IsZero() {
		record.Timestamp = time.Now()
	}

	if record.Currency == "" {
		record.Currency = "USD"
	}

	// Calculate size in GB if not provided
	if record.SizeGB == 0 && record.SizeBytes > 0 {
		record.SizeGB = float64(record.SizeBytes) / (1024 * 1024 * 1024)
	}

	cr.costs = append(cr.costs, record)

	cr.logger.Debug("Cost recorded",
		"operation", record.Operation,
		"service", record.Service,
		"cost", record.Cost,
		"size_gb", record.SizeGB)
}

// RecordArchivalCost records cost for an archival operation
func (cr *CostReporter) RecordArchivalCost(ctx context.Context, fileName string, sizeBytes int64, storageClass config.StorageClass, region string, jobID string, projectID string, tags map[string]string) error {
	sizeGB := float64(sizeBytes) / (1024 * 1024 * 1024)

	// Get cost estimate
	estimate, err := cr.pricingMgr.EstimateArchivalCost(ctx, sizeGB, storageClass, region)
	if err != nil {
		return fmt.Errorf("failed to estimate archival cost: %w", err)
	}

	// Record the cost
	record := CostRecord{
		Timestamp:       time.Now(),
		Operation:       "upload",
		Service:         "s3",
		Region:          region,
		StorageClass:    string(storageClass),
		SizeBytes:       sizeBytes,
		SizeGB:          sizeGB,
		Cost:            estimate.TotalCost,
		OriginalCost:    estimate.Discounts.OriginalCost,
		DiscountApplied: estimate.Discounts.TotalDiscount,
		Currency:        estimate.Currency,
		FileName:        filepath.Base(fileName),
		JobID:           jobID,
		ProjectID:       projectID,
		Tags:            tags,
	}

	cr.RecordCost(record)
	return nil
}

// GenerateReport generates a cost report for the specified period
func (cr *CostReporter) GenerateReport(ctx context.Context, period string) (*CostSummary, error) {
	cr.mu.RLock()
	defer cr.mu.RUnlock()

	// Filter costs by period
	startTime, endTime, err := cr.parsePeriod(period)
	if err != nil {
		return nil, fmt.Errorf("invalid period: %w", err)
	}

	filteredCosts := cr.filterCostsByPeriod(startTime, endTime)
	if len(filteredCosts) == 0 {
		return &CostSummary{
			Period:         period,
			Currency:       "USD",
			ByService:      make(map[string]float64),
			ByRegion:       make(map[string]float64),
			ByStorageClass: make(map[string]float64),
			ByOperation:    make(map[string]float64),
			DailyCosts:     make(map[string]float64),
		}, nil
	}

	summary := &CostSummary{
		Period:         period,
		Currency:       filteredCosts[0].Currency,
		ByService:      make(map[string]float64),
		ByRegion:       make(map[string]float64),
		ByStorageClass: make(map[string]float64),
		ByOperation:    make(map[string]float64),
		ByProject:      make(map[string]float64),
		DailyCosts:     make(map[string]float64),
	}

	// Calculate totals and breakdowns
	for _, cost := range filteredCosts {
		summary.TotalCost += cost.Cost
		summary.TotalSavings += cost.DiscountApplied

		summary.ByService[cost.Service] += cost.Cost
		summary.ByRegion[cost.Region] += cost.Cost
		if cost.StorageClass != "" {
			summary.ByStorageClass[cost.StorageClass] += cost.Cost
		}
		summary.ByOperation[cost.Operation] += cost.Cost
		if cost.ProjectID != "" {
			summary.ByProject[cost.ProjectID] += cost.Cost
		}

		// Daily breakdown
		day := cost.Timestamp.Format("2006-01-02")
		summary.DailyCosts[day] += cost.Cost
	}

	// Get top cost files
	summary.TopFiles = cr.getTopCostFiles(filteredCosts, 10)

	// Calculate trends
	summary.Trends = cr.calculateTrends(filteredCosts, startTime, endTime)

	// Generate recommendations
	summary.Recommendations = cr.generateRecommendations(filteredCosts)

	return summary, nil
}

// ExportReport exports a cost report in the specified format
func (cr *CostReporter) ExportReport(ctx context.Context, summary *CostSummary, format string, outputPath string) error {
	switch strings.ToLower(format) {
	case "json":
		return cr.exportJSON(summary, outputPath)
	case "csv":
		return cr.exportCSV(summary, outputPath)
	default:
		return fmt.Errorf("unsupported export format: %s", format)
	}
}

// UploadReportToS3 uploads a report to S3
func (cr *CostReporter) UploadReportToS3(ctx context.Context, reportPath string, bucket string, key string) error {
	if cr.s3Client == nil {
		return fmt.Errorf("S3 client not configured")
	}

	file, err := os.Open(reportPath)
	if err != nil {
		return fmt.Errorf("failed to open report file: %w", err)
	}
	defer func() {
		if err := file.Close(); err != nil {
			cr.logger.Warn("Failed to close report file", "error", err)
		}
	}()

	_, err = cr.s3Client.PutObject(ctx, &s3.PutObjectInput{
		Bucket: aws.String(bucket),
		Key:    aws.String(key),
		Body:   file,
		Metadata: map[string]string{
			"generated-by": "cargoship-cost-reporter",
			"generated-at": time.Now().Format(time.RFC3339),
		},
	})

	if err != nil {
		return fmt.Errorf("failed to upload report to S3: %w", err)
	}

	cr.logger.Info("Cost report uploaded to S3", "bucket", bucket, "key", key)
	return nil
}

// Helper methods

func (cr *CostReporter) parsePeriod(period string) (time.Time, time.Time, error) {
	now := time.Now()

	switch strings.ToLower(period) {
	case "today":
		start := time.Date(now.Year(), now.Month(), now.Day(), 0, 0, 0, 0, now.Location())
		end := start.Add(24 * time.Hour)
		return start, end, nil

	case "yesterday":
		start := time.Date(now.Year(), now.Month(), now.Day()-1, 0, 0, 0, 0, now.Location())
		end := start.Add(24 * time.Hour)
		return start, end, nil

	case "week", "this_week":
		// Calculate start of current week (Sunday at 00:00:00 local time)
		weekday := int(now.Weekday())
		startOfWeek := now.AddDate(0, 0, -weekday)
		start := time.Date(startOfWeek.Year(), startOfWeek.Month(), startOfWeek.Day(), 0, 0, 0, 0, now.Location())
		end := start.AddDate(0, 0, 7)
		return start, end, nil

	case "month", "this_month":
		start := time.Date(now.Year(), now.Month(), 1, 0, 0, 0, 0, now.Location())
		end := start.AddDate(0, 1, 0)
		return start, end, nil

	case "last_month":
		start := time.Date(now.Year(), now.Month()-1, 1, 0, 0, 0, 0, now.Location())
		end := start.AddDate(0, 1, 0)
		return start, end, nil

	default:
		// Try to parse custom date range
		parts := strings.Split(period, "_to_")
		if len(parts) == 2 {
			start, err := time.Parse("2006-01-02", parts[0])
			if err != nil {
				return time.Time{}, time.Time{}, fmt.Errorf("invalid start date: %w", err)
			}
			end, err := time.Parse("2006-01-02", parts[1])
			if err != nil {
				return time.Time{}, time.Time{}, fmt.Errorf("invalid end date: %w", err)
			}
			return start, end.Add(24 * time.Hour), nil
		}

		return time.Time{}, time.Time{}, fmt.Errorf("unsupported period: %s", period)
	}
}

func (cr *CostReporter) filterCostsByPeriod(start, end time.Time) []CostRecord {
	var filtered []CostRecord

	for _, cost := range cr.costs {
		// Use inclusive start bound and exclusive end bound: [start, end)
		if (cost.Timestamp.After(start) || cost.Timestamp.Equal(start)) && cost.Timestamp.Before(end) {
			filtered = append(filtered, cost)
		}
	}

	return filtered
}

func (cr *CostReporter) getTopCostFiles(costs []CostRecord, limit int) []CostRecord {
	// Sort by cost descending
	sort.Slice(costs, func(i, j int) bool {
		return costs[i].Cost > costs[j].Cost
	})

	if len(costs) > limit {
		return costs[:limit]
	}

	return costs
}

func (cr *CostReporter) calculateTrends(costs []CostRecord, start, end time.Time) CostTrends {
	if len(costs) == 0 {
		return CostTrends{}
	}

	totalCost := 0.0
	totalSize := 0.0
	totalOps := float64(len(costs))

	dailyCosts := make(map[string]float64)

	for _, cost := range costs {
		totalCost += cost.Cost
		totalSize += cost.SizeGB

		day := cost.Timestamp.Format("2006-01-02")
		dailyCosts[day] += cost.Cost
	}

	days := end.Sub(start).Hours() / 24
	if days == 0 {
		days = 1
	}

	trends := CostTrends{
		DailyAverage:      totalCost / days,
		WeeklyAverage:     totalCost / days * 7,
		MonthlyProjection: totalCost / days * 30,
		CostPerGB:         totalCost / totalSize,
		CostPerOperation:  totalCost / totalOps,
	}

	// Calculate growth rate (simplified)
	if len(dailyCosts) > 1 {
		var values []float64
		for _, v := range dailyCosts {
			values = append(values, v)
		}

		if len(values) >= 2 {
			firstHalf := values[:len(values)/2]
			secondHalf := values[len(values)/2:]

			firstAvg := average(firstHalf)
			secondAvg := average(secondHalf)

			if firstAvg > 0 {
				trends.GrowthRate = ((secondAvg - firstAvg) / firstAvg) * 100
			}
		}
	}

	return trends
}

func (cr *CostReporter) generateRecommendations(costs []CostRecord) []CostRecommendation {
	var recommendations []CostRecommendation

	// Analyze storage class usage
	storageClassCosts := make(map[string]float64)
	storageClassSizes := make(map[string]float64)

	for _, cost := range costs {
		if cost.StorageClass != "" {
			storageClassCosts[cost.StorageClass] += cost.Cost
			storageClassSizes[cost.StorageClass] += cost.SizeGB
		}
	}

	// Recommend moving to cheaper storage classes
	standardCost := storageClassCosts["STANDARD"]
	standardSize := storageClassSizes["STANDARD"]

	if standardCost > 0 && standardSize > 0 {
		// Suggest moving to Intelligent Tiering
		potentialSaving := standardCost * 0.2 // Assume 20% savings
		if potentialSaving > 10 {             // Only recommend if savings > $10
			recommendations = append(recommendations, CostRecommendation{
				Type:            "storage_class",
				Priority:        "medium",
				Description:     "Consider moving infrequently accessed data from Standard to Intelligent Tiering",
				PotentialSaving: potentialSaving,
				Implementation:  "Configure lifecycle policies to automatically transition objects",
				Impact:          "No performance impact, automatic cost optimization",
			})
		}
	}

	// Recommend archive for old data
	if standardSize > 100 { // > 100GB
		archiveSaving := standardCost * 0.8 // Assume 80% savings for archive
		recommendations = append(recommendations, CostRecommendation{
			Type:            "lifecycle",
			Priority:        "high",
			Description:     "Archive data older than 90 days to Glacier or Deep Archive",
			PotentialSaving: archiveSaving,
			Implementation:  "Set up S3 lifecycle rules for automatic archiving",
			Impact:          "Longer retrieval times for archived data",
		})
	}

	return recommendations
}

func (cr *CostReporter) exportJSON(summary *CostSummary, outputPath string) error {
	file, err := os.Create(outputPath)
	if err != nil {
		return err
	}
	defer func() {
		if err := file.Close(); err != nil {
			// Error closing file during export - not critical
			_ = err
		}
	}()

	encoder := json.NewEncoder(file)
	encoder.SetIndent("", "  ")
	return encoder.Encode(summary)
}

func (cr *CostReporter) exportCSV(summary *CostSummary, outputPath string) error {
	file, err := os.Create(outputPath)
	if err != nil {
		return err
	}
	defer func() {
		if err := file.Close(); err != nil {
			// Error closing file during export - not critical
			_ = err
		}
	}()

	writer := csv.NewWriter(file)
	defer writer.Flush()

	// Write header
	headers := []string{"Date", "Service", "Region", "Operation", "Cost", "Currency"}
	if err := writer.Write(headers); err != nil {
		return err
	}

	// Write daily costs
	for date, cost := range summary.DailyCosts {
		record := []string{
			date,
			"s3", // Simplified for now
			"",   // Region would need to be tracked separately
			"",   // Operation would need to be tracked separately
			fmt.Sprintf("%.4f", cost),
			summary.Currency,
		}
		if err := writer.Write(record); err != nil {
			return err
		}
	}

	return nil
}

// Utility functions
func average(values []float64) float64 {
	if len(values) == 0 {
		return 0
	}

	sum := 0.0
	for _, v := range values {
		sum += v
	}

	return sum / float64(len(values))
}

// GetCurrentMonthCosts returns costs for the current month
func (cr *CostReporter) GetCurrentMonthCosts() float64 {
	cr.mu.RLock()
	defer cr.mu.RUnlock()

	now := time.Now()
	monthStart := time.Date(now.Year(), now.Month(), 1, 0, 0, 0, 0, now.Location())

	totalCost := 0.0
	for _, cost := range cr.costs {
		if cost.Timestamp.After(monthStart) {
			totalCost += cost.Cost
		}
	}

	return totalCost
}

// PurgeCosts removes costs older than the specified duration
func (cr *CostReporter) PurgeCosts(olderThan time.Duration) int {
	cr.mu.Lock()
	defer cr.mu.Unlock()

	cutoff := time.Now().Add(-olderThan)
	originalLen := len(cr.costs)

	filtered := make([]CostRecord, 0, len(cr.costs))
	for _, cost := range cr.costs {
		if cost.Timestamp.After(cutoff) {
			filtered = append(filtered, cost)
		}
	}

	cr.costs = filtered
	purged := originalLen - len(cr.costs)

	if purged > 0 {
		cr.logger.Info("Purged old cost records", "count", purged, "cutoff", cutoff)
	}

	return purged
}

// Project-based cost tracking methods (Issue #147 Phase 2)

// ListProjects returns all unique project IDs that have cost records
func (cr *CostReporter) ListProjects() []string {
	cr.mu.RLock()
	defer cr.mu.RUnlock()

	projectSet := make(map[string]bool)
	for _, cost := range cr.costs {
		if cost.ProjectID != "" {
			projectSet[cost.ProjectID] = true
		}
	}

	projects := make([]string, 0, len(projectSet))
	for project := range projectSet {
		projects = append(projects, project)
	}

	// Sort alphabetically for consistent ordering
	sort.Strings(projects)
	return projects
}

// GetProjectCosts returns total costs for a specific project
func (cr *CostReporter) GetProjectCosts(projectID string) float64 {
	cr.mu.RLock()
	defer cr.mu.RUnlock()

	totalCost := 0.0
	for _, cost := range cr.costs {
		if cost.ProjectID == projectID {
			totalCost += cost.Cost
		}
	}

	return totalCost
}

// GetProjectVolume returns total volume in GB for a specific project
func (cr *CostReporter) GetProjectVolume(projectID string) float64 {
	cr.mu.RLock()
	defer cr.mu.RUnlock()

	totalVolume := 0.0
	for _, cost := range cr.costs {
		if cost.ProjectID == projectID {
			totalVolume += cost.SizeGB
		}
	}

	return totalVolume
}

// GetCurrentPeriodVolume returns total volume in GB for the current period
func (cr *CostReporter) GetCurrentPeriodVolume(start, end time.Time) float64 {
	cr.mu.RLock()
	defer cr.mu.RUnlock()

	totalVolume := 0.0
	for _, cost := range cr.costs {
		if (cost.Timestamp.After(start) || cost.Timestamp.Equal(start)) && cost.Timestamp.Before(end) {
			totalVolume += cost.SizeGB
		}
	}

	return totalVolume
}

// GetProjectCostsByPeriod returns costs for a specific project within a period
func (cr *CostReporter) GetProjectCostsByPeriod(projectID string, period string) (float64, error) {
	cr.mu.RLock()
	defer cr.mu.RUnlock()

	startTime, endTime, err := cr.parsePeriod(period)
	if err != nil {
		return 0, fmt.Errorf("invalid period: %w", err)
	}

	totalCost := 0.0
	for _, cost := range cr.costs {
		if cost.ProjectID == projectID && (cost.Timestamp.After(startTime) || cost.Timestamp.Equal(startTime)) && cost.Timestamp.Before(endTime) {
			totalCost += cost.Cost
		}
	}

	return totalCost, nil
}

// ProjectSummary provides detailed summary for a single project
type ProjectSummary struct {
	ProjectID        string             `json:"project_id"`
	TotalCost        float64            `json:"total_cost"`
	TotalSavings     float64            `json:"total_savings"`
	TotalFiles       int                `json:"total_files"`
	TotalSizeGB      float64            `json:"total_size_gb"`
	Currency         string             `json:"currency"`
	FirstUpload      time.Time          `json:"first_upload"`
	LastUpload       time.Time          `json:"last_upload"`
	ByRegion         map[string]float64 `json:"by_region"`
	ByStorageClass   map[string]float64 `json:"by_storage_class"`
	AverageCostPerGB float64            `json:"average_cost_per_gb"`
}

// GetProjectSummary returns detailed summary for a specific project
func (cr *CostReporter) GetProjectSummary(projectID string) (*ProjectSummary, error) {
	cr.mu.RLock()
	defer cr.mu.RUnlock()

	var projectCosts []CostRecord
	for _, cost := range cr.costs {
		if cost.ProjectID == projectID {
			projectCosts = append(projectCosts, cost)
		}
	}

	if len(projectCosts) == 0 {
		return nil, fmt.Errorf("no cost records found for project: %s", projectID)
	}

	summary := &ProjectSummary{
		ProjectID:      projectID,
		Currency:       projectCosts[0].Currency,
		ByRegion:       make(map[string]float64),
		ByStorageClass: make(map[string]float64),
		FirstUpload:    projectCosts[0].Timestamp,
		LastUpload:     projectCosts[0].Timestamp,
	}

	totalSize := 0.0

	for _, cost := range projectCosts {
		summary.TotalCost += cost.Cost
		summary.TotalSavings += cost.DiscountApplied
		summary.TotalFiles++
		totalSize += cost.SizeGB

		summary.ByRegion[cost.Region] += cost.Cost
		if cost.StorageClass != "" {
			summary.ByStorageClass[cost.StorageClass] += cost.Cost
		}

		// Track first and last upload times
		if cost.Timestamp.Before(summary.FirstUpload) {
			summary.FirstUpload = cost.Timestamp
		}
		if cost.Timestamp.After(summary.LastUpload) {
			summary.LastUpload = cost.Timestamp
		}
	}

	summary.TotalSizeGB = totalSize
	if totalSize > 0 {
		summary.AverageCostPerGB = summary.TotalCost / totalSize
	}

	return summary, nil
}

// GenerateProjectReport generates a cost report filtered by project
func (cr *CostReporter) GenerateProjectReport(ctx context.Context, projectID string, period string) (*CostSummary, error) {
	cr.mu.RLock()
	defer cr.mu.RUnlock()

	// Filter costs by period
	startTime, endTime, err := cr.parsePeriod(period)
	if err != nil {
		return nil, fmt.Errorf("invalid period: %w", err)
	}

	// Filter by both project and period
	var filteredCosts []CostRecord
	for _, cost := range cr.costs {
		if cost.ProjectID == projectID && (cost.Timestamp.After(startTime) || cost.Timestamp.Equal(startTime)) && cost.Timestamp.Before(endTime) {
			filteredCosts = append(filteredCosts, cost)
		}
	}

	if len(filteredCosts) == 0 {
		return &CostSummary{
			Period:         period,
			Currency:       "USD",
			ByService:      make(map[string]float64),
			ByRegion:       make(map[string]float64),
			ByStorageClass: make(map[string]float64),
			ByOperation:    make(map[string]float64),
			ByProject:      make(map[string]float64),
			DailyCosts:     make(map[string]float64),
		}, nil
	}

	summary := &CostSummary{
		Period:         period,
		Currency:       filteredCosts[0].Currency,
		ByService:      make(map[string]float64),
		ByRegion:       make(map[string]float64),
		ByStorageClass: make(map[string]float64),
		ByOperation:    make(map[string]float64),
		ByProject:      map[string]float64{projectID: 0},
		DailyCosts:     make(map[string]float64),
	}

	// Calculate totals and breakdowns
	for _, cost := range filteredCosts {
		summary.TotalCost += cost.Cost
		summary.TotalSavings += cost.DiscountApplied

		summary.ByService[cost.Service] += cost.Cost
		summary.ByRegion[cost.Region] += cost.Cost
		if cost.StorageClass != "" {
			summary.ByStorageClass[cost.StorageClass] += cost.Cost
		}
		summary.ByOperation[cost.Operation] += cost.Cost
		summary.ByProject[projectID] += cost.Cost

		// Daily breakdown
		day := cost.Timestamp.Format("2006-01-02")
		summary.DailyCosts[day] += cost.Cost
	}

	// Get top cost files
	summary.TopFiles = cr.getTopCostFiles(filteredCosts, 10)

	// Calculate trends
	summary.Trends = cr.calculateTrends(filteredCosts, startTime, endTime)

	// Generate recommendations
	summary.Recommendations = cr.generateRecommendations(filteredCosts)

	return summary, nil
}
