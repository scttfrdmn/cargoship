package manifest

import (
	"testing"
	"time"
)

func TestRelativeToSource(t *testing.T) {
	cases := []struct{ src, entry, want string }{
		{"/data/src", "/data/src/a.txt", "a.txt"},
		{"/data/src", "/data/src/docs/readme.md", "docs/readme.md"},
		{"src", "src/a.txt", "a.txt"},
		{"/data/src/", "/data/src/a.txt", "a.txt"},                    // trailing slash on source
		{"", "a.txt", "a.txt"},                                        // empty source → unchanged
		{"/data/src", "/data/srcextra/a.txt", "/data/srcextra/a.txt"}, // no false segment match
	}
	for _, c := range cases {
		if got := RelativeToSource(c.src, c.entry); got != c.want {
			t.Errorf("RelativeToSource(%q,%q)=%q, want %q", c.src, c.entry, got, c.want)
		}
	}
}

// TestComputeDelta_FullPathManifest_Unchanged is the #624 regression: manifests store
// FileEntry.Path as the full (source-prefixed) walked path, while ScanLocalFiles yields
// source-relative paths. An unchanged file must resolve to Same (not New/Modified) and
// must not be reported deleted.
func TestComputeDelta_FullPathManifest_Unchanged(t *testing.T) {
	mod := time.Now().Add(-time.Hour)
	cases := []struct{ name, sourcePath, entryPath, local string }{
		{"absolute source", "/data/src", "/data/src/a.txt", "a.txt"},
		{"relative root", "src", "src/a.txt", "a.txt"},
		{"nested", "/data/src", "/data/src/docs/readme.md", "docs/readme.md"},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			prev := &Manifest{
				SourcePath: tc.sourcePath,
				Files:      []FileEntry{{Path: tc.entryPath, Size: 3, ModTime: mod}},
			}
			local := []FileInfo{{Path: tc.local, Size: 3, ModTime: mod}}
			d, err := ComputeDelta(local, prev, &SyncOptions{TrackDeletes: true})
			if err != nil {
				t.Fatalf("ComputeDelta: %v", err)
			}
			if len(d.New) != 0 || len(d.Modified) != 0 || len(d.Same) != 1 {
				t.Fatalf("unchanged file should be Same: New=%v Modified=%v Same=%v", d.New, d.Modified, d.Same)
			}
			if len(d.Deleted) != 0 {
				t.Fatalf("unchanged file must not be reported deleted, got %v", d.Deleted)
			}
		})
	}
}
