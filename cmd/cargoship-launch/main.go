// CargoShip Launch Agent - Headless agent for NAS deployment
package main

import (
	"context"
	"flag"
	"fmt"
	"log/slog"
	"os"
	"os/signal"
	"strings"
	"syscall"
	"time"

	"gopkg.in/yaml.v3"

	"github.com/scttfrdmn/cargoship/pkg/launch"
)

const (
	defaultConfigPath = "/config/agent.yaml"
	defaultLogLevel   = "info"
)

var (
	configPath = flag.String("config", defaultConfigPath, "Path to agent configuration file")
	logLevel   = flag.String("log-level", defaultLogLevel, "Log level (debug, info, warn, error)")
	validate   = flag.Bool("validate", false, "Validate configuration and exit")
	version    = flag.Bool("version", false, "Show version and exit")
)

func main() {
	flag.Parse()

	// Show version
	if *version {
		fmt.Printf("CargoShip Launch Agent v%s\n", getVersion())
		os.Exit(0)
	}

	// Setup logging
	logger := setupLogging(*logLevel)
	logger.Info("Starting CargoShip Launch Agent", "version", getVersion())

	// Load configuration
	config, err := loadConfig(*configPath)
	if err != nil {
		logger.Error("Failed to load configuration", "error", err)
		os.Exit(1)
	}

	// Validate configuration
	if *validate {
		logger.Info("Configuration is valid")
		os.Exit(0)
	}

	// Create and start agent
	agent, err := launch.NewAgent(config, logger)
	if err != nil {
		logger.Error("Failed to create agent", "error", err)
		os.Exit(1)
	}

	// Start agent
	if err := agent.Start(); err != nil {
		logger.Error("Failed to start agent", "error", err)
		os.Exit(1)
	}

	logger.Info("CargoShip Launch Agent started successfully", "agent_id", config.ID)

	// Wait for shutdown signal
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	sigChan := make(chan os.Signal, 1)
	signal.Notify(sigChan, syscall.SIGINT, syscall.SIGTERM)

	select {
	case sig := <-sigChan:
		logger.Info("Received shutdown signal", "signal", sig)
	case <-ctx.Done():
		logger.Info("Context cancelled")
	}

	// Graceful shutdown
	logger.Info("Shutting down agent...")
	if err := agent.Stop(); err != nil {
		logger.Error("Error during shutdown", "error", err)
		os.Exit(1)
	}

	logger.Info("CargoShip Launch Agent stopped")
}

// loadConfig loads the agent configuration from file or environment
func loadConfig(configPath string) (*launch.AgentConfig, error) {
	config := &launch.AgentConfig{}

	// Try to load from file first
	if _, err := os.Stat(configPath); err == nil {
		data, err := os.ReadFile(configPath)
		if err != nil {
			return nil, fmt.Errorf("failed to read config file: %w", err)
		}

		if err := yaml.Unmarshal(data, config); err != nil {
			return nil, fmt.Errorf("failed to parse config file: %w", err)
		}
	}

	// Override with environment variables
	if err := loadConfigFromEnv(config); err != nil {
		return nil, fmt.Errorf("failed to load config from environment: %w", err)
	}

	// Set defaults
	setConfigDefaults(config)

	// Validate
	if err := validateConfig(config); err != nil {
		return nil, fmt.Errorf("invalid configuration: %w", err)
	}

	return config, nil
}

// loadConfigFromEnv loads configuration from environment variables
func loadConfigFromEnv(config *launch.AgentConfig) error {
	// Agent identification
	if id := os.Getenv("CARGOSHIP_AGENT_ID"); id != "" {
		config.ID = id
	}
	if name := os.Getenv("CARGOSHIP_AGENT_NAME"); name != "" {
		config.Name = name
	}
	if desc := os.Getenv("CARGOSHIP_AGENT_DESCRIPTION"); desc != "" {
		config.Description = desc
	}

	// Controller connection
	if url := os.Getenv("CARGOSHIP_CONTROLLER_URL"); url != "" {
		config.ControllerURL = url
	}
	if token := os.Getenv("CARGOSHIP_AUTH_TOKEN"); token != "" {
		config.AuthToken = token
	}

	// Watch paths
	if paths := os.Getenv("CARGOSHIP_WATCH_PATHS"); paths != "" {
		pathList := strings.Split(paths, ",")
		config.WatchPaths = make([]launch.WatchPath, len(pathList))
		for i, path := range pathList {
			config.WatchPaths[i] = launch.WatchPath{
				Path:      strings.TrimSpace(path),
				Recursive: true,
				MinAge:    7 * 24 * time.Hour, // 7 days default
			}
		}
	}

	// Archive settings
	if dest := os.Getenv("CARGOSHIP_DESTINATION"); dest != "" {
		config.Archive.Destination = dest
	}
	if class := os.Getenv("CARGOSHIP_STORAGE_CLASS"); class != "" {
		config.Archive.StorageClass = class
	}
	if comp := os.Getenv("CARGOSHIP_COMPRESSION"); comp != "" {
		config.Archive.Compression = comp
	}

	// File patterns
	if patterns := os.Getenv("CARGOSHIP_PATTERNS"); patterns != "" {
		includePatterns := strings.Split(patterns, ",")
		for i := range config.WatchPaths {
			config.WatchPaths[i].IncludePatterns = includePatterns
		}
	}
	if excludePatterns := os.Getenv("CARGOSHIP_EXCLUDE_PATTERNS"); excludePatterns != "" {
		excludeList := strings.Split(excludePatterns, ",")
		for i := range config.WatchPaths {
			config.WatchPaths[i].ExcludePatterns = excludeList
		}
	}

	// Timing configuration
	if interval := os.Getenv("CARGOSHIP_CHECK_INTERVAL"); interval != "" {
		if duration, err := time.ParseDuration(interval + "s"); err == nil {
			config.ScanInterval = duration
		}
	}
	if minAge := os.Getenv("CARGOSHIP_MIN_AGE_DAYS"); minAge != "" {
		if days, err := time.ParseDuration(minAge + "d"); err == nil {
			for i := range config.WatchPaths {
				config.WatchPaths[i].MinAge = days
			}
		}
	}

	return nil
}

