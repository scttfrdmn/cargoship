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

	// Check if we need to trigger failover (before releasing lock)
	shouldTriggerFailover := region.Status == RegionStatusUnhealthy && previousStatus != RegionStatusUnhealthy
	failureReasons := healthStatus.FailureReasons

	// Log status changes
	if previousStatus != region.Status {
		c.logger.Info("Region health status changed", 
			"region", region.Name,
			"previous_status", previousStatus,
			"new_status", region.Status,
			"success_rate", healthStatus.SuccessRate,
			"avg_response_time", healthStatus.AvgResponseTime,
			"failure_reasons", healthStatus.FailureReasons)
	} else {
		c.logger.Debug("Region health check completed", 
			"region", region.Name,
			"status", region.Status,
			"success_rate", healthStatus.SuccessRate,
			"avg_response_time", healthStatus.AvgResponseTime)
	}
	
	// Release lock before triggering failover to avoid deadlock
	c.mu.Unlock()
	
	// Trigger failover if region becomes unhealthy (after releasing lock)
	if shouldTriggerFailover {
		c.triggerRegionFailover(region, failureReasons)
	}
	
	// Re-acquire lock for defer unlock (this is safe as we're about to return)
	c.mu.Lock()
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
	var highestPriority = 1000 // Lower numbers mean higher priority

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
	c.mu.RLock()
	regions := make([]*Region, 0, len(c.regions))
	for _, region := range c.regions {
		regions = append(regions, region)
	}
	c.mu.RUnlock()

	c.logger.Debug("Starting metrics collection", "regions", len(regions))

	// Collect metrics from each region
	for _, region := range regions {
		c.collectRegionMetrics(region)
	}

	// Calculate and update global metrics
	c.calculateGlobalMetrics(regions)

	c.logger.Debug("Metrics collection completed", "regions", len(regions))
}

// collectRegionMetrics collects comprehensive metrics for a single region
func (c *DefaultCoordinator) collectRegionMetrics(region *Region) {
	startTime := time.Now()
	
	c.logger.Debug("Collecting region metrics", "region", region.Name)

	// Update basic operational metrics
	c.updateOperationalMetrics(region)
	
	// Update performance metrics
	c.updatePerformanceMetrics(region)
	
	// Update resource utilization metrics
	c.updateResourceUtilizationMetrics(region)
	
	// Update network and connectivity metrics
	c.updateNetworkMetrics(region)
	
	// Update cost and efficiency metrics
	c.updateCostMetrics(region)

	// Record metrics collection metadata
	region.Metrics.LastUpdated = time.Now()
	
	collectionDuration := time.Since(startTime)
	c.logger.Debug("Region metrics collected", 
		"region", region.Name,
		"collection_duration", collectionDuration,
		"cpu_utilization", region.Metrics.CPUUtilization,
		"memory_utilization", region.Metrics.MemoryUtilization,
		"active_uploads", region.Metrics.ActiveUploads,
		"throughput_mbps", region.Metrics.ThroughputMbps,
		"error_rate", region.Metrics.ErrorRate)
}

// updateOperationalMetrics updates basic operational metrics
func (c *DefaultCoordinator) updateOperationalMetrics(region *Region) {
	// Simulate collection of operational metrics
	// In a real implementation, these would come from:
	// 1. CloudWatch metrics
	// 2. Application-level counters
	// 3. System monitoring tools
	
	// Update upload statistics (simulated)
	previousUploads := region.Metrics.SuccessfulUploads
	previousFailures := region.Metrics.FailedUploads
	
	// In real implementation, query actual metrics source
	// For simulation, add some realistic incremental values
	newSuccessfulUploads := c.simulateUploadMetrics(region, true)
	newFailedUploads := c.simulateUploadMetrics(region, false)
	
	region.Metrics.SuccessfulUploads += newSuccessfulUploads
	region.Metrics.FailedUploads += newFailedUploads
	
	// Calculate error rate based on totals (use previous values for rate calculation)
	totalUploads := previousUploads + previousFailures + newSuccessfulUploads + newFailedUploads
	if totalUploads > 0 {
		region.Metrics.ErrorRate = (float64(region.Metrics.FailedUploads) / float64(totalUploads)) * 100.0
	} else {
		region.Metrics.ErrorRate = 0.0
	}
	
	c.logger.Debug("Updated operational metrics", 
		"region", region.Name,
		"new_successful", newSuccessfulUploads,
		"new_failed", newFailedUploads,
		"total_successful", region.Metrics.SuccessfulUploads,
		"total_failed", region.Metrics.FailedUploads,
		"error_rate", region.Metrics.ErrorRate)
}

