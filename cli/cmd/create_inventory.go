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
	"runtime/debug"

	"github.com/dustin/go-humanize"
	"github.com/mailru/easyjson"
	"github.com/rs/zerolog/log"
	"github.com/spf13/cobra"
	"gitlab.oit.duke.edu/oit-ssi-systems/data-suitcase/cli/cmdhelpers"
	"gitlab.oit.duke.edu/oit-ssi-systems/data-suitcase/pkg/helpers"
	"gitlab.oit.duke.edu/oit-ssi-systems/data-suitcase/pkg/inventory"
	//"gopkg.in/yaml.v3"
)

// createInventoryCmd represents the inventory command
var createInventoryCmd = &cobra.Command{
	Use:   "inventory",
	Short: "Generate an inventory file for a directory, or set of directories",
	Args:  cobra.MinimumNArgs(1),
	Run: func(cmd *cobra.Command, args []string) {
		cmdhelpers.PrintMemUsage()
		maxSuitcaseSizeH, err := cmd.Flags().GetString("max-suitcase-size")
		checkErr(err, "")

		bufferSize, err := cmd.Flags().GetInt("buffer-size")
		checkErr(err, "")

		limitFileCount, err := cmd.Flags().GetInt("limit-file-count")
		checkErr(err, "")

		var outF *os.File
		outFile, err := cmd.Flags().GetString("output-file")
		checkErr(err, "")
		if outFile == "" {
			outF, err = os.CreateTemp("", "suitcase-inventory-*.json")
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
			LimitFileCount:        limitFileCount,
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

		cmdhelpers.PrintMemUsage()
		inventoryD, err := inventory.NewDirectoryInventory(opt)
		cobra.CheckErr(err)
		cmdhelpers.PrintMemUsage()
		if maxSuitcaseSize > 0 {
			err := inventory.IndexInventory(inventoryD, maxSuitcaseSize)
			checkErr(err, "")
			log.Info().Int("count", inventoryD.TotalIndexes).Msg("Indexed inventory")
		}

		// Create a new buffered io writer
		cmdhelpers.PrintMemUsage()
		log.Debug().Int("buffer", bufferSize).Msg("About to create a new buffered Writer")
		// Createa a new io.Writer with a buffer
		/*
			writer := bufio.NewWriterSize(outF, bufferSize)
			defer writer.Flush()

			// Pass the buffered IO writer to the encoder
			cmdhelpers.PrintMemUsage()
			log.Debug().Msg("About to create a new JSON encoder")
			enc := json.NewEncoder(writer)

			// Collect that delicious garbage 😋
			cmdhelpers.PrintMemUsage()
			log.Debug().Msg("Running garbage collection")
			runtime.GC()
		*/

		// Do this thing Victor says _may_ help
		cmdhelpers.PrintMemUsage()
		log.Info().Msg("Running FreeOSMemory")
		debug.FreeOSMemory()

		// Write the inventory to the file
		cmdhelpers.PrintMemUsage()
		log.Debug().Msg("About to encode inventory in to yaml")
		_, err = easyjson.MarshalToWriter(inventoryD, outF)
		checkErr(err, "")
		// err = enc.Encode(inventoryD)

		// Donzo!
		cmdhelpers.PrintMemUsage()
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
	createInventoryCmd.PersistentFlags().Int("buffer-size", 1024, "Buffer size for the output file. This may need to be tweaked for the host memory and fileset")
	createInventoryCmd.PersistentFlags().Int("limit-file-count", 0, "Limit the number of files to include in the inventory. If 0, no limit is applied. Should only be used for debugging")
	// createInventoryCmd.PersistentFlags().Int64("large-file-size", 1024*1024, "Size in bytes of files considered 'large'")

	// Cobra supports local flags which will only run when this command
	// is called directly, e.g.:
	// createInventoryCmd.Flags().BoolP("toggle", "t", false, "Help message for toggle")
}
