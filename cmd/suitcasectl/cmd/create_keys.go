package cmd

import (
	"github.com/rs/zerolog/log"
	"github.com/spf13/cobra"
	"gitlab.oit.duke.edu/devil-ops/suitcasectl/pkg/gpg"
)

var gpgKeyType gpg.KeyType

// NewCreateKeysCmd generates a new subcommand for creating key pairs
func NewCreateKeysCmd() *cobra.Command {
	return &cobra.Command{
		Use:   "keys",
		Short: "Create a new private and public key pair",
		Run: func(cmd *cobra.Command, args []string) {
			keyOpts := &gpg.KeyOpts{
				Name:  mustGetCmd[string](cmd, "name"),
				Email: mustGetCmd[string](cmd, "email"),
				Bits:  mustGetCmd[int](cmd, "bits"),
			}

			outDir, err := getDestination(cmd, args)
			checkErr(err, "")

			kp, err := gpg.NewKeyPair(keyOpts)
			checkErr(err, "")

			created, err := gpg.NewKeyFilesWithPair(kp, outDir)
			checkErr(err, "")
			log.Info().
				Strs("created", created).
				Msg("Create key files")
		},
	}
}

func bindCreateKeys(createCmd *cobra.Command) {
	createKeysCmd := NewCreateKeysCmd()
	createCmd.AddCommand(createKeysCmd)
	createKeysCmd.PersistentFlags().StringP("name", "n", "", "Name of the key")
	err := createKeysCmd.MarkPersistentFlagRequired("name")
	checkErr(err, "")
	if nerr := createKeysCmd.RegisterFlagCompletionFunc("name", func(cmd *cobra.Command, args []string, toComplete string) ([]string, cobra.ShellCompDirective) {
		return nil, cobra.ShellCompDirectiveNoFileComp
	}); nerr != nil {
		panic(nerr)
	}

	createKeysCmd.PersistentFlags().StringP("email", "e", "", "Email of the key")
	if eerr := createKeysCmd.RegisterFlagCompletionFunc("email", func(cmd *cobra.Command, args []string, toComplete string) ([]string, cobra.ShellCompDirective) {
		return nil, cobra.ShellCompDirectiveNoFileComp
	}); eerr != nil {
		panic(eerr)
	}

	checkErr(err, "")
	createKeysCmd.PersistentFlags().Var(&gpgKeyType, "type", "key type (rsa, x25519)")
	if err := createKeysCmd.RegisterFlagCompletionFunc("type", gpg.KeyTypeCompletion); err != nil {
		panic(err)
	}
	createKeysCmd.PersistentFlags().Lookup("type").DefValue = "rsa"

	createKeysCmd.PersistentFlags().IntP("bits", "b", 4096, "Bit length of the key")
	if berr := createKeysCmd.RegisterFlagCompletionFunc("bits", func(cmd *cobra.Command, args []string, toComplete string) ([]string, cobra.ShellCompDirective) {
		return nil, cobra.ShellCompDirectiveNoFileComp
	}); berr != nil {
		panic(berr)
	}
}
