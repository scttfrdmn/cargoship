package config

import (
	"os"
	"path/filepath"
	"reflect"
	"strings"
	"testing"
	"time"
)

func TestDefaultConfig(t *testing.T) {
	config := DefaultConfig()

	if config == nil {
		t.Fatalf("DefaultConfig() returned nil")
	}

	// Test AWS defaults
	if config.AWS.Region != "us-west-2" {
		t.Errorf("DefaultConfig() AWS.Region = %v, want us-west-2", config.AWS.Region)
	}
	if config.AWS.MaxRetries != 3 {
		t.Errorf("DefaultConfig() AWS.MaxRetries = %v, want 3", config.AWS.MaxRetries)
	}
	if config.AWS.RetryMaxDelay != "30s" {
		t.Errorf("DefaultConfig() AWS.RetryMaxDelay = %v, want 30s", config.AWS.RetryMaxDelay)
	}
	if config.AWS.RequestTimeout != "5m" {
		t.Errorf("DefaultConfig() AWS.RequestTimeout = %v, want 5m", config.AWS.RequestTimeout)
	}

	// Test Storage defaults
	if config.Storage.DefaultStorageClass != "INTELLIGENT_TIERING" {
		t.Errorf("DefaultConfig() Storage.DefaultStorageClass = %v, want INTELLIGENT_TIERING", config.Storage.DefaultStorageClass)
	}
	if !config.Storage.SSEEncryption {
		t.Errorf("DefaultConfig() Storage.SSEEncryption = false, want true")
	}
	if config.Storage.MetadataDirective != "REPLACE" {
		t.Errorf("DefaultConfig() Storage.MetadataDirective = %v, want REPLACE", config.Storage.MetadataDirective)
	}

	// Test Upload defaults
	if config.Upload.MaxConcurrency != 8 {
		t.Errorf("DefaultConfig() Upload.MaxConcurrency = %v, want 8", config.Upload.MaxConcurrency)
	}
	if config.Upload.ChunkSize != "16MB" {
		t.Errorf("DefaultConfig() Upload.ChunkSize = %v, want 16MB", config.Upload.ChunkSize)
	}
	if !config.Upload.EnableAdaptiveSizing {
		t.Errorf("DefaultConfig() Upload.EnableAdaptiveSizing = false, want true")
	}
	if config.Upload.CompressionType != "zstd" {
		t.Errorf("DefaultConfig() Upload.CompressionType = %v, want zstd", config.Upload.CompressionType)
	}

	// Test Metrics defaults
	if !config.Metrics.Enabled {
		t.Errorf("DefaultConfig() Metrics.Enabled = false, want true")
	}
	if config.Metrics.Namespace != "CargoShip/Production" {
		t.Errorf("DefaultConfig() Metrics.Namespace = %v, want CargoShip/Production", config.Metrics.Namespace)
	}
	if config.Metrics.BatchSize != 20 {
		t.Errorf("DefaultConfig() Metrics.BatchSize = %v, want 20", config.Metrics.BatchSize)
	}

	// Test Logging defaults
	if config.Logging.Level != "info" {
		t.Errorf("DefaultConfig() Logging.Level = %v, want info", config.Logging.Level)
	}
	if config.Logging.Structured {
		t.Errorf("DefaultConfig() Logging.Structured = true, want false")
	}
	if !config.Logging.Timestamp {
		t.Errorf("DefaultConfig() Logging.Timestamp = false, want true")
	}

	// Test Security defaults
	if config.Security.RequireEncryption {
		t.Errorf("DefaultConfig() Security.RequireEncryption = true, want false")
	}
	expectedStorageClasses := []string{
		"STANDARD", "STANDARD_IA", "ONEZONE_IA", "INTELLIGENT_TIERING", "GLACIER", "DEEP_ARCHIVE",
	}
	if !reflect.DeepEqual(config.Security.AllowedStorageClasses, expectedStorageClasses) {
		t.Errorf("DefaultConfig() Security.AllowedStorageClasses mismatch")
	}
}

func TestNewManager(t *testing.T) {
	manager := NewManager()

	if manager == nil {
		t.Fatalf("NewManager() returned nil")
	}
	if manager.config == nil {
		t.Errorf("NewManager() config is nil")
	}
	if manager.viper == nil {
		t.Errorf("NewManager() viper is nil")
	}
}

