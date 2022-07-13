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
	"os"

	"github.com/dustin/go-humanize"
	"github.com/goccy/go-yaml"
	"github.com/rs/zerolog/log"
	"github.com/spf13/cobra"
	"gitlab.oit.duke.edu/oit-ssi-systems/data-suitcase/pkg/helpers"
	"gitlab.oit.duke.edu/oit-ssi-systems/data-suitcase/pkg/inventory"
)

// createInventoryCmd represents the inventory command
var createInventoryCmd = &cobra.Command{
	Use:   "inventory",
	Short: "Generate an inventory file for a directory, or set of directories",
	Args:  cobra.MinimumNArgs(1),
	Run: func(cmd *cobra.Command, args []string) {
		maxSuitcaseSizeH, err := cmd.Flags().GetString("max-suitcase-size")
		checkErr(err, "")

		var outF *os.File
		outFile, err := cmd.Flags().GetString("output-file")
		checkErr(err, "")
		if outFile == "" {
			outF, err = os.CreateTemp("", "suitcase-inventory-*.yaml")
			checkErr(err, "")
			defer outF.Close()
		} else {

			// Go ahead and create the file if it doesn't exist
			outF, err = os.Create(outFile)
			checkErr(err, "")
			defer outF.Close()
		}

		maxSuitcaseSizeU, err := humanize.ParseBytes(maxSuitcaseSizeH)
		checkErr(err, "")
		maxSuitcaseSize := int64(maxSuitcaseSizeU)

		// Use absolute dirs forever
		targetDirs, err := helpers.ConvertDirsToAboluteDirs(args)
		checkErr(err, "")

		// Get the internal and external metadata glob patterns
		internalMetadataGlob, err := cmd.Flags().GetString("internal-metadata-glob")
		checkErr(err, "")

		// External metadata file here
		externalMetadataFiles, err := cmd.Flags().GetStringArray("external-metadata-file")
		checkErr(err, "")

		opt := &inventory.DirectoryInventoryOptions{
			TopLevelDirectories:   targetDirs,
			InternalMetadataGlob:  internalMetadataGlob,
			ExternalMetadataFiles: externalMetadataFiles,
			// SizeConsideredLarge: lfs,
		}

		// Set the stuff to be encyrpted?
		opt.EncryptInner, err = cmd.Flags().GetBool("encrypt-inner")
		checkErr(err, "")

		// Do we want to skip hashes?
		opt.HashInner, err = cmd.Flags().GetBool("hash-inner")
		checkErr(err, "")
		if opt.HashInner {
			log.Warn().
				Msg("Generating file hashes. This will will likely increase the inventory generation time.")
		} else {
			log.Warn().
				Msg("Skipping file hashes. This will increase the speed of the inventory, but will not be able to verify the integrity of the files.")
		}

		// Hash the suitcase itself
		// opt.HashOuter, err = cmd.Flags().GetBool("hash-outer")
		// checkErr(err, "")

		inventoryD, err := inventory.NewDirectoryInventory(opt)
		cobra.CheckErr(err)
		if maxSuitcaseSize > 0 {
			err := inventory.IndexInventory(inventoryD, maxSuitcaseSize)
			checkErr(err, "")
			log.Info().Int("count", inventoryD.TotalIndexes).Msg("Indexed inventory")
		}

		// Long print
		// data, err := yaml.Marshal(inventoryD)
		// cobra.CheckErr(err)
		// fmt.Println(string(data))
		enc := yaml.NewEncoder(outF)
		err = enc.Encode(inventoryD)
		checkErr(err, "")
		log.Info().Str("file", outF.Name()).Msg("Created inventory file")
	},
}

func init() {
	createCmd.AddCommand(createInventoryCmd)

	// Here you will define your flags and configuration settings.

	// Cobra supports Persistent Flags which will work for this command
	// and all subcommands, e.g.:
	// createInventoryCmd.PersistentFlags().String("foo", "", "A help for foo")
	createInventoryCmd.PersistentFlags().StringP("output-file", "o", "", "File to write the inventory to. If not specified, the inventory will be written to a temp file.")
	createInventoryCmd.PersistentFlags().String("max-suitcase-size", "0", "Maximum size for the set of suitcases generated. If no unit is specified, 'bytes' is assumed")
	createInventoryCmd.PersistentFlags().String("internal-metadata-glob", "suitcase-meta*", "Glob pattern for internal metadata files. This should be directly under the top level directories of the targets that are being packaged up. Multiple matches will be included if found.")
	createInventoryCmd.PersistentFlags().StringArray("external-metadata-file", []string{}, "Additional files to include as metadata in the inventory. This should NOT be part of the suitcase target directories...use internal-metadata-glob for those")
	createInventoryCmd.PersistentFlags().Bool("hash-inner", false, "Create SHA256 hashes for the inner contents of the suitcase")
	createInventoryCmd.PersistentFlags().Bool("encrypt-inner", false, "Encrypt files within the suitcase")
	// createInventoryCmd.PersistentFlags().Int64("large-file-size", 1024*1024, "Size in bytes of files considered 'large'")

	// Cobra supports local flags which will only run when this command
	// is called directly, e.g.:
	// createInventoryCmd.Flags().BoolP("toggle", "t", false, "Help message for toggle")
}
