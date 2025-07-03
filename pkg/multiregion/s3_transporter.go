// Package multiregion provides multi-region S3 transport implementation
package multiregion

import (
	"context"
	"fmt"
	"log/slog"
	"sync"
	"time"

	"github.com/aws/aws-sdk-go-v2/service/s3"
	
	awsconfig "github.com/scttfrdmn/cargoship/pkg/aws/config"
	s3transport "github.com/scttfrdmn/cargoship/pkg/aws/s3"
)

// MultiRegionS3Transporter implements multi-region S3 uploads with intelligent failover
type MultiRegionS3Transporter struct {
	coordinator  Coordinator
	transporters map[string]*s3transport.AdaptiveTransporter
	clients      map[string]*s3.Client
	config       *MultiRegionS3Config
	logger       *slog.Logger
	mu           sync.RWMutex
}

// MultiRegionS3Config configures multi-region S3 transport behavior
type MultiRegionS3Config struct {
	*MultiRegionConfig
	S3Config             awsconfig.S3Config                       `yaml:"s3_config" json:"s3_config"`
	AdaptiveConfig       *s3transport.AdaptiveTransporterConfig   `yaml:"adaptive_config" json:"adaptive_config"`
	CrossRegionRetries   int                                      `yaml:"cross_region_retries" json:"cross_region_retries"`
	FailoverDelay        time.Duration                            `yaml:"failover_delay" json:"failover_delay"`
	RedundantUploads     bool                                     `yaml:"redundant_uploads" json:"redundant_uploads"`
	RedundantRegionCount int                                      `yaml:"redundant_region_count" json:"redundant_region_count"`
	SyncValidation       bool                                     `yaml:"sync_validation" json:"sync_validation"`
}

// MultiRegionUploadRequest represents an upload request with multi-region options
type MultiRegionUploadRequest struct {
	*UploadRequest
	Archive             s3transport.Archive   `json:"archive"`
	TargetBucket        string                `json:"target_bucket"`
	PreferredRegions    []string              `json:"preferred_regions"`
	RedundancyLevel     int                   `json:"redundancy_level"`
	AllowDegradedUpload bool                  `json:"allow_degraded_upload"`
}

// MultiRegionUploadResult contains results from multi-region upload
type MultiRegionUploadResult struct {
	*UploadResult
	RegionResults     map[string]*s3transport.UploadResult `json:"region_results"`
	FailedRegions     []string                             `json:"failed_regions"`
	RedundantCopies   int                                  `json:"redundant_copies"`
	PrimaryLocation   string                               `json:"primary_location"`
	ValidationResults map[string]bool                      `json:"validation_results"`
}

// NewMultiRegionS3Transporter creates a new multi-region S3 transporter
func NewMultiRegionS3Transporter(ctx context.Context, config *MultiRegionS3Config, logger *slog.Logger) (*MultiRegionS3Transporter, error) {
	if config == nil {
		return nil, fmt.Errorf("configuration cannot be nil")
	}
	
	if logger == nil {
		logger = slog.Default()
	}
	
	// Create coordinator
	coordinator := NewCoordinator()
	err := coordinator.Initialize(ctx, config.MultiRegionConfig)
	if err != nil {
		return nil, fmt.Errorf("failed to initialize coordinator: %w", err)
	}
	
	t := &MultiRegionS3Transporter{
		coordinator:  coordinator,
		transporters: make(map[string]*s3transport.AdaptiveTransporter),
		clients:      make(map[string]*s3.Client),
		config:       config,
		logger:       logger,
	}
	
	// Initialize S3 clients and transporters for each region
	err = t.initializeRegionTransporters(ctx)
	if err != nil {
		return nil, fmt.Errorf("failed to initialize region transporters: %w", err)
	}
	
	logger.Info("multi-region S3 transporter initialized",
		"regions", len(t.transporters),
		"redundant_uploads", config.RedundantUploads,
		"cross_region_retries", config.CrossRegionRetries)
	
	return t, nil
}

