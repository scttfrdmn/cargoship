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

	// Switch to repl context
	err = manager.SwitchTo(ContextREPL)
	assert.NoError(t, err)
	assert.Equal(t, ContextREPL, manager.Current())

	// Switch back to local context
	err = manager.SwitchTo(ContextLocal)
	assert.NoError(t, err)
	assert.Equal(t, ContextLocal, manager.Current())

	// Invalid context should fail
	err = manager.SwitchTo("invalid")
	assert.Error(t, err)
	// Should remain in local context
	assert.Equal(t, ContextLocal, manager.Current())
}

func TestContextPersistence(t *testing.T) {
	tempDir := t.TempDir()
	contextFile := filepath.Join(tempDir, ".cargoship-context")

	logger := slog.New(slog.NewTextHandler(os.Stdout, &slog.HandlerOptions{Level: slog.LevelError}))

	// Create first manager and switch context
	manager1 := NewManager(logger)
	manager1.contextFile = contextFile

	err := manager1.SwitchTo(ContextREPL)
	require.NoError(t, err)

	// Create second manager (simulating new CLI invocation)
	manager2 := NewManager(logger)
	manager2.contextFile = contextFile

	// Should load the same context
	ctx, err := manager2.Load()
	require.NoError(t, err)
	assert.Equal(t, ContextREPL, ctx.Current)
	assert.Equal(t, ContextREPL, manager2.Current())
}

func TestEndpointManagement(t *testing.T) {
	tempDir := t.TempDir()

	logger := slog.New(slog.NewTextHandler(os.Stdout, &slog.HandlerOptions{Level: slog.LevelError}))
	manager := NewManager(logger)
	manager.contextFile = filepath.Join(tempDir, ".cargoship-context")

	// Load initial context
	_, err := manager.Load()
	require.NoError(t, err)

	// No remaining context supports endpoints (the agent context that did was
	// removed with the v0.20.0 controller runtime).
	for _, ctx := range GetAvailableContexts() {
		require.NoError(t, manager.SwitchTo(ctx))

		err = manager.SetEndpoint("ws://agent.example.com:8080")
		assert.Error(t, err, "endpoints must not be applicable for context %s", ctx)
		assert.Empty(t, manager.GetEndpoint())
	}
}

func TestContextValidation(t *testing.T) {
	tests := []struct {
		name     string
		context  ExecutionContext
		expected bool
	}{
		{"valid local", ContextLocal, true},
		{"valid repl", ContextREPL, true},
		{"removed agent", "agent", false},
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
	err := manager.SwitchTo(ContextREPL)
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
		{ContextREPL, "Interactive shell mode with command discovery"},
		{"unknown", "Unknown context"},
	}

	for _, tt := range tests {
		result := FormatContext(tt.context)
		assert.Equal(t, tt.expected, result)
	}
}
