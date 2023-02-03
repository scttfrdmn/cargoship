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
		Run: func(cmd *cobra.Command, args []string) {
			fmt.Println("create called")
		},
	}
	bindCreateKeys(cmd)
	return cmd
}
