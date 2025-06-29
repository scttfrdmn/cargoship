/*
Package tarzstd creates tar.zst files

https://facebook.github.io/zstd/
*/
package tarzstd

import (
	"io"

	// gzip "github.com/klauspost/pgzip"
	"github.com/klauspost/compress/zstd"

	"github.com/scttfrdmn/cargoship/pkg/config"
	"github.com/scttfrdmn/cargoship/pkg/inventory"
	"github.com/scttfrdmn/cargoship/pkg/suitcase/tar"
)

// Suitcase represents everything needef for a tar.gz suitcase
type Suitcase struct {
	tw     *tar.Suitcase
	gw     *zstd.Encoder
	opts   *config.SuitCaseOpts
	hashes []config.HashSet
}

// New tar archive.
func New(target io.Writer, opts *config.SuitCaseOpts) Suitcase {
	gw, err := zstd.NewWriter(target)
	if err != nil {
		panic("UGH NO ZSTD WRITER!!")
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
