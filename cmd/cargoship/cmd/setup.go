package cmd

import (
	"bufio"
	"context"
	"errors"
	"fmt"
	"io"
	"io/fs"
	"log/slog"
	"os"
	"path/filepath"
	"strings"

	"github.com/aws/aws-sdk-go-v2/config"
	"github.com/aws/aws-sdk-go-v2/service/s3"
	"github.com/aws/aws-sdk-go-v2/service/sts"
	cargoshipConfig "github.com/scttfrdmn/cargoship/pkg/config"
	"github.com/spf13/cobra"
)

var (
	setupNonInteractive bool
	setupOutputPath     string
	setupForce          bool
)

// setupSections are the top-level config sections setup asks about. Only these
// are written back; everything else in an existing file is kept (#909).
var setupSections = []string{"aws", "storage", "upload", "metrics", "logging"}

// Credential and bucket checks, as variables so tests can run setup without AWS.
var (
	setupVerifyCredentials = verifyAWSCredentials
	setupVerifyBucket      = verifyBucketAccess
)

// NewSetupCmd creates an interactive setup wizard for CargoShip
func NewSetupCmd() *cobra.Command {
	cmd := &cobra.Command{
		Use:   "setup",
		Short: "Interactive setup wizard for CargoShip configuration",
		Long: `Interactive setup wizard that guides you through CargoShip configuration.

This wizard will help you:
  • Configure AWS credentials and region
  • Verify S3 bucket access
  • Set optimal upload parameters based on your use case
  • Test your configuration

The wizard writes ~/.cargoship.yaml. If that file exists, its current values
are the defaults, setup asks before changing it (or needs --force), only the
settings it asks about are updated, and the previous file is kept as .bak.

Examples:
  # Run interactive setup
  cargoship setup

  # Accept every default without prompting (needs --force to update an existing file)
  cargoship setup --non-interactive

  # Save configuration to custom location
  cargoship setup --output /path/to/config.yaml`,
		RunE: runSetup,
	}

	cmd.Flags().BoolVar(&setupNonInteractive, "non-interactive", false, "Accept every default without prompting (the existing file's values, if any)")
	cmd.Flags().StringVar(&setupOutputPath, "output", "", "Custom configuration file path (default: ~/.cargoship.yaml)")
	cmd.Flags().BoolVar(&setupForce, "force", false, "Update an existing configuration file without asking")

	return cmd
}

func runSetup(cmd *cobra.Command, args []string) error {
	return runSetupWith(os.Stdin, setupNonInteractive, setupForce, setupOutputPath)
}

