package targzgpg

import (
	gzip "github.com/klauspost/pgzip"
	"errors"
	"io"

	"github.com/ProtonMail/go-crypto/openpgp"
	"gitlab.oit.duke.edu/devil-ops/data-suitcase/pkg/config"
	"gitlab.oit.duke.edu/devil-ops/data-suitcase/pkg/helpers"
	"gitlab.oit.duke.edu/devil-ops/data-suitcase/pkg/inventory"
	"gitlab.oit.duke.edu/devil-ops/data-suitcase/pkg/suitcase/tar"
)

// Archive as tar.
type Suitcase struct {
	tw     *tar.Suitcase
	cw     *io.WriteCloser
	gw     *gzip.Writer
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

func (s Suitcase) Config() *config.SuitCaseOpts {
	return s.opts
}

func (s Suitcase) GetHashes() []helpers.HashSet {
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
func (s Suitcase) Add(f inventory.InventoryFile) (*helpers.HashSet, error) {
	return s.tw.Add(f)
}

// Add and encrypt file to the archive.
func (s Suitcase) AddEncrypt(f inventory.InventoryFile) error {
	return errors.New("file encryption not supported on already encrypted archives")
}
