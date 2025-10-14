// Package repl provides an interactive shell environment for CargoShip
package repl

import (
	"bufio"
	"fmt"
	"log/slog"
	"os"
	"os/signal"
	"sort"
	"strings"
	"syscall"

	"github.com/spf13/cobra"

	"github.com/scttfrdmn/cargoship/pkg/context"
)

// Shell represents an interactive REPL environment
type Shell struct {
	rootCmd        *cobra.Command
	contextManager *context.Manager
	logger         *slog.Logger
	running        bool
	history        []string
	maxHistory     int
}

// NewShell creates a new interactive shell
func NewShell(rootCmd *cobra.Command, logger *slog.Logger) *Shell {
	return &Shell{
		rootCmd:        rootCmd,
		contextManager: context.NewManager(logger),
		logger:         logger.With("component", "repl-shell"),
		maxHistory:     100,
		history:        make([]string, 0),
	}
}

// Start begins the interactive shell session
func (s *Shell) Start() error {
	s.running = true

	// Set context to REPL mode
	if err := s.contextManager.SwitchTo(context.ContextREPL); err != nil {
		s.logger.Warn("Failed to switch to REPL context", "error", err)
	}

	// Setup signal handling
	sigChan := make(chan os.Signal, 1)
	signal.Notify(sigChan, syscall.SIGINT, syscall.SIGTERM)
	go s.handleSignals(sigChan)

	// Display welcome message
	s.showWelcome()

	// Main REPL loop
	scanner := bufio.NewScanner(os.Stdin)

	for s.running {
		// Show prompt
		fmt.Print(s.getPrompt())

		// Read input
		if !scanner.Scan() {
			break // EOF or error
		}

		input := strings.TrimSpace(scanner.Text())

		// Skip empty lines
		if input == "" {
			continue
		}

		// Handle special commands
		if s.handleSpecialCommands(input) {
			continue
		}

		// Add to history
		s.addToHistory(input)

		// Execute command
		s.executeCommand(input)
	}

	if err := scanner.Err(); err != nil {
		return fmt.Errorf("scanner error: %w", err)
	}

	s.showGoodbye()
	return nil
}

// Stop gracefully stops the shell
func (s *Shell) Stop() {
	s.running = false
}

// getPrompt returns the current prompt based on context
func (s *Shell) getPrompt() string {
	currentCtx := s.contextManager.Current()

	switch currentCtx {
	case context.ContextLocal:
		return "cargoship> "
	case context.ContextAgent:
		return "agent> "
	case context.ContextController:
		return "controller> "
	case context.ContextREPL:
		return "repl> "
	default:
		return "cargoship> "
	}
}

// showWelcome displays the welcome message
func (s *Shell) showWelcome() {
	fmt.Print(`
🚢 CargoShip Interactive Shell
━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━

Welcome to the CargoShip interactive environment!

Available commands:
  help           - Show available commands
  context        - View/switch execution context  
  history        - Show command history
  clear          - Clear the screen
  exit, quit     - Exit the shell
  
Context-specific commands will be shown based on your current context.
Type 'help' for a complete list of available commands.

Press Ctrl+C or type 'exit' to quit.
`)

	// Show current context
	currentCtx := s.contextManager.Current()
	fmt.Printf("Current context: %s\n", currentCtx)
	fmt.Printf("Description: %s\n\n", context.FormatContext(currentCtx))
}

// showGoodbye displays the goodbye message
func (s *Shell) showGoodbye() {
	fmt.Println("\n🚢 Thanks for using CargoShip! Safe travels!")
}

// handleSpecialCommands processes special REPL commands
func (s *Shell) handleSpecialCommands(input string) bool {
	parts := strings.Fields(input)
	if len(parts) == 0 {
		return false
	}

	command := strings.ToLower(parts[0])

	switch command {
	case "exit", "quit", "q":
		s.Stop()
		return true

	case "clear", "cls":
		s.clearScreen()
		return true

	case "history", "hist":
		s.showHistory()
		return true

	case "help", "?":
		if len(parts) > 1 {
			// Show help for specific command
			s.showCommandHelp(parts[1])
		} else {
			// Show general help
			s.showHelp()
		}
		return true

	case "context", "ctx":
		if len(parts) > 1 {
			s.handleContextCommand(parts[1:])
		} else {
			s.showCurrentContext()
		}
		return true

	default:
		return false
	}
}

// executeCommand runs a CargoShip command
func (s *Shell) executeCommand(input string) {
	// Parse command and arguments
	args := strings.Fields(input)
	if len(args) == 0 {
		return
	}

	// Create a new command instance for execution
	cmd := s.createCommandInstance()
	cmd.SetArgs(args)

	// Capture output
	cmd.SetOut(os.Stdout)
	cmd.SetErr(os.Stderr)

	// Execute command
	if err := cmd.Execute(); err != nil {
		fmt.Printf("Error: %v\n", err)
	}
}

// createCommandInstance creates a fresh command instance for execution
func (s *Shell) createCommandInstance() *cobra.Command {
	// Create a copy of the root command structure
	// This ensures each execution is clean
	return s.rootCmd
}

// addToHistory adds a command to the history
func (s *Shell) addToHistory(command string) {
	s.history = append(s.history, command)

	// Limit history size
	if len(s.history) > s.maxHistory {
		s.history = s.history[1:]
	}
}

// showHistory displays command history
func (s *Shell) showHistory() {
	if len(s.history) == 0 {
		fmt.Println("No command history available.")
		return
	}

	fmt.Println("Command History:")
	for i, cmd := range s.history {
		fmt.Printf("%3d: %s\n", i+1, cmd)
	}
}

