package cmd

import (
	"context"
	"encoding/json"
	"fmt"
	"log/slog"
	"os"
	"os/exec"
	"path/filepath"

	"github.com/aws/aws-sdk-go-v2/aws"
	awsconfig "github.com/aws/aws-sdk-go-v2/config"
	"github.com/aws/aws-sdk-go-v2/service/s3"
	"github.com/aws/aws-sdk-go-v2/service/sts"
	"github.com/dustin/go-humanize"
	"github.com/scttfrdmn/cargoship/pkg/config"
	"github.com/spf13/cobra"
)

var (
	configFile             string
	configGenerate         bool
	configEdit             bool
	configValidate         bool
	configValidateDetailed bool
	configShow             bool
	configFormat           string
)

// NewConfigCmd creates the config management command
func NewConfigCmd() *cobra.Command {
	cmd := &cobra.Command{
		Use:   "config",
		Short: "Manage CargoShip configuration",
		Long: `Manage CargoShip configuration files and settings.
		
CargoShip uses YAML configuration files to store settings for AWS, storage,
upload optimization, metrics, logging, and security. Configuration can be
loaded from multiple sources with the following precedence:

1. Command line flags (highest priority)
2. Environment variables (CARGOSHIP_*)
3. Configuration file
4. Built-in defaults (lowest priority)

Configuration file locations (searched in order):
- ~/.cargoship.yaml
- ~/.config/cargoship/.cargoship.yaml
- ./.cargoship.yaml

Examples:
  # Generate example configuration file
  cargoship config --generate
  
  # Show current configuration
  cargoship config --show
  
  # Validate configuration file
  cargoship config --validate --file ~/.cargoship.yaml
  
  # Show configuration in JSON format
  cargoship config --show --format json`,
		RunE: runConfig,
	}

	cmd.Flags().StringVar(&configFile, "file", "", "Configuration file path")
	cmd.Flags().BoolVar(&configGenerate, "generate", false, "Generate example configuration file")
	cmd.Flags().BoolVar(&configEdit, "edit", false, "Edit configuration file with default editor")
	cmd.Flags().BoolVar(&configValidate, "validate", false, "Validate configuration file")
	cmd.Flags().BoolVar(&configValidateDetailed, "validate-detailed", false, "Validate configuration with AWS connectivity and bucket access checks")
	cmd.Flags().BoolVar(&configShow, "show", false, "Show current configuration")
	cmd.Flags().StringVar(&configFormat, "format", "yaml", "Output format (yaml, json)")

	return cmd
}

func runConfig(cmd *cobra.Command, args []string) error {
	manager := config.NewManager()

	// Handle generate flag
	if configGenerate {
		return generateConfig()
	}

	// Load configuration if file is specified or for validation/show
	if configFile != "" || configValidate || configShow {
		if err := manager.LoadConfig(configFile); err != nil {
			return fmt.Errorf("failed to load config: %w", err)
		}
	}

	// Handle validate flags
	if configValidate || configValidateDetailed {
		return validateConfig(manager, configValidateDetailed)
	}

	// Handle show flag
	if configShow {
		return showConfig(manager)
	}

	// Handle edit flag
	if configEdit {
		return editConfig()
	}

	// Show help if no flags specified
	return cmd.Help()
}

func generateConfig() error {
	example := config.GenerateExampleConfig()

	fmt.Printf("# CargoShip Configuration Example\n")
	fmt.Printf("# Save this to ~/.cargoship.yaml to use as your configuration\n\n")
	fmt.Print(example)

	// Optionally save to file
	fmt.Printf("\n# To save this configuration:\n")
	fmt.Printf("# cargoship config --generate > ~/.cargoship.yaml\n")

	return nil
}

