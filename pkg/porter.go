/*
Package porter is the superset of things that can operate the suitcases and
such

This package came about because I found myself having these giant functions
that had to pass in tons of individual items to get everything it needed. This
object is an attempt to simplify all those in to one cohesive bit
*/
package porter

import (
	"bufio"
	"crypto/md5"  // nolint:gosec
	"crypto/sha1" // nolint:gosec
	"crypto/sha256"
	"crypto/sha512"
	"encoding/hex"
	"errors"
	"fmt"
	"hash"
	"io"
	"log/slog"
	"os"
	"path"
	"strings"
	"sync/atomic"
	"time"

	"github.com/charmbracelet/huh"
	"github.com/charmbracelet/log"
	"github.com/dustin/go-humanize"
	"github.com/sourcegraph/conc/pool"

	"github.com/spf13/cobra"
	"github.com/spf13/viper"
	"gitlab.oit.duke.edu/devil-ops/suitcasectl/pkg/config"
	"gitlab.oit.duke.edu/devil-ops/suitcasectl/pkg/inventory"
	"gitlab.oit.duke.edu/devil-ops/suitcasectl/pkg/rclone"
	"gitlab.oit.duke.edu/devil-ops/suitcasectl/pkg/suitcase"
	"gitlab.oit.duke.edu/devil-ops/suitcasectl/pkg/travelagent"
)

// Porter holds all the pieces of the suitcases together and such. Trying to
// flatten this nest of modules together, this is the first step in getting
// something that can perform that way
type Porter struct {
	Cmd              *cobra.Command
	Args             []string
	CLIMeta          *CLIMeta
	TravelAgent      travelagent.TravelAgenter
	hasTravelAgent   bool
	Inventory        *inventory.Inventory
	InventoryHash    string
	Logger           *slog.Logger
	HashAlgorithm    inventory.HashAlgorithm
	Hashes           []config.HashSet
	UserOverrides    *viper.Viper
	Destination      string
	Version          string
	SuitcaseOpts     *config.SuitCaseOpts
	LogFile          *os.File
	TotalTransferred int64
	WizardForm       *inventory.WizardForm
	sampleEvery      int
	retryCount       int
	retryInterval    time.Duration
	concurrency      int
	stateC           chan FillState
	statusC          chan rclone.TransferStatus
}

// New returns a new porter using functional options
func New(options ...Option) *Porter {
	p := &Porter{
		Logger: slog.New(slog.NewTextHandler(os.Stderr, nil)),
		SuitcaseOpts: &config.SuitCaseOpts{
			Format: "tar.zst",
		},
		sampleEvery:   100,
		retryCount:    1,
		retryInterval: time.Second * 5,
		concurrency:   10,
		stateC:        make(chan FillState),
		statusC:       make(chan rclone.TransferStatus),
	}
	for _, opt := range options {
		opt(p)
	}
	// There is probably a better way to do this...why do we want both??
	/*
		if p.Destination == "" && p.SuitcaseOpts.Destination != "" {
			p.Destination = p.SuitcaseOpts.Destination
		}
		if p.SuitcaseOpts.Destination == "" && p.Destination != "" {
			p.SuitcaseOpts.Destination = p.Destination
		}
	*/
	return p
}

// Option is a functional option that can be passed to create a new Porter instance
type Option func(*Porter)

// WithUserOverrides sets UserOverrides at create time
func WithUserOverrides(o *viper.Viper) func(*Porter) {
	return func(p *Porter) {
		p.UserOverrides = o
	}
}

// WithInventory sets the inventory at create time
func WithInventory(i *inventory.Inventory) func(*Porter) {
	return func(p *Porter) {
		p.Inventory = i
	}
}

// SetTravelAgent sets the travel agent property
func (p *Porter) SetTravelAgent(t travelagent.TravelAgenter) {
	p.TravelAgent = t
	p.hasTravelAgent = true
}

