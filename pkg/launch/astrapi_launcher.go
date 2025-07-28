package launch

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"log/slog"
	"net/http"
	"strings"
	"time"

	"github.com/scttfrdmn/cargoship/pkg/aws/config"
	"github.com/scttfrdmn/cargoship/pkg/s3optimization"
)

// AstrapiLauncher handles deploying and managing CargoShip launch agents on astrapi NAS
// This implements the true "launch" capability - deploying autonomous archival agents
type AstrapiLauncher struct {
	host           string
	port           int
	containerImage string
	logger         *slog.Logger
	httpClient     *http.Client
}

// LaunchRequest represents a test execution request for astrapi
type LaunchRequest struct {
	TestType        string                    `json:"test_type"`
	TestFiles       []string                  `json:"test_files"`
	S3Config        config.S3Config           `json:"s3_config"`
	OptimizationConfig *s3optimization.Config `json:"optimization_config,omitempty"`
	AWSProfile      string                    `json:"aws_profile"`
	AWSRegion       string                    `json:"aws_region"`
	Parameters      map[string]interface{}    `json:"parameters,omitempty"`
	Timeout         time.Duration             `json:"timeout"`
}

// LaunchResponse represents the response from astrapi test execution
type LaunchResponse struct {
	JobID           string                 `json:"job_id"`
	Status          string                 `json:"status"`
	Results         *TestResults           `json:"results,omitempty"`
	Logs            []string               `json:"logs,omitempty"`
	Error           string                 `json:"error,omitempty"`
	StartTime       time.Time              `json:"start_time"`
	EndTime         *time.Time             `json:"end_time,omitempty"`
	Duration        *time.Duration         `json:"duration,omitempty"`
	ResourceUsage   *ResourceUsage         `json:"resource_usage,omitempty"`
}

// TestResults contains comprehensive test execution results
type TestResults struct {
	TestType              string                         `json:"test_type"`
	Success               bool                           `json:"success"`
	TotalFiles            int                            `json:"total_files"`
	ProcessedFiles        int                            `json:"processed_files"`
	TotalBytes            int64                          `json:"total_bytes"`
	ProcessedBytes        int64                          `json:"processed_bytes"`
	Duration              time.Duration                  `json:"duration"`
	AverageThroughputMBps float64                        `json:"average_throughput_mbps"`
	PeakThroughputMBps    float64                        `json:"peak_throughput_mbps"`
	OptimizationStats     interface{} `json:"optimization_stats,omitempty"`
	NetworkUtilization    *NetworkUtilization            `json:"network_utilization,omitempty"`
	ErrorCount            int                            `json:"error_count"`
	Errors                []string                       `json:"errors,omitempty"`
}

// ResourceUsage tracks container resource consumption during test execution
type ResourceUsage struct {
	CPUUsagePercent    float64 `json:"cpu_usage_percent"`
	MemoryUsageMB      float64 `json:"memory_usage_mb"`
	NetworkInMB        float64 `json:"network_in_mb"`
	NetworkOutMB       float64 `json:"network_out_mb"`
	DiskReadMB         float64 `json:"disk_read_mb"`
	DiskWriteMB        float64 `json:"disk_write_mb"`
	PeakMemoryUsageMB  float64 `json:"peak_memory_usage_mb"`
	AverageCPUPercent  float64 `json:"average_cpu_percent"`
}

// NetworkUtilization provides detailed network performance metrics
type NetworkUtilization struct {
	LocalNetworkMbps    float64 `json:"local_network_mbps"`    // astrapi local network (10Gbps)
	InternetMbps        float64 `json:"internet_mbps"`         // Internet to AWS (5Gbps)
	LocalEfficiency     float64 `json:"local_efficiency"`      // % of 10Gbps utilized
	InternetEfficiency  float64 `json:"internet_efficiency"`   // % of 5Gbps utilized
	OptimalPathUsed     bool    `json:"optimal_path_used"`     // Whether optimal network path was used
}

