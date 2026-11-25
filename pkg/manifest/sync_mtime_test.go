package manifest

import (
	"testing"
	"time"
)

// TestHasChanged_OlderSameSizeCopyIsAChange pins #918: a file replaced by a
// same-size copy with an OLDER mtime (cp -p, rsync -t, a snapshot restore) was
// treated as unchanged, so its current content was never backed up.
func TestHasChanged_OlderSameSizeCopyIsAChange(t *testing.T) {
	rec := time.Date(2026, 11, 20, 10, 30, 45, 123456789, time.UTC)
	entry := FileEntry{Path: "f", Size: 100, ModTime: rec}
	tests := []struct {
		name    string
		mod     time.Time
		size    int64
		changed bool
	}{
		{"identical", rec, 100, false},
		{"same instant, other zone", rec.In(time.FixedZone("x", 3600)), 100, false},
		{"newer", rec.Add(time.Second), 100, true},
		{"older copy, same size", rec.Add(-48 * time.Hour), 100, true},
		{"older by a millisecond", rec.Add(-time.Millisecond), 100, true},
		{"size differs", rec, 101, true},
		{"restored to microsecond precision", rec.Truncate(time.Microsecond), 100, false},
		{"restored to second precision", rec.Truncate(time.Second), 100, false},
		{"restored to FAT 2s precision", rec.Truncate(2 * time.Second), 100, false},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			got := hasChanged(FileInfo{Path: "f", Size: tc.size, ModTime: tc.mod}, entry, &SyncOptions{})
			if got != tc.changed {
				t.Errorf("hasChanged = %v, want %v", got, tc.changed)
			}
		})
	}
}

func TestComputeDelta_OlderCopyIsModified(t *testing.T) {
	rec := time.Unix(1_700_000_000, 0).UTC()
	prev := &Manifest{Files: []FileEntry{{Path: "doc.txt", Size: 10, ModTime: rec, S3Key: "uploads/u/c"}}}
	d, err := ComputeDelta([]FileInfo{{Path: "doc.txt", Size: 10, ModTime: rec.Add(-30 * 24 * time.Hour)}}, prev, &SyncOptions{})
	if err != nil {
		t.Fatal(err)
	}
	if len(d.Modified) != 1 {
		t.Fatalf("an older same-size copy must be re-uploaded: modified=%d same=%d", len(d.Modified), len(d.Same))
	}
}
