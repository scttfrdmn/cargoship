package cmd

import (
	"bytes"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestGhostshipIAMPolicyCmd(t *testing.T) {
	run := func(args ...string) (stdout, stderr string, err error) {
		cmd := NewGhostshipCmd()
		var out, errb bytes.Buffer
		cmd.SetOut(&out)
		cmd.SetErr(&errb)
		cmd.SetArgs(args)
		err = cmd.Execute()
		return out.String(), errb.String(), err
	}

	// Basic writer policy on stdout, scoped and delete-free.
	out, errb, err := run("iam-policy", "s3://b/p", "--writer-id", "w1")
	if err != nil {
		t.Fatalf("execute: %v", err)
	}
	if !strings.Contains(out, "/writers/w1/*") {
		t.Errorf("policy should scope to the writer prefix, got:\n%s", out)
	}
	if strings.Contains(out, "DeleteObject") {
		t.Errorf("policy must be delete-free, got:\n%s", out)
	}
	if strings.Contains(out, "kms:") {
		t.Errorf("no --kms-key-arn should omit kms, got:\n%s", out)
	}
	if !strings.Contains(errb, "delete-free") {
		t.Errorf("stderr should carry the next-step guidance, got:\n%s", errb)
	}

	// With --kms-key-arn the KMS statement appears.
	out, _, err = run("iam-policy", "s3://b/p", "--writer-id", "w1",
		"--kms-key-arn", "arn:aws:kms:us-west-2:111122223333:key/abcd")
	if err != nil {
		t.Fatalf("execute kms: %v", err)
	}
	if !strings.Contains(out, "kms:GenerateDataKey") {
		t.Errorf("kms policy should include GenerateDataKey, got:\n%s", out)
	}

	// Invalid S3 URL is a usage error.
	if _, _, err := run("iam-policy", "nots3://x"); err == nil {
		t.Error("invalid s3 url should error")
	}
}

func TestGhostshipValidateConfigCmd(t *testing.T) {
	dir := t.TempDir()
	write := func(name, body string) string {
		p := filepath.Join(dir, name)
		if err := os.WriteFile(p, []byte(body), 0o644); err != nil {
			t.Fatal(err)
		}
		return p
	}
	run := func(args ...string) (string, error) {
		cmd := NewGhostshipCmd()
		var out bytes.Buffer
		cmd.SetOut(&out)
		cmd.SetErr(&out)
		cmd.SetArgs(args)
		err := cmd.Execute()
		return out.String(), err
	}

	good := write("good.yaml", `id: nas-1
s3_config:
  bucket: my-bucket
  storage_class: STANDARD
watch_paths:
  - path: /data/docs
    include_patterns: ["*.pdf"]
    recursive: true
archival_rules:
  - name: docs
    file_pattern: "*.pdf"
`)
	bad := write("bad.yaml", `id: nas-1
s3_config:
  bucket: ""
watch_paths:
  - path: /data/docs
archival_rules:
  - name: docs
    storage_class: NOPE
`)
	widened := write("widened.yaml", `id: nas-1
s3_config:
  bucket: my-bucket
  storage_class: STANDARD
watch_paths:
  - path: /data/docs
    include_patterns: ["*.pdf"]
    recursive: true
  - path: /data/secrets
archival_rules:
  - name: docs
    file_pattern: "*.pdf"
`)

	// A good config passes.
	if out, err := run("validate-config", good); err != nil || !strings.Contains(out, "OK") {
		t.Errorf("good config: err=%v out=%q", err, out)
	}
	// A bad config fails with an ERROR line.
	if out, err := run("validate-config", bad); err == nil || !strings.Contains(out, "ERROR") {
		t.Errorf("bad config should fail: err=%v out=%q", err, out)
	}
	// --baseline reports a widening but does not fail on its own.
	out, err := run("validate-config", widened, "--baseline", good)
	if err != nil || !strings.Contains(out, "WIDENS-SCOPE") {
		t.Errorf("baseline widening: err=%v out=%q", err, out)
	}
	// --strict turns a widening into a failure.
	if _, err := run("validate-config", widened, "--baseline", good, "--strict"); err == nil {
		t.Error("--strict should fail on a scope-widening")
	}
}
