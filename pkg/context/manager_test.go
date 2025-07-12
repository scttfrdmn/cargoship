package context

import (
	"encoding/json"
	"log/slog"
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestNewManager(t *testing.T) {
	logger := slog.New(slog.NewTextHandler(os.Stdout, &slog.HandlerOptions{Level: slog.LevelError}))
	manager := NewManager(logger)
	
	assert.NotNil(t, manager)
	assert.NotEmpty(t, manager.contextFile)
	assert.Contains(t, manager.contextFile, ".cargoship-context")
	assert.NotNil(t, manager.logger)
}

func TestFirstRunBehavior(t *testing.T) {
	// Create temporary directory for test
	tempDir := t.TempDir()
	
	logger := slog.New(slog.NewTextHandler(os.Stdout, &slog.HandlerOptions{Level: slog.LevelError}))
	manager := NewManager(logger)
	manager.contextFile = filepath.Join(tempDir, ".cargoship-context")
	
	// Should detect first run
	assert.True(t, manager.IsFirstRun())
	
	// Load should create default context
	ctx, err := manager.Load()
	require.NoError(t, err)
	assert.Equal(t, ContextLocal, ctx.Current)
	assert.NotZero(t, ctx.LastUsed)
	assert.Equal(t, "0.3.0", ctx.Version)
	
	// Should no longer be first run
	assert.False(t, manager.IsFirstRun())
	
	// Context file should exist
	_, err = os.Stat(manager.contextFile)
	assert.NoError(t, err)
}

func TestContextSwitching(t *testing.T) {
	tempDir := t.TempDir()
	
	logger := slog.New(slog.NewTextHandler(os.Stdout, &slog.HandlerOptions{Level: slog.LevelError}))
	manager := NewManager(logger)
	manager.contextFile = filepath.Join(tempDir, ".cargoship-context")
	
	// Load initial context (should be local)
	ctx, err := manager.Load()
	require.NoError(t, err)
	assert.Equal(t, ContextLocal, ctx.Current)
	
	// Switch to agent context
	err = manager.SwitchTo(ContextAgent)
	assert.NoError(t, err)
	assert.Equal(t, ContextAgent, manager.Current())
	
	// Switch to controller context
	err = manager.SwitchTo(ContextController)
	assert.NoError(t, err)
	assert.Equal(t, ContextController, manager.Current())
	
	// Invalid context should fail
	err = manager.SwitchTo("invalid")
	assert.Error(t, err)
	// Should remain in controller context
	assert.Equal(t, ContextController, manager.Current())
}

func TestContextPersistence(t *testing.T) {
	tempDir := t.TempDir()
	contextFile := filepath.Join(tempDir, ".cargoship-context")
	
	logger := slog.New(slog.NewTextHandler(os.Stdout, &slog.HandlerOptions{Level: slog.LevelError}))
	
	// Create first manager and switch context
	manager1 := NewManager(logger)
	manager1.contextFile = contextFile
	
	err := manager1.SwitchTo(ContextController)
	require.NoError(t, err)
	
	// Create second manager (simulating new CLI invocation)
	manager2 := NewManager(logger)
	manager2.contextFile = contextFile
	
	// Should load the same context
	ctx, err := manager2.Load()
	require.NoError(t, err)
	assert.Equal(t, ContextController, ctx.Current)
	assert.Equal(t, ContextController, manager2.Current())
}

func TestEndpointManagement(t *testing.T) {
	tempDir := t.TempDir()
	
	logger := slog.New(slog.NewTextHandler(os.Stdout, &slog.HandlerOptions{Level: slog.LevelError}))
	manager := NewManager(logger)
	manager.contextFile = filepath.Join(tempDir, ".cargoship-context")
	
	// Load initial context
	_, err := manager.Load()
	require.NoError(t, err)
	
	// Switch to agent context and set endpoint
	err = manager.SwitchTo(ContextAgent)
	require.NoError(t, err)
	
	err = manager.SetEndpoint("ws://agent.example.com:8080")
	assert.NoError(t, err)
	assert.Equal(t, "ws://agent.example.com:8080", manager.GetEndpoint())
	
	// Switch to controller context and set endpoint
	err = manager.SwitchTo(ContextController)
	require.NoError(t, err)
	
	err = manager.SetEndpoint("ws://controller.example.com:8080")
	assert.NoError(t, err)
	assert.Equal(t, "ws://controller.example.com:8080", manager.GetEndpoint())
	
	// Local context shouldn't support endpoints
	err = manager.SwitchTo(ContextLocal)
	require.NoError(t, err)
	
	err = manager.SetEndpoint("invalid")
	assert.Error(t, err)
	assert.Empty(t, manager.GetEndpoint())
}

func TestContextValidation(t *testing.T) {
	tests := []struct {
		name     string
		context  ExecutionContext
		expected bool
	}{
		{"valid local", ContextLocal, true},
		{"valid agent", ContextAgent, true},
		{"valid controller", ContextController, true},
		{"valid repl", ContextREPL, true},
		{"invalid empty", "", false},
		{"invalid unknown", "unknown", false},
	}
	
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := isValidContext(tt.context)
			assert.Equal(t, tt.expected, result)
		})
	}
}