// NewAstrapiLauncher creates a new launcher for astrapi-based test execution
func NewAstrapiLauncher(host string, port int, containerImage string, logger *slog.Logger) *AstrapiLauncher {
	if logger == nil {
		logger = slog.Default()
	}

	return &AstrapiLauncher{
		host:           host,
		port:           port,
		containerImage: containerImage,
		logger:         logger,
		httpClient: &http.Client{
			Timeout: 30 * time.Second,
		},
	}
}

// LaunchPerformanceTest launches a comprehensive performance test on astrapi
func (al *AstrapiLauncher) LaunchPerformanceTest(ctx context.Context, req *LaunchRequest) (*LaunchResponse, error) {
	al.logger.Info("launching performance test on astrapi",
		"host", al.host,
		"port", al.port,
		"test_type", req.TestType,
		"files", len(req.TestFiles),
		"container", al.containerImage)

	// Validate request
	if err := al.validateRequest(req); err != nil {
		return nil, fmt.Errorf("invalid launch request: %w", err)
	}

	// Submit job to astrapi
	jobID, err := al.submitJob(ctx, req)
	if err != nil {
		return nil, fmt.Errorf("failed to submit job to astrapi: %w", err)
	}

	al.logger.Info("job submitted to astrapi", "job_id", jobID)

	// Monitor job execution
	response, err := al.monitorJob(ctx, jobID, req.Timeout)
	if err != nil {
		return nil, fmt.Errorf("job monitoring failed: %w", err)
	}

	al.logger.Info("astrapi job completed",
		"job_id", jobID,
		"status", response.Status,
		"duration", response.Duration,
		"success", response.Results != nil && response.Results.Success)

	return response, nil
}

// LaunchStressTest launches intensive stress testing on astrapi
func (al *AstrapiLauncher) LaunchStressTest(ctx context.Context, testType string, files []string, s3Config config.S3Config) (*LaunchResponse, error) {
	req := &LaunchRequest{
		TestType:        testType,
		TestFiles:       files,
		S3Config:        s3Config,
		OptimizationConfig: s3optimization.DefaultConfig(),
		AWSProfile:      "aws",
		AWSRegion:       "us-west-2",
		Parameters: map[string]interface{}{
			"stress_mode": true,
			"max_concurrency": 200,
			"chunk_size_mb": 512,
			"enable_monitoring": true,
			"network_optimization": true,
		},
		Timeout: 30 * time.Minute,
	}

	return al.LaunchPerformanceTest(ctx, req)
}

// LaunchLargeFileTest launches large file transfer testing on astrapi
func (al *AstrapiLauncher) LaunchLargeFileTest(ctx context.Context, files []string, s3Config config.S3Config) (*LaunchResponse, error) {
	req := &LaunchRequest{
		TestType:        "large_file_transfer",
		TestFiles:       files,
		S3Config:        s3Config,
		OptimizationConfig: s3optimization.DefaultConfig(),
		AWSProfile:      "aws",
		AWSRegion:       "us-west-2",
		Parameters: map[string]interface{}{
			"enable_progress_tracking": true,
			"chunk_size_mb": 256,
			"max_file_size_gb": 10,
			"enable_resume": true,
			"verify_integrity": true,
		},
		Timeout: 60 * time.Minute,
	}

	return al.LaunchPerformanceTest(ctx, req)
}

// LaunchBenchmarkSuite launches the complete CargoShip benchmark suite on astrapi
func (al *AstrapiLauncher) LaunchBenchmarkSuite(ctx context.Context, s3Config config.S3Config) (*LaunchResponse, error) {
	req := &LaunchRequest{
		TestType:        "benchmark_suite",
		TestFiles:       []string{}, // Will discover files on astrapi
		S3Config:        s3Config,
		OptimizationConfig: s3optimization.DefaultConfig(),
		AWSProfile:      "aws",
		AWSRegion:       "us-west-2",
		Parameters: map[string]interface{}{
			"run_all_tests": true,
			"include_stress_tests": true,
			"include_large_file_tests": true,
			"include_concurrent_tests": true,
			"generate_report": true,
			"network_analysis": true,
			"resource_monitoring": true,
		},
		Timeout: 120 * time.Minute, // 2 hours for complete suite
	}

	return al.LaunchPerformanceTest(ctx, req)
}

