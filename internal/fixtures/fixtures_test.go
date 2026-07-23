package fixtures

import (
	"os"
	"path/filepath"
	"testing"
)

func TestManifestBuilder_Defaults(t *testing.T) {
	m := NewManifest().Build()
	if m.Version == "" {
		t.Fatal("expected a default version")
	}
	if m.UploadID == "" || m.Bucket == "" || m.Region == "" {
		t.Fatalf("expected deterministic defaults, got %+v", m)
	}
	if !m.CreatedAt.Equal(FixedTime) {
		t.Fatalf("CreatedAt = %v, want fixed %v", m.CreatedAt, FixedTime)
	}
}

func TestManifestBuilder_FilesAndStats(t *testing.T) {
	m := NewManifest().
		WithFile("z.txt", 100, 0, 0).
		WithFile("a.txt", 200, 0, 0).
		Build()

	if m.TotalFiles != 2 {
		t.Fatalf("TotalFiles = %d, want 2", m.TotalFiles)
	}
	if m.TotalBytes != 300 {
		t.Fatalf("TotalBytes = %d, want 300", m.TotalBytes)
	}
	// Files sorted by path for stability.
	if m.Files[0].Path != "a.txt" || m.Files[1].Path != "z.txt" {
		t.Fatalf("files not sorted by path: %q, %q", m.Files[0].Path, m.Files[1].Path)
	}
	if m.Files[0].S3Key == "" {
		t.Fatal("expected a derived S3Key")
	}
}

func TestManifestBuilder_Deterministic(t *testing.T) {
	build := func() string {
		m := NewManifest().WithSimpleFiles(5, 1024).Build()
		b, err := m.ToJSON()
		if err != nil {
			t.Fatalf("ToJSON: %v", err)
		}
		return string(b)
	}
	first, second := build(), build()
	if first != second {
		t.Fatal("fixture manifest is not byte-stable across builds")
	}
}

func TestTree_Write(t *testing.T) {
	root := MixedTree().Write(t)
	// Every declared file exists with the right content.
	for name, want := range MixedTree() {
		p := filepath.Join(root, filepath.FromSlash(name))
		got, err := os.ReadFile(p)
		if err != nil {
			t.Fatalf("read %s: %v", name, err)
		}
		if string(got) != want {
			t.Fatalf("%s content = %q, want %q", name, got, want)
		}
	}
}

func TestNormalizeVolatile(t *testing.T) {
	cases := []struct{ in, want string }{
		{"at 2026-07-22T23:30:46.432345-07:00 done", "at <TIMESTAMP> done"},
		{"upload 20260722-abcd1234 ok", "upload <UPLOAD_ID> ok"},
		{"path /tmp/cargoship-integration-999/x", "path <TMP>"},
		{"took 1.53ms flat", "took <DUR> flat"},
	}
	for _, c := range cases {
		if got := NormalizeVolatile(c.in); got != c.want {
			t.Errorf("NormalizeVolatile(%q) = %q, want %q", c.in, got, c.want)
		}
	}
}

// TestGolden_ManifestJSON exercises the golden helper end-to-end using a
// deterministic fixture manifest. The golden file is checked in; regenerate
// with `go test ./internal/fixtures/ -update`.
func TestGolden_ManifestJSON(t *testing.T) {
	m := NewManifest().
		WithUploadID("20260102-testfixture").
		WithSimpleFiles(3, 2048).
		Build()

	data, err := m.ToJSON()
	if err != nil {
		t.Fatalf("ToJSON: %v", err)
	}
	// Manifest JSON is already deterministic here (fixed times, sorted files),
	// so no normalizer is needed — this also guards manifest field-tag drift.
	AssertGolden(t, "manifest_simple", string(data))
}

func TestAssertGolden_UpdateAndCompare(t *testing.T) {
	// Drive the update path into a temp golden dir, then the compare path, without
	// touching checked-in files. We can't easily redirect GoldenPath, so validate
	// the normalize+write+read cycle via the exported pieces directly.
	dir := t.TempDir()
	path := filepath.Join(dir, "sample.golden")
	content := NormalizeVolatile("id 20260101-deadbeef at 2026-01-01T00:00:00Z")
	if err := os.WriteFile(path, []byte(content), 0o644); err != nil {
		t.Fatal(err)
	}
	got, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	if string(got) != "id <UPLOAD_ID> at <TIMESTAMP>" {
		t.Fatalf("normalized golden = %q", got)
	}
}
