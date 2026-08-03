package cmd

import (
	"fmt"
	"log/slog"
	"strings"

	"github.com/spf13/cobra"

	"github.com/scttfrdmn/cargoship/pkg/context"
)

// NewContextCmd creates the context management command
func NewContextCmd() *cobra.Command {
	cmd := &cobra.Command{
		Use:   "context",
		Short: "Manage CargoShip execution context",
		Long: `Manage CargoShip execution context to control which commands are available.

Context determines the operational mode:
- local: Local filesystem operations and archive creation
- agent: Launch agent monitoring and management
- repl:  Interactive shell mode with command discovery

The current context is cached in ~/.cargoship-context and persists between sessions.`,
		Example: `  # Show current context
  cargoship context

  # Switch to agent context
  cargoship context switch agent

  # List available contexts
  cargoship context list

  # Reset to default (local) context
  cargoship context reset

  # Show context with details
  cargoship context show`,
		RunE: showCurrentContext,
	}

	// Subcommands
	cmd.AddCommand(
		newContextSwitchCmd(),
		newContextListCmd(),
		newContextShowCmd(),
		newContextResetCmd(),
	)

	return cmd
}

// showCurrentContext displays the current context (default action)
func showCurrentContext(cmd *cobra.Command, args []string) error {
	logger := slog.Default()
	manager := context.NewManager(logger)

	currentCtx := manager.Current()
	fmt.Printf("Current context: %s\n", currentCtx)

	return nil
}

// newContextSwitchCmd creates the context switch subcommand
func newContextSwitchCmd() *cobra.Command {
	return &cobra.Command{
		Use:   "switch <context>",
		Short: "Switch to a different execution context",
		Long: `Switch to a different execution context.

Available contexts:
- local: Local filesystem operations and archive creation
- agent: Launch agent monitoring and management
- repl:  Interactive shell mode with command discovery`,
		Example: `  cargoship context switch local
  cargoship context switch agent
  cargoship context switch repl`,
		Args: cobra.ExactArgs(1),
		RunE: switchContext,
	}
}

// newContextListCmd creates the context list subcommand
func newContextListCmd() *cobra.Command {
	return &cobra.Command{
		Use:     "list",
		Aliases: []string{"ls"},
		Short:   "List all available execution contexts",
		Long:    `List all available execution contexts with descriptions.`,
		RunE:    listContexts,
	}
}

// newContextShowCmd creates the context show subcommand
func newContextShowCmd() *cobra.Command {
	return &cobra.Command{
		Use:   "show",
		Short: "Show detailed context information",
		Long: `Show detailed information about the current context including
cached endpoints, working directory, and context file location.`,
		RunE: showContextDetails,
	}
}

// newContextResetCmd creates the context reset subcommand
func newContextResetCmd() *cobra.Command {
	return &cobra.Command{
		Use:   "reset",
		Short: "Reset context to default (local)",
		Long: `Reset the execution context to default (local) and remove
the context cache file. This is useful for troubleshooting or
starting fresh.`,
		RunE: resetContext,
	}
}

// switchContext handles context switching
func switchContext(cmd *cobra.Command, args []string) error {
	targetContext := context.ExecutionContext(args[0])

	// Validate context
	validContexts := context.GetAvailableContexts()
	isValid := false
	for _, ctx := range validContexts {
		if ctx == targetContext {
			isValid = true
			break
		}
	}

	if !isValid {
		return fmt.Errorf("invalid context '%s'. Valid contexts: %s",
			targetContext, formatContextList(validContexts))
	}

	logger := slog.Default()
	manager := context.NewManager(logger)

	// Get current context
	currentCtx := manager.Current()

	if currentCtx == targetContext {
		fmt.Printf("Already in %s context\n", targetContext)
		return nil
	}

	// Switch context
	if err := manager.SwitchTo(targetContext); err != nil {
		return fmt.Errorf("failed to switch context: %w", err)
	}

	fmt.Printf("Switched from %s to %s context\n", currentCtx, targetContext)
	fmt.Printf("Description: %s\n", context.FormatContext(targetContext))

	// Show helpful tips for the new context
	showContextTips(targetContext)

	return nil
}

// listContexts displays all available contexts
func listContexts(cmd *cobra.Command, args []string) error {
	logger := slog.Default()
	manager := context.NewManager(logger)

	currentCtx := manager.Current()
	contexts := context.GetAvailableContexts()

	fmt.Println("Available execution contexts:")

	for _, ctx := range contexts {
		prefix := "  "
		if ctx == currentCtx {
			prefix = "* " // Mark current context
		}

		fmt.Printf("%s%s - %s\n", prefix, ctx, context.FormatContext(ctx))
	}

	fmt.Printf("\nCurrent context: %s\n", currentCtx)
	return nil
}

// showContextDetails displays detailed context information
func showContextDetails(cmd *cobra.Command, args []string) error {
	logger := slog.Default()
	manager := context.NewManager(logger)

	// Load full context info
	info, err := manager.Load()
	if err != nil {
		return fmt.Errorf("failed to load context details: %w", err)
	}

	fmt.Printf("Context Details:\n")
	fmt.Printf("  Current:     %s\n", info.Current)
	fmt.Printf("  Description: %s\n", context.FormatContext(info.Current))
	fmt.Printf("  Last Used:   %s\n", info.LastUsed.Format("2006-01-02 15:04:05"))
	fmt.Printf("  Version:     %s\n", info.Version)

	if info.WorkingDir != "" {
		fmt.Printf("  Working Dir: %s\n", info.WorkingDir)
	}

	endpoint := manager.GetEndpoint()
	if endpoint != "" {
		fmt.Printf("  Endpoint:    %s\n", endpoint)
	}

	fmt.Printf("  Config File: %s\n", manager.GetContextFile())

	if manager.IsFirstRun() {
		fmt.Printf("  Status:      First run (context will be created)\n")
	} else {
		fmt.Printf("  Status:      Context file exists\n")
	}

	return nil
}

// resetContext resets context to default
func resetContext(cmd *cobra.Command, args []string) error {
	logger := slog.Default()
	manager := context.NewManager(logger)

	currentCtx := manager.Current()

	if err := manager.Reset(); err != nil {
		return fmt.Errorf("failed to reset context: %w", err)
	}

	fmt.Printf("Reset context from %s to local (default)\n", currentCtx)
	fmt.Printf("Context cache file removed: %s\n", manager.GetContextFile())

	return nil
}

// formatContextList formats a list of contexts for display
func formatContextList(contexts []context.ExecutionContext) string {
	var parts []string
	for _, ctx := range contexts {
		parts = append(parts, string(ctx))
	}
	return strings.Join(parts, ", ")
}

// showContextTips provides helpful tips for the newly switched context
func showContextTips(ctx context.ExecutionContext) {
	fmt.Println()

	switch ctx {
	case context.ContextLocal:
		fmt.Println("💡 Local context tips:")
		fmt.Println("   cargoship create suitcase /path/to/data")
		fmt.Println("   cargoship analyze /path/to/data")
		fmt.Println("   cargoship wizard  # Interactive setup")

	case context.ContextAgent:
		fmt.Println("💡 Agent context tips:")
		fmt.Println("   cargoship agent status")
		fmt.Println("   cargoship agent logs")
		fmt.Println("   cargoship agent config reload")

	case context.ContextREPL:
		fmt.Println("💡 REPL context tips:")
		fmt.Println("   cargoship shell  # Start interactive session")
		fmt.Println("   Use tab completion and help commands")
		fmt.Println("   Type 'exit' to leave interactive mode")
	}
}
