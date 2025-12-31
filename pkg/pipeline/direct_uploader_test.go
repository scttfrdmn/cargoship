package pipeline

import (
	"context"
	"os"
	"path/filepath"
	"sync"
	"testing"
	"time"

	"github.com/aws/aws-sdk-go-v2/service/s3"
	"github.com/scttfrdmn/cargoship/pkg/chunking"
)

// Mock S3 client for testing
type mockS3Client struct {
	mu             sync.Mutex
	putObjectCalls int
	failAfter      int // Fail after N calls (for retry testing)
}

func (m *mockS3Client) PutObject(ctx context.Context, input *s3.PutObjectInput, opts ...func(*s3.Options)) (*s3.PutObjectOutput, error) {
	m.mu.Lock()
	m.putObjectCalls++
	calls := m.putObjectCalls
	m.mu.Unlock()

	if m.failAfter > 0 && calls <= m.failAfter {
		return nil, &mockError{message: "simulated upload failure"}
	}
	return &s3.PutObjectOutput{}, nil
}

// Multipart upload methods (not used by DirectUploaderStage but required by interface)
func (m *mockS3Client) CreateMultipartUpload(ctx context.Context, input *s3.CreateMultipartUploadInput, opts ...func(*s3.Options)) (*s3.CreateMultipartUploadOutput, error) {
	return &s3.CreateMultipartUploadOutput{}, nil
}

func (m *mockS3Client) UploadPart(ctx context.Context, input *s3.UploadPartInput, opts ...func(*s3.Options)) (*s3.UploadPartOutput, error) {
	return &s3.UploadPartOutput{}, nil
}

func (m *mockS3Client) CompleteMultipartUpload(ctx context.Context, input *s3.CompleteMultipartUploadInput, opts ...func(*s3.Options)) (*s3.CompleteMultipartUploadOutput, error) {
	return &s3.CompleteMultipartUploadOutput{}, nil
}

func (m *mockS3Client) AbortMultipartUpload(ctx context.Context, input *s3.AbortMultipartUploadInput, opts ...func(*s3.Options)) (*s3.AbortMultipartUploadOutput, error) {
	return &s3.AbortMultipartUploadOutput{}, nil
}

func (m *mockS3Client) GetPutObjectCalls() int {
	m.mu.Lock()
	defer m.mu.Unlock()
	return m.putObjectCalls
}

type mockError struct {
	message string
}

func (e *mockError) Error() string {
	return e.message
}

func TestDirectUploaderStage_BasicUpload(t *testing.T) {
	ctx := context.Background()

	// Create temporary test file
	tmpDir := t.TempDir()
	testFile := filepath.Join(tmpDir, "test.txt")
	content := []byte("test content")
	if err := os.WriteFile(testFile, content, 0644); err != nil {
		t.Fatalf("Failed to create test file: %v", err)
	}

	// Create mock S3 client
	mockClient := &mockS3Client{}

	// Create channels
	input := make(chan *Job, 1)
	output := make(chan *Job, 1)

	// Create direct uploader with adaptive disabled to avoid goroutine leaks
	config := &DirectUploaderConfig{
		S3Client:   mockClient,
		Bucket:     "test-bucket",
		Prefix:     "test-prefix",
		Workers:    4,
		MaxRetries: 3,
		RetryDelay: 10 * time.Millisecond,
	}

	// Create worker pool with adaptive disabled
	poolConfig := &AdaptiveWorkerPoolConfig{
		InitialWorkers: 4,
		MaxWorkers:     4,
		EnableAdaptive: false, // Disable adaptive to avoid goroutine leak
	}
	config.WorkerPool = NewAdaptiveWorkerPool(ctx, poolConfig)

	uploader, err := NewDirectUploaderStage(config, input, output)
	if err != nil {
		t.Fatalf("Failed to create direct uploader: %v", err)
	}

	// Start uploader
	if err := uploader.Start(ctx); err != nil {
		t.Fatalf("Failed to start uploader: %v", err)
	}

	// Send job with test file
	job := &Job{
		ID: 1,
		Chunk: chunking.Chunk{
			ID: 1,
			Files: []chunking.File{
				{
					Path: testFile,
					Size: int64(len(content)),
				},
			},
			TotalSize: int64(len(content)),
		},
	}

	input <- job
	close(input)

	// Wait for completion
	time.Sleep(100 * time.Millisecond)
	_ = uploader.Stop()

	// Verify upload was called
	if mockClient.GetPutObjectCalls() != 1 {
		t.Errorf("Expected 1 PutObject call, got %d", mockClient.GetPutObjectCalls())
	}

	// Verify stats
	stats := uploader.Stats()
	if stats.JobsProcessed != 1 {
		t.Errorf("Expected 1 job processed, got %d", stats.JobsProcessed)
	}

	if uploader.GetUploadedFiles() != 1 {
		t.Errorf("Expected 1 file uploaded, got %d", uploader.GetUploadedFiles())
	}

	if uploader.GetUploadedBytes() != int64(len(content)) {
		t.Errorf("Expected %d bytes uploaded, got %d", len(content), uploader.GetUploadedBytes())
	}
}