// Upload performs a multi-region upload with intelligent region selection and failover
func (t *MultiRegionS3Transporter) Upload(ctx context.Context, request *MultiRegionUploadRequest) (*MultiRegionUploadResult, error) {
	if request == nil {
		return nil, fmt.Errorf("upload request cannot be nil")
	}
	
	if request.UploadRequest == nil {
		request.UploadRequest = &UploadRequest{
			FilePath: request.Archive.Key,
			Priority: 5, // Normal priority
			Size:     request.Archive.Size,
		}
	}
	
	t.logger.Info("starting multi-region upload",
		"request_id", request.ID,
		"file_path", request.FilePath,
		"target_bucket", request.TargetBucket,
		"preferred_regions", request.PreferredRegions,
		"redundancy_level", request.RedundancyLevel)
	
	if t.config.RedundantUploads && request.RedundancyLevel > 1 {
		return t.uploadRedundant(ctx, request)
	} else {
		return t.uploadSingle(ctx, request)
	}
}

// uploadSingle performs a single-region upload with failover support
func (t *MultiRegionS3Transporter) uploadSingle(ctx context.Context, request *MultiRegionUploadRequest) (*MultiRegionUploadResult, error) {
	// Use coordinator to route the upload request
	result, err := t.coordinator.Upload(ctx, request.UploadRequest)
	if err != nil {
		return nil, fmt.Errorf("coordinator failed to route upload: %w", err)
	}
	
	// Get transporter for the selected region
	transporter, err := t.getRegionTransporter(result.Region)
	if err != nil {
		return nil, fmt.Errorf("failed to get transporter for region %s: %w", result.Region, err)
	}
	
	// Perform the actual upload
	uploadResult, err := t.executeUpload(ctx, transporter, request)
	if err != nil {
		// Try failover if enabled
		if t.config.CrossRegionRetries > 0 {
			return t.uploadWithFailover(ctx, request, result.Region, err)
		}
		return nil, fmt.Errorf("upload failed in region %s: %w", result.Region, err)
	}
	
	return &MultiRegionUploadResult{
		UploadResult: result,
		RegionResults: map[string]*s3transport.UploadResult{
			result.Region: uploadResult,
		},
		RedundantCopies:   1,
		PrimaryLocation:   uploadResult.Location,
		ValidationResults: map[string]bool{result.Region: true},
	}, nil
}

// uploadRedundant performs redundant uploads across multiple regions
func (t *MultiRegionS3Transporter) uploadRedundant(ctx context.Context, request *MultiRegionUploadRequest) (*MultiRegionUploadResult, error) {
	// Use coordinator to select multiple regions
	selector := NewRegionSelector(t.config.MultiRegionConfig, nil)
	regions, err := selector.SelectRegions(ctx, request.UploadRequest, request.RedundancyLevel)
	if err != nil {
		return nil, fmt.Errorf("failed to select regions for redundant upload: %w", err)
	}
	
	t.logger.Info("performing redundant upload",
		"request_id", request.ID,
		"regions", len(regions),
		"target_redundancy", request.RedundancyLevel)
	
	// Perform uploads in parallel
	type uploadAttempt struct {
		region string
		result *s3transport.UploadResult
		err    error
	}
	
	resultChan := make(chan uploadAttempt, len(regions))
	
	for _, region := range regions {
		go func(r *Region) {
			transporter, err := t.getRegionTransporter(r.Name)
			if err != nil {
				resultChan <- uploadAttempt{region: r.Name, err: err}
				return
			}
			
			result, err := t.executeUpload(ctx, transporter, request)
			resultChan <- uploadAttempt{region: r.Name, result: result, err: err}
		}(region)
	}
	
	// Collect results
	regionResults := make(map[string]*s3transport.UploadResult)
	failedRegions := make([]string, 0)
	validationResults := make(map[string]bool)
	var primaryLocation string
	successCount := 0
	
	for i := 0; i < len(regions); i++ {
		attempt := <-resultChan
		
		if attempt.err != nil {
			t.logger.Warn("upload failed in region",
				"region", attempt.region,
				"error", attempt.err.Error())
			failedRegions = append(failedRegions, attempt.region)
			validationResults[attempt.region] = false
		} else {
			regionResults[attempt.region] = attempt.result
			validationResults[attempt.region] = true
			successCount++
			
			if primaryLocation == "" {
				primaryLocation = attempt.result.Location
			}
			
			t.logger.Info("upload succeeded in region",
				"region", attempt.region,
				"location", attempt.result.Location,
				"duration", attempt.result.Duration)
		}
	}
	
	// Check if we have enough successful uploads
	if successCount == 0 {
		return nil, fmt.Errorf("all uploads failed across %d regions", len(regions))
	}
	
	minSuccessful := 1
	if request.RedundancyLevel > 1 {
		minSuccessful = (request.RedundancyLevel + 1) / 2 // Majority
	}
	
	if successCount < minSuccessful && !request.AllowDegradedUpload {
		return nil, fmt.Errorf("insufficient successful uploads: %d/%d (required: %d)", 
			successCount, len(regions), minSuccessful)
	}
	
	// Calculate total duration (max of all uploads)
	maxDuration := time.Duration(0)
	totalBytes := int64(0)
	for _, result := range regionResults {
		if result.Duration > maxDuration {
			maxDuration = result.Duration
		}
		if totalBytes == 0 {
			totalBytes = request.Archive.Size
		}
	}
	
	return &MultiRegionUploadResult{
		UploadResult: &UploadResult{
			RequestID:        request.ID,
			Region:           "multi-region",
			Success:          true,
			Duration:         maxDuration,
			BytesTransferred: totalBytes,
			CompletedAt:      time.Now(),
		},
		RegionResults:     regionResults,
		FailedRegions:     failedRegions,
		RedundantCopies:   successCount,
		PrimaryLocation:   primaryLocation,
		ValidationResults: validationResults,
	}, nil
}

