package launch

import (
	"context"
	"crypto/tls"
	"crypto/x509"
	"encoding/json"
	"fmt"
	"log/slog"
	"net/http"
	"net/url"
	"os"
	"sync"
	"time"

	"github.com/gorilla/websocket"
)

// ControllerConnection manages the secure connection to the CargoShip controller
type ControllerConnection struct {
	config *AgentConfig
	logger *slog.Logger

	// WebSocket connection
	conn   *websocket.Conn
	connMu sync.RWMutex

	// Connection state
	connected bool
	lastPong  time.Time

	// Message handling
	messageCh chan []byte
	errorCh   chan error
}

// Message types for agent-controller communication
type MessageType string

const (
	// Agent to Controller messages
	MsgTypeRegister     MessageType = "register"
	MsgTypeHeartbeat    MessageType = "heartbeat"
	MsgTypeStatusUpdate MessageType = "status_update"
	MsgTypeJobProgress  MessageType = "job_progress"
	MsgTypeJobComplete  MessageType = "job_complete"
	MsgTypeJobFailed    MessageType = "job_failed"
	MsgTypeLogStream    MessageType = "log_stream"

	// Controller to Agent messages
	MsgTypeRegistered   MessageType = "registered"
	MsgTypeJobAssign    MessageType = "job_assign"
	MsgTypeJobCancel    MessageType = "job_cancel"
	MsgTypeConfigUpdate MessageType = "config_update"
	MsgTypeShutdown     MessageType = "shutdown"
	MsgTypePing         MessageType = "ping"
)

// ControllerMessage represents a message between agent and controller
type ControllerMessage struct {
	Type      MessageType     `json:"type"`
	ID        string          `json:"id,omitempty"`
	Timestamp time.Time       `json:"timestamp"`
	AgentID   string          `json:"agent_id"`
	Data      json.RawMessage `json:"data,omitempty"`
}

// RegistrationRequest is sent when agent registers with controller
type RegistrationRequest struct {
	AgentID      string            `json:"agent_id"`
	Name         string            `json:"name"`
	Description  string            `json:"description"`
	Version      string            `json:"version"`
	Capabilities []string          `json:"capabilities"`
	WatchPaths   []WatchPath       `json:"watch_paths"`
	Metadata     map[string]string `json:"metadata,omitempty"`
}

// RegistrationResponse is received after successful registration
type RegistrationResponse struct {
	Success       bool                   `json:"success"`
	AgentID       string                 `json:"agent_id"`
	SessionID     string                 `json:"session_id"`
	Configuration map[string]interface{} `json:"configuration,omitempty"`
	Message       string                 `json:"message,omitempty"`
}

// JobAssignment represents a job assigned by the controller
type JobAssignment struct {
	JobID        string                 `json:"job_id"`
	Type         string                 `json:"type"`
	Path         string                 `json:"path"`
	Destination  string                 `json:"destination"`
	StorageClass string                 `json:"storage_class"`
	Priority     int                    `json:"priority"`
	Parameters   map[string]interface{} `json:"parameters,omitempty"`
	Deadline     *time.Time             `json:"deadline,omitempty"`
}

// StatusUpdate represents agent status sent to controller
type StatusUpdate struct {
	State         AgentState    `json:"state"`
	ActiveJobs    int           `json:"active_jobs"`
	CompletedJobs int64         `json:"completed_jobs"`
	FailedJobs    int64         `json:"failed_jobs"`
	BytesArchived int64         `json:"bytes_archived"`
	Uptime        time.Duration `json:"uptime"`
	SystemInfo    SystemInfo    `json:"system_info"`
	LastError     string        `json:"last_error,omitempty"`
}

// SystemInfo contains system information about the agent
type SystemInfo struct {
	Platform     string  `json:"platform"`
	Architecture string  `json:"architecture"`
	CPUUsage     float64 `json:"cpu_usage"`
	MemoryUsage  float64 `json:"memory_usage"`
	DiskUsage    float64 `json:"disk_usage"`
	NetworkRx    int64   `json:"network_rx"`
	NetworkTx    int64   `json:"network_tx"`
}

// NewControllerConnection creates a new controller connection
func NewControllerConnection(config *AgentConfig, logger *slog.Logger) (*ControllerConnection, error) {
	return &ControllerConnection{
		config:    config,
		logger:    logger.With("component", "controller-connection"),
		messageCh: make(chan []byte, 100),
		errorCh:   make(chan error, 10),
	}, nil
}