func TestDirectUploaderStage_MultipleFiles(t *testing.T) {
	ctx := context.Background()

	// Create temporary test files
	tmpDir := t.TempDir()
	const numFiles = 10
	var testFiles []chunking.File
	var totalSize int64

	for i := 0; i < numFiles; i++ {
		testFile := filepath.Join(tmpDir, filepath.Base(t.Name())+"-"+string(rune('0'+i))+".txt")
		content := []byte("test content " + string(rune('0'+i)))
		if err := os.WriteFile(testFile, content, 0644); err != nil {
			t.Fatalf("Failed to create test file %d: %v", i, err)
		}
		testFiles = append(testFiles, chunking.File{
			Path: testFile,
			Size: int64(len(content)),
		})
		totalSize += int64(len(content))
	}

	// Create mock S3 client
	mockClient := &mockS3Client{}

	// Create channels
	input := make(chan *Job, 1)
	output := make(chan *Job, 10)

	// Create direct uploader with adaptive disabled to avoid goroutine leaks
	config := &DirectUploaderConfig{
		S3Client:   mockClient,
		Bucket:     "test-bucket",
		Workers:    4,
		MaxRetries: 3,
	}

	// Create worker pool with adaptive disabled
	poolConfig := &AdaptiveWorkerPoolConfig{
		InitialWorkers: 4,
		MaxWorkers:     4,
		EnableAdaptive: false, // Disable adaptive to avoid goroutine leak
	}
	config.WorkerPool = NewAdaptiveWorkerPool(ctx, poolConfig)

	uploader, err := NewDirectUploaderStage(config, input, output)
	if err != nil {
		t.Fatalf("Failed to create direct uploader: %v", err)
	}

	// Start uploader
	if err := uploader.Start(ctx); err != nil {
		t.Fatalf("Failed to start uploader: %v", err)
	}

	// Send job with multiple files
	job := &Job{
		ID: 1,
		Chunk: chunking.Chunk{
			ID:        1,
			Files:     testFiles,
			TotalSize: totalSize,
		},
	}

	input <- job
	close(input)

	// Wait for completion via output channel
	select {
	case <-output:
		// Job completed successfully
	case <-time.After(2 * time.Second):
		t.Fatal("Timeout waiting for upload completion")
	}
	_ = uploader.Stop()

	// Verify all files were uploaded
	if mockClient.GetPutObjectCalls() != numFiles {
		t.Errorf("Expected %d PutObject calls, got %d", numFiles, mockClient.GetPutObjectCalls())
	}

	if uploader.GetUploadedFiles() != numFiles {
		t.Errorf("Expected %d files uploaded, got %d", numFiles, uploader.GetUploadedFiles())
	}

	if uploader.GetUploadedBytes() != totalSize {
		t.Errorf("Expected %d bytes uploaded, got %d", totalSize, uploader.GetUploadedBytes())
	}
}