// uploadWithFailover attempts upload with automatic failover
func (t *MultiRegionS3Transporter) uploadWithFailover(ctx context.Context, request *MultiRegionUploadRequest, failedRegion string, originalErr error) (*MultiRegionUploadResult, error) {
	t.logger.Warn("attempting failover upload",
		"request_id", request.ID,
		"failed_region", failedRegion,
		"original_error", originalErr.Error())
	
	// Select alternative region
	selector := NewRegionSelector(t.config.MultiRegionConfig, nil)
	
	for retry := 0; retry < t.config.CrossRegionRetries; retry++ {
		// Add failed region to avoid it
		request.PreferredRegion = "" // Clear preferred region
		
		region, err := selector.SelectRegion(ctx, request.UploadRequest)
		if err != nil {
			continue
		}
		
		if region.Name == failedRegion {
			continue // Skip the failed region
		}
		
		// Wait for failover delay
		if t.config.FailoverDelay > 0 {
			time.Sleep(t.config.FailoverDelay)
		}
		
		transporter, err := t.getRegionTransporter(region.Name)
		if err != nil {
			t.logger.Warn("failed to get transporter for failover region",
				"region", region.Name,
				"error", err.Error())
			continue
		}
		
		uploadResult, err := t.executeUpload(ctx, transporter, request)
		if err != nil {
			t.logger.Warn("failover upload failed",
				"region", region.Name,
				"retry", retry+1,
				"error", err.Error())
			continue
		}
		
		t.logger.Info("failover upload succeeded",
			"region", region.Name,
			"retry", retry+1,
			"location", uploadResult.Location)
		
		return &MultiRegionUploadResult{
			UploadResult: &UploadResult{
				RequestID:        request.ID,
				Region:           region.Name,
				Success:          true,
				Duration:         uploadResult.Duration,
				BytesTransferred: request.Archive.Size,
				CompletedAt:      time.Now(),
			},
			RegionResults: map[string]*s3transport.UploadResult{
				region.Name: uploadResult,
			},
			FailedRegions:     []string{failedRegion},
			RedundantCopies:   1,
			PrimaryLocation:   uploadResult.Location,
			ValidationResults: map[string]bool{region.Name: true},
		}, nil
	}
	
	return nil, fmt.Errorf("all failover attempts exhausted after %d retries, original error: %w", 
		t.config.CrossRegionRetries, originalErr)
}

// executeUpload executes the actual upload using the appropriate transporter
func (t *MultiRegionS3Transporter) executeUpload(ctx context.Context, transporter *s3transport.AdaptiveTransporter, request *MultiRegionUploadRequest) (*s3transport.UploadResult, error) {
	// Use adaptive upload if available
	if transporter != nil {
		return transporter.UploadWithStaging(ctx, request.Archive)
	}
	
	return nil, fmt.Errorf("no transporter available")
}

// getRegionTransporter returns the transporter for a specific region
func (t *MultiRegionS3Transporter) getRegionTransporter(regionName string) (*s3transport.AdaptiveTransporter, error) {
	t.mu.RLock()
	defer t.mu.RUnlock()
	
	transporter, exists := t.transporters[regionName]
	if !exists {
		return nil, fmt.Errorf("no transporter configured for region: %s", regionName)
	}
	
	return transporter, nil
}

