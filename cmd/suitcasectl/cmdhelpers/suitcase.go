package cmdhelpers

import (
	"crypto/sha256"
	"errors"
	"fmt"
	"io"
	"os"
	"sync"
	"sync/atomic"

	"github.com/rs/zerolog/log"
	"github.com/spf13/cobra"
	"gitlab.oit.duke.edu/devil-ops/suitcasectl/pkg/config"
	"gitlab.oit.duke.edu/devil-ops/suitcasectl/pkg/inventory"
	"gitlab.oit.duke.edu/devil-ops/suitcasectl/pkg/suitcase"
)

// ProcessOpts defines the process options
type ProcessOpts struct {
	Concurrency  int
	Inventory    *inventory.DirectoryInventory
	SuitcaseOpts *config.SuitCaseOpts
}

// NewOutDirWithCmd generates a new output directory using cobra.Command options
func NewOutDirWithCmd(cmd *cobra.Command) (string, error) {
	o, err := cmd.Flags().GetString("output-dir")
	if err != nil {
		return "", err
	}
	if o == "" {
		o, err = os.MkdirTemp("", "suitcasectl-")
		if err != nil {
			return "", err
		}
	}
	return o, nil
}

// ProcessLogging processes logging for a run
func ProcessLogging(po *ProcessOpts) ([]string, error) {
	ret := make([]string, po.Inventory.TotalIndexes)
	guard := make(chan struct{}, po.Concurrency)
	var wg sync.WaitGroup
	wg.Add(po.Inventory.TotalIndexes)
	state := make(chan suitcase.FillState, 1)
	processed := int32(0)
	for i := 1; i <= po.Inventory.TotalIndexes; i++ {
		guard <- struct{}{} // would block if guard channel is already filled

		go func(i int) {
			defer wg.Done()
			createdF, err := suitcase.WriteSuitcaseFile(po.SuitcaseOpts, po.Inventory, i, state)
			panicOnError(err)
			// if po.Inventory.Options.Hash
			if po.SuitcaseOpts.HashOuter {
				log.Info().Msg("Generating a hash of the suitcase")
				sf, err := os.Open(createdF) // nolint:gosec
				if err != nil {
					log.Warn().Err(err).Msg("Error writing hash file")
					return
				}
				defer dclose(sf)
				h := sha256.New()
				if _, cperr := io.Copy(h, sf); cperr != nil {
					log.Warn().Err(cperr).Msg("Error copying hash data")
				}
				hashF := fmt.Sprintf("%v.sha256", createdF)
				sumS := fmt.Sprintf("%x", h.Sum(nil))
				log.Info().Msgf("Writing hash to %x", []byte(sumS))
				hf, err := os.Create(hashF) // nolint:gosec
				panicOnError(err)
				defer dclose(hf)
				_, werr := hf.Write([]byte(sumS))
				warnOnError(werr, "error writing file")
			}
			ret[i-1] = createdF
			<-guard // release the guard channel
		}(i)
	}
	for processed < int32(po.Inventory.TotalIndexes) {
		st := <-state
		if st.Completed {
			atomic.AddInt32(&processed, 1)
		}
		log.Debug().
			Int("index", st.Index).
			Uint("current", st.Current).
			Uint("total", st.Total).
			Msg("Progress")
	}
	wg.Wait()
	return ret, nil
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

func dclose(c io.Closer) {
	err := c.Close()
	if err != nil {
		log.Warn().Interface("closer", c).Msg("error closing file")
	}
}

func warnOnError(err error, msg string) {
	if err != nil {
		if msg != "" {
			log.Warn().Err(err).Msg(msg)
		} else {
			log.Warn().Err(err).Send()
		}
	}
}

func panicOnError(err error) {
	if err != nil {
		panic(err)
	}
}