// runSetupWith is the setup wizard reading answers from in.
//
// #909: setup used to start from DefaultConfig, never read the existing file,
// treat an empty answer or end of input as "yes", ignore --non-interactive, and
// rewrite the whole file. `cargoship setup --non-interactive </dev/null` exited 0
// having replaced the user's config with defaults, losing their region, profile,
// bucket, cost_control budgets and comments.
func runSetupWith(in io.Reader, nonInteractive, force bool, outputPath string) error {
	slog.Debug("Starting CargoShip setup wizard", "non-interactive", nonInteractive, "output-path", outputPath)

	configPath := outputPath
	if configPath == "" {
		home, err := os.UserHomeDir()
		if err != nil {
			return fmt.Errorf("failed to get home directory: %w", err)
		}
		configPath = filepath.Join(home, ".cargoship.yaml")
	}

	fmt.Println("╔══════════════════════════════════════════════════════════════╗")
	fmt.Println("║         Welcome to CargoShip Configuration Setup!           ║")
	fmt.Println("╚══════════════════════════════════════════════════════════════╝")
	fmt.Println()
	fmt.Println("This wizard will help you configure CargoShip for S3 uploads.")
	fmt.Println()

	reader := bufio.NewReader(in)
	if nonInteractive {
		reader = bufio.NewReader(strings.NewReader("")) // every prompt takes its default
	}

	cfg := cargoshipConfig.DefaultConfig()
	_, statErr := os.Stat(configPath)
	existing := statErr == nil
	if existing {
		loaded, err := cargoshipConfig.ReadConfigFile(configPath)
		if err != nil {
			return fmt.Errorf("setup will not overwrite a configuration it can't read; fix or move it first: %w", err)
		}
		cfg = loaded
		if !force {
			if nonInteractive {
				return fmt.Errorf("%s already exists; re-run with --force to update it "+
					"(setup changes only the settings it asks about and keeps a .bak copy)", configPath)
			}
			fmt.Printf("%s already exists. Its current values are the defaults below; setup updates only\n", configPath)
			fmt.Println("the settings it asks about, keeps everything else, and saves the old file as .bak.")
			if !confirmTyped(reader, "Type 'yes' to continue:") {
				return fmt.Errorf("setup cancelled; %s is unchanged", configPath)
			}
			fmt.Println()
		}
	} else if !errors.Is(statErr, fs.ErrNotExist) {
		return fmt.Errorf("check %s: %w", configPath, statErr)
	}

	if Verbose {
		slog.Info("Starting configuration",
			"existing", existing,
			"region", cfg.AWS.Region,
			"storage-class", cfg.Storage.DefaultStorageClass,
			"max-concurrency", cfg.Upload.MaxConcurrency,
			"chunk-size", cfg.Upload.ChunkSize)
	}

	// Step 1: AWS Configuration
	if err := setupAWS(reader, cfg); err != nil {
		return fmt.Errorf("AWS configuration failed: %w", err)
	}

	// Step 2: S3 Storage Configuration
	if err := setupStorage(reader, cfg, existing); err != nil {
		return fmt.Errorf("storage configuration failed: %w", err)
	}

	// Step 3: Upload Optimization
	if err := setupUpload(reader, cfg, existing); err != nil {
		return fmt.Errorf("upload configuration failed: %w", err)
	}

	// Step 4: Optional Features
	if err := setupOptionalFeatures(reader, cfg, existing); err != nil {
		return fmt.Errorf("optional features configuration failed: %w", err)
	}

	// Step 5: Test Configuration
	if err := testConfiguration(cfg); err != nil {
		fmt.Printf("\n⚠️  Warning: Configuration test failed: %v\n", err)
		fmt.Println("You can still save the configuration and fix issues later.")
		if !nonInteractive && !confirmDefault(reader, "Continue anyway?", true) {
			return fmt.Errorf("setup cancelled by user")
		}
	} else {
		fmt.Println("\n✅ Configuration test passed!")
	}

	// Step 6: Save Configuration
	backup, err := saveConfiguration(cfg, configPath)
	if err != nil {
		return fmt.Errorf("failed to save configuration: %w", err)
	}

	fmt.Println("\n╔══════════════════════════════════════════════════════════════╗")
	fmt.Println("║              Setup Complete! 🎉                              ║")
	fmt.Println("╚══════════════════════════════════════════════════════════════╝")
	fmt.Printf("\nConfiguration saved to: %s\n", configPath)
	if backup != "" {
		fmt.Printf("Previous configuration kept at: %s\n", backup)
	}
	fmt.Println("\nNext steps:")
	fmt.Println("  1. Test your setup:")
	fmt.Println("     cargoship upload <file> s3://your-bucket/path")
	fmt.Println("  2. View your configuration:")
	fmt.Println("     cargoship config --show")
	fmt.Println("  3. Edit configuration:")
	fmt.Println("     cargoship config --edit")
	fmt.Println()

	return nil
}

func setupAWS(reader *bufio.Reader, cfg *cargoshipConfig.Config) error {
	slog.Debug("Starting AWS configuration step")

	fmt.Println("━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━")
	fmt.Println("Step 1: AWS Configuration")
	fmt.Println("━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━")
	fmt.Println()

	// Region
	fmt.Printf("AWS Region [%s]: ", cfg.AWS.Region)
	region := readLine(reader)
	if region != "" {
		slog.Debug("User provided custom region", "region", region)
		cfg.AWS.Region = region
	} else {
		slog.Debug("Using default region", "region", cfg.AWS.Region)
	}

	// Profile
	if cfg.AWS.Profile != "" {
		fmt.Printf("AWS Profile [%s]: ", cfg.AWS.Profile)
	} else {
		fmt.Print("AWS Profile (leave empty for default credentials): ")
	}
	profile := readLine(reader)
	if profile != "" {
		slog.Debug("User provided AWS profile", "profile", profile)
		cfg.AWS.Profile = profile
	} else {
		slog.Debug("Using default credentials chain")
	}

	// Verify AWS credentials
	fmt.Println("\nVerifying AWS credentials...")
	slog.Debug("Attempting to verify AWS credentials", "region", cfg.AWS.Region, "profile", cfg.AWS.Profile)

	if err := setupVerifyCredentials(cfg.AWS.Region, cfg.AWS.Profile); err != nil {
		slog.Warn("AWS credential verification failed", "error", err)
		fmt.Printf("⚠️  Warning: Could not verify AWS credentials: %v\n", err)
		fmt.Println("You may need to configure AWS credentials before using CargoShip.")
		fmt.Println("See: https://docs.aws.amazon.com/cli/latest/userguide/cli-configure-files.html")
	} else {
		slog.Info("AWS credentials verified successfully", "region", cfg.AWS.Region, "profile", cfg.AWS.Profile)
		fmt.Println("✅ AWS credentials verified")
	}

	fmt.Println()
	return nil
}

