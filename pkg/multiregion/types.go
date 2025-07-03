// Package multiregion provides multi-region pipeline distribution and coordination for CargoShip
package multiregion

import (
	"context"
	"time"

	"github.com/aws/aws-sdk-go-v2/aws"
)

// RegionStatus represents the operational status of a region
type RegionStatus string

const (
	// RegionStatusHealthy indicates the region is fully operational
	RegionStatusHealthy RegionStatus = "healthy"
	
	// RegionStatusDegraded indicates the region has performance issues
	RegionStatusDegraded RegionStatus = "degraded"
	
	// RegionStatusUnhealthy indicates the region is experiencing problems
	RegionStatusUnhealthy RegionStatus = "unhealthy"
	
	// RegionStatusOffline indicates the region is not available
	RegionStatusOffline RegionStatus = "offline"
)

// LoadBalancingStrategy defines how traffic is distributed across regions
type LoadBalancingStrategy string

const (
	// LoadBalancingRoundRobin distributes requests evenly across regions
	LoadBalancingRoundRobin LoadBalancingStrategy = "round_robin"
	
	// LoadBalancingWeighted distributes based on regional capacity/performance
	LoadBalancingWeighted LoadBalancingStrategy = "weighted"
	
	// LoadBalancingLatency routes to lowest latency region
	LoadBalancingLatency LoadBalancingStrategy = "latency"
	
	// LoadBalancingGeographic routes based on geographic proximity
	LoadBalancingGeographic LoadBalancingStrategy = "geographic"
)

// FailoverStrategy defines how failover is handled
type FailoverStrategy string

const (
	// FailoverImmediate switches to backup region immediately on failure
	FailoverImmediate FailoverStrategy = "immediate"
	
	// FailoverGraceful attempts graceful degradation before switching
	FailoverGraceful FailoverStrategy = "graceful"
	
	// FailoverManual requires manual intervention to switch regions
	FailoverManual FailoverStrategy = "manual"
)

// Region represents a deployment region with its configuration and status
type Region struct {
	// Name is the region identifier (e.g., "us-east-1")
	Name string `json:"name" yaml:"name"`
	
	// DisplayName is the human-readable region name
	DisplayName string `json:"display_name" yaml:"display_name"`
	
	// AWSConfig contains AWS-specific configuration for this region
	AWSConfig aws.Config `json:"-" yaml:"-"`
	
	// Status indicates the current operational status
	Status RegionStatus `json:"status" yaml:"status"`
	
	// Priority determines the preference order (1 = highest priority)
	Priority int `json:"priority" yaml:"priority"`
	
	// Weight for load balancing (0-100)
	Weight int `json:"weight" yaml:"weight"`
	
	// Capacity indicates the region's processing capacity
	Capacity RegionCapacity `json:"capacity" yaml:"capacity"`
	
	// HealthCheck configuration for this region
	HealthCheck HealthCheckConfig `json:"health_check" yaml:"health_check"`
	
	// Metrics contains performance and operational metrics
	Metrics RegionMetrics `json:"metrics" yaml:"metrics"`
	
	// LastChecked timestamp of the last health check
	LastChecked time.Time `json:"last_checked" yaml:"last_checked"`
	
	// CreatedAt timestamp when this region was added
	CreatedAt time.Time `json:"created_at" yaml:"created_at"`
	
	// UpdatedAt timestamp when this region was last updated
	UpdatedAt time.Time `json:"updated_at" yaml:"updated_at"`
}

// RegionCapacity represents the operational capacity of a region
type RegionCapacity struct {
	// MaxConcurrentUploads maximum number of concurrent uploads
	MaxConcurrentUploads int `json:"max_concurrent_uploads" yaml:"max_concurrent_uploads"`
	
	// MaxBandwidthMbps maximum bandwidth in Mbps
	MaxBandwidthMbps int `json:"max_bandwidth_mbps" yaml:"max_bandwidth_mbps"`
	
	// MaxStorageGB maximum storage capacity in GB
	MaxStorageGB int64 `json:"max_storage_gb" yaml:"max_storage_gb"`
	
	// CurrentUtilization current usage as percentage (0-100)
	CurrentUtilization float64 `json:"current_utilization" yaml:"current_utilization"`
}

