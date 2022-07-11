package suitcase

import (
	"fmt"
	"io"
	"os"
	"os/exec"
	"path"
	"strings"

	"github.com/rs/zerolog/log"
	"gitlab.oit.duke.edu/oit-ssi-systems/data-suitcase/pkg/config"
	"gitlab.oit.duke.edu/oit-ssi-systems/data-suitcase/pkg/inventory"
	"gitlab.oit.duke.edu/oit-ssi-systems/data-suitcase/pkg/suitcase/tar"
	"gitlab.oit.duke.edu/oit-ssi-systems/data-suitcase/pkg/suitcase/targpg"
	"gitlab.oit.duke.edu/oit-ssi-systems/data-suitcase/pkg/suitcase/targz"
	"gitlab.oit.duke.edu/oit-ssi-systems/data-suitcase/pkg/suitcase/targzgpg"
)

type Suitcase interface {
	Close() error
	Add(f inventory.InventoryFile) error
	AddEncrypt(f inventory.InventoryFile) error
	Config() *config.SuitCaseOpts
}

type SuitcaseFillState struct {
	Current        uint
	Total          uint
	Completed      bool
	CurrentPercent float64
	Index          int
}

// Create a new suitcase
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

func FillWithInventoryIndex(s Suitcase, i *inventory.DirectoryInventory, index int, stateC chan SuitcaseFillState) error {
	var err error

	var total uint
	if index > 0 {
		total = i.IndexSummaries[index].Count
	} else {
		total = uint(len(i.Files))
	}
	cur := uint(0)

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
			err = s.Add(*f)
			if err != nil {
				l.Warn().Err(err).Msg("Failed to add file to suitcase")
			}
		}

		cur++
		if stateC != nil {
			stateC <- SuitcaseFillState{
				Current:        cur,
				Total:          total,
				Index:          index,
				CurrentPercent: float64(cur) / float64(total) * 100,
			}
		}

	}
	return nil
}

func WriteSuitcaseFile(so *config.SuitCaseOpts, i *inventory.DirectoryInventory, index int, stateC chan SuitcaseFillState) (string, error) {
	targetF := path.Join(so.Destination, fmt.Sprintf("%v-%d.%v", i.Options.Name, index, so.Format))
	target, err := os.Create(targetF)
	if err != nil {
		return "", err
	}
	defer target.Close()

	s, err := New(target, so)
	if err != nil {
		return "", err
	}
	defer s.Close()

	log.Info().
		Str("destination", targetF).
		Str("format", so.Format).
		Bool("encryptInner", so.EncryptInner).
		Int("index", index).
		Msg("Filling suitcase")
	err = FillWithInventoryIndex(s, i, index, stateC)
	if err != nil {
		return "", err
	}

	if stateC != nil {
		stateC <- SuitcaseFillState{
			Completed: true,
			Index:     index,
		}
	}
	return targetF, nil
}

func PostProcess(s Suitcase) error {
	c := s.Config()
	cmd := exec.Command(c.PostProcessScript)
	cmd.Env = append(cmd.Env, "SUITCASE_DESTINATION="+c.Destination)
	err := cmd.Run()
	if err != nil {
		return err
	}
	return nil
}
