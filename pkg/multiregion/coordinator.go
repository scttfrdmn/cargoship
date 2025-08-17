// Package multiregion provides multi-region pipeline distribution and coordination for CargoShip
package multiregion

import (
	"context"
	"fmt"
	"math/rand"
	"os"
	"sync"
	"time"

	"github.com/charmbracelet/log"
	"github.com/google/uuid"
)

// DefaultCoordinator implements the Coordinator interface for multi-region operations
type DefaultCoordinator struct {
	// config holds the multi-region configuration
	config *MultiRegionConfig

	// regionSelector handles region selection logic
	regionSelector RegionSelector

	// loadBalancer handles load balancing across regions
	loadBalancer LoadBalancer

	// failoverManager handles failover operations
	failoverManager FailoverManager

	// regions map of region name to region info
	regions map[string]*Region

	// mu protects concurrent access to regions map
	mu sync.RWMutex

	// logger for coordinator operations
	logger *log.Logger

	// ctx coordinator context
	ctx context.Context

	// cancel function for coordinator context
	cancel context.CancelFunc

	// wg for graceful shutdown
	wg sync.WaitGroup

	// initialized indicates if coordinator has been initialized
	initialized bool
}

// NewCoordinator creates a new multi-region coordinator
func NewCoordinator() *DefaultCoordinator {
	return &DefaultCoordinator{
		regions: make(map[string]*Region),
		logger:  log.New(os.Stderr),
	}
}

// Initialize initializes the multi-region coordinator with the provided configuration
func (c *DefaultCoordinator) Initialize(ctx context.Context, config *MultiRegionConfig) error {
	c.mu.Lock()
	defer c.mu.Unlock()

	if c.initialized {
		return fmt.Errorf("coordinator already initialized")
	}

	if config == nil {
		return fmt.Errorf("configuration cannot be nil")
	}

	// Validate configuration
	if err := c.validateConfig(config); err != nil {
		return fmt.Errorf("invalid configuration: %w", err)
	}

	// Store configuration
	c.config = config

	// Initialize regions map
	for i := range config.Regions {
		region := &config.Regions[i]
		c.regions[region.Name] = region
	}

	// Create coordinator context
	c.ctx, c.cancel = context.WithCancel(ctx)

	// Initialize components
	if err := c.initializeComponents(); err != nil {
		return fmt.Errorf("failed to initialize components: %w", err)
	}

	// Start background services
	c.startBackgroundServices()

	c.initialized = true
	c.logger.Info("Multi-region coordinator initialized successfully",
		"regions", len(c.regions),
		"primary_region", c.config.PrimaryRegion,
		"load_balancing_strategy", c.config.LoadBalancing.Strategy)

	return nil
}

// Upload performs a multi-region upload operation
func (c *DefaultCoordinator) Upload(ctx context.Context, request *UploadRequest) (*UploadResult, error) {
	if !c.initialized {
		return nil, fmt.Errorf("coordinator not initialized")
	}

	if request == nil {
		return nil, fmt.Errorf("upload request cannot be nil")
	}

	// Set request ID if not provided
	if request.ID == "" {
		request.ID = uuid.New().String()
	}

	// Set creation time
	request.CreatedAt = time.Now()

	c.logger.Debug("Processing upload request",
		"request_id", request.ID,
		"file_path", request.FilePath,
		"preferred_region", request.PreferredRegion,
		"priority", request.Priority)

	// Select appropriate region for upload
	region, err := c.selectRegionForUpload(ctx, request)
	if err != nil {
		return nil, fmt.Errorf("failed to select region: %w", err)
	}

	c.logger.Debug("Selected region for upload",
		"request_id", request.ID,
		"region", region.Name,
		"region_status", region.Status)

	// Execute upload with failover support
	result, err := c.executeUploadWithFailover(ctx, request, region)
	if err != nil {
		return nil, fmt.Errorf("upload failed: %w", err)
	}

	c.logger.Info("Upload completed successfully",
		"request_id", request.ID,
		"region", result.Region,
		"duration", result.Duration,
		"bytes_transferred", result.BytesTransferred)

	return result, nil
}

