package resume

import (
	"os"
	"path/filepath"
	"testing"
)

// TestStatePath_RefusesEscapingIDs pins #921: the upload ID (from user input or a
// state file's JSON) was joined into a path unchecked, so DeleteState("../x")
// removed a file outside the state directory.
func TestStatePath_RefusesEscapingIDs(t *testing.T) {
	home := t.TempDir()
	t.Setenv("HOME", home)
	t.Setenv("USERPROFILE", home)
	victim := filepath.Join(home, ".cargoship", "precious.json")
	if err := os.MkdirAll(filepath.Dir(victim), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(victim, []byte("{}"), 0o600); err != nil {
		t.Fatal(err)
	}

	for _, id := range []string{"../precious", "..", ".", "a/b", `a\b`, "x..y", ""} {
		if err := DeleteState(id); err == nil {
			t.Errorf("DeleteState(%q) must be refused", id)
		}
		if _, err := LoadState(id); err == nil {
			t.Errorf("LoadState(%q) must be refused", id)
		}
		if StateExists(id) {
			t.Errorf("StateExists(%q) must be false", id)
		}
	}
	if _, err := os.Stat(victim); err != nil {
		t.Fatalf("a file outside the state directory was touched: %v", err)
	}

	if err := SaveState(&UploadState{UploadID: "../escape"}); err == nil {
		t.Error("SaveState with an escaping ID must be refused")
	}
	ok := &UploadState{UploadID: "20261125-abcd1234"}
	if err := SaveState(ok); err != nil {
		t.Fatalf("a normal ID: %v", err)
	}
	if !StateExists(ok.UploadID) {
		t.Error("a normal ID's state must exist after saving")
	}
}