// WithTravelAgent sets the travel agent at create time
func WithTravelAgent(t travelagent.TravelAgenter) func(*Porter) {
	return func(p *Porter) {
		p.hasTravelAgent = true
		p.TravelAgent = t
	}
}

// WithCLIMeta sets CLIMeta at create time
func WithCLIMeta(c *CLIMeta) func(*Porter) {
	return func(p *Porter) {
		p.CLIMeta = c
	}
}

// WithVersion sets the version
func WithVersion(s string) func(*Porter) {
	return func(p *Porter) {
		p.Version = s
	}
}

// WithCmdArgs sets cobra command and args
func WithCmdArgs(cmd *cobra.Command, args []string) func(*Porter) {
	return func(p *Porter) {
		p.Cmd = cmd
		p.Args = args
	}
}

// WithLogger sets the logger at create
func WithLogger(l *slog.Logger) func(*Porter) {
	return func(p *Porter) {
		p.Logger = l
	}
}

// WithDestination sets the destination at create time
func WithDestination(s string) func(*Porter) {
	return func(p *Porter) {
		p.Destination = s
		// Eventually I'd like to get rid of this double Destination declaration...it only buys confusion
		/*
			if p.SuitcaseOpts.Destination == "" {
				p.SuitcaseOpts.Destination = s
			}
		*/
	}
}

// WithHashAlgorithm sets the hash algorithm at create time
func WithHashAlgorithm(h inventory.HashAlgorithm) func(*Porter) {
	return func(p *Porter) {
		p.HashAlgorithm = h
	}
}

// SetConcurrency sets the concurrency for a given porter instance
func (p *Porter) SetConcurrency(c int) {
	p.concurrency = c
}

// SetRetries sets the retry count and interval for retrying various things
func (p *Porter) SetRetries(c int, i time.Duration) {
	p.retryCount = c
	p.retryInterval = i
}

// CreateHashes returns a HashSet from a set of strings
func (p Porter) CreateHashes(s []string) ([]config.HashSet, error) {
	var hs []config.HashSet
	if p.Destination == "" {
		return nil, errors.New("must set Destination in porter before using CreateHashes")
	}
	if p.HashAlgorithm == inventory.NullHash {
		return nil, errors.New("must set HashAlgorithm in porter before using CreateHashes")
	}
	for _, f := range s {
		fh, err := os.Open(f) // nolint:gosec
		if err != nil {
			return nil, err
		}
		defer dclose(fh)
		p.Logger.Info("created file", "file", f)
		hs = append(hs, config.HashSet{
			Filename: strings.TrimPrefix(f, p.Destination+"/"),
			Hash:     MustCalculateHash(fh, p.HashAlgorithm.String()),
		})
	}
	return hs, nil
}

// SendUpdate sends an update to the travel agent if it exists
func (p Porter) SendUpdate(u travelagent.StatusUpdate) error {
	if !p.hasTravelAgent {
		return nil
	}
	log := p.Logger

	resp, err := p.TravelAgent.Update(u)
	if err != nil {
		return err
	}
	if p.Logger != nil {
		if u.Name != "" {
			log = p.Logger.With("component", u.Name)
			if u.SizeBytes > 0 {
				log = log.With(
					"transferred", humanize.Bytes(uint64(u.TransferredBytes)),
					"total", humanize.Bytes(uint64(u.SizeBytes)),
					"avg-speed", fmt.Sprintf("%v/s", humanize.Bytes(uint64(u.Speed))),
				)
				if u.PercentDone > 0 {
					log = log.With("progress", fmt.Sprintf("%v%%", u.PercentDone))
				}
			}
		}

		for _, msg := range resp.Messages {
			if strings.TrimSpace(msg) != "updated fields:" {
				log.Info(msg)
			}
		}
	}

	return nil
}

/*
func prefixLog(s string) string {
	return "☁️ " + s
}
*/

func dclose(c io.Closer) {
	err := c.Close()
	if err != nil {
		fmt.Fprintf(os.Stderr, "error closing %v\n", c)
	}
}