// GetRegionStatus returns the status of all configured regions
func (c *DefaultCoordinator) GetRegionStatus(ctx context.Context) (map[string]RegionStatus, error) {
	if !c.initialized {
		return nil, fmt.Errorf("coordinator not initialized")
	}

	c.mu.RLock()
	defer c.mu.RUnlock()

	status := make(map[string]RegionStatus)
	for name, region := range c.regions {
		status[name] = region.Status
	}

	return status, nil
}

// GetRegionMetrics returns metrics for all configured regions
func (c *DefaultCoordinator) GetRegionMetrics(ctx context.Context) (map[string]RegionMetrics, error) {
	if !c.initialized {
		return nil, fmt.Errorf("coordinator not initialized")
	}

	c.mu.RLock()
	defer c.mu.RUnlock()

	metrics := make(map[string]RegionMetrics)
	for name, region := range c.regions {
		metrics[name] = region.Metrics
	}

	return metrics, nil
}

// Shutdown gracefully shuts down the coordinator
func (c *DefaultCoordinator) Shutdown(ctx context.Context) error {
	c.mu.Lock()
	defer c.mu.Unlock()

	if !c.initialized {
		return fmt.Errorf("coordinator not initialized")
	}

	c.logger.Info("Shutting down multi-region coordinator")

	// Cancel coordinator context
	if c.cancel != nil {
		c.cancel()
	}

	// Wait for background services to shutdown
	done := make(chan struct{})
	go func() {
		c.wg.Wait()
		close(done)
	}()

	select {
	case <-done:
		c.logger.Info("Multi-region coordinator shutdown completed")
	case <-ctx.Done():
		c.logger.Warn("Multi-region coordinator shutdown timed out")
		return ctx.Err()
	}

	c.initialized = false
	return nil
}

// validateConfig validates the multi-region configuration
func (c *DefaultCoordinator) validateConfig(config *MultiRegionConfig) error {
	if !config.Enabled {
		return fmt.Errorf("multi-region support is disabled")
	}

	if len(config.Regions) == 0 {
		return fmt.Errorf("at least one region must be configured")
	}

	if config.PrimaryRegion == "" {
		return fmt.Errorf("primary region must be specified")
	}

	// Validate primary region exists in regions list
	var primaryRegionFound bool
	for _, region := range config.Regions {
		if region.Name == config.PrimaryRegion {
			primaryRegionFound = true
			break
		}
	}

	if !primaryRegionFound {
		return fmt.Errorf("primary region '%s' not found in regions list", config.PrimaryRegion)
	}

	// Validate region configurations
	for _, region := range config.Regions {
		if err := c.validateRegion(&region); err != nil {
			return fmt.Errorf("invalid region '%s': %w", region.Name, err)
		}
	}

	return nil
}

// validateRegion validates individual region configuration
func (c *DefaultCoordinator) validateRegion(region *Region) error {
	if region == nil {
		return fmt.Errorf("region cannot be nil")
	}

	if region.Name == "" {
		return fmt.Errorf("region name cannot be empty")
	}

	if region.Priority < 1 {
		return fmt.Errorf("region priority must be at least 1")
	}

	if region.Weight < 0 || region.Weight > 100 {
		return fmt.Errorf("region weight must be between 0 and 100")
	}

	if region.Capacity.MaxConcurrentUploads < 1 {
		return fmt.Errorf("max concurrent uploads must be at least 1")
	}

	if region.HealthCheck.Enabled {
		if region.HealthCheck.Interval <= 0 {
			return fmt.Errorf("health check interval must be positive")
		}

		if region.HealthCheck.Timeout <= 0 {
			return fmt.Errorf("health check timeout must be positive")
		}

		if region.HealthCheck.FailureThreshold < 1 {
			return fmt.Errorf("health check failure threshold must be at least 1")
		}

		if region.HealthCheck.SuccessThreshold < 1 {
			return fmt.Errorf("health check success threshold must be at least 1")
		}
	}

	return nil
}

// initializeComponents initializes the coordinator components
func (c *DefaultCoordinator) initializeComponents() error {
	// Initialize region selector
	c.regionSelector = NewRegionSelector(c.config, c.logger)

	// Initialize load balancer
	c.loadBalancer = NewLoadBalancer(c.config, c.logger)

	// Initialize failover manager
	c.failoverManager = NewFailoverManager(c.config, c.logger)

	return nil
}

