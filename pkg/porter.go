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
	"context"
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
	"strings"

	"github.com/rs/zerolog/log"

	"github.com/rs/zerolog"
	"github.com/spf13/cobra"
	"github.com/spf13/viper"
	"gitlab.oit.duke.edu/devil-ops/suitcasectl/pkg/config"
	"gitlab.oit.duke.edu/devil-ops/suitcasectl/pkg/inventory"
	"gitlab.oit.duke.edu/devil-ops/suitcasectl/pkg/travelagent"
)

// Porter holds all the pieces of the suitcases together and such
type Porter struct {
	Cmd           *cobra.Command
	Args          []string
	CLIMeta       *CLIMeta
	TravelAgent   *travelagent.TravelAgent
	Inventory     *inventory.Inventory
	InventoryHash string
	Logger        *zerolog.Logger
	HashAlgorithm inventory.HashAlgorithm
	Hashes        []config.HashSet
	UserOverrides *viper.Viper
	Destination   string
	Version       string
	LogFile       *os.File
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
			Hash:     calculateHash(fh, p.HashAlgorithm.String()),
		})
	}
	return hs, nil
}

// SendUpdate sends an update to the travel agent if it exists
func (p Porter) SendUpdate(u travelagent.StatusUpdate) error {
	if p.TravelAgent == nil {
		return nil
	}
	log := *p.Logger

	resp, err := p.TravelAgent.Update(u)
	if err != nil {
		return err
	}
	if p.Logger != nil {
		if u.ComponentName != "" {
			log = log.With().Str("component", u.ComponentName).Logger()
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
	if err != nil {
		panic(err)
	}
	return hex.EncodeToString(dst.Sum(nil))
}

// CreateOrReadInventory returns an inventory and optionally creates it if it didn't exist
func (p *Porter) CreateOrReadInventory(inventoryFile string) (*inventory.Inventory, error) {
	// Create an inventory file if one isn't specified
	var inventoryD *inventory.Inventory
	if inventoryFile == "" {
		p.Logger.Debug().Msg("No inventory file specified, we're going to go ahead and create one")
		var outF *os.File
		var err error
		v := inventory.UserOverrideWithCobra(p.Cmd)
		if p.Destination == "" {
			p.Destination = mustTempDir()
		}
		// inventoryD, outF, err = WriteInventoryAndFileWithViper(v, cmd, args, outDir, version)
		inventoryD, outF, err = inventory.WriteInventoryAndFileWithViper(v, p.Cmd, p.Args, p.Version)
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
	p.Cmd.SetContext(context.WithValue(p.Cmd.Context(), inventory.InventoryKey, inventoryD))
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