// setupStorage asks for the storage settings. With keep (an existing file), an
// empty answer keeps the current value instead of applying the wizard default.
func setupStorage(reader *bufio.Reader, cfg *cargoshipConfig.Config, keep bool) error {
	slog.Debug("Starting S3 storage configuration step")

	fmt.Println("━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━")
	fmt.Println("Step 2: S3 Storage Configuration")
	fmt.Println("━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━")
	fmt.Println()

	// Default bucket
	if cfg.Storage.DefaultBucket != "" {
		fmt.Printf("Default S3 Bucket [%s]: ", cfg.Storage.DefaultBucket)
	} else {
		fmt.Print("Default S3 Bucket (optional): ")
	}
	bucket := readLine(reader)
	if bucket != "" {
		slog.Debug("User provided S3 bucket", "bucket", bucket)
		cfg.Storage.DefaultBucket = bucket

		// Verify bucket access
		fmt.Println("\nVerifying bucket access...")
		slog.Debug("Attempting to verify S3 bucket access", "bucket", bucket, "region", cfg.AWS.Region)

		if err := setupVerifyBucket(cfg.AWS.Region, cfg.AWS.Profile, bucket); err != nil {
			slog.Warn("S3 bucket verification failed", "error", err, "bucket", bucket)
			fmt.Printf("⚠️  Warning: Could not access bucket: %v\n", err)
			fmt.Println("Make sure the bucket exists and you have proper permissions.")
		} else {
			slog.Info("S3 bucket access verified successfully", "bucket", bucket)
			fmt.Println("✅ Bucket access verified")
		}
	} else {
		slog.Debug("No default bucket specified")
	}

	// Storage class
	fmt.Println("\nS3 Storage Classes:")
	fmt.Println("  1. INTELLIGENT_TIERING (recommended - automatic cost optimization)")
	fmt.Println("  2. STANDARD (frequent access)")
	fmt.Println("  3. STANDARD_IA (infrequent access)")
	fmt.Println("  4. GLACIER (long-term archive)")
	fmt.Println("  5. DEEP_ARCHIVE (lowest cost, rare access)")
	if keep {
		fmt.Printf("Choose storage class [keep %s]: ", cfg.Storage.DefaultStorageClass)
	} else {
		fmt.Printf("Choose storage class [1]: ")
	}
	choice := readLine(reader)

	storageClasses := map[string]string{
		"1": "INTELLIGENT_TIERING",
		"2": "STANDARD",
		"3": "STANDARD_IA",
		"4": "GLACIER",
		"5": "DEEP_ARCHIVE",
		"":  "INTELLIGENT_TIERING", // default
	}

	if choice == "" && keep {
		slog.Debug("Keeping storage class", "storage-class", cfg.Storage.DefaultStorageClass)
	} else if class, ok := storageClasses[choice]; ok {
		slog.Debug("Storage class selected", "choice", choice, "storage-class", class)
		cfg.Storage.DefaultStorageClass = class
	} else {
		slog.Warn("Invalid storage class choice, using default", "choice", choice, "default", "INTELLIGENT_TIERING")
		cfg.Storage.DefaultStorageClass = "INTELLIGENT_TIERING"
	}

	// Encryption
	if confirmDefault(reader, "Enable server-side encryption?", !keep || cfg.Storage.SSEEncryption) {
		slog.Debug("Server-side encryption enabled")
		cfg.Storage.SSEEncryption = true
	} else {
		slog.Debug("Server-side encryption disabled")
		cfg.Storage.SSEEncryption = false
	}

	fmt.Println()
	return nil
}

