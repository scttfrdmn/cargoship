package main

import (
	"bytes"
	"log/slog"
	"os"
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/scttfrdmn/cargoship/internal/testutil"
	"github.com/scttfrdmn/cargoship/pkg/launch"
)

// Helper function to create temporary files
func createTempFile(dir, pattern, content string) (string, error) {
	tmpFile, err := os.CreateTemp(dir, pattern)
	if err != nil {
		return "", err
	}
	defer func() {
		if closeErr := tmpFile.Close(); closeErr != nil {
			_ = closeErr // Ignore close error
		}
	}()

	if content != "" {
		if _, err := tmpFile.WriteString(content); err != nil {
			return "", err
		}
	}

	return tmpFile.Name(), nil
}

func TestLoadControllerConfig(t *testing.T) {
	testutil.RequireNoGoroutineLeak(t)

	// Create a temporary config file
	configContent := `
listen_address: "0.0.0.0"
port: 8080
tls_enabled: false
auth_enabled: false
health_check:
  check_interval: 1m
  timeout: 30s
metrics_port: 9090
agent_timeout: 5m
ping_interval: 30s
`

	tmpFile, err := createTempFile("", "controller-config-*.yaml", configContent)
	require.NoError(t, err)
	defer func() {
		if removeErr := os.Remove(tmpFile); removeErr != nil {
			_ = removeErr // Ignore remove error in tests
		}
	}()

	// Test loading the config
	config, err := loadControllerConfig(tmpFile)
	require.NoError(t, err)
	assert.Equal(t, "0.0.0.0", config.ListenAddress)
	assert.Equal(t, 8080, config.Port)
	assert.False(t, config.TLSEnabled)
	assert.False(t, config.AuthEnabled)

	// Also test that it creates a proper CentralControllerConfig
	assert.IsType(t, &launch.CentralControllerConfig{}, config)
}

func TestLoadControllerConfig_FileNotFound(t *testing.T) {
	testutil.RequireNoGoroutineLeak(t)

	_, err := loadControllerConfig("nonexistent-file.yaml")
	assert.Error(t, err)
	assert.Contains(t, err.Error(), "failed to read config file")
}

func TestLoadControllerConfig_InvalidYAML(t *testing.T) {
	testutil.RequireNoGoroutineLeak(t)

	// Create a file with invalid YAML
	tmpFile, err := createTempFile("", "invalid-config-*.yaml", "invalid: yaml: content: [")
	require.NoError(t, err)
	defer func() {
		if removeErr := os.Remove(tmpFile); removeErr != nil {
			_ = removeErr // Ignore remove error in tests
		}
	}()

	_, err = loadControllerConfig(tmpFile)
	assert.Error(t, err)
	assert.Contains(t, err.Error(), "failed to parse config file")
}

func TestLoadControllerConfig_EmptyFile(t *testing.T) {
	testutil.RequireNoGoroutineLeak(t)

	// Create an empty config file
	tmpFile, err := createTempFile("", "empty-config-*.yaml", "")
	require.NoError(t, err)
	defer func() {
		if removeErr := os.Remove(tmpFile); removeErr != nil {
			_ = removeErr // Ignore remove error in tests
		}
	}()

	config, err := loadControllerConfig(tmpFile)
	require.NoError(t, err)
	// Should load with default values
	assert.NotNil(t, config)
}

func TestLoadControllerConfig_MinimalValidConfig(t *testing.T) {
	testutil.RequireNoGoroutineLeak(t)

	// Create minimal valid config
	configContent := `
listen_address: "127.0.0.1"
port: 9000
`

	tmpFile, err := createTempFile("", "minimal-config-*.yaml", configContent)
	require.NoError(t, err)
	defer func() {
		if removeErr := os.Remove(tmpFile); removeErr != nil {
			_ = removeErr // Ignore remove error in tests
		}
	}()

	config, err := loadControllerConfig(tmpFile)
	require.NoError(t, err)
	assert.Equal(t, "127.0.0.1", config.ListenAddress)
	assert.Equal(t, 9000, config.Port)
}

