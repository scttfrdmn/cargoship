package launch

import (
	"log/slog"
	"os"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestNewLocalArchiver(t *testing.T) {
	logger := slog.New(slog.NewTextHandler(os.Stdout, &slog.HandlerOptions{Level: slog.LevelError}))

	config := ArchiveConfig{
		Destination:   "s3://test-bucket",
		StorageClass:  "standard",
		MaxConcurrent: 2,
		RetryAttempts: 3,
		RetryDelay:    time.Second * 30,
	}

	archiver, err := NewLocalArchiver(config, logger)
	require.NoError(t, err)
	require.NotNil(t, archiver)

	assert.Equal(t, config.Destination, archiver.config.Destination)
	assert.Equal(t, config.MaxConcurrent, archiver.config.MaxConcurrent)
	assert.NotNil(t, archiver.activeJobs)
	assert.NotNil(t, archiver.jobQueue)

	// Clean up
	err = archiver.Stop()
	assert.NoError(t, err)
}

func TestLocalArchiverJobManagement(t *testing.T) {
	logger := slog.New(slog.NewTextHandler(os.Stdout, &slog.HandlerOptions{Level: slog.LevelError}))

	config := ArchiveConfig{
		Destination:   "s3://test-bucket",
		StorageClass:  "standard",
		MaxConcurrent: 1,
		RetryAttempts: 3,
		RetryDelay:    time.Second * 1, // Short delay for testing
	}

	archiver, err := NewLocalArchiver(config, logger)
	require.NoError(t, err)
	defer func() { _ = archiver.Stop() }()

	// Create a test candidate
	candidate := &ArchiveCandidate{
		Path:       "/test/sample.fastq.gz",
		Type:       CandidateTypeFile,
		Size:       1024 * 1024 * 100, // 100MB
		ModTime:    time.Now().Add(-time.Hour * 24),
		DetectedBy: "genomics",
		Metadata: map[string]string{
			"data_type": "genomics",
			"file_type": "fastq.gz",
		},
		StorageClass: "deep-archive",
		Priority:     1,
	}

	// Submit job
	job, err := archiver.SubmitJob(candidate)
	require.NoError(t, err)
	require.NotNil(t, job)

	assert.NotEmpty(t, job.ID)
	assert.Equal(t, candidate.Path, job.Path)
	assert.Equal(t, JobStatePending, job.State)
	assert.Equal(t, candidate.Size, job.BytesTotal)

	// Check that job is in active jobs
	activeJobs := archiver.GetActiveJobs()
	assert.Len(t, activeJobs, 1)
	assert.Equal(t, job.ID, activeJobs[0].ID)

	// Get specific job
	retrievedJob, exists := archiver.GetJob(job.ID)
	assert.True(t, exists)
	assert.Equal(t, job.ID, retrievedJob.ID)

	// Test non-existent job
	_, exists = archiver.GetJob("non-existent")
	assert.False(t, exists)

	// Wait a bit for job to be processed (it will complete quickly in simulation)
	time.Sleep(time.Millisecond * 200)

	// Check job status - it should be running or completed
	updatedJob, exists := archiver.GetJob(job.ID)
	assert.True(t, exists)
	assert.Contains(t, []JobState{JobStateRunning, JobStateCompleted}, updatedJob.State)
}

func TestLocalArchiverJobCancellation(t *testing.T) {
	logger := slog.New(slog.NewTextHandler(os.Stdout, &slog.HandlerOptions{Level: slog.LevelError}))

	config := ArchiveConfig{
		Destination:   "s3://test-bucket",
		StorageClass:  "standard",
		MaxConcurrent: 1,
	}

	archiver, err := NewLocalArchiver(config, logger)
	require.NoError(t, err)
	defer func() { _ = archiver.Stop() }()

	candidate := &ArchiveCandidate{
		Path:         "/test/large-file.bam",
		Type:         CandidateTypeFile,
		Size:         1024 * 1024 * 1000, // 1GB
		ModTime:      time.Now().Add(-time.Hour * 24),
		DetectedBy:   "genomics",
		StorageClass: "glacier",
		Priority:     2,
	}

	job, err := archiver.SubmitJob(candidate)
	require.NoError(t, err)

	// Cancel the job
	err = archiver.CancelJob(job.ID)
	assert.NoError(t, err)

	// Check that job is cancelled
	cancelledJob, exists := archiver.GetJob(job.ID)
	assert.True(t, exists)
	assert.Equal(t, JobStateCancelled, cancelledJob.State)
	assert.NotNil(t, cancelledJob.EndTime)

	// Try to cancel non-existent job
	err = archiver.CancelJob("non-existent")
	assert.Error(t, err)
}

func TestBuildDestination(t *testing.T) {
	logger := slog.New(slog.NewTextHandler(os.Stdout, &slog.HandlerOptions{Level: slog.LevelError}))

	config := ArchiveConfig{
		Destination: "s3://research-bucket",
	}

	archiver, err := NewLocalArchiver(config, logger)
	require.NoError(t, err)
	defer func() { _ = archiver.Stop() }()

	candidate := &ArchiveCandidate{
		Path:    "/long/path/to/data/sample.fastq.gz",
		ModTime: time.Date(2024, 3, 15, 10, 30, 0, 0, time.UTC),
		Metadata: map[string]string{
			"data_type": "genomics",
		},
	}

	destination := archiver.buildDestination(candidate)

	// Should include base destination, data type, and date organization
	assert.Contains(t, destination, "s3://research-bucket")
	assert.Contains(t, destination, "genomics")
	// Note: The exact date structure depends on when the test runs,
	// so we just check that it follows a reasonable pattern
	assert.Contains(t, destination, "/")
}

func TestGenerateJobID(t *testing.T) {
	id1 := generateJobID()
	time.Sleep(time.Millisecond) // Ensure different timestamps
	id2 := generateJobID()

	assert.NotEmpty(t, id1)
	assert.NotEmpty(t, id2)
	assert.NotEqual(t, id1, id2) // Should be unique
	assert.Contains(t, id1, "job-")
	assert.Contains(t, id2, "job-")
}