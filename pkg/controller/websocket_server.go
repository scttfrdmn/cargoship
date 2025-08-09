package controller

import (
	"context"
	"crypto/tls"
	"encoding/json"
	"fmt"
	"log/slog"
	"net/http"
	"strings"
	"sync"
	"time"

	"github.com/gorilla/websocket"
	"github.com/scttfrdmn/cargoship/pkg/launch"
)

// WebSocketServer handles WebSocket connections from launch agents
type WebSocketServer struct {
	// Server configuration
	addr      string
	authToken string
	tlsConfig *tls.Config
	logger    *slog.Logger

	// Agent management
	registry    *AgentRegistry
	authManager *AuthManager

	// WebSocket upgrader
	upgrader websocket.Upgrader

	// HTTP server
	server *http.Server

	// Lifecycle
	ctx    context.Context
	cancel context.CancelFunc
	wg     sync.WaitGroup
}

// AgentConnection represents a WebSocket connection to an agent
type AgentConnection struct {
	// Connection details
	conn   *websocket.Conn
	agent  *ConnectedAgent
	logger *slog.Logger

	// Message handling
	sendCh  chan []byte
	closeCh chan struct{}
	onClose func()

	// State
	mu     sync.RWMutex
	closed bool
}

// NewWebSocketServer creates a new WebSocket server for agent connections
func NewWebSocketServer(addr, authToken string, registry *AgentRegistry, logger *slog.Logger) *WebSocketServer {
	ctx, cancel := context.WithCancel(context.Background())

	return &WebSocketServer{
		addr:      addr,
		authToken: authToken,
		registry:  registry,
		logger:    logger.With("component", "websocket-server"),
		upgrader: websocket.Upgrader{
			ReadBufferSize:  1024,
			WriteBufferSize: 1024,
			CheckOrigin: func(r *http.Request) bool {
				return true // Allow all origins for now
			},
		},
		ctx:    ctx,
		cancel: cancel,
	}
}

// SetAuthManager sets the authentication manager for the WebSocket server
func (ws *WebSocketServer) SetAuthManager(authManager *AuthManager) {
	ws.authManager = authManager
}

// Start starts the WebSocket server
func (ws *WebSocketServer) Start() error {
	ws.logger.Info("Starting WebSocket server", "addr", ws.addr)

	// Set up HTTP routes
	mux := http.NewServeMux()
	mux.HandleFunc("/api/v1/agents/connect", ws.handleAgentConnection)
	mux.HandleFunc("/api/v1/agents", ws.handleAgentList)
	mux.HandleFunc("/api/v1/agents/", ws.handleAgentOperations)
	mux.HandleFunc("/api/v1/auth/register", ws.handleAgentRegistration)
	mux.HandleFunc("/api/v1/auth/authenticate", ws.handleAgentAuthentication)
	mux.HandleFunc("/api/v1/auth/validate", ws.handleTokenValidation)
	mux.HandleFunc("/health", ws.handleHealth)

	// Create HTTP server
	ws.server = &http.Server{
		Addr:         ws.addr,
		Handler:      mux,
		TLSConfig:    ws.tlsConfig,
		ReadTimeout:  30 * time.Second,
		WriteTimeout: 30 * time.Second,
		IdleTimeout:  2 * time.Minute,
	}

	// Start server in background
	ws.wg.Add(1)
	go func() {
		defer ws.wg.Done()

		var err error
		if ws.tlsConfig != nil {
			err = ws.server.ListenAndServeTLS("", "")
		} else {
			err = ws.server.ListenAndServe()
		}

		if err != nil && err != http.ErrServerClosed {
			ws.logger.Error("WebSocket server error", "error", err)
		}
	}()

	ws.logger.Info("WebSocket server started successfully", "addr", ws.addr)
	return nil
}

// Stop gracefully stops the WebSocket server
func (ws *WebSocketServer) Stop() error {
	ws.logger.Info("Stopping WebSocket server")

	// Cancel context
	ws.cancel()

	// Shutdown HTTP server
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()

	if ws.server != nil {
		_ = ws.server.Shutdown(ctx)
	}

	// Wait for goroutines
	ws.wg.Wait()

	ws.logger.Info("WebSocket server stopped")
	return nil
}

