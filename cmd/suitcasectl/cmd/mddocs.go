package cmd

import (
	"io"

	"github.com/spf13/cobra"
	"github.com/spf13/cobra/doc"
)

// NewMDDocsCmd generates the markdown docs
func NewMDDocsCmd() *cobra.Command {
	return &cobra.Command{
		Use:    "mddocs",
		Short:  "Create a new tree of markdown docs",
		Args:   cobra.ExactArgs(1),
		Hidden: true,
		RunE: func(cmd *cobra.Command, args []string) error {
			return doc.GenMarkdownTree(NewRootCmd(io.Discard), args[0])
		},
	}
}
