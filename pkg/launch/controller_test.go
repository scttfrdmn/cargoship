package launch

import (
	"log/slog"
	"os"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestNewControllerConnection(t *testing.T) {
	logger := slog.New(slog.NewTextHandler(os.Stdout, &slog.HandlerOptions{Level: slog.LevelError}))

	config := &AgentConfig{
		ID:            "test-agent",
		ControllerURL: "wss://localhost:8080",
		AuthToken:     "test-token",
	}

	conn, err := NewControllerConnection(config, logger)
	require.NoError(t, err)
	require.NotNil(t, conn)

	assert.Equal(t, config, conn.config)
	assert.NotNil(t, conn.messageCh)
	assert.NotNil(t, conn.errorCh)
	assert.False(t, conn.connected)
}

func TestControllerMessageSerialization(t *testing.T) {
	// Test message structure serialization
	msg := ControllerMessage{
		Type:      MsgTypeRegister,
		ID:        "test-123",
		Timestamp: time.Now(),
		AgentID:   "agent-456",
	}

	// Basic validation that message structure is correct
	assert.Equal(t, MsgTypeRegister, msg.Type)
	assert.Equal(t, "test-123", msg.ID)
	assert.Equal(t, "agent-456", msg.AgentID)
}

func TestRegistrationRequest(t *testing.T) {
	req := RegistrationRequest{
		AgentID:     "test-agent",
		Name:        "Test Agent",
		Description: "Test description",
		Version:     "0.3.0",
		Capabilities: []string{"file_watching", "s3_upload"},
		WatchPaths: []WatchPath{
			{
				Path:         "/data",
				Recursive:    true,
				StorageClass: "deep-archive",
			},
		},
		Metadata: map[string]string{
			"platform": "docker",
			"type":     "nas_agent",
		},
	}

	assert.Equal(t, "test-agent", req.AgentID)
	assert.Equal(t, "Test Agent", req.Name)
	assert.Len(t, req.Capabilities, 2)
	assert.Len(t, req.WatchPaths, 1)
	assert.Equal(t, "docker", req.Metadata["platform"])
}

func TestRegistrationResponse(t *testing.T) {
	resp := RegistrationResponse{
		Success:   true,
		AgentID:   "test-agent",
		SessionID: "session-123",
		Configuration: map[string]interface{}{
			"max_concurrent": 3,
			"log_level":      "info",
		},
		Message: "Registration successful",
	}

	assert.True(t, resp.Success)
	assert.Equal(t, "test-agent", resp.AgentID)
	assert.Equal(t, "session-123", resp.SessionID)
	assert.Equal(t, 3, resp.Configuration["max_concurrent"])
}

func TestJobAssignment(t *testing.T) {
	deadline := time.Now().Add(time.Hour)
	
	assignment := JobAssignment{
		JobID:        "job-789",
		Type:         "archive",
		Path:         "/data/sample.fastq.gz",
		Destination:  "s3://bucket/genomics/",
		StorageClass: "deep-archive",
		Priority:     1,
		Parameters: map[string]interface{}{
			"compression": "zstd",
			"encryption":  true,
		},
		Deadline: &deadline,
	}

	assert.Equal(t, "job-789", assignment.JobID)
	assert.Equal(t, "archive", assignment.Type)
	assert.Equal(t, 1, assignment.Priority)
	assert.NotNil(t, assignment.Deadline)
	assert.Equal(t, true, assignment.Parameters["encryption"])
}

func TestStatusUpdate(t *testing.T) {
	status := StatusUpdate{
		State:         AgentStateReady,
		ActiveJobs:    2,
		CompletedJobs: 15,
		FailedJobs:    1,
		BytesArchived: 1024 * 1024 * 1024 * 50, // 50GB
		Uptime:        time.Hour * 24,
		SystemInfo: SystemInfo{
			Platform:     "linux/amd64",
			Architecture: "amd64",
			CPUUsage:     25.5,
			MemoryUsage:  512.0,
			DiskUsage:    75.2,
			NetworkRx:    1024 * 1024 * 100, // 100MB
			NetworkTx:    1024 * 1024 * 50,  // 50MB
		},
	}

	assert.Equal(t, AgentStateReady, status.State)
	assert.Equal(t, 2, status.ActiveJobs)
	assert.Equal(t, int64(15), status.CompletedJobs)
	assert.Equal(t, "linux/amd64", status.SystemInfo.Platform)
	assert.Equal(t, 25.5, status.SystemInfo.CPUUsage)
}

func TestBuildTLSConfig(t *testing.T) {
	logger := slog.New(slog.NewTextHandler(os.Stdout, &slog.HandlerOptions{Level: slog.LevelError}))

	tests := []struct {
		name      string
		tlsConfig *TLSConfig
		wantNil   bool
	}{
		{
			name:      "nil TLS config",
			tlsConfig: nil,
			wantNil:   true,
		},
		{
			name: "disabled TLS",
			tlsConfig: &TLSConfig{
				Enabled: false,
			},
			wantNil: true,
		},
		{
			name: "enabled TLS with insecure skip verify",
			tlsConfig: &TLSConfig{
				Enabled:            true,
				InsecureSkipVerify: true,
			},
			wantNil: false,
		},
		{
			name: "enabled TLS secure",
			tlsConfig: &TLSConfig{
				Enabled:            true,
				InsecureSkipVerify: false,
			},
			wantNil: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			config := &AgentConfig{
				TLSConfig: tt.tlsConfig,
			}

			conn := &ControllerConnection{
				config: config,
				logger: logger,
			}

			tlsConfig := conn.buildTLSConfig()

			if tt.wantNil {
				assert.Nil(t, tlsConfig)
			} else {
				assert.NotNil(t, tlsConfig)
				assert.Equal(t, tt.tlsConfig.InsecureSkipVerify, tlsConfig.InsecureSkipVerify)
			}
		})
	}
}

func TestGenerateMessageID(t *testing.T) {
	id1 := generateMessageID()
	time.Sleep(time.Millisecond) // Ensure different timestamps
	id2 := generateMessageID()

	assert.NotEmpty(t, id1)
	assert.NotEmpty(t, id2)
	assert.NotEqual(t, id1, id2) // Should be unique
}

func TestMessageTypes(t *testing.T) {
	// Test that all message types are properly defined
	agentToController := []MessageType{
		MsgTypeRegister,
		MsgTypeHeartbeat,
		MsgTypeStatusUpdate,
		MsgTypeJobProgress,
		MsgTypeJobComplete,
		MsgTypeJobFailed,
		MsgTypeLogStream,
	}

	controllerToAgent := []MessageType{
		MsgTypeRegistered,
		MsgTypeJobAssign,
		MsgTypeJobCancel,
		MsgTypeConfigUpdate,
		MsgTypeShutdown,
		MsgTypePing,
	}

	// Verify all message types are non-empty strings
	for _, msgType := range agentToController {
		assert.NotEmpty(t, string(msgType))
	}

	for _, msgType := range controllerToAgent {
		assert.NotEmpty(t, string(msgType))
	}

	// Verify some specific values
	assert.Equal(t, "register", string(MsgTypeRegister))
	assert.Equal(t, "registered", string(MsgTypeRegistered))
	assert.Equal(t, "job_assign", string(MsgTypeJobAssign))
}