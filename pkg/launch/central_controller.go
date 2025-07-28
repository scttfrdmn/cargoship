package launch

import (
	"context"
	"encoding/json"
	"fmt"
	"log/slog"
	"net/http"
	"strings"
	"sync"
	"time"

	"github.com/gorilla/mux"
	"github.com/gorilla/websocket"
)

// CentralController coordinates distributed ghost ships and launch agents
type CentralController struct {
	config     *CentralControllerConfig
	logger     *slog.Logger
	
	// Connection management
	upgrader   websocket.Upgrader
	agents     map[string]*ConnectedAgent
	ghostShips map[string]*ConnectedGhostShip
	mu         sync.RWMutex
	
	// HTTP server
	server     *http.Server
	router     *mux.Router
	
	// Lifecycle
	ctx        context.Context
	cancel     context.CancelFunc
	wg         sync.WaitGroup
}

// CentralControllerConfig holds configuration for the central controller
type CentralControllerConfig struct {
	// Server configuration
	ListenAddress string        `json:"listen_address" yaml:"listen_address"`
	Port          int           `json:"port" yaml:"port"`
	TLSEnabled    bool          `json:"tls_enabled" yaml:"tls_enabled"`
	CertFile      string        `json:"cert_file" yaml:"cert_file"`
	KeyFile       string        `json:"key_file" yaml:"key_file"`
	
	// Authentication
	AuthEnabled   bool          `json:"auth_enabled" yaml:"auth_enabled"`
	AuthTokens    []string      `json:"auth_tokens" yaml:"auth_tokens"`
	
	// Monitoring
	HealthCheck   HealthConfig  `json:"health_check" yaml:"health_check"`
	MetricsPort   int           `json:"metrics_port" yaml:"metrics_port"`
	
	// Agent management
	AgentTimeout  time.Duration `json:"agent_timeout" yaml:"agent_timeout"`
	PingInterval  time.Duration `json:"ping_interval" yaml:"ping_interval"`
}

// ConnectedAgent represents a connected launch agent
type ConnectedAgent struct {
	ID           string                  `json:"id"`
	Name         string                  `json:"name"`
	Description  string                  `json:"description"`
	Capabilities []string                `json:"capabilities"`
	WatchPaths   []WatchPath            `json:"watch_paths"`
	Metadata     map[string]string      `json:"metadata"`
	
	// Connection details
	Connection   *websocket.Conn        `json:"-"`
	SessionID    string                 `json:"session_id"`
	ConnectedAt  time.Time              `json:"connected_at"`
	LastSeen     time.Time              `json:"last_seen"`
	LastPing     time.Time              `json:"last_ping"`
	
	// Status
	Status       AgentStatus            `json:"status"`
	Jobs         map[string]*ArchiveJob `json:"jobs"`
	
	// Communication (currently unused but reserved for future use)
	_ chan []byte      `json:"-"`
	_ sync.RWMutex     `json:"-"`
}

// ConnectedGhostShip represents a connected autonomous ghost ship
type ConnectedGhostShip struct {
	ID           string                    `json:"id"`
	Name         string                    `json:"name"`
	Description  string                    `json:"description"`
	WatchPaths   []WatchPath              `json:"watch_paths"`
	Rules        []ArchivalRule           `json:"rules"`
	
	// Connection details
	Connection   *websocket.Conn          `json:"-"`
	SessionID    string                   `json:"session_id"`
	ConnectedAt  time.Time                `json:"connected_at"`
	LastSeen     time.Time                `json:"last_seen"`
	
	// Status
	Status       GhostShipStatus          `json:"status"`
	Jobs         map[string]*ArchivalJob  `json:"jobs"`
	
	// Communication (currently unused but reserved for future use) 
	_ chan []byte      `json:"-"`
	_ sync.RWMutex     `json:"-"`
}

