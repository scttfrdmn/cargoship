// Package context provides command filtering utilities
package context

import (
	"log/slog"

	"github.com/spf13/cobra"
)

// CommandFilter manages context-aware command visibility
type CommandFilter struct {
	manager *Manager
	logger  *slog.Logger
}

// NewCommandFilter creates a new command filter
func NewCommandFilter(logger *slog.Logger) *CommandFilter {
	return &CommandFilter{
		manager: NewManager(logger),
		logger:  logger.With("component", "command-filter"),
	}
}

// ApplyContextFiltering hides commands not available in current context
func (cf *CommandFilter) ApplyContextFiltering(rootCmd *cobra.Command) {
	currentCtx := cf.manager.Current()
	availableCommands := cf.getContextCommands(currentCtx)

	cf.logger.Debug("Applying context filtering", "context", currentCtx, "total_commands", len(rootCmd.Commands()))

	hidden := 0
	for _, cmd := range rootCmd.Commands() {
		if !cf.isCommandAvailable(cmd.Name(), availableCommands) {
			cmd.Hidden = true
			hidden++
			cf.logger.Debug("Hidden command due to context", "command", cmd.Name(), "context", currentCtx)
		} else {
			cmd.Hidden = false // Ensure command is visible
		}
	}

	cf.logger.Debug("Context filtering applied", "context", currentCtx, "hidden_commands", hidden)
}

// GetAvailableCommands returns a list of commands available in current context
func (cf *CommandFilter) GetAvailableCommands() []string {
	currentCtx := cf.manager.Current()
	availableCommands := cf.getContextCommands(currentCtx)

	var commands []string
	for cmd := range availableCommands {
		commands = append(commands, cmd)
	}

	return commands
}

// GetContextDescription returns a description of what commands are available
func (cf *CommandFilter) GetContextDescription(ctx ExecutionContext) string {
	switch ctx {
	case ContextLocal:
		return "Full CargoShip functionality: archive creation, analysis, configuration, and infrastructure management"

	case ContextAgent:
		return "Agent operations: configuration management, monitoring, and status reporting"

	case ContextREPL:
		return "Interactive mode: all commands available with enhanced discovery and help"

	default:
		return "Standard CargoShip operations"
	}
}

// ShowContextSummary displays a summary of available commands for current context
func (cf *CommandFilter) ShowContextSummary(rootCmd *cobra.Command) {
	currentCtx := cf.manager.Current()
	availableCommands := cf.getContextCommands(currentCtx)

	// Count available commands
	availableCount := 0
	for _, cmd := range rootCmd.Commands() {
		if cf.isCommandAvailable(cmd.Name(), availableCommands) {
			availableCount++
		}
	}

	cf.logger.Info("Context summary",
		"context", currentCtx,
		"available_commands", availableCount,
		"total_commands", len(rootCmd.Commands()),
		"description", cf.GetContextDescription(currentCtx))
}

// getContextCommands returns available commands for a specific context
func (cf *CommandFilter) getContextCommands(ctx ExecutionContext) map[string]bool {
	switch ctx {
	case ContextLocal:
		return map[string]bool{
			// Core archive operations
			"create":   true,
			"analyze":  true,
			"find":     true,
			"tree":     true,
			"estimate": true,
			"wizard":   true,

			// File operations
			"benchmark": true,

			// Configuration and management
			"config":    true,
			"lifecycle": true,
			"metrics":   true,
			"retier":    true,
			"context":   true,

			// Infrastructure (can be started from local)
			"travelagent": true,
			"shell":       true,

			// Utilities and documentation
			"schema": true,
			"man":    true,
			"mddocs": true,
		}

	case ContextAgent:
		return map[string]bool{
			// Agent-specific operations (future commands)
			"agent": true, // Future: agent status, logs, config reload

			// Configuration management
			"config":  true,
			"context": true,

			// Monitoring
			"metrics": true,

			// Interactive mode
			"shell": true,

			// Documentation
			"schema": true,
			"man":    true,
		}

	case ContextREPL:
		// REPL mode provides access to all commands through interactive discovery
		return map[string]bool{
			"create":      true,
			"analyze":     true,
			"find":        true,
			"tree":        true,
			"estimate":    true,
			"wizard":      true,
			"benchmark":   true,
			"config":      true,
			"lifecycle":   true,
			"metrics":     true,
			"retier":      true,
			"context":     true,
			"travelagent": true,
			"shell":       true,
			"schema":      true,
			"man":         true,
			"mddocs":      true,
		}

	default:
		// Default to local context commands
		return cf.getContextCommands(ContextLocal)
	}
}

// isCommandAvailable checks if a command is available in the given command set
func (cf *CommandFilter) isCommandAvailable(commandName string, availableCommands map[string]bool) bool {
	return availableCommands[commandName]
}
