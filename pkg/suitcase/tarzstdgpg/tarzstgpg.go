/*
Package tarzstgpg provides gpg encrypted tar.zst suitcases
*/
package tarzstgpg

import (
	"errors"
	"io"

	"github.com/ProtonMail/go-crypto/openpgp"
	"github.com/klauspost/compress/zstd"
	"gitlab.oit.duke.edu/devil-ops/suitcasectl/pkg/config"
	"gitlab.oit.duke.edu/devil-ops/suitcasectl/pkg/inventory"
	"gitlab.oit.duke.edu/devil-ops/suitcasectl/pkg/suitcase/tar"
)

// Suitcase holds all the pieces
type Suitcase struct {
	tw     *tar.Suitcase
	cw     *io.WriteCloser
	gw     *zstd.Encoder
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
	gw, err := zstd.NewWriter(cw)
	if err != nil {
		panic("ERROR CREATING ZSTD GPG Writer")
	}
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
func (s Suitcase) GetHashes() []config.HashSet {
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
func (s Suitcase) Add(f inventory.File) (*config.HashSet, error) {
	return s.tw.Add(f)
}

// AddEncrypt Add and encrypt file to the archive.
func (s Suitcase) AddEncrypt(f inventory.File) error {
	return errors.New("file encryption not supported on already encrypted archives")
}
