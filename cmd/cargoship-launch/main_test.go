package main

import (
	"os"
	"testing"
	"time"

	"github.com/scttfrdmn/cargoship/pkg/launch"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestLoadConfigFromEnv(t *testing.T) {
	// Set up test environment variables
	envVars := map[string]string{
		"CARGOSHIP_AGENT_ID":          "test-agent-123",
		"CARGOSHIP_AGENT_NAME":        "Test NAS Agent",
		"CARGOSHIP_CONTROLLER_URL":    "wss://test-controller:8080",
		"CARGOSHIP_AUTH_TOKEN":        "test-auth-token",
		"CARGOSHIP_WATCH_PATHS":       "/data/test1,/data/test2",
		"CARGOSHIP_DESTINATION":       "s3://test-bucket",
		"CARGOSHIP_STORAGE_CLASS":     "glacier",
		"CARGOSHIP_COMPRESSION":       "zstd",
		"CARGOSHIP_PATTERNS":          "*.fastq.gz,*.bam",
		"CARGOSHIP_EXCLUDE_PATTERNS":  "*.tmp,*.lock",
		"CARGOSHIP_CHECK_INTERVAL":    "600",
		"CARGOSHIP_MIN_AGE_DAYS":      "14",
	}

	// Set environment variables
	for key, value := range envVars {
		err := os.Setenv(key, value)
		require.NoError(t, err)
		defer func(k string) {
			_ = os.Unsetenv(k)
		}(key)
	}

	// Create a config and load from environment
	config := &launch.AgentConfig{}
	err := loadConfigFromEnv(config)
	require.NoError(t, err)

	// Verify configuration was loaded correctly
	assert.Equal(t, "test-agent-123", config.ID)
	assert.Equal(t, "Test NAS Agent", config.Name)
	assert.Equal(t, "wss://test-controller:8080", config.ControllerURL)
	assert.Equal(t, "test-auth-token", config.AuthToken)
	assert.Equal(t, "s3://test-bucket", config.Archive.Destination)
	assert.Equal(t, "glacier", config.Archive.StorageClass)
	assert.Equal(t, "zstd", config.Archive.Compression)

	// Check watch paths
	require.Len(t, config.WatchPaths, 2)
	assert.Equal(t, "/data/test1", config.WatchPaths[0].Path)
	assert.Equal(t, "/data/test2", config.WatchPaths[1].Path)
	assert.True(t, config.WatchPaths[0].Recursive)
	assert.Equal(t, 7*24*time.Hour, config.WatchPaths[0].MinAge) // Default from function

	// Check scan interval
	assert.Equal(t, 600*time.Second, config.ScanInterval)
}

func TestSetConfigDefaults(t *testing.T) {
	config := &launch.AgentConfig{
		ID: "test-agent",
	}

	setConfigDefaults(config)

	// Check that defaults were set
	assert.Equal(t, "CargoShip Agent (test-agent)", config.Name)
	assert.Equal(t, 5*time.Minute, config.ScanInterval)
	assert.Equal(t, "deep-archive", config.Archive.StorageClass)
	assert.Equal(t, "zstd", config.Archive.Compression)
	assert.Equal(t, 2, config.Archive.MaxConcurrent)
	assert.Equal(t, 3, config.Archive.RetryAttempts)
	assert.Equal(t, 30*time.Second, config.Archive.RetryDelay)
	assert.True(t, config.HealthCheck.Enabled)
	assert.Equal(t, 30*time.Second, config.HealthCheck.CheckInterval)
	assert.Equal(t, 5*time.Minute, config.HealthCheck.ReportInterval)
	assert.NotNil(t, config.TLSConfig)
	assert.True(t, config.TLSConfig.Enabled)
}

func TestValidateConfig(t *testing.T) {
	tests := []struct {
		name    string
		config  *launch.AgentConfig
		wantErr bool
	}{
		{
			name: "missing agent ID",
			config: &launch.AgentConfig{
				ControllerURL: "wss://test:8080",
				AuthToken:     "token",
			},
			wantErr: true,
		},
		{
			name: "missing controller URL",
			config: &launch.AgentConfig{
				ID:        "test",
				AuthToken: "token",
			},
			wantErr: true,
		},
		{
			name: "missing auth token",
			config: &launch.AgentConfig{
				ID:            "test",
				ControllerURL: "wss://test:8080",
			},
			wantErr: true,
		},
		{
			name: "missing watch paths",
			config: &launch.AgentConfig{
				ID:            "test",
				ControllerURL: "wss://test:8080",
				AuthToken:     "token",
			},
			wantErr: true,
		},
		{
			name: "missing archive destination",
			config: &launch.AgentConfig{
				ID:            "test",
				ControllerURL: "wss://test:8080",
				AuthToken:     "token",
				WatchPaths: []launch.WatchPath{
					{Path: "/tmp"},
				},
			},
			wantErr: true,
		},
		{
			name: "valid config",
			config: &launch.AgentConfig{
				ID:            "test",
				ControllerURL: "wss://test:8080",
				AuthToken:     "token",
				WatchPaths: []launch.WatchPath{
					{Path: "/tmp"},
				},
				Archive: launch.ArchiveConfig{
					Destination: "s3://test-bucket",
				},
			},
			wantErr: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := validateConfig(tt.config)
			if tt.wantErr {
				assert.Error(t, err)
			} else {
				assert.NoError(t, err)
			}
		})
	}
}

func TestGetVersion(t *testing.T) {
	version := getVersion()
	assert.Equal(t, "0.3.0", version)
}