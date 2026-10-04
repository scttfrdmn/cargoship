package manifestcache

import (
	"encoding/json"
	"errors"
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/scttfrdmn/cargoship/pkg/manifest"
)

func testKey() Key {
	return Key{
		Bucket:     "fleet-bucket",
		Prefix:     "base/writers/lab-nas-1",
		SourcePath: "/share/data",
		WriterID:   "lab-nas-1",
	}
}

func testManifest() *manifest.Manifest {
	return &manifest.Manifest{
		UploadID:       "20261001-120000-abcd1234",
		SourcePath:     "/share/data",
		DatasetID:      "20260901-000000-root0000",
		VersionOrdinal: 4,
		Files: []manifest.FileEntry{
			{Path: "/share/data/a.txt", Size: 11, ModTime: time.Unix(1_700_000_000, 0).UTC()},
			{Path: "/share/data/sub/b.bin", Size: 2048, ModTime: time.Unix(1_700_000_500, 0).UTC()},
		},
	}
}

func newTestStore(t *testing.T) *Store {
	t.Helper()
	s, err := NewStore(t.TempDir())
	if err != nil {
		t.Fatalf("NewStore: %v", err)
	}
	return s
}

// writeRawEntry installs a hand-built entry so tests can corrupt individual fields
// without going through Save.
func writeRawEntry(t *testing.T, s *Store, k Key, mutate func(*Entry)) {
	t.Helper()
	raw, err := json.Marshal(testManifest())
	if err != nil {
		t.Fatalf("marshal manifest: %v", err)
	}
	e := Entry{
		SchemaVersion:  SchemaVersion,
		Key:            k,
		CachedAt:       time.Now().UTC(),
		UploadID:       "20261001-120000-abcd1234",
		ManifestSHA256: sha256Hex(raw),
		Manifest:       raw,
	}
	if mutate != nil {
		mutate(&e)
	}
	// Must match Save's encoder exactly: MarshalIndent would re-indent the embedded
	// RawMessage and break the digest, which is the bug this suite caught.
	data, err := json.Marshal(e)
	if err != nil {
		t.Fatalf("marshal entry: %v", err)
	}
	if err := os.MkdirAll(s.Dir(), 0o700); err != nil {
		t.Fatalf("mkdir: %v", err)
	}
	if err := os.WriteFile(filepath.Join(s.Dir(), k.filename()), data, 0o600); err != nil {
		t.Fatalf("write entry: %v", err)
	}
}

func TestSaveLoadRoundTrip(t *testing.T) {
	s := newTestStore(t)
	k := testKey()
	want := testManifest()

	if err := s.Save(k, want, "0.34.1"); err != nil {
		t.Fatalf("Save: %v", err)
	}
	got, err := s.Load(k, 0)
	if err != nil {
		t.Fatalf("Load: %v", err)
	}

	// The fields ComputeDelta actually reads must survive exactly, or the delta is
	// wrong in a way that silently skips files.
	if got.SourcePath != want.SourcePath {
		t.Errorf("SourcePath = %q, want %q", got.SourcePath, want.SourcePath)
	}
	if len(got.Files) != len(want.Files) {
		t.Fatalf("Files len = %d, want %d", len(got.Files), len(want.Files))
	}
	for i := range want.Files {
		w, g := want.Files[i], got.Files[i]
		if g.Path != w.Path || g.Size != w.Size || !g.ModTime.Equal(w.ModTime) {
			t.Errorf("Files[%d] = {%q %d %v}, want {%q %d %v}",
				i, g.Path, g.Size, g.ModTime, w.Path, w.Size, w.ModTime)
		}
	}
	// Dataset identity must survive too: DatasetIDOf short-circuits on DatasetID, which
	// is what keeps a write-only agent's chain from forking into a new dataset.
	if got.DatasetID != want.DatasetID {
		t.Errorf("DatasetID = %q, want %q", got.DatasetID, want.DatasetID)
	}
	if got.VersionOrdinal != want.VersionOrdinal {
		t.Errorf("VersionOrdinal = %d, want %d", got.VersionOrdinal, want.VersionOrdinal)
	}
}