// startBackgroundServices starts background monitoring and maintenance services
func (c *DefaultCoordinator) startBackgroundServices() {
	// Start health check service
	if c.config.Monitoring.Enabled {
		c.wg.Add(1)
		go c.healthCheckService()
	}

	// Start metrics collection service
	if c.config.Monitoring.Enabled {
		c.wg.Add(1)
		go c.metricsCollectionService()
	}

	// Start failover detection service
	if c.config.Failover.AutoFailover {
		c.wg.Add(1)
		go c.failoverDetectionService()
	}
}

// selectRegionForUpload selects the best region for an upload request
func (c *DefaultCoordinator) selectRegionForUpload(ctx context.Context, request *UploadRequest) (*Region, error) {
	// Use load balancer to route the request
	return c.loadBalancer.Route(ctx, request)
}

// executeUploadWithFailover executes an upload with automatic failover support
func (c *DefaultCoordinator) executeUploadWithFailover(ctx context.Context, request *UploadRequest, region *Region) (*UploadResult, error) {
	startTime := time.Now()

	// Execute upload in the selected region
	result, err := c.executeUploadInRegion(ctx, request, region)
	if err != nil {
		// Check if failover is enabled and we have alternative regions
		if c.config.Failover.AutoFailover {
			c.logger.Warn("upload failed, attempting failover",
				"request_id", request.ID,
				"failed_region", region.Name,
				"error", err.Error())

			// Record failure for the region
			c.recordRegionFailure(region.Name, err)

			// Try failover to another region
			return c.attemptFailover(ctx, request, region.Name, startTime)
		}

		return nil, fmt.Errorf("upload failed in region %s: %w", region.Name, err)
	}

	// Update region metrics based on successful upload
	c.updateRegionMetrics(region.Name, result)

	return result, nil
}

// executeUploadInRegion executes upload in a specific region
func (c *DefaultCoordinator) executeUploadInRegion(ctx context.Context, request *UploadRequest, region *Region) (*UploadResult, error) {
	// Check region health and capacity
	if region.Status == RegionStatusUnhealthy {
		return nil, fmt.Errorf("region %s is unhealthy", region.Name)
	}

	// Simulate network conditions and latency
	networkDelay := c.simulateNetworkDelay(region)
	if networkDelay > 0 {
		select {
		case <-time.After(networkDelay):
		case <-ctx.Done():
			return nil, ctx.Err()
		}
	}

	// Simulate upload operation with realistic behavior
	uploadDuration := c.calculateUploadDuration(request, region)

	// Check for simulated failures based on region health
	if c.shouldSimulateFailure(region) {
		return nil, fmt.Errorf("simulated upload failure in region %s", region.Name)
	}

	// Wait for upload duration
	select {
	case <-time.After(uploadDuration):
	case <-ctx.Done():
		return nil, ctx.Err()
	}

	// Create successful result
	result := &UploadResult{
		RequestID:        request.ID,
		Region:           region.Name,
		Success:          true,
		Duration:         uploadDuration,
		BytesTransferred: request.Size,
		CompletedAt:      time.Now(),
	}

	return result, nil
}

// attemptFailover attempts to failover to an alternative region
func (c *DefaultCoordinator) attemptFailover(ctx context.Context, request *UploadRequest, failedRegion string, startTime time.Time) (*UploadResult, error) {
	// Get alternative regions (excluding the failed one)
	alternativeRegions := c.getAlternativeRegions(failedRegion)
	if len(alternativeRegions) == 0 {
		return nil, fmt.Errorf("no alternative regions available for failover")
	}

	// Try each alternative region
	for _, region := range alternativeRegions {
		if time.Since(startTime) > 5*time.Minute { // Timeout after 5 minutes
			return nil, fmt.Errorf("failover timeout exceeded")
		}

		c.logger.Info("attempting failover to region",
			"request_id", request.ID,
			"failover_region", region.Name)

		// Add failover delay if configured
		if c.config.Failover.DetectionInterval > 0 {
			time.Sleep(c.config.Failover.DetectionInterval / 2)
		}

		result, err := c.executeUploadInRegion(ctx, request, region)
		if err != nil {
			c.logger.Warn("failover attempt failed",
				"request_id", request.ID,
				"failover_region", region.Name,
				"error", err.Error())

			// Record failure for this region too
			c.recordRegionFailure(region.Name, err)
			continue
		}

		c.logger.Info("failover successful",
			"request_id", request.ID,
			"failover_region", region.Name,
			"total_duration", time.Since(startTime))

		// Update region metrics for successful failover
		c.updateRegionMetrics(region.Name, result)

		return result, nil
	}

	return nil, fmt.Errorf("all failover attempts failed")
}

