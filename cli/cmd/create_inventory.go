/*
Copyright © 2022 NAME HERE <EMAIL ADDRESS>

Licensed under the Apache License, Version 2.0 (the "License");
you may not use this file except in compliance with the License.
You may obtain a copy of the License at

    http://www.apache.org/licenses/LICENSE-2.0

Unless required by applicable law or agreed to in writing, software
distributed under the License is distributed on an "AS IS" BASIS,
WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
See the License for the specific language governing permissions and
limitations under the License.
*/
package cmd

import (
	"fmt"

	"github.com/apex/log"
	"github.com/spf13/cobra"
	"gitlab.oit.duke.edu/oit-ssi-systems/data-suitcase/pkg/helpers"
	"gitlab.oit.duke.edu/oit-ssi-systems/data-suitcase/pkg/inventory"
	"gopkg.in/yaml.v2"
)

// createInventoryCmd represents the inventory command
var createInventoryCmd = &cobra.Command{
	Use:   "inventory",
	Short: "Generate an inventory file for a directory, or set of directories",
	Args:  cobra.MinimumNArgs(1),
	Run: func(cmd *cobra.Command, args []string) {
		lfs, err := cmd.Flags().GetInt64("large-file-size")
		checkErr(err, "")

		maxSuitcaseSize, err := cmd.Flags().GetInt64("max-suitcase-size")
		checkErr(err, "")

		// Use absolute dirs forever
		targetDirs, err := helpers.ConvertDirsToAboluteDirs(args)
		checkErr(err, "")

		opt := &inventory.DirectoryInventoryOptions{
			TopLevelDirectories: targetDirs,
			SizeConsideredLarge: lfs,
		}
		inventoryD, err := inventory.NewDirectoryInventory(opt)
		cobra.CheckErr(err)
		if maxSuitcaseSize > 0 {
			numSuitcases, err := inventory.IndexInventory(inventoryD, maxSuitcaseSize)
			checkErr(err, "")
			log.WithField("num", numSuitcases).Info("Indexed inventory")
		}

		// Long print
		data, err := yaml.Marshal(inventoryD)
		cobra.CheckErr(err)
		fmt.Println(string(data))
		log.Info("Completed")
	},
}

func init() {
	createCmd.AddCommand(createInventoryCmd)

	// Here you will define your flags and configuration settings.

	// Cobra supports Persistent Flags which will work for this command
	// and all subcommands, e.g.:
	// createInventoryCmd.PersistentFlags().String("foo", "", "A help for foo")

	// Cobra supports local flags which will only run when this command
	// is called directly, e.g.:
	// createInventoryCmd.Flags().BoolP("toggle", "t", false, "Help message for toggle")
}
