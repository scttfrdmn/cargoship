package cmd

import (
	"bytes"
	"strings"
	"testing"
	"time"

	"github.com/scttfrdmn/cargoship/pkg/fleet"
)

func TestFleetStatus_BadURL(t *testing.T) {
	cmd := NewFleetCmd()
	cmd.SetArgs([]string{"status", "not-an-s3-url"})
	cmd.SetOut(&bytes.Buffer{})
	cmd.SetErr(&bytes.Buffer{})
	err := cmd.Execute()
	if err == nil {
		t.Fatal("expected an error for a non-s3 URL, got nil")
	}
	if !strings.Contains(err.Error(), "invalid S3 target") {
		t.Errorf("error = %q, want it to mention the invalid S3 target", err)
	}
}

func TestFleetMonitor_BadURL(t *testing.T) {
	cmd := NewFleetCmd()
	cmd.SetArgs([]string{"monitor", "not-an-s3-url"})
	cmd.SetOut(&bytes.Buffer{})
	cmd.SetErr(&bytes.Buffer{})
	err := cmd.Execute()
	if err == nil {
		t.Fatal("expected an error for a non-s3 URL, got nil")
	}
	if !strings.Contains(err.Error(), "invalid S3 target") {
		t.Errorf("error = %q, want it to mention the invalid S3 target", err)
	}
}

func TestFleetLockStatus_BadURL(t *testing.T) {
	cmd := NewFleetCmd()
	cmd.SetArgs([]string{"lock-status", "not-an-s3-url"})
	cmd.SetOut(&bytes.Buffer{})
	cmd.SetErr(&bytes.Buffer{})
	err := cmd.Execute()
	if err == nil {
		t.Fatal("expected an error for a non-s3 URL, got nil")
	}
	if !strings.Contains(err.Error(), "invalid S3 target") {
		t.Errorf("error = %q, want it to mention the invalid S3 target", err)
	}
}

func TestRenderLockStatus(t *testing.T) {
	report := fleet.ImmutabilityReport{
		Bucket: "b",
		Findings: []fleet.ImmutabilityFinding{
			{Check: "versioning", Severity: fleet.SevOK, Summary: "Enabled"},
			{Check: "object-lock", Severity: fleet.SevCritical, Summary: "not configured",
				Detail: "a stolen delete credential can destroy history", Remediation: "enable Object Lock"},
			{Check: "lifecycle-mpu", Severity: fleet.SevWarn, Summary: "no AbortIncompleteMultipartUpload rule",
				Remediation: "add an AbortIncompleteMultipartUpload rule"},
		},
	}
	cmd := NewFleetCmd()
	var out bytes.Buffer
	cmd.SetOut(&out)
	renderLockStatus(cmd, report)
	s := out.String()
	for _, want := range []string{
		"CHECK", "STATUS", "DETAIL", "versioning", "OK",
		"object-lock", "CRITICAL", "lifecycle-mpu", "WARN",
		"Overall: CRITICAL", "Remediation:", "enable Object Lock",
	} {
		if !strings.Contains(s, want) {
			t.Errorf("lock-status output missing %q:\n%s", want, s)
		}
	}
	// An OK finding must NOT appear in the remediation block.
	if idx := strings.Index(s, "Remediation:"); idx >= 0 && strings.Contains(s[idx:], "versioning") {
		t.Errorf("OK finding should not be listed under Remediation:\n%s", s)
	}
}

func TestRenderFleetTable_Empty(t *testing.T) {
	cmd := NewFleetCmd()
	var out bytes.Buffer
	cmd.SetOut(&out)
	if err := renderFleetTable(cmd, "s3://b/p", nil, time.Now()); err != nil {
		t.Fatalf("renderFleetTable: %v", err)
	}
	if !strings.Contains(out.String(), "No writers have reported") {
		t.Errorf("empty output = %q", out.String())
	}
}

func TestRenderFleetTable_Rows(t *testing.T) {
	now := time.Date(2026, 9, 15, 12, 0, 0, 0, time.UTC)
	statuses := []fleet.WriterStatus{
		{
			WriterID:  "lab-nas-1",
			Hostname:  "nas.local",
			UpdatedAt: now.Add(-90 * time.Second),
			Sources:   []fleet.SourceStatus{{Path: "/data", OK: true}},
		},
		{
			WriterID:  "lab-nas-2",
			Hostname:  "nas2.local",
			UpdatedAt: now.Add(-3 * time.Hour),
			Sources:   []fleet.SourceStatus{{Path: "/data", OK: false, LastError: "disk gone"}},
		},
	}
	cmd := NewFleetCmd()
	var out bytes.Buffer
	cmd.SetOut(&out)
	if err := renderFleetTable(cmd, "s3://b/p", statuses, now); err != nil {
		t.Fatalf("renderFleetTable: %v", err)
	}
	s := out.String()
	for _, want := range []string{"lab-nas-1", "1m", "yes", "lab-nas-2", "3h", "NO", "disk gone"} {
		if !strings.Contains(s, want) {
			t.Errorf("table missing %q:\n%s", want, s)
		}
	}
}

func TestHumanizeAge(t *testing.T) {
	cases := []struct {
		d    time.Duration
		want string
	}{
		{30 * time.Second, "30s"},
		{90 * time.Second, "1m"},
		{3 * time.Hour, "3h"},
		{50 * time.Hour, "2d"},
		{-5 * time.Second, "0s"},
	}
	for _, c := range cases {
		if got := humanizeAge(c.d); got != c.want {
			t.Errorf("humanizeAge(%v) = %q, want %q", c.d, got, c.want)
		}
	}
}
