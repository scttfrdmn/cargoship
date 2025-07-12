// Package launch provides headless launch agent functionality for remote CargoShip deployment
package launch

import (
	"context"
	"fmt"
	"log/slog"
	"sync"
	"time"
)

// Agent represents a headless launch agent that runs on NAS devices
type Agent struct {
	// Core configuration
	id          string
	config      *AgentConfig
	logger      *slog.Logger
	
	// Communication
	controller  *ControllerConnection
	
	// File watching and archival
	watcher     *FileWatcher
	archiver    *LocalArchiver
	
	// State management
	status      AgentStatus
	jobs        map[string]*ArchiveJob
	mu          sync.RWMutex
	
	// Lifecycle
	ctx         context.Context
	cancel      context.CancelFunc
	wg          sync.WaitGroup
}

// AgentConfig holds configuration for a launch agent
type AgentConfig struct {
	// Agent identification
	ID               string    `json:"id" yaml:"id"`
	Name             string    `json:"name" yaml:"name"`
	Description      string    `json:"description" yaml:"description"`
	
	// Controller connection
	ControllerURL    string    `json:"controller_url" yaml:"controller_url"`
	AuthToken        string    `json:"auth_token" yaml:"auth_token"`
	TLSConfig        *TLSConfig `json:"tls_config" yaml:"tls_config"`
	
	// File watching
	WatchPaths       []WatchPath `json:"watch_paths" yaml:"watch_paths"`
	ScanInterval     time.Duration `json:"scan_interval" yaml:"scan_interval"`
	
	// Archive settings
	Archive          ArchiveConfig `json:"archive" yaml:"archive"`
	
	// Health and monitoring
	HealthCheck      HealthConfig `json:"health_check" yaml:"health_check"`
	LogLevel         string       `json:"log_level" yaml:"log_level"`
}

// TLSConfig holds TLS configuration for secure communication
type TLSConfig struct {
	Enabled            bool   `json:"enabled" yaml:"enabled"`
	InsecureSkipVerify bool   `json:"insecure_skip_verify" yaml:"insecure_skip_verify"`
	CertFile           string `json:"cert_file" yaml:"cert_file"`
	KeyFile            string `json:"key_file" yaml:"key_file"`
	CAFile             string `json:"ca_file" yaml:"ca_file"`
}

// WatchPath defines a directory to watch for archival
type WatchPath struct {
	Path            string        `json:"path" yaml:"path"`
	IncludePatterns []string      `json:"include_patterns" yaml:"include_patterns"`
	ExcludePatterns []string      `json:"exclude_patterns" yaml:"exclude_patterns"`
	MinAge          time.Duration `json:"min_age" yaml:"min_age"`
	StorageClass    string        `json:"storage_class" yaml:"storage_class"`
	Recursive       bool          `json:"recursive" yaml:"recursive"`
}

// ArchiveConfig holds archival configuration
type ArchiveConfig struct {
	Destination     string        `json:"destination" yaml:"destination"`
	StorageClass    string        `json:"storage_class" yaml:"storage_class"`
	Compression     string        `json:"compression" yaml:"compression"`
	Encryption      bool          `json:"encryption" yaml:"encryption"`
	MaxConcurrent   int           `json:"max_concurrent" yaml:"max_concurrent"`
	ChunkSize       int64         `json:"chunk_size" yaml:"chunk_size"`
	RetryAttempts   int           `json:"retry_attempts" yaml:"retry_attempts"`
	RetryDelay      time.Duration `json:"retry_delay" yaml:"retry_delay"`
}

// HealthConfig holds health check configuration
type HealthConfig struct {
	Enabled         bool          `json:"enabled" yaml:"enabled"`
	CheckInterval   time.Duration `json:"check_interval" yaml:"check_interval"`
	ReportInterval  time.Duration `json:"report_interval" yaml:"report_interval"`
	MetricsEnabled  bool          `json:"metrics_enabled" yaml:"metrics_enabled"`
}

// AgentStatus represents the current status of an agent
type AgentStatus struct {
	State           AgentState    `json:"state"`
	LastSeen        time.Time     `json:"last_seen"`
	LastHeartbeat   time.Time     `json:"last_heartbeat"`
	Version         string        `json:"version"`
	Uptime          time.Duration `json:"uptime"`
	ActiveJobs      int           `json:"active_jobs"`
	CompletedJobs   int64         `json:"completed_jobs"`
	FailedJobs      int64         `json:"failed_jobs"`
	BytesArchived   int64         `json:"bytes_archived"`
	LastError       string        `json:"last_error,omitempty"`
}

