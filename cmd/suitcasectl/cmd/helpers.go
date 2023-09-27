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
	"time"

	// "github.com/minio/sha256-simd"

	"github.com/rs/zerolog"
	"github.com/rs/zerolog/log"
	"github.com/sourcegraph/conc/pool"
	"github.com/spf13/cobra"
	porter "gitlab.oit.duke.edu/devil-ops/suitcasectl/pkg"
	"gitlab.oit.duke.edu/devil-ops/suitcasectl/pkg/config"
	"gitlab.oit.duke.edu/devil-ops/suitcasectl/pkg/inventory"
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
	Concurrency int
	SampleEvery int
	Porter      *porter.Porter
	// Inventory    *inventory.Inventory
	SuitcaseOpts *config.SuitCaseOpts
}

// processSuitcases processes the suitcases
func processSuitcases(po *processOpts) []string {
	ret := make([]string, po.Porter.Inventory.TotalIndexes)
	p := pool.New().WithMaxGoroutines(po.Concurrency)
	log.Debug().Int("concurrency", po.Concurrency).Msg("Setting pool guard")
	// state := make(chan suitcase.FillState, 1)
	// state := make(chan suitcase.FillState, po.Inventory.TotalIndexes)
	state := make(chan suitcase.FillState)
	// state := make(chan suitcase.FillState, 2*po.Inventory.TotalIndexes)
	sampled := log.Sample(&zerolog.BasicSampler{N: uint32(po.SampleEvery)})

	// Read in and log state here
	go func() {
		for {
			st := <-state
			sampled.Debug().
				Int("index", st.Index).
				Uint("current", st.Current).
				Uint("total", st.Total).
				Msg("Progress")
		}
	}()

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
			createdF, err := suitcase.WriteSuitcaseFile(po.SuitcaseOpts, po.Porter.Inventory, i, state)
			if err != nil {
				log.Warn().Err(err).Str("file", createdF).Msg("error creating suitcase file, please investigate")
			} else {
				ret[i-1] = createdF
				// Put Transport plugin here!!
				if po.Porter.Inventory.Options.TransportPlugin != nil {
					// First check...
					err := po.Porter.Inventory.Options.TransportPlugin.Check()
					panicIfErr(err)

					// Then end
					serr := po.Porter.Inventory.Options.TransportPlugin.SendWithChannel(createdF, po.Porter.InventoryHash, statusC)
					panicIfErr(serr)
				}
			}
		})
	}
	p.Wait()
	return ret
}

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