// MustCalculateHash returns a certain type of hash string and panics on error
func MustCalculateHash(rd io.Reader, ht string) string {
	got, err := CalculateHash(rd, ht)
	if err != nil {
		panic(err)
	}
	return got
}

// CalculateHash returns a certain type of hash string and an optional error
func CalculateHash(rd io.Reader, ht string) (string, error) {
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
		return "", fmt.Errorf(fmt.Sprintf("unexpected hash type: %v", ht))
	}
	_, err := io.Copy(dst, reader)
	if err != nil {
		return "", err
	}
	return hex.EncodeToString(dst.Sum(nil)), nil
}

// SetOrReadInventory is similar to createorreadinventory, however it sets the inventory in the porter object instead of returning it
func (p *Porter) SetOrReadInventory(invf string) error {
	got, err := p.CreateOrReadInventory(invf)
	if err != nil {
		return err
	}
	p.Inventory = got
	return nil
}

// CreateOrReadInventory returns an inventory and optionally creates it if it didn't exist
func (p *Porter) CreateOrReadInventory(inventoryFile string) (*inventory.Inventory, error) {
	// Create an inventory file if one isn't specified
	var inventoryD *inventory.Inventory
	if inventoryFile == "" {
		p.Logger.Debug("no inventory file specified, we're going to go ahead and create one")
		var outF *os.File
		var err error
		p.UserOverrides = p.getUserOverrides()
		if p.Destination == "" && p.WizardForm != nil && p.WizardForm.Destination != "" {
			p.Destination = p.WizardForm.Destination
		}
		if p.Destination == "" {
			p.Destination = mustTempDir()
		}
		// inventoryD, outF, err = WriteInventoryAndFileWithViper(v, cmd, args, outDir, version)
		inventoryD, outF, err = p.WriteInventory()
		if err != nil {
			return nil, err
		}
		inventoryFile = outF.Name()
		slog.Info("created inventory file", "file", inventoryFile)
	} else {
		var err error
		inventoryD, err = inventory.NewInventoryWithFilename(inventoryFile)
		if err != nil {
			return nil, err
		}
	}

	// Calculate a hash of the inventory
	h, err := calculateMD5Sum(inventoryFile)
	if err != nil {
		return nil, err
	}
	p.InventoryHash = h
	// Store the inventory in context, so we can access it in the other run stages
	p.Inventory = inventoryD
	// p.Cmd.SetContext(context.WithValue(p.Cmd.Context(), inventory.InventoryKey, inventoryD))
	inventoryD.SummaryLog()
	return inventoryD, nil
}

func mustTempDir() string {
	o, err := os.MkdirTemp("", "suitcasectl")
	if err != nil {
		panic(err)
	}
	return o
}

// calculateMD5Sum calculates the MD5 checksum of a file given its path.
func calculateMD5Sum(filePath string) (string, error) {
	file, err := os.Open(filePath) // nolint:gosec
	if err != nil {
		return "", err
	}
	defer dclose(file)

	hash := md5.New() // nolint:gosec
	if _, err := io.Copy(hash, file); err != nil {
		return "", err
	}

	// Convert the MD5 hash to a hexadecimal string representation
	checksum := hex.EncodeToString(hash.Sum(nil))

	return checksum, nil
}

type contextKey int

const (
	// PorterKey is where we sneak porter in to the cmd contexs.
	PorterKey contextKey = iota
)

// UserOverrideWithCobra returns a user override viper object from a cmd
func (p *Porter) getUserOverrides() *viper.Viper {
	if p.UserOverrides == nil {
		return &viper.Viper{}
	}
	return p.UserOverrides
}

// WriteInventory writes out an inventory file, and returns it, along with the actual Inventory
func (p *Porter) WriteInventory() (*inventory.Inventory, *os.File, error) {
	i, f, ir, err := p.inventoryerGeneration()
	if err != nil {
		return nil, nil, err
	}
	if err := ir.Write(f, i); err != nil {
		return nil, nil, err
	}
	return i, f, nil
}

