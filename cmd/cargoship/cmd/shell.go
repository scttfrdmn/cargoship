package cmd

import (
	"log/slog"

	"github.com/spf13/cobra"

	"github.com/scttfrdmn/cargoship/pkg/repl"
)

// NewShellCmd creates the interactive shell command
func NewShellCmd() *cobra.Command {
	cmd := &cobra.Command{
		Use:     "shell",
		Aliases: []string{"repl", "interactive"},
		Short:   "Start an interactive CargoShip shell",
		Long: `Start an interactive CargoShip shell environment.

The shell provides:
- Context-aware command discovery
- Command history and auto-completion
- Interactive context switching
- Persistent session state
- Built-in help system

Available shell commands:
  help           - Show available commands
  context        - View/switch execution context
  history        - Show command history  
  clear          - Clear the screen
  exit, quit     - Exit the shell

All regular CargoShip commands are available based on your current context.
Use 'context switch <context>' to change between operational modes.`,
		Example: `  # Start interactive shell
  cargoship shell

  # Alternative ways to start
  cargoship repl
  cargoship interactive`,
		RunE: runShell,
	}

	return cmd
}

// runShell starts the interactive shell
func runShell(cmd *cobra.Command, args []string) error {
	logger := slog.Default()

	// Get the root command for shell execution
	rootCmd := cmd.Root()

	// Create and start shell
	shell := repl.NewShell(rootCmd, logger)

	logger.Info("Starting CargoShip interactive shell")

	if err := shell.Start(); err != nil {
		return err
	}

	logger.Info("CargoShip interactive shell stopped")
	return nil
}