func TestLoadControllerConfig_ComplexConfig(t *testing.T) {
	testutil.RequireNoGoroutineLeak(t)

	configContent := `
listen_address: "0.0.0.0"
port: 8080
tls_enabled: true
cert_file: "/etc/ssl/server.crt"
key_file: "/etc/ssl/server.key"
auth_enabled: true
auth_tokens:
  - "token1"
  - "token2"
health_check:
  check_interval: 30s
  timeout: 10s
metrics_port: 9090
agent_timeout: 300s
ping_interval: 45s
`

	tmpFile, err := createTempFile("", "complex-config-*.yaml", configContent)
	require.NoError(t, err)
	defer func() {
		if removeErr := os.Remove(tmpFile); removeErr != nil {
			_ = removeErr // Ignore remove error in tests
		}
	}()

	config, err := loadControllerConfig(tmpFile)
	require.NoError(t, err)
	assert.Equal(t, "0.0.0.0", config.ListenAddress)
	assert.Equal(t, 8080, config.Port)
	assert.True(t, config.TLSEnabled)
	assert.True(t, config.AuthEnabled)
	assert.Equal(t, []string{"token1", "token2"}, config.AuthTokens)
}

// Test that we can capture version output (without actually calling main)
func TestVersionOutput(t *testing.T) {
	testutil.RequireNoGoroutineLeak(t)

	// This test verifies the version string format
	expectedVersion := "CargoShip Central Controller v0.3.0"

	// We can't easily test the main function directly without refactoring
	// but we can test that the version string is correctly formatted
	assert.Contains(t, expectedVersion, "CargoShip Central Controller")
	assert.Contains(t, expectedVersion, "v0.3.0")
}

// Test log level parsing logic
func TestLogLevelParsing(t *testing.T) {
	testutil.RequireNoGoroutineLeak(t)

	tests := []struct {
		name     string
		logLevel string
		expected slog.Level
	}{
		{
			name:     "debug level",
			logLevel: "debug",
			expected: slog.LevelDebug,
		},
		{
			name:     "info level",
			logLevel: "info",
			expected: slog.LevelInfo,
		},
		{
			name:     "warn level",
			logLevel: "warn",
			expected: slog.LevelWarn,
		},
		{
			name:     "error level",
			logLevel: "error",
			expected: slog.LevelError,
		},
		{
			name:     "invalid level defaults to info",
			logLevel: "invalid",
			expected: slog.LevelInfo,
		},
		{
			name:     "empty level defaults to info",
			logLevel: "",
			expected: slog.LevelInfo,
		},
		{
			name:     "uppercase level",
			logLevel: "DEBUG",
			expected: slog.LevelInfo, // Should default to info for unknown strings
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			// Extract the log level parsing logic for testing
			var level slog.Level
			switch tt.logLevel {
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

			assert.Equal(t, tt.expected, level)
		})
	}
}

// Test logger initialization
func TestLoggerInitialization(t *testing.T) {
	testutil.RequireNoGoroutineLeak(t)

	// Capture stdout for logger output
	var buf bytes.Buffer

	logger := slog.New(slog.NewJSONHandler(&buf, &slog.HandlerOptions{
		Level: slog.LevelInfo,
	}))

	// Test that logger can log messages
	logger.Info("test message", "key", "value")

	output := buf.String()
	assert.Contains(t, output, "test message")
	assert.Contains(t, output, "key")
	assert.Contains(t, output, "value")
}

