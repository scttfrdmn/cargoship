package main

import (
	"context"
	"flag"
	"fmt"
	"log/slog"
	"os"
	"os/signal"
	"syscall"

	"github.com/scttfrdmn/cargoship/pkg/launch"
	"gopkg.in/yaml.v3"
)

func main() {
	var (
		configFile = flag.String("config", "/etc/cargoship/ghost_ship.yaml", "Configuration file path")
		logLevel   = flag.String("log-level", "info", "Log level (debug, info, warn, error)")
		version    = flag.Bool("version", false, "Show version and exit")
	)
	flag.Parse()

	if *version {
		fmt.Println("CargoShip Ghost Ship v0.3.0")
		return
	}

	// Setup logging
	var level slog.Level
	switch *logLevel {
	case "debug":
		level = slog.LevelDebug
	case "info":
		level = slog.LevelInfo
	case "warn":
		level = slog.LevelWarn
	case "error":
		level = slog.LevelError
	default:
		level = slog.LevelInfo
	}

	logger := slog.New(slog.NewJSONHandler(os.Stdout, &slog.HandlerOptions{
		Level: level,
	}))

	logger.Info("Starting CargoShip Ghost Ship",
		"config", *configFile,
		"log_level", *logLevel)

	// Load configuration
	config, err := loadGhostShipConfig(*configFile)
	if err != nil {
		logger.Error("Failed to load configuration", "error", err)
		os.Exit(1)
	}

	// Create ghost ship
	ghostShip, err := launch.NewGhostShip(config, logger)
	if err != nil {
		logger.Error("Failed to create ghost ship", "error", err)
		os.Exit(1)
	}

	// Setup signal handling
	ctx, cancel := signal.NotifyContext(context.Background(), syscall.SIGINT, syscall.SIGTERM)
	defer cancel()

	// Start ghost ship
	if err := ghostShip.Launch(); err != nil {
		logger.Error("Failed to launch ghost ship", "error", err)
		os.Exit(1)
	}

	logger.Info("👻 CargoShip Ghost Ship launched successfully - autonomous archival active")

	// Wait for shutdown signal
	<-ctx.Done()
	logger.Info("Shutdown signal received, stopping ghost ship...")

	// Stop ghost ship
	if err := ghostShip.Stop(); err != nil {
		logger.Error("Error stopping ghost ship", "error", err)
		os.Exit(1)
	}

	logger.Info("CargoShip Ghost Ship stopped gracefully")
}

func loadGhostShipConfig(filename string) (*launch.GhostShipConfig, error) {
	data, err := os.ReadFile(filename)
	if err != nil {
		return nil, fmt.Errorf("failed to read config file: %w", err)
	}

	var config launch.GhostShipConfig
	if err := yaml.Unmarshal(data, &config); err != nil {
		return nil, fmt.Errorf("failed to parse config file: %w", err)
	}

	return &config, nil
}