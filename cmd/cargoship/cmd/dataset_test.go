package cmd

import (
	"context"
	"testing"
	"time"

	"github.com/scttfrdmn/cargoship/pkg/manifest"
)

func day(s string) time.Time {
	t, _ := time.Parse("2006-01-02", s)
	return t
}

// three versions of dataset "ds" plus an unrelated dataset.
func versionFixtures() []*manifest.Manifest {
	return []*manifest.Manifest{
		{UploadID: "u1", DatasetID: "ds", VersionOrdinal: 1, CreatedAt: day("2026-06-01")},
		{UploadID: "u2", DatasetID: "ds", VersionOrdinal: 2, CreatedAt: day("2026-07-01")},
		{UploadID: "u3", DatasetID: "ds", VersionOrdinal: 3, CreatedAt: day("2026-08-01")},
		{UploadID: "x1", DatasetID: "other", VersionOrdinal: 1, CreatedAt: day("2026-06-15")},
	}
}

func TestSelectDatasetVersion(t *testing.T) {
	all := versionFixtures()
	ctx := context.Background()

	// Exact ordinal.
	if m, err := selectDatasetVersion(ctx, all, nil, "ds", 2, time.Time{}); err != nil || m.UploadID != "u2" {
		t.Errorf("version 2: got %v err %v, want u2", m, err)
	}
	// HEAD (version 0, no asOf) → highest ordinal.
	if m, err := selectDatasetVersion(ctx, all, nil, "ds", 0, time.Time{}); err != nil || m.UploadID != "u3" {
		t.Errorf("HEAD: got %v err %v, want u3", m, err)
	}
	// as-of: newest at or before 2026-07-15 → u2.
	if m, err := selectDatasetVersion(ctx, all, nil, "ds", 0, day("2026-07-15")); err != nil || m.UploadID != "u2" {
		t.Errorf("as-of 07-15: got %v err %v, want u2", m, err)
	}
	// Unknown version and unknown dataset are errors.
	if _, err := selectDatasetVersion(ctx, all, nil, "ds", 9, time.Time{}); err == nil {
		t.Error("version 9 should error")
	}
	if _, err := selectDatasetVersion(ctx, all, nil, "nope", 0, time.Time{}); err == nil {
		t.Error("unknown dataset should error")
	}
	// as-of before any version → error.
	if _, err := selectDatasetVersion(ctx, all, nil, "ds", 0, day("2026-01-01")); err == nil {
		t.Error("as-of before all versions should error")
	}
}

func TestResolveSelector(t *testing.T) {
	all := versionFixtures()
	ctx := context.Background()

	cases := map[string]string{"": "u3", "head": "u3", "HEAD": "u3", "2": "u2", "v2": "u2", "2026-07-15": "u2"}
	for sel, want := range cases {
		m, err := resolveSelector(ctx, all, nil, "ds", sel)
		if err != nil || m.UploadID != want {
			t.Errorf("selector %q: got %v err %v, want %s", sel, m, err, want)
		}
	}
	if _, err := resolveSelector(ctx, all, nil, "ds", "garbage"); err == nil {
		t.Error("garbage selector should error")
	}
}