// inventoryGeneration generates appropriate inventory-er pieces
func (p *Porter) inventoryerGeneration() (*inventory.Inventory, *os.File, inventory.Inventoryer, error) {
	if p.UserOverrides == nil {
		panic("must pass UserOverrides")
	}
	i, f, err := p.inventoryGeneration()
	if err != nil {
		return nil, nil, nil, err
	}
	ir, err := inventory.NewInventoryerWithFilename(f.Name())
	if err != nil {
		return nil, nil, nil, err
	}
	return i, f, ir, nil
}

// inventoryGeneration generates appropriate inventory pieces...
func (p *Porter) inventoryGeneration() (*inventory.Inventory, *os.File, error) {
	iopts := []func(*inventory.Options){}
	if p.Cmd != nil {
		iopts = append(iopts, inventory.WithCobra(p.Cmd, p.Args))
	}
	if p.UserOverrides != nil {
		iopts = append(iopts, inventory.WithViper(p.UserOverrides))
	}
	if p.WizardForm != nil {
		iopts = append(iopts, inventory.WithWizardForm(*p.WizardForm))
	}
	i, err := inventory.NewDirectoryInventory(
		inventory.NewOptions(iopts...),
	)
	if err != nil {
		return nil, nil, err
	}
	if verr := i.ValidateAccess(); verr != nil {
		return nil, nil, verr
	}
	outF, err := os.Create(path.Join(p.Destination, fmt.Sprintf("inventory.%v", i.Options.InventoryFormat))) // nolint:gosec
	if err != nil {
		return nil, nil, err
	}
	return i, outF, nil
}

// RetryTransport does some retries when doing a transport push
func (p *Porter) RetryTransport(f string, statusC chan rclone.TransferStatus, retryCount int, retryInterval time.Duration) error {
	if p.Inventory == nil {
		return errors.New("must have set Inventory")
	}
	if err := p.Inventory.Options.TransportPlugin.Check(); err != nil {
		return err
	}

	// Then end
	var created bool
	attempt := 1
	for (!created && attempt == 1) || attempt <= retryCount {
		if serr := p.Inventory.Options.TransportPlugin.SendWithChannel(f, p.InventoryHash, statusC); serr != nil {
			p.Logger.Warn("suitcase transport failed, sleeping, then will retry", "retry-interval", retryInterval.String())
			time.Sleep(retryInterval)
		} else {
			created = true
		}
		attempt++
	}
	if !created {
		return errors.New("could not transport suitcasefile even with retries")
	}
	return nil
}

// ShipItems sends items through a transporter and optionally reports them to the Travel Agent
func (p *Porter) ShipItems(items []string, uniqDir string) {
	// Running in to a loop issue while this is concurrent
	// var wg conc.WaitGroup
	c := make(chan rclone.TransferStatus)
	go func() {
		for {
			status := <-c
			p.Logger.Debug("status update", "status", status)
			if p.TravelAgent != nil {
				if err := p.SendUpdate(*travelagent.NewStatusUpdate(status)); err != nil {
					p.Logger.Warn("could not update travel agent", "error", err)
				}
			}
		}
	}()

	for _, fn := range items {
		item := path.Join(p.Destination, fn)
		if err := p.Inventory.Options.TransportPlugin.SendWithChannel(item, uniqDir, c); err != nil {
			p.Logger.Warn("error copying file", "file", item)
		}
	}
}

func copySrcDst(src, dst string) error {
	sourceFileStat, err := os.Stat(src)
	if err != nil {
		return err
	}

	if !sourceFileStat.Mode().IsRegular() {
		return fmt.Errorf("%s is not a regular file", src)
	}

	source, err := os.Open(src) // nolint:gosec
	if err != nil {
		return err
	}
	defer dclose(source)

	destination, err := os.Create(dst) // nolint:gosec
	if err != nil {
		return err
	}
	defer dclose(destination)
	_, err = io.Copy(destination, source)
	return err
}

