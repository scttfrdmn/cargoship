package cmd

import (
	"fmt"
	"strings"

	"github.com/spf13/cobra"
	"github.com/xlab/treeprint"
	"gitlab.oit.duke.edu/devil-ops/suitcasectl/pkg/inventory"
)

// NewTreeCmd creates a new 'find' command
func NewTreeCmd() *cobra.Command {
	cmd := &cobra.Command{
		Use:     "tree /path/to/inventory.yaml",
		Short:   "Print out a tree style listing of the files in a given inventory",
		Aliases: []string{"t"},
		Args:    cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			inv, err := inventory.NewInventoryWithFilename(args[0])
			if err != nil {
				return err
			}
			tree := treeprint.NewWithRoot("/")
			nodeMap := map[string]treeprint.Tree{}
			for _, item := range inv.Files {
				parts := strings.Split(item.Path, "/")

				currentNode := tree

				for i, part := range parts {
					if part == "" {
						continue
					}
					currentPath := strings.Join(parts[:i+1], "/")

					if existingNode, exists := nodeMap[currentPath]; exists {
						currentNode = existingNode
					} else {
						currentNode = currentNode.AddBranch(part)
						nodeMap[currentPath] = currentNode
					}
				}
			}
			if _, err := fmt.Fprint(cmd.OutOrStdout(), tree.String()); err != nil {
				return err
			}
			return nil
		},
	}

	cmd.PersistentFlags().StringArray("inventory-directory", []string{"."}, "Directory containing inventories to search. Can be specified multiple times for multiple directories.")
	return cmd
}
