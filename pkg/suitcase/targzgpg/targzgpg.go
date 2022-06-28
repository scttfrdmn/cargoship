package targzgpg

import (
	"compress/gzip"
	"io"

	"github.com/ProtonMail/go-crypto/openpgp"
	"gitlab.oit.duke.edu/oit-ssi-systems/data-suitcase/pkg/config"
	"gitlab.oit.duke.edu/oit-ssi-systems/data-suitcase/pkg/inventory"
	"gitlab.oit.duke.edu/oit-ssi-systems/data-suitcase/pkg/suitcase/tar"
)

// Archive as tar.
type Suitcase struct {
	tw   *tar.Suitcase
	cw   *io.WriteCloser
	gw   *gzip.Writer
	opts *config.SuitCaseOpts
}

// New tar archive.
func New(target io.Writer, opts *config.SuitCaseOpts) Suitcase {
	if opts.EncryptTo == nil {
		panic("NEED ENCRYPT TO")
	}
	cw, _ := openpgp.Encrypt(target, *opts.EncryptTo, nil, &openpgp.FileHints{
		IsBinary: true,
	}, nil)
	gw, _ := gzip.NewWriterLevel(cw, gzip.BestCompression)
	tw := tar.New(gw, opts)
	// tw := tar.New(cw, opts)
	return Suitcase{
		cw:   &cw,
		tw:   &tw,
		gw:   gw,
		opts: opts,
	}
}

// Close all closeables.
func (s Suitcase) Close() error {
	// Gzip -> Cipher -> Tar works, corrupt file
	// Gzip -> Tar -> Cipher works, corrupt file
	// Cipher -> Tar -> Gzip -> busted test
	// Cipher -> Gzip -> Tar -> Busted test
	// Tar -> Gzip -> Cipher -> Works, corrupt file
	// Tar -> Cipher -> Gzip -> busted test

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
func (s Suitcase) Add(f inventory.InventoryFile) error {
	return s.tw.Add(f)
}