func TestDirectUploaderStage_RetryLogic(t *testing.T) {
	ctx := context.Background()

	// Create temporary test file
	tmpDir := t.TempDir()
	testFile := filepath.Join(tmpDir, "test.txt")
	content := []byte("test content")
	if err := os.WriteFile(testFile, content, 0644); err != nil {
		t.Fatalf("Failed to create test file: %v", err)
	}

	// Create mock S3 client that fails first 2 attempts
	mockClient := &mockS3Client{
		failAfter: 2, // Fail first 2 attempts, succeed on 3rd
	}

	// Create channels
	input := make(chan *Job, 1)
	output := make(chan *Job, 1)

	// Create direct uploader with retries and adaptive disabled to avoid goroutine leaks
	config := &DirectUploaderConfig{
		S3Client:   mockClient,
		Bucket:     "test-bucket",
		Workers:    1,
		MaxRetries: 3,
		RetryDelay: 10 * time.Millisecond,
	}

	// Create worker pool with adaptive disabled
	poolConfig := &AdaptiveWorkerPoolConfig{
		InitialWorkers: 1,
		MaxWorkers:     1,
		EnableAdaptive: false, // Disable adaptive to avoid goroutine leak
	}
	config.WorkerPool = NewAdaptiveWorkerPool(ctx, poolConfig)

	uploader, err := NewDirectUploaderStage(config, input, output)
	if err != nil {
		t.Fatalf("Failed to create direct uploader: %v", err)
	}

	// Start uploader
	if err := uploader.Start(ctx); err != nil {
		t.Fatalf("Failed to start uploader: %v", err)
	}

	// Send job
	job := &Job{
		ID: 1,
		Chunk: chunking.Chunk{
			ID: 1,
			Files: []chunking.File{
				{
					Path: testFile,
					Size: int64(len(content)),
				},
			},
		},
	}

	input <- job
	close(input)

	// Wait for completion
	time.Sleep(200 * time.Millisecond)
	_ = uploader.Stop()

	// Verify upload succeeded after retries
	if mockClient.GetPutObjectCalls() != 3 {
		t.Errorf("Expected 3 PutObject calls (2 failures + 1 success), got %d", mockClient.GetPutObjectCalls())
	}

	if uploader.GetUploadedFiles() != 1 {
		t.Errorf("Expected 1 file uploaded after retries, got %d", uploader.GetUploadedFiles())
	}
}

func TestDirectUploaderStage_Configuration(t *testing.T) {
	ctx := context.Background()

	// Test nil config
	_, err := NewDirectUploaderStage(nil, nil, nil)
	if err == nil {
		t.Error("Expected error for nil config")
	}

	// Test nil S3 client
	config := &DirectUploaderConfig{
		Bucket: "test-bucket",
	}
	_, err = NewDirectUploaderStage(config, nil, nil)
	if err == nil {
		t.Error("Expected error for nil S3 client")
	}

	// Test empty bucket
	mockClient := &mockS3Client{}
	config = &DirectUploaderConfig{
		S3Client: mockClient,
	}
	_, err = NewDirectUploaderStage(config, nil, nil)
	if err == nil {
		t.Error("Expected error for empty bucket")
	}

	// Test defaults
	config = &DirectUploaderConfig{
		S3Client: mockClient,
		Bucket:   "test-bucket",
	}

	// Create worker pool with adaptive disabled to avoid goroutine leak
	poolConfig := &AdaptiveWorkerPoolConfig{
		InitialWorkers: 256,
		MaxWorkers:     256,
		EnableAdaptive: false,
	}
	config.WorkerPool = NewAdaptiveWorkerPool(ctx, poolConfig)

	input := make(chan *Job)
	output := make(chan *Job)

	uploader, err := NewDirectUploaderStage(config, input, output)
	if err != nil {
		t.Fatalf("Failed to create uploader: %v", err)
	}
	defer func() {
		_ = uploader.Stop()
	}()

	if uploader.config.Workers != 256 {
		t.Errorf("Expected default workers 256, got %d", uploader.config.Workers)
	}

	if uploader.config.MaxRetries != 3 {
		t.Errorf("Expected default max retries 3, got %d", uploader.config.MaxRetries)
	}

	if uploader.config.RetryDelay != time.Second {
		t.Errorf("Expected default retry delay 1s, got %v", uploader.config.RetryDelay)
	}
}

func TestDirectUploaderStage_AdaptiveWorkerPool(t *testing.T) {
	ctx := context.Background()

	// Create mock S3 client
	mockClient := &mockS3Client{}

	// Create adaptive worker pool
	poolConfig := &AdaptiveWorkerPoolConfig{
		InitialWorkers: 2,
		MaxWorkers:     16,
		EnableAdaptive: true,
	}
	pool := NewAdaptiveWorkerPool(ctx, poolConfig)

	// Create channels
	input := make(chan *Job, 1)
	output := make(chan *Job, 1)

	// Create direct uploader with custom pool
	config := &DirectUploaderConfig{
		S3Client:   mockClient,
		Bucket:     "test-bucket",
		WorkerPool: pool,
	}

	uploader, err := NewDirectUploaderStage(config, input, output)
	if err != nil {
		t.Fatalf("Failed to create direct uploader: %v", err)
	}

	// Verify pool is used
	if uploader.pool != pool {
		t.Error("Expected custom worker pool to be used")
	}

	// Start uploader
	if err := uploader.Start(ctx); err != nil {
		t.Fatalf("Failed to start uploader: %v", err)
	}

	close(input)
	_ = uploader.Stop()

	// Stop the custom pool to avoid goroutine leak
	pool.Stop()
}