// initializeRegionTransporters initializes S3 clients and transporters for each configured region
func (t *MultiRegionS3Transporter) initializeRegionTransporters(ctx context.Context) error {
	for _, region := range t.config.Regions {
		// Create AWS config for the region
		cfg, err := awsconfig.LoadAWSConfig(ctx, "", region.Name)
		if err != nil {
			return fmt.Errorf("failed to load AWS config for region %s: %w", region.Name, err)
		}
		
		// Override region
		cfg.Region = region.Name
		
		// Create S3 client
		client := s3.NewFromConfig(cfg)
		t.clients[region.Name] = client
		
		// Create adaptive transporter
		adaptiveConfig := t.config.AdaptiveConfig
		if adaptiveConfig == nil {
			adaptiveConfig = s3transport.DefaultAdaptiveTransporterConfig()
		}
		
		transporter, err := s3transport.NewAdaptiveTransporter(ctx, client, t.config.S3Config, adaptiveConfig, t.logger)
		if err != nil {
			return fmt.Errorf("failed to create adaptive transporter for region %s: %w", region.Name, err)
		}
		
		t.transporters[region.Name] = transporter
		
		t.logger.Info("initialized transporter for region",
			"region", region.Name,
			"adaptive_enabled", adaptiveConfig.EnableRealTimeAdaptation)
	}
	
	return nil
}

// Shutdown gracefully shuts down all transporters and the coordinator
func (t *MultiRegionS3Transporter) Shutdown(ctx context.Context) error {
	t.logger.Info("shutting down multi-region S3 transporter")
	
	// Shutdown coordinator
	if t.coordinator != nil {
		err := t.coordinator.Shutdown(ctx)
		if err != nil {
			t.logger.Warn("error shutting down coordinator", "error", err.Error())
		}
	}
	
	// Shutdown transporters
	t.mu.Lock()
	defer t.mu.Unlock()
	
	for region, transporter := range t.transporters {
		if transporter != nil {
			// Adaptive transporters don't have a Shutdown method in the current implementation
			// This is a placeholder for future enhancement
			t.logger.Info("shutting down transporter", "region", region)
		}
	}
	
	t.logger.Info("multi-region S3 transporter shutdown completed")
	return nil
}

// DefaultMultiRegionS3Config returns default configuration for multi-region S3 transport
func DefaultMultiRegionS3Config() *MultiRegionS3Config {
	return &MultiRegionS3Config{
		MultiRegionConfig:    DefaultMultiRegionConfig(),
		CrossRegionRetries:   2,
		FailoverDelay:        5 * time.Second,
		RedundantUploads:     false,
		RedundantRegionCount: 2,
		SyncValidation:       true,
	}
}

// DefaultMultiRegionConfig returns default multi-region configuration
func DefaultMultiRegionConfig() *MultiRegionConfig {
	return &MultiRegionConfig{
		Enabled:       true,
		PrimaryRegion: "us-east-1",
		Regions: []Region{
			{
				Name:     "us-east-1",
				Priority: 1,
				Weight:   50,
				Status:   RegionStatusHealthy,
				Capacity: RegionCapacity{
					MaxConcurrentUploads: 10,
					MaxBandwidthMbps:     1000,
				},
				HealthCheck: HealthCheckConfig{
					Enabled:          true,
					Interval:         30 * time.Second,
					Timeout:          5 * time.Second,
					FailureThreshold: 3,
					SuccessThreshold: 2,
				},
			},
			{
				Name:     "us-west-2",
				Priority: 2,
				Weight:   30,
				Status:   RegionStatusHealthy,
				Capacity: RegionCapacity{
					MaxConcurrentUploads: 8,
					MaxBandwidthMbps:     800,
				},
				HealthCheck: HealthCheckConfig{
					Enabled:          true,
					Interval:         30 * time.Second,
					Timeout:          5 * time.Second,
					FailureThreshold: 3,
					SuccessThreshold: 2,
				},
			},
		},
		LoadBalancing: LoadBalancingConfig{
			Strategy:        LoadBalancingRoundRobin,
			StickySessions:  false,
		},
		Failover: FailoverConfig{
			AutoFailover:        true,
			Strategy:           FailoverGraceful,
			DetectionInterval:  15 * time.Second,
			FailoverTimeout:    30 * time.Second,
			RetryAttempts:      2,
		},
		Monitoring: MonitoringConfig{
			Enabled:         true,
			MetricsInterval: 60 * time.Second,
		},
	}
}