// AgentState represents the current state of an agent
type AgentState string

const (
	AgentStateStarting    AgentState = "starting"
	AgentStateConnecting  AgentState = "connecting" 
	AgentStateReady       AgentState = "ready"
	AgentStateWorking     AgentState = "working"
	AgentStateError       AgentState = "error"
	AgentStateDisconnected AgentState = "disconnected"
	AgentStateStopping    AgentState = "stopping"
)

// ArchiveJob represents a single archival job
type ArchiveJob struct {
	ID              string            `json:"id"`
	Path            string            `json:"path"`
	Destination     string            `json:"destination"`
	StorageClass    string            `json:"storage_class"`
	State           JobState          `json:"state"`
	Progress        float64           `json:"progress"`
	BytesTotal      int64             `json:"bytes_total"`
	BytesCompleted  int64             `json:"bytes_completed"`
	StartTime       time.Time         `json:"start_time"`
	EndTime         *time.Time        `json:"end_time,omitempty"`
	Error           string            `json:"error,omitempty"`
	Metadata        map[string]string `json:"metadata,omitempty"`
}

// JobState represents the state of an archive job
type JobState string

const (
	JobStatePending    JobState = "pending"
	JobStateRunning    JobState = "running"
	JobStateCompleted  JobState = "completed"
	JobStateFailed     JobState = "failed"
	JobStateCancelled  JobState = "cancelled"
)

// NewAgent creates a new launch agent with the given configuration
func NewAgent(config *AgentConfig, logger *slog.Logger) (*Agent, error) {
	if config == nil {
		return nil, fmt.Errorf("agent configuration cannot be nil")
	}
	
	if err := validateAgentConfig(config); err != nil {
		return nil, fmt.Errorf("invalid agent configuration: %w", err)
	}
	
	ctx, cancel := context.WithCancel(context.Background())
	
	agent := &Agent{
		id:     config.ID,
		config: config,
		logger: logger.With("component", "launch-agent", "agent_id", config.ID),
		jobs:   make(map[string]*ArchiveJob),
		ctx:    ctx,
		cancel: cancel,
		status: AgentStatus{
			State:   AgentStateStarting,
			Version: getAgentVersion(),
		},
	}
	
	// Initialize components
	var err error
	
	// Create controller connection
	agent.controller, err = NewControllerConnection(config, logger)
	if err != nil {
		cancel()
		return nil, fmt.Errorf("failed to create controller connection: %w", err)
	}
	
	// Create file watcher
	agent.watcher, err = NewFileWatcher(config.WatchPaths, logger)
	if err != nil {
		cancel()
		return nil, fmt.Errorf("failed to create file watcher: %w", err)
	}
	
	// Create local archiver
	agent.archiver, err = NewLocalArchiver(config.Archive, logger)
	if err != nil {
		cancel()
		return nil, fmt.Errorf("failed to create local archiver: %w", err)
	}
	
	agent.logger.Info("Launch agent created successfully",
		"name", config.Name,
		"controller_url", config.ControllerURL,
		"watch_paths", len(config.WatchPaths))
	
	return agent, nil
}

// Start starts the launch agent and begins operation
func (a *Agent) Start() error {
	a.mu.Lock()
	defer a.mu.Unlock()
	
	if a.status.State != AgentStateStarting {
		return fmt.Errorf("agent is not in starting state")
	}
	
	a.logger.Info("Starting launch agent")
	
	// Start controller connection
	a.wg.Add(1)
	go a.runControllerConnection()
	
	// Start file watcher
	a.wg.Add(1)
	go a.runFileWatcher()
	
	// Start health monitoring
	if a.config.HealthCheck.Enabled {
		a.wg.Add(1)
		go a.runHealthMonitor()
	}
	
	// Start job processor
	a.wg.Add(1)
	go a.runJobProcessor()
	
	a.status.State = AgentStateConnecting
	a.status.LastSeen = time.Now()
	
	a.logger.Info("Launch agent started successfully")
	
	return nil
}

// Stop gracefully stops the launch agent
func (a *Agent) Stop() error {
	a.mu.Lock()
	a.status.State = AgentStateStopping
	a.mu.Unlock()
	
	a.logger.Info("Stopping launch agent")
	
	// Cancel context to signal all goroutines to stop
	a.cancel()
	
	// Wait for all goroutines to finish
	done := make(chan struct{})
	go func() {
		a.wg.Wait()
		close(done)
	}()
	
	// Wait for graceful shutdown or timeout
	select {
	case <-done:
		a.logger.Info("Launch agent stopped gracefully")
	case <-time.After(30 * time.Second):
		a.logger.Warn("Launch agent shutdown timed out")
	}
	
	return nil
}