// NewCentralController creates a new central controller
func NewCentralController(config *CentralControllerConfig, logger *slog.Logger) (*CentralController, error) {
	if config == nil {
		return nil, fmt.Errorf("central controller configuration cannot be nil")
	}
	
	if logger == nil {
		logger = slog.Default()
	}
	
	// Validate configuration
	if err := validateCentralControllerConfig(config); err != nil {
		return nil, fmt.Errorf("invalid central controller configuration: %w", err)
	}
	
	ctx, cancel := context.WithCancel(context.Background())
	
	cc := &CentralController{
		config:     config,
		logger:     logger.With("component", "central-controller"),
		agents:     make(map[string]*ConnectedAgent),
		ghostShips: make(map[string]*ConnectedGhostShip), 
		upgrader: websocket.Upgrader{
			CheckOrigin: func(r *http.Request) bool {
				// In production, implement proper origin checking
				return true
			},
			HandshakeTimeout: 10 * time.Second,
		},
		ctx:    ctx,
		cancel: cancel,
	}
	
	// Setup HTTP router
	cc.setupRouter()
	
	cc.logger.Info("Central controller created successfully", 
		"listen_address", config.ListenAddress,
		"port", config.Port,
		"tls_enabled", config.TLSEnabled)
	
	return cc, nil
}

// Start starts the central controller
func (cc *CentralController) Start() error {
	cc.logger.Info("Starting CargoShip central controller")
	
	// Create HTTP server
	addr := fmt.Sprintf("%s:%d", cc.config.ListenAddress, cc.config.Port)
	cc.server = &http.Server{
		Addr:         addr,
		Handler:      cc.router,
		ReadTimeout:  30 * time.Second,
		WriteTimeout: 30 * time.Second,
		IdleTimeout:  120 * time.Second,
	}
	
	// Start connection monitor
	cc.wg.Add(1)
	go cc.runConnectionMonitor()
	
	// Start agent health checker
	cc.wg.Add(1)
	go cc.runHealthChecker()
	
	// Start HTTP server
	cc.wg.Add(1)
	go cc.runHTTPServer()
	
	cc.logger.Info("🚢 CargoShip central controller started successfully",
		"address", addr,
		"tls_enabled", cc.config.TLSEnabled)
	
	return nil
}

// Stop gracefully stops the central controller
func (cc *CentralController) Stop() error {
	cc.logger.Info("Stopping CargoShip central controller")
	
	// Cancel context
	cc.cancel()
	
	// Close all agent connections
	cc.mu.Lock()
	for _, agent := range cc.agents {
		if err := agent.Connection.Close(); err != nil {
			cc.logger.Error("Failed to close agent connection", "error", err)
		}
	}
	for _, ghost := range cc.ghostShips {
		if err := ghost.Connection.Close(); err != nil {
			cc.logger.Error("Failed to close ghost ship connection", "error", err)
		}
	}
	cc.mu.Unlock()
	
	// Shutdown HTTP server
	if cc.server != nil {
		ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
		defer cancel()
		if err := cc.server.Shutdown(ctx); err != nil {
			cc.logger.Error("Failed to shutdown HTTP server", "error", err)
		}
	}
	
	// Wait for goroutines
	done := make(chan struct{})
	go func() {
		cc.wg.Wait()
		close(done)
	}()
	
	select {
	case <-done:
		cc.logger.Info("Central controller stopped gracefully")
	case <-time.After(30 * time.Second):
		cc.logger.Warn("Central controller shutdown timed out")
	}
	
	return nil
}

