package cmd

import (
	"bytes"
	"strings"
	"testing"

	"github.com/scttfrdmn/cargoship/pkg/manifest"
)

func TestAllFilePaths(t *testing.T) {
	m := &manifest.Manifest{Files: []manifest.FileEntry{
		{Path: "/src/a.txt"},
		{Path: "/src/docs/b.md"},
	}}
	got := allFilePaths(m)
	if len(got) != 2 {
		t.Fatalf("want 2 paths, got %d: %v", len(got), got)
	}
	set := map[string]bool{}
	for _, p := range got {
		set[p] = true
	}
	if !set["/src/a.txt"] || !set["/src/docs/b.md"] {
		t.Errorf("missing expected paths: %v", got)
	}
	if n := len(allFilePaths(&manifest.Manifest{})); n != 0 {
		t.Errorf("empty manifest should yield no paths, got %d", n)
	}
}

// TestRestoreCmd_AllRejectsSelectorCombo: --all is standalone and the combo check
// fails before any AWS call.
func TestRestoreCmd_AllRejectsSelectorCombo(t *testing.T) {
	cmd := NewRestoreCmd()
	var out bytes.Buffer
	cmd.SetOut(&out)
	cmd.SetErr(&out)
	cmd.SetArgs([]string{"s3://b/p/uploads/x", t.TempDir(), "--all", "--file", "a.txt"})
	err := cmd.Execute()
	if err == nil || !strings.Contains(err.Error(), "cannot be combined") {
		t.Fatalf("--all with --file should error 'cannot be combined', got: %v", err)
	}
}
