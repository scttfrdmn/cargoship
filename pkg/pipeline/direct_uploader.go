// Package pipeline provides streaming pipeline for CargoShip v0.5.0
package pipeline

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"hash"
	"io"
	"os"
	"path"
	"path/filepath"
	"strconv"
	"strings"
	"sync"
	"sync/atomic"
	"time"

	"github.com/aws/aws-sdk-go-v2/aws"
	"github.com/aws/aws-sdk-go-v2/service/s3"
	"github.com/aws/aws-sdk-go-v2/service/s3/types"
	"github.com/scttfrdmn/cargoship/pkg/chunking"
)

// DirectFilesSegment is the key segment direct-mode objects live under:
// <prefix>/uploads/<upload-id>/files/<relative path> (#1020). It keeps source
// files out of the namespace of CargoShip's own objects (manifest*.json*).
const DirectFilesSegment = "files"

// RequireChunkedUpload makes cfg's upload pack every file into chunks: direct
// mode is never chosen, automatically or otherwise. Direct mode stores each
// file as its own object keyed by its path, so an --encrypt-manifest upload,
// which exists to hide file names, must not use it (#1037). The automatic
// choice requires fileCount <= DirectUploadMaxFiles, which no count meets when
// the limit is negative.
func RequireChunkedUpload(cfg *PipelineConfig) {
	cfg.EnableDirectUpload = false
	cfg.ForceDirectUpload = false
	cfg.DirectUploadMaxFiles = -1
}

// directManifestBuilder is the interface satisfied by *manifest.Builder for direct upload mode.
// Using a minimal interface keeps direct_uploader.go decoupled from the concrete type.
type directManifestBuilder interface {
	// RecordDirectFile records a stored file's key and the checksum, size and
	// modification time of the bytes sent (#1015).
	RecordDirectFile(path string, shardID int, s3Key, checksum string, size int64, modTime time.Time)
}

// S3Uploader is an interface for uploading to S3 (allows mocking)
type S3Uploader interface {
	PutObject(ctx context.Context, input *s3.PutObjectInput, opts ...func(*s3.Options)) (*s3.PutObjectOutput, error)

	// Multipart upload methods (required for streaming large objects)
	CreateMultipartUpload(ctx context.Context, input *s3.CreateMultipartUploadInput, opts ...func(*s3.Options)) (*s3.CreateMultipartUploadOutput, error)
	UploadPart(ctx context.Context, input *s3.UploadPartInput, opts ...func(*s3.Options)) (*s3.UploadPartOutput, error)
	CompleteMultipartUpload(ctx context.Context, input *s3.CompleteMultipartUploadInput, opts ...func(*s3.Options)) (*s3.CompleteMultipartUploadOutput, error)
	AbortMultipartUpload(ctx context.Context, input *s3.AbortMultipartUploadInput, opts ...func(*s3.Options)) (*s3.AbortMultipartUploadOutput, error)
}

// DirectUploaderConfig configures the direct uploader stage
type DirectUploaderConfig struct {
	S3Client        S3Uploader            // AWS S3 client (interface for testability)
	Bucket          string                // Target S3 bucket
	Prefix          string                // S3 key prefix (optional)
	UploadID        string                // Upload session ID; keys are isolated under uploads/<id>/ so distinct uploads never share an object key (#591)
	SourcePath      string                // Source root; keys preserve each file's path relative to it (#480)
	FileChecksums   bool                  // Record a SHA-256 per file in the manifest (CSH-SEC-002; on by default, --no-file-checksums opts out)
	Workers         int                   // Number of concurrent upload workers
	MaxRetries      int                   // Maximum upload retry attempts
	RetryDelay      time.Duration         // Delay between retries
	WorkerPool      *AdaptiveWorkerPool   // Optional: Use adaptive worker pool
	ManifestBuilder directManifestBuilder // Optional: records S3Key per file after upload
	// SSEKMSKeyID, when set, stores every object with SSE-KMS under this key, and
	// StorageClass sets the objects' class (#946: direct uploads applied neither).
	SSEKMSKeyID  string
	StorageClass types.StorageClass
}