// submitJob submits a job to astrapi container orchestration
func (al *AstrapiLauncher) submitJob(ctx context.Context, req *LaunchRequest) (string, error) {
	url := fmt.Sprintf("http://%s:%d/api/v1/launch", al.host, al.port)
	
	// Create job submission payload
	jobPayload := map[string]interface{}{
		"image": al.containerImage,
		"command": al.buildTestCommand(req),
		"environment": al.buildEnvironment(req),
		"volumes": al.buildVolumes(),
		"resources": al.buildResourceLimits(),
		"network_mode": "host", // Use host networking for maximum performance
		"timeout": req.Timeout.Seconds(),
	}

	jsonPayload, err := json.Marshal(jobPayload)
	if err != nil {
		return "", fmt.Errorf("failed to marshal job payload: %w", err)
	}

	httpReq, err := http.NewRequestWithContext(ctx, "POST", url, strings.NewReader(string(jsonPayload)))
	if err != nil {
		return "", fmt.Errorf("failed to create HTTP request: %w", err)
	}

	httpReq.Header.Set("Content-Type", "application/json")

	resp, err := al.httpClient.Do(httpReq)
	if err != nil {
		return "", fmt.Errorf("HTTP request failed: %w", err)
	}
	defer func() {
		if err := resp.Body.Close(); err != nil {
			// Log the error but don't fail the operation
			fmt.Printf("Warning: failed to close response body: %v\n", err)
		}
	}()

	if resp.StatusCode != http.StatusOK {
		body, _ := io.ReadAll(resp.Body)
		return "", fmt.Errorf("job submission failed with status %d: %s", resp.StatusCode, string(body))
	}

	var submitResp struct {
		JobID string `json:"job_id"`
	}

	if err := json.NewDecoder(resp.Body).Decode(&submitResp); err != nil {
		return "", fmt.Errorf("failed to decode submission response: %w", err)
	}

	return submitResp.JobID, nil
}

// monitorJob monitors job execution and returns results
func (al *AstrapiLauncher) monitorJob(ctx context.Context, jobID string, timeout time.Duration) (*LaunchResponse, error) {
	ctx, cancel := context.WithTimeout(ctx, timeout)
	defer cancel()

	ticker := time.NewTicker(5 * time.Second) // Poll every 5 seconds
	defer ticker.Stop()

	for {
		select {
		case <-ctx.Done():
			return nil, fmt.Errorf("job monitoring timeout: %w", ctx.Err())
		case <-ticker.C:
			status, err := al.getJobStatus(ctx, jobID)
			if err != nil {
				al.logger.Warn("failed to get job status", "job_id", jobID, "error", err)
				continue
			}

			switch status.Status {
			case "completed":
				al.logger.Info("job completed successfully", "job_id", jobID)
				return status, nil
			case "failed":
				return status, fmt.Errorf("job failed: %s", status.Error)
			case "running":
				al.logger.Debug("job still running", "job_id", jobID)
				// Continue monitoring
			default:
				al.logger.Debug("job status", "job_id", jobID, "status", status.Status)
			}
		}
	}
}

