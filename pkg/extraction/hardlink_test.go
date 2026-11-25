package extraction

import (
	"archive/tar"
	"bytes"
	"compress/gzip"
	"context"
	"io"
	"os"
	"path/filepath"
	"testing"
)

// tarGz builds a tar.gz from raw headers (with bodies for regular files).
func tarGz(t *testing.T, entries []struct {
	hdr  tar.Header
	body string
}) io.ReadCloser {
	t.Helper()
	var buf bytes.Buffer
	zw := gzip.NewWriter(&buf)
	tw := tar.NewWriter(zw)
	for _, e := range entries {
		h := e.hdr
		if h.Typeflag == tar.TypeReg {
			h.Size = int64(len(e.body))
		}
		if err := tw.WriteHeader(&h); err != nil {
			t.Fatal(err)
		}
		if e.body != "" {
			if _, err := tw.Write([]byte(e.body)); err != nil {
				t.Fatal(err)
			}
		}
	}
	if err := tw.Close(); err != nil {
		t.Fatal(err)
	}
	if err := zw.Close(); err != nil {
		t.Fatal(err)
	}
	return io.NopCloser(&buf)
}

// TestExtract_HardLinksAndUnsupportedEntries pins #911: a hard-link entry was
// silently skipped, so its name vanished and `migrate --delete-original` then
// deleted the only complete copy. Hard links must be restored with the target's
// bytes, and entries that can't be extracted must be counted, not dropped quietly.
func TestExtract_HardLinksAndUnsupportedEntries(t *testing.T) {
	dir := t.TempDir()
	body := tarGz(t, []struct {
		hdr  tar.Header
		body string
	}{
		{tar.Header{Name: "data/original.txt", Typeflag: tar.TypeReg, Mode: 0o644}, "precious bytes"},
		{tar.Header{Name: "data/second-name.txt", Typeflag: tar.TypeLink, Linkname: "data/original.txt", Mode: 0o644}, ""},
		{tar.Header{Name: "data/pipe", Typeflag: tar.TypeFifo, Mode: 0o644}, ""},
	})
	ex, err := NewExtractor(&ExtractorConfig{S3Client: &mockS3Client{body: body}, Bucket: "b", Key: "a.tar.gz",
		OutputDir: dir, CompressionFormat: CompressionAuto})
	if err != nil {
		t.Fatal(err)
	}
	stats, err := ex.Extract(context.Background())
	if err != nil {
		t.Fatal(err)
	}

	got, err := os.ReadFile(filepath.Join(dir, "data", "second-name.txt"))
	if err != nil || string(got) != "precious bytes" {
		t.Fatalf("hard link not restored with the target's content: %q, %v", got, err)
	}
	if stats.FilesExtracted != 2 || stats.HardLinksExtracted != 1 {
		t.Errorf("FilesExtracted=%d HardLinksExtracted=%d, want 2 and 1", stats.FilesExtracted, stats.HardLinksExtracted)
	}
	if stats.UnsupportedEntries != 1 || len(stats.UnsupportedExamples) != 1 {
		t.Errorf("the FIFO must be recorded as unsupported: %+v", stats)
	}
}

func TestExtract_HardLinkRefusesEscapesAndMissingTargets(t *testing.T) {
	for name, link := range map[string]string{
		"escaping target": "../outside.txt",
		"missing target":  "data/never-extracted.txt",
	} {
		t.Run(name, func(t *testing.T) {
			dir := t.TempDir()
			body := tarGz(t, []struct {
				hdr  tar.Header
				body string
			}{
				{tar.Header{Name: "data/link.txt", Typeflag: tar.TypeLink, Linkname: link, Mode: 0o644}, ""},
			})
			ex, err := NewExtractor(&ExtractorConfig{S3Client: &mockS3Client{body: body}, Bucket: "b", Key: "a.tar.gz",
				OutputDir: dir, CompressionFormat: CompressionAuto})
			if err != nil {
				t.Fatal(err)
			}
			if _, err := ex.Extract(context.Background()); err == nil {
				t.Fatal("a hard link to a target outside the archive's extracted files must fail")
			}
		})
	}
}

