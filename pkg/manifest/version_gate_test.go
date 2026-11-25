package manifest

import (
	"strings"
	"testing"
)

// TestFromJSON_RefusesNewerMajorVersion pins #921: a future 3.x manifest was read
// as 2.x and silently misinterpreted. Per the reader contract, a major bump is
// breaking.
func TestFromJSON_RefusesNewerMajorVersion(t *testing.T) {
	for v, ok := range map[string]bool{
		"2.0": true, "2.2": true, "2.9": true, "1.0": true, "": true, "test": true, "v2.1": true,
		"3.0": false, "3": false, "v3.1": false, "10.0": false,
	} {
		_, err := FromJSON([]byte(`{"version":"` + v + `","upload_id":"u","files":[],"chunks":[]}`))
		if ok && err != nil {
			t.Errorf("version %q: unexpected error %v", v, err)
		}
		if !ok && (err == nil || !strings.Contains(err.Error(), "upgrade cargoship")) {
			t.Errorf("version %q: err = %v, want an upgrade-cargoship refusal", v, err)
		}
	}
}

// TestResolveEntry_AmbiguousSuffixIsAnError pins #921: --file a/b.txt silently
// restored the first of x/a/b.txt and y/a/b.txt.
func TestResolveEntry_AmbiguousSuffixIsAnError(t *testing.T) {
	m := &Manifest{Files: []FileEntry{
		{Path: "x/a/b.txt", S3Key: "k"}, {Path: "y/a/b.txt", S3Key: "k"}, {Path: "z/only.txt", S3Key: "k"},
	}}
	se := NewSelectiveExtractor(m, &mockS3Client{}, 0)

	_, err := se.resolveEntryErr("a/b.txt")
	if err == nil || !strings.Contains(err.Error(), "x/a/b.txt") || !strings.Contains(err.Error(), "y/a/b.txt") {
		t.Errorf("an ambiguous suffix must be refused, naming both: %v", err)
	}
	if e, err := se.resolveEntryErr("x/a/b.txt"); err != nil || e == nil || e.Path != "x/a/b.txt" {
		t.Errorf("an exact path still resolves: %v %v", e, err)
	}
	if _, err := se.resolveEntryErr("b.txt"); err == nil {
		t.Error("an ambiguous basename must be refused")
	}
	if e, err := se.resolveEntryErr("only.txt"); err != nil || e == nil {
		t.Errorf("a unique basename resolves: %v %v", e, err)
	}
	if e, err := se.resolveEntryErr("missing.txt"); err != nil || e != nil {
		t.Errorf("a missing target is not an error here: %v %v", e, err)
	}

	stats, err := se.BatchRestore(t.Context(), []string{"a/b.txt"}, t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	if stats.Failed != 1 || !hasReason(stats, "matches 2 files") {
		t.Errorf("the restore must report the ambiguity: failed=%d reasons=%v", stats.Failed, stats.Errors)
	}
}
