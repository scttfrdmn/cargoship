package cmd

import (
	"crypto/rand"
	"encoding/hex"
	"fmt"
	"log/slog"
	"os"
	"os/signal"
	"syscall"

	"github.com/spf13/cobra"
	"github.com/spf13/viper"

	"github.com/scttfrdmn/cargoship/pkg/controller"
)

// NewControllerCmd creates the controller command
func NewControllerCmd() *cobra.Command {
	cmd := &cobra.Command{
		Use:   "controller",
		Short: "Run CargoShip controller for launch agents",
		Long: `Run the CargoShip controller that manages launch agents.

The controller provides a WebSocket server that launch agents connect to for
secure communication, job assignment, and status reporting.

Examples:
  # Run controller on default port with generated auth token
  cargoship controller

  # Run controller on specific port
  cargoship controller --listen :8080

  # Run controller with specific auth token
  cargoship controller --auth-token your-secure-token

  # Run controller with TLS enabled
  cargoship controller --tls --cert-file server.crt --key-file server.key`,
		RunE: runController,
	}

	// Controller-specific flags
	cmd.Flags().String("listen", ":8080", "Address to listen on for agent connections")
	cmd.Flags().String("auth-token", "", "Authentication token for agent connections (auto-generated if not provided)")
	cmd.Flags().Bool("tls", false, "Enable TLS for secure connections")
	cmd.Flags().String("cert-file", "", "TLS certificate file (required with --tls)")
	cmd.Flags().String("key-file", "", "TLS private key file (required with --tls)")
	cmd.Flags().String("log-level", "info", "Log level (debug, info, warn, error)")

	// Bind flags to viper
	_ = viper.BindPFlag("controller.listen", cmd.Flags().Lookup("listen"))
	_ = viper.BindPFlag("controller.auth_token", cmd.Flags().Lookup("auth-token"))
	_ = viper.BindPFlag("controller.tls", cmd.Flags().Lookup("tls"))
	_ = viper.BindPFlag("controller.cert_file", cmd.Flags().Lookup("cert-file"))
	_ = viper.BindPFlag("controller.key_file", cmd.Flags().Lookup("key-file"))
	_ = viper.BindPFlag("controller.log_level", cmd.Flags().Lookup("log-level"))

	return cmd
}

// runController executes the controller command
func runController(cmd *cobra.Command, args []string) error {
	logger := slog.Default()

	// Build configuration
	config, err := buildControllerConfig()
	if err != nil {
		return fmt.Errorf("failed to build controller config: %w", err)
	}

	// Auto-generate auth token if not provided
	if config.AuthToken == "" {
		token, err := generateAuthToken()
		if err != nil {
			return fmt.Errorf("failed to generate auth token: %w", err)
		}
		config.AuthToken = token
		logger.Info("Generated auth token for agents", "auth_token", token)
	}

	// Validate TLS configuration
	if config.TLSEnabled {
		if config.TLSCertFile == "" || config.TLSKeyFile == "" {
			return fmt.Errorf("TLS certificate and key files are required when TLS is enabled")
		}
	}

	// Create controller
	ctrl, err := controller.NewController(config, logger)
	if err != nil {
		return fmt.Errorf("failed to create controller: %w", err)
	}

	// Start controller
	if err := ctrl.Start(); err != nil {
		return fmt.Errorf("failed to start controller: %w", err)
	}

	// Print connection information
	printControllerInfo(config, logger)

	// Wait for shutdown signal
	sigChan := make(chan os.Signal, 1)
	signal.Notify(sigChan, syscall.SIGINT, syscall.SIGTERM)

	logger.Info("Controller running, press Ctrl+C to stop")
	<-sigChan

	logger.Info("Shutdown signal received, stopping controller...")

	// Stop controller
	if err := ctrl.Stop(); err != nil {
		logger.Error("Error stopping controller", "error", err)
		return err
	}

	logger.Info("Controller stopped successfully")
	return nil
}

// buildControllerConfig builds the controller configuration from flags and config file
func buildControllerConfig() (*controller.Config, error) {
	config := &controller.Config{
		ListenAddr:  viper.GetString("controller.listen"),
		AuthToken:   viper.GetString("controller.auth_token"),
		TLSEnabled:  viper.GetBool("controller.tls"),
		TLSCertFile: viper.GetString("controller.cert_file"),
		TLSKeyFile:  viper.GetString("controller.key_file"),
		LogLevel:    viper.GetString("controller.log_level"),
	}

	return config, nil
}

// generateAuthToken generates a secure random authentication token
func generateAuthToken() (string, error) {
	// Generate 32 random bytes (256 bits)
	bytes := make([]byte, 32)
	if _, err := rand.Read(bytes); err != nil {
		return "", err
	}

	// Convert to hex string
	return hex.EncodeToString(bytes), nil
}

// printControllerInfo prints connection information for the controller
func printControllerInfo(config *controller.Config, logger *slog.Logger) {
	protocol := "ws"
	if config.TLSEnabled {
		protocol = "wss"
	}

	// Extract host and port from listen address
	addr := config.ListenAddr
	if addr[0] == ':' {
		addr = "localhost" + addr
	}

	connectionURL := fmt.Sprintf("%s://%s", protocol, addr)

	logger.Info("🚢 CargoShip Controller Ready for Launch Agents!")
	logger.Info("Connection Details:",
		"url", connectionURL,
		"auth_token", config.AuthToken,
		"tls_enabled", config.TLSEnabled)

	fmt.Printf(`
🚢 CargoShip Controller Started Successfully!

Connection Information for Launch Agents:
┌─────────────────────────────────────────────────────────────┐
│  Controller URL: %s  
│  Auth Token:     %s  
│  TLS Enabled:    %t                                         
└─────────────────────────────────────────────────────────────┘

API Endpoints:
  • Agent List:      GET  %s/api/v1/agents
  • Agent Details:   GET  %s/api/v1/agents/{id}
  • Health Check:    GET  %s/health

Configure your launch agents with:
  CARGOSHIP_CONTROLLER_URL=%s
  CARGOSHIP_AUTH_TOKEN=%s

Press Ctrl+C to stop the controller.
`,
		connectionURL,
		config.AuthToken,
		config.TLSEnabled,
		connectionURL,
		connectionURL,
		connectionURL,
		connectionURL,
		config.AuthToken,
	)
}
