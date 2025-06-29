// Package config provides centralized configuration management for CargoShip
package config

import (
	"fmt"
	"os"
	"path/filepath"
	"time"

	"github.com/spf13/viper"
	"gopkg.in/yaml.v3"
)

// Config represents the complete CargoShip configuration
type Config struct {
	AWS      AWSConfig      `yaml:"aws" mapstructure:"aws"`
	Storage  StorageConfig  `yaml:"storage" mapstructure:"storage"`
	Upload   UploadConfig   `yaml:"upload" mapstructure:"upload"`
	Metrics  MetricsConfig  `yaml:"metrics" mapstructure:"metrics"`
	Logging  LoggingConfig  `yaml:"logging" mapstructure:"logging"`
	Security SecurityConfig `yaml:"security" mapstructure:"security"`
}

// AWSConfig contains AWS-specific configuration
type AWSConfig struct {
	Region           string `yaml:"region" mapstructure:"region"`
	Profile          string `yaml:"profile" mapstructure:"profile"`
	AccessKeyID      string `yaml:"access_key_id,omitempty" mapstructure:"access_key_id"`
	SecretAccessKey  string `yaml:"secret_access_key,omitempty" mapstructure:"secret_access_key"`
	SessionToken     string `yaml:"session_token,omitempty" mapstructure:"session_token"`
	S3Endpoint       string `yaml:"s3_endpoint,omitempty" mapstructure:"s3_endpoint"`
	UsePathStyle     bool   `yaml:"use_path_style" mapstructure:"use_path_style"`
	MaxRetries       int    `yaml:"max_retries" mapstructure:"max_retries"`
	RetryMaxDelay    string `yaml:"retry_max_delay" mapstructure:"retry_max_delay"`
	RequestTimeout   string `yaml:"request_timeout" mapstructure:"request_timeout"`
}

// StorageConfig contains S3 storage configuration
type StorageConfig struct {
	DefaultBucket      string `yaml:"default_bucket" mapstructure:"default_bucket"`
	DefaultStorageClass string `yaml:"default_storage_class" mapstructure:"default_storage_class"`
	KMSKeyID           string `yaml:"kms_key_id,omitempty" mapstructure:"kms_key_id"`
	SSEEncryption      bool   `yaml:"sse_encryption" mapstructure:"sse_encryption"`
	ObjectTagging      map[string]string `yaml:"object_tagging,omitempty" mapstructure:"object_tagging"`
	MetadataDirective  string `yaml:"metadata_directive" mapstructure:"metadata_directive"`
}

// UploadConfig contains upload optimization settings
type UploadConfig struct {
	MaxConcurrency      int    `yaml:"max_concurrency" mapstructure:"max_concurrency"`
	ChunkSize           string `yaml:"chunk_size" mapstructure:"chunk_size"`
	EnableAdaptiveSizing bool   `yaml:"enable_adaptive_sizing" mapstructure:"enable_adaptive_sizing"`
	MaxPrefixes         int    `yaml:"max_prefixes" mapstructure:"max_prefixes"`
	PrefixPattern       string `yaml:"prefix_pattern" mapstructure:"prefix_pattern"`
	CompressionType     string `yaml:"compression_type" mapstructure:"compression_type"`
	CompressionLevel    int    `yaml:"compression_level" mapstructure:"compression_level"`
	ChecksumAlgorithm   string `yaml:"checksum_algorithm" mapstructure:"checksum_algorithm"`
	MemoryLimit         string `yaml:"memory_limit,omitempty" mapstructure:"memory_limit"`
}

// MetricsConfig contains CloudWatch metrics configuration
type MetricsConfig struct {
	Enabled       bool   `yaml:"enabled" mapstructure:"enabled"`
	Namespace     string `yaml:"namespace" mapstructure:"namespace"`
	FlushInterval string `yaml:"flush_interval" mapstructure:"flush_interval"`
	BatchSize     int    `yaml:"batch_size" mapstructure:"batch_size"`
	Region        string `yaml:"region,omitempty" mapstructure:"region"`
	DryRun        bool   `yaml:"dry_run" mapstructure:"dry_run"`
}