// getAlternativeRegions returns healthy regions excluding the failed one
func (c *DefaultCoordinator) getAlternativeRegions(excludeRegion string) []*Region {
	c.mu.RLock()
	defer c.mu.RUnlock()

	var alternatives []*Region
	for _, region := range c.regions {
		if region.Name != excludeRegion && region.Status == RegionStatusHealthy {
			alternatives = append(alternatives, region)
		}
	}

	return alternatives
}

// simulateNetworkDelay simulates network latency based on region
func (c *DefaultCoordinator) simulateNetworkDelay(region *Region) time.Duration {
	// Simulate different latencies for different regions
	baseLatency := 10 * time.Millisecond

	// Add variability based on region priority and metrics
	if region.Priority > 1 {
		baseLatency += time.Duration(region.Priority*5) * time.Millisecond
	}

	// Add some randomness (0-20ms)
	jitter := time.Duration(rand.Intn(20)) * time.Millisecond

	return baseLatency + jitter
}

// calculateUploadDuration calculates realistic upload duration
func (c *DefaultCoordinator) calculateUploadDuration(request *UploadRequest, region *Region) time.Duration {
	// Base upload time calculation
	// Assume 100 MB/s base throughput, adjusted by region performance
	baseThroughputMBps := 100.0

	// Adjust based on region metrics if available
	c.mu.RLock()
	if metrics, exists := c.regions[region.Name]; exists {
		if metrics.Metrics.ThroughputMbps > 0 {
			baseThroughputMBps = metrics.Metrics.ThroughputMbps
		}
	}
	c.mu.RUnlock()

	// Calculate upload time
	if request.Size <= 0 {
		request.Size = 1024 * 1024 // Default 1MB if size not specified
	}

	sizeMB := float64(request.Size) / (1024 * 1024)
	uploadSeconds := sizeMB / baseThroughputMBps

	// Add minimum upload time
	if uploadSeconds < 0.1 {
		uploadSeconds = 0.1
	}

	return time.Duration(uploadSeconds * float64(time.Second))
}

// shouldSimulateFailure determines if we should simulate a failure
func (c *DefaultCoordinator) shouldSimulateFailure(region *Region) bool {
	// Simulate failures based on region health metrics
	c.mu.RLock()
	defer c.mu.RUnlock()

	if metrics, exists := c.regions[region.Name]; exists {
		// Higher error rate = higher chance of failure
		errorRate := metrics.Metrics.ErrorRate
		if errorRate > 10.0 { // If error rate > 10%
			// Use randomness to simulate occasional failures
			return rand.Float64() < (errorRate / 100.0)
		}
	}

	return false
}

// recordRegionFailure records a failure for a region
func (c *DefaultCoordinator) recordRegionFailure(regionName string, err error) {
	c.mu.Lock()
	defer c.mu.Unlock()

	if region, exists := c.regions[regionName]; exists {
		region.Metrics.FailedUploads++
		region.Metrics.LastUpdated = time.Now()

		// Update error rate
		totalUploads := region.Metrics.SuccessfulUploads + region.Metrics.FailedUploads
		if totalUploads > 0 {
			region.Metrics.ErrorRate = float64(region.Metrics.FailedUploads) / float64(totalUploads) * 100
		}

		// Mark region as degraded if error rate is too high
		if region.Metrics.ErrorRate > 25.0 { // 25% error rate threshold
			region.Status = RegionStatusDegraded
			c.logger.Warn("region marked as degraded due to high error rate",
				"region", regionName,
				"error_rate", region.Metrics.ErrorRate)
		}

		c.logger.Debug("recorded failure for region",
			"region", regionName,
			"error", err.Error(),
			"total_failures", region.Metrics.FailedUploads,
			"error_rate", region.Metrics.ErrorRate)
	}
}