// handleAgentConnection handles WebSocket connections from agents
func (ws *WebSocketServer) handleAgentConnection(w http.ResponseWriter, r *http.Request) {
	// Validate authentication
	authHeader := r.Header.Get("Authorization")
	var agentID, agentVersion string
	var authenticated bool

	// Try JWT authentication first, then fall back to legacy token
	if authHeader != "" && len(authHeader) > 7 && authHeader[:7] == "Bearer " {
		token := authHeader[7:]

		// Check if it's a JWT token (contains dots)
		if ws.authManager != nil && len(token) > 20 && strings.Contains(token, ".") {
			// Validate JWT token
			claims, err := ws.authManager.ValidateToken(token)
			if err == nil {
				authenticated = true
				agentID = claims.AgentID
				ws.logger.Info("Agent connecting with JWT authentication", "agent_id", agentID)
			}
		}

		// Fall back to legacy token authentication
		if !authenticated && token == ws.authToken {
			authenticated = true
			agentID = r.Header.Get("X-Agent-ID")
			agentVersion = r.Header.Get("X-Agent-Version")
			ws.logger.Info("Agent connecting with legacy token authentication", "agent_id", agentID)
		}
	}

	if !authenticated {
		ws.logger.Warn("Unauthorized agent connection attempt", "remote_addr", r.RemoteAddr)
		http.Error(w, "Unauthorized", http.StatusUnauthorized)
		return
	}

	// Get additional agent info from headers if not already set
	if agentID == "" {
		agentID = r.Header.Get("X-Agent-ID")
	}
	if agentVersion == "" {
		agentVersion = r.Header.Get("X-Agent-Version")
	}

	if agentID == "" {
		ws.logger.Warn("Agent connection missing agent ID", "remote_addr", r.RemoteAddr)
		http.Error(w, "Missing Agent ID", http.StatusBadRequest)
		return
	}

	// Upgrade to WebSocket
	conn, err := ws.upgrader.Upgrade(w, r, nil)
	if err != nil {
		ws.logger.Error("Failed to upgrade WebSocket connection", "error", err, "agent_id", agentID)
		return
	}

	ws.logger.Info("Agent WebSocket connection established",
		"agent_id", agentID,
		"version", agentVersion,
		"remote_addr", r.RemoteAddr)

	// Create agent connection
	agentConn := NewAgentConnection(conn, agentID, ws.logger)

	// Handle the connection
	go ws.handleAgentMessages(agentConn)
}

// handleAgentMessages handles incoming messages from an agent
func (ws *WebSocketServer) handleAgentMessages(agentConn *AgentConnection) {
	defer func() {
		_ = agentConn.Close()
	}()

	// Set up ping/pong for keepalive
	agentConn.conn.SetPongHandler(func(appData string) error {
		return agentConn.conn.SetReadDeadline(time.Now().Add(60 * time.Second))
	})

	// Start ping sender
	go agentConn.startPingSender(ws.ctx)

	// Start message sender
	go agentConn.startMessageSender(ws.ctx)

	for {
		select {
		case <-ws.ctx.Done():
			return
		default:
			// Set read deadline
			_ = agentConn.conn.SetReadDeadline(time.Now().Add(60 * time.Second))

			// Read message
			_, messageData, err := agentConn.conn.ReadMessage()
			if err != nil {
				if websocket.IsUnexpectedCloseError(err, websocket.CloseGoingAway, websocket.CloseAbnormalClosure) {
					ws.logger.Error("WebSocket read error", "error", err, "agent_id", agentConn.agent.ID)
				}
				return
			}

			// Parse message
			var msg launch.ControllerMessage
			if err := json.Unmarshal(messageData, &msg); err != nil {
				ws.logger.Error("Failed to parse agent message", "error", err, "agent_id", agentConn.agent.ID)
				continue
			}

			// Handle message
			if err := ws.handleAgentMessage(agentConn, &msg); err != nil {
				ws.logger.Error("Failed to handle agent message", "error", err, "agent_id", agentConn.agent.ID, "message_type", msg.Type)
			}
		}
	}
}

// handleAgentMessage handles a specific message from an agent
func (ws *WebSocketServer) handleAgentMessage(agentConn *AgentConnection, msg *launch.ControllerMessage) error {
	switch msg.Type {
	case launch.MsgTypeRegister:
		return ws.handleRegistration(agentConn, msg)
	case launch.MsgTypeHeartbeat:
		return ws.handleHeartbeat(agentConn, msg)
	case launch.MsgTypeStatusUpdate:
		return ws.handleStatusUpdate(agentConn, msg)
	case launch.MsgTypeJobProgress:
		return ws.handleJobProgress(agentConn, msg)
	case launch.MsgTypeJobComplete:
		return ws.handleJobComplete(agentConn, msg)
	case launch.MsgTypeJobFailed:
		return ws.handleJobFailed(agentConn, msg)
	default:
		ws.logger.Warn("Unknown message type from agent", "type", msg.Type, "agent_id", agentConn.agent.ID)
		return nil
	}
}