func TestLoadRejectsUntrustworthyEntries(t *testing.T) {
	tests := []struct {
		name   string
		mutate func(*Entry)
	}{
		{
			name:   "schema from a different version",
			mutate: func(e *Entry) { e.SchemaVersion = SchemaVersion + 1 },
		},
		{
			name:   "manifest body does not match its recorded digest",
			mutate: func(e *Entry) { e.ManifestSHA256 = "0000000000000000000000000000000000000000000000000000000000000000" },
		},
		{
			name: "manifest body tampered after the digest was computed",
			mutate: func(e *Entry) {
				e.Manifest = json.RawMessage(`{"upload_id":"evil","source_path":"/share/data","files":[]}`)
			},
		},
		{
			name:   "entry is older than the trust window",
			mutate: func(e *Entry) { e.CachedAt = time.Now().Add(-DefaultMaxAge - time.Hour).UTC() },
		},
		{
			name:   "entry describes a different bucket",
			mutate: func(e *Entry) { e.Key.Bucket = "someone-elses-bucket" },
		},
		{
			name:   "entry describes a different writer",
			mutate: func(e *Entry) { e.Key.WriterID = "other-writer" },
		},
		{
			// Valid JSON, so the entry itself parses and the digest can be made to
			// match -- this reaches the final guard, where the body is decoded as a
			// manifest. A truncated body cannot be built through json.Marshal at all
			// (RawMessage validates), so that case is covered by
			// TestLoadRejectsCorruptFileBytes instead.
			name: "manifest body is valid JSON but not a manifest object",
			mutate: func(e *Entry) {
				e.Manifest = json.RawMessage(`"not a manifest object"`)
				e.ManifestSHA256 = sha256Hex(e.Manifest)
			},
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			s := newTestStore(t)
			k := testKey()
			writeRawEntry(t, s, k, tc.mutate)

			m, err := s.Load(k, 0)
			if err == nil {
				t.Fatal("Load succeeded; a questionable entry must be rejected so the caller does a full sync")
			}
			if m != nil {
				t.Errorf("Load returned a manifest (%v) alongside an error; it must never hand back partially validated data", m.UploadID)
			}
		})
	}
}

func TestLoadMissingEntryIsErrNotFound(t *testing.T) {
	s := newTestStore(t)
	m, err := s.Load(testKey(), 0)
	if err == nil {
		t.Fatal("Load on an empty cache should report an error")
	}
	if !errors.Is(err, ErrNotFound) {
		t.Errorf("err = %v, want ErrNotFound so a first sync is not logged as corruption", err)
	}
	if m != nil {
		t.Error("Load returned a manifest for a missing entry")
	}
}

// A cache written for one destination must not satisfy a lookup for another, even
// though both live in the same directory. This is the guard against a reconfigured
// agent reusing a stale view of a different bucket or source.
func TestDistinctKeysDoNotCollide(t *testing.T) {
	s := newTestStore(t)
	k1 := testKey()
	k2 := testKey()
	k2.SourcePath = "/share/other"

	if err := s.Save(k1, testManifest(), "0.34.1"); err != nil {
		t.Fatalf("Save: %v", err)
	}
	if _, err := s.Load(k2, 0); err == nil {
		t.Fatal("a different source path resolved to the first key's entry")
	}
	if _, err := s.Load(k1, 0); err != nil {
		t.Fatalf("original key no longer loads: %v", err)
	}
}

func TestSaveIsAtomicAndOverwrites(t *testing.T) {
	s := newTestStore(t)
	k := testKey()

	if err := s.Save(k, testManifest(), "0.34.1"); err != nil {
		t.Fatalf("first Save: %v", err)
	}
	second := testManifest()
	second.UploadID = "20261002-130000-ffff9999"
	second.VersionOrdinal = 5
	if err := s.Save(k, second, "0.34.1"); err != nil {
		t.Fatalf("second Save: %v", err)
	}

	got, err := s.Load(k, 0)
	if err != nil {
		t.Fatalf("Load: %v", err)
	}
	if got.UploadID != second.UploadID {
		t.Errorf("UploadID = %q, want the newer %q", got.UploadID, second.UploadID)
	}

	// No temp files may survive a successful save, or the directory grows without bound.
	entries, err := os.ReadDir(s.Dir())
	if err != nil {
		t.Fatalf("ReadDir: %v", err)
	}
	for _, e := range entries {
		if filepath.Ext(e.Name()) == ".tmp" {
			t.Errorf("leftover temp file %q", e.Name())
		}
	}
	if len(entries) != 1 {
		t.Errorf("got %d entries, want 1 (overwrite, not accumulate)", len(entries))
	}
}