func TestManager_LoadConfig_NoFile(t *testing.T) {
	manager := NewManager()

	// Test loading with no config file (should use defaults)
	err := manager.LoadConfig("")
	if err != nil {
		t.Errorf("LoadConfig(\"\") error = %v, want nil", err)
	}

	config := manager.GetConfig()
	if config.AWS.Region != "us-west-2" {
		t.Errorf("LoadConfig with no file should use defaults")
	}
}

func TestManager_LoadConfig_WithFile(t *testing.T) {
	// Create a temporary config file
	tempDir, err := os.MkdirTemp("", "config_test")
	if err != nil {
		t.Fatalf("Failed to create temp dir: %v", err)
	}
	defer func() { _ = os.RemoveAll(tempDir) }()

	configFile := filepath.Join(tempDir, "test_config.yaml")
	configContent := `
aws:
  region: us-east-1
  max_retries: 5
storage:
  default_bucket: test-bucket
  default_storage_class: STANDARD
upload:
  max_concurrency: 16
  chunk_size: 32MB
metrics:
  enabled: false
  namespace: TestNamespace
logging:
  level: debug
  structured: true
`

	err = os.WriteFile(configFile, []byte(configContent), 0644)
	if err != nil {
		t.Fatalf("Failed to write config file: %v", err)
	}

	manager := NewManager()
	err = manager.LoadConfig(configFile)
	if err != nil {
		t.Errorf("LoadConfig() error = %v, want nil", err)
	}

	config := manager.GetConfig()
	if config.AWS.Region != "us-east-1" {
		t.Errorf("LoadConfig() AWS.Region = %v, want us-east-1", config.AWS.Region)
	}
	if config.AWS.MaxRetries != 5 {
		t.Errorf("LoadConfig() AWS.MaxRetries = %v, want 5", config.AWS.MaxRetries)
	}
	if config.Storage.DefaultBucket != "test-bucket" {
		t.Errorf("LoadConfig() Storage.DefaultBucket = %v, want test-bucket", config.Storage.DefaultBucket)
	}
	if config.Upload.MaxConcurrency != 16 {
		t.Errorf("LoadConfig() Upload.MaxConcurrency = %v, want 16", config.Upload.MaxConcurrency)
	}
	if config.Metrics.Enabled {
		t.Errorf("LoadConfig() Metrics.Enabled = true, want false")
	}
	if config.Logging.Level != "debug" {
		t.Errorf("LoadConfig() Logging.Level = %v, want debug", config.Logging.Level)
	}
}

func TestManager_LoadConfig_InvalidFile(t *testing.T) {
	// Create a temporary invalid config file
	tempDir, err := os.MkdirTemp("", "config_test")
	if err != nil {
		t.Fatalf("Failed to create temp dir: %v", err)
	}
	defer func() { _ = os.RemoveAll(tempDir) }()

	configFile := filepath.Join(tempDir, "invalid_config.yaml")
	invalidContent := `
invalid yaml content [
aws:
  region: us-east-1
  invalid_field: invalid value
`

	err = os.WriteFile(configFile, []byte(invalidContent), 0644)
	if err != nil {
		t.Fatalf("Failed to write invalid config file: %v", err)
	}

	manager := NewManager()
	err = manager.LoadConfig(configFile)
	if err == nil {
		t.Errorf("LoadConfig() with invalid file should return error")
	}
}

func TestManager_LoadConfig_ValidationErrors(t *testing.T) {
	tests := []struct {
		name         string
		configContent string
		expectedError string
	}{
		{
			name: "empty region",
			configContent: `
aws:
  region: ""
`,
			expectedError: "aws.region is required",
		},
		{
			name: "invalid storage class",
			configContent: `
storage:
  default_storage_class: INVALID_CLASS
`,
			expectedError: "invalid storage class",
		},
		{
			name: "invalid concurrency",
			configContent: `
upload:
  max_concurrency: 0
`,
			expectedError: "max_concurrency must be between 1 and 100",
		},
		{
			name: "invalid concurrency high",
			configContent: `
upload:
  max_concurrency: 200
`,
			expectedError: "max_concurrency must be between 1 and 100",
		},
		{
			name: "metrics enabled without namespace",
			configContent: `
metrics:
  enabled: true
  namespace: ""
`,
			expectedError: "metrics.namespace is required when metrics are enabled",
		},
		{
			name: "invalid log level",
			configContent: `
logging:
  level: invalid_level
`,
			expectedError: "invalid log level",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			tempDir, err := os.MkdirTemp("", "config_test")
			if err != nil {
				t.Fatalf("Failed to create temp dir: %v", err)
			}
			defer func() { _ = os.RemoveAll(tempDir) }()

			configFile := filepath.Join(tempDir, "test_config.yaml")
			err = os.WriteFile(configFile, []byte(tt.configContent), 0644)
			if err != nil {
				t.Fatalf("Failed to write config file: %v", err)
			}

			manager := NewManager()
			err = manager.LoadConfig(configFile)
			if err == nil {
				t.Errorf("LoadConfig() should return validation error")
			}
			if !strings.Contains(err.Error(), tt.expectedError) {
				t.Errorf("LoadConfig() error = %v, want error containing %v", err, tt.expectedError)
			}
		})
	}
}

