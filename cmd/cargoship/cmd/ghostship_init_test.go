package cmd

import (
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/scttfrdmn/cargoship/pkg/launch"
	"gopkg.in/yaml.v3"
)

func TestGhostshipInit_GeneratesBundle(t *testing.T) {
	keyDir := t.TempDir()
	if _, err := runGhostship(t, "config-keygen", "--out", keyDir); err != nil {
		t.Fatal(err)
	}
	pub := filepath.Join(keyDir, "config-signing-public.pem")
	out := filepath.Join(t.TempDir(), "bundle")

	if _, err := runGhostship(t, "init", "s3://backups/nas",
		"--writer-id", "lab-nas-1", "--kms-key-arn", "arn:aws:kms:us-west-2:111122223333:key/abcd",
		"--public-key", pub, "--out", out); err != nil {
		t.Fatalf("init: %v", err)
	}

	for _, name := range []string{"iam-policy.json", "config.yaml", "config-signing-public.pem", "compose.yaml", "README.md"} {
		if _, err := os.Stat(filepath.Join(out, name)); err != nil {
			t.Fatalf("expected bundle file %s: %v", name, err)
		}
	}

	// The emitted policy is write-only: WriterObjectAccess is PutObject-only, and there
	// is no DeleteObject or kms:Decrypt anywhere.
	policyBytes, _ := os.ReadFile(filepath.Join(out, "iam-policy.json"))
	s := string(policyBytes)
	for _, forbidden := range []string{"s3:DeleteObject", "kms:Decrypt", "s3:*", "kms:*"} {
		if strings.Contains(s, forbidden) {
			t.Errorf("write-only bundle policy must not contain %q:\n%s", forbidden, s)
		}
	}
	var doc struct {
		Statement []struct {
			Sid    string
			Action []string
		}
	}
	if err := json.Unmarshal(policyBytes, &doc); err != nil {
		t.Fatalf("iam-policy.json is not valid JSON: %v", err)
	}
	for _, st := range doc.Statement {
		if st.Sid == "WriterObjectAccess" {
			if len(st.Action) != 1 || st.Action[0] != "s3:PutObject" {
				t.Errorf("write-only object actions = %v, want [s3:PutObject]", st.Action)
			}
		}
	}

	// The baked public key matches the input.
	baked, _ := os.ReadFile(filepath.Join(out, "config-signing-public.pem"))
	orig, _ := os.ReadFile(pub)
	if string(baked) != string(orig) {
		t.Error("baked public key does not match the input key")
	}

	// The config skeleton is structurally valid (parses; passes ValidateConfig).
	cfgBytes, _ := os.ReadFile(filepath.Join(out, "config.yaml"))
	var cfg launch.GhostShipConfig
	if err := yaml.Unmarshal(cfgBytes, &cfg); err != nil {
		t.Fatalf("config.yaml does not parse: %v", err)
	}
	for _, is := range launch.ValidateConfig(&cfg) {
		if is.Severity == launch.SeverityError {
			t.Errorf("config skeleton has a validation error: %s: %s", is.Field, is.Message)
		}
	}
	if cfg.WriterID != "lab-nas-1" || cfg.Version != 1 {
		t.Errorf("config skeleton = %+v, want writer_id lab-nas-1 version 1", cfg)
	}

	// Compose wires the writer + config-url and mounts credentials as a file (no env).
	compose := mustRead(t, filepath.Join(out, "compose.yaml"))
	for _, want := range []string{"lab-nas-1", "--config-url", "s3://backups/nas", "/root/.aws/credentials"} {
		if !strings.Contains(compose, want) {
			t.Errorf("compose.yaml missing %q", want)
		}
	}
	if strings.Contains(compose, "AWS_SECRET_ACCESS_KEY") || strings.Contains(compose, "environment:") {
		t.Errorf("compose.yaml must not pass credentials via environment:\n%s", compose)
	}
}

func TestGhostshipInit_RequiresPublicKey(t *testing.T) {
	_, err := runGhostship(t, "init", "s3://b/p", "--writer-id", "w1", "--out", t.TempDir())
	if err == nil {
		t.Fatal("init should require --public-key")
	}
}

func TestGhostshipInit_NoClobber(t *testing.T) {
	keyDir := t.TempDir()
	if _, err := runGhostship(t, "config-keygen", "--out", keyDir); err != nil {
		t.Fatal(err)
	}
	pub := filepath.Join(keyDir, "config-signing-public.pem")
	out := filepath.Join(t.TempDir(), "bundle")
	if _, err := runGhostship(t, "init", "s3://b/nas", "--writer-id", "w1", "--public-key", pub, "--out", out); err != nil {
		t.Fatalf("first init: %v", err)
	}
	if _, err := runGhostship(t, "init", "s3://b/nas", "--writer-id", "w1", "--public-key", pub, "--out", out); err == nil {
		t.Fatal("second init into the same dir should refuse to overwrite")
	}
}

func mustRead(t *testing.T, path string) string {
	t.Helper()
	b, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("read %s: %v", path, err)
	}
	return string(b)
}
