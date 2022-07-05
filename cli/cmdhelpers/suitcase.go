package cmdhelpers

import (
	"sync"
	"sync/atomic"

	"github.com/rs/zerolog/log"
	"gitlab.oit.duke.edu/oit-ssi-systems/data-suitcase/pkg/config"
	"gitlab.oit.duke.edu/oit-ssi-systems/data-suitcase/pkg/inventory"
	"gitlab.oit.duke.edu/oit-ssi-systems/data-suitcase/pkg/suitcase"
)

type ProcessOpts struct {
	Concurrency  int
	Inventory    *inventory.DirectoryInventory
	SuitcaseOpts *config.SuitCaseOpts
}

/*
func ProcessMultiProgressBar(po *ProcessOpts) error {
	guard := make(chan struct{}, po.Concurrency)
	var wg sync.WaitGroup
	p := mpb.New(mpb.WithWaitGroup(&wg))
	state := make(chan suitcase.SuitcaseFillState, 1)
	processed := int32(0)
	bars := map[int]*mpb.Bar{}
	for i := 1; i <= po.Inventory.TotalIndexes; i++ {
		guard <- struct{}{} // would block if guard channel is already filled
		wg.Add(1)

		// Set up this pretty pretty bar
		bar := p.AddBar(100,
			mpb.PrependDecorators(
				// simple name decorator
				decor.Name(fmt.Sprintf("🧳 %v", i)),
				// decor.DSyncWidth bit enables column width synchronization
				decor.Percentage(decor.WCSyncSpace),
			),
			mpb.AppendDecorators(
			// replace ETA decorator with "done" message, OnComplete event
			//decor.OnComplete(
			// ETA decorator with ewma age of 60
			// decor.EwmaETA(decor.ET_STYLE_GO, 60, decor.WCSyncWidth), "done",
			//),
			),
		)
		bars[i] = bar

		go func(i int) {
			defer wg.Done()
			_, err := suitcase.WriteSuitcaseFile(po.SuitcaseOpts, po.Inventory, i, state)
			if err != nil {
				log.Warn().Err(err).Msg("Error writing suitcase file")
				return
			}
			<-guard // release the guard channel
		}(i)
	}
	for processed < int32(po.Inventory.TotalIndexes) {
		select {
		case st := <-state:
			if st.Completed {
				bars[st.Index].Completed()
				atomic.AddInt32(&processed, 1)
				// log.Info().Msgf("Processed %v of %v", processed, inventory.TotalIndexes)
				break
			}
			bars[st.Index].SetCurrent(int64(st.CurrentPercent))
			// log.Info().Msgf("Suitcase %+v", st)
		}
	}
	wg.Wait()

	return nil
}
*/

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
