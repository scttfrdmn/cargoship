package cmd

import (
	"path/filepath"
	"strings"
	"testing"

	"github.com/scttfrdmn/cargoship/pkg/restore"
)

// TestJobOutputDir pins #917: the download never resolves a relative directory
// against the current one.
func TestJobOutputDir(t *testing.T) {
	abs := filepath.Join(t.TempDir(), "out")
	if got, err := jobOutputDir(&restore.Job{ID: "j", OutputDir: abs}, ""); err != nil || got != abs {
		t.Errorf("saved absolute dir: got %q, %v", got, err)
	}
	_, err := jobOutputDir(&restore.Job{ID: "j", OutputDir: "out"}, "")
	if err == nil || !strings.Contains(err.Error(), "--output") {
		t.Errorf("a legacy relative dir must be refused with a --output hint: %v", err)
	}
	got, err := jobOutputDir(&restore.Job{ID: "j", OutputDir: "out"}, "here")
	if err != nil || !filepath.IsAbs(got) || filepath.Base(got) != "here" {
		t.Errorf("--output must be made absolute: %q, %v", got, err)
	}
}

func TestJobUploadLocation(t *testing.T) {
	// Saved since #917: the resolved location wins (a --dataset-id restore).
	b, p, u, err := jobUploadLocation(&restore.Job{S3URL: "s3://bkt/team/data", Bucket: "bkt", Prefix: "team/data", UploadID: "v3"})
	if err != nil || b != "bkt" || p != "team/data" || u != "v3" {
		t.Errorf("resolved: %q %q %q %v", b, p, u, err)
	}
	// Legacy job: parsed from the URL.
	b, p, u, err = jobUploadLocation(&restore.Job{S3URL: "s3://bkt/team/data/uploads/abc"})
	if err != nil || b != "bkt" || p != "team/data" || u != "abc" {
		t.Errorf("legacy: %q %q %q %v", b, p, u, err)
	}
	b, p, u, err = jobUploadLocation(&restore.Job{S3URL: "s3://bkt/abc"})
	if err != nil || b != "bkt" || p != "" || u != "abc" {
		t.Errorf("bare id: %q %q %q %v", b, p, u, err)
	}
	if _, _, _, err := jobUploadLocation(&restore.Job{S3URL: "not-a-url"}); err == nil {
		t.Error("an invalid URL must fail")
	}
}