func validateConfig(manager *config.Manager, detailed bool) error {
	slog.Debug("Starting configuration validation", "detailed", detailed)

	cfg := manager.GetConfig()

	fmt.Println("╔══════════════════════════════════════════════════════════════╗")
	fmt.Println("║             Configuration Validation                         ║")
	fmt.Println("╚══════════════════════════════════════════════════════════════╝")
	fmt.Println()

	// Track validation errors and warnings
	var errors []string
	var warnings []string

	// Validate AWS Configuration
	fmt.Println("AWS Configuration:")
	if cfg.AWS.Region == "" {
		errors = append(errors, "AWS region is not set")
		fmt.Println("  ❌ Region: Not set")
	} else {
		fmt.Printf("  ✅ Region: %s\n", cfg.AWS.Region)
	}

	if cfg.AWS.Profile != "" {
		fmt.Printf("  ℹ️  Profile: %s\n", cfg.AWS.Profile)
	}

	// Detailed validation: check AWS credentials
	if detailed {
		fmt.Print("  Testing AWS credentials... ")
		if err := verifyAWSConfig(cfg.AWS.Region, cfg.AWS.Profile); err != nil {
			errors = append(errors, fmt.Sprintf("AWS credentials validation failed: %v", err))
			fmt.Println("❌")
		} else {
			fmt.Println("✅")
		}
	}
	fmt.Println()

	// Validate Storage Configuration
	fmt.Println("Storage Configuration:")
	if cfg.Storage.DefaultBucket != "" {
		fmt.Printf("  ✅ Default Bucket: %s\n", cfg.Storage.DefaultBucket)

		// Detailed validation: check bucket access
		if detailed {
			fmt.Print("  Testing bucket access... ")
			if err := verifyS3BucketAccess(cfg.AWS.Region, cfg.AWS.Profile, cfg.Storage.DefaultBucket); err != nil {
				errors = append(errors, fmt.Sprintf("S3 bucket access validation failed: %v", err))
				fmt.Println("❌")
			} else {
				fmt.Println("✅")
			}
		}
	} else {
		warnings = append(warnings, "No default bucket configured")
		fmt.Println("  ⚠️  Default Bucket: Not configured")
	}

	validStorageClasses := []string{"STANDARD", "REDUCED_REDUNDANCY", "STANDARD_IA", "ONEZONE_IA",
		"INTELLIGENT_TIERING", "GLACIER", "DEEP_ARCHIVE", "GLACIER_IR"}
	storageClassValid := false
	for _, sc := range validStorageClasses {
		if cfg.Storage.DefaultStorageClass == sc {
			storageClassValid = true
			break
		}
	}
	if !storageClassValid && cfg.Storage.DefaultStorageClass != "" {
		errors = append(errors, fmt.Sprintf("Invalid storage class: %s", cfg.Storage.DefaultStorageClass))
		fmt.Printf("  ❌ Storage Class: %s (invalid)\n", cfg.Storage.DefaultStorageClass)
	} else if cfg.Storage.DefaultStorageClass != "" {
		fmt.Printf("  ✅ Storage Class: %s\n", cfg.Storage.DefaultStorageClass)
	}

	fmt.Printf("  ✅ SSE Encryption: %t\n", cfg.Storage.SSEEncryption)
	fmt.Println()

	// Validate Upload Configuration
	fmt.Println("Upload Configuration:")
	if cfg.Upload.MaxConcurrency <= 0 {
		errors = append(errors, "Max concurrency must be positive")
		fmt.Printf("  ❌ Max Concurrency: %d (invalid)\n", cfg.Upload.MaxConcurrency)
	} else if cfg.Upload.MaxConcurrency > 100 {
		warnings = append(warnings, fmt.Sprintf("Max concurrency is very high (%d), may cause resource issues", cfg.Upload.MaxConcurrency))
		fmt.Printf("  ⚠️  Max Concurrency: %d (high)\n", cfg.Upload.MaxConcurrency)
	} else {
		fmt.Printf("  ✅ Max Concurrency: %d\n", cfg.Upload.MaxConcurrency)
	}

	// Validate chunk size
	chunkSize := cfg.Upload.ChunkSize
	if chunkSize != "" {
		bytes, err := humanize.ParseBytes(chunkSize)
		if err != nil {
			errors = append(errors, fmt.Sprintf("Invalid chunk size: %s", chunkSize))
			fmt.Printf("  ❌ Chunk Size: %s (invalid)\n", chunkSize)
		} else if bytes < 5*1024*1024 {
			warnings = append(warnings, "Chunk size below S3 minimum (5MB) for multipart uploads")
			fmt.Printf("  ⚠️  Chunk Size: %s (below 5MB minimum)\n", chunkSize)
		} else {
			fmt.Printf("  ✅ Chunk Size: %s\n", chunkSize)
		}
	} else {
		fmt.Println("  ⚠️  Chunk Size: Not set (will use default)")
	}

	if cfg.Upload.CompressionType != "" {
		validCompressions := []string{"gzip", "zstd", "none", "lz4", "brotli"}
		compressionValid := false
		for _, comp := range validCompressions {
			if cfg.Upload.CompressionType == comp {
				compressionValid = true
				break
			}
		}
		if !compressionValid {
			errors = append(errors, fmt.Sprintf("Invalid compression type: %s", cfg.Upload.CompressionType))
			fmt.Printf("  ❌ Compression: %s (invalid)\n", cfg.Upload.CompressionType)
		} else {
			fmt.Printf("  ✅ Compression: %s\n", cfg.Upload.CompressionType)
		}
	}

	fmt.Printf("  ✅ Adaptive Sizing: %t\n", cfg.Upload.EnableAdaptiveSizing)
	fmt.Println()

	// Validate Metrics Configuration
	fmt.Println("Metrics Configuration:")
	fmt.Printf("  ✅ Enabled: %t\n", cfg.Metrics.Enabled)
	if cfg.Metrics.Enabled {
		if cfg.Metrics.Namespace == "" {
			warnings = append(warnings, "Metrics enabled but namespace not set")
			fmt.Println("  ⚠️  Namespace: Not set")
		} else {
			fmt.Printf("  ✅ Namespace: %s\n", cfg.Metrics.Namespace)
		}
	}
	fmt.Println()

	// Validate Logging Configuration
	fmt.Println("Logging Configuration:")
	validLogLevels := []string{"debug", "info", "warn", "error"}
	logLevelValid := false
	for _, level := range validLogLevels {
		if cfg.Logging.Level == level {
			logLevelValid = true
			break
		}
	}
	if !logLevelValid && cfg.Logging.Level != "" {
		errors = append(errors, fmt.Sprintf("Invalid log level: %s", cfg.Logging.Level))
		fmt.Printf("  ❌ Level: %s (invalid)\n", cfg.Logging.Level)
	} else if cfg.Logging.Level != "" {
		fmt.Printf("  ✅ Level: %s\n", cfg.Logging.Level)
	} else {
		fmt.Println("  ℹ️  Level: Not set (will use default)")
	}
	fmt.Println()

	// Validate CargoHold Configuration (Issue #101)
	fmt.Println("CargoHold Configuration:")
	fmt.Printf("  ✅ Enabled: %t\n", cfg.CargoHold.Enable)

	if cfg.CargoHold.ShardCount < 1 || cfg.CargoHold.ShardCount > 100 {
		errors = append(errors, fmt.Sprintf("Invalid shard count: %d (must be 1-100)", cfg.CargoHold.ShardCount))
		fmt.Printf("  ❌ Shard Count: %d (invalid)\n", cfg.CargoHold.ShardCount)
	} else {
		fmt.Printf("  ✅ Shard Count: %d\n", cfg.CargoHold.ShardCount)
	}

	validShardStrategies := []string{"hash", "size", "type", "directory"}
	shardStrategyValid := false
	for _, strategy := range validShardStrategies {
		if cfg.CargoHold.ShardStrategy == strategy {
			shardStrategyValid = true
			break
		}
	}
	if !shardStrategyValid {
		errors = append(errors, fmt.Sprintf("Invalid shard strategy: %s", cfg.CargoHold.ShardStrategy))
		fmt.Printf("  ❌ Shard Strategy: %s (invalid)\n", cfg.CargoHold.ShardStrategy)
	} else {
		fmt.Printf("  ✅ Shard Strategy: %s\n", cfg.CargoHold.ShardStrategy)
	}

	if cfg.CargoHold.CompressionLevel < 1 || cfg.CargoHold.CompressionLevel > 22 {
		errors = append(errors, fmt.Sprintf("Invalid compression level: %d (must be 1-22)", cfg.CargoHold.CompressionLevel))
		fmt.Printf("  ❌ Compression Level: %d (invalid)\n", cfg.CargoHold.CompressionLevel)
	} else {
		fmt.Printf("  ✅ Compression Level: %d\n", cfg.CargoHold.CompressionLevel)
	}
	fmt.Println()

	// Summary
	fmt.Println("━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━")
	fmt.Println("Validation Summary:")
	fmt.Printf("  Errors: %d\n", len(errors))
	fmt.Printf("  Warnings: %d\n", len(warnings))
	fmt.Println("━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━")
	fmt.Println()

	// Print errors
	if len(errors) > 0 {
		fmt.Println("❌ Errors:")
		for _, err := range errors {
			fmt.Printf("   • %s\n", err)
		}
		fmt.Println()
		return fmt.Errorf("configuration validation failed with %d error(s)", len(errors))
	}

	// Print warnings
	if len(warnings) > 0 {
		fmt.Println("⚠️  Warnings:")
		for _, warn := range warnings {
			fmt.Printf("   • %s\n", warn)
		}
		fmt.Println()
	}

	fmt.Println("✅ Configuration is valid!")
	if Verbose {
		slog.Info("Configuration validation completed successfully",
			"errors", len(errors),
			"warnings", len(warnings),
			"detailed", detailed)
	}

	return nil
}

