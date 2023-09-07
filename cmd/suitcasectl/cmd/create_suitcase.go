package cmd

import (
	"context"
	"errors"
	"fmt"
	"io"
	"os"
	"path"
	"strconv"
	"strings"

	"github.com/drewstinnett/gout/v2"
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
	hashAlgo        inventory.HashAlgorithm = inventory.MD5Hash
)

// NewCreateSuitcaseCmd represents the createSuitcase command
func NewCreateSuitcaseCmd() *cobra.Command {
	cmd := &cobra.Command{
		Use:   "suitcase [--inventory-file=INVENTORY_FILE | TARGET_DIR...]",
		Short: "Create a suitcase",
		Long:  "Create a suitcase from either an inventory file or multiple target directories.",
		Args:  cobra.ArbitraryArgs,
		Aliases: []string{
			"suitecase", // Encouraging bad habits
			"s",
			"sc",
		},
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
		fh, err := os.Open(f) // nolint:gosec
		panicIfErr(err)
		defer dclose(fh)
		log.Info().Str("file", f).Msg("Created file")
		hs = append(hs, config.HashSet{
			Filename: strings.TrimPrefix(f, cmd.Context().Value(inventory.DestinationKey).(string)+"/"),
			Hash:     calculateHash(fh, hashAlgo.String()),
			// Hash:     mustGetSha256(f),
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

	// Hashing Algorithms
	cmd.PersistentFlags().Var(&hashAlgo, "hash-algorithm", "Hashing Algorithm for signatures")
	if err := cmd.RegisterFlagCompletionFunc("hash-algorithm", inventory.HashCompletion); err != nil {
		panic(err)
	}
	cmd.PersistentFlags().Lookup("hash-algorithm").DefValue = "md5"

	// cmd.PersistentFlags().String("suitcase-format", "tar.gz", "Format of the suitcase. Valid options are: tar, tar.gz, tar.gpg and tar.gz.gpg")
	// Inventory Format needs some extra love for auto complete
	cmd.PersistentFlags().Var(&suitcaseFormat, "suitcase-format", "Format for the suitcase. Should be 'tar', 'tar.gpg', 'tar.gz' or 'tar.gz.gpg'")
	if err := cmd.RegisterFlagCompletionFunc("suitcase-format", suitcase.FormatCompletion); err != nil {
		panic(err)
	}
	cmd.PersistentFlags().Lookup("suitcase-format").DefValue = inventory.DefaultSuitcaseFormat

	// Get some exclusivity goin'
	cmd.MarkFlagsMutuallyExclusive("only-inventory", "hash-outer")
}

func userOverridesWithCobra(cmd *cobra.Command, args []string) (*viper.Viper, error) {
	userOverrides := viper.New()
	userOverrides.SetConfigName("suitcasectl")
	for _, dir := range args {
		log.Debug().Str("dir", dir).Msg("Adding target dir")
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

type hashFileCreator func([]config.HashSet, io.Writer) error

// This could definitely be cleaner...
func writeHashFile(cmd *cobra.Command, hfc hashFileCreator, ext string) (string, error) {
	hashFile := path.Join(cmd.Context().Value(inventory.DestinationKey).(string), fmt.Sprintf("suitcasectl.%v", hashAlgo.String()+ext))
	log.Info().Str("hash-file", hashFile).Msg("Creating hashes")
	hashF, err := os.Create(hashFile) // nolint:gosec
	if err != nil {
		return "", err
	}
	defer dclose(hashF)
	err = hfc(cmd.Context().Value(inventory.HashesKey).([]config.HashSet), hashF)
	if err != nil {
		return "", err
	}
	return hashF.Name(), nil
}

// setOuterHashes returns a HashSet, hashFileName, hashFileNameBinary and error
func setOuterHashes(cmd *cobra.Command, metaF string) ([]config.HashSet, string, string, error) {
	hashes, ok := cmd.Context().Value(inventory.HashesKey).([]config.HashSet)
	if !ok {
		return nil, "", "", errors.New("could not find inventory.HashesKey in context")
	}
	// hashes = cmd.Context().Value(inventory.HashesKey).([]config.HashSet)
	metaFh, err := os.Open(metaF) // nolint:gosec
	if err != nil {
		return nil, "", "", err
	}
	defer dclose(metaFh)
	hashes = append(hashes, config.HashSet{
		Filename: strings.TrimPrefix(metaF, cmd.Context().Value(inventory.DestinationKey).(string)+"/"),
		Hash:     calculateHash(metaFh, hashAlgo.String()),
	})

	cmd.SetContext(context.WithValue(cmd.Context(), inventory.HashesKey, hashes))
	hashFn, err := writeHashFile(cmd, suitcase.WriteHashFile, "")
	checkErr(err, "Could not write out the hashfile")

	hashFnBin, err := writeHashFile(cmd, suitcase.WriteHashFileBin, "bin")
	checkErr(err, "Could not write out the binary hashfile")
	return hashes, hashFn, hashFnBin, nil
}

func createPostRunE(cmd *cobra.Command, args []string) error {
	cliMeta := cmd.Context().Value(inventory.CLIMetaKey).(*CLIMeta)
	metaF := cliMeta.MustComplete(cmd.Context().Value(inventory.DestinationKey).(string))
	log.Debug().Str("file", metaF).Msg("Created meta file")

	// Hash the outer items if asked
	var hashes []config.HashSet
	var hashFn, hashFnBin string
	if mustGetCmd[bool](cmd, "hash-outer") && !mustGetCmd[bool](cmd, "only-inventory") {
		var err error
		hashes, hashFn, hashFnBin, err = setOuterHashes(cmd, metaF)
		checkErr(err, "Could not write out the hashfile")
	}

	log.Debug().Str("log-file", cmd.Context().Value(inventory.LogFileKey).(*os.File).Name()).Msg("Switching back to stderr logger and closing the multi log writer so we can hash it")
	setupLogging(cmd.OutOrStderr())
	// Do we really care if this closes? maybe...
	_ = cmd.Context().Value(inventory.LogFileKey).(*os.File).Close()

	log.Info().
		Str("runtime", cliMeta.CompletedAt.Sub(*cliMeta.StartedAt).String()).
		Time("start", *cliMeta.StartedAt).
		Time("end", *cliMeta.CompletedAt).
		Msg("🧳 Completed")

	inv := inventory.WithCmd(cmd)
	opts := suitcase.OptsWithCmd(cmd)
	// Copy files up if needed
	mfiles := []string{
		"inventory.yaml",
		"suitcasectl.log",
		"suitcasectl-invocation-meta.yaml",
	}
	if hashFn != "" {
		mfiles = append(mfiles, path.Base(hashFn))
	}
	if hashFnBin != "" {
		mfiles = append(mfiles, path.Base(hashFnBin))
	}
	if inv.Options.TransportPlugin != nil {
		shipMetadata(mfiles, opts, inv, cmd.Context().Value(inventory.InventoryHash).(string))
	}

	gout.MustPrint(runsum{
		Destination: opts.Destination,
		Suitcases:   inv.UniqueSuitcaseNames(),
		Directories: inv.Options.Directories,
		MetaFiles:   mfiles,
		Hashes:      hashes,
	})
	globalPersistentPostRun(cmd, args)
	return nil
}

func shipMetadata(items []string, opts *config.SuitCaseOpts, inv *inventory.Inventory, uniqDir string) {
	// Running in to a loop issue while this is concurrent
	// var wg conc.WaitGroup
	for _, fn := range items {
		// wg.Go(func() {
		item := path.Join(opts.Destination, fn)
		if err := inv.Options.TransportPlugin.Send(item, uniqDir); err != nil {
			log.Warn().Err(err).Str("file", item).Msg("error copying file")
		}
		// })
	}
	// wg.Wait()
}

type runsum struct {
	Directories []string         `yaml:"directories"`
	Suitcases   []string         `yaml:"suitcases"`
	Destination string           `yaml:"destination"`
	MetaFiles   []string         `yaml:"meta_files"`
	Hashes      []config.HashSet `yaml:"hashes,omitempty"`
}

func createPreRunE(cmd *cobra.Command, args []string) error {
	// Get this first, it'll be important
	globalPersistentPreRun(cmd, args)
	outDir, err := newOutDirWithCmd(cmd)
	if err != nil {
		return err
	}
	// log.Fatal().Msgf("ALGO: %+v\n", hashAlgo)
	cmd.SetContext(context.WithValue(cmd.Context(), inventory.DestinationKey, outDir))
	// cmd.SetContext(context.WithValue(cmd.Context(), inventory.HashTypeKey, hashAlgo))

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

func envOr(e, d string) string {
	got := os.Getenv(e)
	if got == "" {
		return d
	}
	return got
}

func createRunE(cmd *cobra.Command, args []string) error {
	// Try to print any panics in mostly sane way
	defer func() {
		if err := recover(); err != nil {
			log.Fatal().Msg(fmt.Sprint(err))
		}
	}()

	if hasDuplicates(args) {
		return errors.New("duplicate path found in arguments")
	}
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

	// We need options even if we already have the inventory
	opts := &config.SuitCaseOpts{
		Destination:  cmd.Context().Value(inventory.DestinationKey).(string),
		EncryptInner: inventoryD.Options.EncryptInner,
		HashInner:    inventoryD.Options.HashInner,
		Format:       inventoryD.Options.SuitcaseFormat,
	}
	// Store in context for later
	cmd.SetContext(context.WithValue(cmd.Context(), inventory.SuitcaseOptionsKey, opts))

	if !onlyInventory {
		return createSuitcases(cmd, opts, inventoryD)
	}
	log.Warn().Msg("Only creating inventory file, no suitcase archives")
	return nil
}

func createSuitcases(cmd *cobra.Command, opts *config.SuitCaseOpts, inventoryD *inventory.Inventory) error {
	if err := opts.EncryptToCobra(cmd); err != nil {
		return err
	}

	sampleI, err := strconv.Atoi(envOr("SAMPLE_EVERY", "100"))
	if err != nil {
		log.Warn().Err(err).Msg("could not set sampling")
	}
	createdFiles := processSuitcases(&processOpts{
		Inventory:    inventoryD,
		SuitcaseOpts: opts,
		SampleEvery:  sampleI,
		Concurrency:  mustGetCmd[int](cmd, "concurrency"),
	}, cmd)

	if mustGetCmd[bool](cmd, "hash-outer") {
		cmd.SetContext(context.WithValue(cmd.Context(), inventory.HashesKey, createHashes(createdFiles, cmd)))
	}

	return nil
}

func hasDuplicates(strArr []string) bool {
	seen := make(map[string]bool)
	for _, str := range strArr {
		if seen[str] {
			return true
		}
		seen[str] = true
	}
	return false
}