// handleRegistration handles agent registration
func (ws *WebSocketServer) handleRegistration(agentConn *AgentConnection, msg *launch.ControllerMessage) error {
	var req launch.RegistrationRequest
	if err := json.Unmarshal(msg.Data, &req); err != nil {
		return fmt.Errorf("failed to parse registration request: %w", err)
	}

	// Register agent with registry
	resp, err := ws.registry.RegisterAgent(agentConn, &req)
	if err != nil {
		return fmt.Errorf("failed to register agent: %w", err)
	}

	// Send response
	return agentConn.SendMessage(launch.MsgTypeRegistered, resp)
}

// handleHeartbeat handles agent heartbeat
func (ws *WebSocketServer) handleHeartbeat(agentConn *AgentConnection, msg *launch.ControllerMessage) error {
	if agentConn.agent == nil {
		return fmt.Errorf("agent not registered")
	}

	// Update last seen time
	agentConn.agent.LastSeen = time.Now()

	ws.logger.Debug("Received heartbeat from agent", "agent_id", agentConn.agent.ID)
	return nil
}

// handleStatusUpdate handles agent status updates
func (ws *WebSocketServer) handleStatusUpdate(agentConn *AgentConnection, msg *launch.ControllerMessage) error {
	if agentConn.agent == nil {
		return fmt.Errorf("agent not registered")
	}

	var status launch.StatusUpdate
	if err := json.Unmarshal(msg.Data, &status); err != nil {
		return fmt.Errorf("failed to parse status update: %w", err)
	}

	return ws.registry.UpdateAgentStatus(agentConn.agent.ID, &status)
}

// handleJobProgress handles job progress updates
func (ws *WebSocketServer) handleJobProgress(agentConn *AgentConnection, msg *launch.ControllerMessage) error {
	// TODO: Implement job progress tracking
	ws.logger.Debug("Received job progress from agent", "agent_id", agentConn.agent.ID)
	return nil
}

// handleJobComplete handles job completion notifications
func (ws *WebSocketServer) handleJobComplete(agentConn *AgentConnection, msg *launch.ControllerMessage) error {
	// TODO: Implement job completion handling
	ws.logger.Info("Job completed by agent", "agent_id", agentConn.agent.ID)
	return nil
}

// handleJobFailed handles job failure notifications
func (ws *WebSocketServer) handleJobFailed(agentConn *AgentConnection, msg *launch.ControllerMessage) error {
	// TODO: Implement job failure handling
	ws.logger.Error("Job failed on agent", "agent_id", agentConn.agent.ID)
	return nil
}

// handleAgentList handles HTTP requests for agent list
func (ws *WebSocketServer) handleAgentList(w http.ResponseWriter, r *http.Request) {
	if r.Method != "GET" {
		http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
		return
	}

	agents := ws.registry.GetAllAgents()

	w.Header().Set("Content-Type", "application/json")
	if err := json.NewEncoder(w).Encode(map[string]interface{}{
		"agents": agents,
		"total":  len(agents),
	}); err != nil {
		ws.logger.Error("Failed to encode agents response", "error", err)
		http.Error(w, "Internal server error", http.StatusInternalServerError)
	}
}

// handleAgentOperations handles HTTP requests for individual agent operations
func (ws *WebSocketServer) handleAgentOperations(w http.ResponseWriter, r *http.Request) {
	// Extract agent ID from path
	path := r.URL.Path
	if len(path) < len("/api/v1/agents/") {
		http.Error(w, "Invalid path", http.StatusBadRequest)
		return
	}

	agentID := path[len("/api/v1/agents/"):]
	if agentID == "" {
		http.Error(w, "Missing agent ID", http.StatusBadRequest)
		return
	}

	switch r.Method {
	case "GET":
		agent, exists := ws.registry.GetAgent(agentID)
		if !exists {
			http.Error(w, "Agent not found", http.StatusNotFound)
			return
		}

		w.Header().Set("Content-Type", "application/json")
		if err := json.NewEncoder(w).Encode(agent); err != nil {
			ws.logger.Error("Failed to encode agent response", "error", err)
			http.Error(w, "Internal server error", http.StatusInternalServerError)
		}

	default:
		http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
	}
}

