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
	"os"
	"path"
	"strings"
	"time"

	"github.com/rs/zerolog/log"

	"github.com/rs/zerolog"
	"github.com/spf13/cobra"
	"github.com/spf13/viper"
	"gitlab.oit.duke.edu/devil-ops/suitcasectl/pkg/config"
	"gitlab.oit.duke.edu/devil-ops/suitcasectl/pkg/inventory"
	"gitlab.oit.duke.edu/devil-ops/suitcasectl/pkg/rclone"
	"gitlab.oit.duke.edu/devil-ops/suitcasectl/pkg/travelagent"
)

// Porter holds all the pieces of the suitcases together and such. Trying to
// flatten this nest of modules together, this is the first step in getting
// something that can perform that way
type Porter struct {
	Cmd            *cobra.Command
	Args           []string
	CLIMeta        *CLIMeta
	TravelAgent    travelagent.TravelAgenter
	hasTravelAgent bool
	Inventory      *inventory.Inventory
	InventoryHash  string
	Logger         *zerolog.Logger
	HashAlgorithm  inventory.HashAlgorithm
	Hashes         []config.HashSet
	UserOverrides  *viper.Viper
	Destination    string
	Version        string
	SuitcaseOpts   *config.SuitCaseOpts
	LogFile        *os.File
}

// New returns a new porter using functional options
func New(options ...func(*Porter)) *Porter {
	p := &Porter{
		Logger: &log.Logger,
	}
	for _, opt := range options {
		opt(p)
	}
	return p
}

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
func WithLogger(l *zerolog.Logger) func(*Porter) {
	return func(p *Porter) {
		p.Logger = l
	}
}

// WithDestination sets the destination at create time
func WithDestination(s string) func(*Porter) {
	return func(p *Porter) {
		p.Destination = s
	}
}

// WithHashAlgorithm sets the hash algorithm at create time
func WithHashAlgorithm(h inventory.HashAlgorithm) func(*Porter) {
	return func(p *Porter) {
		p.HashAlgorithm = h
	}
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
		p.Logger.Info().Str("file", f).Msg("Created file")
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
	log := *p.Logger

	resp, err := p.TravelAgent.Update(u)
	if err != nil {
		return err
	}
	if p.Logger != nil {
		if u.Name != "" {
			log = log.With().Str("component", u.Name).Logger()
		}
		for _, msg := range resp.Messages {
			if strings.TrimSpace(msg) != "updated fields:" {
				log.Info().Msg(msg)
			}
		}
	}

	return nil
}

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

// CreateOrReadInventory returns an inventory and optionally creates it if it didn't exist
func (p *Porter) CreateOrReadInventory(inventoryFile string) (*inventory.Inventory, error) {
	// Create an inventory file if one isn't specified
	var inventoryD *inventory.Inventory
	if inventoryFile == "" {
		p.Logger.Debug().Msg("No inventory file specified, we're going to go ahead and create one")
		var outF *os.File
		var err error
		p.UserOverrides = p.getUserOverrides()
		if p.Destination == "" {
			p.Destination = mustTempDir()
		}
		// inventoryD, outF, err = WriteInventoryAndFileWithViper(v, cmd, args, outDir, version)
		inventoryD, outF, err = p.WriteInventory()
		if err != nil {
			return nil, err
		}
		inventoryFile = outF.Name()
		log.Debug().Str("file", inventoryFile).Msg("Created inventory file")
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
	// p.Cmd.SetContext(context.WithValue(p.Cmd.Context(), inventory.InventoryHash, h))
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
	i, err := inventory.NewDirectoryInventory(
		inventory.NewOptions(
			inventory.WithViper(p.UserOverrides),
			inventory.WithCobra(p.Cmd, p.Args),
		),
	)
	if err != nil {
		return nil, nil, err
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
			log.Warn().Str("retry-interval", retryInterval.String()).Msg("suitcase transport failed, sleeping, then will retry")
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
