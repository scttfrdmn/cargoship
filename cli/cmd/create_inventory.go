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
	"io/ioutil"
	"os"
	"os/user"
	"path"
	"runtime/debug"
	"strings"

	"github.com/dustin/go-humanize"
	"github.com/rs/zerolog/log"
	"github.com/spf13/cobra"
	"gitlab.oit.duke.edu/devil-ops/data-suitcase/pkg/helpers"
	"gitlab.oit.duke.edu/devil-ops/data-suitcase/pkg/inventory"
)

// createInventoryCmd represents the inventory command
var createInventoryCmd = &cobra.Command{
	Use:   "inventory",
	Short: "Generate an inventory file for a directory, or set of directories",
	Args:  cobra.MinimumNArgs(1),
	Run: func(cmd *cobra.Command, args []string) {
		var err error

		printMemUsage()
		maxSuitcaseSizeH, err := cmd.Flags().GetString("max-suitcase-size")
		checkErr(err, "")

		bufferSize, err := cmd.Flags().GetInt("buffer-size")
		checkErr(err, "Could not get buffer size")

		inventoryFormat, err := cmd.Flags().GetString("inventory-format")
		checkErr(err, "Could not get inventory format")
		inventoryFormat = strings.TrimPrefix(inventoryFormat, ".")

		outDir, err = cmd.Flags().GetString("output-dir")
		checkErr(err, "Could not get output directory")

		if outDir == "" {
			outDir, err = ioutil.TempDir("", "suitcase-")
			checkErr(err, "Could not create temp directory")
		}
		inventoryFName := path.Join(outDir, fmt.Sprintf("inventory.%v", inventoryFormat))
		outF, err := os.Create(inventoryFName)
		checkErr(err, "Could not create inventory output file")
		defer outF.Close()

		maxSuitcaseSizeU, err := humanize.ParseBytes(maxSuitcaseSizeH)
		checkErr(err, "")
		maxSuitcaseSize := int64(maxSuitcaseSizeU)

		// Init empty options, we'll fill them in later
		opt := &inventory.DirectoryInventoryOptions{}

		// Use absolute dirs forever
		opt.TopLevelDirectories, err = helpers.ConvertDirsToAboluteDirs(args)
		checkErr(err, "")

		// Get the internal and external metadata glob patterns
		opt.InternalMetadataGlob, err = cmd.Flags().GetString("internal-metadata-glob")
		checkErr(err, "")

		// External metadata file here
		opt.ExternalMetadataFiles, err = cmd.Flags().GetStringArray("external-metadata-file")
		checkErr(err, "")

		// We may want to limit the number of files in the total
		// inventory, mainly to help with debugging, but store that here
		opt.LimitFileCount, err = cmd.Flags().GetInt("limit-file-count")
		checkErr(err, "")

		// Format for the archive/suitcase
		opt.Format, err = cmd.Flags().GetString("format")
		checkErr(err, "")

		// Always strip the leading dot
		opt.Format = strings.TrimPrefix(opt.Format, ".")

		// We want a username so we can shove it in the suitcase name
		opt.User, err = cmd.Flags().GetString("user")
		checkErr(err, "")

		if opt.User == "" {
			log.Info().Msg("No user specified, using current user")
			currentUser, err := user.Current()
			if err != nil {
				log.Fatal().Err(err).Msg("Failed to get current user")
			}
			opt.User = currentUser.Username
		}

		opt.Prefix, err = cmd.Flags().GetString("prefix")
		checkErr(err, "")

		// Set the stuff to be encrypted?
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

		printMemUsage()
		// Create the inventory
		inventoryD, err := inventory.NewDirectoryInventory(opt)
		cobra.CheckErr(err)

		printMemUsage()
		if maxSuitcaseSize > 0 {
			err := inventory.IndexInventory(inventoryD, maxSuitcaseSize)
			checkErr(err, "")
			log.Info().Int("count", inventoryD.TotalIndexes).Msg("Indexed inventory")
		}
		// Add filenames to the inventory
		err = inventory.ExpandSuitcaseNames(inventoryD)
		checkErr(err, "")

		// Create a new buffered io writer
		printMemUsage()
		log.Debug().Int("buffer", bufferSize).Msg("About to create a new buffered Writer")

		// Do this thing Victor says _may_ help
		printMemUsage()
		log.Debug().Msg("Running FreeOSMemory")
		debug.FreeOSMemory()

		// Write the inventory to the file
		printMemUsage()
		// var ir inventory.Inventoryer
		ir, err := inventory.NewInventoryerWithFilename(outF.Name())
		checkErr(err, "")
		err = ir.Write(outF, inventoryD)
		checkErr(err, "")

		// Donzo!
		printMemUsage()
		log.Info().Str("file", outF.Name()).Msg("Created inventory file")
		totalC := uint(0)
		totalS := int64(0)
		for k, item := range inventoryD.IndexSummaries {
			totalC += item.Count
			totalS += item.Size
			log.Info().
				Int("index", k).
				Uint("file-count", item.Count).
				Int64("file-size", item.Size).
				Str("file-size-human", humanize.Bytes(uint64(item.Size))).
				Msg("Individual Suitcase Data")
		}
		log.Info().
			Uint("file-count", totalC).
			Int64("file-size", totalS).
			Str("file-size-human", humanize.Bytes(uint64(totalS))).
			Msg("Total Suitcase Data")
	},
}