func TestManager_SaveConfig(t *testing.T) {
	tempDir, err := os.MkdirTemp("", "config_test")
	if err != nil {
		t.Fatalf("Failed to create temp dir: %v", err)
	}
	defer func() { _ = os.RemoveAll(tempDir) }()

	manager := NewManager()
	manager.config.AWS.Region = "eu-west-1"
	manager.config.Storage.DefaultBucket = "my-test-bucket"

	configFile := filepath.Join(tempDir, "saved_config.yaml")
	err = manager.SaveConfig(configFile)
	if err != nil {
		t.Errorf("SaveConfig() error = %v, want nil", err)
	}

	// Verify file was created
	if _, err := os.Stat(configFile); os.IsNotExist(err) {
		t.Errorf("SaveConfig() did not create file")
	}

	// Load the saved config and verify
	newManager := NewManager()
	err = newManager.LoadConfig(configFile)
	if err != nil {
		t.Errorf("Failed to load saved config: %v", err)
	}

	savedConfig := newManager.GetConfig()
	if savedConfig.AWS.Region != "eu-west-1" {
		t.Errorf("Saved config AWS.Region = %v, want eu-west-1", savedConfig.AWS.Region)
	}
	if savedConfig.Storage.DefaultBucket != "my-test-bucket" {
		t.Errorf("Saved config Storage.DefaultBucket = %v, want my-test-bucket", savedConfig.Storage.DefaultBucket)
	}
}

func TestManager_SaveConfig_NoPath(t *testing.T) {
	manager := NewManager()

	// Should use default path
	err := manager.SaveConfig("")
	if err != nil {
		// This may fail due to permissions, but should not panic
		t.Logf("SaveConfig with no path returned error (expected): %v", err)
	}
}

func TestManager_UpdateConfig(t *testing.T) {
	manager := NewManager()

	updates := map[string]interface{}{
		"aws.region":             "ap-southeast-2",
		"upload.max_concurrency": 12,
		"metrics.enabled":        false,
	}

	err := manager.UpdateConfig(updates)
	if err != nil {
		t.Errorf("UpdateConfig() error = %v, want nil", err)
	}

	config := manager.GetConfig()
	if config.AWS.Region != "ap-southeast-2" {
		t.Errorf("UpdateConfig() AWS.Region = %v, want ap-southeast-2", config.AWS.Region)
	}
	if config.Upload.MaxConcurrency != 12 {
		t.Errorf("UpdateConfig() Upload.MaxConcurrency = %v, want 12", config.Upload.MaxConcurrency)
	}
	if config.Metrics.Enabled {
		t.Errorf("UpdateConfig() Metrics.Enabled = true, want false")
	}
}

func TestManager_UpdateConfig_Invalid(t *testing.T) {
	manager := NewManager()

	updates := map[string]interface{}{
		"aws.region": "", // Invalid - empty region
	}

	err := manager.UpdateConfig(updates)
	if err == nil {
		t.Errorf("UpdateConfig() with invalid values should return error")
	}
}

func TestManager_GetDuration(t *testing.T) {
	manager := NewManager()
	
	// Load config to populate viper with defaults
	err := manager.LoadConfig("")
	if err != nil {
		t.Fatalf("LoadConfig() error = %v", err)
	}

	// Test existing duration
	duration, err := manager.GetDuration("aws.retry_max_delay")
	if err != nil {
		t.Errorf("GetDuration() error = %v, want nil", err)
	}
	if duration != 30*time.Second {
		t.Errorf("GetDuration() = %v, want 30s", duration)
	}

	// Test non-existent key
	_, err = manager.GetDuration("non.existent.key")
	if err == nil {
		t.Errorf("GetDuration() with non-existent key should return error")
	}

	// Test invalid duration format
	manager.viper.Set("test.invalid.duration", "invalid")
	_, err = manager.GetDuration("test.invalid.duration")
	if err == nil {
		t.Errorf("GetDuration() with invalid format should return error")
	}
}