// HealthCheckConfig defines health check parameters for a region
type HealthCheckConfig struct {
	// Enabled indicates if health checks are active
	Enabled bool `json:"enabled" yaml:"enabled"`
	
	// Interval between health checks
	Interval time.Duration `json:"interval" yaml:"interval"`
	
	// Timeout for each health check
	Timeout time.Duration `json:"timeout" yaml:"timeout"`
	
	// FailureThreshold number of consecutive failures before marking unhealthy
	FailureThreshold int `json:"failure_threshold" yaml:"failure_threshold"`
	
	// SuccessThreshold number of consecutive successes before marking healthy
	SuccessThreshold int `json:"success_threshold" yaml:"success_threshold"`
	
	// Endpoint URL for health check requests
	Endpoint string `json:"endpoint" yaml:"endpoint"`
}

// RegionMetrics contains performance and operational metrics for a region
type RegionMetrics struct {
	// AverageLatencyMs average response latency in milliseconds
	AverageLatencyMs float64 `json:"average_latency_ms" yaml:"average_latency_ms"`
	
	// ThroughputMbps current throughput in Mbps
	ThroughputMbps float64 `json:"throughput_mbps" yaml:"throughput_mbps"`
	
	// ErrorRate percentage of failed requests (0-100)
	ErrorRate float64 `json:"error_rate" yaml:"error_rate"`
	
	// SuccessfulUploads number of successful uploads in the last period
	SuccessfulUploads int64 `json:"successful_uploads" yaml:"successful_uploads"`
	
	// FailedUploads number of failed uploads in the last period
	FailedUploads int64 `json:"failed_uploads" yaml:"failed_uploads"`
	
	// LastUpdated timestamp when metrics were last updated
	LastUpdated time.Time `json:"last_updated" yaml:"last_updated"`
}

// MultiRegionConfig contains configuration for multi-region coordination
type MultiRegionConfig struct {
	// Enabled indicates if multi-region support is active
	Enabled bool `json:"enabled" yaml:"enabled"`
	
	// PrimaryRegion is the preferred region for operations
	PrimaryRegion string `json:"primary_region" yaml:"primary_region"`
	
	// Regions list of configured regions
	Regions []Region `json:"regions" yaml:"regions"`
	
	// LoadBalancing configuration
	LoadBalancing LoadBalancingConfig `json:"load_balancing" yaml:"load_balancing"`
	
	// Failover configuration
	Failover FailoverConfig `json:"failover" yaml:"failover"`
	
	// Replication configuration for cross-region data replication
	Replication ReplicationConfig `json:"replication" yaml:"replication"`
	
	// Monitoring configuration
	Monitoring MonitoringConfig `json:"monitoring" yaml:"monitoring"`
}

// LoadBalancingConfig defines load balancing behavior
type LoadBalancingConfig struct {
	// Strategy determines the load balancing algorithm
	Strategy LoadBalancingStrategy `json:"strategy" yaml:"strategy"`
	
	// HealthCheckInterval interval for checking region health
	HealthCheckInterval time.Duration `json:"health_check_interval" yaml:"health_check_interval"`
	
	// StickySessions enables session affinity to regions
	StickySessions bool `json:"sticky_sessions" yaml:"sticky_sessions"`
	
	// SessionTTL duration for session stickiness
	SessionTTL time.Duration `json:"session_ttl" yaml:"session_ttl"`
}

