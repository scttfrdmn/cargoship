package cost

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"sync"
	"time"
)

// UploadHistoryEnv is the environment override for the per-upload outcome
// history location (#261). It takes precedence over the config field. Values:
//
//	unset / empty  -> disabled (no history written), unless config enables it
//	"1" or "true"  -> enabled at the default state path
//	<file path>    -> enabled at that explicit path (used by tests)
const UploadHistoryEnv = "CARGOSHIP_UPLOAD_HISTORY"

// uploadHistoryStoreVersion is the on-disk schema version.
const uploadHistoryStoreVersion = 1

// defaultUploadHistoryMax bounds the append-only history (FIFO — oldest records
// are dropped once the cap is exceeded) so it can't grow without limit.
const defaultUploadHistoryMax = 10000

// UploadOutcome is a compact, metadata-only record joining one completed
// upload's inputs (dataset shape and chosen parameters) to its outcomes
// (actual compression, timing, throughput, reliability, cost). It is the
// training corpus the Option S analysis (#167) needs: no file content, names,
// or paths — only aggregates and type counts. (#261)
type UploadOutcome struct {
	// Identity
	UploadID  string    `json:"upload_id"`
	ProjectID string    `json:"project_id,omitempty"`
	Timestamp time.Time `json:"timestamp"`

	// Inputs — upload characteristics and chosen parameters.
	TotalBytes       int64          `json:"total_bytes"` // uncompressed source size
	FileCount        int64          `json:"file_count"`
	FileTypeMix      map[string]int `json:"file_type_mix,omitempty"` // extension (or detected type) -> count
	ChunkCount       int            `json:"chunk_count"`
	ShardCount       int            `json:"shard_count"`
	CompressionType  string         `json:"compression_type,omitempty"`
	CompressionLevel int            `json:"compression_level"`
	StorageClass     string         `json:"storage_class,omitempty"`
	Region           string         `json:"region,omitempty"`

	// Outcomes — measured results.
	CompressionRatio float64       `json:"compression_ratio"`
	CompressedBytes  int64         `json:"compressed_bytes"`
	Duration         time.Duration `json:"duration_ns"`
	ThroughputMBps   float64       `json:"throughput_mbps"`
	RetryCount       int           `json:"retry_count"`
	ErrorCount       int           `json:"error_count"`
	Success          bool          `json:"success"`

	// Cost is this upload's recorded USD cost. It joins to the #246 CostRecord
	// ledger by UploadID (CostRecord.JobID == UploadID); it is populated here
	// when the caller has it to hand and otherwise left zero (derivable from
	// the ledger). RetryCount is likewise reserved: the pipeline does not yet
	// surface a retry counter, so it stays zero until one exists.
	Cost float64 `json:"cost_usd"`
}

// uploadHistoryDoc is the persisted on-disk shape of the outcome history.
type uploadHistoryDoc struct {
	Version int              `json:"version"`
	Records []*UploadOutcome `json:"records"`
}

// UploadHistoryStore is an opt-in, durable, append-only history of per-upload
// outcomes, reusing the atomic-write + state-dir conventions of the budget
// store (#246). A zero store (enabled=false) is a no-op, so the common
// telemetry-off path performs no I/O.
type UploadHistoryStore struct {
	enabled bool
	path    string
	max     int
	mu      sync.Mutex
}

// NewUploadHistoryStore resolves the opt-in state. Precedence, matching the
// #246 budget-store convention: the CARGOSHIP_UPLOAD_HISTORY env override
// first, then the provided config location (CostControl.UploadHistoryLocation).
// This is telemetry — default OFF. A "1"/"true" spec enables the default state
// path; any other non-empty spec is treated as an explicit file path.
func NewUploadHistoryStore(configLocation string) *UploadHistoryStore {
	spec := os.Getenv(UploadHistoryEnv)
	if spec == "" {
		spec = configLocation
	}
	switch spec {
	case "":
		return &UploadHistoryStore{}
	case "1", "true":
		path, err := defaultUploadHistoryPath()
		if err != nil {
			return &UploadHistoryStore{}
		}
		return &UploadHistoryStore{enabled: true, path: path, max: defaultUploadHistoryMax}
	default:
		return &UploadHistoryStore{enabled: true, path: spec, max: defaultUploadHistoryMax}
	}
}

// Enabled reports whether the history is being persisted.
func (s *UploadHistoryStore) Enabled() bool { return s != nil && s.enabled }

