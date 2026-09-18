package cmd

import (
	"strings"
	"testing"
)

func TestGhostshipScuttle_PurgeDataRequiresYes(t *testing.T) {
	// --purge-data without --yes must fail before any AWS call.
	_, err := runGhostship(t, "scuttle", "lab-nas-1", "--purge-data", "s3://b/nas")
	if err == nil {
		t.Fatal("expected --purge-data without --yes to error")
	}
	if !strings.Contains(err.Error(), "--yes") {
		t.Errorf("error = %q, want it to require --yes", err)
	}
}

func TestGhostshipScuttle_InvalidWriterID(t *testing.T) {
	// A path-separator writer id is rejected before any AWS call.
	_, err := runGhostship(t, "scuttle", "bad/id")
	if err == nil {
		t.Fatal("expected an invalid writer id to error")
	}
	if !strings.Contains(err.Error(), "invalid writer id") {
		t.Errorf("error = %q, want an invalid-writer-id error", err)
	}
}

func TestGhostship_ScuttleRegistered(t *testing.T) {
	cmd := NewGhostshipCmd()
	var found bool
	for _, c := range cmd.Commands() {
		if c.Name() == "scuttle" {
			found = true
		}
	}
	if !found {
		t.Error("ghostship scuttle subcommand should be registered")
	}
}
