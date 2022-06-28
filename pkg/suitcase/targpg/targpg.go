package targpg

import (
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
	tw := tar.New(cw, opts)
	return Suitcase{
		cw:   &cw,
		tw:   &tw,
		opts: opts,
	}
}

// Close all closeables.
func (s Suitcase) Close() error {
	item := *s.cw
	if err := item.Close(); err != nil {
		return err
	}
	return s.tw.Close()
}

// Add file to the archive.
func (s Suitcase) Add(f inventory.InventoryFile) error {
	return s.tw.Add(f)
}
