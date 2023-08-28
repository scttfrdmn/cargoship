package cmd

import (
	"bufio"
	"context"
	"crypto/md5"  // nolint:gosec
	"crypto/sha1" // nolint:gosec
	"crypto/sha256"
	"crypto/sha512"
	"encoding/hex"
	"fmt"
	"hash"
	"io"
	"os"
	"path"
	"reflect"
	"sync/atomic"
	"time"

	// "github.com/minio/sha256-simd"

	"github.com/rs/zerolog"
	"github.com/rs/zerolog/log"
	"github.com/sourcegraph/conc/pool"
	"github.com/spf13/cobra"
	"gitlab.oit.duke.edu/devil-ops/suitcasectl/pkg/config"
	"gitlab.oit.duke.edu/devil-ops/suitcasectl/pkg/inventory"
	"gitlab.oit.duke.edu/devil-ops/suitcasectl/pkg/suitcase"
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
	ctx := context.WithValue(cmd.Context(), inventory.DestinationKey, o)
	cmd.SetContext(ctx)
	return o, nil
}

func dclose(c io.Closer) {
	err := c.Close()
	if err != nil {
		log.Warn().Err(err).Send()
	}
}

// This needs some work. Why do we need it's own context entry instead of just
// using the SuitcaseOpts, which already has it
func getDestination(cmd *cobra.Command) (string, error) {
	d := mustGetCmd[string](cmd, "destination")
	if d == "" {
		var err error
		if d, err = os.MkdirTemp("", "suitcasectl"); err != nil {
			return "", err
		}
	}

	// Set for later use
	cmd.SetContext(context.WithValue(cmd.Context(), inventory.DestinationKey, d))
	logPath := path.Join(d, "suitcasectl.log")
	lf, err := os.Create(logPath) // nolint:gosec
	if err != nil {
		return "", err
	}
	cmd.SetContext(context.WithValue(cmd.Context(), inventory.LogFileKey, lf))

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
	defer dclose(f)

	b, err := io.ReadAll(f)
	if err != nil {
		return "", err
	}

	sum := sha256.Sum256(b)

	return fmt.Sprintf("%x", sum), nil
}

// ProcessOpts defines the process options
type processOpts struct {
	Concurrency  int
	SampleEvery  int
	Inventory    *inventory.Inventory
	SuitcaseOpts *config.SuitCaseOpts
}

// processSuitcases processes the suitcases
func processSuitcases(po *processOpts) []string {
	ret := make([]string, po.Inventory.TotalIndexes)
	p := pool.New().WithMaxGoroutines(po.Concurrency)
	log.Warn().Int("concurrency", po.Concurrency).Msg("Setting pool guard")
	// state := make(chan suitcase.FillState, 1)
	state := make(chan suitcase.FillState, po.Inventory.TotalIndexes)
	sampled := log.Sample(&zerolog.BasicSampler{N: uint32(po.SampleEvery)})
	for i := 1; i <= po.Inventory.TotalIndexes; i++ {
		i := i
		p.Go(func() {
			// defer wg.Done()
			// fmt.Fprintf(os.Stderr, "HELO: %+v\n", ret)
			createdF, err := suitcase.WriteSuitcaseFile(po.SuitcaseOpts, po.Inventory, i, state)
			if err != nil {
				log.Warn().Err(err).Str("file", createdF).Msg("error creating suitcase file, please investigate")
			} else {
				// Put Transport plugin here!!
				ret[i-1] = createdF
			}
		})
	}
	processed := int32(0)
	for processed < int32(po.Inventory.TotalIndexes) {
		st := <-state
		if st.Completed {
			atomic.AddInt32(&processed, 1)
		}
		sampled.Debug().
			Int("index", st.Index).
			Uint("current", st.Current).
			Uint("total", st.Total).
			Msg("Progress")
	}
	p.Wait()
	return ret
}

/*
// processSuitcases processes the suitcases
func processSuitcasesX(po *processOpts) []string {
	ret := make([]string, po.Inventory.TotalIndexes)
	guard := make(chan struct{}, po.Concurrency)
	var wg sync.WaitGroup
	wg.Add(po.Inventory.TotalIndexes)
	state := make(chan suitcase.FillState, 1)
	sampled := log.Sample(&zerolog.BasicSampler{N: uint32(po.SampleEvery)})
	for i := 1; i <= po.Inventory.TotalIndexes; i++ {
		guard <- struct{}{} // would block if guard channel is already filled

		go func(i int) {
			defer wg.Done()
			createdF, err := suitcase.WriteSuitcaseFile(po.SuitcaseOpts, po.Inventory, i, state)
			if err != nil {
				log.Warn().Err(err).Str("file", createdF).Msg("error creating suitcase file, please investigate")
			} else {
				// Put Transport plugin here!!
				ret[i-1] = createdF
			}
			<-guard // release the guard channel
		}(i)
	}
	// Use this to prevent deadlocks
	go func() {
		wg.Wait()
	}()
	processed := int32(0)
	for processed < int32(po.Inventory.TotalIndexes) {
		st := <-state
		if st.Completed {
			atomic.AddInt32(&processed, 1)
		}
		sampled.Debug().
			Int("index", st.Index).
			Uint("current", st.Current).
			Uint("total", st.Total).
			Msg("Progress")
	}
	return ret
}
*/

func calculateHash(rd io.Reader, ht string) string {
	reader := bufio.NewReaderSize(rd, os.Getpagesize())
	var dst hash.Hash
	switch ht {
	case "md5":
		dst = md5.New() // nolint:gosec
	case "sha1":
		dst = sha1.New() // nolint:gosec
	case "sha256":
		dst = sha256.New()
	case "sha512":
		dst = sha512.New()
	default:
		panic(fmt.Sprintf("unexpected hash type: %v", ht))
	}
	_, err := io.Copy(dst, reader)
	panicIfErr(err)
	return hex.EncodeToString(dst.Sum(nil))
}
