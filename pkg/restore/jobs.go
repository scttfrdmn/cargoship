// Package restore provides restore job persistence for tracking long-running
// S3 Glacier/Deep Archive restoration requests across sessions (Issue #202).
package restore

import (
	"crypto/rand"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"time"
)

// JobStatus represents the lifecycle state of a restore job.
type JobStatus string

const (
	// JobStatusPendingGlacier means Glacier restore has been requested but is
	// not yet complete — objects are not yet downloadable.
	JobStatusPendingGlacier JobStatus = "pending_glacier"
	// JobStatusReady means all Glacier objects are accessible and the job can
	// be downloaded.
	JobStatusReady JobStatus = "ready"
	// JobStatusDownloading means the download is currently in progress.
	JobStatusDownloading JobStatus = "downloading"
	// JobStatusComplete means all files have been written to the output directory.
	JobStatusComplete JobStatus = "complete"
	// JobStatusFailed means the job encountered an unrecoverable error.
	JobStatusFailed JobStatus = "failed"
)

// SelectionCriteria records which files in the manifest were targeted.
type SelectionCriteria struct {
	Hash      string   `json:"hash,omitempty"`
	FilePaths []string `json:"file_paths,omitempty"`
	GitCommit string   `json:"git_commit,omitempty"`
	DVCStage  string   `json:"dvc_stage,omitempty"`
}

// Job represents a single restore operation, including its Glacier state and
// the information needed to resume or download it later.
type Job struct {
	// ID is a random 8-hex-byte identifier, e.g. "a3f2c1d4e5b6a7f8".
	ID string `json:"id"`
	// Status is the current lifecycle state.
	Status JobStatus `json:"status"`
	// S3URL is the original source URL (e.g. "s3://bucket/uploads/abc123").
	S3URL string `json:"s3_url"`
	// OutputDir is the local destination directory.
	OutputDir string `json:"output_dir"`
	// Region is the AWS region of the bucket.
	Region string `json:"region"`
	// Selection records what was targeted in the manifest.
	Selection SelectionCriteria `json:"selection"`
	// Tier is the Glacier retrieval tier used.
	Tier string `json:"tier"`
	// RestoreDays is how long the restored copy will remain available.
	RestoreDays int32 `json:"restore_days"`
	// ChunkKeys are the S3 object keys that needed Glacier restoration.
	ChunkKeys []string `json:"chunk_keys"`
	// Bucket is the S3 bucket (parsed from S3URL for convenience).
	Bucket string `json:"bucket"`
	// EstimatedCostUSD is the approximate retrieval fee.
	EstimatedCostUSD float64 `json:"estimated_cost_usd,omitempty"`
	// CreatedAt is when the restore request was submitted.
	CreatedAt time.Time `json:"created_at"`
	// ReadyAt is when all chunks became accessible (set on transition to Ready).
	ReadyAt *time.Time `json:"ready_at,omitempty"`
	// CompletedAt is when the download finished.
	CompletedAt *time.Time `json:"completed_at,omitempty"`
	// Error holds the last error message if Status == JobStatusFailed.
	Error string `json:"error,omitempty"`
	// FilesRestored is populated after a successful download.
	FilesRestored int64 `json:"files_restored,omitempty"`
	// BytesWritten is populated after a successful download.
	BytesWritten int64 `json:"bytes_written,omitempty"`
}

// Store manages restore job persistence in a directory on disk.
type Store struct {
	dir string
}

// DefaultStoreDir returns the default job store directory:
// $XDG_DATA_HOME/cargoship/restore-jobs or ~/.cargoship/restore-jobs.
func DefaultStoreDir() (string, error) {
	base := os.Getenv("XDG_DATA_HOME")
	if base == "" {
		home, err := os.UserHomeDir()
		if err != nil {
			return "", fmt.Errorf("get home dir: %w", err)
		}
		base = filepath.Join(home, ".cargoship")
	}
	return filepath.Join(base, "restore-jobs"), nil
}

// NewStore creates a Store rooted at dir, creating the directory if needed.
func NewStore(dir string) (*Store, error) {
	if err := os.MkdirAll(dir, 0700); err != nil {
		return nil, fmt.Errorf("create job store dir %q: %w", dir, err)
	}
	return &Store{dir: dir}, nil
}

