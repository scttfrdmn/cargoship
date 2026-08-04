package cmd

import (
	"bytes"
	"os"
	"path/filepath"
	"testing"

	"github.com/ProtonMail/go-crypto/openpgp/packet"
	"github.com/ProtonMail/gopenpgp/v2/crypto"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestCreateKeys(t *testing.T) {
	b := bytes.NewBufferString("")
	cmd := NewRootCmd(b)
	dest := t.TempDir()
	t.Setenv(passphraseEnvVar, "test-passphrase")
	// Just do a small key so the test runs fast 🤷‍♀️
	// --destination is mandatory here: the default is now the working directory,
	// so without it this test would write a private key into the repo.
	cmd.SetArgs([]string{"create", "keys", "--name", "test", "--email", "test@example.com",
		"--bits", "1024", "--destination", dest})
	err := cmd.Execute()
	require.NoError(t, err)
	require.Contains(t, b.String(), "created key files")
}

// TestCreateKeys_PassphraseFromEnvLocksKey is the CLI-level counterpart to the
// pkg/gpg unit test: it asserts the key the *command* writes to disk is
// encrypted, not merely that the library can encrypt one.
func TestCreateKeys_PassphraseFromEnvLocksKey(t *testing.T) {
	b := bytes.NewBufferString("")
	cmd := NewRootCmd(b)
	dest := t.TempDir()
	t.Setenv(passphraseEnvVar, "s3cr3t-passphrase")
	cmd.SetArgs([]string{"create", "keys", "--name", "test", "--email", "test@example.com",
		"--bits", "1024", "--destination", dest})
	require.NoError(t, cmd.Execute())

	armored, err := os.ReadFile(filepath.Join(dest, "private.key"))
	require.NoError(t, err)

	key, err := crypto.NewKeyFromArmored(string(armored))
	require.NoError(t, err)
	locked, err := key.IsLocked()
	require.NoError(t, err)
	assert.True(t, locked, "the private key written to disk must be encrypted")

	// The passphrase must never appear in output — the repo rule is "never log
	// credentials or tokens", and a passphrase echoed into CI logs defeats the
	// whole change.
	assert.NotContains(t, b.String(), "s3cr3t-passphrase")
}

// TestCreateKeys_NonInteractiveWithoutPassphraseFails covers the case that
// matters in CI and in scripts: no terminal to prompt on, and no passphrase
// supplied. Silently generating an unprotected key here is exactly the old
// behaviour, so this must be an error.
func TestCreateKeys_NonInteractiveWithoutPassphraseFails(t *testing.T) {
	b := bytes.NewBufferString("")
	cmd := NewRootCmd(b)
	dest := t.TempDir()
	// t.Setenv first so the original value is restored on cleanup, then unset:
	// this test needs the variable absent, not empty (empty is its own error).
	t.Setenv(passphraseEnvVar, "")
	require.NoError(t, os.Unsetenv(passphraseEnvVar))
	cmd.SetArgs([]string{"create", "keys", "--name", "test", "--email", "test@example.com",
		"--bits", "1024", "--destination", dest})

	err := cmd.Execute()
	require.Error(t, err, "must not generate an unprotected key without an explicit opt-in")

	entries, rerr := os.ReadDir(dest)
	require.NoError(t, rerr)
	assert.Empty(t, entries, "no key material may be written when the command fails")
}

// TestCreateKeys_NoPassphraseOptOut confirms the documented escape hatch works
// and warns about what it produced.
func TestCreateKeys_NoPassphraseOptOut(t *testing.T) {
	b := bytes.NewBufferString("")
	cmd := NewRootCmd(b)
	dest := t.TempDir()
	cmd.SetArgs([]string{"create", "keys", "--name", "test", "--email", "test@example.com",
		"--bits", "1024", "--destination", dest, "--no-passphrase"})
	require.NoError(t, cmd.Execute())

	armored, err := os.ReadFile(filepath.Join(dest, "private.key"))
	require.NoError(t, err)
	key, err := crypto.NewKeyFromArmored(string(armored))
	require.NoError(t, err)
	locked, err := key.IsLocked()
	require.NoError(t, err)
	assert.False(t, locked, "--no-passphrase must produce exactly what it says")
	assert.Contains(t, b.String(), "NOT encrypted", "the unprotected case must warn")
}

// TestCreateKeys_EmptyEnvPassphraseIsRejected guards a fail-open path: an empty
// CARGOSHIP_GPG_PASSPHRASE (a common shell accident, e.g. a typo'd variable
// name) must not quietly degrade to an unprotected key.
func TestCreateKeys_EmptyEnvPassphraseIsRejected(t *testing.T) {
	b := bytes.NewBufferString("")
	cmd := NewRootCmd(b)
	dest := t.TempDir()
	t.Setenv(passphraseEnvVar, "")
	cmd.SetArgs([]string{"create", "keys", "--name", "test", "--email", "test@example.com",
		"--bits", "1024", "--destination", dest})

	require.Error(t, cmd.Execute())
	entries, err := os.ReadDir(dest)
	require.NoError(t, err)
	assert.Empty(t, entries)
}

// TestCreateKeys_RefusesToClobberExistingKey pins the destructive case. The
// default destination is now a predictable directory, so a second run in the
// same place would otherwise silently replace an unrecoverable private key.
func TestCreateKeys_RefusesToClobberExistingKey(t *testing.T) {
	dest := t.TempDir()
	t.Setenv(passphraseEnvVar, "test-passphrase")

	first := bytes.NewBufferString("")
	cmd := NewRootCmd(first)
	cmd.SetArgs([]string{"create", "keys", "--name", "test", "--email", "test@example.com",
		"--bits", "1024", "--destination", dest})
	require.NoError(t, cmd.Execute())

	original, err := os.ReadFile(filepath.Join(dest, "private.key"))
	require.NoError(t, err)

	second := bytes.NewBufferString("")
	cmd2 := NewRootCmd(second)
	cmd2.SetArgs([]string{"create", "keys", "--name", "other", "--email", "other@example.com",
		"--bits", "1024", "--destination", dest})
	require.Error(t, cmd2.Execute(), "a second run must not overwrite the first key")

	after, err := os.ReadFile(filepath.Join(dest, "private.key"))
	require.NoError(t, err)
	assert.Equal(t, original, after, "the original private key must survive")
}

// TestCreateKeys_KeyTypeFlagIsHonoured covers a bug found while fixing the
// others: --type was registered and completion-enabled, but never copied into
// KeyOpts, so `--type x25519` silently produced an RSA key. A flag that is
// accepted and ignored is worse than one that is rejected.
func TestCreateKeys_KeyTypeFlagIsHonoured(t *testing.T) {
	b := bytes.NewBufferString("")
	cmd := NewRootCmd(b)
	dest := t.TempDir()
	t.Setenv(passphraseEnvVar, "test-passphrase")
	cmd.SetArgs([]string{"create", "keys", "--name", "test", "--email", "test@example.com",
		"--type", "x25519", "--destination", dest})
	require.NoError(t, cmd.Execute())

	armored, err := os.ReadFile(filepath.Join(dest, "public.key"))
	require.NoError(t, err)
	key, err := crypto.NewKeyFromArmored(string(armored))
	require.NoError(t, err)

	// The primary key algorithm is the unambiguous signal: gopenpgp's "x25519"
	// produces an EdDSA/Ed25519 primary, never PubKeyAlgoRSA.
	assert.NotEqual(t, packet.PubKeyAlgoRSA, key.GetEntity().PrimaryKey.PubKeyAlgo,
		"--type x25519 must not silently produce an RSA key")
}