func verifyAWSConfig(region, profile string) error {
	ctx := context.Background()

	slog.Debug("Verifying AWS configuration", "region", region, "profile", profile)

	var opts []func(*awsconfig.LoadOptions) error
	if region != "" {
		opts = append(opts, awsconfig.WithRegion(region))
	}
	if profile != "" {
		opts = append(opts, awsconfig.WithSharedConfigProfile(profile))
	}

	cfg, err := awsconfig.LoadDefaultConfig(ctx, opts...)
	if err != nil {
		return fmt.Errorf("failed to load AWS config: %w", err)
	}

	// Verify credentials by calling STS
	stsClient := sts.NewFromConfig(cfg)
	_, err = stsClient.GetCallerIdentity(ctx, &sts.GetCallerIdentityInput{})
	if err != nil {
		return fmt.Errorf("failed to verify credentials: %w", err)
	}

	return nil
}

func verifyS3BucketAccess(region, profile, bucket string) error {
	ctx := context.Background()

	slog.Debug("Verifying S3 bucket access", "bucket", bucket, "region", region)

	var opts []func(*awsconfig.LoadOptions) error
	if region != "" {
		opts = append(opts, awsconfig.WithRegion(region))
	}
	if profile != "" {
		opts = append(opts, awsconfig.WithSharedConfigProfile(profile))
	}

	cfg, err := awsconfig.LoadDefaultConfig(ctx, opts...)
	if err != nil {
		return fmt.Errorf("failed to load AWS config: %w", err)
	}

	// Try to head the bucket
	s3Client := s3.NewFromConfig(cfg)
	_, err = s3Client.HeadBucket(ctx, &s3.HeadBucketInput{
		Bucket: aws.String(bucket),
	})
	if err != nil {
		return fmt.Errorf("failed to access bucket: %w", err)
	}

	return nil
}