// FailoverConfig defines failover behavior
type FailoverConfig struct {
	// Strategy determines failover behavior
	Strategy FailoverStrategy `json:"strategy" yaml:"strategy"`
	
	// DetectionInterval how often to check for failures
	DetectionInterval time.Duration `json:"detection_interval" yaml:"detection_interval"`
	
	// AutoFailover enables automatic failover
	AutoFailover bool `json:"auto_failover" yaml:"auto_failover"`
	
	// FailoverTimeout maximum time to wait for failover completion
	FailoverTimeout time.Duration `json:"failover_timeout" yaml:"failover_timeout"`
	
	// RetryAttempts number of retry attempts before failover
	RetryAttempts int `json:"retry_attempts" yaml:"retry_attempts"`
}

// ReplicationConfig defines cross-region replication settings
type ReplicationConfig struct {
	// Enabled indicates if replication is active
	Enabled bool `json:"enabled" yaml:"enabled"`
	
	// ReplicationMode determines replication behavior
	ReplicationMode ReplicationMode `json:"replication_mode" yaml:"replication_mode"`
	
	// ReplicationLag acceptable lag between regions
	ReplicationLag time.Duration `json:"replication_lag" yaml:"replication_lag"`
	
	// ConflictResolution strategy for handling conflicts
	ConflictResolution ConflictResolutionStrategy `json:"conflict_resolution" yaml:"conflict_resolution"`
}

// ReplicationMode defines how data is replicated across regions
type ReplicationMode string

const (
	// ReplicationSync synchronous replication (strong consistency)
	ReplicationSync ReplicationMode = "sync"
	
	// ReplicationAsync asynchronous replication (eventual consistency)
	ReplicationAsync ReplicationMode = "async"
	
	// ReplicationNone no replication
	ReplicationNone ReplicationMode = "none"
)

// ConflictResolutionStrategy defines how to handle replication conflicts
type ConflictResolutionStrategy string

const (
	// ConflictResolutionLastWrite last write wins
	ConflictResolutionLastWrite ConflictResolutionStrategy = "last_write"
	
	// ConflictResolutionFirstWrite first write wins
	ConflictResolutionFirstWrite ConflictResolutionStrategy = "first_write"
	
	// ConflictResolutionManual manual conflict resolution required
	ConflictResolutionManual ConflictResolutionStrategy = "manual"
)

// MonitoringConfig defines monitoring and alerting settings
type MonitoringConfig struct {
	// Enabled indicates if monitoring is active
	Enabled bool `json:"enabled" yaml:"enabled"`
	
	// MetricsInterval how often to collect metrics
	MetricsInterval time.Duration `json:"metrics_interval" yaml:"metrics_interval"`
	
	// AlertingEnabled enables alert notifications
	AlertingEnabled bool `json:"alerting_enabled" yaml:"alerting_enabled"`
	
	// AlertThresholds defines alert conditions
	AlertThresholds AlertThresholds `json:"alert_thresholds" yaml:"alert_thresholds"`
}

// AlertThresholds defines conditions that trigger alerts
type AlertThresholds struct {
	// HighLatencyMs latency threshold in milliseconds
	HighLatencyMs float64 `json:"high_latency_ms" yaml:"high_latency_ms"`
	
	// HighErrorRate error rate threshold (0-100)
	HighErrorRate float64 `json:"high_error_rate" yaml:"high_error_rate"`
	
	// LowThroughputMbps throughput threshold in Mbps
	LowThroughputMbps float64 `json:"low_throughput_mbps" yaml:"low_throughput_mbps"`
	
	// HighUtilization utilization threshold (0-100)
	HighUtilization float64 `json:"high_utilization" yaml:"high_utilization"`
}