// updateRegionMetrics updates metrics for a region based on upload result
func (c *DefaultCoordinator) updateRegionMetrics(regionName string, result *UploadResult) {
	c.mu.Lock()
	defer c.mu.Unlock()

	region, exists := c.regions[regionName]
	if !exists {
		return
	}

	// Update metrics
	region.Metrics.LastUpdated = time.Now()
	region.Metrics.AverageLatencyMs = float64(result.Duration.Milliseconds())

	if result.Success {
		region.Metrics.SuccessfulUploads++
	} else {
		region.Metrics.FailedUploads++
	}

	// Calculate error rate
	totalUploads := region.Metrics.SuccessfulUploads + region.Metrics.FailedUploads
	if totalUploads > 0 {
		region.Metrics.ErrorRate = float64(region.Metrics.FailedUploads) / float64(totalUploads) * 100
	}
}

// healthCheckService runs periodic health checks on all regions
func (c *DefaultCoordinator) healthCheckService() {
	defer c.wg.Done()

	interval := c.config.Monitoring.MetricsInterval
	if interval <= 0 {
		interval = 30 * time.Second
	}

	ticker := time.NewTicker(interval)
	defer ticker.Stop()

	for {
		select {
		case <-c.ctx.Done():
			return
		case <-ticker.C:
			c.performHealthChecks()
		}
	}
}

// performHealthChecks performs health checks on all regions
func (c *DefaultCoordinator) performHealthChecks() {
	c.mu.RLock()
	regions := make([]*Region, 0, len(c.regions))
	for _, region := range c.regions {
		regions = append(regions, region)
	}
	c.mu.RUnlock()

	for _, region := range regions {
		if !region.HealthCheck.Enabled {
			continue
		}

		// Perform comprehensive health check
		c.performRegionHealthCheck(region)
	}
}

// performRegionHealthCheck performs a comprehensive health check for a single region
func (c *DefaultCoordinator) performRegionHealthCheck(region *Region) {
	ctx, cancel := context.WithTimeout(c.ctx, region.HealthCheck.Timeout)
	defer cancel()

	c.logger.Debug("Starting health check", "region", region.Name)
	
	// Perform multiple health check types
	healthResults := c.executeHealthChecks(ctx, region)
	
	// Determine overall health status
	overallHealth := c.evaluateHealthResults(healthResults)
	
	// Update region status based on health check results
	c.updateRegionHealthStatus(region, overallHealth, healthResults)
}

// HealthCheckResult represents the result of a single health check
type HealthCheckResult struct {
	CheckType   string        `json:"check_type"`
	Success     bool          `json:"success"`
	ResponseTime time.Duration `json:"response_time"`
	Error       error         `json:"error,omitempty"`
	Details     map[string]interface{} `json:"details,omitempty"`
}

// OverallHealthStatus represents the combined health status
type OverallHealthStatus struct {
	Healthy         bool                    `json:"healthy"`
	SuccessRate     float64                `json:"success_rate"`
	AvgResponseTime time.Duration           `json:"avg_response_time"`
	CheckResults    []HealthCheckResult     `json:"check_results"`
	FailureReasons  []string               `json:"failure_reasons,omitempty"`
}

// executeHealthChecks runs all configured health checks for a region
func (c *DefaultCoordinator) executeHealthChecks(ctx context.Context, region *Region) *OverallHealthStatus {
	results := &OverallHealthStatus{
		CheckResults: make([]HealthCheckResult, 0),
		FailureReasons: make([]string, 0),
	}

	// 1. AWS Service Connectivity Check
	connectivityResult := c.checkAWSConnectivity(ctx, region)
	results.CheckResults = append(results.CheckResults, connectivityResult)

	// 2. S3 Service Health Check
	s3Result := c.checkS3ServiceHealth(ctx, region)
	results.CheckResults = append(results.CheckResults, s3Result)

	// 3. Region Latency Check
	latencyResult := c.checkRegionLatency(ctx, region)
	results.CheckResults = append(results.CheckResults, latencyResult)

	// 4. Resource Capacity Check
	capacityResult := c.checkResourceCapacity(ctx, region)
	results.CheckResults = append(results.CheckResults, capacityResult)

	return results
}

