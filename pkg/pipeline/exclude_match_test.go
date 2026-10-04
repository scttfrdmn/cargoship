package pipeline

import "testing"

// Exclusion has to be able to say "not this directory, and nothing under it" (#710).
// The previous matcher compared only filepath.Base(path), so a pattern like "#recycle"
// matched the directory entry while every file inside it (basename "invoice.pdf") was
// still archived — subtree exclusion was not expressible at all. That is the dominant
// real need: on a Synology, #recycle alone was 20 GB of 26 GB, and .aws/.ssh hold
// credentials that must never reach an archive.
//
// Semantics implemented here: a pattern matches if it matches the whole relative path,
// or ANY single path segment. Segment matching is what gives subtree exclusion without
// needing "**", and it keeps plain basename patterns like "*.tmp" working.
func TestMatchesExcludePattern(t *testing.T) {
	tests := []struct {
		name     string
		relPath  string
		patterns []string
		want     bool
	}{
		// The case that motivated this.
		{"subtree: the directory itself", "#recycle", []string{"#recycle"}, true},
		{"subtree: a file inside it", "#recycle/invoice.pdf", []string{"#recycle"}, true},
		{"subtree: deep inside it", "#recycle/a/b/c/old.txt", []string{"#recycle"}, true},

		// Secrets must be excludable the same way.
		{"secrets: .aws subtree", ".aws/credentials", []string{".aws", ".ssh"}, true},
		{"secrets: .ssh subtree", ".ssh/id_ed25519", []string{".aws", ".ssh"}, true},

		// NAS furniture appears nested, not only at the top level.
		{"nested metadata dir", "photos/2026/@eaDir/SYNOPHOTO_THUMB.jpg", []string{"@eaDir"}, true},

		// Plain basename globs must keep working.
		{"basename glob", "work/build/out.tmp", []string{"*.tmp"}, true},
		{"basename glob no match", "work/build/out.bin", []string{"*.tmp"}, false},

		// A pattern containing a separator matches the relative path.
		{"path pattern", "erebus-work/private/key.txt", []string{"erebus-work/private"}, true},

		// Must not over-match.
		{"similar name is not a match", "recycle-bin/x.txt", []string{"#recycle"}, false},
		{"partial segment is not a match", "my#recycleX/x.txt", []string{"#recycle"}, false},
		{"no patterns excludes nothing", "anything/at/all", nil, false},
		{"unrelated pattern", "docs/readme.md", []string{"#recycle", ".aws"}, false},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			if got := matchesExcludePattern(tc.relPath, tc.patterns); got != tc.want {
				t.Errorf("matchesExcludePattern(%q, %v) = %v, want %v",
					tc.relPath, tc.patterns, got, tc.want)
			}
		})
	}
}
