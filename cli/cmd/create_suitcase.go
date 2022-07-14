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
	"encoding/json"
	"io/ioutil"
	"strings"

	"gitlab.oit.duke.edu/devil-ops/data-suitcase/cli/cmdhelpers"
	"gitlab.oit.duke.edu/devil-ops/data-suitcase/pkg/config"
	"gitlab.oit.duke.edu/devil-ops/data-suitcase/pkg/gpg"
	"gitlab.oit.duke.edu/devil-ops/data-suitcase/pkg/inventory"

	"github.com/rs/zerolog/log"
	"github.com/spf13/cobra"
)

// createSuitcaseCmd represents the createSuitcase command
var createSuitcaseCmd = &cobra.Command{
	Use:   "suitcase DESTINATION_DIR",
	Short: "Create a new suitcase, which is a binary blob of data",
	Args:  cobra.ExactArgs(1),
	Run: func(cmd *cobra.Command, args []string) {
		inventoryF, err := cmd.Flags().GetString("inventory")
		checkErr(err, "")
		format, err := cmd.Flags().GetString("format")
		checkErr(err, "")
		concurrency, err := cmd.Flags().GetInt("concurrency")
		checkErr(err, "")
		pbarType, err := cmd.Flags().GetString("progress-bar")
		checkErr(err, "")

		// encryptInnerCli, err := cmd.Flags().GetBool("encrypt-inner")
		// checkErr(err, "")

		/*
			var needsEncrypt bool
			if encryptInner {
				needsEncrypt = true
			}
		*/

		// Parse in the inventory
		yfile, err := ioutil.ReadFile(inventoryF)
		checkErr(err, "")
		var inventory inventory.DirectoryInventory
		err = json.Unmarshal(yfile, &inventory)
		checkErr(err, "")

		// Set up options
		opts := &config.SuitCaseOpts{
			Destination:  args[0],
			EncryptInner: inventory.Options.EncryptInner,
			Format:       strings.TrimPrefix(format, "."),
		}

		// Gather EncryptTo if we need it
		if strings.HasSuffix(opts.Format, ".gpg") || opts.EncryptInner {
			opts.EncryptTo, err = gpg.EncryptToWithCmd(cmd)
			checkErr(err, "")
		}

		// opts.Inventory = &inventory

		po := &cmdhelpers.ProcessOpts{
			Concurrency:  concurrency,
			Inventory:    &inventory,
			SuitcaseOpts: opts,
		}
		// We may do different things here...
		var createdFiles []string
		switch pbarType {
		case "mpb":
			log.Fatal().Msg("Sorry, haven't actually implemented this yet. Issues with it hiding errors")
			checkErr(err, "")
		case "none":
			createdFiles, err = cmdhelpers.ProcessLogging(po)
			checkErr(err, "")
		}
		for _, f := range createdFiles {
			log.Info().Str("file", f).Msg("Created file")
		}

		// err = cmdhelpers.ProcessLogging(po)
	},
}

func init() {
	createCmd.AddCommand(createSuitcaseCmd)

	// Here you will define your flags and configuration settings.

	// Cobra supports Persistent Flags which will work for this command
	// and all subcommands, e.g.:
	createSuitcaseCmd.PersistentFlags().StringP("inventory", "i", "", "Inventory file for the suitcase")
	err := createSuitcaseCmd.MarkPersistentFlagRequired("inventory")
	checkErr(err, "")
	createSuitcaseCmd.PersistentFlags().Bool("hash-outer", false, "Create SHA256 hashes for the suitcases")
	createSuitcaseCmd.PersistentFlags().Bool("exclude-systems-pubkeys", false, "By default, we will include the systems teams pubkeys, unless this option is specified")
	createSuitcaseCmd.PersistentFlags().String("name", "suitcase", "Name of the suitcase")
	createSuitcaseCmd.PersistentFlags().String("format", "tar.gz", "Format of the suitcase. Valid options are: tar, tar.gz, tar.gpg and tar.gz.gpg")
	createSuitcaseCmd.PersistentFlags().Int("concurrency", 10, "Number of concurrent files to create")
	createSuitcaseCmd.PersistentFlags().String("progress-bar", "none", "Progress bar to use. Valid options are: 'mpb' or 'none'")

	createSuitcaseCmd.Flags().StringArrayP("public-key", "p", []string{}, "Public keys to use for encryption")

	// Cobra supports local flags which will only run when this command
	// is called directly, e.g.:
	// createSuitcaseCmd.Flags().BoolP("toggle", "t", false, "Help message for toggle")
}
