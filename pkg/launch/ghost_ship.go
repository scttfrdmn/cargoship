package launch

import (
	"context"
	"encoding/json"
	"fmt"
	"log/slog"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"time"

	"github.com/aws/aws-sdk-go-v2/aws"
	awsconfig "github.com/aws/aws-sdk-go-v2/config"
	"github.com/aws/aws-sdk-go-v2/service/s3"
	cargoshipconfig "github.com/scttfrdmn/cargoship/pkg/aws/config"
	s3transport "github.com/scttfrdmn/cargoship/pkg/aws/s3"
	"github.com/scttfrdmn/cargoship/pkg/s3optimization"
)

// regularTransporterWrapper adapts a regular Transporter to match the Uploader interface
type regularTransporterWrapper struct {
	transporter *s3transport.Transporter
}

func (w *regularTransporterWrapper) Upload(ctx context.Context, archive *s3transport.Archive) (*s3transport.UploadResult, error) {
	// Convert pointer to value for the regular transporter
	return w.transporter.Upload(ctx, *archive)
}

// GhostShip represents an autonomous archival process that operates in the background
// It watches directories and automatically archives files to S3 based on configured rules
type GhostShip struct {
	id          string
	config      *GhostShipConfig
	logger      *slog.Logger
	
	// Core components
	watcher     *FileWatcher
	transporter interface{}
	controller  *ControllerConnection
	
	// State management
	status      GhostShipStatus
	activeJobs  map[string]*ArchivalJob
	mu          sync.RWMutex
	
	// Lifecycle management
	ctx         context.Context
	cancel      context.CancelFunc
	wg          sync.WaitGroup
}

// GhostShipConfig defines the configuration for an autonomous ghost ship
type GhostShipConfig struct {
	// Ghost ship identification
	ID               string                    `json:"id" yaml:"id"`
	Name             string                    `json:"name" yaml:"name"`
	Description      string                    `json:"description" yaml:"description"`
	
	// Archival configuration
	S3Config         cargoshipconfig.S3Config  `json:"s3_config" yaml:"s3_config"`
	OptimizationConfig *s3optimization.Config  `json:"optimization_config" yaml:"optimization_config"`
	
	// Watch configuration
	WatchPaths       []WatchPath               `json:"watch_paths" yaml:"watch_paths"`
	ScanInterval     time.Duration             `json:"scan_interval" yaml:"scan_interval"`
	
	// Archival rules
	ArchivalRules    []ArchivalRule            `json:"archival_rules" yaml:"archival_rules"`
	
	// Performance settings
	MaxConcurrentJobs int                      `json:"max_concurrent_jobs" yaml:"max_concurrent_jobs"`
	WorkerPoolSize    int                      `json:"worker_pool_size" yaml:"worker_pool_size"`
	
	// Controller integration
	ControllerURL     string                   `json:"controller_url" yaml:"controller_url"`
	AuthToken         string                   `json:"auth_token" yaml:"auth_token"`
	TLSConfig         *TLSConfig               `json:"tls_config" yaml:"tls_config"`
	
	// Monitoring and reporting
	ReportingEnabled  bool                     `json:"reporting_enabled" yaml:"reporting_enabled"`
	ReportInterval    time.Duration            `json:"report_interval" yaml:"report_interval"`
}

// ArchivalRule defines conditions and actions for automatic archival
type ArchivalRule struct {
	Name             string                    `json:"name" yaml:"name"`
	Description      string                    `json:"description" yaml:"description"`
	
	// Matching conditions
	PathPattern      string                    `json:"path_pattern" yaml:"path_pattern"`
	FilePattern      string                    `json:"file_pattern" yaml:"file_pattern"`
	MinSize          int64                     `json:"min_size" yaml:"min_size"`
	MaxSize          int64                     `json:"max_size" yaml:"max_size"`
	MinAge           time.Duration             `json:"min_age" yaml:"min_age"`
	MaxAge           time.Duration             `json:"max_age" yaml:"max_age"`
	FileTypes        []string                  `json:"file_types" yaml:"file_types"`
	
	// Archive settings
	Destination      string                    `json:"destination" yaml:"destination"`
	StorageClass     string                    `json:"storage_class" yaml:"storage_class"`
	Compression      string                    `json:"compression" yaml:"compression"`
	Encryption       bool                      `json:"encryption" yaml:"encryption"`
	DeleteAfterArchive bool                   `json:"delete_after_archive" yaml:"delete_after_archive"`
	
	// Metadata
	Tags             map[string]string         `json:"tags" yaml:"tags"`
	Metadata         map[string]string         `json:"metadata" yaml:"metadata"`
	
	// Priority and scheduling
	Priority         int                       `json:"priority" yaml:"priority"`
	Schedule         string                    `json:"schedule" yaml:"schedule"` // Cron-like schedule
	Enabled          bool                      `json:"enabled" yaml:"enabled"`
}