// getJobStatus retrieves current job status from astrapi
func (al *AstrapiLauncher) getJobStatus(ctx context.Context, jobID string) (*LaunchResponse, error) {
	url := fmt.Sprintf("http://%s:%d/api/v1/jobs/%s", al.host, al.port, jobID)

	req, err := http.NewRequestWithContext(ctx, "GET", url, nil)
	if err != nil {
		return nil, fmt.Errorf("failed to create status request: %w", err)
	}

	resp, err := al.httpClient.Do(req)
	if err != nil {
		return nil, fmt.Errorf("status request failed: %w", err)
	}
	defer func() {
		if err := resp.Body.Close(); err != nil {
			// Log the error but don't fail the operation
			fmt.Printf("Warning: failed to close response body: %v\n", err)
		}
	}()

	if resp.StatusCode != http.StatusOK {
		body, _ := io.ReadAll(resp.Body)
		return nil, fmt.Errorf("status request failed with status %d: %s", resp.StatusCode, string(body))
	}

	var status LaunchResponse
	if err := json.NewDecoder(resp.Body).Decode(&status); err != nil {
		return nil, fmt.Errorf("failed to decode status response: %w", err)
	}

	return &status, nil
}

// buildTestCommand constructs the command to run inside the container
func (al *AstrapiLauncher) buildTestCommand(req *LaunchRequest) []string {
	cmd := []string{
		"/usr/local/bin/cargoship-test",
		"--test-type", req.TestType,
		"--aws-profile", req.AWSProfile,
		"--aws-region", req.AWSRegion,
		"--s3-bucket", req.S3Config.Bucket,
		"--output-format", "json",
		"--verbose",
	}

	// Add test files
	if len(req.TestFiles) > 0 {
		cmd = append(cmd, "--test-files")
		cmd = append(cmd, strings.Join(req.TestFiles, ","))
	}

	// Add optimization configuration
	if req.OptimizationConfig != nil {
		cmd = append(cmd, "--enable-optimization")
		if req.OptimizationConfig.EnableBBR {
			cmd = append(cmd, "--enable-bbr")
		}
		if req.OptimizationConfig.EnableCUBIC {
			cmd = append(cmd, "--enable-cubic")
		}
	}

	// Add S3 configuration
	cmd = append(cmd, "--concurrency", fmt.Sprintf("%d", req.S3Config.Concurrency))
	cmd = append(cmd, "--multipart-threshold", fmt.Sprintf("%d", req.S3Config.MultipartThreshold))
	cmd = append(cmd, "--multipart-chunk-size", fmt.Sprintf("%d", req.S3Config.MultipartChunkSize))

	// Add custom parameters
	for key, value := range req.Parameters {
		cmd = append(cmd, fmt.Sprintf("--%s", strings.ReplaceAll(key, "_", "-")))
		if value != true { // Don't add value for boolean flags
			cmd = append(cmd, fmt.Sprintf("%v", value))
		}
	}

	return cmd
}

// buildEnvironment constructs environment variables for the container
func (al *AstrapiLauncher) buildEnvironment(req *LaunchRequest) map[string]string {
	env := map[string]string{
		"AWS_PROFILE":                req.AWSProfile,
		"AWS_DEFAULT_REGION":         req.AWSRegion,
		"CARGOSHIP_LOG_LEVEL":        "info",
		"CARGOSHIP_METRICS_ENABLED":  "true",
		"CARGOSHIP_NETWORK_OPTIMIZE": "true",
		"GOMAXPROCS":                 "0", // Use all available CPUs
	}

	// Add optimization environment variables
	if req.OptimizationConfig != nil {
		env["CARGOSHIP_BBR_ENABLED"] = fmt.Sprintf("%t", req.OptimizationConfig.EnableBBR)
		env["CARGOSHIP_CUBIC_ENABLED"] = fmt.Sprintf("%t", req.OptimizationConfig.EnableCUBIC)
		env["CARGOSHIP_PREDICTIVE_MODE"] = fmt.Sprintf("%t", req.OptimizationConfig.PredictiveMode)
		env["CARGOSHIP_MAX_CONNECTIONS"] = fmt.Sprintf("%d", req.OptimizationConfig.MaxConnections)
		env["CARGOSHIP_BUFFER_SIZE"] = fmt.Sprintf("%d", req.OptimizationConfig.BufferSize)
	}

	return env
}

