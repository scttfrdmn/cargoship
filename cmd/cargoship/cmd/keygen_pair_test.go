package cmd

import (
	"os"
	"path/filepath"
	"testing"
)

// TestConfigKeygen_NoMismatchedPair pins #921: keygen wrote a new private key
// before failing on an existing public key, leaving a mismatched pair.
func TestConfigKeygen_NoMismatchedPair(t *testing.T) {
	dir := t.TempDir()
	pub := filepath.Join(dir, "config-signing-public.pem")
	if err := os.WriteFile(pub, []byte("existing public key"), 0o644); err != nil {
		t.Fatal(err)
	}
	cmd := newGhostshipConfigKeygenCmd()
	cmd.SetArgs([]string{"--out", dir})
	cmd.SilenceErrors, cmd.SilenceUsage = true, true
	if err := cmd.Execute(); err == nil {
		t.Fatal("an existing key must be refused")
	}
	if _, err := os.Stat(filepath.Join(dir, "config-signing-private.pem")); !os.IsNotExist(err) {
		t.Error("no private key may be written when the public key already exists")
	}
	got, _ := os.ReadFile(pub)
	if string(got) != "existing public key" {
		t.Error("the existing public key must be untouched")
	}
}
