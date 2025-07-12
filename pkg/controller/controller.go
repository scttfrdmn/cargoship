package controller

import (
	"context"
	"crypto/tls"
	"fmt"
	"log/slog"
	"sync"
)

// Controller manages the overall controller service
type Controller struct {
	// Configuration
	config *Config
	logger *slog.Logger
	
	// Components
	registry  *AgentRegistry
	wsServer  *WebSocketServer
	
	// Lifecycle
	ctx    context.Context
	cancel context.CancelFunc
	wg     sync.WaitGroup
}

// Config holds controller configuration
type Config struct {
	// Server settings
	ListenAddr string `json:"listen_addr" yaml:"listen_addr"`
	AuthToken  string `json:"auth_token" yaml:"auth_token"`
	
	// TLS settings
	TLSEnabled  bool   `json:"tls_enabled" yaml:"tls_enabled"`
	TLSCertFile string `json:"tls_cert_file" yaml:"tls_cert_file"`
	TLSKeyFile  string `json:"tls_key_file" yaml:"tls_key_file"`
	
	// Logging
	LogLevel string `json:"log_level" yaml:"log_level"`
}

// NewController creates a new controller instance
func NewController(config *Config, logger *slog.Logger) (*Controller, error) {
	if config == nil {
		return nil, fmt.Errorf("controller config cannot be nil")
	}
	
	if err := validateConfig(config); err != nil {
		return nil, fmt.Errorf("invalid controller config: %w", err)
	}
	
	ctx, cancel := context.WithCancel(context.Background())
	
	controller := &Controller{
		config: config,
		logger: logger.With("component", "controller"),
		ctx:    ctx,
		cancel: cancel,
	}
	
	// Create agent registry
	controller.registry = NewAgentRegistry(config.AuthToken, logger)
	
	// Create WebSocket server
	controller.wsServer = NewWebSocketServer(config.ListenAddr, config.AuthToken, controller.registry, logger)
	
	// Configure TLS if enabled
	if config.TLSEnabled {
		tlsConfig, err := buildTLSConfig(config)
		if err != nil {
			cancel()
			return nil, fmt.Errorf("failed to configure TLS: %w", err)
		}
		controller.wsServer.tlsConfig = tlsConfig
	}
	
	controller.logger.Info("Controller created successfully", 
		"listen_addr", config.ListenAddr,
		"tls_enabled", config.TLSEnabled)
	
	return controller, nil
}

// Start starts the controller and all its components
func (c *Controller) Start() error {
	c.logger.Info("Starting CargoShip controller")
	
	// Start agent registry
	if err := c.registry.Start(); err != nil {
		return fmt.Errorf("failed to start agent registry: %w", err)
	}
	
	// Start WebSocket server
	if err := c.wsServer.Start(); err != nil {
		_ = c.registry.Stop()
		return fmt.Errorf("failed to start WebSocket server: %w", err)
	}
	
	c.logger.Info("Controller started successfully", 
		"listen_addr", c.config.ListenAddr,
		"protocol", func() string {
			if c.config.TLSEnabled {
				return "wss"
			}
			return "ws"
		}())
	
	return nil
}

// Stop gracefully stops the controller
func (c *Controller) Stop() error {
	c.logger.Info("Stopping CargoShip controller")
	
	// Cancel context
	c.cancel()
	
	// Stop WebSocket server
	if c.wsServer != nil {
		_ = c.wsServer.Stop()
	}
	
	// Stop agent registry
	if c.registry != nil {
		_ = c.registry.Stop()
	}
	
	// Wait for all goroutines
	c.wg.Wait()
	
	c.logger.Info("Controller stopped successfully")
	return nil
}

// GetRegistry returns the agent registry
func (c *Controller) GetRegistry() *AgentRegistry {
	return c.registry
}

// GetWebSocketServer returns the WebSocket server
func (c *Controller) GetWebSocketServer() *WebSocketServer {
	return c.wsServer
}

// validateConfig validates the controller configuration
func validateConfig(config *Config) error {
	if config.ListenAddr == "" {
		return fmt.Errorf("listen address is required")
	}
	
	if config.AuthToken == "" {
		return fmt.Errorf("auth token is required")
	}
	
	if config.TLSEnabled {
		if config.TLSCertFile == "" {
			return fmt.Errorf("TLS cert file is required when TLS is enabled")
		}
		if config.TLSKeyFile == "" {
			return fmt.Errorf("TLS key file is required when TLS is enabled")
		}
	}
	
	return nil
}

// buildTLSConfig creates a TLS configuration
func buildTLSConfig(config *Config) (*tls.Config, error) {
	if !config.TLSEnabled {
		return nil, nil
	}
	
	cert, err := tls.LoadX509KeyPair(config.TLSCertFile, config.TLSKeyFile)
	if err != nil {
		return nil, fmt.Errorf("failed to load TLS certificate: %w", err)
	}
	
	return &tls.Config{
		Certificates: []tls.Certificate{cert},
		MinVersion:   tls.VersionTLS12,
	}, nil
}