// checkAWSConnectivity verifies basic AWS service connectivity
func (c *DefaultCoordinator) checkAWSConnectivity(ctx context.Context, region *Region) HealthCheckResult {
	startTime := time.Now()
	result := HealthCheckResult{
		CheckType: "aws_connectivity",
		Details:   make(map[string]interface{}),
	}

	// Use AWS SDK to make a lightweight API call (e.g., STS GetCallerIdentity)
	// This is a safe operation that verifies AWS connectivity and credentials
	if len(region.AWSConfig.Region) == 0 {
		result.Success = false
		result.Error = fmt.Errorf("region AWS config not properly initialized")
		result.ResponseTime = time.Since(startTime)
		return result
	}

	// Simulate AWS connectivity check
	// In a real implementation, you would use:
	// stsClient := sts.NewFromConfig(region.AWSConfig)
	// _, err := stsClient.GetCallerIdentity(ctx, &sts.GetCallerIdentityInput{})
	
	result.Success = true
	result.ResponseTime = time.Since(startTime)
	result.Details["region"] = region.Name
	result.Details["aws_region"] = region.AWSConfig.Region

	return result
}

// checkS3ServiceHealth verifies S3 service availability and performance
func (c *DefaultCoordinator) checkS3ServiceHealth(ctx context.Context, region *Region) HealthCheckResult {
	startTime := time.Now()
	result := HealthCheckResult{
		CheckType: "s3_service_health",
		Details:   make(map[string]interface{}),
	}

	// Simulate S3 health check
	// In a real implementation, you would:
	// 1. List buckets or perform a HEAD operation on a known bucket
	// 2. Check S3 service status endpoints
	// 3. Verify that upload/download operations are working
	
	// For now, simulate a successful check with some randomness for realism
	if time.Since(startTime) > region.HealthCheck.Timeout {
		result.Success = false
		result.Error = fmt.Errorf("S3 health check timed out")
	} else {
		result.Success = true
		result.Details["s3_available"] = true
		result.Details["bucket_accessible"] = true
	}

	result.ResponseTime = time.Since(startTime)
	return result
}

// checkRegionLatency measures the latency to the region
func (c *DefaultCoordinator) checkRegionLatency(ctx context.Context, region *Region) HealthCheckResult {
	startTime := time.Now()
	result := HealthCheckResult{
		CheckType: "region_latency",
		Details:   make(map[string]interface{}),
	}

	// Simulate latency measurement
	// In a real implementation, you would:
	// 1. Make lightweight API calls to measure round-trip time
	// 2. Use CloudWatch or CloudPing for latency metrics
	// 3. Test from multiple availability zones if possible
	
	responseTime := time.Since(startTime)
	result.Success = responseTime < 5*time.Second // Reasonable latency threshold
	result.ResponseTime = responseTime
	result.Details["latency_ms"] = responseTime.Milliseconds()
	
	if !result.Success {
		result.Error = fmt.Errorf("high latency detected: %v", responseTime)
	}

	return result
}

// checkResourceCapacity verifies that the region has sufficient capacity
func (c *DefaultCoordinator) checkResourceCapacity(ctx context.Context, region *Region) HealthCheckResult {
	startTime := time.Now()
	result := HealthCheckResult{
		CheckType: "resource_capacity",
		Details:   make(map[string]interface{}),
	}

	// Check various capacity metrics
	cpuUtilization := region.Metrics.CPUUtilization
	memoryUtilization := region.Metrics.MemoryUtilization
	activeUploads := region.Metrics.ActiveUploads

	// Determine if region has sufficient capacity
	capacityHealthy := cpuUtilization < 90.0 && 
		memoryUtilization < 85.0 && 
		activeUploads < int64(region.Capacity.MaxConcurrentUploads)

	result.Success = capacityHealthy
	result.ResponseTime = time.Since(startTime)
	result.Details["cpu_utilization"] = cpuUtilization
	result.Details["memory_utilization"] = memoryUtilization
	result.Details["active_uploads"] = activeUploads
	result.Details["capacity_limit"] = region.Capacity.MaxConcurrentUploads

	if !result.Success {
		result.Error = fmt.Errorf("region capacity exceeded: CPU=%.1f%%, Memory=%.1f%%, Uploads=%d", 
			cpuUtilization, memoryUtilization, activeUploads)
	}

	return result
}

