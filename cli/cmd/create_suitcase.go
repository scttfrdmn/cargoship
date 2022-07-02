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
	"path"
	"strings"

	"github.com/rs/zerolog/log"
	"gitlab.oit.duke.edu/oit-ssi-systems/data-suitcase/pkg/config"
	"gitlab.oit.duke.edu/oit-ssi-systems/data-suitcase/pkg/gpg"
	"gitlab.oit.duke.edu/oit-ssi-systems/data-suitcase/pkg/inventory"
	"gitlab.oit.duke.edu/oit-ssi-systems/data-suitcase/pkg/suitcase"

	"github.com/spf13/cobra"
	"gopkg.in/yaml.v2"
)

// createSuitcaseCmd represents the createSuitcase command
var createSuitcaseCmd = &cobra.Command{
	Use:   "suitcase DESTINATION_DIR",
	Short: "Create a new suitcase, which is a binary blob of data",
	Args:  cobra.ExactArgs(1),
	Run: func(cmd *cobra.Command, args []string) {
		inventoryF, err := cmd.Flags().GetString("inventory")
		checkErr(err, "")
		name, err := cmd.Flags().GetString("name")
		checkErr(err, "")
		format, err := cmd.Flags().GetString("format")
		checkErr(err, "")

		encryptInner, err := cmd.Flags().GetBool("encrypt-inner")
		checkErr(err, "")

		// Set up options
		opts := &config.SuitCaseOpts{
			Destination:  args[0],
			EncryptInner: encryptInner,
		}

		opts.EncryptTo, err = gpg.EncryptToWithCmd(cmd)
		checkErr(err, "")

		yfile, err := ioutil.ReadFile(inventoryF)
		checkErr(err, "")
		var inventory inventory.DirectoryInventory
		err = yaml.Unmarshal(yfile, &inventory)
		checkErr(err, "")
		// opts.Inventory = &inventory

		// here is where we create the actual files
		for i := 1; i <= inventory.TotalIndexes; i++ {
			targetF := path.Join(opts.Destination, fmt.Sprintf("%v-%d-%v", name, i, format))
			target, err := os.Create(targetF)
			checkErr(err, "")
			defer target.Close()

			opts.Format = strings.TrimPrefix(format, ".")

			s, err := suitcase.New(target, opts)
			checkErr(err, "")
			defer s.Close()

			log.Info().
				Str("destination", targetF).
				Str("format", opts.Format).
				Bool("encryptInner", opts.EncryptInner).
				Int("index", i).
				Msg("Filling suitcase")
			err = suitcase.FillWithInventory(s, &inventory, i)
			checkErr(err, "")
		}
	},
}

func init() {
	createCmd.AddCommand(createSuitcaseCmd)

	// Here you will define your flags and configuration settings.

	// Cobra supports Persistent Flags which will work for this command
	// and all subcommands, e.g.:
	createSuitcaseCmd.PersistentFlags().StringP("inventory", "i", "", "Inventory file for the suitcase")
	createSuitcaseCmd.MarkPersistentFlagRequired("inventory")
	createSuitcaseCmd.PersistentFlags().Bool("encrypt-inner", false, "Encrypt files within the suitcase")
	createSuitcaseCmd.PersistentFlags().Bool("exclude-systems-pubkeys", false, "By default, we will include the systems teams pubkeys, unless this option is specified")
	createSuitcaseCmd.PersistentFlags().String("name", "suitcase", "Name of the suitcase")
	createSuitcaseCmd.PersistentFlags().String("format", "tar.gz", "Format of the suitcase. Valid options are: tar, tar.gz, tar.gpg and tar.gz.gpg")

	createSuitcaseCmd.Flags().StringArrayP("public-key", "p", []string{}, "Public keys to use for encryption")

	// Cobra supports local flags which will only run when this command
	// is called directly, e.g.:
	// createSuitcaseCmd.Flags().BoolP("toggle", "t", false, "Help message for toggle")
}