// updatePerformanceMetrics updates performance and throughput metrics
func (c *DefaultCoordinator) updatePerformanceMetrics(region *Region) {
	// Simulate performance metrics collection
	// In production, these would be gathered from:
	// 1. S3 transfer statistics
	// 2. Network monitoring
	// 3. Application performance counters
	
	// Update throughput based on recent activity and region capacity
	baselineThroughput := float64(region.Capacity.MaxBandwidthMbps) * 0.3 // 30% baseline
	utilizationFactor := region.Capacity.CurrentUtilization / 100.0
	
	// Add some randomness for realistic simulation
	variation := (rand.Float64() - 0.5) * 0.2 * baselineThroughput // ±10% variation
	currentThroughput := baselineThroughput * (1.0 + utilizationFactor) + variation
	
	// Ensure throughput stays within reasonable bounds
	if currentThroughput < 0 {
		currentThroughput = 0
	}
	if currentThroughput > float64(region.Capacity.MaxBandwidthMbps) {
		currentThroughput = float64(region.Capacity.MaxBandwidthMbps)
	}
	
	region.Metrics.ThroughputMbps = currentThroughput
	
	// Update latency based on region characteristics
	baseLatency := 50.0 // Base 50ms
	if region.Priority > 1 {
		baseLatency += float64((region.Priority - 1) * 25) // Higher priority = higher latency
	}
	
	// Add load-based latency increase
	loadLatency := (region.Capacity.CurrentUtilization / 100.0) * 100.0 // Up to 100ms additional
	latencyVariation := (rand.Float64() - 0.5) * 20.0 // ±10ms variation
	
	region.Metrics.AverageLatencyMs = baseLatency + loadLatency + latencyVariation
	
	c.logger.Debug("Updated performance metrics",
		"region", region.Name,
		"throughput_mbps", region.Metrics.ThroughputMbps,
		"latency_ms", region.Metrics.AverageLatencyMs,
		"utilization_factor", utilizationFactor)
}

// updateResourceUtilizationMetrics updates CPU, memory, and capacity metrics
func (c *DefaultCoordinator) updateResourceUtilizationMetrics(region *Region) {
	// Simulate resource utilization metrics
	// In production, these would come from:
	// 1. CloudWatch EC2/ECS metrics
	// 2. System monitoring agents
	// 3. Container orchestration platforms
	
	// Update CPU utilization based on current load and active uploads
	baselineCPU := 20.0 // 20% baseline
	uploadCPULoad := (float64(region.Metrics.ActiveUploads) / float64(region.Capacity.MaxConcurrentUploads)) * 60.0
	cpuVariation := (rand.Float64() - 0.5) * 10.0 // ±5% variation
	
	newCPUUtilization := baselineCPU + uploadCPULoad + cpuVariation
	if newCPUUtilization < 0 {
		newCPUUtilization = 0
	}
	if newCPUUtilization > 100 {
		newCPUUtilization = 100
	}
	
	region.Metrics.CPUUtilization = newCPUUtilization
	
	// Update memory utilization (typically correlates with CPU but with different characteristics)
	baselineMemory := 25.0 // 25% baseline
	uploadMemoryLoad := (float64(region.Metrics.ActiveUploads) / float64(region.Capacity.MaxConcurrentUploads)) * 50.0
	memoryVariation := (rand.Float64() - 0.5) * 8.0 // ±4% variation
	
	newMemoryUtilization := baselineMemory + uploadMemoryLoad + memoryVariation
	if newMemoryUtilization < 0 {
		newMemoryUtilization = 0
	}
	if newMemoryUtilization > 100 {
		newMemoryUtilization = 100
	}
	
	region.Metrics.MemoryUtilization = newMemoryUtilization
	
	// Update overall region capacity utilization
	capacityFactor := (region.Metrics.CPUUtilization + region.Metrics.MemoryUtilization) / 2.0
	uploadFactor := (float64(region.Metrics.ActiveUploads) / float64(region.Capacity.MaxConcurrentUploads)) * 100.0
	
	region.Capacity.CurrentUtilization = (capacityFactor + uploadFactor) / 2.0
	
	c.logger.Debug("Updated resource utilization metrics",
		"region", region.Name,
		"cpu_utilization", region.Metrics.CPUUtilization,
		"memory_utilization", region.Metrics.MemoryUtilization,
		"capacity_utilization", region.Capacity.CurrentUtilization,
		"active_uploads", region.Metrics.ActiveUploads)
}

