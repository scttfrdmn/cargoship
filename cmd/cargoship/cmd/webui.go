package cmd

import (
	"context"
	"fmt"
	"log/slog"
	"os"
	"os/signal"
	"syscall"

	"github.com/scttfrdmn/cargoship/pkg/controller"
	"github.com/spf13/cobra"
)

var webuiCmd = &cobra.Command{
	Use:   "webui",
	Short: "Start the web UI for managing connected agents",
	Long: `Start the CargoShip web interface for managing connected launch agents.
	
The web UI provides a dashboard for:
- Viewing connected agents and their status
- Monitoring active jobs and progress
- Managing agent connections
- Real-time updates via WebSocket

Example:
  cargoship webui --addr :8081 --auth-token your-secret-token`,
	RunE: runWebUI,
}

var (
	webUIAddr      string
	webUIAuthToken string
	webUITLS       bool
	webUITLSCert   string
	webUITLSKey    string
)

func init() {
	// This will be added to root command in main.go

	webuiCmd.Flags().StringVar(&webUIAddr, "addr", ":8081", "Address to bind the web server")
	webuiCmd.Flags().StringVar(&webUIAuthToken, "auth-token", "", "Authentication token for API access")
	webuiCmd.Flags().BoolVar(&webUITLS, "tls", false, "Enable TLS")
	webuiCmd.Flags().StringVar(&webUITLSCert, "tls-cert", "", "TLS certificate file")
	webuiCmd.Flags().StringVar(&webUITLSKey, "tls-key", "", "TLS private key file")

	_ = webuiCmd.MarkFlagRequired("auth-token")
}

func runWebUI(cmd *cobra.Command, args []string) error {
	logger := slog.New(slog.NewTextHandler(os.Stdout, &slog.HandlerOptions{
		Level: slog.LevelInfo,
	}))

	// Create controller configuration
	config := &controller.Config{
		ListenAddr:  webUIAddr,
		AuthToken:   webUIAuthToken,
		TLSEnabled:  webUITLS,
		TLSCertFile: webUITLSCert,
		TLSKeyFile:  webUITLSKey,
		LogLevel:    "info",
	}

	// Create controller (which includes web server)
	ctrl, err := controller.NewController(config, logger)
	if err != nil {
		return fmt.Errorf("failed to create controller: %w", err)
	}

	// Setup graceful shutdown
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	// Handle interrupt signals
	sigChan := make(chan os.Signal, 1)
	signal.Notify(sigChan, syscall.SIGINT, syscall.SIGTERM)

	// Start controller in background
	go func() {
		if err := ctrl.Start(); err != nil {
			logger.Error("Controller failed to start", "error", err)
			cancel()
		}
	}()

	logger.Info("CargoShip Web UI started",
		"addr", webUIAddr,
		"tls", webUITLS,
		"dashboard_url", func() string {
			scheme := "http"
			if webUITLS {
				scheme = "https"
			}
			return fmt.Sprintf("%s://localhost%s", scheme, webUIAddr)
		}())

	// Wait for shutdown signal
	select {
	case <-sigChan:
		logger.Info("Received shutdown signal")
	case <-ctx.Done():
		logger.Info("Context cancelled")
	}

	// Graceful shutdown
	logger.Info("Shutting down web UI...")
	if err := ctrl.Stop(); err != nil {
		logger.Error("Error during shutdown", "error", err)
		return err
	}

	logger.Info("Web UI shutdown complete")
	return nil
}
