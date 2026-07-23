package staging

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"time"
)

// compressionHistoryEnv is the opt-in switch (and optional location override)
// for durable compression-ratio history. This is derived telemetry — only
// content-type -> ratio aggregates, never file content — so it is off by
// default, consistent with the rest of the Option S measurement work (#262).
//
//	unset / empty  -> persistence disabled (in-memory only, prior behavior)
//	"1" or "true"  -> enabled at the default state path
//	<file path>    -> enabled at that explicit path (used by tests)
const compressionHistoryEnv = "CARGOSHIP_COMPRESSION_HISTORY"

// compressionHistoryDecayWindow is the age past which historical results are
// discarded on load and down-weighted in GetAverageRatio. Kept as a single
// source of truth so the on-load pruning and the running average agree.
const compressionHistoryDecayWindow = 30 * 24 * time.Hour

// compressionHistoryStoreVersion is the on-disk schema version.
const compressionHistoryStoreVersion = 1

// compressionHistoryDoc is the persisted on-disk shape of a CompressionHistory.
type compressionHistoryDoc struct {
	Version int                                       `json:"version"`
	Results map[string][]*HistoricalCompressionResult `json:"results"`
}

// compressionHistoryStore locates and persists the learned compression history.
// A zero store (enabled=false) is a no-op, so an in-memory-only history carries
// no persistence cost.
type compressionHistoryStore struct {
	enabled bool
	path    string
}

// newCompressionHistoryStore resolves the opt-in state from the environment.
// When persistence is disabled it returns a no-op store.
func newCompressionHistoryStore() compressionHistoryStore {
	v := os.Getenv(compressionHistoryEnv)
	switch v {
	case "":
		return compressionHistoryStore{}
	case "1", "true":
		path, err := defaultCompressionHistoryPath()
		if err != nil {
			return compressionHistoryStore{}
		}
		return compressionHistoryStore{enabled: true, path: path}
	default:
		return compressionHistoryStore{enabled: true, path: v}
	}
}

// defaultCompressionHistoryPath mirrors the state-dir convention used by the
// budget store (pkg/aws/cost/budget_store.go): $XDG_DATA_HOME/cargoship, else
// ~/.cargoship.
func defaultCompressionHistoryPath() (string, error) {
	base := os.Getenv("XDG_DATA_HOME")
	if base == "" {
		home, err := os.UserHomeDir()
		if err != nil {
			return "", fmt.Errorf("get home dir: %w", err)
		}
		base = filepath.Join(home, ".cargoship")
	} else {
		base = filepath.Join(base, "cargoship")
	}
	return filepath.Join(base, "compression_history.json"), nil
}

// load reads the persisted history, pruning entries older than the decay window
// and enforcing the per-content-type cap (most recent kept). A missing file is
// not an error — it returns an empty map. Persistence being disabled likewise
// yields an empty map with no I/O.
func (s compressionHistoryStore) load(now time.Time, maxResults int) (map[string][]*HistoricalCompressionResult, error) {
	if !s.enabled {
		return map[string][]*HistoricalCompressionResult{}, nil
	}
	data, err := os.ReadFile(s.path) //nolint:gosec // path is env-configured (opt-in) or derived from home/XDG, not attacker input
	if err != nil {
		if os.IsNotExist(err) {
			return map[string][]*HistoricalCompressionResult{}, nil
		}
		return nil, fmt.Errorf("read compression history %q: %w", s.path, err)
	}
	var doc compressionHistoryDoc
	if err := json.Unmarshal(data, &doc); err != nil {
		return nil, fmt.Errorf("parse compression history %q: %w", s.path, err)
	}
	if doc.Results == nil {
		return map[string][]*HistoricalCompressionResult{}, nil
	}

	cutoff := now.Add(-compressionHistoryDecayWindow)
	pruned := make(map[string][]*HistoricalCompressionResult, len(doc.Results))
	for contentType, results := range doc.Results {
		kept := make([]*HistoricalCompressionResult, 0, len(results))
		for _, r := range results {
			if r == nil || !r.Timestamp.After(cutoff) {
				continue // drop nil and stale (older than the decay window) entries
			}
			kept = append(kept, r)
		}
		if len(kept) == 0 {
			continue
		}
		// Oldest-first ordering matches AddResult's append semantics.
		sort.Slice(kept, func(i, j int) bool {
			return kept[i].Timestamp.Before(kept[j].Timestamp)
		})
		if maxResults > 0 && len(kept) > maxResults {
			kept = kept[len(kept)-maxResults:] // keep the most recent maxResults
		}
		pruned[contentType] = kept
	}
	return pruned, nil
}

// save writes the history atomically (temp file + rename) so a crash mid-write
// cannot corrupt the store. It is a no-op when persistence is disabled.
func (s compressionHistoryStore) save(results map[string][]*HistoricalCompressionResult) error {
	if !s.enabled {
		return nil
	}
	doc := compressionHistoryDoc{Version: compressionHistoryStoreVersion, Results: results}
	data, err := json.MarshalIndent(doc, "", "  ")
	if err != nil {
		return fmt.Errorf("marshal compression history: %w", err)
	}
	if err := os.MkdirAll(filepath.Dir(s.path), 0700); err != nil {
		return fmt.Errorf("create compression history dir: %w", err)
	}
	tmp, err := os.CreateTemp(filepath.Dir(s.path), ".compression-history-*.tmp")
	if err != nil {
		return fmt.Errorf("create temp compression history file: %w", err)
	}
	tmpName := tmp.Name()
	defer func() { _ = os.Remove(tmpName) }() // no-op after a successful rename
	if _, err := tmp.Write(data); err != nil {
		_ = tmp.Close()
		return fmt.Errorf("write temp compression history file: %w", err)
	}
	if err := tmp.Chmod(0600); err != nil {
		_ = tmp.Close()
		return fmt.Errorf("chmod temp compression history file: %w", err)
	}
	if err := tmp.Close(); err != nil {
		return fmt.Errorf("close temp compression history file: %w", err)
	}
	if err := os.Rename(tmpName, s.path); err != nil {
		return fmt.Errorf("rename compression history into place: %w", err)
	}
	return nil
}
