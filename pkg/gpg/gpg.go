package gpg

import (
	"bytes"
	"errors"
	"fmt"
	"io"
	"io/ioutil"
	"os"
	"path"
	"path/filepath"
	"time"

	"github.com/apex/log"
	"github.com/go-git/go-git/v5"
	"github.com/spf13/cobra"

	"github.com/ProtonMail/go-crypto/openpgp"
	"github.com/ProtonMail/go-crypto/openpgp/armor"
	"github.com/ProtonMail/go-crypto/openpgp/packet"
)

func EncryptToWithCmd(cmd *cobra.Command) (*openpgp.EntityList, error) {
	pubKeyFiles, err := cmd.Flags().GetStringArray("public-key")
	if err != nil {
		return nil, err
	}

	excludeSystems, err := cmd.Flags().GetBool("exclude-systems-pubkeys")
	if err != nil {
		return nil, err
	}
	var encryptTo *openpgp.EntityList
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
	var buffer *bytes.Buffer = &bytes.Buffer{}
	var armoredWriter io.WriteCloser
	var cipheredWriter io.WriteCloser
	var err error

	// Create an openpgp armored cipher writer pointing on our
	// buffer
	if useArmor {
		armoredWriter, err = armor.Encode(buffer, "PGP MESSAGE", nil)
		if err != nil {
			return nil, errors.New("Bad Writer")
		}
		cipheredWriter, err = openpgp.Encrypt(armoredWriter, *encryptionKeys, nil, nil, nil)
		if err != nil {
			return nil, errors.New("Bad Cipher")
		}
	} else {
		cipheredWriter, err = openpgp.Encrypt(buffer, *encryptionKeys, nil, nil, nil)
		if err != nil {
			return nil, errors.New("Bad Cipher")
		}
	}

	// Create an encrypted writer using the provided encryption keys

	// Write (encrypts on the fly) the provided bytes to
	// cipheredWriter
	_, err = cipheredWriter.Write(d)
	if err != nil {
		return nil, errors.New("Bad Ciphered Writer")
	}

	cipheredWriter.Close()
	if useArmor {
		armoredWriter.Close()
	}

	return buffer.Bytes(), nil
}

func ReadEntity(name string) (*openpgp.Entity, error) {
	f, err := os.Open(name)
	if err != nil {
		return nil, err
	}
	defer f.Close()
	block, err := armor.Decode(f)
	if err != nil {
		return nil, err
	}
	return openpgp.ReadEntity(packet.NewReader(block.Body))
}

func CollectGPGPubKeys(fp string) (*openpgp.EntityList, error) {
	var els openpgp.EntityList

	if fp == "" {
		gitlabKeysUrl := "https://gitlab.oit.duke.edu/oit-ssi-systems/staff-public-keys.git"
		subDir := "linux"
		tmpdir, err := ioutil.TempDir("", "gpg-pub-tmpdir")
		defer os.RemoveAll(tmpdir)
		if err != nil {
			return nil, err
		}
		_, err = git.PlainClone(tmpdir, false, &git.CloneOptions{
			URL:               gitlabKeysUrl,
			RecurseSubmodules: git.DefaultSubmoduleRecursionDepth,
		})
		if err != nil {
			return nil, err
		}
		fp = path.Join(tmpdir, subDir)
		log.WithFields(log.Fields{
			"url":    gitlabKeysUrl,
			"subdir": subDir,
		}).Info("Pulling in pubkeys")
	}

	matches, err := filepath.Glob(fmt.Sprintf("%v/*.gpg", fp))
	if err != nil {
		return nil, err
	}
	for _, pubKeyFile := range matches {
		e, err := ReadEntity(pubKeyFile)
		if err != nil {
			log.Warnf("Error opening gpg file: %v, skipping", pubKeyFile)
			continue
		}
		els = append(els, e)
	}
	if len(els) == 0 {
		return nil, errors.New("No gpg keys found")
	}
	return &els, nil
}

type GPGFileInfo struct {
	FileName         string
	Data             []byte
	OriginalFileInfo os.FileInfo
	IsDirectory      bool
}

func (gfi GPGFileInfo) Name() string { return gfi.FileName }
func (gfi GPGFileInfo) Size() int64 {
	return int64(len(gfi.Data))
}

func (gfi GPGFileInfo) Mode() os.FileMode {
	return gfi.OriginalFileInfo.Mode()
}

func (gfi GPGFileInfo) ModTime() time.Time {
	return gfi.OriginalFileInfo.ModTime()
}

func (gfi GPGFileInfo) IsDir() bool {
	return gfi.IsDirectory
}
func (gfi GPGFileInfo) Sys() interface{} { return nil }

func NewGPGFileInfo(data []byte, ogi os.FileInfo) (*GPGFileInfo, error) {
	ret := &GPGFileInfo{
		FileName:         fmt.Sprintf("%s.gpg", ogi.Name()),
		Data:             data,
		OriginalFileInfo: ogi,
	}
	return ret, nil
}
