package cmd

import (
	"fmt"

	"github.com/spf13/cobra"
)

// createCmd represents the create command
var createCmd = &cobra.Command{
	Use:   "create",
	Short: "Create something!",
	Args:  cobra.ExactArgs(1),
	Run: func(cmd *cobra.Command, args []string) {
		fmt.Println("create called")
	},
}

func init() {
	/*
		createCmd.PersistentFlags().StringVarP(&outDir, "output-dir", "o", "", "Directory to write files in to. If not specified, we'll use an auto generated temp dir")
		if derr := createCmd.PersistentFlags().MarkDeprecated("output-dir", "Please use --destination instead of --output-dir"); derr != nil {
			panic(derr)
		}
	*/

	createCmd.PersistentFlags().StringP("destination", "d", "", "Directory to write files in to. If not specified, we'll use an auto generated temp dir")
	if oerr := createCmd.MarkPersistentFlagDirname("destination"); oerr != nil {
		panic(oerr)
	}

	// Cobra supports local flags which will only run when this command
	// is called directly, e.g.:
	// createCmd.Flags().BoolP("toggle", "t", false, "Help message for toggle")
}
