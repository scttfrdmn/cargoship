// Package manifestcache persists the manifest of a completed upload locally so a
// later incremental sync can compute a delta without reading the previous manifest
// back from S3.
//
// It exists for the write-only fleet agent (#604). A strict write-only identity has
// PutObject but no GetObject on its own data, so `downloadLatestManifest` fails and
// ComputeDelta runs against a nil previous manifest — which marks every file New and
// re-uploads the entire source every cycle. That is correct but unusable past a few
// hundred GB. A locally cached manifest restores incremental behaviour without
// granting the agent read access to its own archive.
//
// # Trust posture
//
// The cache is a claim about what is already in S3, and a wrong claim means files are
// silently never uploaded. Every guard here therefore fails CLOSED: any doubt about an
// entry (missing, truncated, corrupt, written by a different schema, describing a
// different destination, or simply too old) makes Load reject it, and the caller falls
// back to a full sync. A full re-upload is expensive; a wrongly skipped file is data
// loss, so the asymmetry is deliberate.
//
// The cache is never authoritative. Callers must prefer the manifest in S3 whenever
// they can read it, and use the cache only as a fallback.
package manifestcache

import (
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"time"

	"github.com/scttfrdmn/cargoship/pkg/manifest"
)

// SchemaVersion is the on-disk format version of a cache entry. Load rejects any
// entry written by a different version rather than guessing at its meaning, so this
// must be bumped whenever Entry's semantics change.
//
// 2: the entry holds the EFFECTIVE manifest — the whole dataset as of that cycle — not
// the raw per-cycle manifest. Version 1 stored the raw one, which lists only that
// cycle's increment, so a following cycle diffed against a near-empty file set and
// re-uploaded everything (#691). A v1 entry is rejected by Load, which costs one full
// sync and then self-corrects.
const SchemaVersion = 2

// DefaultMaxAge bounds how long a cached manifest is trusted. The cache cannot detect
// that objects it vouches for were deleted, expired by a lifecycle rule, or lost, so
// trusting one indefinitely would let such a gap persist forever. Expiring it forces a
// periodic full sync that re-establishes the real contents of the destination.
const DefaultMaxAge = 30 * 24 * time.Hour

// ErrNotFound reports that no cache entry exists for a key. It is an ordinary
// condition — the first sync of a source always hits it — and is distinguished from a
// rejected entry so callers can log the two differently.
var ErrNotFound = errors.New("manifestcache: no entry")

// Key identifies the destination a cached manifest describes. All four fields take
// part in both the filename and the integrity check: an entry whose stored key does
// not match the requested one is rejected, so a cache written for one
// bucket/prefix/source/writer can never satisfy a lookup for another.
type Key struct {
	Bucket     string `json:"bucket"`
	Prefix     string `json:"prefix"`
	SourcePath string `json:"source_path"`
	WriterID   string `json:"writer_id"`
}

// filename derives the on-disk name for a key. The digest keeps arbitrary bucket
// names, prefixes and absolute source paths out of the filesystem namespace; the key
// is stored inside the file and re-verified on load, so the digest is a lookup
// shortcut and never the thing that establishes identity.
func (k Key) filename() string {
	sum := sha256.Sum256([]byte(fmt.Sprintf("%q|%q|%q|%q", k.Bucket, k.Prefix, k.SourcePath, k.WriterID)))
	return hex.EncodeToString(sum[:16]) + ".json"
}

// sha256Hex is the digest form recorded in and verified from Entry.ManifestSHA256.
func sha256Hex(b []byte) string {
	sum := sha256.Sum256(b)
	return hex.EncodeToString(sum[:])
}

// Entry is one cached manifest together with everything needed to decide whether it
// can still be trusted.
type Entry struct {
	SchemaVersion    int             `json:"schema_version"`
	Key              Key             `json:"key"`
	CachedAt         time.Time       `json:"cached_at"`
	CargoshipVersion string          `json:"cargoship_version,omitempty"`
	UploadID         string          `json:"upload_id"`
	ManifestSHA256   string          `json:"manifest_sha256"`
	Manifest         json.RawMessage `json:"manifest"`
}

// Store is a directory of cache entries.
type Store struct {
	dir string
}

// DefaultDir is the default cache location, alongside the resume state in
// ~/.cargoship. In the fleet container this path is the mounted `cargoship-state`
// volume, so the cache survives container restarts — without that it would be
// recreated empty on every restart and every first cycle would re-upload everything.
func DefaultDir() (string, error) {
	home, err := os.UserHomeDir()
	if err != nil {
		return "", fmt.Errorf("manifestcache: resolve home directory: %w", err)
	}
	return filepath.Join(home, ".cargoship", "manifest-cache"), nil
}

// NewStore returns a Store rooted at dir, or at DefaultDir when dir is empty. The
// directory is not created until a Save needs it.
func NewStore(dir string) (*Store, error) {
	if dir == "" {
		d, err := DefaultDir()
		if err != nil {
			return nil, err
		}
		dir = d
	}
	return &Store{dir: dir}, nil
}

// Dir reports the directory the store writes to.
func (s *Store) Dir() string { return s.dir }