// ArchivalJob represents a single file archival operation
type ArchivalJob struct {
	ID               string                    `json:"id"`
	GhostShipID      string                    `json:"ghost_ship_id"`
	RuleName         string                    `json:"rule_name"`
	SourcePath       string                    `json:"source_path"`
	Destination      string                    `json:"destination"`
	
	// Job state
	State            JobState                  `json:"state"`
	Progress         float64                   `json:"progress"`
	BytesTotal       int64                     `json:"bytes_total"`
	BytesTransferred int64                     `json:"bytes_transferred"`
	TransferRate     float64                   `json:"transfer_rate_mbps"`
	
	// Timing
	CreatedAt        time.Time                 `json:"created_at"`
	StartedAt        *time.Time                `json:"started_at,omitempty"`
	CompletedAt      *time.Time                `json:"completed_at,omitempty"`
	Duration         *time.Duration            `json:"duration,omitempty"`
	
	// Results
	Success          bool                      `json:"success"`
	Error            string                    `json:"error,omitempty"`
	S3Key            string                    `json:"s3_key,omitempty"`
	ETag             string                    `json:"etag,omitempty"`
	
	// Optimization metrics
	OptimizationStats interface{} `json:"optimization_stats,omitempty"`
}

// GhostShipStatus represents the current status of a ghost ship
type GhostShipStatus struct {
	State            GhostShipState            `json:"state"`
	StartTime        time.Time                 `json:"start_time"`
	LastActivity     time.Time                 `json:"last_activity"`
	Uptime          time.Duration             `json:"uptime"`
	
	// Job statistics
	ActiveJobs       int                       `json:"active_jobs"`
	QueuedJobs       int                       `json:"queued_jobs"`
	CompletedJobs    int64                     `json:"completed_jobs"`
	FailedJobs       int64                     `json:"failed_jobs"`
	
	// Performance metrics
	TotalBytesArchived int64                   `json:"total_bytes_archived"`
	AverageThroughput  float64                 `json:"average_throughput_mbps"`
	
	// System metrics
	WatchedPaths     int                       `json:"watched_paths"`
	ActiveRules      int                       `json:"active_rules"`
	LastScan         time.Time                 `json:"last_scan"`
	NextScan         time.Time                 `json:"next_scan"`
	
	// Health status
	Healthy          bool                      `json:"healthy"`
	LastError        string                    `json:"last_error,omitempty"`
}

// GhostShipState represents the operational state
type GhostShipState string

const (
	GhostShipStateStarting    GhostShipState = "starting"
	GhostShipStateRunning     GhostShipState = "running"
	GhostShipStateIdle        GhostShipState = "idle"
	GhostShipStateWorking     GhostShipState = "working"
	GhostShipStatePaused      GhostShipState = "paused"
	GhostShipStateError       GhostShipState = "error"
	GhostShipStateStopping    GhostShipState = "stopping"
	GhostShipStateStopped     GhostShipState = "stopped"
)

// NewGhostShip creates a new autonomous ghost ship archival system
func NewGhostShip(config *GhostShipConfig, logger *slog.Logger) (*GhostShip, error) {
	if config == nil {
		return nil, fmt.Errorf("ghost ship configuration cannot be nil")
	}
	
	if err := validateGhostShipConfig(config); err != nil {
		return nil, fmt.Errorf("invalid ghost ship configuration: %w", err)
	}
	
	if logger == nil {
		logger = slog.Default()
	}
	
	ctx, cancel := context.WithCancel(context.Background())
	
	ghost := &GhostShip{
		id:         config.ID,
		config:     config,
		logger:     logger.With("component", "ghost-ship", "ghost_id", config.ID),
		activeJobs: make(map[string]*ArchivalJob),
		ctx:        ctx,
		cancel:     cancel,
		status: GhostShipStatus{
			State:       GhostShipStateStarting,
			StartTime:   time.Now(),
			Healthy:     true,
		},
	}
	
	// Initialize file watcher
	watcher, err := NewFileWatcher(config.WatchPaths, logger)
	if err != nil {
		cancel()
		return nil, fmt.Errorf("failed to create file watcher: %w", err)
	}
	ghost.watcher = watcher
	
	// Initialize S3 transporter with optimization
	transporter, err := ghost.createOptimizedTransporter(ctx)
	if err != nil {
		cancel()
		return nil, fmt.Errorf("failed to create S3 transporter: %w", err)
	}
	ghost.transporter = transporter
	
	// Initialize controller connection if configured
	if config.ControllerURL != "" {
		agentConfig := &AgentConfig{
			ID:            config.ID,
			Name:          config.Name,
			Description:   config.Description,
			ControllerURL: config.ControllerURL,
			AuthToken:     config.AuthToken,
			TLSConfig:     config.TLSConfig,
			WatchPaths:    config.WatchPaths,
		}
		
		controller, err := NewControllerConnection(agentConfig, logger)
		if err != nil {
			cancel()
			return nil, fmt.Errorf("failed to create controller connection: %w", err)
		}
		ghost.controller = controller
	}
	
	ghost.logger.Info("Ghost ship created successfully",
		"name", config.Name,
		"watch_paths", len(config.WatchPaths),
		"archival_rules", len(config.ArchivalRules),
		"optimization_enabled", config.OptimizationConfig != nil,
		"controller_enabled", ghost.controller != nil)
	
	return ghost, nil
}