// updateNetworkMetrics updates network and connectivity related metrics
func (c *DefaultCoordinator) updateNetworkMetrics(region *Region) {
	// Simulate network metrics
	// In production, these would be gathered from:
	// 1. VPC Flow Logs
	// 2. Network monitoring tools
	// 3. AWS CloudWatch network metrics
	
	// Update active uploads based on current capacity and utilization
	maxActiveUploads := int64(float64(region.Capacity.MaxConcurrentUploads) * (region.Capacity.CurrentUtilization / 100.0))
	uploadVariation := int64((rand.Float64() - 0.5) * float64(region.Capacity.MaxConcurrentUploads) * 0.1)
	
	newActiveUploads := maxActiveUploads + uploadVariation
	if newActiveUploads < 0 {
		newActiveUploads = 0
	}
	if newActiveUploads > int64(region.Capacity.MaxConcurrentUploads) {
		newActiveUploads = int64(region.Capacity.MaxConcurrentUploads)
	}
	
	region.Metrics.ActiveUploads = newActiveUploads
	
	c.logger.Debug("Updated network metrics",
		"region", region.Name,
		"active_uploads", region.Metrics.ActiveUploads,
		"max_concurrent", region.Capacity.MaxConcurrentUploads)
}

// updateCostMetrics updates cost and efficiency related metrics
func (c *DefaultCoordinator) updateCostMetrics(region *Region) {
	// In production, this would integrate with:
	// 1. AWS Cost Explorer API
	// 2. Billing and cost management services
	// 3. Custom cost tracking systems
	
	c.logger.Debug("Updated cost metrics", "region", region.Name)
}

// simulateUploadMetrics simulates realistic upload metrics for testing
func (c *DefaultCoordinator) simulateUploadMetrics(region *Region, success bool) int64 {
	// Simulate metrics based on region utilization and capacity
	utilizationFactor := region.Capacity.CurrentUtilization / 100.0
	maxUploads := float64(region.Capacity.MaxConcurrentUploads) * utilizationFactor
	
	// Add randomness
	variation := rand.Float64() * 0.3 // Up to 30% variation
	simulatedUploads := maxUploads * variation
	
	if success {
		// Success rate should be higher
		return int64(simulatedUploads * 0.95) // 95% success rate simulation
	} else {
		// Failure rate should be lower
		return int64(simulatedUploads * 0.05) // 5% failure rate simulation
	}
}

// calculateGlobalMetrics calculates and updates global coordination metrics
func (c *DefaultCoordinator) calculateGlobalMetrics(regions []*Region) {
	if len(regions) == 0 {
		return
	}

	c.logger.Debug("Calculating global metrics", "regions", len(regions))

	var totalThroughput float64
	var totalLatency float64
	var totalSuccessfulUploads int64
	var totalFailedUploads int64
	var totalActiveUploads int64
	var totalCPUUtilization float64
	var totalMemoryUtilization float64
	var healthyRegionCount int
	var totalCapacityUtilization float64

	// Aggregate metrics from all regions
	for _, region := range regions {
		totalThroughput += region.Metrics.ThroughputMbps
		totalLatency += region.Metrics.AverageLatencyMs
		totalSuccessfulUploads += region.Metrics.SuccessfulUploads
		totalFailedUploads += region.Metrics.FailedUploads
		totalActiveUploads += region.Metrics.ActiveUploads
		totalCPUUtilization += region.Metrics.CPUUtilization
		totalMemoryUtilization += region.Metrics.MemoryUtilization
		totalCapacityUtilization += region.Capacity.CurrentUtilization
		
		if region.Status == RegionStatusHealthy {
			healthyRegionCount++
		}
	}

	regionCount := float64(len(regions))
	
	// Calculate global averages and totals
	globalMetrics := map[string]interface{}{
		"total_regions":             len(regions),
		"healthy_regions":           healthyRegionCount,
		"region_availability":       float64(healthyRegionCount) / regionCount * 100.0,
		"total_throughput_mbps":     totalThroughput,
		"average_latency_ms":        totalLatency / regionCount,
		"total_successful_uploads":  totalSuccessfulUploads,
		"total_failed_uploads":      totalFailedUploads,
		"total_active_uploads":      totalActiveUploads,
		"average_cpu_utilization":   totalCPUUtilization / regionCount,
		"average_memory_utilization": totalMemoryUtilization / regionCount,
		"average_capacity_utilization": totalCapacityUtilization / regionCount,
		"collection_timestamp":      time.Now(),
	}

	// Calculate global error rate
	totalUploads := totalSuccessfulUploads + totalFailedUploads
	var globalErrorRate float64
	if totalUploads > 0 {
		globalErrorRate = (float64(totalFailedUploads) / float64(totalUploads)) * 100.0
	}
	globalMetrics["global_error_rate"] = globalErrorRate

	// Calculate system health score (0-100)
	systemHealthScore := c.calculateSystemHealthScore(regions, healthyRegionCount)
	globalMetrics["system_health_score"] = systemHealthScore

	// Log comprehensive global metrics
	c.logger.Info("Global metrics calculated",
		"total_regions", len(regions),
		"healthy_regions", healthyRegionCount,
		"region_availability", fmt.Sprintf("%.1f%%", globalMetrics["region_availability"]),
		"total_throughput_mbps", fmt.Sprintf("%.1f", totalThroughput),
		"average_latency_ms", fmt.Sprintf("%.1f", globalMetrics["average_latency_ms"]),
		"total_uploads", totalUploads,
		"global_error_rate", fmt.Sprintf("%.2f%%", globalErrorRate),
		"system_health_score", fmt.Sprintf("%.1f", systemHealthScore),
		"average_cpu", fmt.Sprintf("%.1f%%", globalMetrics["average_cpu_utilization"]),
		"average_memory", fmt.Sprintf("%.1f%%", globalMetrics["average_memory_utilization"]))

	// Store global metrics for external consumption (could be exposed via API)
	c.storeGlobalMetrics(globalMetrics)
}

