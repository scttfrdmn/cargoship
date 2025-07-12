package controller

import (
	"fmt"
	"log/slog"
	"os"
	"testing"
	"time"

	"github.com/scttfrdmn/cargoship/pkg/launch"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestNewAgentRegistry(t *testing.T) {
	logger := slog.New(slog.NewTextHandler(os.Stdout, &slog.HandlerOptions{Level: slog.LevelError}))
	authToken := "test-token"

	registry := NewAgentRegistry(authToken, logger)

	assert.NotNil(t, registry)
	assert.Equal(t, authToken, registry.authToken)
	assert.NotNil(t, registry.agents)
	assert.NotNil(t, registry.logger)
}

func TestAgentRegistryLifecycle(t *testing.T) {
	logger := slog.New(slog.NewTextHandler(os.Stdout, &slog.HandlerOptions{Level: slog.LevelError}))
	registry := NewAgentRegistry("test-token", logger)

	// Test start
	err := registry.Start()
	assert.NoError(t, err)

	// Test stop
	err = registry.Stop()
	assert.NoError(t, err)
}

func TestRegisterAgent(t *testing.T) {
	logger := slog.New(slog.NewTextHandler(os.Stdout, &slog.HandlerOptions{Level: slog.LevelError}))
	registry := NewAgentRegistry("test-token", logger)
	defer func() { _ = registry.Stop() }()

	// Create mock connection
	conn := &mockAgentConnection{
		messages: make([]mockMessage, 0),
	}

	// Create registration request
	req := &launch.RegistrationRequest{
		AgentID:     "test-agent-123",
		Name:        "Test Agent",
		Description: "Test Description",
		Version:     "0.3.0",
		Capabilities: []string{"file_watching", "s3_upload"},
		WatchPaths: []launch.WatchPath{
			{
				Path:         "/test/data",
				Recursive:    true,
				StorageClass: "deep-archive",
			},
		},
		Metadata: map[string]string{
			"platform": "test",
		},
	}

	// Register agent
	resp, err := registry.RegisterAgent(conn, req)
	require.NoError(t, err)
	require.NotNil(t, resp)

	// Verify response
	assert.True(t, resp.Success)
	assert.Equal(t, req.AgentID, resp.AgentID)
	assert.NotEmpty(t, resp.SessionID)
	assert.Equal(t, "Registration successful", resp.Message)

	// Verify agent is stored
	agent, exists := registry.GetAgent(req.AgentID)
	assert.True(t, exists)
	assert.Equal(t, req.AgentID, agent.ID)
	assert.Equal(t, req.Name, agent.Name)
	assert.Equal(t, req.Version, agent.Version)
	assert.Equal(t, launch.AgentStateReady, agent.Status.State)
}

func TestGetAllAgents(t *testing.T) {
	logger := slog.New(slog.NewTextHandler(os.Stdout, &slog.HandlerOptions{Level: slog.LevelError}))
	registry := NewAgentRegistry("test-token", logger)
	defer func() { _ = registry.Stop() }()

	// Initially empty
	agents := registry.GetAllAgents()
	assert.Empty(t, agents)

	// Register an agent
	conn := &mockAgentConnection{
		messages: make([]mockMessage, 0),
	}
	req := &launch.RegistrationRequest{
		AgentID: "test-agent-1",
		Name:    "Test Agent 1",
		Version: "0.3.0",
		Capabilities: []string{"file_watching"},
		WatchPaths: []launch.WatchPath{
			{Path: "/test"},
		},
	}

	_, err := registry.RegisterAgent(conn, req)
	require.NoError(t, err)

	// Check agents list
	agents = registry.GetAllAgents()
	assert.Len(t, agents, 1)
	assert.Equal(t, "test-agent-1", agents[0].ID)
}

func TestUpdateAgentStatus(t *testing.T) {
	logger := slog.New(slog.NewTextHandler(os.Stdout, &slog.HandlerOptions{Level: slog.LevelError}))
	registry := NewAgentRegistry("test-token", logger)
	defer func() { _ = registry.Stop() }()

	// Register an agent first
	conn := &mockAgentConnection{
		messages: make([]mockMessage, 0),
	}
	req := &launch.RegistrationRequest{
		AgentID: "test-agent-1",
		Name:    "Test Agent 1",
		Version: "0.3.0",
		Capabilities: []string{"file_watching"},
		WatchPaths: []launch.WatchPath{
			{Path: "/test"},
		},
	}

	_, err := registry.RegisterAgent(conn, req)
	require.NoError(t, err)

	// Update status
	statusUpdate := &launch.StatusUpdate{
		State:         launch.AgentStateWorking,
		ActiveJobs:    2,
		CompletedJobs: 10,
		FailedJobs:    1,
		BytesArchived: 1024 * 1024 * 100, // 100MB
		Uptime:        time.Hour,
	}

	err = registry.UpdateAgentStatus("test-agent-1", statusUpdate)
	assert.NoError(t, err)

	// Verify status was updated
	agent, exists := registry.GetAgent("test-agent-1")
	assert.True(t, exists)
	assert.Equal(t, launch.AgentStateWorking, agent.Status.State)
	assert.Equal(t, 2, agent.Status.ActiveJobs)
	assert.Equal(t, int64(10), agent.Status.CompletedJobs)
	assert.Equal(t, int64(1), agent.Status.FailedJobs)
	assert.Equal(t, int64(1024*1024*100), agent.Status.BytesArchived)
}

func TestAssignJob(t *testing.T) {
	logger := slog.New(slog.NewTextHandler(os.Stdout, &slog.HandlerOptions{Level: slog.LevelError}))
	registry := NewAgentRegistry("test-token", logger)
	defer func() { _ = registry.Stop() }()

	// Create mock connection
	conn := &mockAgentConnection{
		messages: make([]mockMessage, 0),
	}

	// Register an agent
	req := &launch.RegistrationRequest{
		AgentID: "test-agent-1",
		Name:    "Test Agent 1",
		Version: "0.3.0",
		Capabilities: []string{"file_watching"},
		WatchPaths: []launch.WatchPath{
			{Path: "/test"},
		},
	}

	_, err := registry.RegisterAgent(conn, req)
	require.NoError(t, err)

	// Assign a job
	jobAssignment := &launch.JobAssignment{
		JobID:        "job-123",
		Type:         "archive",
		Path:         "/test/data/sample.fastq.gz",
		Destination:  "s3://bucket/genomics/",
		StorageClass: "deep-archive",
		Priority:     1,
	}

	err = registry.AssignJob("test-agent-1", jobAssignment)
	assert.NoError(t, err)

	// Verify job was assigned
	agent, exists := registry.GetAgent("test-agent-1")
	assert.True(t, exists)
	assert.Len(t, agent.Jobs, 1)

	job, exists := agent.Jobs["job-123"]
	assert.True(t, exists)
	assert.Equal(t, "job-123", job.ID)
	assert.Equal(t, "archive", job.Type)
	assert.Equal(t, JobStatusAssigned, job.Status)

	// Verify message was sent to agent
	assert.Len(t, conn.messages, 1)
	assert.Equal(t, launch.MsgTypeJobAssign, conn.messages[0].msgType)
}

func TestAgentNotFound(t *testing.T) {
	logger := slog.New(slog.NewTextHandler(os.Stdout, &slog.HandlerOptions{Level: slog.LevelError}))
	registry := NewAgentRegistry("test-token", logger)
	defer func() { _ = registry.Stop() }()

	// Try to get non-existent agent
	_, exists := registry.GetAgent("non-existent")
	assert.False(t, exists)

	// Try to update status of non-existent agent
	err := registry.UpdateAgentStatus("non-existent", &launch.StatusUpdate{})
	assert.Equal(t, ErrAgentNotFound, err)

	// Try to assign job to non-existent agent
	err = registry.AssignJob("non-existent", &launch.JobAssignment{})
	assert.Equal(t, ErrAgentNotFound, err)
}

// mockAgentConnection implements AgentConnection for testing
type mockAgentConnection struct {
	messages []mockMessage
	closed   bool
}

type mockMessage struct {
	msgType launch.MessageType
	data    interface{}
}

func (m *mockAgentConnection) SetAgent(agent *ConnectedAgent) {}

func (m *mockAgentConnection) OnClose(callback func()) {}

func (m *mockAgentConnection) SendMessage(msgType launch.MessageType, data interface{}) error {
	if m.closed {
		return fmt.Errorf("connection closed")
	}
	m.messages = append(m.messages, mockMessage{
		msgType: msgType,
		data:    data,
	})
	return nil
}

func (m *mockAgentConnection) Close() error {
	m.closed = true
	return nil
}