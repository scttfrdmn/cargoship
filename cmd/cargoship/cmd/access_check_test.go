package cmd

import (
	"bytes"
	"io"
	"os"
	"strings"
	"testing"

	"github.com/scttfrdmn/cargoship/pkg/aws/access"
)

func TestAccessCheckCmd_Validation(t *testing.T) {
	tests := []struct {
		name string
		args []string
		want string
	}{
		{"missing bucket", []string{}, "bucket is required"},
		{"bad s3 url", []string{"http://nope"}, "invalid S3 URL"},
		{"bad format", []string{"s3://b/p", "--format", "yaml"}, "unsupported --format"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			c := NewAccessCheckCmd()
			c.SetArgs(tt.args)
			c.SilenceUsage = true
			c.SilenceErrors = true
			err := c.Execute()
			if err == nil || !strings.Contains(err.Error(), tt.want) {
				t.Fatalf("want error containing %q, got %v", tt.want, err)
			}
		})
	}
}

func TestPrintAccessReport(t *testing.T) {
	r := &access.Report{
		Bucket:    "my-bucket",
		Prefix:    "archives",
		Region:    "us-west-2",
		Account:   "111122223333",
		CallerARN: "arn:aws:iam::111122223333:role/writer",
		Findings: []access.Finding{
			{Check: "block-public-access", Severity: access.SeverityOK, Summary: "Block Public Access fully enabled"},
			{Check: "bucket-policy", Severity: access.SeverityCritical, Summary: "Wildcard principal", Detail: "statement[0]", Remediation: "scope it"},
			{Check: "kms-key-policy", Severity: access.SeverityUnknown, Summary: "denied", Remediation: "grant kms:GetKeyPolicy"},
		},
	}
	out := captureStdout(t, func() { printAccessReport(r) })
	for _, want := range []string{"my-bucket/archives", "us-west-2", "role/writer", "block-public-access", "scope it", "CRITICAL"} {
		if !strings.Contains(out, want) {
			t.Errorf("report output missing %q\n%s", want, out)
		}
	}
}

func TestSeverityIcon(t *testing.T) {
	for _, s := range []access.Severity{
		access.SeverityOK, access.SeverityInfo, access.SeverityWarn, access.SeverityCritical, access.SeverityUnknown,
	} {
		if severityIcon(s) == "" {
			t.Errorf("severity %q has no icon", s)
		}
	}
}

// captureStdout runs fn and returns everything it wrote to os.Stdout.
func captureStdout(t *testing.T, fn func()) string {
	t.Helper()
	orig := os.Stdout
	rd, wr, err := os.Pipe()
	if err != nil {
		t.Fatalf("pipe: %v", err)
	}
	os.Stdout = wr
	defer func() { os.Stdout = orig }()

	fn()
	_ = wr.Close()
	var buf bytes.Buffer
	_, _ = io.Copy(&buf, rd)
	return buf.String()
}