// handleHealth handles health check requests
func (ws *WebSocketServer) handleHealth(w http.ResponseWriter, r *http.Request) {
	if r.Method != "GET" {
		http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
		return
	}

	agentCount := ws.registry.GetAgentCount()

	w.Header().Set("Content-Type", "application/json")
	if err := json.NewEncoder(w).Encode(map[string]interface{}{
		"status":      "healthy",
		"agent_count": agentCount,
		"timestamp":   time.Now().Format(time.RFC3339),
	}); err != nil {
		ws.logger.Error("Failed to encode health response", "error", err)
		http.Error(w, "Internal server error", http.StatusInternalServerError)
	}
}

// NewAgentConnection creates a new agent connection
func NewAgentConnection(conn *websocket.Conn, agentID string, logger *slog.Logger) *AgentConnection {
	return &AgentConnection{
		conn:    conn,
		logger:  logger.With("agent_id", agentID),
		sendCh:  make(chan []byte, 100),
		closeCh: make(chan struct{}),
	}
}

// SetAgent sets the agent for this connection
func (ac *AgentConnection) SetAgent(agent *ConnectedAgent) {
	ac.agent = agent
}

// OnClose sets a callback for when the connection closes
func (ac *AgentConnection) OnClose(callback func()) {
	ac.onClose = callback
}

// SendMessage sends a message to the agent
func (ac *AgentConnection) SendMessage(msgType launch.MessageType, data interface{}) error {
	ac.mu.RLock()
	defer ac.mu.RUnlock()

	if ac.closed {
		return fmt.Errorf("connection closed")
	}

	// Serialize data
	var jsonData json.RawMessage
	if data != nil {
		dataBytes, err := json.Marshal(data)
		if err != nil {
			return fmt.Errorf("failed to marshal message data: %w", err)
		}
		jsonData = dataBytes
	}

	// Create message
	msg := launch.ControllerMessage{
		Type:      msgType,
		ID:        generateMessageID(),
		Timestamp: time.Now(),
		AgentID:   ac.agent.ID,
		Data:      jsonData,
	}

	// Serialize message
	messageBytes, err := json.Marshal(msg)
	if err != nil {
		return fmt.Errorf("failed to marshal message: %w", err)
	}

	// Send via channel
	select {
	case ac.sendCh <- messageBytes:
		return nil
	default:
		return fmt.Errorf("send buffer full")
	}
}

// Close closes the agent connection
func (ac *AgentConnection) Close() error {
	ac.mu.Lock()
	defer ac.mu.Unlock()

	if ac.closed {
		return nil
	}

	ac.closed = true
	close(ac.closeCh)

	if ac.onClose != nil {
		ac.onClose()
	}

	return ac.conn.Close()
}

// startPingSender sends periodic ping messages
func (ac *AgentConnection) startPingSender(ctx context.Context) {
	ticker := time.NewTicker(30 * time.Second)
	defer ticker.Stop()

	for {
		select {
		case <-ctx.Done():
			return
		case <-ac.closeCh:
			return
		case <-ticker.C:
			if err := ac.conn.WriteMessage(websocket.PingMessage, nil); err != nil {
				ac.logger.Error("Failed to send ping", "error", err)
				return
			}
		}
	}
}

// startMessageSender handles outgoing messages
func (ac *AgentConnection) startMessageSender(ctx context.Context) {
	for {
		select {
		case <-ctx.Done():
			return
		case <-ac.closeCh:
			return
		case message := <-ac.sendCh:
			if err := ac.conn.WriteMessage(websocket.TextMessage, message); err != nil {
				ac.logger.Error("Failed to send message", "error", err)
				return
			}
		}
	}
}

// generateMessageID generates a unique message ID
func generateMessageID() string {
	return fmt.Sprintf("%d", time.Now().UnixNano())
}

