package cmdhelpers

import (
	"crypto/sha256"
	"fmt"
	"io"
	"io/ioutil"
	"os"
	"os/user"
	"strings"
	"sync"
	"sync/atomic"

	"github.com/dustin/go-humanize"
	"github.com/rs/zerolog/log"
	"github.com/spf13/cobra"
	"github.com/spf13/viper"
	"gitlab.oit.duke.edu/devil-ops/data-suitcase/pkg/config"
	"gitlab.oit.duke.edu/devil-ops/data-suitcase/pkg/helpers"
	"gitlab.oit.duke.edu/devil-ops/data-suitcase/pkg/inventory"
	"gitlab.oit.duke.edu/devil-ops/data-suitcase/pkg/suitcase"
)

type ProcessOpts struct {
	Concurrency  int
	Inventory    *inventory.DirectoryInventory
	SuitcaseOpts *config.SuitCaseOpts
}

func NewOutDirWithCmd(cmd *cobra.Command) (string, error) {
	o, err := cmd.Flags().GetString("output-dir")
	if err != nil {
		return "", err
	}
	if o == "" {
		o, err = ioutil.TempDir("", "suitcasectl-")
		if err != nil {
			return "", err
		}
	}
	return o, nil
}

func ProcessLogging(po *ProcessOpts) ([]string, error) {
	var sm sync.Map
	guard := make(chan struct{}, po.Concurrency)
	var wg sync.WaitGroup
	state := make(chan suitcase.SuitcaseFillState, 1)
	processed := int32(0)
	for i := 1; i <= po.Inventory.TotalIndexes; i++ {
		guard <- struct{}{} // would block if guard channel is already filled
		wg.Add(1)

		go func(i int) {
			defer wg.Done()
			createdF, err := suitcase.WriteSuitcaseFile(po.SuitcaseOpts, po.Inventory, i, state)
			if err != nil {
				log.Warn().Err(err).Msg("Error writing suitcase file")
				return
			}
			// if po.Inventory.Options.Hash
			if po.SuitcaseOpts.HashOuter {
				log.Info().Msg("Generating a hash of the suitcase")
				sf, err := os.Open(createdF)
				if err != nil {
					log.Warn().Err(err).Msg("Error writing hash file")
					return
				}
				defer sf.Close()
				h := sha256.New()
				if _, err := io.Copy(h, sf); err != nil {
					log.Warn().Err(err).Msg("Error copying hash data")
				}
				hashF := fmt.Sprintf("%v.sha256", createdF)
				sumS := fmt.Sprintf("%x", h.Sum(nil))
				log.Info().Msgf("Writing hash to %x", []byte(sumS))
				hf, err := os.Create(hashF)
				if err != nil {
					log.Warn().Err(err).Msg("Error error writing hash file")
				}
				defer hf.Close()
				hf.Write([]byte(sumS))
			}
			sm.Store(i, createdF)
			<-guard // release the guard channel
		}(i)
	}
	for processed < int32(po.Inventory.TotalIndexes) {
		select {
		case st := <-state:
			if st.Completed {
				atomic.AddInt32(&processed, 1)
			}
			log.Debug().
				Int("index", st.Index).
				Uint("current", st.Current).
				Uint("total", st.Total).
				Msg("Progress")
		}
	}
	wg.Wait()
	var ret []string
	sm.Range(func(key, value any) bool {
		ret = append(ret, value.(string))
		return true
	})

	return ret, nil
}

