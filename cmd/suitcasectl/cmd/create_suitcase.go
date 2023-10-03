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
	"time"

	"github.com/drewstinnett/gout/v2"
	"github.com/rs/zerolog/log"
	"github.com/spf13/cobra"
	"github.com/spf13/viper"

	porter "gitlab.oit.duke.edu/devil-ops/suitcasectl/pkg"
	"gitlab.oit.duke.edu/devil-ops/suitcasectl/pkg/config"
	"gitlab.oit.duke.edu/devil-ops/suitcasectl/pkg/inventory"
	"gitlab.oit.duke.edu/devil-ops/suitcasectl/pkg/suitcase"
	"gitlab.oit.duke.edu/devil-ops/suitcasectl/pkg/travelagent"
)

var (
	inventoryFormat inventory.Format
	suitcaseFormat  suitcase.Format
	hashAlgo        inventory.HashAlgorithm = inventory.MD5Hash
)

// NewCreateSuitcaseCmd represents the createSuitcase command
func NewCreateSuitcaseCmd() *cobra.Command {
	cmd := &cobra.Command{
		Use:     "suitcase [--inventory-file=INVENTORY_FILE | TARGET_DIR...]",
		Short:   "Create a suitcase",
		Long:    "Create a suitcase from either an inventory file or multiple target directories.",
		Args:    cobra.ArbitraryArgs,
		Version: version, // Needed so we can put the version in the metadata
		Example: ` # Create a suitcase using the defaults. This will land in a new TEMPDIR on your host:
$ suitcasectl create suitcase ~/example
...

# Specify a destination for the generated files:
$ suitcasectl create suitcase ~/example --destination=/srv/storage
...

# Specify a maximum suitcase size:
$ suitcasectl create suitcase ~/example --max-suitcase-size=500MiB
...
`,
		Aliases: []string{
			"suitecase", // Encouraging bad habits
			"s",
			"sc",
		},
		RunE:               createRunE,
		PersistentPreRunE:  createPreRunE,
		PersistentPostRunE: createPostRunE,
	}
	err := bindInventoryCmd(cmd)
	panicIfErr(err)
	travelagent.BindCobra(cmd)

	return cmd
}

// validateCmdArgs ensures we are passing valid arguments in
func validateCmdArgs(inventoryFile string, onlyInventory bool, args []string) error {
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
	if cerr := validateCmdArgs(inventoryFile, onlyInventory, args); cerr != nil {
		return "", false, cerr
	}

	return inventoryFile, onlyInventory, nil
}

func bindInventoryCmd(cmd *cobra.Command) error {
	inventory.BindCobra(cmd)
	// Inventory Format needs some extra love for auto complete
	cmd.PersistentFlags().Var(&inventoryFormat, "inventory-format", "Format for the inventory. Should be 'yaml' or 'json'")
	if err := cmd.RegisterFlagCompletionFunc("inventory-format", inventory.FormatCompletion); err != nil {
		return err
	}
	cmd.PersistentFlags().Lookup("inventory-format").DefValue = "yaml"

	// Hashing Algorithms
	cmd.PersistentFlags().Var(&hashAlgo, "hash-algorithm", "Hashing Algorithm for signatures")
	if err := cmd.RegisterFlagCompletionFunc("hash-algorithm", inventory.HashCompletion); err != nil {
		return err
	}
	cmd.PersistentFlags().Lookup("hash-algorithm").DefValue = "md5"

	// Inventory Format needs some extra love for auto complete
	cmd.PersistentFlags().Var(&suitcaseFormat, "suitcase-format", "Format for the suitcase. Should be 'tar', 'tar.gpg', 'tar.gz' or 'tar.gz.gpg'")
	if err := cmd.RegisterFlagCompletionFunc("suitcase-format", suitcase.FormatCompletion); err != nil {
		return err
	}
	cmd.PersistentFlags().Lookup("suitcase-format").DefValue = inventory.DefaultSuitcaseFormat

	// Get some exclusivity goin'
	cmd.MarkFlagsMutuallyExclusive("only-inventory", "hash-outer")
	return nil
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
	return userOverrides, nil
}

type hashFileCreator func([]config.HashSet, io.Writer) error

// This could definitely be cleaner...
func writeHashFile(ptr *porter.Porter, hfc hashFileCreator, ext string) (string, error) {
	hashFile := path.Join(ptr.Destination, fmt.Sprintf("suitcasectl.%v", hashAlgo.String()+ext))
	log.Info().Str("hash-file", hashFile).Msg("Creating hashes")
	hashF, err := os.Create(hashFile) // nolint:gosec
	if err != nil {
		return "", err
	}
	defer dclose(hashF)
	err = hfc(ptr.Hashes, hashF)
	if err != nil {
		return "", err
	}
	return hashF.Name(), nil
}