// DirectUploaderStage uploads files directly to S3 without archiving or compression
// This provides maximum speed at the cost of more S3 operations
type DirectUploaderStage struct {
	config *DirectUploaderConfig
	input  <-chan *Job
	output chan<- *Job
	ctx    context.Context
	cancel context.CancelFunc
	wg     sync.WaitGroup
	mu     sync.RWMutex
	stats  StageStats

	// Atomic counters
	jobsProcessed  int64
	filesProcessed int64
	bytesProcessed int64

	// Worker pool
	pool *AdaptiveWorkerPool
}

// NewDirectUploaderStage creates a new direct uploader stage
func NewDirectUploaderStage(config *DirectUploaderConfig, input <-chan *Job, output chan<- *Job) (*DirectUploaderStage, error) {
	if config == nil {
		return nil, fmt.Errorf("config cannot be nil")
	}
	if config.S3Client == nil {
		return nil, fmt.Errorf("S3Client cannot be nil")
	}
	if config.Bucket == "" {
		return nil, fmt.Errorf("bucket cannot be empty")
	}
	if config.Workers <= 0 {
		config.Workers = 256 // Default to 256 workers (match s5cmd)
	}
	if config.MaxRetries <= 0 {
		config.MaxRetries = 3
	}
	if config.RetryDelay <= 0 {
		config.RetryDelay = time.Second
	}

	stage := &DirectUploaderStage{
		config: config,
		input:  input,
		output: output,
		stats: StageStats{
			Name: "direct_uploader",
		},
	}

	// Store optional pool reference
	stage.pool = config.WorkerPool

	return stage, nil
}

// Name returns the stage name
func (s *DirectUploaderStage) Name() string {
	return "direct_uploader"
}

// Start starts the direct uploader stage
func (s *DirectUploaderStage) Start(ctx context.Context) error {
	// Create child context from parent (inherits trace context for Issue #155)
	s.ctx, s.cancel = context.WithCancel(ctx)

	// Create adaptive worker pool if not provided
	if s.pool == nil {
		poolConfig := &AdaptiveWorkerPoolConfig{
			MaxWorkers:     s.config.Workers,
			EnableAdaptive: true,
		}
		s.pool = NewAdaptiveWorkerPool(s.ctx, poolConfig)
	}

	// Start main dispatcher goroutine
	s.wg.Add(1)
	go s.dispatcher(s.ctx)

	return nil
}

// Stop stops the direct uploader stage
func (s *DirectUploaderStage) Stop() error {
	if s.cancel != nil {
		s.cancel()
	}
	s.wg.Wait()
	if s.pool != nil {
		s.pool.Stop()
	}
	return nil
}

// dispatcher reads jobs and submits files to worker pool
func (s *DirectUploaderStage) dispatcher(ctx context.Context) {
	defer s.wg.Done()
	defer close(s.output)

	for {
		select {
		case <-ctx.Done():
			return
		case job, ok := <-s.input:
			if !ok {
				// Input closed, wait for all workers to finish
				s.pool.Wait()
				return
			}

			// Process each file in the chunk independently
			var (
				fileWg   sync.WaitGroup
				jobErr   error
				jobErrMu sync.Mutex
				// #694: bytes this job actually uploaded. waitForCompletion builds
				// Result.TotalBytes by summing job.ArchiveSize, which only the archiver
				// ever set — so every direct-upload cycle reported bytes=0 after moving
				// real data, and the heartbeat's bytes field was useless for exactly the
				// small-file workloads this fast path exists for. Counted per file on
				// success rather than summed from the chunk, so a partial failure does not
				// claim bytes that never landed.
				jobBytes int64
			)
			for _, file := range job.Chunk.Files {
				// Create a copy of job for each file to avoid race conditions
				fileCopy := file
				fileWg.Add(1)
				s.submitFileUpload(ctx, fileCopy, &fileWg, func(err error) {
					if err != nil {
						jobErrMu.Lock()
						if jobErr == nil {
							jobErr = err
						}
						jobErrMu.Unlock()
						return
					}
					atomic.AddInt64(&jobBytes, fileCopy.Size)
				})
			}

			// Wait for all files in this job to complete
			fileWg.Wait()
			job.Error = jobErr
			atomic.StoreInt64(&job.ArchiveSize, atomic.LoadInt64(&jobBytes))

			// Mark job as processed and send to output
			atomic.AddInt64(&s.jobsProcessed, 1)

			// Send job to output (non-blocking)
			select {
			case s.output <- job:
			case <-ctx.Done():
				return
			}
		}
	}
}