func TestSaveRejectsNilManifest(t *testing.T) {
	s := newTestStore(t)
	if err := s.Save(testKey(), nil, "0.34.1"); err == nil {
		t.Fatal("Save(nil) should fail rather than record an empty dataset as complete")
	}
}

func TestDeleteRemovesEntryAndIgnoresMissing(t *testing.T) {
	s := newTestStore(t)
	k := testKey()
	if err := s.Save(k, testManifest(), "0.34.1"); err != nil {
		t.Fatalf("Save: %v", err)
	}
	if err := s.Delete(k); err != nil {
		t.Fatalf("Delete: %v", err)
	}
	if _, err := s.Load(k, 0); !errors.Is(err, ErrNotFound) {
		t.Errorf("after Delete, Load err = %v, want ErrNotFound", err)
	}
	if err := s.Delete(k); err != nil {
		t.Errorf("Delete on a missing entry should be a no-op, got %v", err)
	}
}

func TestCustomMaxAgeOverridesDefault(t *testing.T) {
	s := newTestStore(t)
	k := testKey()
	writeRawEntry(t, s, k, func(e *Entry) {
		e.CachedAt = time.Now().Add(-2 * time.Hour).UTC()
	})

	if _, err := s.Load(k, time.Hour); err == nil {
		t.Error("a 2h-old entry should be rejected under a 1h limit")
	}
	if _, err := s.Load(k, 24*time.Hour); err != nil {
		t.Errorf("a 2h-old entry should be accepted under a 24h limit, got %v", err)
	}
}

func TestDefaultDirIsUnderCargoshipHome(t *testing.T) {
	d, err := DefaultDir()
	if err != nil {
		t.Skipf("no home directory available: %v", err)
	}
	if filepath.Base(d) != "manifest-cache" || filepath.Base(filepath.Dir(d)) != ".cargoship" {
		t.Errorf("DefaultDir = %q, want .../.cargoship/manifest-cache so the fleet state volume covers it", d)
	}
}

// Entries can be damaged by something other than our own writer -- disk corruption, a
// half-written file predating the atomic-rename path, or hand editing. Load must reject
// those too rather than panicking or returning a half-decoded manifest.
func TestLoadRejectsCorruptFileBytes(t *testing.T) {
	tests := []struct {
		name    string
		content string
	}{
		{"empty file", ""},
		{"not JSON at all", "\x00\x01garbage"},
		{"truncated entry JSON", `{"schema_version":1,"key":{"bucket":"fleet-bucket"`},
		{"truncated manifest body", `{"schema_version":1,"manifest":{"files":`},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			s := newTestStore(t)
			k := testKey()
			if err := os.MkdirAll(s.Dir(), 0o700); err != nil {
				t.Fatalf("mkdir: %v", err)
			}
			if err := os.WriteFile(filepath.Join(s.Dir(), k.filename()), []byte(tc.content), 0o600); err != nil {
				t.Fatalf("write: %v", err)
			}

			m, err := s.Load(k, 0)
			if err == nil {
				t.Fatal("Load accepted a corrupt entry; it must fall back to a full sync")
			}
			if m != nil {
				t.Error("Load returned a manifest from corrupt bytes")
			}
		})
	}
}

// A v1 entry holds the RAW per-cycle manifest; v2 holds the EFFECTIVE dataset (#691).
// Trusting a v1 entry under v2 semantics would diff against one cycle's increment and
// re-upload everything, so it must be refused. Any agent that ran v0.35.x has v1 entries
// on disk, making this the actual upgrade path rather than a hypothetical.
func TestLoadRejectsPreviousSchemaVersion(t *testing.T) {
	s := newTestStore(t)
	k := testKey()
	writeRawEntry(t, s, k, func(e *Entry) { e.SchemaVersion = 1 })

	m, err := s.Load(k, 0)
	if err == nil {
		t.Fatal("a v1 entry must be rejected: its manifest is the increment, not the dataset")
	}
	if m != nil {
		t.Error("Load returned a manifest from a v1 entry")
	}
	// Rejection costs one full sync and then self-corrects, so it must not look like
	// corruption to the operator.
	if errors.Is(err, ErrNotFound) {
		t.Error("a present-but-outdated entry should not report as ErrNotFound")
	}
}