// setOuterHashes returns a HashSet, hashFileName, hashFileNameBinary and error
func setOuterHashes(ptr *porter.Porter, metaF string) ([]config.HashSet, string, string, error) {
	hashes := ptr.Hashes
	metaFh, err := os.Open(metaF) // nolint:gosec
	if err != nil {
		return nil, "", "", err
	}
	defer dclose(metaFh)
	hashes = append(hashes, config.HashSet{
		Filename: strings.TrimPrefix(metaF, ptr.Destination+"/"),
		Hash:     porter.MustCalculateHash(metaFh, hashAlgo.String()),
	})

	hashFn, err := writeHashFile(ptr, suitcase.WriteHashFile, "")
	checkErr(err, "Could not write out the hashfile")

	hashFnBin, err := writeHashFile(ptr, suitcase.WriteHashFileBin, "bin")
	checkErr(err, "Could not write out the binary hashfile")
	return hashes, hashFn, hashFnBin, nil
}

func createPostRunE(cmd *cobra.Command, args []string) error {
	ptr := mustPorterWithCmd(cmd)
	metaF := ptr.CLIMeta.MustComplete(ptr.Destination)
	log.Debug().Str("file", metaF).Msg("Created meta file")

	// Hash the outer items if asked
	var hashes []config.HashSet
	var hashFn, hashFnBin string
	if mustGetCmd[bool](cmd, "hash-outer") && !mustGetCmd[bool](cmd, "only-inventory") {
		var err error
		hashes, hashFn, hashFnBin, err = setOuterHashes(ptr, metaF)
		if err != nil {
			return err
		}
	}

	log.Debug().Str("log-file", ptr.LogFile.Name()).Msg("Switching back to stderr logger and closing the multi log writer so we can hash it")
	setupLogging(cmd.OutOrStderr())
	// Do we really care if this closes? maybe...
	_ = ptr.LogFile.Close()

	log.Info().
		Str("runtime", ptr.CLIMeta.CompletedAt.Sub(*ptr.CLIMeta.StartedAt).String()).
		Time("start", *ptr.CLIMeta.StartedAt).
		Time("end", *ptr.CLIMeta.CompletedAt).
		Msg("🧳 Completed")

	// opts := suitcase.OptsWithCmd(cmd)
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
	if ptr.Inventory.Options.TransportPlugin != nil {
		ptr.ShipItems(mfiles, ptr.InventoryHash)

		now := time.Now()
		if serr := ptr.SendUpdate(travelagent.StatusUpdate{
			Status:      travelagent.StatusComplete,
			CompletedAt: &now,
		}); serr != nil {
			log.Warn().Err(serr).Msg("failed to send final status update")
		}
	}

	gout.MustPrint(runsum{
		Destination: ptr.SuitcaseOpts.Destination,
		Suitcases:   ptr.Inventory.UniqueSuitcaseNames(),
		Directories: ptr.Inventory.Options.Directories,
		MetaFiles:   mfiles,
		Hashes:      hashes,
	})
	globalPersistentPostRun(cmd, args)
	return nil
}

/*
func shipMetadata(po *porter.Porter, items []string, uniqDir string) {
	// Running in to a loop issue while this is concurrent
	// var wg conc.WaitGroup
	c := make(chan rclone.TransferStatus)
	go func() {
		for {
			status := <-c
			log.Debug().Interface("status", status).Msgf("status update")
			if po.TravelAgent != nil {
				if err := po.SendUpdate(*travelagent.NewStatusUpdate(status)); err != nil {
					log.Warn().Err(err).Msg("could not update travel agent")
				}
			}
		}
	}()

	for _, fn := range items {
		item := path.Join(po.SuitcaseOpts.Destination, fn)
		if err := po.Inventory.Options.TransportPlugin.SendWithChannel(item, uniqDir, c); err != nil {
			log.Warn().Err(err).Str("file", item).Msg("error copying file")
		}
	}
}
*/

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
	/*
		outDir, err := newOutDirWithCmd(cmd)
		if err != nil {
			return err
		}
	*/
	if hasDuplicates(args) {
		return errors.New("duplicate path found in arguments")
	}

	// Check to see if prefix has a slash
	if strings.Contains(mustGetCmd[string](cmd, "prefix"), "/") {
		return errors.New("prefix cannot contain a /")
	}

	userOverrides, err := userOverridesWithCobra(cmd, args)
	checkErr(err, "")
	cliMeta := porter.NewCLIMeta(cmd, args)
	cliMeta.ViperConfig = userOverrides.AllSettings()

	outDir, err := newOutDirWithCmd(cmd)
	if err != nil {
		return err
	}

	// Shove porter in to the cmd context so we can use it later
	cmd.SetContext(context.WithValue(cmd.Context(), porter.PorterKey, porter.New(
		porter.WithCmdArgs(cmd, args),
		porter.WithLogger(&log.Logger),
		porter.WithVersion(version),
		porter.WithDestination(outDir),
		porter.WithHashAlgorithm(hashAlgo),
		porter.WithUserOverrides(userOverrides),
		porter.WithCLIMeta(cliMeta),
	)))

	err = setupMultiLoggingWithCmd(cmd)
	if err != nil {
		return err
	}

	return nil
}