// clearScreen clears the terminal screen
func (s *Shell) clearScreen() {
	fmt.Print("\033[H\033[2J")
}

// showHelp displays available commands based on current context
func (s *Shell) showHelp() {
	currentCtx := s.contextManager.Current()

	fmt.Printf("Available commands in [%s] context:\n\n", currentCtx)

	// Get available commands for current context
	availableCommands := s.getAvailableCommands(currentCtx)

	// Sort commands for better display
	var commands []string
	for _, cmd := range s.rootCmd.Commands() {
		if !cmd.Hidden && availableCommands[cmd.Name()] {
			commands = append(commands, cmd.Name())
		}
	}
	sort.Strings(commands)

	// Display commands with descriptions
	for _, cmdName := range commands {
		if cmd := s.findCommand(cmdName); cmd != nil {
			fmt.Printf("  %-12s - %s\n", cmdName, cmd.Short)
		}
	}

	fmt.Println("\nSpecial REPL commands:")
	fmt.Println("  help         - Show this help message")
	fmt.Println("  context      - View/switch execution context")
	fmt.Println("  history      - Show command history")
	fmt.Println("  clear        - Clear the screen")
	fmt.Println("  exit         - Exit the shell")

	fmt.Printf("\nType 'help <command>' for detailed help on a specific command.\n")
	fmt.Printf("Current context: %s\n", currentCtx)
}

// showCommandHelp shows help for a specific command
func (s *Shell) showCommandHelp(cmdName string) {
	cmd := s.findCommand(cmdName)
	if cmd == nil {
		fmt.Printf("Command '%s' not found. Type 'help' for available commands.\n", cmdName)
		return
	}

	// Show command help
	cmd.SetArgs([]string{cmdName, "--help"})
	_ = cmd.Execute()
}

// findCommand finds a command by name
func (s *Shell) findCommand(name string) *cobra.Command {
	for _, cmd := range s.rootCmd.Commands() {
		if cmd.Name() == name {
			return cmd
		}
	}
	return nil
}

// handleContextCommand handles context-related commands in REPL
func (s *Shell) handleContextCommand(args []string) {
	if len(args) == 0 {
		s.showCurrentContext()
		return
	}

	subcommand := args[0]

	switch subcommand {
	case "switch", "sw":
		if len(args) < 2 {
			fmt.Println("Usage: context switch <context>")
			fmt.Println("Available contexts: local, agent, controller, repl")
			return
		}

		targetContext := context.ExecutionContext(args[1])
		if err := s.contextManager.SwitchTo(targetContext); err != nil {
			fmt.Printf("Error switching context: %v\n", err)
			return
		}

		fmt.Printf("Switched to %s context\n", targetContext)
		fmt.Printf("Description: %s\n", context.FormatContext(targetContext))

	case "list", "ls":
		s.showAvailableContexts()

	default:
		s.showCurrentContext()
	}
}

// showCurrentContext displays the current context
func (s *Shell) showCurrentContext() {
	currentCtx := s.contextManager.Current()
	fmt.Printf("Current context: %s\n", currentCtx)
	fmt.Printf("Description: %s\n", context.FormatContext(currentCtx))

	if endpoint := s.contextManager.GetEndpoint(); endpoint != "" {
		fmt.Printf("Endpoint: %s\n", endpoint)
	}
}

// showAvailableContexts lists all available contexts
func (s *Shell) showAvailableContexts() {
	currentCtx := s.contextManager.Current()
	contexts := context.GetAvailableContexts()

	fmt.Println("Available contexts:")
	for _, ctx := range contexts {
		prefix := "  "
		if ctx == currentCtx {
			prefix = "* "
		}
		fmt.Printf("%s%s - %s\n", prefix, ctx, context.FormatContext(ctx))
	}
}

// getAvailableCommands returns commands available for the current context
func (s *Shell) getAvailableCommands(ctx context.ExecutionContext) map[string]bool {
	switch ctx {
	case context.ContextLocal:
		return map[string]bool{
			"create": true, "analyze": true, "find": true, "tree": true,
			"estimate": true, "wizard": true, "rclone": true, "benchmark": true,
			"config": true, "lifecycle": true, "metrics": true, "retier": true,
			"context": true, "controller": true, "travelagent": true,
			"schema": true, "man": true, "mddocs": true,
		}

	case context.ContextAgent:
		return map[string]bool{
			"config": true, "context": true, "metrics": true,
			"schema": true, "man": true,
		}

	case context.ContextController:
		return map[string]bool{
			"controller": true, "context": true, "metrics": true,
			"config": true, "schema": true, "man": true,
		}

	case context.ContextREPL:
		// REPL has access to all commands
		return map[string]bool{
			"create": true, "analyze": true, "find": true, "tree": true,
			"estimate": true, "wizard": true, "rclone": true, "benchmark": true,
			"config": true, "lifecycle": true, "metrics": true, "retier": true,
			"context": true, "controller": true, "travelagent": true,
			"schema": true, "man": true, "mddocs": true,
		}

	default:
		return s.getAvailableCommands(context.ContextLocal)
	}
}

// handleSignals handles OS signals gracefully
func (s *Shell) handleSignals(sigChan <-chan os.Signal) {
	sig := <-sigChan
	s.logger.Info("Received signal, shutting down", "signal", sig)
	fmt.Println("\n\nReceived interrupt signal. Exiting...")
	s.Stop()
}
