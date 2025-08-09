package controller

import (
	"context"
	"encoding/json"
	"fmt"
	"log/slog"
	"net/http"
	"sync"
	"time"

	"github.com/gorilla/mux"
	"github.com/gorilla/websocket"
)

// WebServer provides HTTP REST API and web interface for agent management
type WebServer struct {
	// Server configuration
	addr      string
	authToken string
	logger    *slog.Logger

	// Dependencies
	registry *AgentRegistry

	// HTTP server
	server *http.Server
	router *mux.Router

	// WebSocket upgrader for real-time updates
	upgrader websocket.Upgrader

	// Active WebSocket connections for real-time updates
	wsClients map[*websocket.Conn]bool
	wsMutex   sync.RWMutex

	// Lifecycle
	ctx    context.Context
	cancel context.CancelFunc
}

// WebAgentInfo represents agent information for web API responses
type WebAgentInfo struct {
	ID         string            `json:"id"`
	Name       string            `json:"name"`
	Status     string            `json:"status"`
	Endpoint   string            `json:"endpoint"`
	Jobs       int               `json:"jobs"`
	Throughput string            `json:"throughput"`
	LastSeen   time.Time         `json:"last_seen"`
	Progress   float64           `json:"progress"`
	Metadata   map[string]string `json:"metadata"`
	Connected  bool              `json:"connected"`
}

// WebJobInfo represents job information for web API responses
type WebJobInfo struct {
	ID        string    `json:"id"`
	AgentID   string    `json:"agent_id"`
	Type      string    `json:"type"`
	Path      string    `json:"path"`
	Status    string    `json:"status"`
	Progress  float64   `json:"progress"`
	StartTime time.Time `json:"start_time"`
	Size      string    `json:"size"`
	Rate      string    `json:"rate"`
}

// WebMetrics represents system metrics for web API responses
type WebMetrics struct {
	TotalAgents     int           `json:"total_agents"`
	ActiveJobs      int           `json:"active_jobs"`
	CompletedJobs   int64         `json:"completed_jobs"`
	FailedJobs      int64         `json:"failed_jobs"`
	TotalThroughput string        `json:"total_throughput"`
	Uptime          time.Duration `json:"uptime"`
	MemoryUsage     string        `json:"memory_usage"`
	CPUUsage        float64       `json:"cpu_usage"`
	LastUpdate      time.Time     `json:"last_update"`
}

// NewWebServer creates a new web server instance
func NewWebServer(addr, authToken string, registry *AgentRegistry, logger *slog.Logger) *WebServer {
	ctx, cancel := context.WithCancel(context.Background())

	ws := &WebServer{
		addr:      addr,
		authToken: authToken,
		logger:    logger,
		registry:  registry,
		router:    mux.NewRouter(),
		upgrader: websocket.Upgrader{
			CheckOrigin: func(r *http.Request) bool {
				return true // Allow all origins for now
			},
		},
		wsClients: make(map[*websocket.Conn]bool),
		ctx:       ctx,
		cancel:    cancel,
	}

	ws.setupRoutes()

	return ws
}

// setupRoutes configures HTTP routes
func (ws *WebServer) setupRoutes() {
	// API routes
	api := ws.router.PathPrefix("/api/v1").Subrouter()
	api.Use(ws.authMiddleware)

	// Agent management endpoints
	api.HandleFunc("/agents", ws.handleGetAgents).Methods("GET")
	api.HandleFunc("/agents/{id}", ws.handleGetAgent).Methods("GET")
	api.HandleFunc("/agents/{id}/jobs", ws.handleGetAgentJobs).Methods("GET")
	api.HandleFunc("/agents/{id}/disconnect", ws.handleDisconnectAgent).Methods("POST")

	// Job management endpoints
	api.HandleFunc("/jobs", ws.handleGetJobs).Methods("GET")
	api.HandleFunc("/jobs/{id}", ws.handleGetJob).Methods("GET")
	api.HandleFunc("/jobs/{id}/cancel", ws.handleCancelJob).Methods("POST")

	// Metrics endpoint
	api.HandleFunc("/metrics", ws.handleGetMetrics).Methods("GET")

	// WebSocket endpoint for real-time updates
	api.HandleFunc("/ws", ws.handleWebSocket)

	// Static file server for web UI
	ws.router.PathPrefix("/").Handler(http.FileServer(http.Dir("./web/static/")))
}

// authMiddleware validates API authentication
func (ws *WebServer) authMiddleware(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		token := r.Header.Get("Authorization")
		if token != "Bearer "+ws.authToken && ws.authToken != "" {
			http.Error(w, "Unauthorized", http.StatusUnauthorized)
			return
		}
		next.ServeHTTP(w, r)
	})
}

