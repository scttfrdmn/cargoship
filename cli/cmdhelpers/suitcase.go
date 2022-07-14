package cmdhelpers

import (
	"crypto/sha256"
	"fmt"
	"io"
	"os"
	"sync"
	"sync/atomic"

	"github.com/rs/zerolog/log"
	"gitlab.oit.duke.edu/devil-ops/data-suitcase/pkg/config"
	"gitlab.oit.duke.edu/devil-ops/data-suitcase/pkg/inventory"
	"gitlab.oit.duke.edu/devil-ops/data-suitcase/pkg/suitcase"
)

type ProcessOpts struct {
	Concurrency  int
	Inventory    *inventory.DirectoryInventory
	SuitcaseOpts *config.SuitCaseOpts
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