// LoggingConfig contains logging configuration
type LoggingConfig struct {
	Level      string `yaml:"level" mapstructure:"level"`
	Structured bool   `yaml:"structured" mapstructure:"structured"`
	Timestamp  bool   `yaml:"timestamp" mapstructure:"timestamp"`
	Caller     bool   `yaml:"caller" mapstructure:"caller"`
	Output     string `yaml:"output,omitempty" mapstructure:"output"`
}

// SecurityConfig contains security-related settings
type SecurityConfig struct {
	RequireEncryption    bool     `yaml:"require_encryption" mapstructure:"require_encryption"`
	AllowedRegions       []string `yaml:"allowed_regions,omitempty" mapstructure:"allowed_regions"`
	AllowedStorageClasses []string `yaml:"allowed_storage_classes,omitempty" mapstructure:"allowed_storage_classes"`
	MaxFileSize          string   `yaml:"max_file_size,omitempty" mapstructure:"max_file_size"`
	BlockedExtensions    []string `yaml:"blocked_extensions,omitempty" mapstructure:"blocked_extensions"`
}

// DefaultConfig returns a configuration with sensible defaults
func DefaultConfig() *Config {
	return &Config{
		AWS: AWSConfig{
			Region:         "us-west-2",
			MaxRetries:     3,
			RetryMaxDelay:  "30s",
			RequestTimeout: "5m",
		},
		Storage: StorageConfig{
			DefaultStorageClass: "INTELLIGENT_TIERING",
			SSEEncryption:      true,
			MetadataDirective:  "REPLACE",
		},
		Upload: UploadConfig{
			MaxConcurrency:       8,
			ChunkSize:           "16MB",
			EnableAdaptiveSizing: true,
			MaxPrefixes:         8,
			PrefixPattern:       "hash",
			CompressionType:     "zstd",
			CompressionLevel:    3,
			ChecksumAlgorithm:   "SHA256",
		},
		Metrics: MetricsConfig{
			Enabled:       true,
			Namespace:     "CargoShip/Production",
			FlushInterval: "30s",
			BatchSize:     20,
			DryRun:        false,
		},
		Logging: LoggingConfig{
			Level:      "info",
			Structured: false,
			Timestamp:  true,
			Caller:     false,
		},
		Security: SecurityConfig{
			RequireEncryption: false,
			AllowedStorageClasses: []string{
				"STANDARD",
				"STANDARD_IA",
				"ONEZONE_IA",
				"INTELLIGENT_TIERING",
				"GLACIER",
				"DEEP_ARCHIVE",
			},
		},
	}
}

// Manager handles configuration loading, validation, and management
type Manager struct {
	config     *Config
	configPath string
	viper      *viper.Viper
}

// NewManager creates a new configuration manager
func NewManager() *Manager {
	return &Manager{
		config: DefaultConfig(),
		viper:  viper.New(),
	}
}

// LoadConfig loads configuration from file and environment variables
func (m *Manager) LoadConfig(configPath string) error {
	m.configPath = configPath
	
	// Set up viper
	m.viper.SetConfigType("yaml")
	m.viper.SetEnvPrefix("CARGOSHIP")
	m.viper.AutomaticEnv()
	
	// Set defaults
	m.setDefaults()
	
	// Load from config file if it exists
	if configPath != "" {
		m.viper.SetConfigFile(configPath)
	} else {
		// Look for config in standard locations
		home, err := os.UserHomeDir()
		if err == nil {
			m.viper.AddConfigPath(home)
			m.viper.AddConfigPath(filepath.Join(home, ".config", "cargoship"))
		}
		m.viper.AddConfigPath(".")
		m.viper.SetConfigName(".cargoship")
	}
	
	// Read config file (ignore if not found)
	if err := m.viper.ReadInConfig(); err != nil {
		if _, ok := err.(viper.ConfigFileNotFoundError); !ok {
			return fmt.Errorf("failed to read config file: %w", err)
		}
	}
	
	// Unmarshal into struct
	if err := m.viper.Unmarshal(m.config); err != nil {
		return fmt.Errorf("failed to unmarshal config: %w", err)
	}
	
	// Validate configuration
	if err := m.validateConfig(); err != nil {
		return fmt.Errorf("config validation failed: %w", err)
	}
	
	return nil
}