// GetStatus returns the current status of the agent
func (a *Agent) GetStatus() AgentStatus {
	a.mu.RLock()
	defer a.mu.RUnlock()
	
	status := a.status
	status.ActiveJobs = len(a.jobs)
	status.Uptime = time.Since(a.status.LastSeen)
	
	return status
}

// GetJobs returns all current jobs
func (a *Agent) GetJobs() map[string]*ArchiveJob {
	a.mu.RLock()
	defer a.mu.RUnlock()
	
	jobs := make(map[string]*ArchiveJob)
	for id, job := range a.jobs {
		jobCopy := *job
		jobs[id] = &jobCopy
	}
	
	return jobs
}

// runControllerConnection manages the connection to the CargoShip controller
func (a *Agent) runControllerConnection() {
	defer a.wg.Done()
	
	a.logger.Info("Starting controller connection")
	
	for {
		select {
		case <-a.ctx.Done():
			return
		default:
			if err := a.controller.Connect(a.ctx); err != nil {
				a.logger.Error("Failed to connect to controller", "error", err)
				a.updateStatus(AgentStateError, fmt.Sprintf("Controller connection failed: %v", err))
				
				// Retry after delay
				select {
				case <-time.After(30 * time.Second):
					continue
				case <-a.ctx.Done():
					return
				}
			}
			
			a.updateStatus(AgentStateReady, "")
			
			// Handle messages from controller
			a.controller.HandleMessages(a.ctx, a.handleControllerMessage)
		}
	}
}

// runFileWatcher monitors filesystem for files to archive
func (a *Agent) runFileWatcher() {
	defer a.wg.Done()
	
	a.logger.Info("Starting file watcher")
	
	ticker := time.NewTicker(a.config.ScanInterval)
	defer ticker.Stop()
	
	for {
		select {
		case <-a.ctx.Done():
			return
		case <-ticker.C:
			a.scanForFiles()
		}
	}
}

// runHealthMonitor periodically reports health status
func (a *Agent) runHealthMonitor() {
	defer a.wg.Done()
	
	a.logger.Info("Starting health monitor")
	
	ticker := time.NewTicker(a.config.HealthCheck.ReportInterval)
	defer ticker.Stop()
	
	for {
		select {
		case <-a.ctx.Done():
			return
		case <-ticker.C:
			a.reportHealth()
		}
	}
}

// runJobProcessor handles archival jobs
func (a *Agent) runJobProcessor() {
	defer a.wg.Done()
	
	a.logger.Info("Starting job processor")
	
	// Job processing logic will be implemented here
	for {
		select {
		case <-a.ctx.Done():
			return
		case <-time.After(5 * time.Second):
			a.processJobs()
		}
	}
}

// Helper methods

func (a *Agent) updateStatus(state AgentState, errorMsg string) {
	a.mu.Lock()
	defer a.mu.Unlock()
	
	a.status.State = state
	a.status.LastSeen = time.Now()
	if errorMsg != "" {
		a.status.LastError = errorMsg
	}
}

func (a *Agent) handleControllerMessage(message []byte) error {
	// Handle incoming messages from controller
	// This will include job assignments, configuration updates, etc.
	a.logger.Debug("Received message from controller", "size", len(message))
	return nil
}

func (a *Agent) scanForFiles() {
	// Scan watched directories for files to archive
	a.logger.Debug("Scanning for files to archive")
}

func (a *Agent) reportHealth() {
	// Report health status to controller
	status := a.GetStatus()
	a.logger.Debug("Reporting health status", "state", status.State, "active_jobs", status.ActiveJobs)
}

func (a *Agent) processJobs() {
	// Process pending archival jobs
	a.logger.Debug("Processing archival jobs")
}

func validateAgentConfig(config *AgentConfig) error {
	if config.ID == "" {
		return fmt.Errorf("agent ID cannot be empty")
	}
	
	if config.ControllerURL == "" {
		return fmt.Errorf("controller URL cannot be empty")
	}
	
	if config.AuthToken == "" {
		return fmt.Errorf("auth token cannot be empty")
	}
	
	if len(config.WatchPaths) == 0 {
		return fmt.Errorf("at least one watch path must be configured")
	}
	
	if config.ScanInterval <= 0 {
		config.ScanInterval = 5 * time.Minute // Default
	}
	
	return nil
}

func getAgentVersion() string {
	// Return the current agent version
	return "0.3.0"
}