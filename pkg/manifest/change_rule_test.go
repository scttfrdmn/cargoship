package manifest

import (
	"testing"
	"time"
)

// TestFileChanged pins the exported change rule (#1002) to sync's hasChanged,
// plus the one allowance sync's sameModTime still lacks: NTFS's 100 ns time
// precision.
func TestFileChanged(t *testing.T) {
	rec := time.Date(2026, 11, 1, 10, 30, 15, 123456789, time.UTC)
	entry := FileEntry{Size: 12, ModTime: rec}
	tests := []struct {
		name     string
		size     int64
		mtime    time.Time
		want     bool
		syncSame bool // sync's hasChanged agrees
	}{
		{"unchanged", 12, rec, false, true},
		{"size differs", 13, rec, true, true},
		{"newer mtime, same size", 12, rec.Add(time.Second), true, true},
		{"older mtime, same size", 12, rec.Add(-time.Hour), true, true},
		{"1s-precision filesystem", 12, rec.Truncate(time.Second), false, true},
		{"FAT 2s precision", 12, rec.Truncate(2 * time.Second), false, true},
		{"older by less than 100ns is not a truncation", 12, rec.Add(-150 * time.Nanosecond), true, true},
		// sameModTime has no 100 ns entry, so sync reports this as changed
		// (see FileChanged's doc comment).
		{"NTFS 100ns precision", 12, rec.Truncate(100 * time.Nanosecond), false, false},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := FileChanged(tt.size, tt.mtime, entry); got != tt.want {
				t.Fatalf("FileChanged = %v, want %v", got, tt.want)
			}
			if !tt.syncSame {
				return
			}
			local := FileInfo{Size: tt.size, ModTime: tt.mtime}
			if got := hasChanged(local, entry, nil); got != tt.want {
				t.Fatalf("sync's hasChanged = %v, but FileChanged = %v: the rules diverged", got, tt.want)
			}
		})
	}
}