// handleGetAgents returns all connected agents
func (ws *WebServer) handleGetAgents(w http.ResponseWriter, r *http.Request) {
	agents := ws.registry.GetAllAgents()
	webAgents := make([]WebAgentInfo, 0, len(agents))

	for _, agent := range agents {
		webAgents = append(webAgents, WebAgentInfo{
			ID:         agent.ID,
			Name:       agent.Name,
			Status:     string(agent.Status.State),
			Endpoint:   "", // Will need to get from connection
			Jobs:       len(agent.Jobs),
			Throughput: "0 MB/s", // Will calculate from agent data
			LastSeen:   agent.LastSeen,
			Progress:   0.0, // Will calculate from jobs
			Metadata:   agent.Metadata,
			Connected:  true, // If in registry, it's connected
		})
	}

	w.Header().Set("Content-Type", "application/json")
	_ = json.NewEncoder(w).Encode(webAgents)
}

// handleGetAgent returns a specific agent by ID
func (ws *WebServer) handleGetAgent(w http.ResponseWriter, r *http.Request) {
	vars := mux.Vars(r)
	agentID := vars["id"]

	agent, exists := ws.registry.GetAgent(agentID)
	if !exists {
		http.Error(w, "Agent not found", http.StatusNotFound)
		return
	}

	webAgent := WebAgentInfo{
		ID:         agent.ID,
		Name:       agent.Name,
		Status:     string(agent.Status.State),
		Endpoint:   "", // Will need to get from connection
		Jobs:       len(agent.Jobs),
		Throughput: "0 MB/s", // Will calculate from agent data
		LastSeen:   agent.LastSeen,
		Progress:   0.0, // Will calculate from jobs
		Metadata:   agent.Metadata,
		Connected:  true, // If in registry, it's connected
	}

	w.Header().Set("Content-Type", "application/json")
	_ = json.NewEncoder(w).Encode(webAgent)
}

// handleGetAgentJobs returns jobs for a specific agent
func (ws *WebServer) handleGetAgentJobs(w http.ResponseWriter, r *http.Request) {
	vars := mux.Vars(r)
	agentID := vars["id"]

	agent, exists := ws.registry.GetAgent(agentID)
	if !exists {
		http.Error(w, "Agent not found", http.StatusNotFound)
		return
	}

	webJobs := make([]WebJobInfo, 0, len(agent.Jobs))
	for _, job := range agent.Jobs {
		webJobs = append(webJobs, WebJobInfo{
			ID:        job.ID,
			AgentID:   agentID,
			Type:      job.Type,
			Path:      job.Path,
			Status:    string(job.Status),
			Progress:  job.Progress,
			StartTime: job.AssignedAt,
			Size:      "0 MB",   // Will calculate from job data
			Rate:      "0 MB/s", // Will calculate from job data
		})
	}

	w.Header().Set("Content-Type", "application/json")
	_ = json.NewEncoder(w).Encode(webJobs)
}

// handleDisconnectAgent forcibly disconnects an agent
func (ws *WebServer) handleDisconnectAgent(w http.ResponseWriter, r *http.Request) {
	vars := mux.Vars(r)
	agentID := vars["id"]

	agent, exists := ws.registry.GetAgent(agentID)
	if !exists {
		http.Error(w, "Agent not found", http.StatusNotFound)
		return
	}

	// Disconnect the agent through its connection
	if agent.Connection != nil {
		_ = agent.Connection.Close()
	}

	w.WriteHeader(http.StatusOK)
	ws.broadcastUpdate("agent_disconnected", map[string]string{"agent_id": agentID})
}

// handleGetJobs returns all jobs across all agents
func (ws *WebServer) handleGetJobs(w http.ResponseWriter, r *http.Request) {
	agents := ws.registry.GetAllAgents()
	var allJobs []WebJobInfo

	for _, agent := range agents {
		for _, job := range agent.Jobs {
			allJobs = append(allJobs, WebJobInfo{
				ID:        job.ID,
				AgentID:   agent.ID,
				Type:      job.Type,
				Path:      job.Path,
				Status:    string(job.Status),
				Progress:  job.Progress,
				StartTime: job.AssignedAt,
				Size:      "0 MB",   // Will calculate from job data
				Rate:      "0 MB/s", // Will calculate from job data
			})
		}
	}

	w.Header().Set("Content-Type", "application/json")
	_ = json.NewEncoder(w).Encode(allJobs)
}

