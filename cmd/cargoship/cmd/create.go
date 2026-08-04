package cmd

import (
	"fmt"

	"github.com/spf13/cobra"
)

// NewCreateCmd creates a new 'create' command
func NewCreateCmd() *cobra.Command {
	cmd := &cobra.Command{
		Use:   "create",
		Short: "Create something!",
		Args:  cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			// `create` on its own is not runnable; it dispatches to a subcommand.
			// Returning cmd.UsageString() as the error *message*, as this used to,
			// sent the entire multi-line usage block through slog.Error as a single
			// quoted line, and produced exit 1 for what is a usage error.
			if len(args) > 0 {
				return fmt.Errorf("%w: unknown command %q for %q", ErrUsage, args[0], cmd.CommandPath())
			}
			return fmt.Errorf("%w: %q requires a subcommand", ErrUsage, cmd.CommandPath())
		},
	}
	bindCreateKeys(cmd)
	bindCreatePipeline(cmd)
	return cmd
}

// bindCreatePipeline adds the pipeline upload command
func bindCreatePipeline(createCmd *cobra.Command) {
	pipelineCmd := NewCreatePipelineCmd()
	createCmd.AddCommand(pipelineCmd)
}
