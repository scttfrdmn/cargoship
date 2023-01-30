package cmd

import (
	"encoding/json"
	"fmt"

	"github.com/invopop/jsonschema"
	"github.com/spf13/cobra"
	"gitlab.oit.duke.edu/devil-ops/suitcasectl/pkg/inventory"
)

// schemaCmd represents the schema command
var schemaCmd = &cobra.Command{
	Use:    "schema",
	Short:  "Generate json schema for current inventory definition",
	Hidden: true,
	RunE:   schemaRunE,
}

func schemaRunE(cmd *cobra.Command, args []string) error {
	schema := jsonschema.Reflect(&inventory.DirectoryInventory{})
	bts, err := json.MarshalIndent(schema, "  ", "  ")
	if err != nil {
		return fmt.Errorf("failed to create jsonschema: %w", err)
	}
	fmt.Println(string(bts))
	return nil
}

func init() {
	// Here you will define your flags and configuration settings.

	// Cobra supports Persistent Flags which will work for this command
	// and all subcommands, e.g.:
	// schemaCmd.PersistentFlags().String("foo", "", "A help for foo")

	// Cobra supports local flags which will only run when this command
	// is called directly, e.g.:
	// schemaCmd.Flags().BoolP("toggle", "t", false, "Help message for toggle")
}
