package targz

import (
	"compress/gzip"
	"io"

	"gitlab.oit.duke.edu/oit-ssi-systems/data-suitcase/pkg/config"
	"gitlab.oit.duke.edu/oit-ssi-systems/data-suitcase/pkg/inventory"
	"gitlab.oit.duke.edu/oit-ssi-systems/data-suitcase/pkg/suitcase/tar"
)

// Archive as tar.
type Suitcase struct {
	tw   *tar.Suitcase
	gw   *gzip.Writer
	opts *config.SuitCaseOpts
}

// New tar archive.
func New(target io.Writer, opts *config.SuitCaseOpts) Suitcase {
	gw, err := gzip.NewWriterLevel(target, gzip.BestCompression)
	if err != nil {
		panic("UGH NO GZIP WRITER!!")
	}
	tw := tar.New(gw, opts)
	return Suitcase{
		gw:   gw,
		tw:   &tw,
		opts: opts,
	}
}

// Close all closeables.
func (s Suitcase) Close() error {
	// Close tar writer first here!
	if err := s.tw.Close(); err != nil {
		return err
	}
	return s.gw.Close()
}

// Add file to the archive.
func (s Suitcase) Add(f inventory.InventoryFile) error {
	return s.tw.Add(f)
}

// Add and encrypt file to the archive.
func (s Suitcase) AddEncrypt(f inventory.InventoryFile) error {
	return s.tw.AddEncrypt(f)
}