func envOrTmpDir(e string) string {
	got := os.Getenv(e)
	if got == "" {
		tdir, err := os.MkdirTemp("", "suitcasectl")
		if err != nil {
			panic(err)
		}
		return tdir
	}
	return got
}

func envOrString(e, d string) string {
	got := os.Getenv(e)
	if got != "" {
		return got
	}
	return d
}

func createForm(wf *inventory.WizardForm) *huh.Form {
	return huh.NewForm(
		huh.NewGroup(
			huh.NewInput().
				Title("Source of the data you want to package up").
				Placeholder("/some/local/dir").
				Description(`Files within the given directory will be packaged up in to suitcases and transferred to their final destination.
Defaults to SUITCASECTL_SOURCE if set in the env`).
				Validate(validateIsDir).
				Value(&wf.Source),
			huh.NewInput().
				Title("Maximum Suitcase Size").
				Description(`This is the maximum size for each suitcase created
Defaults to SUITCASECTL_MAXSIZE if set in the env`).
				Value(&wf.MaxSize),
			huh.NewInput().
				Title("Destination for files").
				Placeholder("/srv/cold-storage").
				Description("When using a travel agent, this will be used for temporary storage. To use your current systems tmp space, leave this field blank.\nDefaults to SUITCASECTL_DESTINATION if set in the env").
				Value(&wf.Destination),
			huh.NewInput().
				Title("Travel Agent Token").
				Description(`Using a Travel Agent? Enter it here. If not, you can just leave this blank.
Defaults to SUITCASECTL_TRAVELAGENT if set in the env`).
				Value(&wf.TravelAgentToken),
		),
	)
}

func runForm(wf *inventory.WizardForm) error {
	fmt.Println("!! NOTE: This is still in beta, use with caution\nFor a full experience, use `suitcasectl create suitcase`!!")
	if err := createForm(wf).Run(); err != nil {
		return err
	}
	return nil
}

// RunWizard uses an interactive form to select some base pieces and package up date in to suitcases
func (p *Porter) RunWizard() error {
	p.WizardForm = &inventory.WizardForm{
		Source:           os.Getenv("SUITCASECTL_SOURCE"),
		Destination:      envOrTmpDir("SUITCASECTL_DESTINATION"),
		MaxSize:          envOrString("SUITCASECTL_MAXSIZE", "200Gb"),
		TravelAgentToken: os.Getenv("SUITCASECTL_TRAVELAGENT"),
	}
	if err := runForm(p.WizardForm); err != nil {
		return err
	}

	// Expand the homedir
	p.WizardForm.Source = mustExpandDir(p.WizardForm.Source)

	var terr error
	var ta *travelagent.TravelAgent
	if ta, terr = travelagent.New(travelagent.WithCredentialBlob(p.WizardForm.TravelAgentToken)); terr != nil {
		p.Logger.Warn("no valid travel agent found", "error", terr)
	} else {
		p.SetTravelAgent(ta)
		log.Info("Thanks for using a TravelAgent! Check out this URL for full info on your suitcases fun travel", "url", p.TravelAgent.StatusURL())
		if serr := p.SendUpdate(travelagent.StatusUpdate{
			Status: travelagent.StatusPending,
		}); serr != nil {
			return serr
		}
	}
	if err := p.SetOrReadInventory(""); err != nil {
		return err
	}
	// Replace the travel agent with one that knows the inventory hash
	// This doesn't work yet, need to find out why
	if ta != nil {
		ta.UniquePrefix = p.InventoryHash
		p.SetTravelAgent(ta)
	}

	return p.mergeWizard()
}

