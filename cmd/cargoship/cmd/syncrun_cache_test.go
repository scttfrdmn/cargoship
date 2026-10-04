package cmd

import (
	"encoding/json"
	"errors"
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/scttfrdmn/cargoship/pkg/manifest"
	"github.com/scttfrdmn/cargoship/pkg/manifestcache"
)

func cacheTestParams(t *testing.T, store *manifestcache.Store) syncRunParams {
	t.Helper()
	return syncRunParams{
		bucket:        "fleet-bucket",
		prefix:        "base/writers/lab-nas-1",
		sourcePath:    "/share/data",
		writerID:      "lab-nas-1",
		manifestCache: store,
	}
}

func cachedManifest(uploadID string) *manifest.Manifest {
	return &manifest.Manifest{
		UploadID:   uploadID,
		SourcePath: "/share/data",
		Files: []manifest.FileEntry{
			{Path: "/share/data/a.txt", Size: 11, ModTime: time.Unix(1_700_000_000, 0).UTC()},
		},
	}
}

func s3Returns(m *manifest.Manifest) func() (*manifest.Manifest, error) {
	return func() (*manifest.Manifest, error) { return m, nil }
}

// accessDenied models the write-only case this feature exists for: the agent can PUT
// but cannot GET its own previous manifest.
func accessDenied() func() (*manifest.Manifest, error) {
	return func() (*manifest.Manifest, error) { return nil, errors.New("AccessDenied") }
}

// backdateCacheEntry rewrites the stored entry's cached_at to age it by d.
//
// Staleness must NOT be expressed as "a trust window narrower than the time that has
// elapsed since Save" (e.g. maxAge = 1ns). Save stamps CachedAt with time.Now().UTC(),
// and .UTC() strips the monotonic reading, so Load's time.Since falls back to the wall
// clock — whose granularity on Windows is coarse enough (~15ms) that both calls can land
// in the same tick and report an age of exactly 0, which is not > 1ns. The entry then
// reads as fresh and the case silently asserts the opposite of its name. That is what
// made this subtest flake on windows-latest while passing everywhere else.
//
// Backdating the timestamp instead makes the age explicit and the test independent of
// clock resolution. Only cached_at is touched: the recorded digest covers the embedded
// manifest, so the entry stays internally consistent and still exercises the real
// expiry branch in Load rather than a stub.
func backdateCacheEntry(t *testing.T, store *manifestcache.Store, d time.Duration) {
	t.Helper()
	entries, err := filepath.Glob(filepath.Join(store.Dir(), "*.json"))
	if err != nil {
		t.Fatalf("glob cache dir: %v", err)
	}
	if len(entries) != 1 {
		t.Fatalf("want exactly 1 cache entry to backdate, found %d", len(entries))
	}
	raw, err := os.ReadFile(entries[0])
	if err != nil {
		t.Fatalf("read cache entry: %v", err)
	}
	// json.RawMessage, NOT map[string]any: decoding into `any` and re-encoding rewrites
	// the embedded manifest (key order, number formatting), which changes its bytes and
	// so breaks the manifest_sha256 recorded at Save. The entry would then be rejected as
	// CORRUPT rather than EXPIRED — the subtest would still pass, for the wrong reason,
	// and would no longer detect a broken expiry check at all. Verified by mutation: with
	// the age comparison in Load disabled, this subtest must fail.
	var e map[string]json.RawMessage
	if err := json.Unmarshal(raw, &e); err != nil {
		t.Fatalf("parse cache entry: %v", err)
	}
	stamp, err := json.Marshal(time.Now().UTC().Add(-d))
	if err != nil {
		t.Fatalf("encode cached_at: %v", err)
	}
	e["cached_at"] = stamp
	out, err := json.Marshal(e)
	if err != nil {
		t.Fatalf("re-encode cache entry: %v", err)
	}
	if err := os.WriteFile(entries[0], out, 0o600); err != nil {
		t.Fatalf("write cache entry: %v", err)
	}
}

