package gpg

import (
	"errors"
	"io/ioutil"
	"path"

	"github.com/ProtonMail/gopenpgp/v2/crypto"
)

type KeyOpts struct {
	Name       string
	Email      string
	KeyType    string
	Bits       int
	Passphrase []byte
}

type KeyPair struct {
	Private string
	Public  string
}

func NewKeyPair(opts *KeyOpts) (*KeyPair, error) {
	if opts.Name == "" {
		return nil, errors.New("Name is required")
	}
	if opts.Email == "" {
		return nil, errors.New("Email is required")
	}
	if opts.KeyType == "" {
		opts.KeyType = "rsa"
	}
	if opts.Bits == 0 && opts.KeyType == "rsa" {
		opts.Bits = 4096
	}
	key, err := crypto.GenerateKey(opts.Name, opts.Email, opts.KeyType, opts.Bits)
	if err != nil {
		return nil, err
	}
	pubKey, err := key.GetArmoredPublicKey()
	if err != nil {
		return nil, err
	}
	privKey, err := key.Armor()
	if err != nil {
		return nil, err
	}
	kp := &KeyPair{
		Public:  pubKey,
		Private: privKey,
	}
	return kp, nil
}

// Given a keypair object, write the contents to a public and private key file, returning those paths
func NewKeyFilesWithPair(kp *KeyPair, dest string) ([]string, error) {
	var err error
	if dest == "" {
		dest, err = ioutil.TempDir("", "gpg-keys")
		if err != nil {
			return nil, err
		}
	}
	privPath := path.Join(dest, "private.key")
	pubPath := path.Join(dest, "public.key")

	err = ioutil.WriteFile(privPath, []byte(kp.Private), 0o600)
	if err != nil {
		return nil, err
	}
	err = ioutil.WriteFile(pubPath, []byte(kp.Public), 0o644)
	if err != nil {
		return nil, err
	}
	return []string{privPath, pubPath}, nil
}
