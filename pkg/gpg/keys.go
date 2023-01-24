package gpg

import (
	"errors"
	"os"
	"path"

	"github.com/ProtonMail/gopenpgp/v2/crypto"
)

// KeyOpts is options for a gpg key
type KeyOpts struct {
	Name       string
	Email      string
	KeyType    string
	Bits       int
	Passphrase []byte
}

// KeyPair represents both the public and private keys
type KeyPair struct {
	Private string
	Public  string
}

// NewKeyPair generates a new gpg private and public key
func NewKeyPair(opts *KeyOpts) (*KeyPair, error) {
	if opts.Name == "" {
		return nil, errors.New("name is required")
	}
	if opts.Email == "" {
		return nil, errors.New("email is required")
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

// NewKeyFilesWithPair Given a keypair object, write the contents to a public
// and private key file, returning those paths
func NewKeyFilesWithPair(kp *KeyPair, dest string) ([]string, error) {
	var err error
	if dest == "" {
		dest, err = os.MkdirTemp("", "gpg-keys")
		if err != nil {
			return nil, err
		}
	}
	privPath := path.Join(dest, "private.key")
	pubPath := path.Join(dest, "public.key")

	err = os.WriteFile(privPath, []byte(kp.Private), 0o600)
	if err != nil {
		return nil, err
	}
	err = os.WriteFile(pubPath, []byte(kp.Public), 0o600)
	if err != nil {
		return nil, err
	}
	return []string{privPath, pubPath}, nil
}