func TestManager_GetBytes(t *testing.T) {
	manager := NewManager()
	
	// Load config to populate viper with defaults
	err := manager.LoadConfig("")
	if err != nil {
		t.Fatalf("LoadConfig() error = %v", err)
	}

	// Test existing byte size
	bytes, err := manager.GetBytes("upload.chunk_size")
	if err != nil {
		t.Errorf("GetBytes() error = %v, want nil", err)
	}
	if bytes != 16*1024*1024 {
		t.Errorf("GetBytes() = %v, want %v", bytes, 16*1024*1024)
	}

	// Test non-existent key
	_, err = manager.GetBytes("non.existent.key")
	if err == nil {
		t.Errorf("GetBytes() with non-existent key should return error")
	}

	// Test invalid byte format
	manager.viper.Set("test.invalid.bytes", "invalid")
	_, err = manager.GetBytes("test.invalid.bytes")
	if err == nil {
		t.Errorf("GetBytes() with invalid format should return error")
	}
}

func TestParseBytes(t *testing.T) {
	tests := []struct {
		input    string
		expected int64
		hasError bool
	}{
		{"1B", 1, false},
		{"1KB", 1024, false},
		{"1MB", 1024 * 1024, false},
		{"1GB", 1024 * 1024 * 1024, false},
		{"1TB", 1024 * 1024 * 1024 * 1024, false},
		{"16MB", 16 * 1024 * 1024, false},
		{"2.5GB", int64(2.5 * 1024 * 1024 * 1024), false},
		{"invalid", 0, true},
		{"1XB", 0, true},
		{"", 0, true},
		{"1", 0, true},
	}

	for _, tt := range tests {
		t.Run(tt.input, func(t *testing.T) {
			result, err := parseBytes(tt.input)
			if tt.hasError {
				if err == nil {
					t.Errorf("parseBytes(%v) should return error", tt.input)
				}
			} else {
				if err != nil {
					t.Errorf("parseBytes(%v) error = %v, want nil", tt.input, err)
				}
				if result != tt.expected {
					t.Errorf("parseBytes(%v) = %v, want %v", tt.input, result, tt.expected)
				}
			}
		})
	}
}

func TestGenerateExampleConfig(t *testing.T) {
	example := GenerateExampleConfig()

	if example == "" {
		t.Errorf("GenerateExampleConfig() returned empty string")
	}

	// Check that it contains expected sections
	if !strings.Contains(example, "aws:") {
		t.Errorf("GenerateExampleConfig() missing aws section")
	}
	if !strings.Contains(example, "storage:") {
		t.Errorf("GenerateExampleConfig() missing storage section")
	}
	if !strings.Contains(example, "upload:") {
		t.Errorf("GenerateExampleConfig() missing upload section")
	}
	if !strings.Contains(example, "metrics:") {
		t.Errorf("GenerateExampleConfig() missing metrics section")
	}
	if !strings.Contains(example, "logging:") {
		t.Errorf("GenerateExampleConfig() missing logging section")
	}
	if !strings.Contains(example, "security:") {
		t.Errorf("GenerateExampleConfig() missing security section")
	}

	// Check for example values
	if !strings.Contains(example, "cargoship") {
		t.Errorf("GenerateExampleConfig() missing example profile")
	}
	if !strings.Contains(example, "my-archive-bucket") {
		t.Errorf("GenerateExampleConfig() missing example bucket")
	}
}