func TestDirectUploaderStage_BuildS3Key(t *testing.T) {
	ctx := context.Background()
	mockClient := &mockS3Client{}

	// Create worker pool with adaptive disabled to avoid goroutine leak
	poolConfig := &AdaptiveWorkerPoolConfig{
		InitialWorkers: 4,
		MaxWorkers:     4,
		EnableAdaptive: false,
	}

	// Test without prefix
	config := &DirectUploaderConfig{
		S3Client:   mockClient,
		Bucket:     "test-bucket",
		WorkerPool: NewAdaptiveWorkerPool(ctx, poolConfig),
	}

	input := make(chan *Job)
	output := make(chan *Job)

	uploader, err := NewDirectUploaderStage(config, input, output)
	if err != nil {
		t.Fatalf("Failed to create uploader: %v", err)
	}
	defer func() {
		_ = uploader.Stop()
	}()

	key := uploader.buildS3Key("/path/to/file.txt")
	expected := "file.txt"
	if key != expected {
		t.Errorf("Expected key %s, got %s", expected, key)
	}

	// Test with prefix
	config.Prefix = "my-prefix"
	config.WorkerPool = NewAdaptiveWorkerPool(ctx, poolConfig)
	uploader2, err := NewDirectUploaderStage(config, input, output)
	if err != nil {
		t.Fatalf("Failed to create uploader: %v", err)
	}
	defer func() {
		_ = uploader2.Stop()
	}()

	key = uploader2.buildS3Key("/path/to/file.txt")
	expected = "my-prefix/file.txt"
	if key != expected {
		t.Errorf("Expected key %s, got %s", expected, key)
	}
}

func BenchmarkDirectUploaderStage_SingleFile(b *testing.B) {
	ctx := context.Background()

	// Create temporary test file
	tmpDir := b.TempDir()
	testFile := filepath.Join(tmpDir, "test.txt")
	content := make([]byte, 1024*1024) // 1MB
	if err := os.WriteFile(testFile, content, 0644); err != nil {
		b.Fatalf("Failed to create test file: %v", err)
	}

	// Create mock S3 client
	mockClient := &mockS3Client{}

	b.ResetTimer()

	for i := 0; i < b.N; i++ {
		input := make(chan *Job, 1)
		output := make(chan *Job, 1)

		config := &DirectUploaderConfig{
			S3Client:   mockClient,
			Bucket:     "test-bucket",
			Workers:    256,
			MaxRetries: 1,
		}

		uploader, _ := NewDirectUploaderStage(config, input, output)
		_ = uploader.Start(ctx)

		job := &Job{
			ID: 1,
			Chunk: chunking.Chunk{
				Files: []chunking.File{{Path: testFile, Size: int64(len(content))}},
			},
		}

		input <- job
		close(input)

		time.Sleep(10 * time.Millisecond)
		_ = uploader.Stop()
	}
}

func BenchmarkDirectUploaderStage_MultipleFiles(b *testing.B) {
	ctx := context.Background()

	// Create temporary test files
	tmpDir := b.TempDir()
	const numFiles = 100
	var testFiles []chunking.File

	for i := 0; i < numFiles; i++ {
		testFile := filepath.Join(tmpDir, "test-"+string(rune('0'+i%10))+".txt")
		content := make([]byte, 10*1024) // 10KB each
		if err := os.WriteFile(testFile, content, 0644); err != nil {
			b.Fatalf("Failed to create test file: %v", err)
		}
		testFiles = append(testFiles, chunking.File{
			Path: testFile,
			Size: int64(len(content)),
		})
	}

	// Create mock S3 client
	mockClient := &mockS3Client{}

	b.ResetTimer()

	for i := 0; i < b.N; i++ {
		input := make(chan *Job, 1)
		output := make(chan *Job, 100)

		config := &DirectUploaderConfig{
			S3Client:   mockClient,
			Bucket:     "test-bucket",
			Workers:    256,
			MaxRetries: 1,
		}

		uploader, _ := NewDirectUploaderStage(config, input, output)
		_ = uploader.Start(ctx)

		job := &Job{
			ID:    1,
			Chunk: chunking.Chunk{Files: testFiles},
		}

		input <- job
		close(input)

		time.Sleep(100 * time.Millisecond)
		_ = uploader.Stop()
	}
}