// defaultUploadHistoryPath mirrors the budget store's state-dir convention:
// $XDG_DATA_HOME/cargoship, else ~/.cargoship.
func defaultUploadHistoryPath() (string, error) {
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
	return filepath.Join(base, "upload_history.json"), nil
}

// LoadOutcomes reads the persisted outcome history, oldest-first. A missing
// file (or a disabled store) returns an empty slice, not an error.
func (s *UploadHistoryStore) LoadOutcomes() ([]*UploadOutcome, error) {
	if !s.Enabled() {
		return nil, nil
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.loadLocked()
}

// loadLocked reads and parses the store. Caller must hold s.mu.
func (s *UploadHistoryStore) loadLocked() ([]*UploadOutcome, error) {
	data, err := os.ReadFile(s.path) //nolint:gosec // path is env/config-configured (opt-in) or derived from home/XDG, not attacker input
	if err != nil {
		if os.IsNotExist(err) {
			return nil, nil
		}
		return nil, fmt.Errorf("read upload history %q: %w", s.path, err)
	}
	var doc uploadHistoryDoc
	if err := json.Unmarshal(data, &doc); err != nil {
		return nil, fmt.Errorf("parse upload history %q: %w", s.path, err)
	}
	return doc.Records, nil
}

// Append adds one outcome to the durable history (read-modify-write under an
// exclusive lock), enforcing the FIFO cap, and writes it back atomically. It is
// a no-op when the store is disabled. A load error on an existing-but-corrupt
// store is surfaced; callers on the upload path treat any error as best-effort
// and must never fail an upload because of it (mirrors persistLedger, #246).
func (s *UploadHistoryStore) Append(outcome *UploadOutcome) error {
	if !s.Enabled() || outcome == nil {
		return nil
	}
	s.mu.Lock()
	defer s.mu.Unlock()

	records, err := s.loadLocked()
	if err != nil {
		return err
	}
	records = append(records, outcome)
	if s.max > 0 && len(records) > s.max {
		records = records[len(records)-s.max:] // FIFO: keep the most recent s.max
	}
	return s.saveLocked(records)
}

// saveLocked writes the history atomically (temp file + rename, 0600). Caller
// must hold s.mu.
func (s *UploadHistoryStore) saveLocked(records []*UploadOutcome) error {
	doc := uploadHistoryDoc{Version: uploadHistoryStoreVersion, Records: records}
	data, err := json.MarshalIndent(doc, "", "  ")
	if err != nil {
		return fmt.Errorf("marshal upload history: %w", err)
	}
	if err := os.MkdirAll(filepath.Dir(s.path), 0700); err != nil {
		return fmt.Errorf("create upload history dir: %w", err)
	}
	tmp, err := os.CreateTemp(filepath.Dir(s.path), ".upload-history-*.tmp")
	if err != nil {
		return fmt.Errorf("create temp upload history file: %w", err)
	}
	tmpName := tmp.Name()
	defer func() { _ = os.Remove(tmpName) }() // no-op after a successful rename
	if _, err := tmp.Write(data); err != nil {
		_ = tmp.Close()
		return fmt.Errorf("write temp upload history file: %w", err)
	}
	if err := tmp.Chmod(0600); err != nil {
		_ = tmp.Close()
		return fmt.Errorf("chmod temp upload history file: %w", err)
	}
	if err := tmp.Close(); err != nil {
		return fmt.Errorf("close temp upload history file: %w", err)
	}
	if err := os.Rename(tmpName, s.path); err != nil {
		return fmt.Errorf("rename upload history into place: %w", err)
	}
	return nil
}

// FileTypeMixFromExtensions builds a metadata-only extension -> count histogram
// from a list of relative paths. Only the lowercased extension (or "none" when
// absent) is retained — never the file name or path. Kept as a helper so the
// caller can assemble the mix from a manifest's file list without this package
// importing pkg/manifest.
func FileTypeMixFromExtensions(exts []string) map[string]int {
	if len(exts) == 0 {
		return nil
	}
	mix := make(map[string]int)
	for _, e := range exts {
		if e == "" {
			e = "none"
		}
		mix[e]++
	}
	return mix
}

// sortOutcomesByTime returns the records sorted oldest-first. Exposed for the
// debug view; the store already persists in append (oldest-first) order.
func sortOutcomesByTime(records []*UploadOutcome) {
	sort.SliceStable(records, func(i, j int) bool {
		return records[i].Timestamp.Before(records[j].Timestamp)
	})
}
