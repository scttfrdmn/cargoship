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
	"errors"
	"fmt"
	"io/ioutil"
	"os"
	"path"
	"strings"

	"github.com/dustin/go-humanize"
	"github.com/rs/zerolog/log"
	"github.com/spf13/cobra"
	"gitlab.oit.duke.edu/devil-ops/data-suitcase/cli/cmdhelpers"
	"gitlab.oit.duke.edu/devil-ops/data-suitcase/pkg/config"
	"gitlab.oit.duke.edu/devil-ops/data-suitcase/pkg/gpg"
	"gitlab.oit.duke.edu/devil-ops/data-suitcase/pkg/helpers"
	"gitlab.oit.duke.edu/devil-ops/data-suitcase/pkg/inventory"
)

// createSuitcaseCmd represents the createSuitcase command
// var createSuitcaseCmd = &cobra.Command{
func NewCreateSuitcaseCmd() *cobra.Command {
	cmd := &cobra.Command{
		Use:     "suitcase [--inventory-file=INVENTORY_FILE | TARGET_DIR...]",
		Short:   "Create a suitcase",
		Long:    "Create a suitcase from either an inventory file or multiple target directories.",
		Args:    cobra.ArbitraryArgs,
		Aliases: []string{"suitecase"}, // Encouraging bad habits
		RunE: func(cmd *cobra.Command, args []string) error {
			// Figure out if we are using an inventory file, or creating one
			inventoryFile, err := cmd.Flags().GetString("inventory-file")
			checkErr(err, "Error getting inventory file")
			if inventoryFile != "" && len(args) > 0 {
				return errors.New("Error: You can't specify an inventory file and target dir arguments at the same time")
			}

			// Make sure we are actually using either an inventory file or target dirs
			if inventoryFile == "" && len(args) == 0 {
				log.Fatal().Msg("Error: You must specify an inventory file or target dirs")
			}

			onlyInventory, err := cmd.Flags().GetBool("only-inventory")
			checkErr(err, "")
			if onlyInventory && inventoryFile != "" {
				log.Fatal().Msg("You can't specify an inventory file and only-inventory at the same time")
			}

			// Get this first, it'll be important
			outDir, err = cmdhelpers.NewOutDirWithCmd(cmd)
			checkErr(err, "Could not figure out the output directory")

			// Create an inventory file if one isn't specified
			var inventoryD *inventory.DirectoryInventory
			if inventoryFile == "" {
				log.Info().Msg("No inventory file specified, we're going to go ahead and create one")
				inventoryOpts, err := cmdhelpers.NewDirectoryInventoryOptionsWithCmd(cmd, args)
				checkErr(err, "Could not get inventory options from cmd and args")

				// Create the inventory
				inventoryD, err = inventory.NewDirectoryInventory(inventoryOpts)
				checkErr(err, "Could not create the inventory")

				// Add filenames to the inventory
				log.Info().Msg("Now that we have the inventory, sub in the real suitcase names")
				err = inventory.ExpandSuitcaseNames(inventoryD)
				checkErr(err, "")

				outF, err := os.Create(path.Join(outDir, fmt.Sprintf("inventory.%v", inventoryOpts.InventoryFormat)))
				checkErr(err, "Could not create inventory file")
				ir, err := inventory.NewInventoryerWithFilename(outF.Name())
				checkErr(err, "")
				err = ir.Write(outF, inventoryD)
				checkErr(err, "")
				log.Info().Str("file", outF.Name()).Msg("Created inventory file")
			} else {
				ib, err := ioutil.ReadFile(inventoryFile)
				checkErr(err, "Could not read inventory file")
				ir, err := inventory.NewInventoryerWithFilename(inventoryFile)
				checkErr(err, "")

				inventoryD, err = ir.Read(ib)
				checkErr(err, "")
			}

			// Print some summary info about the index
			var totalC uint
			var totalS int64
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

				// Ok, now create the suitcase!
				// Set up options

			if !onlyInventory {
				opts := &config.SuitCaseOpts{
					Destination:  outDir,
					EncryptInner: inventoryD.Options.EncryptInner,
					HashInner:    inventoryD.Options.HashInner,
					Format:       inventoryD.Options.SuitcaseFormat,
				}

				// Gather EncryptTo if we need it
				if strings.HasSuffix(opts.Format, ".gpg") || opts.EncryptInner {
					opts.EncryptTo, err = gpg.EncryptToWithCmd(cmd)
					checkErr(err, "Could not find who to encrypt this to")
				}

				po := &cmdhelpers.ProcessOpts{
					Inventory:    inventoryD,
					SuitcaseOpts: opts,
				}
				po.Concurrency, err = cmd.Flags().GetInt("concurrency")
				checkErr(err, "")
				createdFiles, err := cmdhelpers.ProcessLogging(po)
				checkErr(err, "")
				for _, f := range createdFiles {
					log.Info().Str("file", f).Msg("Created file")
					hashes = append(hashes, helpers.HashSet{
						Filename: strings.TrimPrefix(f, outDir+"/"),
						Hash:     helpers.MustGetSha256(f),
					})
				}
			} else {
				log.Warn().Msg("Only creating inventory file, no suitcase archives")
			}
			return nil
		},
		PersistentPreRun: func(cmd *cobra.Command, args []string) {
			setupMultiLogging()
			hashes = []helpers.HashSet{}
			// Set up new CLI meta stuff
			cliMeta = cmdhelpers.NewCLIMeta(args, cmd)
		},
		PersistentPostRun: func(cmd *cobra.Command, args []string) {
			metaF, err := cliMeta.Complete(outDir)
			checkErr(err, "")

			log.Info().Str("file", metaF).Msg("Created meta file")

			// Hash the outer items if asked
			hashOuter, err := cmd.Flags().GetBool("hash-outer")
			checkErr(err, "")
			if hashOuter {
				hashes = append(hashes, helpers.HashSet{
					Filename: strings.TrimPrefix(metaF, outDir+"/"),
					Hash:     helpers.MustGetSha256(metaF),
				})

				hashFile := path.Join(outDir, "suitcasectl.sha256")
				log.Info().Str("hash-file", hashFile).Msg("Creating hashes")
				hashF, err := os.Create(hashFile)
				checkErr(err, "")
				defer hashF.Close()
				err = helpers.WriteHashFile(hashes, hashF)
				checkErr(err, "")
			}

			// stats.Runtime = stats.End.Sub(stats.Start)
			log.Info().Str("log-file", logFile).Msg("Log File written")
			log.Info().
				// Dur("runtime", stats.Runtime).
				Str("runtime", cliMeta.CompletedAt.Sub(*cliMeta.StartedAt).String()).
				Str("start", cliMeta.StartedAt.String()).
				Str("end", cliMeta.CompletedAt.String()).
				Msg("Completed")
		},
	}
	cmd.PersistentFlags().Int("concurrency", 10, "Number of concurrent files to create")
	cmd.PersistentFlags().String("inventory-file", "", "Use the given inventory file to create the suitcase")
	cmd.PersistentFlags().String("inventory-format", "yaml", "Format for the inventory. Should be 'yaml' or 'json'")
	cmd.PersistentFlags().String("max-suitcase-size", "0", "Maximum size for the set of suitcases generated. If no unit is specified, 'bytes' is assumed. 0 means no limit.")
	cmd.PersistentFlags().String("internal-metadata-glob", "suitcase-meta*", "Glob pattern for internal metadata files. This should be directly under the top level directories of the targets that are being packaged up. Multiple matches will be included if found.")
	cmd.PersistentFlags().StringArray("external-metadata-file", []string{}, "Additional files to include as metadata in the inventory. This should NOT be part of the suitcase target directories...use internal-metadata-glob for those")
	cmd.PersistentFlags().Bool("hash-inner", false, "Create SHA256 hashes for the inner contents of the suitcase")
	cmd.PersistentFlags().Bool("hash-outer", false, "Create SHA256 hashes for the container and metadata files")
	cmd.PersistentFlags().Bool("encrypt-inner", false, "Encrypt files within the suitcase")
	cmd.PersistentFlags().Int("buffer-size", 1024, "Buffer size if using a YAML inventory.")
	cmd.PersistentFlags().Int("limit-file-count", 0, "Limit the number of files to include in the inventory. If 0, no limit is applied. Should only be used for debugging")
	cmd.PersistentFlags().String("suitcase-format", "tar.gz", "Format of the suitcase. Valid options are: tar, tar.gz, tar.gpg and tar.gz.gpg")
	cmd.PersistentFlags().String("user", "", "Username to insert into the suitcase filename. If omitted, we'll try and detect from the current user")
	cmd.PersistentFlags().String("prefix", "suitcase", "Prefex to insert into the suitcase filename")
	cmd.PersistentFlags().StringArrayP("public-key", "p", []string{}, "Public keys to use for encryption")
	cmd.PersistentFlags().Bool("exclude-systems-pubkeys", false, "By default, we will include the systems teams pubkeys, unless this option is specified")
	cmd.PersistentFlags().Bool("only-inventory", false, "Only generate the inventory file, skip the actual suitcase archive creation")
	cmd.PersistentFlags().StringVarP(&outDir, "output-dir", "o", "", "Directory to write files in to. If not specified, we'll use an auto generated temp dir")
	return cmd
}

func init() {
	createSuitcaseCmd := NewCreateSuitcaseCmd()
	createCmd.AddCommand(createSuitcaseCmd)
}