// setConfigDefaults sets default values for configuration
func setConfigDefaults(config *launch.AgentConfig) {
	// Generate agent ID if not set
	if config.ID == "" {
		hostname, _ := os.Hostname()
		config.ID = fmt.Sprintf("agent-%s-%d", hostname, time.Now().Unix())
	}

	// Set default name
	if config.Name == "" {
		config.Name = fmt.Sprintf("CargoShip Agent (%s)", config.ID)
	}

	// Set default scan interval
	if config.ScanInterval == 0 {
		config.ScanInterval = 5 * time.Minute
	}

	// Set default archive configuration
	if config.Archive.StorageClass == "" {
		config.Archive.StorageClass = "deep-archive"
	}
	if config.Archive.Compression == "" {
		config.Archive.Compression = "zstd"
	}
	if config.Archive.MaxConcurrent == 0 {
		config.Archive.MaxConcurrent = 2
	}
	if config.Archive.RetryAttempts == 0 {
		config.Archive.RetryAttempts = 3
	}
	if config.Archive.RetryDelay == 0 {
		config.Archive.RetryDelay = 30 * time.Second
	}

	// Set default health check configuration
	if !config.HealthCheck.Enabled {
		config.HealthCheck.Enabled = true
		config.HealthCheck.CheckInterval = 30 * time.Second
		config.HealthCheck.ReportInterval = 5 * time.Minute
	}

	// Set default TLS configuration
	if config.TLSConfig == nil {
		config.TLSConfig = &launch.TLSConfig{
			Enabled: true,
		}
	}

	// Set default patterns for watch paths
	for i := range config.WatchPaths {
		if len(config.WatchPaths[i].IncludePatterns) == 0 {
			config.WatchPaths[i].IncludePatterns = []string{"*"}
		}
		if len(config.WatchPaths[i].ExcludePatterns) == 0 {
			config.WatchPaths[i].ExcludePatterns = []string{
				"*.tmp", "*.lock", ".DS_Store", "*.partial",
				"Thumbs.db", "desktop.ini", "._*",
			}
		}
		if config.WatchPaths[i].MinAge == 0 {
			config.WatchPaths[i].MinAge = 7 * 24 * time.Hour // 7 days
		}
		if config.WatchPaths[i].StorageClass == "" {
			config.WatchPaths[i].StorageClass = config.Archive.StorageClass
		}
	}
}

// validateConfig validates the configuration
func validateConfig(config *launch.AgentConfig) error {
	if config.ID == "" {
		return fmt.Errorf("agent ID is required")
	}

	if config.ControllerURL == "" {
		return fmt.Errorf("controller URL is required")
	}

	if config.AuthToken == "" {
		return fmt.Errorf("auth token is required")
	}

	if len(config.WatchPaths) == 0 {
		return fmt.Errorf("at least one watch path is required")
	}

	if config.Archive.Destination == "" {
		return fmt.Errorf("archive destination is required")
	}

	// Validate watch paths exist
	for _, watchPath := range config.WatchPaths {
		if _, err := os.Stat(watchPath.Path); os.IsNotExist(err) {
			return fmt.Errorf("watch path does not exist: %s", watchPath.Path)
		}
	}

	return nil
}

// setupLogging configures the logger
func setupLogging(level string) *slog.Logger {
	var logLevel slog.Level
	switch strings.ToLower(level) {
	case "debug":
		logLevel = slog.LevelDebug
	case "info":
		logLevel = slog.LevelInfo
	case "warn":
		logLevel = slog.LevelWarn
	case "error":
		logLevel = slog.LevelError
	default:
		logLevel = slog.LevelInfo
	}

	opts := &slog.HandlerOptions{
		Level: logLevel,
		ReplaceAttr: func(groups []string, a slog.Attr) slog.Attr {
			// Add timestamp formatting
			if a.Key == slog.TimeKey {
				a.Value = slog.StringValue(a.Value.Time().Format(time.RFC3339))
			}
			return a
		},
	}

	return slog.New(slog.NewJSONHandler(os.Stdout, opts))
}

// getVersion returns the current version
func getVersion() string {
	return "0.3.0"
}