func showConfig(manager *config.Manager) error {
	cfg := manager.GetConfig()

	switch configFormat {
	case "json":
		encoder := json.NewEncoder(os.Stdout)
		encoder.SetIndent("", "  ")
		return encoder.Encode(cfg)
	case "yaml", "yml":
		if err := manager.SaveConfig(""); err != nil {
			return fmt.Errorf("failed to format config as YAML: %w", err)
		}
		// Read the saved config and print it
		home, err := os.UserHomeDir()
		if err != nil {
			return fmt.Errorf("failed to get home directory: %w", err)
		}
		configPath := filepath.Join(home, ".cargoship.yaml")
		data, err := os.ReadFile(configPath)
		if err != nil {
			return fmt.Errorf("failed to read config file: %w", err)
		}
		fmt.Print(string(data))
		return nil
	default:
		return fmt.Errorf("unsupported format: %s (use yaml or json)", configFormat)
	}
}

func editConfig() error {
	// Find config file
	home, err := os.UserHomeDir()
	if err != nil {
		return fmt.Errorf("failed to get home directory: %w", err)
	}

	configPath := configFile
	if configPath == "" {
		configPath = filepath.Join(home, ".cargoship.yaml")
	}

	// Create config file if it doesn't exist
	if _, err := os.Stat(configPath); os.IsNotExist(err) {
		fmt.Printf("Creating new configuration file at %s\n", configPath)
		example := config.GenerateExampleConfig()
		if err := os.WriteFile(configPath, []byte(example), 0644); err != nil {
			return fmt.Errorf("failed to create config file: %w", err)
		}
	}

	// Get editor from environment
	editor := os.Getenv("EDITOR")
	if editor == "" {
		editor = os.Getenv("VISUAL")
	}
	if editor == "" {
		// Try common editors
		editors := []string{"nano", "vim", "vi", "emacs"}
		for _, e := range editors {
			if _, err := exec.LookPath(e); err == nil {
				editor = e
				break
			}
		}
	}

	if editor == "" {
		return fmt.Errorf("no editor found. Set EDITOR or VISUAL environment variable")
	}

	fmt.Printf("Opening %s with %s...\n", configPath, editor)

	// Execute editor
	cmd := exec.Command(editor, configPath)
	cmd.Stdin = os.Stdin
	cmd.Stdout = os.Stdout
	cmd.Stderr = os.Stderr

	if err := cmd.Run(); err != nil {
		return fmt.Errorf("editor failed: %w", err)
	}

	// Validate the edited configuration
	manager := config.NewManager()
	if err := manager.LoadConfig(configPath); err != nil {
		fmt.Printf("⚠️ Configuration validation failed: %v\n", err)
		fmt.Printf("Please fix the errors and try again.\n")
		return nil
	}

	fmt.Printf("✅ Configuration saved and validated successfully!\n")
	return nil
}

func init() {
	// This command will be added to root in root.go
}
