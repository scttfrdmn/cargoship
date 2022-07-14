package tar

import (
	"archive/tar"
	"bytes"
	"fmt"
	"io"
	"io/ioutil"
	"os"

	"gitlab.oit.duke.edu/devil-ops/data-suitcase/pkg/config"
	"gitlab.oit.duke.edu/devil-ops/data-suitcase/pkg/gpg"
	"gitlab.oit.duke.edu/devil-ops/data-suitcase/pkg/inventory"
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

func (s Suitcase) Config() *config.SuitCaseOpts {
	return s.opts
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
	if err = a.tw.WriteHeader(header); err != nil {
		return err
	}
	file, err := os.Open(f.Path) // #nosec
	if err != nil {
		return err
	}
	defer file.Close()
	_, err = io.Copy(a.tw, file)
	return err
}

// Add and encrypt file to the archive.
func (a Suitcase) AddEncrypt(f inventory.InventoryFile) error {
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
	dest := fmt.Sprintf("%v.gpg", f.Destination)

	unencryptedData, err := ioutil.ReadFile(f.Path)
	if err != nil {
		return err
	}

	encryptedD, err := gpg.Encrypt(unencryptedData, a.opts.EncryptTo, true)
	if err != nil {
		return err
	}

	eInfo := gpg.GPGFileInfo{
		FileName:         dest,
		Data:             encryptedD,
		OriginalFileInfo: info,
		IsDirectory:      info.IsDir(),
	}

	header, err := tar.FileInfoHeader(&eInfo, link)
	if err != nil {
		return err
	}
	header.Name = dest
	if !f.ModTime.IsZero() {
		header.ModTime = f.ModTime
	}
	if err = a.tw.WriteHeader(header); err != nil {
		return err
	}
	_, err = io.Copy(a.tw, bytes.NewReader(encryptedD))
	return err
}
