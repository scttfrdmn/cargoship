package tar

import (
	"archive/tar"
	"bytes"
	"crypto/sha256"
	"fmt"
	"io"
	"io/ioutil"
	"os"
	"path/filepath"

	"gitlab.oit.duke.edu/devil-ops/data-suitcase/pkg/config"
	"gitlab.oit.duke.edu/devil-ops/data-suitcase/pkg/gpg"
	"gitlab.oit.duke.edu/devil-ops/data-suitcase/pkg/helpers"
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
func (a Suitcase) Add(f inventory.InventoryFile) (*helpers.HashSet, error) {
	info, err := os.Lstat(f.Path) // #nosec
	if err != nil {
		return nil, err
	}
	var link string

	if info.Mode()&os.ModeSymlink != 0 {
		link, err = os.Readlink(f.Path) // #nosec
		if err != nil {
			return nil, err
		}
	}
	header, err := tar.FileInfoHeader(info, link)
	if err != nil {
		return nil, err
	}
	header.Name = f.Destination
	/*
		if !f.ModTime.IsZero() {
			header.ModTime = f.ModTime
		}
	*/
	if err = a.tw.WriteHeader(header); err != nil {
		return nil, err
	}
	file, err := os.Open(f.Path) // #nosec
	if err != nil {
		return nil, err
	}
	defer file.Close()
	var hs *helpers.HashSet
	if a.opts.HashInner {
		absPath, err := filepath.Abs(f.Path)
		if err != nil {
			return nil, err
		}

		// Get the contents in a temp buffer that we can calculate the hash on
		buf := bytes.NewBuffer(nil)
		io.Copy(buf, file)
		if err != nil {
			return nil, err
		}

		// Calculate and return the hash
		h := sha256.Sum256(buf.Bytes())
		hs = &helpers.HashSet{
			Filename: absPath,
			Hash:     fmt.Sprintf("%x", h),
		}
		// Reset the cursor so it can go back in the archive
		file.Seek(0, 0)
	}
	_, err = io.Copy(a.tw, file)
	return hs, err
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
	/*
		if !f.ModTime.IsZero() {
			header.ModTime = f.ModTime
		}
	*/
	if err = a.tw.WriteHeader(header); err != nil {
		return err
	}
	_, err = io.Copy(a.tw, bytes.NewReader(encryptedD))
	return err
}
