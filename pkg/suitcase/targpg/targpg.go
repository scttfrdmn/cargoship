package targpg

import (
	"errors"
	"io"

	"github.com/ProtonMail/go-crypto/openpgp"
	"github.com/rs/zerolog/log"
	"gitlab.oit.duke.edu/devil-ops/data-suitcase/pkg/config"
	"gitlab.oit.duke.edu/devil-ops/data-suitcase/pkg/helpers"
	"gitlab.oit.duke.edu/devil-ops/data-suitcase/pkg/inventory"
	"gitlab.oit.duke.edu/devil-ops/data-suitcase/pkg/suitcase/tar"
)

// Archive as tar.
type Suitcase struct {
	tw     *tar.Suitcase
	cw     *io.WriteCloser
	opts   *config.SuitCaseOpts
	hashes []helpers.HashSet
}

// New tar archive.
func New(target io.Writer, opts *config.SuitCaseOpts) Suitcase {
	if opts.EncryptTo == nil {
		panic("NEED ENCRYPT TO")
	}
	cw, _ := openpgp.Encrypt(target, *opts.EncryptTo, nil, &openpgp.FileHints{
		IsBinary: true,
	}, nil)
	tw := tar.New(cw, opts)
	return Suitcase{
		cw:   &cw,
		tw:   &tw,
		opts: opts,
	}
}

func (s Suitcase) Config() *config.SuitCaseOpts {
	return s.opts
}

func (s Suitcase) GetHashes() []helpers.HashSet {
	return s.hashes
}

// Close all closeables.
func (s Suitcase) Close() error {
	// Cipher Writer Close
	log.Debug().Msg("Closing Cipher Writer")
	item := *s.cw
	if err := item.Close(); err != nil {
		return err
	}

	// Tar Writer Close
	/*
		if err := s.tw.Close(); err != nil {
			return err
		}
	*/

	log.Debug().Msg("Closing Tar Writer")
	return s.tw.Close()
}

// Add file to the archive.
func (s Suitcase) Add(f inventory.InventoryFile) (*helpers.HashSet, error) {
	return s.tw.Add(f)
}

// Add encrypted file to the archive.
func (s Suitcase) AddEncrypt(f inventory.InventoryFile) error {
	return errors.New("file encryption not supported on already encrypted archives")
}
