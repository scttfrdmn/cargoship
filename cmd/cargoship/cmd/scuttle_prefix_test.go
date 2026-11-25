package cmd

import "testing"

// TestScuttlePrefix pins #920: --prefix backup also matched backup-2024/...
func TestScuttlePrefix(t *testing.T) {
	for in, want := range map[string]string{"": "", "backup": "backup/", "backup/": "backup/", "a/b": "a/b/"} {
		if got := scuttlePrefix(in); got != want {
			t.Errorf("scuttlePrefix(%q) = %q, want %q", in, got, want)
		}
	}
}
