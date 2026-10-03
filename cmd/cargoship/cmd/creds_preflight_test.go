package cmd

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// The whole point of the preflight is to name an unreadable credentials file instead of
// letting the SDK time out against EC2 IMDS (#692), so the cases that matter are
// "unreadable" vs "every other state", and the message has to be actionable.
func TestCredentialsPreflight(t *testing.T) {
	if os.Getuid() == 0 {
		t.Skip("running as root: mode 0000 is still readable, so unreadable cannot be simulated")
	}

	t.Run("unreadable file is reported with the fix", func(t *testing.T) {
		dir := t.TempDir()
		p := filepath.Join(dir, "credentials")
		if err := os.WriteFile(p, []byte("[default]\n"), 0o600); err != nil {
			t.Fatalf("write: %v", err)
		}
		if err := os.Chmod(p, 0o000); err != nil {
			t.Fatalf("chmod: %v", err)
		}
		t.Setenv("AWS_SHARED_CREDENTIALS_FILE", p)

		err := credentialsPreflight()
		if err == nil {
			t.Fatal("an unreadable credentials file must be reported; otherwise the SDK fails on IMDS instead")
		}
		// The message has to carry the path and the remedy, or it is no better than the
		// IMDS error it replaces.
		for _, want := range []string{p, "not readable", "chown"} {
			if !strings.Contains(err.Error(), want) {
				t.Errorf("error %q does not mention %q", err, want)
			}
		}
	})

	t.Run("readable file passes", func(t *testing.T) {
		dir := t.TempDir()
		p := filepath.Join(dir, "credentials")
		if err := os.WriteFile(p, []byte("[default]\n"), 0o600); err != nil {
			t.Fatalf("write: %v", err)
		}
		t.Setenv("AWS_SHARED_CREDENTIALS_FILE", p)
		if err := credentialsPreflight(); err != nil {
			t.Errorf("a readable file must pass, got %v", err)
		}
	})

	// A missing file is a legitimate configuration — env vars, an instance role, a mounted
	// web-identity token. Reporting it would break every deployment that does not use a
	// credentials file at all.
	t.Run("missing file is not an error", func(t *testing.T) {
		t.Setenv("AWS_SHARED_CREDENTIALS_FILE", filepath.Join(t.TempDir(), "nope"))
		if err := credentialsPreflight(); err != nil {
			t.Errorf("a missing credentials file must not fail: %v", err)
		}
	})

	// `docker run -v /host/missing:/home/cargoship/.aws/credentials` silently creates a
	// DIRECTORY at that path. The SDK's error for that is no clearer than the IMDS one.
	t.Run("a directory at the credentials path is reported", func(t *testing.T) {
		dir := t.TempDir()
		p := filepath.Join(dir, "credentials")
		if err := os.Mkdir(p, 0o755); err != nil {
			t.Fatalf("mkdir: %v", err)
		}
		t.Setenv("AWS_SHARED_CREDENTIALS_FILE", p)
		err := credentialsPreflight()
		if err == nil || !strings.Contains(err.Error(), "directory") {
			t.Errorf("a directory at the credentials path should be reported, got %v", err)
		}
	})
}