// Connect establishes a secure WebSocket connection to the controller
func (cc *ControllerConnection) Connect(ctx context.Context) error {
	cc.logger.Info("Connecting to CargoShip controller", "url", cc.config.ControllerURL)

	// Parse controller URL
	u, err := url.Parse(cc.config.ControllerURL)
	if err != nil {
		return fmt.Errorf("invalid controller URL: %w", err)
	}

	// Convert HTTP(S) URL to WebSocket URL
	wsScheme := "ws"
	if u.Scheme == "https" {
		wsScheme = "wss"
	}

	wsURL := fmt.Sprintf("%s://%s/api/v1/agents/connect", wsScheme, u.Host)

	// Prepare WebSocket dialer with TLS configuration
	dialer := websocket.Dialer{
		HandshakeTimeout: 30 * time.Second,
		TLSClientConfig:  cc.buildTLSConfig(),
	}

	// Add authentication headers
	headers := http.Header{}
	headers.Set("Authorization", "Bearer "+cc.config.AuthToken)
	headers.Set("X-Agent-ID", cc.config.ID)
	headers.Set("X-Agent-Version", getAgentVersion())

	// Establish WebSocket connection
	conn, resp, err := dialer.DialContext(ctx, wsURL, headers)
	if err != nil {
		if resp != nil {
			return fmt.Errorf("failed to connect to controller (status: %d): %w", resp.StatusCode, err)
		}
		return fmt.Errorf("failed to connect to controller: %w", err)
	}

	// Store connection
	cc.connMu.Lock()
	cc.conn = conn
	cc.connected = true
	cc.lastPong = time.Now()
	cc.connMu.Unlock()

	cc.logger.Info("Successfully connected to controller")

	// Set up ping/pong handling for connection keepalive
	conn.SetPongHandler(func(appData string) error {
		cc.connMu.Lock()
		cc.lastPong = time.Now()
		cc.connMu.Unlock()
		return nil
	})

	// Register agent with controller
	if err := cc.registerAgent(); err != nil {
		_ = cc.Close()
		return fmt.Errorf("failed to register agent: %w", err)
	}

	return nil
}

// Close closes the controller connection
func (cc *ControllerConnection) Close() error {
	cc.connMu.Lock()
	defer cc.connMu.Unlock()

	if cc.conn != nil {
		cc.connected = false
		err := cc.conn.Close()
		cc.conn = nil
		return err
	}

	return nil
}