// Launch starts the ghost ship autonomous archival operations
func (gs *GhostShip) Launch() error {
	gs.mu.Lock()
	defer gs.mu.Unlock()
	
	if gs.status.State != GhostShipStateStarting {
		return fmt.Errorf("ghost ship is not in starting state")
	}
	
	gs.logger.Info("🚢 Launching ghost ship autonomous archival system")
	
	// Start file scanner
	gs.wg.Add(1)
	go gs.runFileScanner()
	
	// Start job processor
	gs.wg.Add(1)
	go gs.runJobProcessor()
	
	// Start status reporter (if enabled)
	if gs.config.ReportingEnabled {
		gs.wg.Add(1)
		go gs.runStatusReporter()
	}
	
	// Start health monitor
	gs.wg.Add(1)
	go gs.runHealthMonitor()
	
	// Start controller connection if configured
	if gs.controller != nil {
		gs.wg.Add(1)
		go gs.runControllerConnection()
	}
	
	gs.status.State = GhostShipStateRunning
	gs.status.LastActivity = time.Now()
	
	gs.logger.Info("👻 Ghost ship launched successfully - autonomous archival active")
	return nil
}

// Stop gracefully stops the ghost ship
func (gs *GhostShip) Stop() error {
	gs.mu.Lock()
	gs.status.State = GhostShipStateStopping
	gs.mu.Unlock()
	
	gs.logger.Info("🛑 Stopping ghost ship autonomous archival system")
	
	// Cancel all operations
	gs.cancel()
	
	// Wait for graceful shutdown
	done := make(chan struct{})
	go func() {
		gs.wg.Wait()
		close(done)
	}()
	
	select {
	case <-done:
		gs.logger.Info("👻 Ghost ship stopped gracefully")
	case <-time.After(30 * time.Second):
		gs.logger.Warn("Ghost ship shutdown timed out")
	}
	
	gs.mu.Lock()
	gs.status.State = GhostShipStateStopped
	gs.mu.Unlock()
	
	return nil
}

// GetStatus returns the current ghost ship status
func (gs *GhostShip) GetStatus() GhostShipStatus {
	gs.mu.RLock()
	defer gs.mu.RUnlock()
	
	status := gs.status
	status.ActiveJobs = len(gs.activeJobs)
	status.Uptime = time.Since(gs.status.StartTime)
	status.ActiveRules = countActiveRules(gs.config.ArchivalRules)
	status.WatchedPaths = len(gs.config.WatchPaths)
	
	return status
}

// GetJobs returns all current archival jobs
func (gs *GhostShip) GetJobs() map[string]*ArchivalJob {
	gs.mu.RLock()
	defer gs.mu.RUnlock()
	
	jobs := make(map[string]*ArchivalJob)
	for id, job := range gs.activeJobs {
		jobCopy := *job
		jobs[id] = &jobCopy
	}
	
	return jobs
}

// runFileScanner continuously scans for files matching archival rules
func (gs *GhostShip) runFileScanner() {
	defer gs.wg.Done()
	
	gs.logger.Info("🔍 Starting autonomous file scanner")
	
	ticker := time.NewTicker(gs.config.ScanInterval)
	defer ticker.Stop()
	
	for {
		select {
		case <-gs.ctx.Done():
			return
		case <-ticker.C:
			gs.performFileScan()
		}
	}
}