// setupRouter configures HTTP routes
func (cc *CentralController) setupRouter() {
	cc.router = mux.NewRouter()
	
	// Public health endpoint (no auth required)
	cc.router.HandleFunc("/health", cc.healthCheck).Methods("GET")
	
	// WebSocket endpoints
	cc.router.HandleFunc("/api/v1/agents/connect", cc.handleAgentConnect).Methods("GET")
	cc.router.HandleFunc("/api/v1/ghostships/connect", cc.handleGhostShipConnect).Methods("GET")
	
	// REST API endpoints
	api := cc.router.PathPrefix("/api/v1").Subrouter()
	api.Use(cc.authMiddleware)
	
	// Agent management
	api.HandleFunc("/agents", cc.listAgents).Methods("GET")
	api.HandleFunc("/agents/{id}", cc.getAgent).Methods("GET")
	api.HandleFunc("/agents/{id}/jobs", cc.getAgentJobs).Methods("GET")
	api.HandleFunc("/agents/{id}/assign", cc.assignJobToAgent).Methods("POST")
	api.HandleFunc("/agents/{id}/cancel/{jobId}", cc.cancelAgentJob).Methods("POST")
	
	// Ghost ship management
	api.HandleFunc("/ghostships", cc.listGhostShips).Methods("GET")
	api.HandleFunc("/ghostships/{id}", cc.getGhostShip).Methods("GET")
	api.HandleFunc("/ghostships/{id}/jobs", cc.getGhostShipJobs).Methods("GET")
	api.HandleFunc("/ghostships/{id}/launch", cc.launchGhostShip).Methods("POST")
	api.HandleFunc("/ghostships/{id}/stop", cc.stopGhostShip).Methods("POST")
	
	// Global operations
	api.HandleFunc("/status", cc.getGlobalStatus).Methods("GET")
	api.HandleFunc("/health", cc.healthCheck).Methods("GET")
	
	// Static files (if needed for dashboard)
	cc.router.PathPrefix("/").Handler(http.FileServer(http.Dir("./web/static/")))
}

// runHTTPServer runs the HTTP server
func (cc *CentralController) runHTTPServer() {
	defer cc.wg.Done()
	
	var err error
	if cc.config.TLSEnabled {
		err = cc.server.ListenAndServeTLS(cc.config.CertFile, cc.config.KeyFile)
	} else {
		err = cc.server.ListenAndServe()
	}
	
	if err != nil && err != http.ErrServerClosed {
		cc.logger.Error("HTTP server error", "error", err)
	}
}

// runConnectionMonitor monitors agent and ghost ship connections
func (cc *CentralController) runConnectionMonitor() {
	defer cc.wg.Done()
	
	ticker := time.NewTicker(cc.config.PingInterval)
	defer ticker.Stop()
	
	for {
		select {
		case <-cc.ctx.Done():
			return
		case <-ticker.C:
			cc.checkConnections()
		}
	}
}

// runHealthChecker performs health checks on connected agents
func (cc *CentralController) runHealthChecker() {
	defer cc.wg.Done()
	
	ticker := time.NewTicker(cc.config.HealthCheck.CheckInterval)
	defer ticker.Stop()
	
	for {
		select {
		case <-cc.ctx.Done():
			return
		case <-ticker.C:
			cc.performHealthChecks()
		}
	}
}

// WebSocket handlers

// handleAgentConnect handles agent WebSocket connections
func (cc *CentralController) handleAgentConnect(w http.ResponseWriter, r *http.Request) {
	// Upgrade to WebSocket
	conn, err := cc.upgrader.Upgrade(w, r, nil)
	if err != nil {
		cc.logger.Error("Failed to upgrade agent connection", "error", err)
		return
	}
	defer func() {
		if err := conn.Close(); err != nil {
			cc.logger.Error("Failed to close WebSocket connection", "error", err)
		}
	}()
	
	// Handle agent connection
	if err := cc.handleAgentConnection(conn, r); err != nil {
		cc.logger.Error("Agent connection error", "error", err)
	}
}

