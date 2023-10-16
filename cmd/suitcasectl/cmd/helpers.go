package cmd

import (

	// nolint:gosec
	// nolint:gosec
	"crypto/sha256"
	"errors"
	"fmt"
	"io"
	"os"
	"path"
	"reflect"
	"time"

	// "github.com/minio/sha256-simd"

	"github.com/rs/zerolog"
	"github.com/rs/zerolog/log"
	"github.com/sourcegraph/conc/pool"
	"github.com/spf13/cobra"
	porter "gitlab.oit.duke.edu/devil-ops/suitcasectl/pkg"
	"gitlab.oit.duke.edu/devil-ops/suitcasectl/pkg/config"
	"gitlab.oit.duke.edu/devil-ops/suitcasectl/pkg/rclone"
	"gitlab.oit.duke.edu/devil-ops/suitcasectl/pkg/suitcase"
	"gitlab.oit.duke.edu/devil-ops/suitcasectl/pkg/travelagent"
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

	// Also shove this in to porter. We'll use it later there.
	if ptr, err := porterWithCmd(cmd); err == nil {
		ptr.Destination = o
	}
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
	ptr, err := porterWithCmd(cmd)
	if err == nil {
		ptr.Destination = d
	}
	logPath := path.Join(d, "suitcasectl.log")
	lf, err := os.Create(logPath) // nolint:gosec
	if err != nil {
		return "", err
	}

	// If we have a porter, set the logfile here
	if ptr, err := porterWithCmd(cmd); err == nil {
		ptr.LogFile = lf
	}

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
	case *time.Duration:
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
	Concurrency   int
	SampleEvery   int
	Porter        *porter.Porter
	SuitcaseOpts  *config.SuitCaseOpts
	RetryCount    int
	RetryInterval time.Duration
}

func startFillStateC(state chan suitcase.FillState, se uint32) {
	sampled := log.Sample(&zerolog.BasicSampler{N: se})
	for {
		st := <-state
		sampled.Debug().
			Int("index", st.Index).
			Uint("current", st.Current).
			Uint("total", st.Total).
			Msg("Progress")
	}
}

// processSuitcases processes the suitcases
func processSuitcases(po *processOpts) []string {
	ret := make([]string, po.Porter.Inventory.TotalIndexes)
	p := pool.New().WithMaxGoroutines(po.Concurrency)
	log.Debug().Int("concurrency", po.Concurrency).Msg("Setting pool guard")
	state := make(chan suitcase.FillState)

	// Read in and log state here
	go func() { startFillStateC(state, uint32(po.SampleEvery)) }()

	statusC := make(chan rclone.TransferStatus)
	go func() {
		for {
			status := <-statusC
			log.Debug().Interface("status", status).Msgf("status update")
			if po.Porter.TravelAgent != nil {
				if err := po.Porter.SendUpdate(*travelagent.NewStatusUpdate(status)); err != nil {
					log.Warn().Err(err).Msg("could not update travel agent")
				}
			}
		}
	}()
	for i := 1; i <= po.Porter.Inventory.TotalIndexes; i++ {
		i := i
		p.Go(func() {
			createdF, err := retryWriteSuitcase(po, i, state)
			if err != nil {
				log.Warn().Err(err).Str("file", createdF).Msg("error creating suitcase file, please investigate")
			} else {
				ret[i-1] = createdF
				// Put Transport plugin here!!
				if po.Porter.Inventory.Options.TransportPlugin != nil {
					// First check...
					panicIfErr(po.Porter.RetryTransport(createdF, statusC, po.RetryCount, po.RetryInterval))
				}

				// Insert TravelAgent upload right here yo'
				if po.Porter.TravelAgent != nil {
					panicIfErr(po.Porter.TravelAgent.Upload(createdF, statusC))
				}
			}
		})
	}
	p.Wait()
	return ret
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

func mustPorterWithCmd(cmd *cobra.Command) *porter.Porter {
	p, err := porterWithCmd(cmd)
	if err != nil {
		panic(err)
	}
	return p
}

func retryWriteSuitcase(po *processOpts, i int, state chan suitcase.FillState) (string, error) {
	var err error
	var createdF string
	var created bool
	attempt := 1
	log := log.With().Int("index", i).Logger()
	for (!created && attempt == 1) || (attempt <= po.RetryCount) {
		log.Debug().Msg("about to write out suitcase file")
		createdF, err = suitcase.WriteSuitcaseFile(po.SuitcaseOpts, po.Porter.Inventory, i, state)
		if err != nil {
			log.Warn().Str("retry-interval", po.RetryInterval.String()).Msg("suitcase creation failed, sleeping, then will retry")
			time.Sleep(po.RetryInterval)
		} else {
			created = true
		}
		attempt++
	}
	if !created {
		return "", errors.New("could not create suitcasefile even with retries")
	}
	return createdF, nil
}