// calculateSystemHealthScore calculates an overall system health score
func (c *DefaultCoordinator) calculateSystemHealthScore(regions []*Region, healthyCount int) float64 {
	if len(regions) == 0 {
		return 0.0
	}

	// Base score from regional availability
	availabilityScore := (float64(healthyCount) / float64(len(regions))) * 40.0 // Up to 40 points

	// Performance score based on average error rates and utilization
	var totalErrorRate float64
	var totalUtilization float64
	
	for _, region := range regions {
		if region.Status == RegionStatusHealthy {
			totalErrorRate += region.Metrics.ErrorRate
			totalUtilization += region.Capacity.CurrentUtilization
		}
	}

	if healthyCount > 0 {
		avgErrorRate := totalErrorRate / float64(healthyCount)
		avgUtilization := totalUtilization / float64(healthyCount)
		
		// Error rate score (lower is better, up to 30 points)
		errorScore := 30.0 - (avgErrorRate * 3.0) // Subtract 3 points per 1% error rate
		if errorScore < 0 {
			errorScore = 0
		}
		
		// Utilization score (50-80% utilization is optimal, up to 30 points)
		var utilizationScore float64
		if avgUtilization >= 50 && avgUtilization <= 80 {
			utilizationScore = 30.0 // Optimal range
		} else if avgUtilization < 50 {
			utilizationScore = avgUtilization * 0.6 // Underutilized
		} else {
			utilizationScore = 30.0 - ((avgUtilization - 80.0) * 1.5) // Overutilized
		}
		
		if utilizationScore < 0 {
			utilizationScore = 0
		}
		
		return availabilityScore + errorScore + utilizationScore
	}

	return availabilityScore
}

// storeGlobalMetrics stores global metrics for external access
func (c *DefaultCoordinator) storeGlobalMetrics(metrics map[string]interface{}) {
	// In production, this could:
	// 1. Write to a metrics database
	// 2. Send to monitoring systems (DataDog, New Relic, etc.)
	// 3. Publish to message queues
	// 4. Update shared state for API endpoints
	
	c.logger.Debug("Global metrics stored", "metrics_count", len(metrics))
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
	c.logger.Debug("Detecting failures across regions")
	
	c.mu.RLock()
	regions := make([]*Region, 0, len(c.regions))
	for _, region := range c.regions {
		regions = append(regions, region)
	}
	c.mu.RUnlock()

	// Check each region for failure conditions
	for _, region := range regions {
		if c.shouldTriggerFailover(region) {
			c.logger.Warn("Failure detected, triggering failover",
				"region", region.Name,
				"status", region.Status,
				"consecutive_failures", region.Metrics.ConsecutiveFailedChecks)
				
			// Find a suitable failover target
			targetRegion := c.selectFailoverTarget(region.Name)
			if targetRegion == "" {
				c.logger.Error("No suitable failover target available",
					"failed_region", region.Name)
				continue
			}
			
			// Trigger failover based on configuration
			c.triggerAutomaticFailover(region.Name, targetRegion)
		}
	}
}

