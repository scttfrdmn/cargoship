/*
Package targz creates tar.gz files
*/
package targz

import (
	"io"

	gzip "github.com/klauspost/pgzip"

	"gitlab.oit.duke.edu/devil-ops/suitcasectl/pkg/config"
	"gitlab.oit.duke.edu/devil-ops/suitcasectl/pkg/inventory"
	"gitlab.oit.duke.edu/devil-ops/suitcasectl/pkg/suitcase/tar"
)

// Suitcase represents everything needef for a tar.gz suitcase
type Suitcase struct {
	tw     *tar.Suitcase
	gw     *gzip.Writer
	opts   *config.SuitCaseOpts
	hashes []config.HashSet
}

// New tar archive.
func New(target io.Writer, opts *config.SuitCaseOpts) Suitcase {
	gw, err := gzip.NewWriterLevel(target, gzip.BestCompression)
	if err != nil {
		panic("UGH NO GZIP WRITER!!")
	}
	return Suitcase{
		gw:   gw,
		tw:   tar.New(gw, opts),
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

// Config returns the config options
func (s Suitcase) Config() *config.SuitCaseOpts {
	return s.opts
}

// GetHashes returns the hashes
func (s Suitcase) GetHashes() []config.HashSet {
	return s.hashes
}

// Add file to the archive.
func (s Suitcase) Add(f inventory.File) (*config.HashSet, error) {
	return s.tw.Add(f)
}

// AddEncrypt Adds and encrypt file to the archive.
func (s Suitcase) AddEncrypt(f inventory.File) error {
	return s.tw.AddEncrypt(f)
}
