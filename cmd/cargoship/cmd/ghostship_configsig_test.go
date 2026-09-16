package cmd

import (
	"bytes"
	"os"
	"path/filepath"
	"testing"
)

const minimalValidBox = `id: box-1
version: 7
s3_config:
  bucket: my-bucket
watch_paths:
  - path: /data
`

// runGhostship executes the ghostship command tree with args, returning combined
// stdout+stderr and the error.
func runGhostship(t *testing.T, args ...string) (string, error) {
	t.Helper()
	cmd := NewGhostshipCmd()
	var out bytes.Buffer
	cmd.SetOut(&out)
	cmd.SetErr(&out)
	cmd.SetArgs(args)
	err := cmd.Execute()
	return out.String(), err
}

func TestGhostshipConfigSign_VerifyRoundTrip(t *testing.T) {
	dir := t.TempDir()
	cfgPath := filepath.Join(dir, "box.yaml")
	if err := os.WriteFile(cfgPath, []byte(minimalValidBox), 0o644); err != nil {
		t.Fatal(err)
	}

	// keygen
	if _, err := runGhostship(t, "config-keygen", "--out", dir); err != nil {
		t.Fatalf("config-keygen: %v", err)
	}
	priv := filepath.Join(dir, "config-signing-private.pem")
	pub := filepath.Join(dir, "config-signing-public.pem")
	for _, p := range []string{priv, pub} {
		if _, err := os.Stat(p); err != nil {
			t.Fatalf("expected key file %s: %v", p, err)
		}
	}

	// keygen must not overwrite existing keys
	if _, err := runGhostship(t, "config-keygen", "--out", dir); err == nil {
		t.Fatal("config-keygen should refuse to overwrite existing keys")
	}

	// sign
	if _, err := runGhostship(t, "sign-config", cfgPath, "--key", priv); err != nil {
		t.Fatalf("sign-config: %v", err)
	}
	if _, err := os.Stat(cfgPath + ".sig"); err != nil {
		t.Fatalf("expected signature sidecar: %v", err)
	}

	// validate-config --public-key verifies the good signature
	out, err := runGhostship(t, "validate-config", cfgPath, "--public-key", pub)
	if err != nil {
		t.Fatalf("validate-config with good signature failed: %v\n%s", err, out)
	}
	if !bytes.Contains([]byte(out), []byte("SIGNATURE: ok")) {
		t.Errorf("expected SIGNATURE: ok, got:\n%s", out)
	}
}

func TestGhostshipValidateConfig_TamperedFailsSignature(t *testing.T) {
	dir := t.TempDir()
	cfgPath := filepath.Join(dir, "box.yaml")
	if err := os.WriteFile(cfgPath, []byte(minimalValidBox), 0o644); err != nil {
		t.Fatal(err)
	}
	if _, err := runGhostship(t, "config-keygen", "--out", dir); err != nil {
		t.Fatal(err)
	}
	priv := filepath.Join(dir, "config-signing-private.pem")
	pub := filepath.Join(dir, "config-signing-public.pem")
	if _, err := runGhostship(t, "sign-config", cfgPath, "--key", priv); err != nil {
		t.Fatal(err)
	}

	// Tamper the config after signing (widen scope: add a watch path).
	tampered := minimalValidBox + "  - path: /etc\n"
	if err := os.WriteFile(cfgPath, []byte(tampered), 0o644); err != nil {
		t.Fatal(err)
	}

	_, err := runGhostship(t, "validate-config", cfgPath, "--public-key", pub)
	if err == nil {
		t.Fatal("validate-config should fail signature verification on a tampered config")
	}
}

func TestGhostshipSignConfig_RefusesInvalidConfig(t *testing.T) {
	dir := t.TempDir()
	// Missing id + bucket + watch_paths → validation errors.
	cfgPath := filepath.Join(dir, "bad.yaml")
	if err := os.WriteFile(cfgPath, []byte("name: nope\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	if _, err := runGhostship(t, "config-keygen", "--out", dir); err != nil {
		t.Fatal(err)
	}
	priv := filepath.Join(dir, "config-signing-private.pem")

	_, err := runGhostship(t, "sign-config", cfgPath, "--key", priv)
	if err == nil {
		t.Fatal("sign-config should refuse to sign a config with validation errors")
	}
	if _, statErr := os.Stat(cfgPath + ".sig"); statErr == nil {
		t.Error("no signature should be written for an invalid config")
	}
}