func init() {
	createCmd.AddCommand(createInventoryCmd)

	// Here you will define your flags and configuration settings.

	// Cobra supports Persistent Flags which will work for this command
	// and all subcommands, e.g.:
	// createInventoryCmd.PersistentFlags().String("foo", "", "A help for foo")
	createInventoryCmd.PersistentFlags().String("inventory-format", "yaml", "Format for the inventory. Should be 'yaml' or 'json'")
	createInventoryCmd.PersistentFlags().String("max-suitcase-size", "0", "Maximum size for the set of suitcases generated. If no unit is specified, 'bytes' is assumed")
	createInventoryCmd.PersistentFlags().String("internal-metadata-glob", "suitcase-meta*", "Glob pattern for internal metadata files. This should be directly under the top level directories of the targets that are being packaged up. Multiple matches will be included if found.")
	createInventoryCmd.PersistentFlags().StringArray("external-metadata-file", []string{}, "Additional files to include as metadata in the inventory. This should NOT be part of the suitcase target directories...use internal-metadata-glob for those")
	createInventoryCmd.PersistentFlags().Bool("hash-inner", false, "Create SHA256 hashes for the inner contents of the suitcase")
	createInventoryCmd.PersistentFlags().Bool("encrypt-inner", false, "Encrypt files within the suitcase")
	createInventoryCmd.PersistentFlags().Int("buffer-size", 1024, "Buffer size for the output file. This may need to be tweaked for the host memory and fileset")
	createInventoryCmd.PersistentFlags().Int("limit-file-count", 0, "Limit the number of files to include in the inventory. If 0, no limit is applied. Should only be used for debugging")
	createInventoryCmd.PersistentFlags().String("format", "tar.gz", "Format of the suitcase. Valid options are: tar, tar.gz, tar.gpg and tar.gz.gpg")
	createInventoryCmd.PersistentFlags().String("user", "", "Username to insert into the suitcase filename. If omitted, we'll try and detect from the current user")
	createInventoryCmd.PersistentFlags().String("prefix", "suitcase", "Prefex to insert into the suitcase filename")
	// createInventoryCmd.PersistentFlags().Int64("large-file-size", 1024*1024, "Size in bytes of files considered 'large'")

	// Cobra supports local flags which will only run when this command
	// is called directly, e.g.:
	// createInventoryCmd.Flags().BoolP("toggle", "t", false, "Help message for toggle")
}
