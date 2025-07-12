package launch

import (
	"log/slog"
	"os"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestNewAgent(t *testing.T) {
	logger := slog.New(slog.NewTextHandler(os.Stdout, &slog.HandlerOptions{Level: slog.LevelError}))

	tests := []struct {
		name    string
		config  *AgentConfig
		wantErr bool
	}{
		{
			name:    "nil config",
			config:  nil,
			wantErr: true,
		},
		{
			name: "missing required fields",
			config: &AgentConfig{
				ID: "test-agent",
				// Missing ControllerURL, AuthToken
			},
			wantErr: true,
		},
		{
			name: "valid config",
			config: &AgentConfig{
				ID:            "test-agent",
				Name:          "Test Agent",
				ControllerURL: "wss://localhost:8080",
				AuthToken:     "test-token",
				WatchPaths: []WatchPath{
					{
						Path:      "/tmp",
						Recursive: true,
						MinAge:    time.Hour,
					},
				},
				Archive: ArchiveConfig{
					Destination:   "s3://test-bucket",
					StorageClass:  "standard",
					MaxConcurrent: 1,
				},
				ScanInterval: 5 * time.Minute,
			},
			wantErr: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			agent, err := NewAgent(tt.config, logger)
			if tt.wantErr {
				assert.Error(t, err)
				assert.Nil(t, agent)
			} else {
				assert.NoError(t, err)
				assert.NotNil(t, agent)
				assert.Equal(t, tt.config.ID, agent.id)
				assert.Equal(t, AgentStateStarting, agent.status.State)
			}
		})
	}
}

func TestAgentLifecycle(t *testing.T) {
	logger := slog.New(slog.NewTextHandler(os.Stdout, &slog.HandlerOptions{Level: slog.LevelError}))

	config := &AgentConfig{
		ID:            "test-agent",
		Name:          "Test Agent",
		ControllerURL: "wss://localhost:8080",
		AuthToken:     "test-token",
		WatchPaths: []WatchPath{
			{
				Path:      "/tmp",
				Recursive: true,
				MinAge:    time.Hour,
			},
		},
		Archive: ArchiveConfig{
			Destination:   "s3://test-bucket",
			StorageClass:  "standard",
			MaxConcurrent: 1,
		},
		ScanInterval: 5 * time.Minute,
	}

	agent, err := NewAgent(config, logger)
	require.NoError(t, err)
	require.NotNil(t, agent)

	// Test GetStatus
	status := agent.GetStatus()
	assert.Equal(t, AgentStateStarting, status.State)
	assert.Equal(t, "0.3.0", status.Version)

	// Test GetJobs (should be empty initially)
	jobs := agent.GetJobs()
	assert.Empty(t, jobs)

	// Note: We can't easily test Start() without a real controller
	// This would require integration tests with mock WebSocket server
}

func TestValidateAgentConfig(t *testing.T) {
	tests := []struct {
		name    string
		config  *AgentConfig
		wantErr bool
	}{
		{
			name: "missing ID",
			config: &AgentConfig{
				ControllerURL: "wss://localhost:8080",
				AuthToken:     "token",
			},
			wantErr: true,
		},
		{
			name: "missing controller URL",
			config: &AgentConfig{
				ID:        "test",
				AuthToken: "token",
			},
			wantErr: true,
		},
		{
			name: "missing auth token",
			config: &AgentConfig{
				ID:            "test",
				ControllerURL: "wss://localhost:8080",
			},
			wantErr: true,
		},
		{
			name: "missing watch paths",
			config: &AgentConfig{
				ID:            "test",
				ControllerURL: "wss://localhost:8080",
				AuthToken:     "token",
				WatchPaths:    []WatchPath{},
			},
			wantErr: true,
		},
		{
			name: "valid config",
			config: &AgentConfig{
				ID:            "test",
				ControllerURL: "wss://localhost:8080",
				AuthToken:     "token",
				WatchPaths: []WatchPath{
					{Path: "/tmp"},
				},
			},
			wantErr: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := validateAgentConfig(tt.config)
			if tt.wantErr {
				assert.Error(t, err)
			} else {
				assert.NoError(t, err)
				// Validate that defaults are set
				if tt.config.ScanInterval <= 0 {
					assert.Equal(t, 5*time.Minute, tt.config.ScanInterval)
				}
			}
		})
	}
}

func TestGetAgentVersion(t *testing.T) {
	version := getAgentVersion()
	assert.Equal(t, "0.3.0", version)
}