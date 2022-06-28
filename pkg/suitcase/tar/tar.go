package tar

import (
	"archive/tar"
	"io"
	"os"

	"gitlab.oit.duke.edu/oit-ssi-systems/data-suitcase/pkg/config"
	"gitlab.oit.duke.edu/oit-ssi-systems/data-suitcase/pkg/inventory"
)

// Archive as tar.
type Suitcase struct {
	tw   *tar.Writer
	opts *config.SuitCaseOpts
}

// New tar archive.
func New(target io.Writer, opts *config.SuitCaseOpts) Suitcase {
	return Suitcase{
		tw:   tar.NewWriter(target),
		opts: opts,
	}
}

// Close all closeables.
func (a Suitcase) Close() error {
	return a.tw.Close()
}

// Add file to the archive.
func (a Suitcase) Add(f inventory.InventoryFile) error {
	info, err := os.Lstat(f.Path) // #nosec
	if err != nil {
		return err
	}
	var link string
	if info.Mode()&os.ModeSymlink != 0 {
		link, err = os.Readlink(f.Path) // #nosec
		if err != nil {
			return err
		}
	}
	header, err := tar.FileInfoHeader(info, link)
	if err != nil {
		return err
	}
	header.Name = f.Destination
	if !f.ModTime.IsZero() {
		header.ModTime = f.ModTime
	}
	if f.Mode != 0 {
		header.Mode = int64(f.Mode)
	}
	if err = a.tw.WriteHeader(header); err != nil {
		return err
	}
	if info.IsDir() || info.Mode()&os.ModeSymlink != 0 {
		return nil
	}
	file, err := os.Open(f.Path) // #nosec
	if err != nil {
		return err
	}
	defer file.Close()
	_, err = io.Copy(a.tw, file)
	return err
}
