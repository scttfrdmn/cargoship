package cmd

import (
	"github.com/scttfrdmn/cargoship/pkg/gpg"
	"github.com/spf13/cobra"
)

var gpgKeyType gpg.KeyType

// NewCreateKeysCmd generates a new subcommand for creating key pairs
func NewCreateKeysCmd() *cobra.Command {
	return &cobra.Command{
		Use:   "keys",
		Short: "Create a new private and public key pair",
		Run: func(cmd *cobra.Command, _ []string) {
			// Get flags directly
			name, err := cmd.Flags().GetString("name")
			checkErr(err, "could not get name flag")
			email, err := cmd.Flags().GetString("email")
			checkErr(err, "could not get email flag")
			bits, err := cmd.Flags().GetInt("bits")
			checkErr(err, "could not get bits flag")

			keyOpts := &gpg.KeyOpts{
				Name:  name,
				Email: email,
				Bits:  bits,
			}

			// Get destination from parent create command's persistent flag
			outDir, err := cmd.Flags().GetString("destination")
			checkErr(err, "could not get destination flag")

			kp, err := gpg.NewKeyPair(keyOpts)
			checkErr(err, "")

			created, err := gpg.NewKeyFilesWithPair(kp, outDir)
			checkErr(err, "")
			logger.Info("created key files", "created", created)
		},
	}
}

func bindCreateKeys(createCmd *cobra.Command) {
	createKeysCmd := NewCreateKeysCmd()
	createCmd.AddCommand(createKeysCmd)
	createKeysCmd.PersistentFlags().StringP("name", "n", "", "Name of the key")
	err := createKeysCmd.MarkPersistentFlagRequired("name")
	checkErr(err, "")
	if nerr := createKeysCmd.RegisterFlagCompletionFunc("name", func(_ *cobra.Command, _ []string, _ string) ([]string, cobra.ShellCompDirective) {
		return nil, cobra.ShellCompDirectiveNoFileComp
	}); nerr != nil {
		panic(nerr)
	}

	createKeysCmd.PersistentFlags().StringP("email", "e", "", "Email of the key")
	if eerr := createKeysCmd.RegisterFlagCompletionFunc("email", func(_ *cobra.Command, _ []string, _ string) ([]string, cobra.ShellCompDirective) {
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
	if berr := createKeysCmd.RegisterFlagCompletionFunc("bits", func(_ *cobra.Command, _ []string, _ string) ([]string, cobra.ShellCompDirective) {
		return nil, cobra.ShellCompDirectiveNoFileComp
	}); berr != nil {
		panic(berr)
	}
}