// mergeWizard puts the fields from the wizard form in to the standard porter spots
func (p *Porter) mergeWizard() error {
	// We need options even if we already have the inventory
	if p.Inventory == nil {
		return errors.New("must have an Inventory set before merge can happen")
	}
	if p.WizardForm == nil {
		return errors.New("must have a WizardForm set before merge can happen")
	}
	p.SuitcaseOpts = &config.SuitCaseOpts{
		// Destination:  p.Destination,
		EncryptInner: p.Inventory.Options.EncryptInner,
		HashInner:    p.Inventory.Options.HashInner,
		Format:       p.Inventory.Options.SuitcaseFormat,
	}

	if p.WizardForm.Destination != "" {
		p.Destination = p.WizardForm.Destination
	} else {
		td, err := os.MkdirTemp("", "suitcasectl-wizard")
		if err != nil {
			return nil
		}
		p.Destination = td
	}
	p.CLIMeta = NewCLIMeta()
	p.CLIMeta.Wizard = p.WizardForm

	logPath := path.Join(p.Destination, "suitcasectl.log")
	lf, err := os.Create(path.Clean(logPath))
	if err != nil {
		return err
	}
	p.LogFile = lf
	return nil
}

func (p *Porter) startFillStateC(state chan FillState) {
	// sampled := log.Sample(&zerolog.BasicSampler{N: se})
	i := uint32(0)
	for {
		st := <-state
		if i%uint32(p.sampleEvery) == 0 {
			slog.Debug("progress", "index", st.Index, "current", st.Current, "total", st.Total)
		}
		i++
	}
}

func (p *Porter) startTransferStatusC(statusC chan rclone.TransferStatus) {
	for {
		status := <-statusC
		slog.Debug("status update", "status", status)
		if p.TravelAgent != nil {
			if err := p.SendUpdate(*travelagent.NewStatusUpdate(status)); err != nil {
				slog.Warn("could not update travel agent", "error", err)
			}
		}
	}
}

