package cmd

import (
	"os"
	"path"
	"strings"

	"github.com/rs/zerolog/log"
	"github.com/spf13/cobra"
	"github.com/spf13/viper"
	"gitlab.oit.duke.edu/devil-ops/suitcasectl/cmd/suitcasectl/cmdhelpers"
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
		Use:               "suitcase [--inventory-file=INVENTORY_FILE | TARGET_DIR...]",
		Short:             "Create a suitcase",
		Long:              "Create a suitcase from either an inventory file or multiple target directories.",
		Args:              cobra.ArbitraryArgs,
		Aliases:           []string{"suitecase"}, // Encouraging bad habits
		RunE:              createRunE,
		PersistentPreRun:  createPreRun,
		PersistentPostRun: createPostRun,
	}
	bindInventoryCmd(cmd)

	return cmd
}

func init() {
	createSuitcaseCmd := NewCreateSuitcaseCmd()
	createCmd.AddCommand(createSuitcaseCmd)
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
	if cerr := cmdhelpers.ValidateCmdArgs(inventoryFile, onlyInventory, args); cerr != nil {
		return "", false, cerr
	}

	return inventoryFile, onlyInventory, nil
}

func createHashes(s []string) []inventory.HashSet {
	var hs []inventory.HashSet
	for _, f := range s {
		log.Info().Str("file", f).Msg("Created file")
		hs = append(hs, inventory.HashSet{
			Filename: strings.TrimPrefix(f, outDir+"/"),
			Hash:     mustGetSha256(f),
		})
	}
	return hs
}

func bindInventoryCmd(cmd *cobra.Command) {
	cmd.PersistentFlags().Int("concurrency", 10, "Number of concurrent files to create")
	cmd.PersistentFlags().String("inventory-file", "", "Use the given inventory file to create the suitcase")

	// Inventory Format needs some extra love for auto complete
	cmd.PersistentFlags().Var(&inventoryFormat, "inventory-format", "Format for the inventory. Should be 'yaml' or 'json'")
	if err := cmd.RegisterFlagCompletionFunc("inventory-format", inventory.FormatCompletion); err != nil {
		panic(err)
	}
	cmd.PersistentFlags().Lookup("inventory-format").DefValue = "yaml"

	cmd.PersistentFlags().String("max-suitcase-size", "0", "Maximum size for the set of suitcases generated. If no unit is specified, 'bytes' is assumed. 0 means no limit.")
	cmd.PersistentFlags().String("internal-metadata-glob", "suitcase-meta*", "Glob pattern for internal metadata files. This should be directly under the top level directories of the targets that are being packaged up. Multiple matches will be included if found.")
	cmd.PersistentFlags().StringArray("external-metadata-file", []string{}, "Additional files to include as metadata in the inventory. This should NOT be part of the suitcase target directories...use internal-metadata-glob for those")
	cmd.PersistentFlags().StringArray("ignore-glob", []string{}, "Ignore files matching this glob pattern. Can be specified multiple times")
	cmd.PersistentFlags().Bool("hash-inner", false, "Create SHA256 hashes for the inner contents of the suitcase")
	cmd.PersistentFlags().Bool("hash-outer", false, "Create SHA256 hashes for the container and metadata files")
	cmd.PersistentFlags().Bool("encrypt-inner", false, "Encrypt files within the suitcase")
	cmd.PersistentFlags().Bool("follow-symlinks", false, "Follow symlinks when traversing the target directories and files")
	cmd.PersistentFlags().Int("buffer-size", 1024, "Buffer size if using a YAML inventory.")
	cmd.PersistentFlags().Int("limit-file-count", 0, "Limit the number of files to include in the inventory. If 0, no limit is applied. Should only be used for debugging")

	// cmd.PersistentFlags().String("suitcase-format", "tar.gz", "Format of the suitcase. Valid options are: tar, tar.gz, tar.gpg and tar.gz.gpg")
	// Inventory Format needs some extra love for auto complete
	cmd.PersistentFlags().Var(&suitcaseFormat, "suitcase-format", "Format for the suitcase. Should be 'tar', 'tar.gpg', 'tar.gz' or 'tar.gz.gpg'")
	if err := cmd.RegisterFlagCompletionFunc("suitcase-format", suitcase.FormatCompletion); err != nil {
		panic(err)
	}
	cmd.PersistentFlags().Lookup("suitcase-format").DefValue = inventory.DefaultSuitcaseFormat

	cmd.PersistentFlags().String("user", "", "Username to insert into the suitcase filename. If omitted, we'll try and detect from the current user")
	cmd.PersistentFlags().String("prefix", "suitcase", "Prefix to insert into the suitcase filename")
	cmd.PersistentFlags().StringArrayP("public-key", "p", []string{}, "Public keys to use for encryption")
	cmd.PersistentFlags().Bool("exclude-systems-pubkeys", false, "By default, we will include the systems teams pubkeys, unless this option is specified")
	cmd.PersistentFlags().Bool("only-inventory", false, "Only generate the inventory file, skip the actual suitcase archive creation")
}

