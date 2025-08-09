// Package controller provides the WebSocket server and agent management for CargoShip launch agents
package controller

import (
	"context"
	"fmt"
	"log/slog"
	"sync"
	"time"

	"github.com/scttfrdmn/cargoship/pkg/launch"
)

// AgentRegistry manages all connected launch agents
type AgentRegistry struct {
	// Agent connections
	agents map[string]*ConnectedAgent
	mu     sync.RWMutex

	// Configuration
	logger    *slog.Logger
	authToken string

	// Lifecycle
	ctx    context.Context
	cancel context.CancelFunc
	wg     sync.WaitGroup
}

// ConnectedAgent represents a connected launch agent
type ConnectedAgent struct {
	// Agent information
	ID           string             `json:"id"`
	Name         string             `json:"name"`
	Description  string             `json:"description"`
	Version      string             `json:"version"`
	Capabilities []string           `json:"capabilities"`
	WatchPaths   []launch.WatchPath `json:"watch_paths"`
	Metadata     map[string]string  `json:"metadata"`

	// Connection details
	Connection  AgentConnectionInterface `json:"-"`
	ConnectedAt time.Time                `json:"connected_at"`
	LastSeen    time.Time                `json:"last_seen"`

	// Status tracking
	Status launch.AgentStatus      `json:"status"`
	Jobs   map[string]*AssignedJob `json:"jobs"`
}

// AssignedJob represents a job assigned to an agent
type AssignedJob struct {
	ID           string                 `json:"id"`
	Type         string                 `json:"type"`
	Path         string                 `json:"path"`
	Destination  string                 `json:"destination"`
	StorageClass string                 `json:"storage_class"`
	Priority     int                    `json:"priority"`
	Parameters   map[string]interface{} `json:"parameters,omitempty"`
	AssignedAt   time.Time              `json:"assigned_at"`
	Status       JobStatus              `json:"status"`
	Progress     float64                `json:"progress"`
	Error        string                 `json:"error,omitempty"`
}

// JobStatus represents the status of an assigned job
type JobStatus string

const (
	JobStatusAssigned  JobStatus = "assigned"
	JobStatusRunning   JobStatus = "running"
	JobStatusCompleted JobStatus = "completed"
	JobStatusFailed    JobStatus = "failed"
	JobStatusCancelled JobStatus = "cancelled"
)

// NewAgentRegistry creates a new agent registry
func NewAgentRegistry(authToken string, logger *slog.Logger) *AgentRegistry {
	ctx, cancel := context.WithCancel(context.Background())

	return &AgentRegistry{
		agents:    make(map[string]*ConnectedAgent),
		logger:    logger.With("component", "agent-registry"),
		authToken: authToken,
		ctx:       ctx,
		cancel:    cancel,
	}
}

// Start starts the agent registry background services
func (ar *AgentRegistry) Start() error {
	ar.logger.Info("Starting agent registry")

	// Start health check monitor
	ar.wg.Add(1)
	go ar.runHealthMonitor()

	return nil
}

// Stop gracefully stops the agent registry
func (ar *AgentRegistry) Stop() error {
	ar.logger.Info("Stopping agent registry")

	ar.cancel()
	ar.wg.Wait()

	// Disconnect all agents
	ar.mu.Lock()
	defer ar.mu.Unlock()

	for _, agent := range ar.agents {
		if agent.Connection != nil {
			_ = agent.Connection.Close()
		}
	}

	ar.logger.Info("Agent registry stopped")
	return nil
}

// AgentConnectionInterface defines the interface for agent connections
type AgentConnectionInterface interface {
	SetAgent(agent *ConnectedAgent)
	OnClose(callback func())
	SendMessage(msgType launch.MessageType, data interface{}) error
	Close() error
}

// RegisterAgent registers a new agent connection
func (ar *AgentRegistry) RegisterAgent(conn AgentConnectionInterface, req *launch.RegistrationRequest) (*launch.RegistrationResponse, error) {
	ar.mu.Lock()
	defer ar.mu.Unlock()

	ar.logger.Info("Registering new agent", "agent_id", req.AgentID, "name", req.Name)

	// Check if agent already exists
	if existing, exists := ar.agents[req.AgentID]; exists {
		// Close existing connection if any
		if existing.Connection != nil {
			_ = existing.Connection.Close()
		}
		ar.logger.Warn("Agent reconnecting", "agent_id", req.AgentID)
	}

	// Create connected agent
	agent := &ConnectedAgent{
		ID:           req.AgentID,
		Name:         req.Name,
		Description:  req.Description,
		Version:      req.Version,
		Capabilities: req.Capabilities,
		WatchPaths:   req.WatchPaths,
		Metadata:     req.Metadata,
		Connection:   conn,
		ConnectedAt:  time.Now(),
		LastSeen:     time.Now(),
		Status: launch.AgentStatus{
			State:   launch.AgentStateReady,
			Version: req.Version,
		},
		Jobs: make(map[string]*AssignedJob),
	}

	// Store agent
	ar.agents[req.AgentID] = agent

	// Set up connection callbacks
	conn.SetAgent(agent)
	conn.OnClose(func() {
		ar.handleAgentDisconnection(req.AgentID)
	})

	ar.logger.Info("Agent registered successfully",
		"agent_id", req.AgentID,
		"name", req.Name,
		"capabilities", req.Capabilities,
		"watch_paths", len(req.WatchPaths))

	// Return success response
	return &launch.RegistrationResponse{
		Success:   true,
		AgentID:   req.AgentID,
		SessionID: generateSessionID(),
		Configuration: map[string]interface{}{
			"heartbeat_interval": "30s",
			"log_level":          "info",
		},
		Message: "Registration successful",
	}, nil
}