func porterWithCmd(cmd *cobra.Command) (*porter.Porter, error) {
	p, ok := cmd.Context().Value(porter.PorterKey).(*porter.Porter)
	if !ok {
		return nil, errors.New("could not find Porter in this context")
	}
	return p, nil
}

func envOr(e, d string) string {
	got := os.Getenv(e)
	if got == "" {
		return d
	}
	return got
}

func createRunE(cmd *cobra.Command, args []string) error { // nolint:funlen
	// Try to print any panics in mostly sane way
	defer func() {
		if err := recover(); err != nil {
			log.Fatal().Msg(fmt.Sprint(err))
		}
	}()

	var err error
	ptr := mustPorterWithCmd(cmd)
	taOpts := []travelagent.Option{
		travelagent.WithCmd(cmd),
	}
	if Verbose {
		taOpts = append(taOpts, travelagent.WithPrintCurl())
	}
	if ta, terr := travelagent.New(taOpts...); terr != nil {
		log.Debug().Err(terr).Msg("no valid travel agent found")
	} else {
		ptr.SetTravelAgent(ta)
		log.Info().Str("url", ptr.TravelAgent.StatusURL()).Msg("Thanks for using a TravelAgent! Check out this URL for full info on your suitcases fun travel ☀️")
		if serr := ptr.SendUpdate(travelagent.StatusUpdate{
			Status: travelagent.StatusPending,
		}); serr != nil {
			return serr
		}
	}

	// Get option bits
	inventoryFile, onlyInventory, err := inventoryOptsWithCobra(cmd, args)
	if err != nil {
		return err
	}

	// Create an inventory file if one isn't specified
	ptr.Inventory, err = ptr.CreateOrReadInventory(inventoryFile)
	if err != nil {
		return err
	}

	now := time.Now()
	if err = ptr.SendUpdate(travelagent.StatusUpdate{
		SuitcasectlSource:      strings.Join(args, ", "),
		Status:                 travelagent.StatusInProgress,
		StartedAt:              &now,
		SuitcasectlDestination: ptr.Destination,
		Metadata:               ptr.Inventory.MustJSONString(),
		MetadataCheckSum:       ptr.InventoryHash,
	}); err != nil {
		log.Warn().Err(err).Msg("error sending status update")
	}

	// We need options even if we already have the inventory
	ptr.SuitcaseOpts = &config.SuitCaseOpts{
		Destination:  ptr.Destination,
		EncryptInner: ptr.Inventory.Options.EncryptInner,
		HashInner:    ptr.Inventory.Options.HashInner,
		Format:       ptr.Inventory.Options.SuitcaseFormat,
	}

	if !onlyInventory {
		// return createSuitcases(cmd, opts, ptr.Inventory)
		if cerr := createSuitcases(ptr); cerr != nil {
			if serr := ptr.SendUpdate(travelagent.StatusUpdate{
				Status: travelagent.StatusFailed,
			}); serr != nil {
				log.Warn().Err(cerr).Msg("failed to send final status update")
			}
			return err
		}
		return nil
	}
	log.Warn().Msg("Only creating inventory file, no suitcase archives")
	return nil
}

// func createSuitcases(cmd *cobra.Command, opts *config.SuitCaseOpts, inventoryD *inventory.Inventory) error {
func createSuitcases(ptr *porter.Porter) error {
	if err := ptr.SuitcaseOpts.EncryptToCobra(ptr.Cmd); err != nil {
		return err
	}

	sampleI, err := strconv.Atoi(envOr("SAMPLE_EVERY", "100"))
	if err != nil {
		log.Warn().Err(err).Msg("could not set sampling")
	}
	createdFiles := processSuitcases(&processOpts{
		Porter:        ptr,
		SuitcaseOpts:  ptr.SuitcaseOpts,
		SampleEvery:   sampleI,
		Concurrency:   mustGetCmd[int](ptr.Cmd, "concurrency"),
		RetryCount:    mustGetCmd[int](ptr.Cmd, "retry-count"),
		RetryInterval: mustGetCmd[time.Duration](ptr.Cmd, "retry-interval"),
	})

	if mustGetCmd[bool](ptr.Cmd, "hash-outer") {
		ptr.Hashes, err = ptr.CreateHashes(createdFiles)
		if err != nil {
			return err
		}
	}

	return nil
}
