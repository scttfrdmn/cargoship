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
	"strings"
	"sync/atomic"
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
	o, err := getDestinationWithCobra(cmd)
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
func getDestinationWithCobra(cmd *cobra.Command) (string, error) {
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
					xferred, err := po.Porter.TravelAgent.Upload(createdF, statusC)
					panicIfErr(err)
					atomic.AddInt64(&po.Porter.TotalTransferred, xferred)
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

func porterWithCmd(cmd *cobra.Command) (*porter.Porter, error) {
	p, ok := cmd.Context().Value(porter.PorterKey).(*porter.Porter)
	if !ok {
		return nil, errors.New("could not find Porter in this context")
	}

	return p, nil
}

/*
func porterTravelAgentWithForm() (*porter.Porter, error) {
	p := porter.New()
	wf := inventory.WizardForm{
		Source: os.Getenv("SUITCASECTL_SOURCE"),
	}
	var dest string

	// var src string
	var tatoken string

	maxsize := "200Gb"

	form := huh.NewForm(
		huh.NewGroup(
			huh.NewInput().
				Title("Source of the data you want to package up").
				Placeholder("/some/local/dir").
				Description("Files within the given directory will be packaged up in to suitcases and transferred to their final destination").
				Validate(validateIsDir).
				Value(&wf.Source),
			huh.NewInput().
				Title("Maximum Suitcase Size").
				Description("This is the maximum size for each suitcase created").
				Value(&maxsize),
			huh.NewInput().
				Title("Destination for files").
				Placeholder("/srv/cold-storage").
				Description("When using a travel agent, this will be used for temporary storage. To use your current systems tmp space, leave this field blank").
				Value(&dest),
			huh.NewInput().
				Title("Travel Agent Token").
				Description("Using a Travel Agent? Enter it here. If not, you can just leave this blank").
				Value(&tatoken),
		),
	)

	if err := form.Run(); err != nil {
		return nil, err
	}

	p.Destination = dest
	p.WizardForm = &wf

	var terr error
	var ta *travelagent.TravelAgent
	if ta, terr = travelagent.New(travelagent.WithToken(tatoken)); terr != nil {
		log.Debug().Err(terr).Msg("🧳 no valid travel agent found")
	} else {
		p.SetTravelAgent(ta)
		log.Info().Str("url", p.TravelAgent.StatusURL()).Msg("☀️ Thanks for using a TravelAgent! Check out this URL for full info on your suitcases fun travel")
		if serr := p.SendUpdate(travelagent.StatusUpdate{
			Status: travelagent.StatusPending,
		}); serr != nil {
			return nil, serr
		}
	}
	// Get option bits
	// Create an inventory file if one isn't specified
	// p.Inventory.Options.Directories = []string{src}
	var err error
	if p.Inventory, err = p.CreateOrReadInventory(""); err != nil {
		return nil, err
	}
	// Replace the travel agent with one that knows the inventory hash
	// This doesn't work yet, need to find out why
	if ta != nil {
		ta.UniquePrefix = p.InventoryHash
		p.SetTravelAgent(ta)
	}

	// We need options even if we already have the inventory
	p.SuitcaseOpts = &config.SuitCaseOpts{
		Destination:  p.Destination,
		EncryptInner: p.Inventory.Options.EncryptInner,
		HashInner:    p.Inventory.Options.HashInner,
		Format:       p.Inventory.Options.SuitcaseFormat,
	}

	p.CLIMeta = porter.NewCLIMeta()
	p.CLIMeta.Wizard = &wf

	return p, nil
}

*/

func porterTravelAgentWithCmd(cmd *cobra.Command, args []string) (*porter.Porter, bool, error) {
	p, err := porterWithCmd(cmd)
	if err != nil {
		return nil, false, err
	}
	taOpts := []travelagent.Option{
		travelagent.WithCmd(cmd),
		travelagent.WithUploadRetries(mustGetCmd[int](cmd, "retry-count")),
		travelagent.WithUploadRetryTime(mustGetCmd[time.Duration](cmd, "retry-interval")),
	}
	if Verbose {
		taOpts = append(taOpts, travelagent.WithPrintCurl())
	}
	var terr error
	var ta *travelagent.TravelAgent
	if ta, terr = travelagent.New(taOpts...); terr != nil {
		log.Debug().Err(terr).Msg("🧳 no valid travel agent found")
	} else {
		p.SetTravelAgent(ta)
		log.Info().Str("url", p.TravelAgent.StatusURL()).Msg("☀️ Thanks for using a TravelAgent! Check out this URL for full info on your suitcases fun travel")
		if serr := p.SendUpdate(travelagent.StatusUpdate{
			Status: travelagent.StatusPending,
		}); serr != nil {
			return nil, false, serr
		}
	}
	// Get option bits
	inventoryFile, onlyInventory, err := inventoryOptsWithCobra(cmd, args)
	if err != nil {
		return nil, false, err
	}

	// Create an inventory file if one isn't specified
	if p.Inventory, err = p.CreateOrReadInventory(inventoryFile); err != nil {
		return nil, onlyInventory, err
	}
	// Replace the travel agent with one that knows the inventory hash
	// This doesn't work yet, need to find out why
	if ta != nil {
		ta.UniquePrefix = p.InventoryHash
		p.SetTravelAgent(ta)
	}

	// We need options even if we already have the inventory
	p.SuitcaseOpts = &config.SuitCaseOpts{
		Destination:  p.Destination,
		EncryptInner: p.Inventory.Options.EncryptInner,
		HashInner:    p.Inventory.Options.HashInner,
		Format:       p.Inventory.Options.SuitcaseFormat,
	}

	return p, onlyInventory, nil
}

// validateCmdArgs ensures we are passing valid arguments in
func validateCmdArgs(inventoryFile string, onlyInventory bool, cmd cobra.Command, args []string) error {
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

	if hasDuplicates(args) {
		return errors.New("duplicate path found in arguments")
	}

	if strings.Contains(mustGetCmd[string](&cmd, "prefix"), "/") {
		return errors.New("prefix cannot contain a /")
	}

	return nil
}