// The decision table is the whole trust argument for the cache, so it is tested
// directly rather than inferred from an end-to-end run.
func TestResolvePreviousManifest(t *testing.T) {
	tests := []struct {
		name       string
		force      bool
		withStore  bool
		seedCache  *manifest.Manifest
		backdate   time.Duration // age the seeded entry by this much before resolving
		maxAge     time.Duration
		fromS3     func() (*manifest.Manifest, error)
		wantSource string
		wantUpload string // "" = expect no previous manifest
		wantNote   bool
	}{
		{
			name:       "S3 readable wins outright",
			withStore:  true,
			seedCache:  cachedManifest("from-cache"),
			fromS3:     s3Returns(cachedManifest("from-s3")),
			wantSource: deltaSourceS3,
			wantUpload: "from-s3",
		},
		{
			name:       "write-only falls back to the cache",
			withStore:  true,
			seedCache:  cachedManifest("from-cache"),
			fromS3:     accessDenied(),
			wantSource: deltaSourceCache,
			wantUpload: "from-cache",
		},
		{
			name:       "force ignores both, so a full re-upload is always reachable",
			force:      true,
			withStore:  true,
			seedCache:  cachedManifest("from-cache"),
			fromS3:     s3Returns(cachedManifest("from-s3")),
			wantSource: deltaSourceNone,
		},
		{
			name:       "no cache configured and no S3 means a full sync",
			withStore:  false,
			fromS3:     accessDenied(),
			wantSource: deltaSourceNone,
		},
		{
			name:       "first cycle: empty cache is not an error worth reporting",
			withStore:  true,
			fromS3:     accessDenied(),
			wantSource: deltaSourceNone,
		},
		{
			name:      "an expired entry is refused and reported",
			withStore: true,
			seedCache: cachedManifest("stale"),
			// Explicitly aged well past the window, rather than relying on time having
			// elapsed since Save — see backdateCacheEntry for why that flaked on Windows.
			backdate:   48 * time.Hour,
			maxAge:     24 * time.Hour,
			fromS3:     accessDenied(),
			wantSource: deltaSourceNone,
			wantNote:   true,
		},
		{
			name:       "S3 returning a nil manifest without an error is not treated as readable",
			withStore:  true,
			seedCache:  cachedManifest("from-cache"),
			fromS3:     s3Returns(nil),
			wantSource: deltaSourceCache,
			wantUpload: "from-cache",
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			var store *manifestcache.Store
			if tc.withStore {
				s, err := manifestcache.NewStore(t.TempDir())
				if err != nil {
					t.Fatalf("NewStore: %v", err)
				}
				store = s
			}
			p := cacheTestParams(t, store)
			p.force = tc.force
			p.manifestCacheMaxAge = tc.maxAge

			if tc.seedCache != nil {
				if err := store.Save(cacheKeyFor(p), tc.seedCache, "test"); err != nil {
					t.Fatalf("seed Save: %v", err)
				}
				if tc.backdate > 0 {
					backdateCacheEntry(t, store, tc.backdate)
				}
			}

			got := resolvePreviousManifest(p, tc.fromS3)

			if got.source != tc.wantSource {
				t.Errorf("source = %q, want %q", got.source, tc.wantSource)
			}
			if tc.wantUpload == "" {
				if got.manifest != nil {
					t.Errorf("got manifest %q, want none", got.manifest.UploadID)
				}
				if got.syncType() != manifest.SyncTypeFull {
					t.Errorf("syncType = %q, want full when there is no previous manifest", got.syncType())
				}
			} else {
				if got.manifest == nil {
					t.Fatalf("got no manifest, want %q", tc.wantUpload)
				}
				if got.manifest.UploadID != tc.wantUpload {
					t.Errorf("manifest = %q, want %q", got.manifest.UploadID, tc.wantUpload)
				}
				if got.syncType() != manifest.SyncTypeIncremental {
					t.Errorf("syncType = %q, want incremental", got.syncType())
				}
			}
			if gotNote := got.note != ""; gotNote != tc.wantNote {
				t.Errorf("note present = %v (%q), want %v", gotNote, got.note, tc.wantNote)
			}
		})
	}
}

