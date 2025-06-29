/*
Package targpg works the tar.gpg suitcases
*/
package targpg

import (
	"errors"
	"io"

	"github.com/ProtonMail/go-crypto/openpgp"
	"github.com/scttfrdmn/cargoship-cli/pkg/config"
	"github.com/scttfrdmn/cargoship-cli/pkg/inventory"
	"github.com/scttfrdmn/cargoship-cli/pkg/suitcase/tar"
)

// Suitcase as a tar.gpg
type Suitcase struct {
	tw     *tar.Suitcase
	cw     *io.WriteCloser
	opts   *config.SuitCaseOpts
	hashes []config.HashSet
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
		tw:   tw,
		opts: opts,
	}
}

// Config is the configuration for a suitcase
func (s Suitcase) Config() *config.SuitCaseOpts {
	return s.opts
}

// GetHashes returns all the hashes
func (s Suitcase) GetHashes() []config.HashSet {
	return s.hashes
}

// Close all closeables.
func (s Suitcase) Close() error {
	// Cipher Writer Close
	item := *s.cw
	if err := item.Close(); err != nil {
		return err
	}
	return s.tw.Close()
}

// Add file to the archive.
func (s Suitcase) Add(f inventory.File) (*config.HashSet, error) {
	return s.tw.Add(f)
}

// AddEncrypt Add encrypted file to the archive.
func (s Suitcase) AddEncrypt(_ inventory.File) error {
	return errors.New("file encryption not supported on already encrypted archives")
}