// submitFileUpload submits a single file upload to the worker pool.
// onDone is called with the upload error (nil on success) after the worker completes.
func (s *DirectUploaderStage) submitFileUpload(ctx context.Context, file chunking.File, wg *sync.WaitGroup, onDone func(error)) {
	err := s.pool.Submit(func(workerCtx context.Context) error {
		defer wg.Done()
		uploadErr := s.uploadFile(workerCtx, file)
		onDone(uploadErr)
		return uploadErr
	})

	if err != nil {
		// If submission failed, still decrement WaitGroup
		wg.Done()
		onDone(err)
		// Log error but continue processing other files
		s.mu.Lock()
		s.stats.JobsFailed++
		s.mu.Unlock()
	}
}

// uploadFile uploads a single file to S3
func (s *DirectUploaderStage) uploadFile(ctx context.Context, file chunking.File) error {
	startTime := time.Now()

	// Build S3 key
	s3Key := s.buildS3Key(file.Path)

	// Open the file through a root anchored at the source dir so a path swapped
	// to a symlink after scanning can't redirect the read outside the tree
	// (CSH-SEC-001).
	f, err := openContainedSourceFile(s.config.SourcePath, file.Path)
	if err != nil {
		return fmt.Errorf("failed to open file %s: %w", file.Path, err)
	}
	defer func() {
		_ = f.Close()
	}()

	// Upload with retries
	var lastErr error
	for attempt := 0; attempt < s.config.MaxRetries; attempt++ {
		if attempt > 0 {
			select {
			case <-ctx.Done():
				return ctx.Err()
			case <-time.After(s.config.RetryDelay):
			}
		}

		sent, err := s.putFile(ctx, f, file, s3Key)
		if err != nil {
			lastErr = err
			continue
		}

		// Record what was sent in the manifest: the key, the checksum of the
		// bytes sent (when enabled), and their size and modification time (#1015).
		if s.config.ManifestBuilder != nil {
			s.config.ManifestBuilder.RecordDirectFile(file.Path, 0, s3Key, sent.checksum, sent.size, sent.modTime)
		}

		// Success
		atomic.AddInt64(&s.filesProcessed, 1)
		atomic.AddInt64(&s.bytesProcessed, sent.size)
		s.pool.AddBytes(sent.size) // For throughput monitoring

		s.mu.Lock()
		s.stats.JobsProcessed++
		s.stats.BytesProcessed += sent.size
		s.stats.TotalTime += time.Since(startTime)
		if s.stats.JobsProcessed > 0 {
			s.stats.AverageTime = s.stats.TotalTime / time.Duration(s.stats.JobsProcessed)
		}
		s.mu.Unlock()

		return nil
	}

	return fmt.Errorf("upload failed after %d attempts for %s: %w", s.config.MaxRetries, file.Path, lastErr)
}

// sentFile is what one successful direct PUT sent.
type sentFile struct {
	checksum string // SHA-256 of the bytes sent; "" when checksums are off
	size     int64
	modTime  time.Time
}

