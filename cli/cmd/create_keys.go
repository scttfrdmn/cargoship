/*
Copyright © 2022 Drew Stinnett <drew@drewlink.com>

Permission is hereby granted, free of charge, to any person obtaining a copy
of this software and associated documentation files (the "Software"), to deal
in the Software without restriction, including without limitation the rights
to use, copy, modify, merge, publish, distribute, sublicense, and/or sell
copies of the Software, and to permit persons to whom the Software is
furnished to do so, subject to the following conditions:

The above copyright notice and this permission notice shall be included in
all copies or substantial portions of the Software.

THE SOFTWARE IS PROVIDED "AS IS", WITHOUT WARRANTY OF ANY KIND, EXPRESS OR
IMPLIED, INCLUDING BUT NOT LIMITED TO THE WARRANTIES OF MERCHANTABILITY,
FITNESS FOR A PARTICULAR PURPOSE AND NONINFRINGEMENT. IN NO EVENT SHALL THE
AUTHORS OR COPYRIGHT HOLDERS BE LIABLE FOR ANY CLAIM, DAMAGES OR OTHER
LIABILITY, WHETHER IN AN ACTION OF CONTRACT, TORT OR OTHERWISE, ARISING FROM,
OUT OF OR IN CONNECTION WITH THE SOFTWARE OR THE USE OR OTHER DEALINGS IN
THE SOFTWARE.
*/
package cmd

import (
	"github.com/rs/zerolog/log"
	"github.com/spf13/cobra"
	"gitlab.oit.duke.edu/devil-ops/data-suitcase/pkg/gpg"
)

// createKeysCmd represents the createKeys command
var createKeysCmd = &cobra.Command{
	Use:   "keys",
	Short: "Create a new private and public key pair",
	Run: func(cmd *cobra.Command, args []string) {
		var err error
		keyOpts := &gpg.KeyOpts{}

		keyOpts.Name, err = cmd.Flags().GetString("name")
		checkErr(err, "")
		keyOpts.Email, err = cmd.Flags().GetString("email")
		checkErr(err, "")
		keyOpts.KeyType, err = cmd.Flags().GetString("type")
		checkErr(err, "")
		keyOpts.Bits, err = cmd.Flags().GetInt("bits")
		checkErr(err, "")

		kp, err := gpg.NewKeyPair(keyOpts)
		checkErr(err, "")

		dest, err := cmd.Flags().GetString("destination")
		checkErr(err, "")

		created, err := gpg.NewKeyFilesWithPair(kp, dest)
		checkErr(err, "")
		log.Info().
			Strs("created", created).
			Msg("Create key files")
	},
}

func init() {
	createCmd.AddCommand(createKeysCmd)

	// Here you will define your flags and configuration settings.

	// Cobra supports Persistent Flags which will work for this command
	// and all subcommands, e.g.:
	createKeysCmd.PersistentFlags().StringP("name", "n", "", "Name of the key")
	err := createKeysCmd.MarkPersistentFlagRequired("name")
	checkErr(err, "")
	createKeysCmd.PersistentFlags().StringP("email", "e", "", "Email of the key")
	err = createKeysCmd.MarkPersistentFlagRequired("email")
	checkErr(err, "")
	createKeysCmd.PersistentFlags().StringP("type", "t", "", "Type of the key")
	createKeysCmd.PersistentFlags().IntP("bits", "b", 4096, "Bit length of the key")

	createKeysCmd.PersistentFlags().StringP("destination", "d", "", "Destination directory for the files")

	// Cobra supports local flags which will only run when this command
	// is called directly, e.g.:
	// createKeysCmd.Flags().BoolP("toggle", "t", false, "Help message for toggle")
}