/*
func NewDirectoryInventoryOptionsWithCmd(cmd *cobra.Command, args []string) (*inventory.DirectoryInventoryOptions, error) {
	var err error

	opt := &inventory.DirectoryInventoryOptions{
		TopLevelDirectories: args,
	}
	opt.TopLevelDirectories, err = helpers.ConvertDirsToAboluteDirs(args)
	if err != nil {
		return nil, err
	}

	// User can specify a human readable string here. We will convert it to bytes for them
	mssF, err := cmd.Flags().GetString("max-suitcase-size")
	if err != nil {
		return nil, err
	}
	mssU, err := humanize.ParseBytes(mssF)
	if err != nil {
		return nil, err
	}
	opt.MaxSuitcaseSize = int64(mssU)

	// Get the internal and external metadata glob patterns
	opt.InternalMetadataGlob, err = cmd.Flags().GetString("internal-metadata-glob")
	if err != nil {
		return nil, err
	}

	// External metadata file here
	opt.ExternalMetadataFiles, err = cmd.Flags().GetStringArray("external-metadata-file")
	if err != nil {
		return nil, err
	}

	// We may want to limit the number of files in the total
	// inventory, mainly to help with debugging, but store that here
	opt.LimitFileCount, err = cmd.Flags().GetInt("limit-file-count")
	if err != nil {
		return nil, err
	}

	// Format for the archive/suitcase
	opt.SuitcaseFormat, err = cmd.Flags().GetString("suitcase-format")
	if err != nil {
		return nil, err
	}
	opt.SuitcaseFormat = strings.TrimPrefix(opt.SuitcaseFormat, ".")

	// Inventory file format (yaml or json)
	opt.InventoryFormat, err = cmd.Flags().GetString("inventory-format")
	if err != nil {
		return nil, err
	}
	opt.InventoryFormat = strings.TrimPrefix(opt.InventoryFormat, ".")

	// We want a username so we can shove it in the suitcase name
	opt.User, err = cmd.Flags().GetString("user")
	if err != nil {
		return nil, err
	}

	if opt.User == "" {
		log.Info().Msg("No user specified, using current user")
		currentUser, err := user.Current()
		if err != nil {
			log.Fatal().Err(err).Msg("Failed to get current user")
		}
		opt.User = currentUser.Username
	}

	opt.Prefix, err = cmd.Flags().GetString("prefix")
	if err != nil {
		return nil, err
	}

	// Set the stuff to be encrypted?
	opt.EncryptInner, err = cmd.Flags().GetBool("encrypt-inner")
	if err != nil {
		return nil, err
	}

	// Do we want to skip hashes?
	opt.HashInner, err = cmd.Flags().GetBool("hash-inner")
	if err != nil {
		return nil, err
	}
	if opt.HashInner {
		log.Warn().
			Msg("Generating file hashes. This will will likely increase the inventory generation time.")
	} else {
		log.Warn().
			Msg("Skipping file hashes. This will increase the speed of the inventory, but will not be able to verify the integrity of the files.")
	}

	// optbufferSize, err = cmd.Flags().GetInt("buffer-size")
	// checkErr(err, "Could not get buffer size")

	// inventoryFormat, err := cmd.Flags().GetString("inventory-format")
	// checkErr(err, "Could not get inventory format")
	return opt, nil
}
*/

func NewDirectoryInventoryOptionsWithViper(v *viper.Viper, args []string) (*inventory.DirectoryInventoryOptions, error) {
	var err error

	opt := &inventory.DirectoryInventoryOptions{
		TopLevelDirectories: args,
	}
	opt.TopLevelDirectories, err = helpers.ConvertDirsToAboluteDirs(args)
	if err != nil {
		return nil, err
	}

	// User can specify a human readable string here. We will convert it to bytes for them
	mssF := v.GetString("max-suitcase-size")
	if mssF == "" {
		mssF = "0"
	}
	mssU, err := humanize.ParseBytes(mssF)
	if err != nil {
		return nil, err
	}
	opt.MaxSuitcaseSize = int64(mssU)

	// Get the internal and external metadata glob patterns
	opt.InternalMetadataGlob = v.GetString("internal-metadata-glob")

	// External metadata file here
	opt.ExternalMetadataFiles = v.GetStringSlice("external-metadata-file")

	// Globs to ignore
	opt.IgnoreGlobs = v.GetStringSlice("ignore-glob")

	// We may want to limit the number of files in the total
	// inventory, mainly to help with debugging, but store that here
	opt.LimitFileCount = v.GetInt("limit-file-count")

	// Format for the archive/suitcase
	opt.SuitcaseFormat = v.GetString("suitcase-format")
	opt.SuitcaseFormat = strings.TrimPrefix(opt.SuitcaseFormat, ".")

	// Inventory file format (yaml or json)
	opt.InventoryFormat = v.GetString("inventory-format")
	opt.InventoryFormat = strings.TrimPrefix(opt.InventoryFormat, ".")

	// We want a username so we can shove it in the suitcase name
	opt.User = v.GetString("user")
	if err != nil {
		return nil, err
	}

	if opt.User == "" {
		log.Info().Msg("No user specified, using current user")
		currentUser, err := user.Current()
		if err != nil {
			return nil, err
		}
		opt.User = currentUser.Username
	}

	opt.Prefix = v.GetString("prefix")

	// Set the stuff to be encrypted?
	opt.EncryptInner = v.GetBool("encrypt-inner")

	// Symlinks?
	opt.FollowSymlinks = v.GetBool("follow-symlinks")

	// Do we want to skip hashes?
	opt.HashInner = v.GetBool("hash-inner")
	if opt.HashInner {
		log.Warn().
			Msg("Generating file hashes. This will will likely increase the inventory generation time.")
	} else {
		log.Warn().
			Msg("Skipping file hashes. This will increase the speed of the inventory, but will not be able to verify the integrity of the files.")
	}

	return opt, nil
}
