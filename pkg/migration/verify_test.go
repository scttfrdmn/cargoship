package migration

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/scttfrdmn/cargoship/pkg/extraction"
	"github.com/scttfrdmn/cargoship/pkg/manifest"
)

func writeTree(t *testing.T, files map[string]string) string {
	t.Helper()
	dir := t.TempDir()
	for rel, body := range files {
		p := filepath.Join(dir, filepath.FromSlash(rel))
		if err := os.MkdirAll(filepath.Dir(p), 0o755); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(p, []byte(body), 0o600); err != nil {
			t.Fatal(err)
		}
	}
	return dir
}

func uploaded(dir string, files map[string]int64) *manifest.Manifest {
	m := &manifest.Manifest{SourcePath: dir}
	for rel, size := range files {
		m.Files = append(m.Files, manifest.FileEntry{Path: filepath.Join(dir, filepath.FromSlash(rel)), Size: size, S3Key: "uploads/u/c"})
	}
	return m
}

// TestVerifyUploadCovers pins #911's deletion gate: the original archive may be
// deleted only when every extracted file is in the new upload at its size and
// the archive held nothing the upload can't store.
func TestVerifyUploadCovers(t *testing.T) {
	dir := writeTree(t, map[string]string{"a.txt": "aaa", "sub/b.txt": "bb"})
	ok := &extraction.ExtractionStats{}

	if err := verifyUploadCovers(dir, ok, uploaded(dir, map[string]int64{"a.txt": 3, "sub/b.txt": 2})); err != nil {
		t.Fatalf("a complete upload must allow the delete: %v", err)
	}

	tests := []struct {
		name  string
		stats *extraction.ExtractionStats
		m     *manifest.Manifest
		want  string
	}{
		{"a file missing from the upload", ok, uploaded(dir, map[string]int64{"a.txt": 3}), "sub/b.txt"},
		{"a file stored short", ok, uploaded(dir, map[string]int64{"a.txt": 1, "sub/b.txt": 2}), "stored 1 of 3"},
		{"unsupported entries", &extraction.ExtractionStats{UnsupportedEntries: 1, UnsupportedExamples: []string{"dev/null"}},
			uploaded(dir, map[string]int64{"a.txt": 3, "sub/b.txt": 2}), "dev/null"},
		{"symlinks", &extraction.ExtractionStats{SymlinksExtracted: 2}, uploaded(dir, map[string]int64{"a.txt": 3, "sub/b.txt": 2}), "symlink"},
		{"no manifest", ok, nil, "manifest"},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			err := verifyUploadCovers(dir, tc.stats, tc.m)
			if err == nil || !strings.Contains(err.Error(), tc.want) {
				t.Errorf("err = %v, want one mentioning %q", err, tc.want)
			}
		})
	}

	unstored := uploaded(dir, map[string]int64{"a.txt": 3, "sub/b.txt": 2})
	unstored.Files[0].S3Key = ""
	if err := verifyUploadCovers(dir, ok, unstored); err == nil {
		t.Error("an entry with no stored location does not count as uploaded")
	}
}
