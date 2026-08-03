package repl

import (
	"log/slog"
	"os"
	"testing"

	"github.com/spf13/cobra"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/scttfrdmn/cargoship/pkg/context"
)

func TestNewShell(t *testing.T) {
	logger := slog.New(slog.NewTextHandler(os.Stdout, &slog.HandlerOptions{Level: slog.LevelError}))
	rootCmd := &cobra.Command{Use: "test"}

	shell := NewShell(rootCmd, logger)

	assert.NotNil(t, shell)
	assert.Equal(t, rootCmd, shell.rootCmd)
	assert.NotNil(t, shell.contextManager)
	assert.NotNil(t, shell.logger)
	assert.Equal(t, 100, shell.maxHistory)
	assert.Empty(t, shell.history)
	assert.False(t, shell.running)
}

func TestGetPrompt(t *testing.T) {
	logger := slog.New(slog.NewTextHandler(os.Stdout, &slog.HandlerOptions{Level: slog.LevelError}))
	rootCmd := &cobra.Command{Use: "test"}
	shell := NewShell(rootCmd, logger)

	tests := []struct {
		context  context.ExecutionContext
		expected string
	}{
		{context.ContextLocal, "cargoship> "},
		{context.ContextAgent, "agent> "},
		{context.ContextREPL, "repl> "},
	}

	for _, tt := range tests {
		t.Run(string(tt.context), func(t *testing.T) {
			// Switch to context
			err := shell.contextManager.SwitchTo(tt.context)
			require.NoError(t, err)

			// Check prompt
			prompt := shell.getPrompt()
			assert.Equal(t, tt.expected, prompt)
		})
	}
}

func TestHandleSpecialCommands(t *testing.T) {
	logger := slog.New(slog.NewTextHandler(os.Stdout, &slog.HandlerOptions{Level: slog.LevelError}))
	rootCmd := &cobra.Command{Use: "test"}
	shell := NewShell(rootCmd, logger)

	tests := []struct {
		input    string
		expected bool
		running  bool
	}{
		{"exit", true, false},
		{"quit", true, false},
		{"q", true, false},
		{"clear", true, true},
		{"history", true, true},
		{"help", true, true},
		{"context", true, true},
		{"regular command", false, true},
		{"", false, true},
	}

	for _, tt := range tests {
		t.Run(tt.input, func(t *testing.T) {
			shell.running = true

			result := shell.handleSpecialCommands(tt.input)
			assert.Equal(t, tt.expected, result)

			if tt.input == "exit" || tt.input == "quit" || tt.input == "q" {
				assert.Equal(t, tt.running, shell.running)
			}
		})
	}
}

func TestAddToHistory(t *testing.T) {
	logger := slog.New(slog.NewTextHandler(os.Stdout, &slog.HandlerOptions{Level: slog.LevelError}))
	rootCmd := &cobra.Command{Use: "test"}
	shell := NewShell(rootCmd, logger)

	// Add some commands
	shell.addToHistory("command1")
	shell.addToHistory("command2")
	shell.addToHistory("command3")

	assert.Len(t, shell.history, 3)
	assert.Equal(t, "command1", shell.history[0])
	assert.Equal(t, "command2", shell.history[1])
	assert.Equal(t, "command3", shell.history[2])
}

func TestAddToHistoryLimit(t *testing.T) {
	logger := slog.New(slog.NewTextHandler(os.Stdout, &slog.HandlerOptions{Level: slog.LevelError}))
	rootCmd := &cobra.Command{Use: "test"}
	shell := NewShell(rootCmd, logger)
	shell.maxHistory = 2 // Set small limit for testing

	// Add commands beyond limit
	shell.addToHistory("command1")
	shell.addToHistory("command2")
	shell.addToHistory("command3")

	assert.Len(t, shell.history, 2)
	assert.Equal(t, "command2", shell.history[0]) // First command should be removed
	assert.Equal(t, "command3", shell.history[1])
}

func TestFindCommand(t *testing.T) {
	logger := slog.New(slog.NewTextHandler(os.Stdout, &slog.HandlerOptions{Level: slog.LevelError}))

	// Create root command with subcommands
	rootCmd := &cobra.Command{Use: "test"}
	subCmd := &cobra.Command{Use: "subcmd", Short: "Test subcommand"}
	rootCmd.AddCommand(subCmd)

	shell := NewShell(rootCmd, logger)

	// Test finding existing command
	found := shell.findCommand("subcmd")
	assert.NotNil(t, found)
	assert.Equal(t, "subcmd", found.Name())

	// Test finding non-existent command
	notFound := shell.findCommand("nonexistent")
	assert.Nil(t, notFound)
}

func TestGetAvailableCommands(t *testing.T) {
	logger := slog.New(slog.NewTextHandler(os.Stdout, &slog.HandlerOptions{Level: slog.LevelError}))
	rootCmd := &cobra.Command{Use: "test"}
	shell := NewShell(rootCmd, logger)

	tests := []struct {
		context   context.ExecutionContext
		hasCreate bool
		hasConfig bool
	}{
		{context.ContextLocal, true, true},
		{context.ContextAgent, false, true},
		{context.ContextREPL, true, true},
	}

	for _, tt := range tests {
		t.Run(string(tt.context), func(t *testing.T) {
			commands := shell.getAvailableCommands(tt.context)

			assert.Equal(t, tt.hasCreate, commands["create"])
			assert.Equal(t, tt.hasConfig, commands["config"])
			assert.False(t, commands["controller"], "the controller command was removed in v0.20.0 (#340)")
		})
	}
}

func TestStop(t *testing.T) {
	logger := slog.New(slog.NewTextHandler(os.Stdout, &slog.HandlerOptions{Level: slog.LevelError}))
	rootCmd := &cobra.Command{Use: "test"}
	shell := NewShell(rootCmd, logger)

	shell.running = true
	shell.Stop()

	assert.False(t, shell.running)
}