// NewDefaultStore creates a Store at the default directory.
func NewDefaultStore() (*Store, error) {
	dir, err := DefaultStoreDir()
	if err != nil {
		return nil, err
	}
	return NewStore(dir)
}

// Save writes a job to disk, creating or overwriting the job file.
func (s *Store) Save(job *Job) error {
	data, err := json.MarshalIndent(job, "", "  ")
	if err != nil {
		return fmt.Errorf("marshal job %s: %w", job.ID, err)
	}
	path := s.jobPath(job.ID)
	if err := os.WriteFile(path, data, 0600); err != nil {
		return fmt.Errorf("write job %s: %w", job.ID, err)
	}
	return nil
}

// Load reads a single job by ID.
func (s *Store) Load(id string) (*Job, error) {
	data, err := os.ReadFile(s.jobPath(id))
	if err != nil {
		if os.IsNotExist(err) {
			return nil, fmt.Errorf("no restore job with ID %q", id)
		}
		return nil, fmt.Errorf("read job %s: %w", id, err)
	}
	var job Job
	if err := json.Unmarshal(data, &job); err != nil {
		return nil, fmt.Errorf("parse job %s: %w", id, err)
	}
	return &job, nil
}

// List returns all jobs, sorted by creation time (oldest first).
func (s *Store) List() ([]*Job, error) {
	entries, err := os.ReadDir(s.dir)
	if err != nil {
		if os.IsNotExist(err) {
			return nil, nil
		}
		return nil, fmt.Errorf("read job store: %w", err)
	}
	var jobs []*Job
	for _, e := range entries {
		if e.IsDir() || filepath.Ext(e.Name()) != ".json" {
			continue
		}
		id := e.Name()[:len(e.Name())-5] // strip .json
		job, err := s.Load(id)
		if err != nil {
			continue // skip corrupt files silently
		}
		jobs = append(jobs, job)
	}
	sort.Slice(jobs, func(i, j int) bool {
		return jobs[i].CreatedAt.Before(jobs[j].CreatedAt)
	})
	return jobs, nil
}

// Delete removes a job file from disk.
func (s *Store) Delete(id string) error {
	err := os.Remove(s.jobPath(id))
	if err != nil && !os.IsNotExist(err) {
		return fmt.Errorf("delete job %s: %w", id, err)
	}
	return nil
}

// CleanCompleted removes all jobs with status Complete or Failed older than age.
func (s *Store) CleanCompleted(age time.Duration) (int, error) {
	jobs, err := s.List()
	if err != nil {
		return 0, err
	}
	cutoff := time.Now().Add(-age)
	removed := 0
	for _, j := range jobs {
		if (j.Status == JobStatusComplete || j.Status == JobStatusFailed) &&
			j.CreatedAt.Before(cutoff) {
			if err := s.Delete(j.ID); err == nil {
				removed++
			}
		}
	}
	return removed, nil
}

// jobPath returns the file path for job with the given ID.
func (s *Store) jobPath(id string) string {
	return filepath.Join(s.dir, id+".json")
}

// NewJob creates a new Job with a random ID and the current time, saving it to
// the store. The caller should populate all fields before calling NewJob.
func (s *Store) NewJob(
	s3URL, outputDir, region, tier string,
	restoreDays int32,
	bucket string,
	chunkKeys []string,
	selection SelectionCriteria,
	estimatedCostUSD float64,
) (*Job, error) {
	id, err := generateID()
	if err != nil {
		return nil, fmt.Errorf("generate job ID: %w", err)
	}
	job := &Job{
		ID:               id,
		Status:           JobStatusPendingGlacier,
		S3URL:            s3URL,
		OutputDir:        outputDir,
		Region:           region,
		Tier:             tier,
		RestoreDays:      restoreDays,
		Bucket:           bucket,
		ChunkKeys:        chunkKeys,
		Selection:        selection,
		EstimatedCostUSD: estimatedCostUSD,
		CreatedAt:        time.Now(),
	}
	if err := s.Save(job); err != nil {
		return nil, err
	}
	return job, nil
}

// generateID returns a random 8-byte hex string.
func generateID() (string, error) {
	b := make([]byte, 8)
	if _, err := rand.Read(b); err != nil {
		return "", err
	}
	return hex.EncodeToString(b), nil
}
