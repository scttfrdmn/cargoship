package gpg

import (
	"errors"
	"fmt"
	"os"
	"path"
	"sort"

	"github.com/ProtonMail/gopenpgp/v2/crypto"
	"github.com/spf13/cobra"
)

// KeyType represents the different types of GPG keys supported
type KeyType int

// KeyTypeCompletion returns shell completion
func KeyTypeCompletion(cmd *cobra.Command, args []string, toComplete string) ([]string, cobra.ShellCompDirective) {
	return nonEmptyKeys(keyTypeMap), cobra.ShellCompDirectiveNoFileComp
}

const (
	// NullKeyType is the unset value for this type
	NullKeyType KeyType = iota
	// RSAKeyType represents and RSA key. This is the most commonly used option
	RSAKeyType
	// X25519Type is an elliptic curve Diffie-Hellman key exchange using
	// Curve25519. It allows two parties to jointly agree on a shared secret
	// using an insecure channel.
	X25519Type
)

var keyTypeMap map[string]KeyType = map[string]KeyType{
	"rsa":    RSAKeyType,
	"x25519": X25519Type,
	"":       NullKeyType,
}

func (k KeyType) String() string {
	m := reverseMap(keyTypeMap)
	if v, ok := m[k]; ok {
		return v
	}
	panic("invalid KeyType")
}

// Type satisfies part of the pflags.Value interface
func (k KeyType) Type() string {
	return "KeyType"
}

// Set helps fulfill the pflag.Value interface
func (k *KeyType) Set(v string) error {
	if v, ok := keyTypeMap[v]; ok {
		*k = v
		return nil
	}
	return fmt.Errorf("ProductionLevel should be one of: %v", nonEmptyKeys(keyTypeMap))
}

// MarshalJSON ensures that json conversions use the string value here, not the int value
func (k *KeyType) MarshalJSON() ([]byte, error) {
	return []byte(fmt.Sprintf("\"%v\"", k.String())), nil
}

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

// reverseMap takes a map[k]v and returns a map[v]k
func reverseMap[K string, V string | KeyType](m map[K]V) map[V]K {
	ret := make(map[V]K, len(m))
	for k, v := range m {
		ret[v] = k
	}
	return ret
}

// nonEmptyKeys returns the non-empty keys of a map in an array
func nonEmptyKeys[V any](m map[string]V) []string {
	var ret []string
	for k := range m {
		if k != "" {
			ret = append(ret, k)
		}
	}
	sort.Strings(ret)
	return ret
}