// setupUpload asks for the upload profile. With keep, an empty answer keeps the
// current settings.
func setupUpload(reader *bufio.Reader, cfg *cargoshipConfig.Config, keep bool) error {
	slog.Debug("Starting upload optimization step")

	fmt.Println("━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━")
	fmt.Println("Step 3: Upload Optimization")
	fmt.Println("━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━")
	fmt.Println()

	fmt.Println("What type of files will you primarily upload?")
	fmt.Println("  1. Large files (>1GB) - videos, archives, datasets")
	fmt.Println("  2. Medium files (100MB-1GB) - documents, images")
	fmt.Println("  3. Small files (<100MB) - logs, configs, code")
	fmt.Println("  4. Mixed sizes")
	if keep {
		fmt.Printf("Choose file size profile [keep current: %d workers, %s chunks]: ", cfg.Upload.MaxConcurrency, cfg.Upload.ChunkSize)
	} else {
		fmt.Printf("Choose file size profile [4]: ")
	}
	sizeProfile := readLine(reader)

	if sizeProfile == "" && keep {
		fmt.Println("\n✅ Keeping the current upload settings")
	} else {
		applySizeProfile(cfg, sizeProfile)
	}

	// Compression
	fmt.Println("\nCompression options:")
	fmt.Println("  1. zstd (recommended - fast and efficient)")
	fmt.Println("  2. gzip (compatible, slower)")
	fmt.Println("  3. none (no compression)")
	if keep {
		fmt.Printf("Choose compression [keep %s]: ", cfg.Upload.CompressionType)
	} else {
		fmt.Printf("Choose compression [1]: ")
	}
	compressionChoice := readLine(reader)

	compressionMap := map[string]string{
		"1": "zstd",
		"2": "gzip",
		"3": "none",
		"":  "zstd",
	}

	if compressionChoice == "" && keep {
		slog.Debug("Keeping compression", "algorithm", cfg.Upload.CompressionType)
	} else if compression, ok := compressionMap[compressionChoice]; ok {
		slog.Debug("Compression algorithm selected", "choice", compressionChoice, "algorithm", compression)
		cfg.Upload.CompressionType = compression
	} else {
		slog.Warn("Invalid compression choice, using default", "choice", compressionChoice, "default", "zstd")
		cfg.Upload.CompressionType = "zstd"
	}

	if Verbose {
		slog.Info("Upload optimization configured",
			"max-concurrency", cfg.Upload.MaxConcurrency,
			"chunk-size", cfg.Upload.ChunkSize,
			"compression", cfg.Upload.CompressionType,
			"adaptive-sizing", cfg.Upload.EnableAdaptiveSizing)
	}

	fmt.Println()
	return nil
}

// setupOptionalFeatures asks about metrics and logging. With keep, an empty
// answer keeps the current value.
func setupOptionalFeatures(reader *bufio.Reader, cfg *cargoshipConfig.Config, keep bool) error {
	slog.Debug("Starting optional features configuration step")

	fmt.Println("━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━")
	fmt.Println("Step 4: Optional Features")
	fmt.Println("━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━")
	fmt.Println()

	// CloudWatch metrics
	if confirmDefault(reader, "Enable CloudWatch metrics?", !keep || cfg.Metrics.Enabled) {
		cfg.Metrics.Enabled = true
		slog.Debug("CloudWatch metrics enabled")
		fmt.Print("Metrics namespace [CargoShip/Production]: ")
		namespace := readLine(reader)
		if namespace != "" {
			slog.Debug("Custom metrics namespace specified", "namespace", namespace)
			cfg.Metrics.Namespace = namespace
		} else {
			slog.Debug("Using default metrics namespace", "namespace", "CargoShip/Production")
		}
	} else {
		cfg.Metrics.Enabled = false
		slog.Debug("CloudWatch metrics disabled")
	}

	// Logging level
	fmt.Println("\nLogging levels:")
	fmt.Println("  1. debug (detailed logs)")
	fmt.Println("  2. info (standard logs)")
	fmt.Println("  3. warn (warnings only)")
	fmt.Println("  4. error (errors only)")
	if keep {
		fmt.Printf("Choose log level [keep %s]: ", cfg.Logging.Level)
	} else {
		fmt.Printf("Choose log level [2]: ")
	}
	logChoice := readLine(reader)

	logLevelMap := map[string]string{
		"1": "debug",
		"2": "info",
		"3": "warn",
		"4": "error",
		"":  "info",
	}

	if logChoice == "" && keep {
		slog.Debug("Keeping log level", "level", cfg.Logging.Level)
	} else if level, ok := logLevelMap[logChoice]; ok {
		slog.Debug("Log level selected", "choice", logChoice, "level", level)
		cfg.Logging.Level = level
	} else {
		slog.Warn("Invalid log level choice, using default", "choice", logChoice, "default", "info")
		cfg.Logging.Level = "info"
	}

	if Verbose {
		slog.Info("Optional features configured",
			"metrics-enabled", cfg.Metrics.Enabled,
			"metrics-namespace", cfg.Metrics.Namespace,
			"log-level", cfg.Logging.Level)
	}

	fmt.Println()
	return nil
}