func TestConfigStructFields(t *testing.T) {
	config := &Config{
		AWS: AWSConfig{
			Region:          "us-west-1",
			Profile:         "test-profile",
			AccessKeyID:     "test-key",
			SecretAccessKey: "test-secret",
			SessionToken:    "test-token",
			S3Endpoint:      "https://s3.example.com",
			UsePathStyle:    true,
			MaxRetries:      5,
			RetryMaxDelay:   "60s",
			RequestTimeout:  "10m",
		},
		Storage: StorageConfig{
			DefaultBucket:      "test-bucket",
			DefaultStorageClass: "STANDARD",
			KMSKeyID:           "test-kms-key",
			SSEEncryption:      true,
			ObjectTagging:      map[string]string{"key": "value"},
			MetadataDirective:  "COPY",
		},
		Upload: UploadConfig{
			MaxConcurrency:       10,
			ChunkSize:           "32MB",
			EnableAdaptiveSizing: false,
			MaxPrefixes:         4,
			PrefixPattern:       "time",
			CompressionType:     "gzip",
			CompressionLevel:    6,
			ChecksumAlgorithm:   "MD5",
			MemoryLimit:         "1GB",
		},
		Metrics: MetricsConfig{
			Enabled:       false,
			Namespace:     "Test/Namespace",
			FlushInterval: "60s",
			BatchSize:     50,
			Region:        "us-east-1",
			DryRun:        true,
		},
		Logging: LoggingConfig{
			Level:      "debug",
			Structured: true,
			Timestamp:  false,
			Caller:     true,
			Output:     "/var/log/cargoship.log",
		},
		Security: SecurityConfig{
			RequireEncryption:     true,
			AllowedRegions:        []string{"us-west-1", "us-west-2"},
			AllowedStorageClasses: []string{"STANDARD", "GLACIER"},
			MaxFileSize:           "5GB",
			BlockedExtensions:     []string{".tmp", ".bak"},
		},
	}

	// Verify all fields are accessible and have expected values
	if config.AWS.Region != "us-west-1" {
		t.Errorf("AWS.Region field mismatch")
	}
	if config.Storage.DefaultBucket != "test-bucket" {
		t.Errorf("Storage.DefaultBucket field mismatch")
	}
	if config.Upload.MaxConcurrency != 10 {
		t.Errorf("Upload.MaxConcurrency field mismatch")
	}
	if config.Metrics.BatchSize != 50 {
		t.Errorf("Metrics.BatchSize field mismatch")
	}
	if config.Logging.Level != "debug" {
		t.Errorf("Logging.Level field mismatch")
	}
	if !config.Security.RequireEncryption {
		t.Errorf("Security.RequireEncryption field mismatch")
	}
}

func TestManager_GetConfig(t *testing.T) {
	manager := NewManager()
	config := manager.GetConfig()

	if config == nil {
		t.Errorf("GetConfig() returned nil")
	}

	// Should return the same instance
	config2 := manager.GetConfig()
	if config != config2 {
		t.Errorf("GetConfig() should return same instance")
	}
}

func TestManager_LoadConfig_HomeDirectory(t *testing.T) {
	manager := NewManager()

	// This should not panic even if home directory access fails
	// We can't easily test the actual home directory logic without complex setup
	err := manager.LoadConfig("")
	if err != nil {
		// Expected in many test environments
		t.Logf("LoadConfig with home directory failed (expected in test env): %v", err)
	}
}

func TestConfigValidation_EdgeCases(t *testing.T) {
	manager := NewManager()

	// Test with valid values at boundaries
	manager.config.Upload.MaxConcurrency = 1
	if err := manager.validateConfig(); err != nil {
		t.Errorf("validateConfig() with MaxConcurrency=1 should be valid")
	}

	manager.config.Upload.MaxConcurrency = 100
	if err := manager.validateConfig(); err != nil {
		t.Errorf("validateConfig() with MaxConcurrency=100 should be valid")
	}

	// Test all valid storage classes
	validClasses := []string{"STANDARD", "STANDARD_IA", "ONEZONE_IA", "INTELLIGENT_TIERING", "GLACIER", "DEEP_ARCHIVE"}
	for _, class := range validClasses {
		manager.config.Storage.DefaultStorageClass = class
		if err := manager.validateConfig(); err != nil {
			t.Errorf("validateConfig() with storage class %s should be valid", class)
		}
	}

	// Test all valid log levels
	validLevels := []string{"debug", "info", "warn", "error"}
	for _, level := range validLevels {
		manager.config.Logging.Level = level
		if err := manager.validateConfig(); err != nil {
			t.Errorf("validateConfig() with log level %s should be valid", level)
		}
	}
}

func TestManager_SaveConfig_CreateDirectory(t *testing.T) {
	tempDir, err := os.MkdirTemp("", "config_test")
	if err != nil {
		t.Fatalf("Failed to create temp dir: %v", err)
	}
	defer func() { _ = os.RemoveAll(tempDir) }()

	manager := NewManager()

	// Test saving to a nested directory that doesn't exist
	configFile := filepath.Join(tempDir, "nested", "dir", "config.yaml")
	err = manager.SaveConfig(configFile)
	if err != nil {
		t.Errorf("SaveConfig() should create nested directories, error = %v", err)
	}

	// Verify file exists
	if _, err := os.Stat(configFile); os.IsNotExist(err) {
		t.Errorf("SaveConfig() did not create file in nested directory")
	}
}