package launch

import (
	"log/slog"
	"os"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// validAgentConfig returns the minimum config NewAgent accepts. Since #340
// removed the controller connection, that minimum is an ID and one watch path —
// no controller URL and no auth token.
func validAgentConfig() *AgentConfig {
	return &AgentConfig{
		ID:   "test-agent",
		Name: "Test Agent",
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
}

func TestNewAgent(t *testing.T) {
	logger := slog.New(slog.NewTextHandler(os.Stdout, &slog.HandlerOptions{Level: slog.LevelError}))

	noWatchPaths := validAgentConfig()
	noWatchPaths.WatchPaths = nil

	noID := validAgentConfig()
	noID.ID = ""

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
			name:    "missing ID",
			config:  noID,
			wantErr: true,
		},
		{
			name:    "missing watch paths",
			config:  noWatchPaths,
			wantErr: true,
		},
		{
			name:    "valid config needs no controller URL or auth token",
			config:  validAgentConfig(),
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

	agent, err := NewAgent(validAgentConfig(), logger)
	require.NoError(t, err)
	require.NotNil(t, agent)

	// Test GetStatus
	status := agent.GetStatus()
	assert.Equal(t, AgentStateStarting, status.State)
	assert.Equal(t, "0.3.0", status.Version)

	// Test GetJobs (should be empty initially)
	jobs := agent.GetJobs()
	assert.Empty(t, jobs)
}

// TestAgentStartReachesReady covers what was previously untestable: before #340
// Start() dialed a controller, so the old test noted it "can't easily test
// Start() without a real controller". With no controller to wait on, the agent
// is operational the moment its watcher and job processor are running.
func TestAgentStartReachesReady(t *testing.T) {
	logger := slog.New(slog.NewTextHandler(os.Stdout, &slog.HandlerOptions{Level: slog.LevelError}))

	config := validAgentConfig()
	config.HealthCheck = HealthConfig{Enabled: true, ReportInterval: time.Hour}

	agent, err := NewAgent(config, logger)
	require.NoError(t, err)

	require.NoError(t, agent.Start())
	assert.Equal(t, AgentStateReady, agent.GetStatus().State)

	// Starting twice must be refused — Start() requires the starting state.
	assert.Error(t, agent.Start())

	require.NoError(t, agent.Stop())
	assert.Equal(t, AgentStateStopping, agent.GetStatus().State)
}

func TestAgentUpdateStatus(t *testing.T) {
	logger := slog.New(slog.NewTextHandler(os.Stdout, &slog.HandlerOptions{Level: slog.LevelError}))

	agent, err := NewAgent(validAgentConfig(), logger)
	require.NoError(t, err)

	agent.updateStatus(AgentStateError, "disk full")
	status := agent.GetStatus()
	assert.Equal(t, AgentStateError, status.State)
	assert.Equal(t, "disk full", status.LastError)

	// An empty message must not clear the previous error.
	agent.updateStatus(AgentStateWorking, "")
	status = agent.GetStatus()
	assert.Equal(t, AgentStateWorking, status.State)
	assert.Equal(t, "disk full", status.LastError)
}

func TestAgentGetJobsReturnsCopies(t *testing.T) {
	logger := slog.New(slog.NewTextHandler(os.Stdout, &slog.HandlerOptions{Level: slog.LevelError}))

	agent, err := NewAgent(validAgentConfig(), logger)
	require.NoError(t, err)

	agent.jobs["job-1"] = &ArchiveJob{ID: "job-1", State: JobStatePending}

	jobs := agent.GetJobs()
	require.Contains(t, jobs, "job-1")
	jobs["job-1"].State = JobStateFailed

	assert.Equal(t, JobStatePending, agent.jobs["job-1"].State,
		"GetJobs must hand out copies, not pointers into the agent's map")
	assert.Equal(t, 1, agent.GetStatus().ActiveJobs)
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
				WatchPaths: []WatchPath{{Path: "/tmp"}},
			},
			wantErr: true,
		},
		{
			name: "missing watch paths",
			config: &AgentConfig{
				ID:         "test",
				WatchPaths: []WatchPath{},
			},
			wantErr: true,
		},
		{
			name: "valid config",
			config: &AgentConfig{
				ID: "test",
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
