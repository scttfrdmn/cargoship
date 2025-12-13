// Package context provides CLI integration utilities
package context

import (
	"fmt"
	"log/slog"
	"os"

	"github.com/spf13/cobra"
)

// EnhanceRootCommand adds context awareness to the root command
func EnhanceRootCommand(cmd *cobra.Command, logger *slog.Logger) {
	// Add context information to help text
	originalLong := cmd.Long
	cmd.Long = originalLong + "\n\n" + getContextHelpText()

	// Add context flag for explicit context switching
	cmd.PersistentFlags().String("context", "", "Override execution context (local, agent, controller, repl)")

	// Add pre-run hook to handle context
	originalPreRun := cmd.PersistentPreRun
	cmd.PersistentPreRun = func(cmd *cobra.Command, args []string) {
		// Handle context flag if provided
		if contextFlag, _ := cmd.Flags().GetString("context"); contextFlag != "" {
			if err := handleContextFlag(contextFlag, logger); err != nil {
				fmt.Fprintf(os.Stderr, "Context error: %v\n", err)
				os.Exit(1)
			}
		}

		// Apply context-aware command filtering
		FilterCommandsByContext(cmd.Root(), logger)

		// Show context in verbose mode
		if verbose, _ := cmd.Flags().GetBool("verbose"); verbose {
			showContextInfo(logger)
		}

		// Call original pre-run if it exists
		if originalPreRun != nil {
			originalPreRun(cmd, args)
		}
	}
}

// FilterCommandsByContext removes commands that aren't available in the current context
func FilterCommandsByContext(cmd *cobra.Command, logger *slog.Logger) {
	manager := NewManager(logger)
	currentCtx := manager.Current()

	// Get available commands for current context
	availableCommands := getAvailableCommands(currentCtx)

	// Hide commands not available in current context
	for _, subcmd := range cmd.Commands() {
		if !isCommandAvailable(subcmd.Name(), availableCommands) {
			// Hide command but don't remove (for help text consistency)
			subcmd.Hidden = true
		}
	}
}

// getContextHelpText returns help text about context system
func getContextHelpText() string {
	return `Context System:
  CargoShip uses execution contexts to organize commands by operational mode.
  
  Available contexts:
    local      - Local filesystem operations and archive creation
    agent      - Launch agent monitoring and management
    controller - Controller operations and agent coordination
    repl       - Interactive shell mode with command discovery
  
  Use 'cargoship context' to view or change the current context.
  The current context is cached in ~/.cargoship-context.`
}

// handleContextFlag processes the --context flag
func handleContextFlag(contextStr string, logger *slog.Logger) error {
	targetContext := ExecutionContext(contextStr)

	// Validate context
	if !isValidContext(targetContext) {
		return fmt.Errorf("invalid context '%s'. Valid contexts: local, agent, controller, repl", contextStr)
	}

	manager := NewManager(logger)
	currentCtx := manager.Current()

	// Switch context if different
	if currentCtx != targetContext {
		if err := manager.SwitchTo(targetContext); err != nil {
			return fmt.Errorf("failed to switch context: %w", err)
		}
		logger.Info("Context switched for this command", "from", currentCtx, "to", targetContext)
	}

	return nil
}

// showContextInfo displays current context in verbose mode
func showContextInfo(logger *slog.Logger) {
	manager := NewManager(logger)
	currentCtx := manager.Current()

	logger.Info("Current execution context", "context", currentCtx, "description", FormatContext(currentCtx))

	// Show endpoint if applicable
	if endpoint := manager.GetEndpoint(); endpoint != "" {
		logger.Info("Context endpoint", "endpoint", endpoint)
	}
}

// getAvailableCommands returns commands available for a given context
func getAvailableCommands(ctx ExecutionContext) map[string]bool {
	switch ctx {
	case ContextLocal:
		return map[string]bool{
			// Core local operations
			"create":    true,
			"analyze":   true,
			"find":      true,
			"tree":      true,
			"estimate":  true,
			"wizard":    true,
			"benchmark": true,

			// Management commands
			"config":    true,
			"lifecycle": true,
			"metrics":   true,
			"retier":    true,
			"context":   true,

			// Infrastructure (can start from local)
			"controller":  true,
			"travelagent": true,

			// Utilities
			"schema": true,
			"man":    true,
			"mddocs": true,
		}

	case ContextAgent:
		return map[string]bool{
			// Agent-specific operations
			"agent":   true, // Future: agent status/management commands
			"config":  true, // Config reload/view
			"context": true,

			// Monitoring and utilities
			"metrics": true,
			"schema":  true,
			"man":     true,
		}

	case ContextController:
		return map[string]bool{
			// Controller operations
			"controller": true,
			"context":    true,

			// Monitoring and management
			"metrics": true,
			"config":  true,

			// Utilities
			"schema": true,
			"man":    true,
		}

	case ContextREPL:
		// REPL mode has access to all commands through interactive discovery
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
			"controller":  true,
			"travelagent": true,
			"agent":       true,
			"schema":      true,
			"man":         true,
			"mddocs":      true,
			"shell":       true, // Future: REPL command
		}

	default:
		// Default to local context commands
		return getAvailableCommands(ContextLocal)
	}
}

// isCommandAvailable checks if a command is available in the current context
func isCommandAvailable(commandName string, availableCommands map[string]bool) bool {
	return availableCommands[commandName]
}

// GetContextPrompt returns a prompt string for the current context (for REPL use)
func GetContextPrompt(logger *slog.Logger) string {
	manager := NewManager(logger)
	currentCtx := manager.Current()

	switch currentCtx {
	case ContextLocal:
		return "cargoship> "
	case ContextAgent:
		return "agent> "
	case ContextController:
		return "controller> "
	case ContextREPL:
		return "repl> "
	default:
		return "cargoship> "
	}
}

// DetectContextFromEnvironment attempts to detect context from environment variables
func DetectContextFromEnvironment() ExecutionContext {
	// Check for common environment variables that indicate context
	if os.Getenv("CARGOSHIP_AGENT_MODE") != "" || os.Getenv("CARGOSHIP_CONTROLLER_URL") != "" {
		return ContextAgent
	}

	if os.Getenv("CARGOSHIP_CONTROLLER_MODE") != "" {
		return ContextController
	}

	if os.Getenv("CARGOSHIP_REPL_MODE") != "" {
		return ContextREPL
	}

	// Default to local
	return ContextLocal
}
