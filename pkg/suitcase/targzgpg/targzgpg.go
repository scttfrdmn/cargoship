/*
Package targzgpg provides gpg encrypted tar.gz suitcases
*/
package targzgpg

import (
	"errors"
	"io"

	"github.com/klauspost/pgzip"

	"github.com/ProtonMail/go-crypto/openpgp"
	"gitlab.oit.duke.edu/devil-ops/suitcasectl/pkg/config"
	"gitlab.oit.duke.edu/devil-ops/suitcasectl/pkg/inventory"
	"gitlab.oit.duke.edu/devil-ops/suitcasectl/pkg/suitcase/tar"
)

// Suitcase holds all the pieces
type Suitcase struct {
	tw     *tar.Suitcase
	cw     *io.WriteCloser
	gw     *pgzip.Writer
	opts   *config.SuitCaseOpts
	hashes []inventory.HashSet
}

// New tar archive.
func New(target io.Writer, opts *config.SuitCaseOpts) Suitcase {
	if opts.EncryptTo == nil {
		panic("NEED ENCRYPT TO")
	}
	cw, _ := openpgp.Encrypt(target, *opts.EncryptTo, nil, &openpgp.FileHints{
		IsBinary: true,
	}, nil)
	gw, _ := pgzip.NewWriterLevel(cw, pgzip.BestCompression)
	tw := tar.New(gw, opts)
	return Suitcase{
		cw:   &cw,
		tw:   tw,
		gw:   gw,
		opts: opts,
	}
}

// Config returns configuration options
func (s Suitcase) Config() *config.SuitCaseOpts {
	return s.opts
}

// GetHashes returns hashses
func (s Suitcase) GetHashes() []inventory.HashSet {
	return s.hashes
}

// Close all closeables.
func (s Suitcase) Close() error {
	// Tar -> Gzip -> Cipher -> Works as intended

	// Tar Writer
	if err := s.tw.Close(); err != nil {
		return err
	}

	// Gzip Writer
	if err := s.gw.Close(); err != nil {
		return err
	}

	// Cipher Writer
	item := *s.cw
	if err := item.Close(); err != nil {
		return err
	}

	return nil
	// return s.tw.Close()
}

// Add file to the archive.
func (s Suitcase) Add(f inventory.File) (*inventory.HashSet, error) {
	return s.tw.Add(f)
}

// AddEncrypt Add and encrypt file to the archive.
func (s Suitcase) AddEncrypt(f inventory.File) error {
	return errors.New("file encryption not supported on already encrypted archives")
}