// handleGetJob returns a specific job by ID
func (ws *WebServer) handleGetJob(w http.ResponseWriter, r *http.Request) {
	vars := mux.Vars(r)
	jobID := vars["id"]

	// Find job across all agents
	agents := ws.registry.GetAllAgents()
	for _, agent := range agents {
		for _, job := range agent.Jobs {
			if job.ID == jobID {
				webJob := WebJobInfo{
					ID:        job.ID,
					AgentID:   agent.ID,
					Type:      job.Type,
					Path:      job.Path,
					Status:    string(job.Status),
					Progress:  job.Progress,
					StartTime: job.AssignedAt,
					Size:      "0 MB",   // Will calculate from job data
					Rate:      "0 MB/s", // Will calculate from job data
				}

				w.Header().Set("Content-Type", "application/json")
				_ = json.NewEncoder(w).Encode(webJob)
				return
			}
		}
	}

	http.Error(w, "Job not found", http.StatusNotFound)
}

// handleCancelJob cancels a specific job
func (ws *WebServer) handleCancelJob(w http.ResponseWriter, r *http.Request) {
	vars := mux.Vars(r)
	jobID := vars["id"]

	// Implementation would depend on job management system
	// For now, return success
	w.WriteHeader(http.StatusOK)
	ws.broadcastUpdate("job_cancelled", map[string]string{"job_id": jobID})
}

// handleGetMetrics returns system metrics
func (ws *WebServer) handleGetMetrics(w http.ResponseWriter, r *http.Request) {
	agents := ws.registry.GetAllAgents()

	totalAgents := len(agents)
	activeJobs := 0
	var completedJobs, failedJobs int64

	for _, agent := range agents {
		for _, job := range agent.Jobs {
			switch job.Status {
			case JobStatusRunning, JobStatusAssigned:
				activeJobs++
			case JobStatusCompleted:
				completedJobs++
			case JobStatusFailed:
				failedJobs++
			}
		}
	}

	metrics := WebMetrics{
		TotalAgents:     totalAgents,
		ActiveJobs:      activeJobs,
		CompletedJobs:   completedJobs,
		FailedJobs:      failedJobs,
		TotalThroughput: "0 MB/s",               // Would calculate from agent data
		Uptime:          time.Since(time.Now()), // Would track actual uptime
		MemoryUsage:     "0 MB",                 // Would get from system metrics
		CPUUsage:        0.0,                    // Would get from system metrics
		LastUpdate:      time.Now(),
	}

	w.Header().Set("Content-Type", "application/json")
	_ = json.NewEncoder(w).Encode(metrics)
}

// handleWebSocket handles WebSocket connections for real-time updates
func (ws *WebServer) handleWebSocket(w http.ResponseWriter, r *http.Request) {
	conn, err := ws.upgrader.Upgrade(w, r, nil)
	if err != nil {
		ws.logger.Error("WebSocket upgrade failed", "error", err)
		return
	}
	defer func() { _ = conn.Close() }()

	// Add to active connections
	ws.wsMutex.Lock()
	ws.wsClients[conn] = true
	ws.wsMutex.Unlock()

	// Remove on disconnect
	defer func() {
		ws.wsMutex.Lock()
		delete(ws.wsClients, conn)
		ws.wsMutex.Unlock()
	}()

	// Keep connection alive
	ticker := time.NewTicker(30 * time.Second)
	defer ticker.Stop()

	for {
		select {
		case <-ticker.C:
			if err := conn.WriteMessage(websocket.PingMessage, nil); err != nil {
				ws.logger.Debug("WebSocket ping failed", "error", err)
				return
			}
		case <-ws.ctx.Done():
			return
		}
	}
}

// broadcastUpdate sends updates to all connected WebSocket clients
func (ws *WebServer) broadcastUpdate(eventType string, data interface{}) {
	message := map[string]interface{}{
		"type":      eventType,
		"data":      data,
		"timestamp": time.Now(),
	}

	messageBytes, err := json.Marshal(message)
	if err != nil {
		ws.logger.Error("Failed to marshal WebSocket message", "error", err)
		return
	}

	ws.wsMutex.RLock()
	defer ws.wsMutex.RUnlock()

	for conn := range ws.wsClients {
		if err := conn.WriteMessage(websocket.TextMessage, messageBytes); err != nil {
			ws.logger.Debug("Failed to send WebSocket message", "error", err)
			_ = conn.Close()
			delete(ws.wsClients, conn)
		}
	}
}

// Start starts the web server
func (ws *WebServer) Start() error {
	ws.server = &http.Server{
		Addr:    ws.addr,
		Handler: ws.router,
	}

	ws.logger.Info("Starting web server", "addr", ws.addr)

	if err := ws.server.ListenAndServe(); err != nil && err != http.ErrServerClosed {
		return fmt.Errorf("web server failed: %w", err)
	}

	return nil
}

// Stop stops the web server
func (ws *WebServer) Stop() error {
	ws.cancel()

	if ws.server != nil {
		ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
		defer cancel()
		return ws.server.Shutdown(ctx)
	}

	return nil
}