// runJobProcessor handles queued archival jobs
func (gs *GhostShip) runJobProcessor() {
	defer gs.wg.Done()
	
	gs.logger.Info("⚙️ Starting autonomous job processor")
	
	// Create worker pool
	workerPool := make(chan struct{}, gs.config.WorkerPoolSize)
	
	ticker := time.NewTicker(5 * time.Second)
	defer ticker.Stop()
	
	for {
		select {
		case <-gs.ctx.Done():
			return
		case <-ticker.C:
			gs.processQueuedJobs(workerPool)
		}
	}
}

// runStatusReporter periodically reports status to controller
func (gs *GhostShip) runStatusReporter() {
	defer gs.wg.Done()
	
	gs.logger.Info("📊 Starting status reporter")
	
	ticker := time.NewTicker(gs.config.ReportInterval)
	defer ticker.Stop()
	
	for {
		select {
		case <-gs.ctx.Done():
			return
		case <-ticker.C:
			gs.reportStatus()
		}
	}
}

// runHealthMonitor monitors ghost ship health
func (gs *GhostShip) runHealthMonitor() {
	defer gs.wg.Done()
	
	ticker := time.NewTicker(30 * time.Second)
	defer ticker.Stop()
	
	for {
		select {
		case <-gs.ctx.Done():
			return
		case <-ticker.C:
			gs.performHealthCheck()
		}
	}
}

// performFileScan scans directories for files matching archival rules
func (gs *GhostShip) performFileScan() {
	gs.logger.Debug("🔍 Scanning directories for archival candidates")
	
	gs.mu.Lock()
	gs.status.LastScan = time.Now()
	gs.status.NextScan = time.Now().Add(gs.config.ScanInterval)
	gs.status.State = GhostShipStateWorking
	gs.mu.Unlock()
	
	for _, rule := range gs.config.ArchivalRules {
		if !rule.Enabled {
			continue
		}
		
		// Find files matching this rule
		candidates, err := gs.findArchivalCandidates(rule)
		if err != nil {
			gs.logger.Error("Failed to scan for archival candidates", 
				"rule", rule.Name, 
				"error", err)
			continue
		}
		
		// Create archival jobs for candidates
		for _, candidate := range candidates {
			// Check if job already exists for this file
			gs.mu.RLock()
			exists := false
			for _, existingJob := range gs.activeJobs {
				if existingJob.SourcePath == candidate && existingJob.RuleName == rule.Name {
					exists = true
					break
				}
			}
			gs.mu.RUnlock()
			
			if exists {
				continue // Skip if job already exists
			}
			
			job := gs.createArchivalJob(candidate, rule)
			
			gs.mu.Lock()
			gs.activeJobs[job.ID] = job
			gs.mu.Unlock()
			
			gs.logger.Info("📁 Queued file for autonomous archival",
				"file", candidate,
				"rule", rule.Name,
				"job_id", job.ID)
		}
	}
	
	gs.mu.Lock()
	if len(gs.activeJobs) == 0 {
		gs.status.State = GhostShipStateIdle
	}
	gs.status.LastActivity = time.Now()
	gs.mu.Unlock()
}

// processQueuedJobs processes pending archival jobs
func (gs *GhostShip) processQueuedJobs(workerPool chan struct{}) {
	gs.mu.RLock()
	var pendingJobs []*ArchivalJob
	totalJobs := len(gs.activeJobs)
	for _, job := range gs.activeJobs {
		if job.State == JobStatePending {
			pendingJobs = append(pendingJobs, job)
		}
	}
	gs.mu.RUnlock()
	
	gs.logger.Debug("Processing job queue", "total_jobs", totalJobs, "pending_jobs", len(pendingJobs))
	
	for _, job := range pendingJobs {
		// Count running jobs (not pending ones)
		gs.mu.RLock()
		runningJobs := 0
		for _, activeJob := range gs.activeJobs {
			if activeJob.State == JobStateRunning {
				runningJobs++
			}
		}
		gs.mu.RUnlock()
		
		// Limit concurrent jobs
		if runningJobs >= gs.config.MaxConcurrentJobs {
			gs.logger.Debug("Max concurrent jobs reached", "running", runningJobs, "max", gs.config.MaxConcurrentJobs)
			break
		}
		
		gs.logger.Debug("Attempting to execute job", "job_id", job.ID, "source", job.SourcePath)
		
		select {
		case workerPool <- struct{}{}:
			gs.logger.Debug("Worker acquired, starting job execution", "job_id", job.ID)
			go func(j *ArchivalJob) {
				defer func() { <-workerPool }()
				gs.executeArchivalJob(j)
			}(job)
		case <-gs.ctx.Done():
			return
		default:
			// Worker pool full, skip this job and try next one
			gs.logger.Debug("Worker pool full, skipping job", "job_id", job.ID)
			continue
		}
	}
}