func userOverridesWithCobra(cmd *cobra.Command, args []string) (*viper.Viper, error) {
	userOverrides = viper.New()
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
	return userOverrides, nil
}

func writeHashFile() error {
	hashFile := path.Join(outDir, "suitcasectl.sha256")
	log.Info().Str("hash-file", hashFile).Msg("Creating hashes")
	hashF, err := os.Create(hashFile) // nolint:gosec
	if err != nil {
		return err
	}
	defer dclose(hashF)
	err = inventory.WriteHashFile(hashes, hashF)
	if err != nil {
		return err
	}
	return nil
}

func createPostRun(cmd *cobra.Command, args []string) {
	metaF := cliMeta.MustComplete(outDir)
	log.Info().Str("file", metaF).Msg("Created meta file")

	// Hash the outer items if asked
	if mustGetCmd[bool](cmd, "hash-outer") {
		hashes = append(hashes, inventory.HashSet{
			Filename: strings.TrimPrefix(metaF, outDir+"/"),
			Hash:     mustGetSha256(metaF),
		})
		err := writeHashFile()
		checkErr(err, "")
	}

	// stats.Runtime = stats.End.Sub(stats.Start)
	log.Info().Str("log-file", logFile).Msg("Switching back to stderr logger and closing the multi log writer so we can hash it")
	setupLogging(os.Stderr)
	// Do we really care if this closes? maybe...
	_ = logF.Close()

	log.Info().
		Str("runtime", cliMeta.CompletedAt.Sub(*cliMeta.StartedAt).String()).
		Time("start", *cliMeta.StartedAt).
		Time("end", *cliMeta.CompletedAt).
		Msg("Completed")
}

func createPreRun(cmd *cobra.Command, args []string) {
	// Get this first, it'll be important
	var err error
	outDir, err = newOutDirWithCmd(cmd)
	checkErr(err, "Could not figure out the output directory")

	err = setupMultiLogging(outDir)
	if err != nil {
		panic(err)
	}
	hashes = []inventory.HashSet{}
	cliMeta = cmdhelpers.NewCLIMeta(args, cmd)

	userOverrides, err = userOverridesWithCobra(cmd, args)
	checkErr(err, "")
	cliMeta.ViperConfig = userOverrides.AllSettings()
}

func createRunE(cmd *cobra.Command, args []string) error {
	// Get option bits
	inventoryFile, onlyInventory, err := inventoryOptsWithCobra(cmd, args)
	if err != nil {
		return err
	}

	// Create an inventory file if one isn't specified
	inventoryD, err := inventory.CreateOrReadInventory(inventoryFile, userOverrides, args, outDir, version)
	if err != nil {
		return err
	}

	if !onlyInventory {
		opts := &config.SuitCaseOpts{
			Destination:  outDir,
			EncryptInner: inventoryD.Options.EncryptInner,
			HashInner:    inventoryD.Options.HashInner,
			Format:       inventoryD.Options.SuitcaseFormat,
		}
		err := opts.EncryptToCobra(cmd)
		if err != nil {
			return err
		}

		po := &cmdhelpers.ProcessOpts{
			Inventory:    inventoryD,
			SuitcaseOpts: opts,
			Concurrency:  mustGetCmd[int](cmd, "concurrency"),
		}
		createdFiles, err := cmdhelpers.ProcessLogging(po)
		if err != nil {
			return err
		}
		hashes = createHashes(createdFiles)
		return nil
	}
	log.Warn().Msg("Only creating inventory file, no suitcase archives")
	return nil
}