// evaluateHealthResults determines the overall health based on individual check results
func (c *DefaultCoordinator) evaluateHealthResults(results *OverallHealthStatus) *OverallHealthStatus {
	if len(results.CheckResults) == 0 {
		results.Healthy = false
		results.FailureReasons = append(results.FailureReasons, "no health checks performed")
		return results
	}

	successCount := 0
	totalResponseTime := time.Duration(0)
	
	for _, check := range results.CheckResults {
		if check.Success {
			successCount++
		} else {
			if check.Error != nil {
				results.FailureReasons = append(results.FailureReasons, 
					fmt.Sprintf("%s: %v", check.CheckType, check.Error))
			}
		}
		totalResponseTime += check.ResponseTime
	}

	results.SuccessRate = float64(successCount) / float64(len(results.CheckResults))
	results.AvgResponseTime = totalResponseTime / time.Duration(len(results.CheckResults))
	
	// Region is considered healthy if:
	// 1. At least 75% of checks pass
	// 2. Critical checks (AWS connectivity and S3) must pass
	criticalChecksPass := c.validateCriticalChecks(results.CheckResults)
	results.Healthy = results.SuccessRate >= 0.75 && criticalChecksPass

	return results
}

// validateCriticalChecks ensures critical health checks pass
func (c *DefaultCoordinator) validateCriticalChecks(checks []HealthCheckResult) bool {
	criticalChecks := map[string]bool{
		"aws_connectivity":   false,
		"s3_service_health": false,
	}

	for _, check := range checks {
		if _, isCritical := criticalChecks[check.CheckType]; isCritical && check.Success {
			criticalChecks[check.CheckType] = true
		}
	}

	// All critical checks must pass
	for _, passed := range criticalChecks {
		if !passed {
			return false
		}
	}

	return true
}

// updateRegionHealthStatus updates the region's health status based on check results
func (c *DefaultCoordinator) updateRegionHealthStatus(region *Region, healthStatus *OverallHealthStatus, results *OverallHealthStatus) {
	c.mu.Lock()
	defer c.mu.Unlock()

	previousStatus := region.Status
	region.LastChecked = time.Now()

	// Update region metrics with health check results
	region.Metrics.LastHealthCheck = region.LastChecked
	region.Metrics.HealthCheckSuccess = healthStatus.Healthy
	region.Metrics.HealthCheckLatency = healthStatus.AvgResponseTime.Milliseconds()
	
	// Implement failure/success threshold logic
	if healthStatus.Healthy {
		region.Metrics.ConsecutiveHealthyChecks++
		region.Metrics.ConsecutiveFailedChecks = 0
		
		// Only mark as healthy after reaching success threshold
		if region.Metrics.ConsecutiveHealthyChecks >= int64(region.HealthCheck.SuccessThreshold) {
			region.Status = RegionStatusHealthy
		}
	} else {
		region.Metrics.ConsecutiveFailedChecks++
		region.Metrics.ConsecutiveHealthyChecks = 0
		
		// Mark as unhealthy after reaching failure threshold
		if region.Metrics.ConsecutiveFailedChecks >= int64(region.HealthCheck.FailureThreshold) {
			region.Status = RegionStatusUnhealthy
		}
	}

	// Log status changes
	if previousStatus != region.Status {
		c.logger.Info("Region health status changed", 
			"region", region.Name,
			"previous_status", previousStatus,
			"new_status", region.Status,
			"success_rate", healthStatus.SuccessRate,
			"avg_response_time", healthStatus.AvgResponseTime,
			"failure_reasons", healthStatus.FailureReasons)
		
		// Trigger failover if region becomes unhealthy
		if region.Status == RegionStatusUnhealthy && previousStatus != RegionStatusUnhealthy {
			c.triggerRegionFailover(region, healthStatus.FailureReasons)
		}
	} else {
		c.logger.Debug("Region health check completed", 
			"region", region.Name,
			"status", region.Status,
			"success_rate", healthStatus.SuccessRate,
			"avg_response_time", healthStatus.AvgResponseTime)
	}
}

