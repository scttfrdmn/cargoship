package cmd

import (
	"errors"
	"fmt"
	"os"

	"github.com/scttfrdmn/cargoship/pkg/gpg"
	"github.com/spf13/cobra"
	"golang.org/x/term"
)

var gpgKeyType gpg.KeyType

// passphraseEnvVar is the non-interactive source for the key passphrase.
//
// There is deliberately no --passphrase flag: a flag value lands in shell
// history, in `ps` output for the lifetime of the process, and in any CI log
// that echoes its command line.
const passphraseEnvVar = "CARGOSHIP_GPG_PASSPHRASE"

// NewCreateKeysCmd generates a new subcommand for creating key pairs
func NewCreateKeysCmd() *cobra.Command {
	return &cobra.Command{
		Use:   "keys",
		Short: "Create a new private and public key pair",
		Long: `Create a new GPG private and public key pair.

The private key is encrypted with a passphrase. Supply it interactively when
prompted, or set ` + passphraseEnvVar + ` for non-interactive use. To generate an
unencrypted private key, pass --no-passphrase explicitly.

Keys are written to the current directory unless --destination is given. An
existing private.key or public.key is never overwritten.`,
		RunE: func(cmd *cobra.Command, _ []string) error {
			name, err := cmd.Flags().GetString("name")
			if err != nil {
				return fmt.Errorf("could not get name flag: %w", err)
			}
			email, err := cmd.Flags().GetString("email")
			if err != nil {
				return fmt.Errorf("could not get email flag: %w", err)
			}
			bits, err := cmd.Flags().GetInt("bits")
			if err != nil {
				return fmt.Errorf("could not get bits flag: %w", err)
			}
			noPassphrase, err := cmd.Flags().GetBool("no-passphrase")
			if err != nil {
				return fmt.Errorf("could not get no-passphrase flag: %w", err)
			}

			// Get destination from parent create command's persistent flag
			outDir, err := cmd.Flags().GetString("destination")
			if err != nil {
				return fmt.Errorf("could not get destination flag: %w", err)
			}

			var passphrase []byte
			if !noPassphrase {
				passphrase, err = resolvePassphrase(cmd)
				if err != nil {
					return err
				}
			}

			keyOpts := &gpg.KeyOpts{
				Name:             name,
				Email:            email,
				Bits:             bits,
				KeyType:          gpgKeyType.String(),
				Passphrase:       passphrase,
				AllowUnprotected: noPassphrase,
			}

			kp, err := gpg.NewKeyPair(keyOpts)
			if err != nil {
				return err
			}

			created, err := gpg.NewKeyFilesWithPair(kp, outDir)
			if err != nil {
				if errors.Is(err, gpg.ErrKeyFileExists) {
					return fmt.Errorf("%w\nrefusing to overwrite: a replaced private key cannot be recovered. "+
						"Move the existing keys aside, or use --destination to write elsewhere", err)
				}
				return err
			}
			logger.Info("created key files", "created", created)
			if noPassphrase {
				logger.Warn("the private key is NOT encrypted; protect the file accordingly",
					"private_key", created[0])
			}
			return nil
		},
	}
}

// resolvePassphrase returns the passphrase from the environment, or prompts for
// it twice on an interactive terminal. It never returns an empty passphrase, and
// the value is never logged or echoed.
func resolvePassphrase(cmd *cobra.Command) ([]byte, error) {
	if env, ok := os.LookupEnv(passphraseEnvVar); ok {
		if env == "" {
			return nil, fmt.Errorf("%s is set but empty; unset it and use --no-passphrase to generate an unencrypted key", passphraseEnvVar)
		}
		return []byte(env), nil
	}

	if !term.IsTerminal(int(os.Stdin.Fd())) {
		return nil, fmt.Errorf("no passphrase available: stdin is not a terminal, so it cannot be prompted for.\n"+
			"Set %s, or pass --no-passphrase to generate an unencrypted private key deliberately", passphraseEnvVar)
	}

	// Prompt-write errors are ignored deliberately: a failed prompt does not stop
	// the terminal read below, and failing here would be worse than a missing
	// prompt. The read itself is checked.
	out := cmd.OutOrStdout()
	_, _ = fmt.Fprint(out, "Passphrase for the new private key: ")
	first, err := term.ReadPassword(int(os.Stdin.Fd()))
	_, _ = fmt.Fprintln(out)
	if err != nil {
		return nil, fmt.Errorf("reading passphrase: %w", err)
	}
	_, _ = fmt.Fprint(out, "Confirm passphrase: ")
	second, err := term.ReadPassword(int(os.Stdin.Fd()))
	_, _ = fmt.Fprintln(out)
	if err != nil {
		return nil, fmt.Errorf("reading passphrase confirmation: %w", err)
	}

	if len(first) == 0 {
		return nil, errors.New("passphrase must not be empty; use --no-passphrase to generate an unencrypted key deliberately")
	}
	if string(first) != string(second) {
		return nil, errors.New("passphrases do not match")
	}
	return first, nil
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

	createKeysCmd.PersistentFlags().Bool("no-passphrase", false,
		"Generate an UNENCRYPTED private key (not recommended; the file is plaintext key material)")
	if perr := createKeysCmd.RegisterFlagCompletionFunc("no-passphrase", func(_ *cobra.Command, _ []string, _ string) ([]string, cobra.ShellCompDirective) {
		return nil, cobra.ShellCompDirectiveNoFileComp
	}); perr != nil {
		panic(perr)
	}
}