// GetAgent returns information about a specific agent
func (ar *AgentRegistry) GetAgent(agentID string) (*ConnectedAgent, bool) {
	ar.mu.RLock()
	defer ar.mu.RUnlock()

	agent, exists := ar.agents[agentID]
	if !exists {
		return nil, false
	}

	// Return a copy to avoid race conditions
	agentCopy := *agent
	agentCopy.Jobs = make(map[string]*AssignedJob)
	for k, v := range agent.Jobs {
		jobCopy := *v
		agentCopy.Jobs[k] = &jobCopy
	}

	return &agentCopy, true
}

// GetAllAgents returns information about all connected agents
func (ar *AgentRegistry) GetAllAgents() []*ConnectedAgent {
	ar.mu.RLock()
	defer ar.mu.RUnlock()

	agents := make([]*ConnectedAgent, 0, len(ar.agents))
	for _, agent := range ar.agents {
		agentCopy := *agent
		agentCopy.Jobs = make(map[string]*AssignedJob)
		for k, v := range agent.Jobs {
			jobCopy := *v
			agentCopy.Jobs[k] = &jobCopy
		}
		agents = append(agents, &agentCopy)
	}

	return agents
}

// AssignJob assigns a job to a specific agent
func (ar *AgentRegistry) AssignJob(agentID string, jobAssignment *launch.JobAssignment) error {
	ar.mu.Lock()
	defer ar.mu.Unlock()

	agent, exists := ar.agents[agentID]
	if !exists {
		return ErrAgentNotFound
	}

	if agent.Connection == nil {
		return ErrAgentNotConnected
	}

	// Create assigned job
	assignedJob := &AssignedJob{
		ID:           jobAssignment.JobID,
		Type:         jobAssignment.Type,
		Path:         jobAssignment.Path,
		Destination:  jobAssignment.Destination,
		StorageClass: jobAssignment.StorageClass,
		Priority:     jobAssignment.Priority,
		Parameters:   jobAssignment.Parameters,
		AssignedAt:   time.Now(),
		Status:       JobStatusAssigned,
		Progress:     0.0,
	}

	// Store job
	agent.Jobs[jobAssignment.JobID] = assignedJob

	// Send job to agent
	err := agent.Connection.SendMessage(launch.MsgTypeJobAssign, jobAssignment)
	if err != nil {
		delete(agent.Jobs, jobAssignment.JobID)
		return err
	}

	ar.logger.Info("Job assigned to agent",
		"agent_id", agentID,
		"job_id", jobAssignment.JobID,
		"path", jobAssignment.Path)

	return nil
}

// UpdateAgentStatus updates the status of an agent
func (ar *AgentRegistry) UpdateAgentStatus(agentID string, status *launch.StatusUpdate) error {
	ar.mu.Lock()
	defer ar.mu.Unlock()

	agent, exists := ar.agents[agentID]
	if !exists {
		return ErrAgentNotFound
	}

	// Update agent status
	agent.Status.State = status.State
	agent.Status.ActiveJobs = status.ActiveJobs
	agent.Status.CompletedJobs = status.CompletedJobs
	agent.Status.FailedJobs = status.FailedJobs
	agent.Status.BytesArchived = status.BytesArchived
	agent.Status.Uptime = status.Uptime
	agent.Status.LastError = status.LastError
	agent.LastSeen = time.Now()

	ar.logger.Debug("Agent status updated",
		"agent_id", agentID,
		"state", status.State,
		"active_jobs", status.ActiveJobs)

	return nil
}

// GetAgentCount returns the number of connected agents
func (ar *AgentRegistry) GetAgentCount() int {
	ar.mu.RLock()
	defer ar.mu.RUnlock()

	return len(ar.agents)
}

// handleAgentDisconnection handles when an agent disconnects
func (ar *AgentRegistry) handleAgentDisconnection(agentID string) {
	ar.mu.Lock()
	defer ar.mu.Unlock()

	agent, exists := ar.agents[agentID]
	if !exists {
		return
	}

	ar.logger.Info("Agent disconnected", "agent_id", agentID, "name", agent.Name)

	// Update agent status
	agent.Status.State = launch.AgentStateDisconnected
	agent.Connection = nil

	// Mark running jobs as failed
	for _, job := range agent.Jobs {
		if job.Status == JobStatusRunning || job.Status == JobStatusAssigned {
			job.Status = JobStatusFailed
			job.Error = "Agent disconnected"
		}
	}
}

// runHealthMonitor monitors agent health and removes stale connections
func (ar *AgentRegistry) runHealthMonitor() {
	defer ar.wg.Done()

	ticker := time.NewTicker(30 * time.Second)
	defer ticker.Stop()

	for {
		select {
		case <-ar.ctx.Done():
			return
		case <-ticker.C:
			ar.checkAgentHealth()
		}
	}
}

// checkAgentHealth checks the health of all connected agents
func (ar *AgentRegistry) checkAgentHealth() {
	ar.mu.Lock()
	defer ar.mu.Unlock()

	now := time.Now()
	staleThreshold := 5 * time.Minute

	for agentID, agent := range ar.agents {
		if now.Sub(agent.LastSeen) > staleThreshold && agent.Connection != nil {
			ar.logger.Warn("Agent appears stale, disconnecting",
				"agent_id", agentID,
				"last_seen", agent.LastSeen)

			_ = agent.Connection.Close()
			agent.Connection = nil
			agent.Status.State = launch.AgentStateDisconnected
		}
	}
}

// generateSessionID generates a unique session ID
func generateSessionID() string {
	return fmt.Sprintf("session-%d", time.Now().UnixNano())
}

// Common errors
var (
	ErrAgentNotFound     = fmt.Errorf("agent not found")
	ErrAgentNotConnected = fmt.Errorf("agent not connected")
)