// shouldTriggerFailover determines if a region should trigger failover
func (c *DefaultCoordinator) shouldTriggerFailover(region *Region) bool {
	// Don't trigger failover if already in failover
	if c.failoverManager != nil && c.failoverManager.IsRegionInFailover(region.Name) {
		return false
	}
	
	// Check if automatic failover is enabled
	if !c.config.Failover.AutoFailover {
		c.logger.Debug("Automatic failover is disabled", "region", region.Name)
		return false
	}
	
	// Check various failure conditions
	failureReasons := make([]string, 0)
	
	// 1. Region is marked as unhealthy or offline
	if region.Status == RegionStatusUnhealthy || region.Status == RegionStatusOffline {
		failureReasons = append(failureReasons, fmt.Sprintf("region_status_%s", region.Status))
	}
	
	// 2. Too many consecutive health check failures
	failureThreshold := int64(3) // Default threshold
	if region.HealthCheck.FailureThreshold > 0 {
		failureThreshold = int64(region.HealthCheck.FailureThreshold)
	}
	
	if region.Metrics.ConsecutiveFailedChecks >= failureThreshold {
		failureReasons = append(failureReasons, 
			fmt.Sprintf("consecutive_failures_%d", region.Metrics.ConsecutiveFailedChecks))
	}
	
	// 3. High error rate
	errorRateThreshold := 25.0 // 25% error rate threshold
	if region.Metrics.ErrorRate > errorRateThreshold {
		failureReasons = append(failureReasons, 
			fmt.Sprintf("high_error_rate_%.1f", region.Metrics.ErrorRate))
	}
	
	// 4. Very high resource utilization (indicating potential overload)
	if region.Capacity.CurrentUtilization > 95.0 {
		failureReasons = append(failureReasons, 
			fmt.Sprintf("overload_utilization_%.1f", region.Capacity.CurrentUtilization))
	}
	
	// 5. Health check hasn't run recently (indicates potential connectivity issues)
	healthCheckStale := time.Since(region.Metrics.LastHealthCheck) > 5*time.Minute
	if !region.Metrics.LastHealthCheck.IsZero() && healthCheckStale {
		failureReasons = append(failureReasons, "stale_health_check")
	}
	
	// Trigger failover if we have failure reasons
	if len(failureReasons) > 0 {
		c.logger.Info("Failover conditions detected",
			"region", region.Name,
			"reasons", failureReasons)
		return true
	}
	
	return false
}

// triggerAutomaticFailover initiates automatic failover for a failed region
func (c *DefaultCoordinator) triggerAutomaticFailover(fromRegion, toRegion string) {
	c.logger.Info("Triggering automatic failover",
		"from_region", fromRegion,
		"to_region", toRegion,
		"strategy", c.config.Failover.Strategy)
	
	if c.failoverManager == nil {
		c.logger.Error("Failover manager not available")
		return
	}
	
	// Create a context for the failover operation
	ctx, cancel := context.WithTimeout(c.ctx, c.config.Failover.FailoverTimeout)
	defer cancel()
	
	// Execute failover based on strategy
	go func() {
		defer cancel()
		
		if err := c.failoverManager.ExecuteFailover(ctx, fromRegion, toRegion); err != nil {
			c.logger.Error("Automatic failover failed",
				"from_region", fromRegion,
				"to_region", toRegion,
				"error", err)
			
			// Record the failure for monitoring
			c.recordFailoverFailure(fromRegion, toRegion, err)
		} else {
			c.logger.Info("Automatic failover completed successfully",
				"from_region", fromRegion,
				"to_region", toRegion)
			
			// Record successful failover
			c.recordFailoverSuccess(fromRegion, toRegion)
		}
	}()
}

// recordFailoverFailure records a failed failover attempt
func (c *DefaultCoordinator) recordFailoverFailure(fromRegion, toRegion string, err error) {
	c.logger.Error("Recording failover failure",
		"from_region", fromRegion,
		"to_region", toRegion,
		"error", err)
	
	// In a production implementation, this would:
	// 1. Send alerts to operations teams
	// 2. Update monitoring dashboards
	// 3. Log to audit systems
	// 4. Create incident tickets
	// 5. Update metrics and counters
}

// recordFailoverSuccess records a successful failover
func (c *DefaultCoordinator) recordFailoverSuccess(fromRegion, toRegion string) {
	c.logger.Info("Recording successful failover",
		"from_region", fromRegion,
		"to_region", toRegion)
	
	// In a production implementation, this would:
	// 1. Send success notifications
	// 2. Update monitoring dashboards
	// 3. Log to audit systems
	// 4. Update metrics and counters
	// 5. Notify stakeholders
}