// executeArchivalJob executes a single archival job
func (gs *GhostShip) executeArchivalJob(job *ArchivalJob) {
	gs.logger.Info("🚀 Starting autonomous archival job",
		"job_id", job.ID,
		"source", job.SourcePath,
		"destination", job.Destination)
	
	// Update job state
	gs.mu.Lock()
	job.State = JobStateRunning
	now := time.Now()
	job.StartedAt = &now
	gs.status.State = GhostShipStateWorking
	gs.mu.Unlock()
	
	// Execute the archival
	err := gs.performArchival(job)
	
	// Update job completion
	gs.mu.Lock()
	completed := time.Now()
	job.CompletedAt = &completed
	if job.StartedAt != nil {
		duration := completed.Sub(*job.StartedAt)
		job.Duration = &duration
	}
	
	if err != nil {
		job.State = JobStateFailed
		job.Success = false
		job.Error = err.Error()
		gs.status.FailedJobs++
		gs.logger.Error("❌ Autonomous archival job failed",
			"job_id", job.ID,
			"error", err)
	} else {
		job.State = JobStateCompleted
		job.Success = true
		gs.status.CompletedJobs++
		gs.status.TotalBytesArchived += job.BytesTotal
		gs.logger.Info("✅ Autonomous archival job completed successfully",
			"job_id", job.ID,
			"bytes", job.BytesTotal,
			"duration", job.Duration,
			"rate", job.TransferRate)
	}
	
	gs.status.LastActivity = time.Now()
	
	// Remove completed job after delay (keep for status reporting)
	go func() {
		time.Sleep(5 * time.Minute)
		gs.mu.Lock()
		delete(gs.activeJobs, job.ID)
		gs.mu.Unlock()
	}()
	
	gs.mu.Unlock()
}

// createOptimizedTransporter creates an S3 transporter with optimization
func (gs *GhostShip) createOptimizedTransporter(ctx context.Context) (interface{}, error) {
	// Load AWS configuration with profile support from environment variables
	profile := os.Getenv("AWS_PROFILE")
	
	var cfg aws.Config
	var err error
	
	if profile != "" {
		cfg, err = awsconfig.LoadDefaultConfig(ctx,
			awsconfig.WithSharedConfigProfile(profile),
		)
	} else {
		cfg, err = awsconfig.LoadDefaultConfig(ctx)
	}
	
	if err != nil {
		return nil, fmt.Errorf("failed to load AWS config: %w", err)
	}
	
	// Create S3 client
	s3Client := s3.NewFromConfig(cfg)
	
	if gs.config.OptimizationConfig != nil {
		// Create optimized transporter
		optimizedTransporter, err := s3transport.NewOptimizedTransporter(ctx, s3Client, gs.config.S3Config, gs.logger)
		if err != nil {
			return nil, err
		}
		return optimizedTransporter, nil
	}
	
	// Create regular transporter
	regularTransporter := s3transport.NewTransporter(s3Client, gs.config.S3Config)
	return regularTransporter, nil
}

// Helper functions

func validateGhostShipConfig(config *GhostShipConfig) error {
	if config.ID == "" {
		return fmt.Errorf("ghost ship ID cannot be empty")
	}
	
	if config.S3Config.Bucket == "" {
		return fmt.Errorf("S3 bucket must be configured")
	}
	
	if len(config.WatchPaths) == 0 {
		return fmt.Errorf("at least one watch path must be configured")
	}
	
	if len(config.ArchivalRules) == 0 {
		return fmt.Errorf("at least one archival rule must be configured")
	}
	
	if config.ScanInterval <= 0 {
		config.ScanInterval = 5 * time.Minute // Default
	}
	
	if config.MaxConcurrentJobs <= 0 {
		config.MaxConcurrentJobs = 5 // Default
	}
	
	if config.WorkerPoolSize <= 0 {
		config.WorkerPoolSize = 3 // Default
	}
	
	if config.ReportInterval <= 0 {
		config.ReportInterval = 1 * time.Minute // Default
	}
	
	return nil
}

func countActiveRules(rules []ArchivalRule) int {
	count := 0
	for _, rule := range rules {
		if rule.Enabled {
			count++
		}
	}
	return count
}