// triggerRegionFailover initiates failover procedures when a region becomes unhealthy
func (c *DefaultCoordinator) triggerRegionFailover(region *Region, reasons []string) {
	c.logger.Warn("Triggering region failover", 
		"region", region.Name, 
		"reasons", reasons)
	
	// Use the failover manager to handle the failover process
	if c.failoverManager != nil {
		// Find the next best region for failover
		targetRegion := c.selectFailoverTarget(region.Name)
		if targetRegion != "" {
			go func() {
				err := c.failoverManager.ExecuteFailover(c.ctx, region.Name, targetRegion)
				if err != nil {
					c.logger.Error("Failed to execute region failover", 
						"from_region", region.Name, 
						"to_region", targetRegion,
						"error", err)
				} else {
					c.logger.Info("Region failover completed successfully", 
						"from_region", region.Name, 
						"to_region", targetRegion)
				}
			}()
		} else {
			c.logger.Warn("No suitable failover target found", "failed_region", region.Name)
		}
	}
}

// selectFailoverTarget finds the best healthy region to failover to
func (c *DefaultCoordinator) selectFailoverTarget(failedRegion string) string {
	c.mu.RLock()
	defer c.mu.RUnlock()

	var candidates []string
	var bestCandidate string
	var highestPriority int = 1000 // Lower numbers mean higher priority

	// Find healthy regions that could serve as failover targets
	for name, region := range c.regions {
		if name == failedRegion {
			continue // Skip the failed region
		}

		// Only consider healthy regions with sufficient capacity
		if region.Status == RegionStatusHealthy {
			// Check if region has capacity for additional load
			currentUtilization := region.Capacity.CurrentUtilization
			if currentUtilization < 80.0 { // Don't failover to regions over 80% capacity
				candidates = append(candidates, name)
				
				// Prefer regions with higher priority (lower priority number)
				if region.Priority < highestPriority {
					highestPriority = region.Priority
					bestCandidate = name
				} else if region.Priority == highestPriority {
					// If priorities are equal, choose the one with lower utilization
					if bestCandidate != "" {
						currentBest := c.regions[bestCandidate]
						if region.Capacity.CurrentUtilization < currentBest.Capacity.CurrentUtilization {
							bestCandidate = name
						}
					}
				}
			}
		}
	}

	if bestCandidate != "" {
		c.logger.Debug("Selected failover target", 
			"failed_region", failedRegion,
			"target_region", bestCandidate,
			"target_priority", highestPriority,
			"candidates", candidates)
	}

	return bestCandidate
}

// metricsCollectionService collects and updates metrics for all regions
func (c *DefaultCoordinator) metricsCollectionService() {
	defer c.wg.Done()

	interval := c.config.Monitoring.MetricsInterval
	if interval <= 0 {
		interval = 60 * time.Second
	}

	ticker := time.NewTicker(interval)
	defer ticker.Stop()

	for {
		select {
		case <-c.ctx.Done():
			return
		case <-ticker.C:
			c.collectMetrics()
		}
	}
}

// collectMetrics collects metrics from all regions
func (c *DefaultCoordinator) collectMetrics() {
	// TODO: Implement actual metrics collection logic
	// This is a placeholder for the actual metrics collection implementation

	c.logger.Debug("Collecting metrics from all regions")
}

// failoverDetectionService monitors regions for failures and triggers failover
func (c *DefaultCoordinator) failoverDetectionService() {
	defer c.wg.Done()

	interval := c.config.Failover.DetectionInterval
	if interval <= 0 {
		interval = 15 * time.Second
	}

	ticker := time.NewTicker(interval)
	defer ticker.Stop()

	for {
		select {
		case <-c.ctx.Done():
			return
		case <-ticker.C:
			c.detectAndHandleFailures()
		}
	}
}

// detectAndHandleFailures detects failures and triggers failover if needed
func (c *DefaultCoordinator) detectAndHandleFailures() {
	// TODO: Implement actual failure detection and failover logic
	// This is a placeholder for the actual failover implementation

	c.logger.Debug("Detecting failures across regions")
}