// handleGhostShipConnect handles ghost ship WebSocket connections  
func (cc *CentralController) handleGhostShipConnect(w http.ResponseWriter, r *http.Request) {
	// Upgrade to WebSocket
	conn, err := cc.upgrader.Upgrade(w, r, nil)
	if err != nil {
		cc.logger.Error("Failed to upgrade ghost ship connection", "error", err)
		return
	}
	defer func() {
		if err := conn.Close(); err != nil {
			cc.logger.Error("Failed to close WebSocket connection", "error", err)
		}
	}()
	
	// Handle ghost ship connection
	if err := cc.handleGhostShipConnection(conn, r); err != nil {
		cc.logger.Error("Ghost ship connection error", "error", err)
	}
}

// REST API handlers

// listAgents returns all connected agents
func (cc *CentralController) listAgents(w http.ResponseWriter, r *http.Request) {
	cc.mu.RLock()
	agents := make([]*ConnectedAgent, 0, len(cc.agents))
	for _, agent := range cc.agents {
		agentCopy := &ConnectedAgent{
			ID:           agent.ID,
			Name:         agent.Name,
			ConnectedAt:  agent.ConnectedAt,
			LastSeen:     agent.LastSeen,
			Status:       agent.Status,
			Connection:   nil, // Don't serialize connection
		}
		agents = append(agents, agentCopy)
	}
	cc.mu.RUnlock()
	
	w.Header().Set("Content-Type", "application/json")
	if err := json.NewEncoder(w).Encode(map[string]interface{}{
		"agents": agents,
		"count":  len(agents),
	}); err != nil {
		cc.logger.Error("Failed to encode agents response", "error", err)
		http.Error(w, "Internal server error", http.StatusInternalServerError)
	}
}

// listGhostShips returns all connected ghost ships
func (cc *CentralController) listGhostShips(w http.ResponseWriter, r *http.Request) {
	cc.mu.RLock()
	ghostShips := make([]*ConnectedGhostShip, 0, len(cc.ghostShips))
	for _, ghost := range cc.ghostShips {
		ghostCopy := &ConnectedGhostShip{
			ID:           ghost.ID,
			Name:         ghost.Name,
			ConnectedAt:  ghost.ConnectedAt,
			LastSeen:     ghost.LastSeen,
			Status:       ghost.Status,
			Connection:   nil, // Don't serialize connection
		}
		ghostShips = append(ghostShips, ghostCopy)
	}
	cc.mu.RUnlock()
	
	w.Header().Set("Content-Type", "application/json")
	if err := json.NewEncoder(w).Encode(map[string]interface{}{
		"ghost_ships": ghostShips,
		"count":       len(ghostShips),
	}); err != nil {
		cc.logger.Error("Failed to encode ghost ships response", "error", err)
		http.Error(w, "Internal server error", http.StatusInternalServerError)
	}
}

// getGlobalStatus returns global system status
func (cc *CentralController) getGlobalStatus(w http.ResponseWriter, r *http.Request) {
	cc.mu.RLock()
	agentCount := len(cc.agents)
	ghostShipCount := len(cc.ghostShips)
	
	var totalActiveJobs int
	var totalCompletedJobs int64
	var totalFailedJobs int64
	
	for _, agent := range cc.agents {
		totalActiveJobs += agent.Status.ActiveJobs
		totalCompletedJobs += agent.Status.CompletedJobs
		totalFailedJobs += agent.Status.FailedJobs
	}
	
	for _, ghost := range cc.ghostShips {
		totalActiveJobs += ghost.Status.ActiveJobs
		totalCompletedJobs += ghost.Status.CompletedJobs
		totalFailedJobs += ghost.Status.FailedJobs
	}
	cc.mu.RUnlock()
	
	status := map[string]interface{}{
		"controller_status": "running",
		"uptime":           time.Since(time.Now()), // This would be tracked properly
		"connected_agents": agentCount,
		"connected_ghost_ships": ghostShipCount,
		"total_active_jobs": totalActiveJobs,
		"total_completed_jobs": totalCompletedJobs,
		"total_failed_jobs": totalFailedJobs,
		"timestamp": time.Now(),
	}
	
	w.Header().Set("Content-Type", "application/json")
	if err := json.NewEncoder(w).Encode(status); err != nil {
		cc.logger.Error("Failed to encode status response", "error", err)
		http.Error(w, "Internal server error", http.StatusInternalServerError)
	}
}