func TestCorruptedContextFile(t *testing.T) {
	tempDir := t.TempDir()
	contextFile := filepath.Join(tempDir, ".cargoship-context")
	
	// Create corrupted context file
	err := os.WriteFile(contextFile, []byte("invalid json"), 0644)
	require.NoError(t, err)
	
	logger := slog.New(slog.NewTextHandler(os.Stdout, &slog.HandlerOptions{Level: slog.LevelError}))
	manager := NewManager(logger)
	manager.contextFile = contextFile
	
	// Should handle gracefully and create default context
	ctx, err := manager.Load()
	assert.NoError(t, err)
	assert.Equal(t, ContextLocal, ctx.Current)
}

func TestInvalidContextInFile(t *testing.T) {
	tempDir := t.TempDir()
	contextFile := filepath.Join(tempDir, ".cargoship-context")
	
	// Create context file with invalid context
	invalidCtx := ContextInfo{
		Current:  "invalid-context",
		LastUsed: time.Now(),
		Version:  "0.3.0",
	}
	
	data, err := json.Marshal(invalidCtx)
	require.NoError(t, err)
	
	err = os.WriteFile(contextFile, data, 0644)
	require.NoError(t, err)
	
	logger := slog.New(slog.NewTextHandler(os.Stdout, &slog.HandlerOptions{Level: slog.LevelError}))
	manager := NewManager(logger)
	manager.contextFile = contextFile
	
	// Should reset to local context
	ctx, err := manager.Load()
	assert.NoError(t, err)
	assert.Equal(t, ContextLocal, ctx.Current)
}

func TestReset(t *testing.T) {
	tempDir := t.TempDir()
	
	logger := slog.New(slog.NewTextHandler(os.Stdout, &slog.HandlerOptions{Level: slog.LevelError}))
	manager := NewManager(logger)
	manager.contextFile = filepath.Join(tempDir, ".cargoship-context")
	
	// Create and switch context
	err := manager.SwitchTo(ContextController)
	require.NoError(t, err)
	
	// Verify file exists
	_, err = os.Stat(manager.contextFile)
	assert.NoError(t, err)
	
	// Reset
	err = manager.Reset()
	assert.NoError(t, err)
	
	// File should be gone
	_, err = os.Stat(manager.contextFile)
	assert.True(t, os.IsNotExist(err))
	
	// Should be first run again
	assert.True(t, manager.IsFirstRun())
}

func TestGetAvailableContexts(t *testing.T) {
	contexts := GetAvailableContexts()
	
	expected := []ExecutionContext{
		ContextLocal,
		ContextAgent,
		ContextController,
		ContextREPL,
	}
	
	assert.Equal(t, expected, contexts)
}

func TestFormatContext(t *testing.T) {
	tests := []struct {
		context  ExecutionContext
		expected string
	}{
		{ContextLocal, "Local filesystem operations and archive creation"},
		{ContextAgent, "Launch agent monitoring and management"},
		{ContextController, "Controller operations and agent coordination"},
		{ContextREPL, "Interactive shell mode with command discovery"},
		{"unknown", "Unknown context"},
	}
	
	for _, tt := range tests {
		result := FormatContext(tt.context)
		assert.Equal(t, tt.expected, result)
	}
}