func (gs *GhostShip) findArchivalCandidates(rule ArchivalRule) ([]string, error) {
	var candidates []string
	
	// Find matching watch paths for this rule
	for _, watchPath := range gs.config.WatchPaths {
		// Check if this watch path applies to this rule
		if !gs.watchPathMatchesRule(watchPath, rule) {
			continue
		}
		
		gs.logger.Debug("Scanning watch path", "path", watchPath.Path, "rule", rule.Name)
		
		// Walk the directory tree
		err := filepath.Walk(watchPath.Path, func(path string, info os.FileInfo, err error) error {
			if err != nil {
				return nil // Skip problematic files
			}
			
			// Skip directories
			if info.IsDir() {
				return nil
			}
			
			// Check if file matches the rule patterns
			if gs.fileMatchesRule(path, rule, info) {
				candidates = append(candidates, path)
				gs.logger.Debug("Found archival candidate", "file", path, "rule", rule.Name)
			}
			
			return nil
		})
		
		if err != nil {
			gs.logger.Error("Failed to scan directory", "path", watchPath.Path, "error", err)
		}
	}
	
	gs.logger.Debug("Found archival candidates", "count", len(candidates), "rule", rule.Name)
	return candidates, nil
}

func (gs *GhostShip) watchPathMatchesRule(watchPath WatchPath, rule ArchivalRule) bool {
	// Check if the rule's path pattern matches this watch path
	if rule.PathPattern != "" {
		matched, _ := filepath.Match(rule.PathPattern, watchPath.Path+"/**")
		if !matched {
			return false
		}
	}
	return true
}

func (gs *GhostShip) fileMatchesRule(filePath string, rule ArchivalRule, info os.FileInfo) bool {
	// Check file pattern
	if rule.FilePattern != "" {
		// Handle glob patterns like *.{fasta,vcf,fastq}
		patterns := []string{rule.FilePattern}
		if strings.Contains(rule.FilePattern, "{") && strings.Contains(rule.FilePattern, "}") {
			// Expand brace patterns like *.{fasta,vcf} -> [*.fasta, *.vcf]
			patterns = gs.expandBracePattern(rule.FilePattern)
		}
		
		matched := false
		for _, pattern := range patterns {
			if match, _ := filepath.Match(pattern, filepath.Base(filePath)); match {
				matched = true
				break
			}
		}
		if !matched {
			return false
		}
	}
	
	// Check file size limits
	if rule.MinSize > 0 && info.Size() < int64(rule.MinSize) {
		return false
	}
	if rule.MaxSize > 0 && info.Size() > int64(rule.MaxSize) {
		return false
	}
	
	// Check file age
	if rule.MinAge > 0 {
		age := time.Since(info.ModTime())
		if age < rule.MinAge {
			return false
		}
	}
	
	if rule.MaxAge > 0 {
		age := time.Since(info.ModTime())
		if age > rule.MaxAge {
			return false
		}
	}
	
	return true
}

func (gs *GhostShip) expandBracePattern(pattern string) []string {
	// Simple brace expansion: *.{fasta,vcf,fastq} -> [*.fasta, *.vcf, *.fastq]
	if !strings.Contains(pattern, "{") || !strings.Contains(pattern, "}") {
		return []string{pattern}
	}
	
	start := strings.Index(pattern, "{")
	end := strings.Index(pattern, "}")
	if start >= end {
		return []string{pattern}
	}
	
	prefix := pattern[:start]
	suffix := pattern[end+1:]
	options := strings.Split(pattern[start+1:end], ",")
	
	var expanded []string
	for _, option := range options {
		expanded = append(expanded, prefix+strings.TrimSpace(option)+suffix)
	}
	
	return expanded
}

func (gs *GhostShip) createArchivalJob(filePath string, rule ArchivalRule) *ArchivalJob {
	return &ArchivalJob{
		ID:          fmt.Sprintf("job-%d", time.Now().UnixNano()),
		GhostShipID: gs.id,
		RuleName:    rule.Name,
		SourcePath:  filePath,
		Destination: rule.Destination,
		State:       JobStatePending,
		CreatedAt:   time.Now(),
	}
}