// handleAgentRegistration handles agent registration requests
func (ws *WebSocketServer) handleAgentRegistration(w http.ResponseWriter, r *http.Request) {
	if r.Method != "POST" {
		http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
		return
	}

	if ws.authManager == nil {
		http.Error(w, "Authentication not configured", http.StatusServiceUnavailable)
		return
	}

	// Parse registration request
	var req struct {
		AgentID      string            `json:"agent_id"`
		AgentName    string            `json:"agent_name"`
		PublicKey    string            `json:"public_key,omitempty"`
		Role         string            `json:"role"`
		Capabilities []string          `json:"capabilities,omitempty"`
		Metadata     map[string]string `json:"metadata,omitempty"`
	}

	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		ws.logger.Error("Failed to decode registration request", "error", err)
		http.Error(w, "Invalid request body", http.StatusBadRequest)
		return
	}

	// Validate required fields
	if req.AgentID == "" || req.AgentName == "" || req.Role == "" {
		http.Error(w, "Missing required fields: agent_id, agent_name, role", http.StatusBadRequest)
		return
	}

	// Register agent with auth manager
	err := ws.authManager.RegisterAgent(req.AgentID, req.AgentName, req.PublicKey, req.Role, req.Capabilities, req.Metadata)
	if err != nil {
		ws.logger.Error("Failed to register agent", "error", err, "agent_id", req.AgentID)
		http.Error(w, fmt.Sprintf("Registration failed: %v", err), http.StatusBadRequest)
		return
	}

	ws.logger.Info("Agent registered successfully", "agent_id", req.AgentID, "name", req.AgentName, "role", req.Role)

	// Return success response
	w.Header().Set("Content-Type", "application/json")
	_ = json.NewEncoder(w).Encode(map[string]interface{}{
		"success":   true,
		"agent_id":  req.AgentID,
		"message":   "Agent registered successfully",
		"timestamp": time.Now().Format(time.RFC3339),
	})
}

// handleAgentAuthentication handles agent authentication requests
func (ws *WebSocketServer) handleAgentAuthentication(w http.ResponseWriter, r *http.Request) {
	if r.Method != "POST" {
		http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
		return
	}

	if ws.authManager == nil {
		http.Error(w, "Authentication not configured", http.StatusServiceUnavailable)
		return
	}

	// Parse authentication request
	var req struct {
		AgentID   string `json:"agent_id"`
		Signature string `json:"signature,omitempty"`
	}

	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		ws.logger.Error("Failed to decode authentication request", "error", err)
		http.Error(w, "Invalid request body", http.StatusBadRequest)
		return
	}

	// Validate required fields
	if req.AgentID == "" {
		http.Error(w, "Missing required field: agent_id", http.StatusBadRequest)
		return
	}

	// Authenticate agent
	token, err := ws.authManager.AuthenticateAgent(req.AgentID, req.Signature)
	if err != nil {
		ws.logger.Error("Authentication failed", "error", err, "agent_id", req.AgentID)
		http.Error(w, "Authentication failed", http.StatusUnauthorized)
		return
	}

	ws.logger.Info("Agent authenticated successfully", "agent_id", req.AgentID)

	// Return JWT token
	w.Header().Set("Content-Type", "application/json")
	_ = json.NewEncoder(w).Encode(map[string]interface{}{
		"success":   true,
		"token":     token,
		"agent_id":  req.AgentID,
		"timestamp": time.Now().Format(time.RFC3339),
	})
}

// handleTokenValidation handles token validation requests
func (ws *WebSocketServer) handleTokenValidation(w http.ResponseWriter, r *http.Request) {
	if r.Method != "POST" {
		http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
		return
	}

	if ws.authManager == nil {
		http.Error(w, "Authentication not configured", http.StatusServiceUnavailable)
		return
	}

	// Get token from Authorization header or request body
	var token string

	// Try Authorization header first
	authHeader := r.Header.Get("Authorization")
	if authHeader != "" && len(authHeader) > 7 && authHeader[:7] == "Bearer " {
		token = authHeader[7:]
	} else {
		// Try request body
		var req struct {
			Token string `json:"token"`
		}

		if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
			http.Error(w, "Invalid request: provide token in Authorization header or request body", http.StatusBadRequest)
			return
		}
		token = req.Token
	}

	if token == "" {
		http.Error(w, "Missing token", http.StatusBadRequest)
		return
	}

	// Validate token
	claims, err := ws.authManager.ValidateToken(token)
	if err != nil {
		ws.logger.Debug("Token validation failed", "error", err)
		http.Error(w, "Invalid token", http.StatusUnauthorized)
		return
	}

	// Get agent capabilities
	capabilities := ws.authManager.GetAgentCapabilities(claims)

	// Return validation result
	w.Header().Set("Content-Type", "application/json")
	_ = json.NewEncoder(w).Encode(map[string]interface{}{
		"valid":        true,
		"agent_id":     claims.AgentID,
		"agent_name":   claims.AgentName,
		"role":         claims.Role,
		"permissions":  claims.Permissions,
		"capabilities": capabilities,
		"session_id":   claims.SessionID,
		"expires_at":   claims.ExpiresAt.Format(time.RFC3339),
		"timestamp":    time.Now().Format(time.RFC3339),
	})
}