// putFile sends f as it is now and reports what was sent (#1015). The size and
// modification time come from a stat taken just before the PUT, the body is
// bounded to that size, and the checksum is taken from the bytes the SDK read
// for the request rather than from an earlier pass, which a change in between
// left describing other bytes. A stat after the PUT that differs means the file
// changed while it was read; that attempt fails, so the caller sends it again.
func (s *DirectUploaderStage) putFile(ctx context.Context, f *os.File, file chunking.File, s3Key string) (sentFile, error) {
	before, err := f.Stat()
	if err != nil {
		return sentFile{}, fmt.Errorf("stat %s: %w", file.Path, err)
	}
	size := before.Size()
	section := io.NewSectionReader(f, 0, size)
	var body io.ReadSeeker = section
	var hasher *sentBytesHasher
	if s.config.FileChecksums {
		hasher = &sentBytesHasher{r: section, h: sha256.New()}
		body = hasher
	}

	input := &s3.PutObjectInput{
		Bucket:        aws.String(s.config.Bucket),
		Key:           aws.String(s3Key),
		Body:          body, // seekable, so the SDK can sign it and rewind on retry
		ContentLength: aws.Int64(size),
		StorageClass:  s.config.StorageClass,
		// #1037: no cargoship-file-path. It held the file's absolute local
		// path, which nothing reads, and it exposed every name to anyone who
		// can HeadObject. The manifest records the path.
		Metadata: map[string]string{
			"cargoship-mode":      "direct",
			"cargoship-file-size": strconv.FormatInt(size, 10),
		},
	}
	if s.config.SSEKMSKeyID != "" {
		input.ServerSideEncryption = types.ServerSideEncryptionAwsKms
		input.SSEKMSKeyId = aws.String(s.config.SSEKMSKeyID)
	}
	if _, err := s.config.S3Client.PutObject(ctx, input); err != nil {
		return sentFile{}, err
	}

	after, err := f.Stat()
	if err != nil {
		return sentFile{}, fmt.Errorf("stat %s after upload: %w", file.Path, err)
	}
	if after.Size() != size || !after.ModTime().Equal(before.ModTime()) {
		return sentFile{}, fmt.Errorf("%s changed while it was being uploaded (%d bytes, modified %s, became %d bytes, modified %s); upload it again",
			file.Path, size, before.ModTime().Format(time.RFC3339Nano), after.Size(), after.ModTime().Format(time.RFC3339Nano))
	}
	sent := sentFile{size: size, modTime: before.ModTime()}
	if hasher != nil {
		sum, ok := hasher.sum(size)
		if !ok {
			return sentFile{}, fmt.Errorf("%s: the upload did not read the file once in full (%d of %d bytes hashed); upload it again",
				file.Path, hasher.n, size)
		}
		sent.checksum = sum
	}
	return sent, nil
}

// sentBytesHasher hashes the bytes read from a seekable upload body. The SDK can
// read a body more than once (to sign it, to checksum it, to retry), rewinding
// to the start before each pass, so the hash restarts on every rewind. Once the
// PUT returns, it covers the pass that was sent (#1015).
type sentBytesHasher struct {
	r      io.ReadSeeker
	h      hash.Hash
	n      int64 // bytes hashed since the last rewind
	pos    int64 // current offset in r
	broken bool  // a read since the last rewind didn't continue where the hashed bytes end
}

// Read reads from the body and hashes what it returns.
func (s *sentBytesHasher) Read(p []byte) (int, error) {
	n, err := s.r.Read(p)
	if n > 0 {
		if s.pos != s.n {
			s.broken = true
		}
		s.h.Write(p[:n])
		s.n += int64(n)
		s.pos += int64(n)
	}
	return n, err
}

// Seek seeks the body; seeking to the start restarts the hash.
func (s *sentBytesHasher) Seek(offset int64, whence int) (int64, error) {
	pos, err := s.r.Seek(offset, whence)
	if err != nil {
		return pos, err
	}
	s.pos = pos
	if pos == 0 {
		s.h.Reset()
		s.n, s.broken = 0, false
	}
	return pos, nil
}

