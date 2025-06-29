package cmd

import (
	"encoding/json"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"

	"github.com/spf13/cobra"
	"github.com/scttfrdmn/cargoship-cli/pkg/config"
)

var (
	configFile     string
	configGenerate bool
	configEdit     bool
	configValidate bool
	configShow     bool
	configFormat   string
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

	// Handle validate flag
	if configValidate {
		return validateConfig(manager)
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

func validateConfig(manager *config.Manager) error {
	fmt.Printf("✅ Configuration is valid!\n")
	
	cfg := manager.GetConfig()
	
	fmt.Printf("\nConfiguration summary:\n")
	fmt.Printf("  AWS Region: %s\n", cfg.AWS.Region)
	if cfg.AWS.Profile != "" {
		fmt.Printf("  AWS Profile: %s\n", cfg.AWS.Profile)
	}
	if cfg.Storage.DefaultBucket != "" {
		fmt.Printf("  Default Bucket: %s\n", cfg.Storage.DefaultBucket)
	}
	fmt.Printf("  Storage Class: %s\n", cfg.Storage.DefaultStorageClass)
	fmt.Printf("  Upload Concurrency: %d\n", cfg.Upload.MaxConcurrency)
	fmt.Printf("  Chunk Size: %s\n", cfg.Upload.ChunkSize)
	fmt.Printf("  Metrics Enabled: %t\n", cfg.Metrics.Enabled)
	if cfg.Metrics.Enabled {
		fmt.Printf("  Metrics Namespace: %s\n", cfg.Metrics.Namespace)
	}
	fmt.Printf("  Log Level: %s\n", cfg.Logging.Level)
	
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