func (gs *GhostShip) performArchival(job *ArchivalJob) error {
	gs.logger.Info("Starting S3 archival", "job_id", job.ID, "source", job.SourcePath)
	
	// Get file info
	fileInfo, err := os.Stat(job.SourcePath)
	if err != nil {
		return fmt.Errorf("failed to stat file: %w", err)
	}
	
	// Open the source file
	file, err := os.Open(job.SourcePath)
	if err != nil {
		return fmt.Errorf("failed to open file: %w", err)
	}
	defer func() {
		if closeErr := file.Close(); closeErr != nil {
			gs.logger.Error("Failed to close file", "file", job.SourcePath, "error", closeErr)
		}
	}()
	
	// Generate S3 key from file path
	s3Key := gs.generateS3Key(job.SourcePath, job.RuleName)
	
	// Get the rule for this job
	var rule *ArchivalRule
	for _, r := range gs.config.ArchivalRules {
		if r.Name == job.RuleName {
			rule = &r
			break
		}
	}
	
	if rule == nil {
		return fmt.Errorf("archival rule not found: %s", job.RuleName)
	}
	
	// Use the S3 transporter for upload - support both regular and optimized transporters
	type Uploader interface {
		Upload(ctx context.Context, archive *s3transport.Archive) (*s3transport.UploadResult, error)
	}
	
	// Handle both OptimizedTransporter (which implements Uploader directly) and regular Transporter
	var uploader Uploader
	
	if optimized, ok := gs.transporter.(*s3transport.OptimizedTransporter); ok {
		// OptimizedTransporter already implements the Uploader interface
		uploader = optimized
	} else if regular, ok := gs.transporter.(*s3transport.Transporter); ok {
		// Regular transporter needs a wrapper to match the interface
		uploader = &regularTransporterWrapper{regular}
	} else {
		return fmt.Errorf("transporter does not implement Upload method")
	}
	
	// Create S3 archive for upload
	archive := s3transport.Archive{
		Key:              s3Key,
		Reader:           file,
		Size:             fileInfo.Size(),
		StorageClass:     cargoshipconfig.StorageClass(rule.StorageClass),
		Metadata: map[string]string{
			"source":      job.SourcePath,
			"ghost_ship":  gs.id,
			"rule":        job.RuleName,
			"archived_at": time.Now().Format(time.RFC3339),
		},
		OriginalSize:     fileInfo.Size(),
		CompressionType:  rule.Compression,
		AccessPattern:    "sequential", // Default for archival
		RetentionDays:    30,          // Default retention
	}
	
	// Perform the upload
	ctx := context.Background()
	result, err := uploader.Upload(ctx, &archive)
	if err != nil {
		return fmt.Errorf("failed to upload to S3: %w", err)
	}
	
	gs.logger.Debug("S3 upload completed", "result", result)
	
	gs.logger.Info("Successfully archived file", 
		"job_id", job.ID, 
		"source", job.SourcePath,
		"s3_key", s3Key,
		"size", fileInfo.Size(),
		"storage_class", rule.StorageClass)
	
	// Update job state
	job.State = JobStateCompleted
	now := time.Now()
	job.CompletedAt = &now
	job.BytesTransferred = fileInfo.Size()
	
	// Update statistics
	gs.mu.Lock()
	gs.status.CompletedJobs++
	gs.status.TotalBytesArchived += fileInfo.Size()
	gs.mu.Unlock()
	
	return nil
}

func (gs *GhostShip) generateS3Key(filePath, ruleName string) string {
	// Generate a structured S3 key: ghost-ship-id/rule-name/year/month/day/filename
	now := time.Now()
	fileName := filepath.Base(filePath)
	
	return fmt.Sprintf("%s/%s/%04d/%02d/%02d/%s", 
		gs.id, 
		ruleName,
		now.Year(), 
		now.Month(), 
		now.Day(), 
		fileName)
}

func (gs *GhostShip) reportStatus() {
	status := gs.GetStatus()
	
	// Report to controller if connected
	if gs.controller != nil {
		statusUpdate := StatusUpdate{
			State:         AgentState(status.State),
			ActiveJobs:    status.ActiveJobs,
			CompletedJobs: status.CompletedJobs,
			FailedJobs:    status.FailedJobs,
			BytesArchived: status.TotalBytesArchived,
			Uptime:        status.Uptime,
			LastError:     status.LastError,
		}
		
		if err := gs.controller.SendMessage(MsgTypeStatusUpdate, statusUpdate); err != nil {
			gs.logger.Warn("Failed to send status update to controller", "error", err)
		}
	}
	
	gs.logger.Debug("Ghost ship status",
		"state", status.State,
		"active_jobs", status.ActiveJobs,
		"completed_jobs", status.CompletedJobs)
}