// setDefaults sets default values in viper
func (m *Manager) setDefaults() {
	defaults := DefaultConfig()
	
	// AWS defaults
	m.viper.SetDefault("aws.region", defaults.AWS.Region)
	m.viper.SetDefault("aws.max_retries", defaults.AWS.MaxRetries)
	m.viper.SetDefault("aws.retry_max_delay", defaults.AWS.RetryMaxDelay)
	m.viper.SetDefault("aws.request_timeout", defaults.AWS.RequestTimeout)
	
	// Storage defaults
	m.viper.SetDefault("storage.default_storage_class", defaults.Storage.DefaultStorageClass)
	m.viper.SetDefault("storage.sse_encryption", defaults.Storage.SSEEncryption)
	m.viper.SetDefault("storage.metadata_directive", defaults.Storage.MetadataDirective)
	
	// Upload defaults
	m.viper.SetDefault("upload.max_concurrency", defaults.Upload.MaxConcurrency)
	m.viper.SetDefault("upload.chunk_size", defaults.Upload.ChunkSize)
	m.viper.SetDefault("upload.enable_adaptive_sizing", defaults.Upload.EnableAdaptiveSizing)
	m.viper.SetDefault("upload.max_prefixes", defaults.Upload.MaxPrefixes)
	m.viper.SetDefault("upload.prefix_pattern", defaults.Upload.PrefixPattern)
	m.viper.SetDefault("upload.compression_type", defaults.Upload.CompressionType)
	m.viper.SetDefault("upload.compression_level", defaults.Upload.CompressionLevel)
	m.viper.SetDefault("upload.checksum_algorithm", defaults.Upload.ChecksumAlgorithm)
	
	// Metrics defaults
	m.viper.SetDefault("metrics.enabled", defaults.Metrics.Enabled)
	m.viper.SetDefault("metrics.namespace", defaults.Metrics.Namespace)
	m.viper.SetDefault("metrics.flush_interval", defaults.Metrics.FlushInterval)
	m.viper.SetDefault("metrics.batch_size", defaults.Metrics.BatchSize)
	m.viper.SetDefault("metrics.dry_run", defaults.Metrics.DryRun)
	
	// Logging defaults
	m.viper.SetDefault("logging.level", defaults.Logging.Level)
	m.viper.SetDefault("logging.structured", defaults.Logging.Structured)
	m.viper.SetDefault("logging.timestamp", defaults.Logging.Timestamp)
	m.viper.SetDefault("logging.caller", defaults.Logging.Caller)
	
	// Security defaults
	m.viper.SetDefault("security.require_encryption", defaults.Security.RequireEncryption)
	m.viper.SetDefault("security.allowed_storage_classes", defaults.Security.AllowedStorageClasses)
}

// validateConfig validates the loaded configuration
func (m *Manager) validateConfig() error {
	// Validate AWS region
	if m.config.AWS.Region == "" {
		return fmt.Errorf("aws.region is required")
	}
	
	// Validate storage class
	validStorageClasses := map[string]bool{
		"STANDARD":            true,
		"STANDARD_IA":         true,
		"ONEZONE_IA":          true,
		"INTELLIGENT_TIERING": true,
		"GLACIER":             true,
		"DEEP_ARCHIVE":        true,
	}
	
	if !validStorageClasses[m.config.Storage.DefaultStorageClass] {
		return fmt.Errorf("invalid storage class: %s", m.config.Storage.DefaultStorageClass)
	}
	
	// Validate upload settings
	if m.config.Upload.MaxConcurrency < 1 || m.config.Upload.MaxConcurrency > 100 {
		return fmt.Errorf("max_concurrency must be between 1 and 100")
	}
	
	// Validate metrics settings
	if m.config.Metrics.Enabled && m.config.Metrics.Namespace == "" {
		return fmt.Errorf("metrics.namespace is required when metrics are enabled")
	}
	
	// Validate logging level
	validLogLevels := map[string]bool{
		"debug": true,
		"info":  true,
		"warn":  true,
		"error": true,
	}
	
	if !validLogLevels[m.config.Logging.Level] {
		return fmt.Errorf("invalid log level: %s", m.config.Logging.Level)
	}
	
	return nil
}

