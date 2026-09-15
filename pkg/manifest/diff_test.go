package manifest

import "testing"

func TestDiffFiles(t *testing.T) {
	from := []FileEntry{
		{Path: "keep.txt", Checksum: "aaa", Size: 10},
		{Path: "change.txt", Checksum: "bbb", Size: 20},
		{Path: "gone.txt", Checksum: "ccc", Size: 30},
	}
	to := []FileEntry{
		{Path: "keep.txt", Checksum: "aaa", Size: 10},    // unchanged
		{Path: "change.txt", Checksum: "bbb2", Size: 25}, // modified (checksum differs)
		{Path: "new.txt", Checksum: "ddd", Size: 40},     // added
	}
	d := DiffFiles(from, to)

	if len(d.Added) != 1 || d.Added[0] != "new.txt" {
		t.Errorf("Added = %v, want [new.txt]", d.Added)
	}
	if len(d.Removed) != 1 || d.Removed[0] != "gone.txt" {
		t.Errorf("Removed = %v, want [gone.txt]", d.Removed)
	}
	if len(d.Modified) != 1 || d.Modified[0] != "change.txt" {
		t.Errorf("Modified = %v, want [change.txt]", d.Modified)
	}
	if d.Unchanged != 1 {
		t.Errorf("Unchanged = %d, want 1", d.Unchanged)
	}
	if !d.HasChanges() {
		t.Error("HasChanges should be true")
	}
}

// TestDiffFiles_FallsBackToSize: with no checksums, content change is detected by size.
func TestDiffFiles_FallsBackToSize(t *testing.T) {
	from := []FileEntry{{Path: "a", Size: 10}}
	to := []FileEntry{{Path: "a", Size: 11}}
	if d := DiffFiles(from, to); len(d.Modified) != 1 {
		t.Errorf("size-only change should be Modified, got %+v", d)
	}
	// Identical size, no checksum → unchanged.
	if d := DiffFiles([]FileEntry{{Path: "a", Size: 10}}, []FileEntry{{Path: "a", Size: 10}}); d.Unchanged != 1 || d.HasChanges() {
		t.Errorf("identical entries should be unchanged, got %+v", d)
	}
}

func TestDiffFiles_Identical(t *testing.T) {
	files := []FileEntry{{Path: "a", Checksum: "x"}, {Path: "b", Checksum: "y"}}
	d := DiffFiles(files, files)
	if d.HasChanges() || d.Unchanged != 2 {
		t.Errorf("identical sets: got %+v, want no changes / 2 unchanged", d)
	}
}