// healthCheck returns controller health status
func (cc *CentralController) healthCheck(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Content-Type", "application/json")
	if err := json.NewEncoder(w).Encode(map[string]interface{}{
		"status": "healthy",
		"timestamp": time.Now(),
	}); err != nil {
		cc.logger.Error("Failed to encode health check response", "error", err)
		http.Error(w, "Internal server error", http.StatusInternalServerError)
	}
}

// Middleware

// authMiddleware handles authentication
func (cc *CentralController) authMiddleware(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if !cc.config.AuthEnabled {
			next.ServeHTTP(w, r)
			return
		}
		
		// Check Authorization header
		authHeader := r.Header.Get("Authorization")
		if authHeader == "" {
			http.Error(w, "Authorization required", http.StatusUnauthorized)
			return
		}
		
		// Extract bearer token
		const bearerPrefix = "Bearer "
		if !strings.HasPrefix(authHeader, bearerPrefix) {
			http.Error(w, "Invalid authorization format", http.StatusUnauthorized)
			return
		}
		
		token := authHeader[len(bearerPrefix):]
		
		// Check if token is valid
		validToken := false
		for _, authToken := range cc.config.AuthTokens {
			if token == authToken {
				validToken = true
				break
			}
		}
		
		if !validToken {
			http.Error(w, "Invalid token", http.StatusUnauthorized)
			return
		}
		
		next.ServeHTTP(w, r)
	})
}

// Helper functions

func validateCentralControllerConfig(config *CentralControllerConfig) error {
	if config.ListenAddress == "" {
		config.ListenAddress = "0.0.0.0"
	}
	
	if config.Port <= 0 {
		config.Port = 8080
	}
	
	if config.AgentTimeout <= 0 {
		config.AgentTimeout = 5 * time.Minute
	}
	
	if config.PingInterval <= 0 {
		config.PingInterval = 30 * time.Second
	}
	
	if config.HealthCheck.CheckInterval <= 0 {
		config.HealthCheck.CheckInterval = 1 * time.Minute
	}
	
	return nil
}

// Stub implementations for connection handlers (these would be fully implemented)
func (cc *CentralController) handleAgentConnection(conn *websocket.Conn, r *http.Request) error {
	// Implementation would handle agent registration and message processing
	cc.logger.Info("Agent connected", "remote_addr", r.RemoteAddr)
	return nil
}

func (cc *CentralController) handleGhostShipConnection(conn *websocket.Conn, r *http.Request) error {
	// Implementation would handle ghost ship registration and message processing  
	cc.logger.Info("Ghost ship connected", "remote_addr", r.RemoteAddr)
	return nil
}

func (cc *CentralController) checkConnections() {
	// Implementation would ping all connections and remove stale ones
}

func (cc *CentralController) performHealthChecks() {
	// Implementation would check health of all connected agents and ghost ships
}

// Stub implementations for remaining REST handlers
func (cc *CentralController) getAgent(w http.ResponseWriter, r *http.Request) {}
func (cc *CentralController) getAgentJobs(w http.ResponseWriter, r *http.Request) {}
func (cc *CentralController) assignJobToAgent(w http.ResponseWriter, r *http.Request) {}
func (cc *CentralController) cancelAgentJob(w http.ResponseWriter, r *http.Request) {}
func (cc *CentralController) getGhostShip(w http.ResponseWriter, r *http.Request) {}
func (cc *CentralController) getGhostShipJobs(w http.ResponseWriter, r *http.Request) {}
func (cc *CentralController) launchGhostShip(w http.ResponseWriter, r *http.Request) {}
func (cc *CentralController) stopGhostShip(w http.ResponseWriter, r *http.Request) {}