// sum returns the digest of the last pass, and false unless that pass read the
// body once, in order, from the start to size.
func (s *sentBytesHasher) sum(size int64) (string, bool) {
	if s.broken || s.n != size {
		return "", false
	}
	return hex.EncodeToString(s.h.Sum(nil)), true
}

// buildS3Key constructs the S3 key for a file, preserving its path relative to
// the source root so that two files with the same basename in different
// directories get distinct keys. Keying by basename alone (the old behavior)
// made same-basename files collide: the second upload overwrote the first and
// restore silently returned the survivor's bytes for both (#480). Falls back to
// the basename when no source root is set or the file lies outside it.
//
// #591: the key is isolated under uploads/<uploadID>/ (matching the chunked
// path's layout) so two uploads of the same relative path to the same
// bucket/prefix — across hosts, or one host re-uploading a changed tree — no
// longer resolve to the same object key and silently overwrite each other.
// Without the segment, restoring an older upload's manifest fetched the newer
// upload's bytes. When UploadID is empty (unit tests), the pre-#591 flat layout
// is kept; restore is unaffected either way because it resolves the manifest's
// stored FileEntry.S3Key, and ResolveObjectKey is idempotent for both shapes.
//
// #1020: the files sit one level down, under uploads/<uploadID>/files/, so no
// source file can take a key CargoShip writes for itself. At uploads/<id>/ a
// root-level manifest.json.gz was overwritten by the manifest,
// manifest.partial.json.gz was deleted with the partial manifest, and a
// manifest.encrypted.json[.gz] made readers try to decrypt the user's file.
// Uploads written before this keep their keys and still restore (readers use
// the recorded S3Key).
func (s *DirectUploaderStage) buildS3Key(filePath string) string {
	relPath := filepath.Base(filePath)
	if s.config.SourcePath != "" {
		if rel, err := filepath.Rel(s.config.SourcePath, filePath); err == nil &&
			rel != "" && !strings.HasPrefix(rel, "..") && !filepath.IsAbs(rel) {
			relPath = rel
		}
	}
	// S3 keys are always slash-separated, regardless of host OS.
	relPath = filepath.ToSlash(relPath)

	segments := make([]string, 0, 5)
	if s.config.Prefix != "" {
		segments = append(segments, s.config.Prefix)
	}
	if s.config.UploadID != "" {
		segments = append(segments, "uploads", s.config.UploadID, DirectFilesSegment)
	}
	segments = append(segments, relPath)
	return path.Join(segments...)
}

// Process processes a single job (not used in direct mode, kept for interface compatibility)
func (s *DirectUploaderStage) Process(ctx context.Context, job *Job) error {
	// Direct uploader processes files individually in dispatcher
	// This method is kept for Stage interface compatibility
	return nil
}

// Stats returns stage statistics
func (s *DirectUploaderStage) Stats() StageStats {
	s.mu.RLock()
	defer s.mu.RUnlock()

	stats := s.stats
	stats.JobsProcessed = atomic.LoadInt64(&s.jobsProcessed)
	stats.BytesProcessed = atomic.LoadInt64(&s.bytesProcessed)
	stats.ActiveWorkers = s.pool.GetWorkerCount()

	// Add custom metadata
	stats.Metadata = map[string]interface{}{
		"files_uploaded":  atomic.LoadInt64(&s.filesProcessed),
		"worker_count":    s.pool.GetWorkerCount(),
		"throughput_mbps": float64(s.pool.GetThroughput()) / (1024 * 1024),
	}

	return stats
}

// GetUploadedFiles returns total files uploaded
func (s *DirectUploaderStage) GetUploadedFiles() int64 {
	return atomic.LoadInt64(&s.filesProcessed)
}

// GetUploadedBytes returns total bytes uploaded
func (s *DirectUploaderStage) GetUploadedBytes() int64 {
	return atomic.LoadInt64(&s.bytesProcessed)
}

// GetWorkerCount returns current worker count
func (s *DirectUploaderStage) GetWorkerCount() int {
	return s.pool.GetWorkerCount()
}