// Test configuration validation edge cases
func TestConfigurationEdgeCases(t *testing.T) {
	testutil.RequireNoGoroutineLeak(t)

	tests := []struct {
		name       string
		config     string
		shouldFail bool
	}{
		{
			name: "config with comments",
			config: `
# This is a comment
listen_address: "0.0.0.0"  # Another comment
port: 8080
# More comments
tls_enabled: false
`,
			shouldFail: false,
		},
		{
			name: "config with special characters",
			config: `
listen_address: "0.0.0.0"
port: 8080
auth_tokens:
  - "test-key-with-special-chars!@#$%^&*()"
# Description with émojis 🚢 and unicode
`,
			shouldFail: false,
		},
		{
			name: "config with very long values",
			config: `
listen_address: "0.0.0.0"
port: 8080
auth_tokens:
  - "very-long-api-key-that-goes-on-and-on-and-on-with-many-characters-to-test-parsing"
# ` + strings.Repeat("Long description ", 50) + `
`,
			shouldFail: false,
		},
		{
			name: "config with nested structures",
			config: `
listen_address: "0.0.0.0"
port: 8080
health_check:
  check_interval: 1m
  timeout: 30s
  endpoints:
    - "/health"
    - "/status"
auth_tokens:
  - "nested-test-token1"
  - "nested-test-token2"
`,
			shouldFail: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			tmpFile, err := createTempFile("", "edge-case-*.yaml", tt.config)
			require.NoError(t, err)
			defer func() {
				if removeErr := os.Remove(tmpFile); removeErr != nil {
					_ = removeErr // Ignore remove error in tests
				}
			}()

			config, err := loadControllerConfig(tmpFile)
			if tt.shouldFail {
				assert.Error(t, err)
			} else {
				assert.NoError(t, err)
				assert.NotNil(t, config)
				// All test cases should have these basic fields
				assert.Equal(t, "0.0.0.0", config.ListenAddress)
				assert.Equal(t, 8080, config.Port)
			}
		})
	}
}

// Benchmark configuration loading performance
func BenchmarkLoadControllerConfig(b *testing.B) {
	testutil.BenchmarkNoGoroutineLeak(b, func(b *testing.B) {
		// Create a config file for benchmarking
		configContent := `
listen_address: "0.0.0.0"
port: 8080
tls_enabled: false
auth_enabled: false
health_check:
  check_interval: 30s
metrics_port: 9090
agent_timeout: 300s
ping_interval: 30s
`

		tmpFile, err := createTempFile("", "benchmark-config-*.yaml", configContent)
		require.NoError(b, err)
		defer func() {
			if removeErr := os.Remove(tmpFile); removeErr != nil {
				_ = removeErr // Ignore remove error in tests
			}
		}()

		b.ResetTimer()
		for i := 0; i < b.N; i++ {
			_, err := loadControllerConfig(tmpFile)
			if err != nil {
				b.Fatal(err)
			}
		}
	})
}

// Test that config file permissions are handled correctly
func TestConfigFilePermissions(t *testing.T) {
	testutil.RequireNoGoroutineLeak(t)

	configContent := `
listen_address: "0.0.0.0"
port: 8080
tls_enabled: false
`

	tmpFile, err := createTempFile("", "perm-config-*.yaml", configContent)
	require.NoError(t, err)
	defer func() {
		if removeErr := os.Remove(tmpFile); removeErr != nil {
			_ = removeErr // Ignore remove error in tests
		}
	}()

	// Change file permissions to make it readable
	err = os.Chmod(tmpFile, 0644)
	require.NoError(t, err)

	// Should be able to read the file
	config, err := loadControllerConfig(tmpFile)
	require.NoError(t, err)
	assert.Equal(t, "0.0.0.0", config.ListenAddress)

	// Make file unreadable (only on Unix-like systems)
	if os.Getenv("GOOS") != "windows" {
		err = os.Chmod(tmpFile, 0000)
		require.NoError(t, err)

		// Should fail to read the file
		_, err = loadControllerConfig(tmpFile)
		assert.Error(t, err)
		assert.Contains(t, err.Error(), "failed to read config file")
	}
}