// The point of the whole feature: a write-only agent's SECOND cycle must diff against
// what the first cycle stored, instead of calling every file new again.
func TestWriteOnlyAgentGoesIncrementalOnTheSecondCycle(t *testing.T) {
	store, err := manifestcache.NewStore(t.TempDir())
	if err != nil {
		t.Fatalf("NewStore: %v", err)
	}
	p := cacheTestParams(t, store)

	// Cycle 1: nothing cached yet, S3 unreadable -> full sync, as today.
	first := resolvePreviousManifest(p, accessDenied())
	if first.source != deltaSourceNone || first.manifest != nil {
		t.Fatalf("cycle 1: source=%q manifest=%v, want a full sync", first.source, first.manifest)
	}

	// The pipeline succeeded, so the cycle records what it stored.
	stored := cachedManifest("cycle-1-upload")
	if err := store.Save(cacheKeyFor(p), stored, "test"); err != nil {
		t.Fatalf("Save: %v", err)
	}

	// Cycle 2: still no read access, but now there is a previous manifest to diff against.
	second := resolvePreviousManifest(p, accessDenied())
	if second.source != deltaSourceCache {
		t.Fatalf("cycle 2: source = %q, want %q -- without this the agent re-uploads everything forever",
			second.source, deltaSourceCache)
	}
	if second.manifest == nil || second.manifest.UploadID != "cycle-1-upload" {
		t.Fatalf("cycle 2 diffed against %v, want cycle 1's manifest", second.manifest)
	}

	// And the delta against an unchanged tree must be empty, which is what turns a
	// full re-upload into a no-op cycle.
	local := []manifest.FileInfo{
		{Path: "a.txt", Size: 11, ModTime: time.Unix(1_700_000_000, 0).UTC()},
	}
	delta, err := manifest.ComputeDelta(local, second.manifest, &manifest.SyncOptions{})
	if err != nil {
		t.Fatalf("ComputeDelta: %v", err)
	}
	if delta.HasChanges() {
		t.Errorf("delta reports changes for an unchanged tree: new=%d modified=%d",
			len(delta.New), len(delta.Modified))
	}
}

// --force must remain an escape hatch: if an operator suspects the cache is wrong, it
// has to be possible to rebuild from scratch.
func TestForceBypassesAUsableCache(t *testing.T) {
	store, err := manifestcache.NewStore(t.TempDir())
	if err != nil {
		t.Fatalf("NewStore: %v", err)
	}
	p := cacheTestParams(t, store)
	if err := store.Save(cacheKeyFor(p), cachedManifest("cached"), "test"); err != nil {
		t.Fatalf("Save: %v", err)
	}

	p.force = true
	got := resolvePreviousManifest(p, accessDenied())
	if got.manifest != nil || got.source != deltaSourceNone {
		t.Errorf("force gave source=%q manifest=%v, want a full sync", got.source, got.manifest)
	}
}

// WriterID must contribute to the key on its own. In production the prefix is
// writer-folded so the two co-vary, which means a test that changes both proves only
// that SOMETHING isolates them -- it would still pass if WriterID were dropped from the
// key. Holding bucket/prefix/source identical pins WriterID's own contribution, so
// isolation cannot be lost by a refactor that stops folding the id into the prefix.
func TestCacheKeyIsolatesOnWriterIDAlone(t *testing.T) {
	store, err := manifestcache.NewStore(t.TempDir())
	if err != nil {
		t.Fatalf("NewStore: %v", err)
	}
	a := cacheTestParams(t, store)
	b := cacheTestParams(t, store)
	b.writerID = "lab-nas-2" // ONLY the writer id differs

	if err := store.Save(cacheKeyFor(a), cachedManifest("writer-a"), "test"); err != nil {
		t.Fatalf("Save: %v", err)
	}
	if got := resolvePreviousManifest(b, accessDenied()); got.manifest != nil {
		t.Errorf("writer %q read writer %q's cached manifest (%q); WriterID is not isolating",
			b.writerID, a.writerID, got.manifest.UploadID)
	}
}

// Two writers sharing a bucket must not inherit each other's view of the world.
func TestCacheKeyIsolatesWriters(t *testing.T) {
	store, err := manifestcache.NewStore(t.TempDir())
	if err != nil {
		t.Fatalf("NewStore: %v", err)
	}
	a := cacheTestParams(t, store)
	b := cacheTestParams(t, store)
	b.writerID = "lab-nas-2"
	b.prefix = "base/writers/lab-nas-2"

	if err := store.Save(cacheKeyFor(a), cachedManifest("writer-a"), "test"); err != nil {
		t.Fatalf("Save: %v", err)
	}

	gotB := resolvePreviousManifest(b, accessDenied())
	if gotB.manifest != nil {
		t.Errorf("writer b picked up writer a's manifest (%q)", gotB.manifest.UploadID)
	}
	gotA := resolvePreviousManifest(a, accessDenied())
	if gotA.manifest == nil || gotA.manifest.UploadID != "writer-a" {
		t.Errorf("writer a lost its own manifest: %v", gotA.manifest)
	}
}
