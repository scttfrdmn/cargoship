package version

import (
	"regexp"
	"testing"
)

// TestVersionIsSemver ensures the embedded canonical version is a clean,
// trimmed semantic version without a leading "v" — the form goreleaser and the
// doc drift check expect.
func TestVersionIsSemver(t *testing.T) {
	if Version == "" {
		t.Fatal("embedded Version is empty")
	}
	if Version != trim(Version) {
		t.Errorf("Version %q has surrounding whitespace", Version)
	}
	if Version[0] == 'v' {
		t.Errorf("Version %q must not carry a leading v", Version)
	}
	if !regexp.MustCompile(`^\d+\.\d+\.\d+`).MatchString(Version) {
		t.Errorf("Version %q is not a semantic version", Version)
	}
}

func trim(s string) string {
	for len(s) > 0 && (s[0] == ' ' || s[0] == '\n' || s[0] == '\t' || s[0] == '\r') {
		s = s[1:]
	}
	for len(s) > 0 {
		c := s[len(s)-1]
		if c == ' ' || c == '\n' || c == '\t' || c == '\r' {
			s = s[:len(s)-1]
			continue
		}
		break
	}
	return s
}
