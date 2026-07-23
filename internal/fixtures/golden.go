package fixtures

import (
	"flag"
	"os"
	"path/filepath"
	"regexp"
	"testing"
)

// updateGolden is toggled by `go test -update`. When set, AssertGolden rewrites
// the golden file instead of comparing against it.
var updateGolden = flag.Bool("update", false, "update golden files instead of comparing")

// Normalizer rewrites volatile substrings (timestamps, generated IDs, temp
// paths) in command output so golden comparisons stay stable across runs.
type Normalizer func(string) string

// Common volatile-output patterns. These are deliberately broad: golden tests
// assert the stable shape of output, not the exact timestamp/id/path.
var (
	reRFC3339  = regexp.MustCompile(`\d{4}-\d{2}-\d{2}T\d{2}:\d{2}:\d{2}(?:\.\d+)?(?:Z|[+-]\d{2}:\d{2})`)
	reUploadID = regexp.MustCompile(`\d{8}-[0-9a-f]{6,}`)
	reTmpPath  = regexp.MustCompile(`/(?:tmp|var|private)/[^\s"']*cargoship[^\s"']*`)
	reDuration = regexp.MustCompile(`\d+(?:\.\d+)?(?:ns|µs|us|ms|s)\b`)
)

// NormalizeVolatile replaces timestamps, upload IDs, temp paths, and durations
// with stable placeholders. It's the default normalization for CLI output.
func NormalizeVolatile(s string) string {
	s = reRFC3339.ReplaceAllString(s, "<TIMESTAMP>")
	s = reUploadID.ReplaceAllString(s, "<UPLOAD_ID>")
	s = reTmpPath.ReplaceAllString(s, "<TMP>")
	s = reDuration.ReplaceAllString(s, "<DUR>")
	return s
}

// GoldenPath returns testdata/golden/<name>.golden relative to the caller's
// package directory.
func GoldenPath(name string) string {
	return filepath.Join("testdata", "golden", name+".golden")
}

// AssertGolden compares got (after applying normalizers) against the golden
// file for name. With `-update`, it writes the normalized output instead.
// Normalizers are applied in order; pass NormalizeVolatile for CLI output.
func AssertGolden(t *testing.T, name, got string, normalizers ...Normalizer) {
	t.Helper()
	for _, n := range normalizers {
		got = n(got)
	}

	path := GoldenPath(name)
	if *updateGolden {
		if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
			t.Fatalf("golden: mkdir %s: %v", filepath.Dir(path), err)
		}
		if err := os.WriteFile(path, []byte(got), 0o644); err != nil {
			t.Fatalf("golden: write %s: %v", path, err)
		}
		t.Logf("golden: updated %s", path)
		return
	}

	want, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("golden: read %s: %v (run `go test -update` to create it)", path, err)
	}
	if got != string(want) {
		t.Errorf("golden mismatch for %s.\n--- want ---\n%s\n--- got ---\n%s\n(run `go test -update` to accept)",
			name, string(want), got)
	}
}
