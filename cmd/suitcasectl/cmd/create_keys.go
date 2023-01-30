package cmd

import (
	"github.com/rs/zerolog/log"
	"github.com/spf13/cobra"
	"gitlab.oit.duke.edu/devil-ops/suitcasectl/pkg/gpg"
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

		outDir, err = cmd.Flags().GetString("output-dir")
		checkErr(err, "")

		created, err := gpg.NewKeyFilesWithPair(kp, outDir)
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
	createKeysCmd.PersistentFlags().String("type", "", "Type of the key")
	createKeysCmd.PersistentFlags().IntP("bits", "b", 4096, "Bit length of the key")

	createKeysCmd.PersistentFlags().StringP("destination", "d", "", "Destination directory for the files")

	// Cobra supports local flags which will only run when this command
	// is called directly, e.g.:
	// createKeysCmd.Flags().BoolP("toggle", "t", false, "Help message for toggle")
}
