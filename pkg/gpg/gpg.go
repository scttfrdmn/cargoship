/*
Package gpg provides encrypted files
*/
package gpg

import (
	"bytes"
	"errors"
	"fmt"
	"io"
	"os"
	"path"
	"path/filepath"
	"time"

	"github.com/go-git/go-git/v5"
	"github.com/rs/zerolog/log"
	"github.com/spf13/cobra"

	"github.com/ProtonMail/go-crypto/openpgp"
	"github.com/ProtonMail/go-crypto/openpgp/armor"
	"github.com/ProtonMail/go-crypto/openpgp/packet"
)

// EncryptToWithCmd uses a cobra.Command to create an EntityList
func EncryptToWithCmd(cmd *cobra.Command) (*openpgp.EntityList, error) {
	pubKeyFiles, err := cmd.Flags().GetStringArray("public-key")
	if err != nil {
		return nil, err
	}

	excludeSystems, err := cmd.Flags().GetBool("exclude-systems-pubkeys")
	if err != nil {
		return nil, err
	}
	encryptTo := &openpgp.EntityList{}
	if !excludeSystems {
		encryptTo, err = CollectGPGPubKeys("")
		if err != nil {
			return nil, err
		}
	}

	for _, pkf := range pubKeyFiles {
		pke, err := ReadEntity(pkf)
		if err != nil {
			return nil, err
		}
		*encryptTo = append(*encryptTo, pke)
	}
	return encryptTo, nil
}

// Encrypt the provided bytes for the provided encryption
// keys recipients. Returns the encrypted content bytes.
func Encrypt(d []byte, encryptionKeys *openpgp.EntityList, useArmor bool) ([]byte, error) {
	buffer := &bytes.Buffer{}
	var armoredWriter io.WriteCloser
	var cipheredWriter io.WriteCloser
	var err error

	// Create an openpgp armored cipher writer pointing on our
	// buffer
	if useArmor {
		armoredWriter, err = armor.Encode(buffer, "PGP MESSAGE", nil)
		if err != nil {
			return nil, errors.New("bad Writer")
		}
		cipheredWriter, err = openpgp.Encrypt(armoredWriter, *encryptionKeys, nil, nil, nil)
		if err != nil {
			return nil, errors.New("bad Cipher")
		}
	} else {
		cipheredWriter, err = openpgp.Encrypt(buffer, *encryptionKeys, nil, nil, nil)
		if err != nil {
			return nil, errors.New("bad Cipher")
		}
	}

	// Create an encrypted writer using the provided encryption keys

	// Write (encrypts on the fly) the provided bytes to
	// cipheredWriter
	_, err = cipheredWriter.Write(d)
	if err != nil {
		return nil, errors.New("bad ciphered writer")
	}

	err = cipheredWriter.Close()
	if err != nil {
		return nil, err
	}
	if useArmor {
		err = armoredWriter.Close()
		if err != nil {
			return nil, err
		}
	}

	return buffer.Bytes(), nil
}

// ReadEntity returns an Entity from a string
func ReadEntity(name string) (*openpgp.Entity, error) {
	f, err := os.Open(name) // nolint:gosec
	if err != nil {
		return nil, err
	}
	defer func() {
		cerr := f.Close()
		if cerr != nil {
			panic(cerr)
		}
	}()
	block, err := armor.Decode(f)
	if err != nil {
		return nil, err
	}
	return openpgp.ReadEntity(packet.NewReader(block.Body))
}

// CollectGPGPubKeys returns an EntityList from a place of pub keys
func CollectGPGPubKeys(fp string) (*openpgp.EntityList, error) {
	var els openpgp.EntityList

	if fp == "" {
		gitlabKeysURL := "https://gitlab.oit.duke.edu/oit-ssi-systems/staff-public-keys.git"
		subDir := "linux"
		tmpdir, err := os.MkdirTemp("", "gpg-pub-tmpdir")
		if err != nil {
			return nil, err
		}
		defer func() {
			rerr := os.RemoveAll(tmpdir)
			if rerr != nil {
				panic(rerr)
			}
		}()
		_, err = git.PlainClone(tmpdir, false, &git.CloneOptions{
			URL:               gitlabKeysURL,
			RecurseSubmodules: git.DefaultSubmoduleRecursionDepth,
		})
		if err != nil {
			return nil, err
		}
		fp = path.Join(tmpdir, subDir)
		log.Info().
			Str("url", gitlabKeysURL).
			Str("subdir", subDir).
			Msg("Cloned GPG keys from Git")
	}

	matches, err := filepath.Glob(fmt.Sprintf("%v/*.gpg", fp))
	if err != nil {
		return nil, err
	}
	for _, pubKeyFile := range matches {
		e, err := ReadEntity(pubKeyFile)
		if err != nil {
			log.Warn().
				Str("file", pubKeyFile).
				Msg("Error opening gpg file, skipping")
			continue
		}
		els = append(els, e)
	}
	if len(els) == 0 {
		return nil, errors.New("no gpg keys found")
	}
	return &els, nil
}

// FileInfo is information about a GPGFile
type FileInfo struct {
	FileName         string
	Data             []byte
	OriginalFileInfo os.FileInfo
	IsDirectory      bool
}

// Name is the name of a file
func (gfi FileInfo) Name() string { return gfi.FileName }

// Size is the size of a file in bytes
func (gfi FileInfo) Size() int64 {
	return int64(len(gfi.Data))
}

// Mode returns the file mode
func (gfi FileInfo) Mode() os.FileMode {
	return gfi.OriginalFileInfo.Mode()
}

// ModTime returns the files modification time
func (gfi FileInfo) ModTime() time.Time {
	return gfi.OriginalFileInfo.ModTime()
}

// IsDir returns a boolean representing if the file is also a Dir
func (gfi FileInfo) IsDir() bool {
	return gfi.IsDirectory
}

// Sys is just a placeholder right now
func (gfi FileInfo) Sys() interface{} { return nil }

// NewFileInfo returns a new FileInfo
func NewFileInfo(data []byte, ogi os.FileInfo) (*FileInfo, error) {
	ret := &FileInfo{
		FileName:         fmt.Sprintf("%s.gpg", ogi.Name()),
		Data:             data,
		OriginalFileInfo: ogi,
	}
	return ret, nil
}
