package manifest

import (
	"testing"
	"time"
)

// TestFileChanged pins the exported change rule (#1002) to sync's hasChanged.
func TestFileChanged(t *testing.T) {
	rec := time.Date(2026, 11, 1, 10, 30, 15, 123456789, time.UTC)
	entry := FileEntry{Size: 12, ModTime: rec}
	tests := []struct {
		name  string
		size  int64
		mtime time.Time
		want  bool
	}{
		{"unchanged", 12, rec, false},
		{"size differs", 13, rec, true},
		{"newer mtime, same size", 12, rec.Add(time.Second), true},
		{"older mtime, same size", 12, rec.Add(-time.Hour), true},
		{"1s-precision filesystem", 12, rec.Truncate(time.Second), false},
		{"FAT 2s precision", 12, rec.Truncate(2 * time.Second), false},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := FileChanged(tt.size, tt.mtime, entry); got != tt.want {
				t.Fatalf("FileChanged = %v, want %v", got, tt.want)
			}
			local := FileInfo{Size: tt.size, ModTime: tt.mtime}
			if got := hasChanged(local, entry, nil); got != tt.want {
				t.Fatalf("sync's hasChanged = %v, but FileChanged = %v: the rules diverged", got, tt.want)
			}
		})
	}
}