func testConfiguration(cfg *cargoshipConfig.Config) error {
	slog.Debug("Starting configuration validation step")

	fmt.Println("━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━")
	fmt.Println("Step 5: Testing Configuration")
	fmt.Println("━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━")
	fmt.Println()

	// Test AWS credentials
	fmt.Print("Testing AWS credentials... ")
	slog.Debug("Testing AWS credentials", "region", cfg.AWS.Region, "profile", cfg.AWS.Profile)

	if err := setupVerifyCredentials(cfg.AWS.Region, cfg.AWS.Profile); err != nil {
		fmt.Println("❌")
		slog.Error("AWS credentials test failed", "error", err)
		return err
	}
	fmt.Println("✅")
	slog.Info("AWS credentials test passed")

	// Test bucket access (if configured)
	if cfg.Storage.DefaultBucket != "" {
		fmt.Printf("Testing bucket access... ")
		slog.Debug("Testing S3 bucket access", "bucket", cfg.Storage.DefaultBucket)

		if err := setupVerifyBucket(cfg.AWS.Region, cfg.AWS.Profile, cfg.Storage.DefaultBucket); err != nil {
			fmt.Println("❌")
			slog.Error("S3 bucket access test failed", "error", err, "bucket", cfg.Storage.DefaultBucket)
			return err
		}
		fmt.Println("✅")
		slog.Info("S3 bucket access test passed", "bucket", cfg.Storage.DefaultBucket)
	} else {
		slog.Debug("Skipping bucket access test (no bucket configured)")
	}

	slog.Info("Configuration validation completed successfully")
	return nil
}

// saveConfiguration writes setup's sections into path, keeping everything else
// in an existing file, and returns the backup of the previous file ("" if new).
func saveConfiguration(cfg *cargoshipConfig.Config, path string) (string, error) {
	slog.Debug("Saving configuration", "path", path)

	if Verbose {
		slog.Info("Configuration summary",
			"aws-region", cfg.AWS.Region,
			"aws-profile", cfg.AWS.Profile,
			"default-bucket", cfg.Storage.DefaultBucket,
			"storage-class", cfg.Storage.DefaultStorageClass,
			"sse-encryption", cfg.Storage.SSEEncryption,
			"max-concurrency", cfg.Upload.MaxConcurrency,
			"chunk-size", cfg.Upload.ChunkSize,
			"compression", cfg.Upload.CompressionType,
			"adaptive-sizing", cfg.Upload.EnableAdaptiveSizing,
			"metrics-enabled", cfg.Metrics.Enabled,
			"log-level", cfg.Logging.Level)
	}

	backup, err := cargoshipConfig.WriteConfigSections(path, cfg, setupSections)
	if err != nil {
		slog.Error("Failed to save configuration", "error", err, "path", path)
		return "", err
	}

	slog.Info("Configuration saved successfully", "path", path)
	return backup, nil
}

// Helper functions

// readLine reads one answer. A last line without a newline still counts.
func readLine(reader *bufio.Reader) string {
	line, err := reader.ReadString('\n')
	if err != nil && line == "" {
		return ""
	}
	return strings.TrimSpace(line)
}

// confirmDefault asks a yes/no question whose empty answer is def.
func confirmDefault(reader *bufio.Reader, prompt string, def bool) bool {
	hint := "[y/N]"
	if def {
		hint = "[Y/n]"
	}
	fmt.Print(prompt + " " + hint + " ")
	switch strings.ToLower(readLine(reader)) {
	case "":
		return def
	case "y", "yes":
		return true
	default:
		return false
	}
}

