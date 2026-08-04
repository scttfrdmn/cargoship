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
func KeyTypeCompletion(_ *cobra.Command, _ []string, _ string) ([]string, cobra.ShellCompDirective) {
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

var keyTypeMap = map[string]KeyType{
	"rsa":    RSAKeyType,
	"x25519": X25519Type,
	"":       NullKeyType,
}

func (k KeyType) String() string {
	m := reverseMap(keyTypeMap)
	if v, ok := m[k]; ok {
		return v
	}
	// Return empty string for invalid key type instead of panicking
	// Callers should validate key type values before using
	return ""
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

// Sentinel errors returned when a key-generation request would produce
// unprotected key material, or contradicts itself about whether it should.
var (
	// ErrNoPassphrase is returned when neither Passphrase nor AllowUnprotected
	// is set. Generating an unprotected private key is a deliberate choice, not
	// a default.
	ErrNoPassphrase = errors.New("a passphrase is required to protect the private key; set AllowUnprotected to generate an unencrypted key deliberately")

	// ErrConflictingPassphrase is returned when a request both supplies a
	// passphrase and asks for an unprotected key.
	ErrConflictingPassphrase = errors.New("passphrase and AllowUnprotected are mutually exclusive")

	// ErrKeyFileExists is returned rather than overwriting an existing key. A
	// private key is unrecoverable once replaced.
	ErrKeyFileExists = errors.New("key file already exists")
)

// KeyOpts is options for a gpg key
type KeyOpts struct {
	Name    string
	Email   string
	KeyType string
	Bits    int

	// Passphrase encrypts the generated private key at rest. When empty,
	// AllowUnprotected must be set explicitly.
	Passphrase []byte

	// AllowUnprotected permits generating a private key with no passphrase.
	// The resulting file is plaintext key material.
	AllowUnprotected bool
}

// KeyPair represents both the public and private keys
type KeyPair struct {
	Private string
	Public  string
}

// NewKeyPair generates a new gpg private and public key.
//
// The private key is encrypted with opts.Passphrase. Generating an unencrypted
// private key requires opts.AllowUnprotected — omitting the passphrase is not
// enough, because a long-lived plaintext private key should be an explicit
// decision rather than what happens when a field is left blank.
func NewKeyPair(opts *KeyOpts) (*KeyPair, error) {
	if opts.Name == "" {
		return nil, errors.New("name is required")
	}
	if opts.Email == "" {
		return nil, errors.New("email is required")
	}
	if len(opts.Passphrase) > 0 && opts.AllowUnprotected {
		return nil, ErrConflictingPassphrase
	}
	if len(opts.Passphrase) == 0 && !opts.AllowUnprotected {
		return nil, ErrNoPassphrase
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

	// Derive the public key before locking: Lock operates on a copy, and the
	// public half must stay usable by recipients regardless of protection.
	pubKey, err := key.GetArmoredPublicKey()
	if err != nil {
		return nil, err
	}

	if len(opts.Passphrase) > 0 {
		locked, lerr := key.Lock(opts.Passphrase)
		if lerr != nil {
			// Deliberately not wrapped with any value from opts — the error must
			// not carry passphrase material into a log.
			return nil, fmt.Errorf("locking private key: %w", lerr)
		}
		key = locked
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
// and private key file, returning those paths.
//
// An empty dest means the current working directory. It used to mean a fresh
// os.MkdirTemp, which left private key material in $TMPDIR for the lifetime of
// the machine — and on the shared HPC systems CargoShip targets, $TMPDIR is
// often a shared filesystem with a cleanup policy that can also delete the key
// out from under its owner. The working directory is predictable, user-owned,
// and not subject to a reaper.
//
// Existing files are never overwritten: a replaced private key is unrecoverable.
func NewKeyFilesWithPair(kp *KeyPair, dest string) ([]string, error) {
	if dest == "" {
		wd, err := os.Getwd()
		if err != nil {
			return nil, fmt.Errorf("resolving working directory for key output: %w", err)
		}
		dest = wd
	}
	privPath := path.Join(dest, "private.key")
	pubPath := path.Join(dest, "public.key")

	// Check both paths before writing either, so a collision on the second does
	// not leave the first behind.
	for _, p := range []string{privPath, pubPath} {
		if _, err := os.Lstat(p); err == nil {
			return nil, fmt.Errorf("%w: %s", ErrKeyFileExists, p)
		} else if !os.IsNotExist(err) {
			return nil, fmt.Errorf("checking key destination %s: %w", p, err)
		}
	}

	// O_EXCL so a file created between the check above and the write here is an
	// error rather than a silent overwrite.
	if err := writeNew(privPath, kp.Private); err != nil {
		return nil, err
	}
	if err := writeNew(pubPath, kp.Public); err != nil {
		return nil, err
	}
	return []string{privPath, pubPath}, nil
}

// writeNew creates path with owner-only permissions, failing if it already
// exists rather than truncating it.
func writeNew(path, content string) error {
	f, err := os.OpenFile(path, os.O_WRONLY|os.O_CREATE|os.O_EXCL, 0o600) // #nosec G304 -- caller-supplied key destination
	if err != nil {
		if os.IsExist(err) {
			return fmt.Errorf("%w: %s", ErrKeyFileExists, path)
		}
		return fmt.Errorf("creating %s: %w", path, err)
	}
	if _, err := f.WriteString(content); err != nil {
		_ = f.Close()
		return fmt.Errorf("writing %s: %w", path, err)
	}
	return f.Close()
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