func extractEntries(t *testing.T, dir string, overwrite bool, entries []struct {
	hdr  tar.Header
	body string
}) (*ExtractionStats, error) {
	t.Helper()
	ex, err := NewExtractor(&ExtractorConfig{S3Client: &mockS3Client{body: tarGz(t, entries)}, Bucket: "b", Key: "a.tar.gz",
		OutputDir: dir, CompressionFormat: CompressionAuto, OverwriteExisting: overwrite})
	if err != nil {
		t.Fatal(err)
	}
	return ex.Extract(context.Background())
}

func TestExtract_HardLinkDestinationCases(t *testing.T) {
	reg := struct {
		hdr  tar.Header
		body string
	}{tar.Header{Name: "t.txt", Typeflag: tar.TypeReg, Mode: 0o644}, "target"}
	link := struct {
		hdr  tar.Header
		body string
	}{tar.Header{Name: "l.txt", Typeflag: tar.TypeLink, Linkname: "t.txt", Mode: 0o644}, ""}

	t.Run("existing file kept without overwrite", func(t *testing.T) {
		dir := t.TempDir()
		if err := os.WriteFile(filepath.Join(dir, "l.txt"), []byte("mine"), 0o600); err != nil {
			t.Fatal(err)
		}
		stats, err := extractEntries(t, dir, false, []struct {
			hdr  tar.Header
			body string
		}{reg, link})
		if err != nil {
			t.Fatal(err)
		}
		got, _ := os.ReadFile(filepath.Join(dir, "l.txt"))
		if string(got) != "mine" || stats.SkippedFiles != 1 {
			t.Errorf("content %q skipped %d, want the existing file kept and counted", got, stats.SkippedFiles)
		}
	})
	t.Run("existing file replaced with overwrite", func(t *testing.T) {
		dir := t.TempDir()
		if err := os.WriteFile(filepath.Join(dir, "l.txt"), []byte("old"), 0o600); err != nil {
			t.Fatal(err)
		}
		if _, err := extractEntries(t, dir, true, []struct {
			hdr  tar.Header
			body string
		}{reg, link}); err != nil {
			t.Fatal(err)
		}
		if got, _ := os.ReadFile(filepath.Join(dir, "l.txt")); string(got) != "target" {
			t.Errorf("content %q, want the link's target", got)
		}
	})
	t.Run("target is a directory", func(t *testing.T) {
		dir := t.TempDir()
		_, err := extractEntries(t, dir, true, []struct {
			hdr  tar.Header
			body string
		}{{tar.Header{Name: "d/", Typeflag: tar.TypeDir, Mode: 0o755}, ""},
			{tar.Header{Name: "l.txt", Typeflag: tar.TypeLink, Linkname: "d", Mode: 0o644}, ""}})
		if err == nil {
			t.Error("a hard link to a directory must fail")
		}
	})
	t.Run("destination is a symlink", func(t *testing.T) {
		dir := t.TempDir()
		if err := os.Symlink("t.txt", filepath.Join(dir, "l.txt")); err != nil {
			t.Skipf("symlinks unavailable: %v", err)
		}
		_, err := extractEntries(t, dir, true, []struct {
			hdr  tar.Header
			body string
		}{reg, link})
		if err == nil {
			t.Error("writing a hard link through a symlink must fail")
		}
	})
}

func TestCopyContained(t *testing.T) {
	dir := t.TempDir()
	if err := os.WriteFile(filepath.Join(dir, "src"), []byte("bytes"), 0o600); err != nil {
		t.Fatal(err)
	}
	root, err := os.OpenRoot(dir)
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = root.Close() }()

	if err := copyContained(root, "src", "dst", 0o600); err != nil {
		t.Fatal(err)
	}
	if got, _ := os.ReadFile(filepath.Join(dir, "dst")); string(got) != "bytes" {
		t.Errorf("copy = %q", got)
	}
	if err := copyContained(root, "src", "dst", 0o600); err == nil {
		t.Error("an existing destination must not be overwritten")
	}
	if err := copyContained(root, "missing", "x", 0o600); err == nil {
		t.Error("a missing source must fail")
	}
}