// confirmTyped asks for an explicit "yes". Anything else, including an empty
// answer or end of input, is no (#909: EOF used to count as yes).
func confirmTyped(reader *bufio.Reader, prompt string) bool {
	fmt.Print(prompt + " ")
	return strings.ToLower(readLine(reader)) == "yes"
}

func verifyAWSCredentials(region, profile string) error {
	ctx := context.Background()

	slog.Debug("Verifying AWS credentials via STS GetCallerIdentity", "region", region, "profile", profile)

	var opts []func(*config.LoadOptions) error
	if region != "" {
		opts = append(opts, config.WithRegion(region))
	}
	if profile != "" {
		opts = append(opts, config.WithSharedConfigProfile(profile))
	}

	cfg, err := config.LoadDefaultConfig(ctx, opts...)
	if err != nil {
		slog.Error("Failed to load AWS config", "error", err)
		return fmt.Errorf("failed to load AWS config: %w", err)
	}

	// Try to get caller identity
	stsClient := sts.NewFromConfig(cfg)
	identity, err := stsClient.GetCallerIdentity(ctx, &sts.GetCallerIdentityInput{})
	if err != nil {
		slog.Error("Failed to call STS GetCallerIdentity", "error", err)
		return fmt.Errorf("failed to verify AWS credentials: %w", err)
	}

	if Verbose && identity != nil {
		slog.Info("AWS identity verified",
			"account", *identity.Account,
			"user-id", *identity.UserId,
			"arn", *identity.Arn)
	}

	return nil
}

func verifyBucketAccess(region, profile, bucket string) error {
	ctx := context.Background()

	slog.Debug("Verifying S3 bucket access via HeadBucket", "bucket", bucket, "region", region, "profile", profile)

	var opts []func(*config.LoadOptions) error
	if region != "" {
		opts = append(opts, config.WithRegion(region))
	}
	if profile != "" {
		opts = append(opts, config.WithSharedConfigProfile(profile))
	}

	cfg, err := config.LoadDefaultConfig(ctx, opts...)
	if err != nil {
		slog.Error("Failed to load AWS config for S3", "error", err)
		return fmt.Errorf("failed to load AWS config: %w", err)
	}

	// Try to head the bucket
	s3Client := s3.NewFromConfig(cfg)
	output, err := s3Client.HeadBucket(ctx, &s3.HeadBucketInput{
		Bucket: &bucket,
	})
	if err != nil {
		slog.Error("Failed to call S3 HeadBucket", "error", err, "bucket", bucket)
		return fmt.Errorf("failed to access bucket: %w", err)
	}

	if Verbose && output != nil {
		slog.Info("S3 bucket access verified", "bucket", bucket, "region", region)
	}

	return nil
}

func init() {
	// This command will be added to root in root.go
}

// applySizeProfile applies the upload settings for a file size profile choice;
// anything other than 1-3 is the mixed profile.
func applySizeProfile(cfg *cargoshipConfig.Config, sizeProfile string) {
	switch sizeProfile {
	case "1": // Large files
		cfg.Upload.MaxConcurrency = 16
		cfg.Upload.ChunkSize = "32MB"
		slog.Info("Configured for large files", "max-concurrency", 16, "chunk-size", "32MB")
		fmt.Println("\n✅ Configured for large files (high concurrency, large chunks)")
	case "2": // Medium files
		cfg.Upload.MaxConcurrency = 8
		cfg.Upload.ChunkSize = "16MB"
		slog.Info("Configured for medium files", "max-concurrency", 8, "chunk-size", "16MB")
		fmt.Println("\n✅ Configured for medium files (balanced settings)")
	case "3": // Small files
		cfg.Upload.MaxConcurrency = 4
		cfg.Upload.ChunkSize = "8MB"
		slog.Info("Configured for small files", "max-concurrency", 4, "chunk-size", "8MB")
		fmt.Println("\n✅ Configured for small files (lower concurrency, small chunks)")
	default: // Mixed or unspecified
		cfg.Upload.MaxConcurrency = 8
		cfg.Upload.ChunkSize = "16MB"
		cfg.Upload.EnableAdaptiveSizing = true
		slog.Info("Configured for mixed files", "max-concurrency", 8, "chunk-size", "16MB", "adaptive-sizing", true)
		fmt.Println("\n✅ Configured for mixed files (adaptive sizing enabled)")

	}
}