func (p *Porter) retryWriteSuitcase(i int, state chan FillState) (string, error) {
	var err error
	var createdF string
	var created bool
	attempt := 1
	log := slog.With("index", i)
	// log := log.With().Int("index", i).Logger()
	for (!created && attempt == 1) || (attempt <= p.retryCount) {
		log.Debug("about to write out suitcase file")
		createdF, err = p.WriteSuitcaseFile(i, state)
		if err != nil {
			log.Warn("suitcase creation failed, sleeping, then will retry", "interval", p.retryInterval.String(), "error", err)
			time.Sleep(p.retryInterval)
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

func (p *Porter) processSuitcases() ([]string, error) {
	pl := newPool(p.concurrency)

	// Launch some reading of these channels
	go p.startFillStateC(p.stateC)
	go p.startTransferStatusC(p.statusC)

	ret := make([]string, p.Inventory.TotalIndexes)
	for i := 1; i <= p.Inventory.TotalIndexes; i++ {
		i := i
		pl.Go(func() error {
			var err error
			if ret[i-1], err = p.retryWriteSuitcase(i, p.stateC); err != nil {
				return err
			}
			if p.Inventory.Options.TransportPlugin != nil {
				// First check...
				if err := p.RetryTransport(ret[i-1], p.statusC, p.retryCount, p.retryInterval); err != nil {
					return err
				}
			}

			// Insert TravelAgent upload right here yo'
			if p.TravelAgent != nil {
				xferred, err := p.TravelAgent.Upload(ret[i-1], p.statusC)
				if err != nil {
					return err
				}
				atomic.AddInt64(&p.TotalTransferred, xferred)
			}
			return nil
		})
	}
	err := pl.Wait()
	return ret, err
}

// Run does the actual suitcase creation
func (p *Porter) Run() error {
	if p.SuitcaseOpts != nil {
		if err := p.SuitcaseOpts.EncryptToCobra(p.Cmd); err != nil {
			return err
		}
	}

	createdFiles, err := p.processSuitcases()
	if err != nil {
		return err
	}

	if p.Cmd != nil {
		if mustGetCmd[bool](p.Cmd, "hash-outer") {
			p.Hashes, err = p.CreateHashes(createdFiles)
			if err != nil {
				return err
			}
		}
	}

	return nil
}

// WriteSuitcaseFile will write out the suitcase
func (p *Porter) WriteSuitcaseFile(index int, stateC chan FillState) (string, error) {
	if p.Inventory == nil {
		return "", errors.New("inventory must not be nil in WriteSuitcaseFile")
	}
	targetFn := path.Join(p.Destination, p.Inventory.SuitcaseNameWithIndex(index))
	log := slog.With("suitcase", targetFn)
	if fileExists(targetFn) {
		return targetFn, nil
	}

	tmpTargetFn := inProcessName(targetFn)
	target, err := os.Create(tmpTargetFn) // nolint:gosec
	if err != nil {
		return "", err
	}
	defer func() {
		if terr := target.Close(); terr != nil {
			panic(terr)
		}
	}()

	s, err := suitcase.New(target, p.SuitcaseOpts)
	if err != nil {
		return "", err
	}
	defer dclose(s)

	log.Debug("Filling suitcase", "destination", targetFn, "format", p.SuitcaseOpts.Format, "encrypt-inner", p.SuitcaseOpts.EncryptInner)
	hashes, err := p.Fill(s, index, stateC)
	if err != nil {
		return "", err
	}

	if stateC != nil {
		// This is hanging... maybe?
		stateC <- newCompleteFillState(index)
	}

	if p.SuitcaseOpts.HashInner {
		if err := hashInner(targetFn, p.Inventory.Options.HashAlgorithm, hashes); err != nil {
			return "", err
		}
	}

	if err := os.Rename(tmpTargetFn, targetFn); err != nil {
		return "", err
	}

	return targetFn, nil
}

// Fill fills up a suitcase using the given inventory
func (p *Porter) Fill(s suitcase.Suitcase, index int, stateC chan FillState) ([]config.HashSet, error) {
	if p.Inventory == nil {
		return nil, errors.New("inventory is nil")
	}
	var err error

	var total uint
	if index > 0 {
		if _, ok := p.Inventory.IndexSummaries[index]; ok {
			total = p.Inventory.IndexSummaries[index].Count
		}
	} else {
		total = uint(len(p.Inventory.Files))
	}
	cur := uint(0)
	var suitcaseHashes []config.HashSet

	for _, f := range p.Inventory.Files {
		l := slog.With(
			"path", f.Path,
			"index", index)
		if f.SuitcaseIndex != index {
			continue
		}

		l.Debug("Adding file to suitcase",
			"cur", cur,
			"total", total,
		)

		if s.Config().EncryptInner {
			err = s.AddEncrypt(*f)
			if err != nil {
				return nil, fmt.Errorf("encountered error adding file to suitcase: %v", err)
			}
		} else {
			hs, err := s.Add(*f)
			if err != nil {
				return nil, fmt.Errorf("encountered error adding file to suitcase: %v", err)
			}
			if s.Config().HashInner {
				suitcaseHashes = append(suitcaseHashes, *hs)
			}
		}

		cur++
		if stateC != nil {
			stateC <- newInProgressFillState(cur, total, index)
		}
	}
	return suitcaseHashes, nil
}

func newPool(c int) *pool.ErrorPool {
	slog.Debug("setting pool guard", "concurrency", c)
	return pool.New().WithMaxGoroutines(c).WithErrors()
}

func newCompleteFillState(index int) FillState {
	return FillState{
		Completed: true,
		Index:     index,
	}
}

func newInProgressFillState(current, total uint, index int) FillState {
	return FillState{
		Current:        current,
		Total:          total,
		Index:          index,
		CurrentPercent: float64(current) / float64(total) * 100,
	}
}

// FillState is the current state of a suitcase file
type FillState struct {
	Current        uint
	Total          uint
	Completed      bool
	CurrentPercent float64
	Index          int
}
