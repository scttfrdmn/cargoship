package cmd

import (
	"context"
	"errors"
	"os"
	"path"
	"strings"

	"github.com/rs/zerolog/log"
	"github.com/spf13/cobra"
	"github.com/spf13/viper"
	"gitlab.oit.duke.edu/devil-ops/suitcasectl/pkg/config"
	"gitlab.oit.duke.edu/devil-ops/suitcasectl/pkg/inventory"
	"gitlab.oit.duke.edu/devil-ops/suitcasectl/pkg/suitcase"
)

var (
	inventoryFormat inventory.Format
	suitcaseFormat  suitcase.Format
)

// NewCreateSuitcaseCmd represents the createSuitcase command
func NewCreateSuitcaseCmd() *cobra.Command {
	cmd := &cobra.Command{
		Use:                "suitcase [--inventory-file=INVENTORY_FILE | TARGET_DIR...]",
		Short:              "Create a suitcase",
		Long:               "Create a suitcase from either an inventory file or multiple target directories.",
		Args:               cobra.ArbitraryArgs,
		Aliases:            []string{"suitecase"}, // Encouraging bad habits
		RunE:               createRunE,
		PersistentPreRunE:  createPreRunE,
		PersistentPostRunE: createPostRunE,
	}
	bindInventoryCmd(cmd)

	return cmd
}

// ValidateCmdArgs ensures we are passing valid arguments in
func ValidateCmdArgs(inventoryFile string, onlyInventory bool, args []string) error {
	// Figure out if we are using an inventory file, or creating one
	if inventoryFile != "" && len(args) > 0 {
		return errors.New("error: You can't specify an inventory file and target dir arguments at the same time")
	}

	// Make sure we are actually using either an inventory file or target dirs
	if inventoryFile == "" && len(args) == 0 {
		return errors.New("error: You must specify an inventory file or target dirs")
	}

	if onlyInventory && inventoryFile != "" {
		return errors.New("you can't specify an inventory file and only-inventory at the same time")
	}
	return nil
}

func inventoryOptsWithCobra(cmd *cobra.Command, args []string) (string, bool, error) {
	inventoryFile, err := cmd.Flags().GetString("inventory-file")
	if err != nil {
		return "", false, err
	}

	onlyInventory, err := cmd.Flags().GetBool("only-inventory")
	if err != nil {
		return "", false, err
	}

	// Return the error here for use in testing, vs just barfing with checkErr
	if cerr := ValidateCmdArgs(inventoryFile, onlyInventory, args); cerr != nil {
		return "", false, cerr
	}

	return inventoryFile, onlyInventory, nil
}

func createHashes(s []string, cmd *cobra.Command) []config.HashSet {
	var hs []config.HashSet
	for _, f := range s {
		log.Info().Str("file", f).Msg("Created file")
		hs = append(hs, config.HashSet{
			Filename: strings.TrimPrefix(f, cmd.Context().Value(inventory.DestinationKey).(string)+"/"),
			Hash:     mustGetSha256(f),
		})
	}
	return hs
}

func bindInventoryCmd(cmd *cobra.Command) {
	inventory.BindCobra(cmd)
	// Inventory Format needs some extra love for auto complete
	cmd.PersistentFlags().Var(&inventoryFormat, "inventory-format", "Format for the inventory. Should be 'yaml' or 'json'")
	if err := cmd.RegisterFlagCompletionFunc("inventory-format", inventory.FormatCompletion); err != nil {
		panic(err)
	}
	cmd.PersistentFlags().Lookup("inventory-format").DefValue = "yaml"

	// cmd.PersistentFlags().String("suitcase-format", "tar.gz", "Format of the suitcase. Valid options are: tar, tar.gz, tar.gpg and tar.gz.gpg")
	// Inventory Format needs some extra love for auto complete
	cmd.PersistentFlags().Var(&suitcaseFormat, "suitcase-format", "Format for the suitcase. Should be 'tar', 'tar.gpg', 'tar.gz' or 'tar.gz.gpg'")
	if err := cmd.RegisterFlagCompletionFunc("suitcase-format", suitcase.FormatCompletion); err != nil {
		panic(err)
	}
	cmd.PersistentFlags().Lookup("suitcase-format").DefValue = inventory.DefaultSuitcaseFormat
}

func userOverridesWithCobra(cmd *cobra.Command, args []string) (*viper.Viper, error) {
	userOverrides := viper.New()
	userOverrides.SetConfigName("suitcasectl")
	for _, dir := range args {
		log.Info().Str("dir", dir).Msg("Adding target dir to user overrides")
		userOverrides.AddConfigPath(dir)
	}
	if rerr := userOverrides.ReadInConfig(); rerr == nil {
		log.Info().Str("override-file", userOverrides.ConfigFileUsed()).Msg("Found user overrides, using them")
	}
	for _, field := range []string{"follow-symlinks", "ignore-glob", "inventory-format", "internal-metadata-glob", "max-suitcase-size", "prefix", "user", "suitcase-format"} {
		err := userOverrides.BindPFlag(field, cmd.PersistentFlags().Lookup(field))
		if err != nil {
			return nil, err
		}
	}

	// Store in context for later retrieval
	cmd.SetContext(context.WithValue(cmd.Context(), inventory.UserOverrideKey, userOverrides))
	return userOverrides, nil
}

