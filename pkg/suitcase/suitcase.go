package suitcase

import (
	"fmt"
	"io"

	"github.com/apex/log"
	"gitlab.oit.duke.edu/oit-ssi-systems/data-suitcase/pkg/config"
	"gitlab.oit.duke.edu/oit-ssi-systems/data-suitcase/pkg/inventory"
	"gitlab.oit.duke.edu/oit-ssi-systems/data-suitcase/pkg/suitcase/tar"
	"gitlab.oit.duke.edu/oit-ssi-systems/data-suitcase/pkg/suitcase/targpg"
)

type Suitcase interface {
	Close() error
	Add(f inventory.InventoryFile) error
}

func New(w io.Writer, opts *config.SuitCaseOpts) (Suitcase, error) {
	switch opts.Format {
	case "tar":
		return tar.New(w, opts), nil
	case "tar.gpg":
		return targpg.New(w, opts), nil
	}
	return nil, fmt.Errorf("invalid archive format: %s", opts.Format)
}

func FillWithInventory(s Suitcase, i *inventory.DirectoryInventory) error {
	var err error
	defer s.Close()
	fs := []inventory.InventoryFile{}
	fs = append(fs, i.SmallFiles...)
	fs = append(fs, i.LargeFiles...)

	for _, f := range fs {
		log.WithFields(log.Fields{
			"path": f.Path,
			"dest": f.Destination,
		}).Debug("Adding file to suitcase")
		err = s.Add(f)
		if err != nil {
			log.WithError(err).WithFields(log.Fields{
				"path": f.Path,
			}).Warn("Error adding file")
		}
	}
	return nil
}

/*
func NewSuitCase(opts *SuitCaseOpts) error {
	// Create new Writers for gzip and tar
	// These writers are chained. Writing to the tar writer will
	// write to the gzip writer which in turn will write to
	// the "buf" writer
	var tw *tar.Writer

	if opts == nil {
		return errors.New("We need options passed")
	}
	buf, err := os.Create(opts.Destination)
	if err != nil {
		return err
	}
	defer buf.Close()

	gw := gzip.NewWriter(buf)
	defer gw.Close()
	tw = tar.NewWriter(gw)
	defer tw.Close()

	// Iterate over files and add them to the tar archive
	allFiles := []inventory.InventoryFile{}
	allFiles = append(allFiles, opts.Inventory.LargeFiles...)
	allFiles = append(allFiles, opts.Inventory.SmallFiles...)
	for _, file := range allFiles {
		if opts.EncryptInner {
			err = addEncryptedToArchive(tw, file.Path, opts.EncryptTo)
		} else {
			err = addToArchive(tw, file.Path)
		}
		if err != nil {
			log.WithError(err).WithField("file", file.Path).Warn("Error adding file")
		}
	}

	return nil
}

func addToArchive(tw *tar.Writer, filename string) error {
	// Open the file which will be written into the archive
	file, err := os.Open(filename)
	if err != nil {
		return err
	}
	defer file.Close()

	// Get FileInfo about our file providing file size, mode, etc.
	info, err := file.Stat()
	if err != nil {
		return err
	}

	// Create a tar Header from the FileInfo data
	header, err := tar.FileInfoHeader(info, info.Name())
	if err != nil {
		return err
	}

	// Use full path as name (FileInfoHeader only takes the basename)
	// If we don't do this the directory strucuture would
	// not be preserved
	// https://golang.org/src/archive/tar/common.go?#L626
	header.Name = filename

	// Write file header to the tar archive
	err = tw.WriteHeader(header)
	if err != nil {
		return err
	}

	// Copy file content to tar archive
	_, err = io.Copy(tw, file)
	if err != nil {
		return err
	}

	return nil
}

func addEncryptedToArchive(tw *tar.Writer, filename string, to *openpgp.EntityList) error {
	// Open the file which will be written into the archive
	eFilename := fmt.Sprintf("%s.gpg", filename)
	file, err := os.Open(filename)
	if err != nil {
		return err
	}
	defer file.Close()

	// Get FileInfo about our file providing file size, mode, etc.
	info, err := file.Stat()
	if err != nil {
		return err
	}
	log.Debugf("INFO: %+v", info)

	unEncrypted := bytes.NewBuffer(nil)
	io.Copy(unEncrypted, file)
	if err != nil {
		return err
	}
	// I really want a reader to pass here, not just the bytes, for performance
	encrypted, err := gpg.Encrypt(unEncrypted.Bytes(), to, false)
	if err != nil {
		return err
	}

	gpgF, err := gpg.NewGPGFileInfo(encrypted, info)
	if err != nil {
		return err
	}
	// Create a tar Header from the FileInfo data
	eName := fmt.Sprintf("%s.gpg", info.Name())
	log.WithField("name", eName).Debug("Encrypted file Name()")
	header, err := tar.FileInfoHeader(gpgF, eName)
	if err != nil {
		return err
	}

	header.Name = eFilename

	// Write file header to the tar archive
	log.WithFields(log.Fields{
		"header.name": header.Name,
	}).Debug("Header info")
	err = tw.WriteHeader(header)
	if err != nil {
		return err
	}
	encryptedReader := bytes.NewReader(encrypted)

	// Copy file content to tar archive
		//_, err = tw.Write(encrypted)
		//if err != nil {
			//return err
		//}
	_, err = io.Copy(tw, encryptedReader)
	if err != nil {
		return err
	}

	return nil
}

func NewEncryptedSuitCase(opts *SuitCaseOpts) error {
	// Create new Writers for gzip and tar
	// These writers are chained. Writing to the tar writer will
	// write to the gzip writer which in turn will write to
	// the "buf" writer
	var tw *tar.Writer

	var dst string
	if strings.HasSuffix(opts.Destination, ".gpg") {
		dst = opts.Destination
	} else {
		dst = fmt.Sprintf("%v.gpg", opts.Destination)
	}

	if opts == nil {
		return errors.New("We need options passed")
	}
	buf, err := os.Create(dst)
	if err != nil {
		return err
	}
	defer buf.Close()
	cw, err := openpgp.Encrypt(buf, *opts.EncryptTo, nil, nil, nil)
	if err != nil {
		return errors.New("Bad Cipher")
	}
	tw = tar.NewWriter(cw)
	defer tw.Close()

	// Iterate over files and add them to the tar archive
	allFiles := []inventory.InventoryFile{}
	allFiles = append(allFiles, opts.Inventory.LargeFiles...)
	allFiles = append(allFiles, opts.Inventory.SmallFiles...)
	for _, file := range allFiles {
		if opts.EncryptInner {
			err = addEncryptedToArchive(tw, file.Path, opts.EncryptTo)
		} else {
			err = addToArchive(tw, file.Path)
		}
		if err != nil {
			log.WithError(err).WithField("file", file.Path).Warn("Error adding file")
		}
	}
	cw.Close()

	return nil
}

*/