// SendMessage sends a message to the controller
func (cc *ControllerConnection) SendMessage(msgType MessageType, data interface{}) error {
	cc.connMu.RLock()
	conn := cc.conn
	connected := cc.connected
	cc.connMu.RUnlock()

	if !connected || conn == nil {
		return fmt.Errorf("not connected to controller")
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
	msg := ControllerMessage{
		Type:      msgType,
		ID:        generateMessageID(),
		Timestamp: time.Now(),
		AgentID:   cc.config.ID,
		Data:      jsonData,
	}

	// Send message
	if err := conn.WriteJSON(msg); err != nil {
		cc.logger.Error("Failed to send message to controller", "type", msgType, "error", err)
		return fmt.Errorf("failed to send message: %w", err)
	}

	cc.logger.Debug("Sent message to controller", "type", msgType, "message_id", msg.ID)
	return nil
}

// HandleMessages handles incoming messages from the controller
func (cc *ControllerConnection) HandleMessages(ctx context.Context, handler func([]byte) error) {
	cc.logger.Info("Starting message handler")

	// Start ping sender
	go cc.startPingSender(ctx)

	// Start connection monitor
	go cc.startConnectionMonitor(ctx)

	// Handle incoming messages
	for {
		select {
		case <-ctx.Done():
			cc.logger.Info("Message handler stopped")
			return
		default:
			cc.connMu.RLock()
			conn := cc.conn
			connected := cc.connected
			cc.connMu.RUnlock()

			if !connected || conn == nil {
				cc.logger.Warn("Connection lost, attempting to reconnect")
				return
			}

			// Set read deadline
			_ = conn.SetReadDeadline(time.Now().Add(60 * time.Second))

			// Read message
			_, message, err := conn.ReadMessage()
			if err != nil {
				cc.logger.Error("Failed to read message from controller", "error", err)
				cc.connMu.Lock()
				cc.connected = false
				cc.connMu.Unlock()
				return
			}

			// Handle message
			if err := handler(message); err != nil {
				cc.logger.Error("Failed to handle controller message", "error", err)
			}
		}
	}
}

// registerAgent registers the agent with the controller
func (cc *ControllerConnection) registerAgent() error {
	cc.logger.Info("Registering agent with controller")

	// Prepare registration request
	req := RegistrationRequest{
		AgentID:      cc.config.ID,
		Name:         cc.config.Name,
		Description:  cc.config.Description,
		Version:      getAgentVersion(),
		Capabilities: []string{"file_watching", "s3_upload", "compression", "encryption"},
		WatchPaths:   cc.config.WatchPaths,
		Metadata: map[string]string{
			"platform": "docker",
			"type":     "nas_agent",
		},
	}

	// Send registration message
	if err := cc.SendMessage(MsgTypeRegister, req); err != nil {
		return fmt.Errorf("failed to send registration: %w", err)
	}

	// Wait for registration response
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()

	for {
		select {
		case <-ctx.Done():
			return fmt.Errorf("registration timeout")
		default:
			cc.connMu.RLock()
			conn := cc.conn
			cc.connMu.RUnlock()

			if conn == nil {
				return fmt.Errorf("connection lost during registration")
			}

			_ = conn.SetReadDeadline(time.Now().Add(5 * time.Second))
			_, message, err := conn.ReadMessage()
			if err != nil {
				continue // Timeout, try again
			}

			// Parse message
			var msg ControllerMessage
			if err := json.Unmarshal(message, &msg); err != nil {
				cc.logger.Warn("Failed to parse controller message", "error", err)
				continue
			}

			if msg.Type == MsgTypeRegistered {
				var resp RegistrationResponse
				if err := json.Unmarshal(msg.Data, &resp); err != nil {
					return fmt.Errorf("failed to parse registration response: %w", err)
				}

				if !resp.Success {
					return fmt.Errorf("registration failed: %s", resp.Message)
				}

				cc.logger.Info("Successfully registered with controller", "session_id", resp.SessionID)
				return nil
			}
		}
	}
}

// startPingSender sends periodic ping messages to maintain connection
func (cc *ControllerConnection) startPingSender(ctx context.Context) {
	ticker := time.NewTicker(30 * time.Second)
	defer ticker.Stop()

	for {
		select {
		case <-ctx.Done():
			return
		case <-ticker.C:
			cc.connMu.RLock()
			conn := cc.conn
			connected := cc.connected
			cc.connMu.RUnlock()

			if connected && conn != nil {
				if err := conn.WriteMessage(websocket.PingMessage, nil); err != nil {
					cc.logger.Error("Failed to send ping", "error", err)
					cc.connMu.Lock()
					cc.connected = false
					cc.connMu.Unlock()
					return
				}
			}
		}
	}
}

// startConnectionMonitor monitors connection health
func (cc *ControllerConnection) startConnectionMonitor(ctx context.Context) {
	ticker := time.NewTicker(60 * time.Second)
	defer ticker.Stop()

	for {
		select {
		case <-ctx.Done():
			return
		case <-ticker.C:
			cc.connMu.RLock()
			lastPong := cc.lastPong
			connected := cc.connected
			cc.connMu.RUnlock()

			if connected && time.Since(lastPong) > 2*time.Minute {
				cc.logger.Warn("Connection appears to be stale, marking as disconnected")
				cc.connMu.Lock()
				cc.connected = false
				cc.connMu.Unlock()
				return
			}
		}
	}
}

// buildTLSConfig creates TLS configuration based on agent config
func (cc *ControllerConnection) buildTLSConfig() *tls.Config {
	if cc.config.TLSConfig == nil || !cc.config.TLSConfig.Enabled {
		return nil
	}

	tlsConfig := &tls.Config{
		InsecureSkipVerify: cc.config.TLSConfig.InsecureSkipVerify,
		MinVersion:         tls.VersionTLS12, // Enforce TLS 1.2 minimum
	}

	// Issue #141: Load client certificate if specified
	if cc.config.TLSConfig.CertFile != "" && cc.config.TLSConfig.KeyFile != "" {
		cert, err := tls.LoadX509KeyPair(cc.config.TLSConfig.CertFile, cc.config.TLSConfig.KeyFile)
		if err != nil {
			cc.logger.Error("Failed to load TLS certificate", "cert", cc.config.TLSConfig.CertFile, "key", cc.config.TLSConfig.KeyFile, "error", err)
			// Continue without client cert - server may not require it
		} else {
			tlsConfig.Certificates = []tls.Certificate{cert}
			cc.logger.Info("Loaded TLS client certificate", "cert", cc.config.TLSConfig.CertFile)
		}
	}

	// Issue #141: Load CA certificate bundle if specified
	if cc.config.TLSConfig.CAFile != "" {
		caCert, err := os.ReadFile(cc.config.TLSConfig.CAFile)
		if err != nil {
			cc.logger.Error("Failed to read CA certificate", "ca", cc.config.TLSConfig.CAFile, "error", err)
		} else {
			caCertPool := x509.NewCertPool()
			if !caCertPool.AppendCertsFromPEM(caCert) {
				cc.logger.Error("Failed to parse CA certificate", "ca", cc.config.TLSConfig.CAFile)
			} else {
				tlsConfig.RootCAs = caCertPool
				cc.logger.Info("Loaded CA certificate bundle", "ca", cc.config.TLSConfig.CAFile)
			}
		}
	}

	return tlsConfig
}

// generateMessageID generates a unique message ID
func generateMessageID() string {
	return fmt.Sprintf("%d", time.Now().UnixNano())
}