// buildVolumes defines volume mounts for the container
func (al *AstrapiLauncher) buildVolumes() []map[string]string {
	return []map[string]string{
		{
			"host_path":      "/volume1/Public",       // astrapi public data
			"container_path": "/data/public",
			"readonly":       "true",
		},
		{
			"host_path":      "/volume1/homes/.aws",   // AWS credentials
			"container_path": "/root/.aws",
			"readonly":       "true",
		},
		{
			"host_path":      "/tmp/cargoship-results", // Results output
			"container_path": "/results",
			"readonly":       "false",
		},
	}
}

// buildResourceLimits defines container resource constraints
func (al *AstrapiLauncher) buildResourceLimits() map[string]interface{} {
	return map[string]interface{}{
		"memory":     "8GB",    // 8GB memory limit
		"cpu_cores":  4,        // 4 CPU cores
		"network_mbps": 10000,  // 10Gbps network limit (astrapi max)
	}
}

// validateRequest validates the launch request
func (al *AstrapiLauncher) validateRequest(req *LaunchRequest) error {
	if req.TestType == "" {
		return fmt.Errorf("test_type is required")
	}

	if req.S3Config.Bucket == "" {
		return fmt.Errorf("S3 bucket is required")
	}

	if req.AWSProfile == "" {
		return fmt.Errorf("AWS profile is required")
	}

	if req.AWSRegion == "" {
		return fmt.Errorf("AWS region is required")
	}

	if req.Timeout <= 0 {
		req.Timeout = 30 * time.Minute // Default timeout
	}

	return nil
}

// GetJobLogs retrieves logs from a completed or running job
func (al *AstrapiLauncher) GetJobLogs(ctx context.Context, jobID string) ([]string, error) {
	url := fmt.Sprintf("http://%s:%d/api/v1/jobs/%s/logs", al.host, al.port, jobID)

	req, err := http.NewRequestWithContext(ctx, "GET", url, nil)
	if err != nil {
		return nil, fmt.Errorf("failed to create logs request: %w", err)
	}

	resp, err := al.httpClient.Do(req)
	if err != nil {
		return nil, fmt.Errorf("logs request failed: %w", err)
	}
	defer func() {
		if err := resp.Body.Close(); err != nil {
			// Log the error but don't fail the operation
			fmt.Printf("Warning: failed to close response body: %v\n", err)
		}
	}()

	if resp.StatusCode != http.StatusOK {
		body, _ := io.ReadAll(resp.Body)
		return nil, fmt.Errorf("logs request failed with status %d: %s", resp.StatusCode, string(body))
	}

	var logsResp struct {
		Logs []string `json:"logs"`
	}

	if err := json.NewDecoder(resp.Body).Decode(&logsResp); err != nil {
		return nil, fmt.Errorf("failed to decode logs response: %w", err)
	}

	return logsResp.Logs, nil
}

// CancelJob cancels a running job on astrapi
func (al *AstrapiLauncher) CancelJob(ctx context.Context, jobID string) error {
	url := fmt.Sprintf("http://%s:%d/api/v1/jobs/%s/cancel", al.host, al.port, jobID)

	req, err := http.NewRequestWithContext(ctx, "POST", url, nil)
	if err != nil {
		return fmt.Errorf("failed to create cancel request: %w", err)
	}

	resp, err := al.httpClient.Do(req)
	if err != nil {
		return fmt.Errorf("cancel request failed: %w", err)
	}
	defer func() {
		if err := resp.Body.Close(); err != nil {
			// Log the error but don't fail the operation
			fmt.Printf("Warning: failed to close response body: %v\n", err)
		}
	}()

	if resp.StatusCode != http.StatusOK {
		body, _ := io.ReadAll(resp.Body)
		return fmt.Errorf("cancel request failed with status %d: %s", resp.StatusCode, string(body))
	}

	al.logger.Info("job cancelled successfully", "job_id", jobID)
	return nil
}