// runControllerConnection manages the connection to the central controller
func (gs *GhostShip) runControllerConnection() {
	defer gs.wg.Done()
	
	gs.logger.Info("Starting controller connection for ghost ship")
	
	for {
		select {
		case <-gs.ctx.Done():
			return
		default:
			if err := gs.controller.Connect(gs.ctx); err != nil {
				gs.logger.Error("Failed to connect to controller", "error", err)
				gs.mu.Lock()
				gs.status.State = GhostShipStateError
				gs.status.LastError = fmt.Sprintf("Controller connection failed: %v", err)
				gs.mu.Unlock()
				
				// Retry after delay
				select {
				case <-time.After(30 * time.Second):
					continue
				case <-gs.ctx.Done():
					return
				}
			}
			
			gs.mu.Lock()
			if gs.status.State == GhostShipStateError {
				gs.status.State = GhostShipStateRunning
				gs.status.LastError = ""
			}
			gs.mu.Unlock()
			
			// Handle messages from controller
			gs.controller.HandleMessages(gs.ctx, gs.handleControllerMessage)
		}
	}
}

// handleControllerMessage processes messages from the central controller
func (gs *GhostShip) handleControllerMessage(message []byte) error {
	var msg ControllerMessage
	if err := json.Unmarshal(message, &msg); err != nil {
		return fmt.Errorf("failed to unmarshal controller message: %w", err)
	}
	
	gs.logger.Debug("Received message from controller", 
		"type", msg.Type, 
		"message_id", msg.ID)
	
	switch msg.Type {
	case MsgTypeJobAssign:
		return gs.handleJobAssignment(msg.Data)
	case MsgTypeJobCancel:
		return gs.handleJobCancellation(msg.Data)
	case MsgTypeConfigUpdate:
		return gs.handleConfigUpdate(msg.Data)
	case MsgTypeShutdown:
		gs.logger.Info("Received shutdown command from controller")
		go func() {
			if err := gs.Stop(); err != nil {
				gs.logger.Error("Error during shutdown", "error", err)
			}
		}()
		return nil
	case MsgTypePing:
		// Respond with pong
		return gs.controller.SendMessage(MsgTypeHeartbeat, nil)
	default:
		gs.logger.Warn("Unknown message type from controller", "type", msg.Type)
	}
	
	return nil
}

// handleJobAssignment processes job assignments from controller
func (gs *GhostShip) handleJobAssignment(data json.RawMessage) error {
	var assignment JobAssignment
	if err := json.Unmarshal(data, &assignment); err != nil {
		return fmt.Errorf("failed to unmarshal job assignment: %w", err)
	}
	
	gs.logger.Info("Received job assignment from controller",
		"job_id", assignment.JobID,
		"type", assignment.Type,
		"path", assignment.Path)
	
	// Create archival rule for this assignment
	rule := ArchivalRule{
		Name:               fmt.Sprintf("controller-job-%s", assignment.JobID),
		Description:        "Job assigned by central controller",
		PathPattern:        assignment.Path,
		Destination:        assignment.Destination,
		StorageClass:       assignment.StorageClass,
		Priority:           assignment.Priority,
		Enabled:            true,
	}
	
	// Find matching files and create job
	candidates, err := gs.findArchivalCandidates(rule)
	if err != nil {
		return fmt.Errorf("failed to find candidates for job: %w", err)
	}
	
	for _, candidate := range candidates {
		job := gs.createArchivalJob(candidate, rule)
		job.ID = assignment.JobID // Use controller-assigned ID
		
		gs.mu.Lock()
		gs.activeJobs[job.ID] = job
		gs.mu.Unlock()
		
		gs.logger.Info("Queued controller-assigned job",
			"job_id", job.ID,
			"file", candidate)
	}
	
	return nil
}

// handleJobCancellation processes job cancellations from controller
func (gs *GhostShip) handleJobCancellation(data json.RawMessage) error {
	var cancelReq struct {
		JobID string `json:"job_id"`
	}
	if err := json.Unmarshal(data, &cancelReq); err != nil {
		return fmt.Errorf("failed to unmarshal job cancellation: %w", err)
	}
	
	gs.logger.Info("Received job cancellation from controller", "job_id", cancelReq.JobID)
	
	gs.mu.Lock()
	if job, exists := gs.activeJobs[cancelReq.JobID]; exists {
		job.State = JobStateCancelled
		job.Error = "Cancelled by controller"
	}
	gs.mu.Unlock()
	
	return nil
}

// handleConfigUpdate processes configuration updates from controller
func (gs *GhostShip) handleConfigUpdate(data json.RawMessage) error {
	gs.logger.Info("Received configuration update from controller")
	// Implementation would update ghost ship configuration
	return nil
}

func (gs *GhostShip) performHealthCheck() {
	// Implementation would check ghost ship health
	gs.mu.Lock()
	gs.status.Healthy = gs.status.State != GhostShipStateError
	gs.mu.Unlock()
}