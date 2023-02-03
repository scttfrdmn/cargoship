package cmd

import (
	"context"
	"crypto/sha256"
	"fmt"
	"io"
	"os"
	"path"
	"reflect"
	"sync"
	"sync/atomic"
	"time"

	// "github.com/minio/sha256-simd"

	"github.com/rs/zerolog/log"
	"github.com/spf13/cobra"
	"gitlab.oit.duke.edu/devil-ops/suitcasectl/pkg/config"
	"gitlab.oit.duke.edu/devil-ops/suitcasectl/pkg/inventory"
	"gitlab.oit.duke.edu/devil-ops/suitcasectl/pkg/suitcase"
)

type contextKey int

const (
	destinationKey contextKey = iota
	logFileKey
	hashesKey
	userOverrideKey
)

// newOutDirWithCmd generates a new output directory using cobra.Command options
func newOutDirWithCmd(cmd *cobra.Command) (string, error) {
	o, err := getDestination(cmd)
	if err != nil {
		return "", err
	}
	if o == "" {
		o, err = os.MkdirTemp("", "suitcasectl-")
		if err != nil {
			return "", err
		}
	}

	// Also shove this in to the context. We'll use it later there.
	ctx := context.WithValue(cmd.Context(), destinationKey, o)
	cmd.SetContext(ctx)
	return o, nil
}

func dclose(c io.Closer) {
	err := c.Close()
	if err != nil {
		log.Warn().Err(err).Send()
	}
}

func getDestination(cmd *cobra.Command) (string, error) {
	d := mustGetCmd[string](cmd, "destination")
	if d == "" {
		var err error
		d, err = os.MkdirTemp("", "suitcasectl")
		if err != nil {
			return "", err
		}
	}

	// Set for later use
	cmd.SetContext(context.WithValue(cmd.Context(), destinationKey, d))
	logPath := path.Join(d, "suitcasectl.log")
	lf, err := os.Create(logPath) // nolint:gosec
	if err != nil {
		return "", err
	}
	cmd.SetContext(context.WithValue(cmd.Context(), logFileKey, lf))

	return d, nil
}

// mustGetCmd uses generics to get a given flag with the appropriate Type from a cobra.Command
func mustGetCmd[T []int | int | string | bool | time.Duration](cmd *cobra.Command, s string) T {
	switch any(new(T)).(type) {
	case *int:
		item, err := cmd.Flags().GetInt(s)
		panicIfErr(err)
		return any(item).(T)
	case *string:
		item, err := cmd.Flags().GetString(s)
		panicIfErr(err)
		return any(item).(T)
	case *bool:
		item, err := cmd.Flags().GetBool(s)
		panicIfErr(err)
		return any(item).(T)
	case *[]int:
		item, err := cmd.Flags().GetIntSlice(s)
		panicIfErr(err)
		return any(item).(T)
	case *time.Time:
		item, err := cmd.Flags().GetDuration(s)
		panicIfErr(err)
		return any(item).(T)
	default:
		panic(fmt.Sprintf("unexpected use of mustGetCmd: %v", reflect.TypeOf(s)))
	}
}

func panicIfErr(err error) {
	if err != nil {
		panic(err)
	}
}

// getSha256 Get sha256 hash from a file
func getSha256(file string) (string, error) {
	f, err := os.Open(file) // nolint:gosec
	if err != nil {
		return "", err
	}
	defer func() {
		cerr := f.Close()
		if err != nil {
			panic(cerr)
		}
	}()

	/*
		h := sha256.New()
		if _, err := io.Copy(h, f); err != nil {
			return "", err
		}
	*/

	b, err := io.ReadAll(f)
	if err != nil {
		return "", err
	}

	sum := sha256.Sum256(b)

	return fmt.Sprintf("%x", sum), nil
}

// mustGetSha256 panics if a Sha256 cannot be generated
func mustGetSha256(file string) string {
	hash, err := getSha256(file)
	if err != nil {
		panic(err)
	}
	return hash
}

// ProcessOpts defines the process options
type processOpts struct {
	Concurrency  int
	Inventory    *inventory.DirectoryInventory
	SuitcaseOpts *config.SuitCaseOpts
}

// processLogging processes logging for a run
func processLogging(po *processOpts) []string {
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
	return ret
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