// UploadRequest represents a multi-region upload request
type UploadRequest struct {
	// ID unique identifier for the upload request
	ID string `json:"id" yaml:"id"`
	
	// FilePath path to the file being uploaded
	FilePath string `json:"file_path" yaml:"file_path"`
	
	// DestinationKey S3 key for the uploaded file
	DestinationKey string `json:"destination_key" yaml:"destination_key"`
	
	// Size file size in bytes
	Size int64 `json:"size" yaml:"size"`
	
	// PreferredRegion preferred region for upload (optional)
	PreferredRegion string `json:"preferred_region" yaml:"preferred_region"`
	
	// Priority upload priority (1-10, higher is more urgent)
	Priority int `json:"priority" yaml:"priority"`
	
	// Context for the upload operation
	Context context.Context `json:"-" yaml:"-"`
	
	// Metadata additional metadata for the upload
	Metadata map[string]string `json:"metadata" yaml:"metadata"`
	
	// CreatedAt timestamp when the request was created
	CreatedAt time.Time `json:"created_at" yaml:"created_at"`
}

// UploadResult represents the result of a multi-region upload
type UploadResult struct {
	// RequestID the original request ID
	RequestID string `json:"request_id" yaml:"request_id"`
	
	// Region the region where the upload was executed
	Region string `json:"region" yaml:"region"`
	
	// Success indicates if the upload was successful
	Success bool `json:"success" yaml:"success"`
	
	// Error contains error information if the upload failed
	Error error `json:"error" yaml:"error"`
	
	// Duration how long the upload took
	Duration time.Duration `json:"duration" yaml:"duration"`
	
	// BytesTransferred number of bytes transferred
	BytesTransferred int64 `json:"bytes_transferred" yaml:"bytes_transferred"`
	
	// UploadID S3 upload ID for multipart uploads
	UploadID string `json:"upload_id" yaml:"upload_id"`
	
	// ETag S3 ETag of the uploaded object
	ETag string `json:"etag" yaml:"etag"`
	
	// CompletedAt timestamp when the upload completed
	CompletedAt time.Time `json:"completed_at" yaml:"completed_at"`
}

// RegionSelector defines the interface for region selection logic
type RegionSelector interface {
	// SelectRegion selects the best region for an upload request
	SelectRegion(ctx context.Context, request *UploadRequest) (*Region, error)
	
	// SelectRegions selects multiple regions for redundant uploads
	SelectRegions(ctx context.Context, request *UploadRequest, count int) ([]*Region, error)
	
	// UpdateRegionMetrics updates metrics for a region
	UpdateRegionMetrics(ctx context.Context, regionName string, metrics RegionMetrics) error
}

// LoadBalancer defines the interface for load balancing across regions
type LoadBalancer interface {
	// Route routes an upload request to the most appropriate region
	Route(ctx context.Context, request *UploadRequest) (*Region, error)
	
	// GetAvailableRegions returns list of healthy regions
	GetAvailableRegions(ctx context.Context) ([]*Region, error)
	
	// UpdateRegionStatus updates the status of a region
	UpdateRegionStatus(ctx context.Context, regionName string, status RegionStatus) error
}

// FailoverManager defines the interface for failover management
type FailoverManager interface {
	// DetectFailure detects if a region has failed
	DetectFailure(ctx context.Context, regionName string) (bool, error)
	
	// ExecuteFailover performs failover to backup region
	ExecuteFailover(ctx context.Context, fromRegion, toRegion string) error
	
	// GetFailoverStatus gets the current failover status
	GetFailoverStatus(ctx context.Context) (map[string]string, error)
}

// Coordinator defines the interface for multi-region coordination
type Coordinator interface {
	// Initialize initializes the multi-region coordinator
	Initialize(ctx context.Context, config *MultiRegionConfig) error
	
	// Upload performs a multi-region upload
	Upload(ctx context.Context, request *UploadRequest) (*UploadResult, error)
	
	// GetRegionStatus gets the status of all regions
	GetRegionStatus(ctx context.Context) (map[string]RegionStatus, error)
	
	// GetRegionMetrics gets metrics for all regions
	GetRegionMetrics(ctx context.Context) (map[string]RegionMetrics, error)
	
	// Shutdown gracefully shuts down the coordinator
	Shutdown(ctx context.Context) error
}