func writeHashFile(cmd *cobra.Command) error {
	hashFile := path.Join(cmd.Context().Value(inventory.DestinationKey).(string), "suitcasectl.sha256")
	log.Info().Str("hash-file", hashFile).Msg("Creating hashes")
	hashF, err := os.Create(hashFile) // nolint:gosec
	if err != nil {
		return err
	}
	defer dclose(hashF)
	err = suitcase.WriteHashFile(cmd.Context().Value(inventory.HashesKey).([]config.HashSet), hashF)
	if err != nil {
		return err
	}
	return nil
}

func createPostRunE(cmd *cobra.Command, args []string) error {
	cliMeta := cmd.Context().Value(inventory.CLIMetaKey).(*CLIMeta)
	metaF := cliMeta.MustComplete(cmd.Context().Value(inventory.DestinationKey).(string))
	log.Info().Str("file", metaF).Msg("Created meta file")

	// Hash the outer items if asked
	if mustGetCmd[bool](cmd, "hash-outer") {
		hashes := cmd.Context().Value(inventory.HashesKey).([]config.HashSet)
		hashes = append(hashes, config.HashSet{
			Filename: strings.TrimPrefix(metaF, cmd.Context().Value(inventory.DestinationKey).(string)+"/"),
			Hash:     mustGetSha256(metaF),
		})
		cmd.SetContext(context.WithValue(cmd.Context(), inventory.HashesKey, hashes))
		err := writeHashFile(cmd)
		checkErr(err, "Could not write out the hashfile")
	}

	// stats.Runtime = stats.End.Sub(stats.Start)
	log.Info().Str("log-file", cmd.Context().Value(inventory.LogFileKey).(*os.File).Name()).Msg("Switching back to stderr logger and closing the multi log writer so we can hash it")
	setupLogging(cmd.OutOrStderr())
	// Do we really care if this closes? maybe...
	_ = cmd.Context().Value(inventory.LogFileKey).(*os.File).Close()

	log.Info().
		Str("runtime", cliMeta.CompletedAt.Sub(*cliMeta.StartedAt).String()).
		Time("start", *cliMeta.StartedAt).
		Time("end", *cliMeta.CompletedAt).
		Msg("Completed")

	globalPersistentPostRun(cmd, args)
	return nil
}

func createPreRunE(cmd *cobra.Command, args []string) error {
	// Get this first, it'll be important
	outDir, err := newOutDirWithCmd(cmd)
	if err != nil {
		return err
	}
	cmd.SetContext(context.WithValue(cmd.Context(), inventory.DestinationKey, outDir))

	err = setupMultiLoggingWithCmd(cmd)
	if err != nil {
		return err
	}

	userOverrides, err := userOverridesWithCobra(cmd, args)
	checkErr(err, "")
	cliMeta := NewCLIMeta(args, cmd)
	cliMeta.ViperConfig = userOverrides.AllSettings()

	cmd.SetContext(context.WithValue(cmd.Context(), inventory.CLIMetaKey, cliMeta))
	return nil
}

func createRunE(cmd *cobra.Command, args []string) error {
	// Get option bits
	inventoryFile, onlyInventory, err := inventoryOptsWithCobra(cmd, args)
	if err != nil {
		return err
	}

	// Create an inventory file if one isn't specified
	// inventoryD, err := inventory.CreateOrReadInventory(inventoryFile, userOverrides, args, cmd.Context().Value(destinationKey).(string), version)
	inventoryD, err := inventory.CreateOrReadInventory(
		inventoryFile,
		cmd,
		args,
		version,
	)
	if err != nil {
		return err
	}

	if !onlyInventory {
		opts := &config.SuitCaseOpts{
			Destination:  cmd.Context().Value(inventory.DestinationKey).(string),
			EncryptInner: inventoryD.Options.EncryptInner,
			HashInner:    inventoryD.Options.HashInner,
			Format:       inventoryD.Options.SuitcaseFormat,
		}
		err := opts.EncryptToCobra(cmd)
		if err != nil {
			return err
		}

		po := &processOpts{
			Inventory:    inventoryD,
			SuitcaseOpts: opts,
			SampleEvery:  100,
			Concurrency:  mustGetCmd[int](cmd, "concurrency"),
		}
		createdFiles := processSuitcases(po)
		if mustGetCmd[bool](cmd, "hash-outer") {
			hashes := createHashes(createdFiles, cmd)
			cmd.SetContext(context.WithValue(cmd.Context(), inventory.HashesKey, hashes))
		}
		return nil
	}
	log.Warn().Msg("Only creating inventory file, no suitcase archives")
	return nil
}
