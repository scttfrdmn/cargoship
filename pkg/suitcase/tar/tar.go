/*
Package tar provides simple tar suitcases
*/
package tar

import (
	"archive/tar"
	"bytes"
	"crypto/sha256"
	"fmt"
	"io"
	"os"
	"path/filepath"

	"github.com/rs/zerolog/log"
	"gitlab.oit.duke.edu/devil-ops/suitcasectl/pkg/config"
	"gitlab.oit.duke.edu/devil-ops/suitcasectl/pkg/gpg"
	"gitlab.oit.duke.edu/devil-ops/suitcasectl/pkg/inventory"
)

// Suitcase as tar.
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

// Config is the configuration for a suitcase
func (a Suitcase) Config() *config.SuitCaseOpts {
	return a.opts
}

// Close all closeables.
func (a Suitcase) Close() error {
	return a.tw.Close()
}

// Add file to the archive.
func (a Suitcase) Add(f inventory.File) (*inventory.HashSet, error) {
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
	if err = a.tw.WriteHeader(header); err != nil {
		return nil, err
	}
	file, err := os.Open(f.Path) // #nosec
	if err != nil {
		return nil, err
	}

	defer dclose(file)
	var hs *inventory.HashSet
	if a.opts.HashInner {
		absPath, ferr := filepath.Abs(f.Path)
		if ferr != nil {
			return nil, ferr
		}

		// Get the contents in a temp buffer that we can calculate the hash on
		buf := bytes.NewBuffer(nil)
		_, cerr := io.Copy(buf, file)
		if cerr != nil {
			return nil, cerr
		}

		// Calculate and return the hash
		h := sha256.Sum256(buf.Bytes())
		hs = &inventory.HashSet{
			Filename: absPath,
			Hash:     fmt.Sprintf("%x", h),
		}
		// Reset the cursor so it can go back in the archive
		_, serr := file.Seek(0, 0)
		if serr != nil {
			return nil, serr
		}
	}
	_, err = io.Copy(a.tw, file)
	return hs, err
}

// AddEncrypt adds and encrypts file to the archive.
func (a Suitcase) AddEncrypt(f inventory.File) error {
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

	unencryptedData, err := os.ReadFile(f.Path)
	if err != nil {
		return err
	}

	encryptedD, err := gpg.Encrypt(unencryptedData, a.opts.EncryptTo, true)
	if err != nil {
		return err
	}

	eInfo := gpg.FileInfo{
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

func dclose(c io.Closer) {
	err := c.Close()
	if err != nil {
		log.Warn().Interface("closer", c).Msg("error closing file")
	}
}
