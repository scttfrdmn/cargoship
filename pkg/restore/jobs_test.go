package restore

import (
	"os"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func newTestStore(t *testing.T) *Store {
	t.Helper()
	dir := t.TempDir()
	s, err := NewStore(dir)
	require.NoError(t, err)
	return s
}

func TestNewJob(t *testing.T) {
	s := newTestStore(t)

	job, err := s.NewJob(
		"s3://bucket/uploads/abc",
		"/tmp/out",
		"us-east-1",
		"standard",
		7,
		"bucket",
		[]string{"shard-0/chunk-0.tar.zst"},
		SelectionCriteria{DVCStage: "train"},
		0.042,
	)
	require.NoError(t, err)
	assert.NotEmpty(t, job.ID)
	assert.Equal(t, JobStatusPendingGlacier, job.Status)
	assert.Equal(t, "s3://bucket/uploads/abc", job.S3URL)
	assert.Equal(t, "train", job.Selection.DVCStage)
	assert.InDelta(t, 0.042, job.EstimatedCostUSD, 0.001)
}

func TestSaveAndLoad(t *testing.T) {
	s := newTestStore(t)

	job, err := s.NewJob("s3://b/p", "/out", "us-west-2", "bulk", 3, "b",
		[]string{"key1", "key2"}, SelectionCriteria{GitCommit: "abc123"}, 0)
	require.NoError(t, err)

	loaded, err := s.Load(job.ID)
	require.NoError(t, err)
	assert.Equal(t, job.ID, loaded.ID)
	assert.Equal(t, job.Status, loaded.Status)
	assert.Equal(t, "abc123", loaded.Selection.GitCommit)
	assert.Len(t, loaded.ChunkKeys, 2)
}

func TestLoadNotFound(t *testing.T) {
	s := newTestStore(t)
	_, err := s.Load("doesnotexist")
	assert.Error(t, err)
}

func TestList(t *testing.T) {
	s := newTestStore(t)

	// Empty store.
	jobs, err := s.List()
	require.NoError(t, err)
	assert.Empty(t, jobs)

	// Add a couple of jobs.
	for range 3 {
		_, err := s.NewJob("s3://b/p", "/out", "us-east-1", "standard", 7, "b", nil, SelectionCriteria{}, 0)
		require.NoError(t, err)
	}

	jobs, err = s.List()
	require.NoError(t, err)
	assert.Len(t, jobs, 3)
}

func TestListSortedByCreatedAt(t *testing.T) {
	s := newTestStore(t)

	j1, _ := s.NewJob("s3://b/p1", "/out", "us-east-1", "standard", 7, "b", nil, SelectionCriteria{}, 0)
	j2, _ := s.NewJob("s3://b/p2", "/out", "us-east-1", "standard", 7, "b", nil, SelectionCriteria{}, 0)
	j3, _ := s.NewJob("s3://b/p3", "/out", "us-east-1", "standard", 7, "b", nil, SelectionCriteria{}, 0)

	// Manually set timestamps to guarantee ordering.
	now := time.Now()
	j1.CreatedAt = now.Add(-2 * time.Hour)
	j2.CreatedAt = now.Add(-1 * time.Hour)
	j3.CreatedAt = now
	_ = s.Save(j1)
	_ = s.Save(j2)
	_ = s.Save(j3)

	jobs, err := s.List()
	require.NoError(t, err)
	require.Len(t, jobs, 3)
	assert.Equal(t, j1.ID, jobs[0].ID)
	assert.Equal(t, j2.ID, jobs[1].ID)
	assert.Equal(t, j3.ID, jobs[2].ID)
}

func TestDelete(t *testing.T) {
	s := newTestStore(t)

	job, _ := s.NewJob("s3://b/p", "/out", "us-east-1", "standard", 7, "b", nil, SelectionCriteria{}, 0)
	err := s.Delete(job.ID)
	require.NoError(t, err)

	_, err = s.Load(job.ID)
	assert.Error(t, err)

	// Deleting again should not error.
	assert.NoError(t, s.Delete(job.ID))
}

func TestCleanCompleted(t *testing.T) {
	s := newTestStore(t)

	// Old completed job.
	old, _ := s.NewJob("s3://b/p", "/out", "us-east-1", "standard", 7, "b", nil, SelectionCriteria{}, 0)
	old.Status = JobStatusComplete
	old.CreatedAt = time.Now().Add(-25 * time.Hour)
	_ = s.Save(old)

	// Recent complete job.
	recent, _ := s.NewJob("s3://b/p2", "/out", "us-east-1", "standard", 7, "b", nil, SelectionCriteria{}, 0)
	recent.Status = JobStatusComplete
	recent.CreatedAt = time.Now().Add(-1 * time.Hour)
	_ = s.Save(recent)

	// Pending job — should never be cleaned.
	pending, _ := s.NewJob("s3://b/p3", "/out", "us-east-1", "standard", 7, "b", nil, SelectionCriteria{}, 0)
	pending.Status = JobStatusPendingGlacier
	pending.CreatedAt = time.Now().Add(-48 * time.Hour)
	_ = s.Save(pending)

	removed, err := s.CleanCompleted(24 * time.Hour)
	require.NoError(t, err)
	assert.Equal(t, 1, removed)

	jobs, _ := s.List()
	assert.Len(t, jobs, 2) // recent complete + pending remain
}

func TestSave_UpdatesStatus(t *testing.T) {
	s := newTestStore(t)

	job, _ := s.NewJob("s3://b/p", "/out", "us-east-1", "standard", 7, "b",
		[]string{"chunk.tar.zst"}, SelectionCriteria{}, 0)
	assert.Equal(t, JobStatusPendingGlacier, job.Status)

	now := time.Now()
	job.Status = JobStatusReady
	job.ReadyAt = &now
	require.NoError(t, s.Save(job))

	loaded, _ := s.Load(job.ID)
	assert.Equal(t, JobStatusReady, loaded.Status)
	assert.NotNil(t, loaded.ReadyAt)
}

func TestGenerateID_Unique(t *testing.T) {
	seen := make(map[string]bool)
	for range 100 {
		id, err := generateID()
		require.NoError(t, err)
		assert.Len(t, id, 16) // 8 bytes = 16 hex chars
		assert.False(t, seen[id], "duplicate ID: %s", id)
		seen[id] = true
	}
}

func TestDefaultStoreDir(t *testing.T) {
	// Should not error when home dir is available.
	dir, err := DefaultStoreDir()
	require.NoError(t, err)
	assert.NotEmpty(t, dir)
	assert.Contains(t, dir, "restore-jobs")
}

func TestDefaultStoreDir_XDGOverride(t *testing.T) {
	t.Setenv("XDG_DATA_HOME", t.TempDir())
	dir, err := DefaultStoreDir()
	require.NoError(t, err)
	assert.Contains(t, dir, "restore-jobs")
	assert.Contains(t, dir, os.Getenv("XDG_DATA_HOME"))
}

func TestNewDefaultStore(t *testing.T) {
	t.Setenv("XDG_DATA_HOME", t.TempDir())
	store, err := NewDefaultStore()
	require.NoError(t, err)
	assert.NotNil(t, store)

	// Verify the directory was created.
	dir, _ := DefaultStoreDir()
	_, statErr := os.Stat(dir)
	assert.NoError(t, statErr)
}
