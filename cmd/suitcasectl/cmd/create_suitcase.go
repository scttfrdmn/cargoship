package cmd

import (
	"context"
	"fmt"
	"io"
	"os"
	"path"
	"strings"
	"sync"
	"sync/atomic"
	"time"

	"github.com/drewstinnett/gout/v2"
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
	if cerr := validateCmdArgs(inventoryFile, onlyInventory, *cmd, args); cerr != nil {
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
		logger.Debug("adding target dir", "dir", dir)
		userOverrides.AddConfigPath(dir)
	}
	if rerr := userOverrides.ReadInConfig(); rerr == nil {
		logger.Info("found user overrides", "file", userOverrides.ConfigFileUsed())
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
	logger.Info("creating hashes", "file", hashFile)
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

	logger.Info("creating hash for file", "file", metaF)
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

func appendHashes(mfiles []string, items ...string) []string {
	for _, item := range items {
		if item != "" {
			mfiles = append(mfiles, path.Base(item))
		}
	}
	return mfiles
}

func createPostRunE(cmd *cobra.Command, args []string) error {
	ptr := mustPorterWithCmd(cmd)
	// gout.MustPrint(ptr)
	metaF := ptr.CLIMeta.MustComplete(ptr.Destination)
	logger.Debug("created meta file", "file", metaF)

	// Hash the outer items if asked
	var hashes []config.HashSet
	var hashFn, hashFnBin string
	if mustGetCmd[bool](cmd, "hash-outer") && !mustGetCmd[bool](cmd, "only-inventory") {
		var err error
		if hashes, hashFn, hashFnBin, err = setOuterHashes(ptr, metaF); err != nil {
			return err
		}
	}

	logger.Debug("switching back to stderr logger and closing the multi log writer so we can hash it", "log", ptr.LogFile.Name())
	setupLogging(cmd.OutOrStderr())
	// Do we really care if this closes? maybe...
	_ = ptr.LogFile.Close()
	logger.Info("completed",
		"runtime", ptr.CLIMeta.CompletedAt.Sub(*ptr.CLIMeta.StartedAt).String(),
		"start", *ptr.CLIMeta.StartedAt,
		"end", *ptr.CLIMeta.CompletedAt,
	)

	// opts := suitcase.OptsWithCmd(cmd)
	// Copy files up if needed
	mfiles := appendHashes([]string{
		"inventory.yaml",
		"suitcasectl.log",
		"suitcasectl-invocation-meta.yaml",
	}, hashFn, hashFnBin)
	if ptr.Inventory.Options.TransportPlugin != nil {
		ptr.ShipItems(mfiles, ptr.InventoryHash)
	}

	if err := uploadMeta(ptr, mfiles); err != nil {
		return err
	}

	if serr := ptr.SendUpdate(travelagent.StatusUpdate{
		Status:      travelagent.StatusComplete,
		CompletedAt: nowPtr(),
		SizeBytes:   ptr.TotalTransferred,
	}); serr != nil {
		logger.Warn("failed to send final status update", "error", serr)
	}

	gout.MustPrint(runsum{
		Destination: ptr.Destination,
		Suitcases:   ptr.Inventory.UniqueSuitcaseNames(),
		Directories: ptr.Inventory.Options.Directories,
		MetaFiles:   mfiles,
		Hashes:      hashes,
	})
	globalPersistentPostRun(cmd, args)
	return nil
}

func uploadMeta(ptr *porter.Porter, mfiles []string) error {
	if ptr.TravelAgent != nil {
		var wg sync.WaitGroup
		wg.Add(len(mfiles))
		for _, mfile := range mfiles {
			mfile := mfile
			go func() {
				defer wg.Done()
				var xferred int64
				var err error
				if xferred, err = ptr.TravelAgent.Upload(path.Join(ptr.Destination, mfile), nil); err != nil {
					panic(err)
				}
				atomic.AddInt64(&ptr.TotalTransferred, xferred)
			}()
		}
		wg.Wait()
	}
	return nil
}

func nowPtr() *time.Time {
	n := time.Now()
	return &n
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

	cmdOpts, err := porterOptsWithCmd(cmd, args)
	checkErr(err, "could not get porter options")
	opts := append(
		[]porter.Option{
			porter.WithLogger(logger),
			porter.WithHashAlgorithm(hashAlgo),
			porter.WithVersion(version),
		},
		cmdOpts...)

	// Shove porter in to the cmd context so we can use it later
	cmd.SetContext(context.WithValue(cmd.Context(), porter.PorterKey, porter.New(opts...)))

	return setupMultiLoggingWithCmd(cmd)
}

func porterOptsWithCmd(cmd *cobra.Command, args []string) ([]porter.Option, error) {
	outDir, err := newOutDirWithCmd(cmd)
	if err != nil {
		return nil, err
	}
	userOverrides, err := userOverridesWithCobra(cmd, args)
	if err != nil {
		return nil, err
	}
	cliMeta := porter.NewCLIMetaWithCobra(cmd, args)
	cliMeta.ViperConfig = userOverrides.AllSettings()
	return []porter.Option{
		porter.WithCmdArgs(cmd, args),
		porter.WithUserOverrides(userOverrides),
		porter.WithCLIMeta(cliMeta),
		porter.WithDestination(outDir),
	}, nil
}

func createRunE(cmd *cobra.Command, args []string) error { // nolint:funlen
	// Try to print any panics in mostly sane way
	defer func() {
		if err := recover(); err != nil {
			logger.Error("fatal error", "error", fmt.Sprint(err))
			os.Exit(3)
		}
	}()

	ptr, onlyInventory, err := porterTravelAgentWithCmd(cmd, args)
	if err != nil {
		return err
	}

	if !onlyInventory {
		if err = ptr.SendUpdate(travelagent.StatusUpdate{
			SuitcasectlSource:      strings.Join(args, ", "),
			Status:                 travelagent.StatusInProgress,
			StartedAt:              nowPtr(),
			SuitcasectlDestination: ptr.Destination,
			Metadata:               ptr.Inventory.MustJSONString(),
			MetadataCheckSum:       ptr.InventoryHash,
		}); err != nil {
			logger.Warn("error sending status update", "error", err)
		}

		if cerr := createSuitcases(ptr); cerr != nil {
			return cerr
		}
		return nil
	}
	logger.Warn("only creating inventory file, no suitcases")
	return nil
}

// func createSuitcases(cmd *cobra.Command, opts *config.SuitCaseOpts, inventoryD *inventory.Inventory) error {
func createSuitcases(ptr *porter.Porter) error {
	if err := ptr.SuitcaseOpts.EncryptToCobra(ptr.Cmd); err != nil {
		return err
	}

	if ptr.Cmd != nil {
		ptr.SetConcurrency(mustGetCmd[int](ptr.Cmd, "concurrency"))
		ptr.SetRetries(
			mustGetCmd[int](ptr.Cmd, "retry-count"),
			mustGetCmd[time.Duration](ptr.Cmd, "retry-interval"),
		)
	}
	return ptr.Run()
}
