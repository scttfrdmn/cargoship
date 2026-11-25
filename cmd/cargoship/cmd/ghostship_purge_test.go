package cmd

import (
	"strings"
	"testing"
)

// TestFleetPurgePrefixes pins #920 part 1: a purge is scoped to the fleet base.
// It used to delete writers/<id>/ at the bucket root, shared by every fleet in
// the bucket with a writer of that name.
func TestFleetPurgePrefixes(t *testing.T) {
	got := fleetPurgePrefixes("fleets/a", "nas-1")
	want := []string{"fleets/a/writers/nas-1", "fleets/a/fleet/nas-1"}
	if strings.Join(got, ",") != strings.Join(want, ",") {
		t.Errorf("prefixes = %v, want %v", got, want)
	}
	for _, p := range got {
		if strings.HasPrefix(p, "writers/") {
			t.Errorf("%s is the bucket-root path shared across fleets", p)
		}
	}
}

func TestGhostshipScuttle_PurgeNeedsFleetBase(t *testing.T) {
	for _, url := range []string{"s3://bucket", "s3://bucket/", "s3://bucket//"} {
		cmd := newGhostshipScuttleCmd()
		cmd.SetArgs([]string{"nas-1", "--purge-data", url, "--yes"})
		cmd.SilenceErrors, cmd.SilenceUsage = true, true
		err := cmd.Execute()
		if err == nil || !strings.Contains(err.Error(), "fleet base") {
			t.Errorf("%s: err = %v, want a refusal asking for the fleet base", url, err)
		}
	}
}