// Save writes m to the cache under k, replacing any existing entry.
//
// m must be the EFFECTIVE manifest — the full dataset as of this cycle, not the
// increment this cycle uploaded. Storing the raw per-cycle manifest is what caused
// #691: the next cycle diffs against it, sees a near-empty file set, and re-uploads the
// whole source. A write-only agent cannot walk the chain to work this out later, so the
// merge has to happen before the entry is written.
//
// Call it only after an upload has fully succeeded. Caching a manifest for an upload
// that failed partway would record files as present that were never stored, and the
// next delta would skip them.
//
// The write is atomic (temp file then rename) so a crash mid-write leaves the previous
// entry intact rather than a truncated one.
func (s *Store) Save(k Key, m *manifest.Manifest, cargoshipVersion string) error {
	if m == nil {
		return errors.New("manifestcache: save: nil manifest")
	}
	raw, err := json.Marshal(m)
	if err != nil {
		return fmt.Errorf("manifestcache: marshal manifest: %w", err)
	}
	e := Entry{
		SchemaVersion:    SchemaVersion,
		Key:              k,
		CachedAt:         time.Now().UTC(),
		CargoshipVersion: cargoshipVersion,
		UploadID:         m.UploadID,
		ManifestSHA256:   sha256Hex(raw),
		Manifest:         raw,
	}
	// json.Marshal, NOT MarshalIndent: indenting the entry also RE-INDENTS the embedded
	// json.RawMessage, so the manifest bytes on disk would no longer be the bytes the
	// digest above was taken over and every Load would fail the checksum. The entry is
	// machine-read, so compactness costs nothing.
	data, err := json.Marshal(e)
	if err != nil {
		return fmt.Errorf("manifestcache: marshal entry: %w", err)
	}

	// 0700 to match the 0600 entries below: the cache enumerates the source tree, so
	// it is owner-only rather than the 0755 used for less sensitive local state.
	if err := os.MkdirAll(s.dir, 0o700); err != nil {
		return fmt.Errorf("manifestcache: create cache dir: %w", err)
	}
	final := filepath.Join(s.dir, k.filename())
	tmp := final + ".tmp"
	// 0600: a manifest enumerates every path and size under the source tree, which is
	// more than other local users need to know about the data being archived.
	if err := os.WriteFile(tmp, data, 0o600); err != nil {
		return fmt.Errorf("manifestcache: write temp entry: %w", err)
	}
	if err := os.Rename(tmp, final); err != nil {
		_ = os.Remove(tmp)
		return fmt.Errorf("manifestcache: install entry: %w", err)
	}
	return nil
}

// Load returns the cached manifest for k, or an error explaining why no cached
// manifest can be trusted. maxAge <= 0 uses DefaultMaxAge.
//
// Callers must treat ANY error as "no previous manifest" and fall back to a full sync.
// Load never returns a partially validated manifest: the entry must parse, declare the
// current schema, carry the requested key, be within maxAge, and have a manifest whose
// SHA-256 matches the digest recorded when it was written.
func (s *Store) Load(k Key, maxAge time.Duration) (*manifest.Manifest, error) {
	if maxAge <= 0 {
		maxAge = DefaultMaxAge
	}
	path := filepath.Join(s.dir, k.filename())
	data, err := os.ReadFile(path) // #nosec G304 -- path is dir + a digest-derived name
	if err != nil {
		if os.IsNotExist(err) {
			return nil, ErrNotFound
		}
		return nil, fmt.Errorf("manifestcache: read entry: %w", err)
	}

	var e Entry
	if err := json.Unmarshal(data, &e); err != nil {
		return nil, fmt.Errorf("manifestcache: entry is not valid JSON (ignoring): %w", err)
	}
	if e.SchemaVersion != SchemaVersion {
		return nil, fmt.Errorf("manifestcache: entry schema %d, want %d (ignoring)", e.SchemaVersion, SchemaVersion)
	}
	if e.Key != k {
		// Only reachable via a digest collision or a hand-edited file, but the whole
		// point of storing the key is to not take the filename's word for it.
		return nil, fmt.Errorf("manifestcache: entry describes a different destination (ignoring)")
	}
	if age := time.Since(e.CachedAt); age > maxAge {
		return nil, fmt.Errorf("manifestcache: entry is %s old, limit %s (ignoring, forcing a full sync)",
			age.Truncate(time.Hour), maxAge)
	}
	if sha256Hex(e.Manifest) != e.ManifestSHA256 {
		return nil, fmt.Errorf("manifestcache: manifest checksum mismatch (ignoring)")
	}

	var m manifest.Manifest
	if err := json.Unmarshal(e.Manifest, &m); err != nil {
		return nil, fmt.Errorf("manifestcache: cached manifest is not valid JSON (ignoring): %w", err)
	}
	return &m, nil
}

// Delete removes the entry for k. A missing entry is not an error.
func (s *Store) Delete(k Key) error {
	err := os.Remove(filepath.Join(s.dir, k.filename()))
	if err != nil && !os.IsNotExist(err) {
		return fmt.Errorf("manifestcache: delete entry: %w", err)
	}
	return nil
}
