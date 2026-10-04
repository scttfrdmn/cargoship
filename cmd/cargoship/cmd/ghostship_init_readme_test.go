package cmd

import (
	"encoding/json"
	"strings"
	"testing"
)

// #715: the bundle must hand the operator the lifecycle rule as an ARTIFACT, not
// just prose. `fleet lock-status` already warns when the rule is absent, but its
// remediation was "add a lifecycle rule" with nothing to apply.
//
// The rule matters because a cancelled cycle leaves incomplete multipart uploads
// that are billed indefinitely, are invisible to `aws s3 ls`, and SURVIVE
// `aws s3 rm --recursive` — so a purged prefix can read as 0 objects while still
// accruing cost. Two were found that way on a real deployment.
func TestInitBucketLifecycleJSON(t *testing.T) {
	var cfg struct {
		Rules []struct {
			ID                             string `json:"ID"`
			Status                         string `json:"Status"`
			AbortIncompleteMultipartUpload *struct {
				DaysAfterInitiation int `json:"DaysAfterInitiation"`
			} `json:"AbortIncompleteMultipartUpload"`
		} `json:"Rules"`
	}
	if err := json.Unmarshal([]byte(initBucketLifecycleJSON()), &cfg); err != nil {
		t.Fatalf("emitted lifecycle policy is not valid JSON (S3 would reject it): %v", err)
	}
	if len(cfg.Rules) != 1 {
		t.Fatalf("want exactly 1 rule, got %d", len(cfg.Rules))
	}
	r := cfg.Rules[0]
	if r.AbortIncompleteMultipartUpload == nil {
		t.Fatal("the rule must carry AbortIncompleteMultipartUpload, or it does not address #715")
	}
	if r.Status != "Enabled" {
		t.Errorf("Status = %q, want Enabled (a disabled rule aborts nothing)", r.Status)
	}
	if d := r.AbortIncompleteMultipartUpload.DaysAfterInitiation; d <= 0 {
		t.Errorf("DaysAfterInitiation = %d, want positive", d)
	}
	// This is what fleet lock-status looks for, so the names must line up.
	if !strings.Contains(r.ID, "multipart") {
		t.Errorf("rule ID %q should name what it does", r.ID)
	}
}

// The README is a format string with nine verbs; `go vet` catches a wrong COUNT but
// not a wrong ORDER, and a bundle that tells the operator to run a command against
// the wrong bucket is worse than no instruction.
func TestInitReadmeRendersBucketInLifecycleStep(t *testing.T) {
	out := initReadme("lab-nas-1", "my-bucket", "nas/base")
	if strings.Contains(out, "%!") || strings.Contains(out, "(MISSING)") {
		t.Fatalf("format verb mismatch in README:\n%s", out)
	}
	for _, want := range []string{
		"--bucket my-bucket --lifecycle-configuration file://bucket-lifecycle.json",
		"s3://my-bucket/nas/base/fleet/lab-nas-1/config.yaml",
		"writers/lab-nas-1/",
		"bucket-lifecycle.json` — bucket rule",
		"fleet lock-status",
	} {
		if !strings.Contains(out, want) {
			t.Errorf("README missing %q", want)
		}
	}
}