// GetConfig returns the current configuration
func (m *Manager) GetConfig() *Config {
	return m.config
}

// SaveConfig saves the current configuration to file
func (m *Manager) SaveConfig(path string) error {
	if path == "" {
		if m.configPath != "" {
			path = m.configPath
		} else {
			home, err := os.UserHomeDir()
			if err != nil {
				return fmt.Errorf("failed to get home directory: %w", err)
			}
			path = filepath.Join(home, ".cargoship.yaml")
		}
	}
	
	// Create directory if it doesn't exist
	dir := filepath.Dir(path)
	if err := os.MkdirAll(dir, 0755); err != nil {
		return fmt.Errorf("failed to create config directory: %w", err)
	}
	
	// Marshal to YAML
	data, err := yaml.Marshal(m.config)
	if err != nil {
		return fmt.Errorf("failed to marshal config: %w", err)
	}
	
	// Write to file
	if err := os.WriteFile(path, data, 0644); err != nil {
		return fmt.Errorf("failed to write config file: %w", err)
	}
	
	m.configPath = path
	return nil
}

// UpdateConfig updates specific configuration values
func (m *Manager) UpdateConfig(updates map[string]interface{}) error {
	for key, value := range updates {
		m.viper.Set(key, value)
	}
	
	// Re-unmarshal the configuration
	if err := m.viper.Unmarshal(m.config); err != nil {
		return fmt.Errorf("failed to update config: %w", err)
	}
	
	// Validate updated configuration
	if err := m.validateConfig(); err != nil {
		return fmt.Errorf("updated config validation failed: %w", err)
	}
	
	return nil
}

// GetDuration parses a duration string from config
func (m *Manager) GetDuration(key string) (time.Duration, error) {
	value := m.viper.GetString(key)
	if value == "" {
		return 0, fmt.Errorf("duration not found: %s", key)
	}
	
	return time.ParseDuration(value)
}

// GetBytes parses a byte size string from config (e.g., "16MB", "1GB")
func (m *Manager) GetBytes(key string) (int64, error) {
	value := m.viper.GetString(key)
	if value == "" {
		return 0, fmt.Errorf("byte size not found: %s", key)
	}
	
	return parseBytes(value)
}

// parseBytes parses byte size strings like "16MB", "1GB", etc.
func parseBytes(s string) (int64, error) {
	multipliers := map[string]int64{
		"B":  1,
		"KB": 1024,
		"MB": 1024 * 1024,
		"GB": 1024 * 1024 * 1024,
		"TB": 1024 * 1024 * 1024 * 1024,
	}
	
	// Simple parsing - in production, use a more robust parser
	var value float64
	var unit string
	
	n, err := fmt.Sscanf(s, "%f%s", &value, &unit)
	if err != nil || n != 2 {
		return 0, fmt.Errorf("invalid byte size format: %s", s)
	}
	
	multiplier, ok := multipliers[unit]
	if !ok {
		return 0, fmt.Errorf("unknown unit: %s", unit)
	}
	
	return int64(value * float64(multiplier)), nil
}

// GenerateExampleConfig generates an example configuration file
func GenerateExampleConfig() string {
	config := DefaultConfig()
	
	// Add example values
	config.AWS.Profile = "cargoship"
	config.Storage.DefaultBucket = "my-archive-bucket"
	config.Storage.KMSKeyID = "arn:aws:kms:us-west-2:123456789012:key/12345678-1234-1234-1234-123456789012"
	config.Storage.ObjectTagging = map[string]string{
		"Environment": "production",
		"Project":     "data-archive",
	}
	config.Security.AllowedRegions = []string{"us-west-2", "us-east-1"}
	config.Security.MaxFileSize = "10GB"
	config.Security.BlockedExtensions = []string{".exe", ".bat", ".sh"}
	
	data, _ := yaml.Marshal(config)
	return string(data)
}