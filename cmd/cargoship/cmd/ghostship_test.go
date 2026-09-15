package cmd

import (
	"bytes"
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
