/*
Package suitcase holds all the stuff necessary for a suitecase generation
*/
package suitcase

import (
	"errors"
	"fmt"
	"io"
	"os"
	"os/exec"
	"path"
	"strings"

	"github.com/rs/zerolog/log"
	"gitlab.oit.duke.edu/devil-ops/data-suitcase/pkg/config"
	"gitlab.oit.duke.edu/devil-ops/data-suitcase/pkg/helpers"
	"gitlab.oit.duke.edu/devil-ops/data-suitcase/pkg/inventory"
	"gitlab.oit.duke.edu/devil-ops/data-suitcase/pkg/suitcase/tar"
	"gitlab.oit.duke.edu/devil-ops/data-suitcase/pkg/suitcase/targpg"
	"gitlab.oit.duke.edu/devil-ops/data-suitcase/pkg/suitcase/targz"
	"gitlab.oit.duke.edu/devil-ops/data-suitcase/pkg/suitcase/targzgpg"
)

// Suitcase is the interface that describes what a Suitcase does
type Suitcase interface {
	Close() error
	Add(inventory.File) (*helpers.HashSet, error)
	AddEncrypt(f inventory.File) error
	Config() *config.SuitCaseOpts
}

// FillState is the current state of a suitcase file
type FillState struct {
	Current        uint
	Total          uint
	Completed      bool
	CurrentPercent float64
	Index          int
}

// New Create a new suitcase
func New(w io.Writer, opts *config.SuitCaseOpts) (Suitcase, error) {
	// Decide if we are encrypting the whole shebang or not
	if strings.HasSuffix(opts.Format, ".gpg") {
		opts.EncryptOuter = true
	}
	// We may want to allow this later...but not yet
	if opts.EncryptInner && opts.EncryptOuter {
		return nil, fmt.Errorf("cannot encrypt inner and outer")
	}
	// If we are encrypting something, be sure encryptTo is set
	if (opts.EncryptInner || opts.EncryptOuter) && opts.EncryptTo == nil {
		return nil, fmt.Errorf("cannot encrypt without EncryptTo")
	}
	switch opts.Format {
	case "tar":
		return tar.New(w, opts), nil
	case "tar.gpg":
		return targpg.New(w, opts), nil
	case "tar.gz":
		return targz.New(w, opts), nil
	case "tar.gz.gpg":
		return targzgpg.New(w, opts), nil
	}
	return nil, fmt.Errorf("invalid archive format: %s", opts.Format)
}

// FillWithInventoryIndex fills up a suitcase using the given inventory
func FillWithInventoryIndex(s Suitcase, i *inventory.DirectoryInventory, index int, stateC chan FillState) ([]helpers.HashSet, error) {
	if i == nil {
		return nil, errors.New("inventory is nil")
	}
	var err error

	var total uint
	if index > 0 {
		if _, ok := i.IndexSummaries[index]; ok {
			total = i.IndexSummaries[index].Count
		}
	} else {
		total = uint(len(i.Files))
	}
	cur := uint(0)
	var suitcaseHashes []helpers.HashSet

	for _, f := range i.Files {
		l := log.With().
			Str("path", f.Path).
			Int("index", index).
			Logger()
		if f.SuitcaseIndex != index {
			continue
		}

		l.Debug().
			Uint("cur", cur).
			Uint("total", total).
			Msg("Adding file to suitcase")

		if s.Config().EncryptInner {
			err = s.AddEncrypt(*f)
			if err != nil {
				l.Warn().Err(err).Msg("Failed to add file to suitcase")
			}
		} else {
			hs, err := s.Add(*f)
			if err != nil {
				l.Warn().Err(err).Msg("Failed to add file to suitcase")
			}
			if s.Config().HashInner {
				suitcaseHashes = append(suitcaseHashes, *hs)
			}
		}

		cur++
		if stateC != nil {
			stateC <- FillState{
				Current:        cur,
				Total:          total,
				Index:          index,
				CurrentPercent: float64(cur) / float64(total) * 100,
			}
		}
	}
	return suitcaseHashes, nil
}

// WriteSuitcaseFile will write out the suitcase
func WriteSuitcaseFile(so *config.SuitCaseOpts, i *inventory.DirectoryInventory, index int, stateC chan FillState) (string, error) {
	targetF := path.Join(so.Destination, inventory.FormatSuitcaseName(i.Options.Prefix, i.Options.User, index, i.TotalIndexes, so.Format))
	target, err := os.Create(targetF) // nolint:gosec
	if err != nil {
		return "", err
	}
	defer func() {
		terr := target.Close()
		if terr != nil {
			panic(terr)
		}
	}()

	s, err := New(target, so)
	if err != nil {
		return "", err
	}
	defer func() {
		serr := s.Close()
		if serr != nil {
			panic(serr)
		}
	}()

	log.Info().
		Str("destination", targetF).
		Str("format", so.Format).
		Bool("encryptInner", so.EncryptInner).
		Int("index", index).
		Msg("Filling suitcase")
	hashes, err := FillWithInventoryIndex(s, i, index, stateC)
	if err != nil {
		return "", err
	}

	if stateC != nil {
		stateC <- FillState{
			Completed: true,
			Index:     index,
		}
	}

	if so.HashInner {
		hashFN := fmt.Sprintf("%v.sha256", targetF)
		log.Info().Str("file", hashFN).Msgf("Creating hashes file")
		hashF, err := os.Create(hashFN) // nolint:gosec
		if err != nil {
			return "", err
		}
		defer func() {
			herr := hashF.Close()
			if herr != nil {
				panic(herr)
			}
		}()
		err = helpers.WriteHashFile(hashes, hashF)
		if err != nil {
			return "", err
		}
	}
	return targetF, nil
}

// PostProcess executes post processing commands
func PostProcess(s Suitcase) error {
	c := s.Config()
	cmd := exec.Command(c.PostProcessScript) // nolint:gosec
	cmd.Env = append(cmd.Env, "SUITCASE_DESTINATION="+c.Destination)
	err := cmd.Run()
	if err != nil {
		return err
	}
	return nil
}
