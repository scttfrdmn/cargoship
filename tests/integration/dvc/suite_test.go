//go:build integration

// Package dvc contains end-to-end integration tests for the DVC workflow:
// incremental sync, selective restore, budget enforcement, and concurrent
// operations. By default tests run against an in-process Substrate server.
// Set CARGOSHIP_ENABLE_S3_INTEGRATION_TESTS=1 and CARGOSHIP_TEST_BUCKET to
// run against real AWS S3.
//
// Run with:
//
//	go test -v -tags=integration ./tests/integration/dvc/... -timeout=30m
package dvc

import (
	"archive/tar"
	"bytes"
	"context"
	"crypto/md5" //nolint:gosec // MD5 for DVC compatibility, not security
	"fmt"
	"log/slog"
	"math/rand"
	"net"
	"net/http"
	"os"
	"path/filepath"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/aws/aws-sdk-go-v2/aws"
	"github.com/aws/aws-sdk-go-v2/config"
	"github.com/aws/aws-sdk-go-v2/credentials"
	"github.com/aws/aws-sdk-go-v2/service/s3"
	"github.com/klauspost/compress/zstd"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	substrate "github.com/scttfrdmn/substrate/emulator"

	"github.com/scttfrdmn/cargoship/pkg/manifest"
)

// ---------------------------------------------------------------------------
// Test environment helpers
// ---------------------------------------------------------------------------

const (
	envEnableInteg = "CARGOSHIP_ENABLE_S3_INTEGRATION_TESTS"
	envTestBucket  = "CARGOSHIP_TEST_BUCKET"
	envAWSRegion   = "AWS_REGION"
)

var substrateURL string

func TestMain(m *testing.M) {
	if os.Getenv(envEnableInteg) != "1" {
		url, cancel, err := launchSubstrate()
		if err != nil {
			fmt.Fprintf(os.Stderr, "substrate: %v\n", err)
			os.Exit(1)
		}
		defer cancel()
		substrateURL = url
		os.Setenv("AWS_ENDPOINT_URL", url)
		os.Setenv("AWS_ACCESS_KEY_ID", "test")
		os.Setenv("AWS_SECRET_ACCESS_KEY", "test")
		os.Setenv("AWS_REGION", "us-east-1")
	}
	os.Exit(m.Run())
}

// launchSubstrate starts an in-process Substrate server for use in TestMain.
func launchSubstrate() (string, context.CancelFunc, error) {
	cfg := substrate.DefaultConfig()
	cfg.Server.Address = "127.0.0.1:0"
	cfg.EventStore.Enabled = false
	cfg.Log.Level = "error"

	state := substrate.NewMemoryStateManager()
	tc := substrate.NewTimeController(time.Now())
	registry := substrate.NewPluginRegistry()
	logger := substrate.NewDefaultLogger(slog.LevelError, false)
	store := substrate.NewEventStore(cfg.EventStore.ToEventStoreConfig(), substrate.WithTimeController(tc))

	ctx := context.Background()
	if err := substrate.RegisterDefaultPlugins(ctx, registry, state, tc, logger, store, nil); err != nil {
		return "", nil, fmt.Errorf("register plugins: %w", err)
	}

	ln, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		return "", nil, fmt.Errorf("listen: %w", err)
	}
	port := ln.Addr().(*net.TCPAddr).Port

	srv := substrate.NewServer(*cfg, registry, store, state, tc, logger)
	srvCtx, cancel := context.WithCancel(context.Background())
	go func() { _ = srv.Serve(srvCtx, ln) }()

	baseURL := fmt.Sprintf("http://127.0.0.1:%d", port)
	deadline := time.Now().Add(5 * time.Second)
	for time.Now().Before(deadline) {
		resp, pingErr := http.Get(baseURL + "/health") //nolint:noctx
		if pingErr == nil {
			_ = resp.Body.Close()
			break
		}
		time.Sleep(10 * time.Millisecond)
	}
	return baseURL, cancel, nil
}

// createSubstrateBucket creates an S3 bucket on the Substrate server.
func createSubstrateBucket(baseURL, bucket string) error {
	cfg := aws.Config{
		Region:       "us-east-1",
		BaseEndpoint: aws.String(baseURL),
		Credentials:  credentials.NewStaticCredentialsProvider("test", "test", ""),
	}
	client := s3.NewFromConfig(cfg, func(o *s3.Options) { o.UsePathStyle = true })
	_, err := client.CreateBucket(context.Background(), &s3.CreateBucketInput{
		Bucket: aws.String(bucket),
	})
	return err
}

// integEnv holds the shared test environment for S3-backed tests.
type integEnv struct {
	ctx      context.Context
	s3Client *s3.Client
	bucket   string
	prefix   string
}

// newIntegEnv returns a configured environment with a cleanup hook.
// If CARGOSHIP_TEST_BUCKET is set it uses real AWS; otherwise it uses Substrate.
func newIntegEnv(t *testing.T) *integEnv {
	t.Helper()
	bucket := os.Getenv(envTestBucket)
	region := os.Getenv(envAWSRegion)
	if region == "" {
		region = "us-east-1"
	}
	ctx := context.Background()
	cfg, err := config.LoadDefaultConfig(ctx, config.WithRegion(region))
	require.NoError(t, err)

	var s3Opts []func(*s3.Options)
	if bucket == "" {
		bucket = "cargoship-dvc-test"
		if err := createSubstrateBucket(substrateURL, bucket); err != nil {
			t.Logf("bucket may already exist: %v", err)
		}
		s3Opts = append(s3Opts, func(o *s3.Options) { o.UsePathStyle = true })
	}

	env := &integEnv{
		ctx:      ctx,
		s3Client: s3.NewFromConfig(cfg, s3Opts...),
		bucket:   bucket,
		prefix:   fmt.Sprintf("dvc-integ/%s", t.Name()),
	}
	t.Cleanup(func() { env.cleanup(t) })
	return env
}

// cleanup removes all test objects under env.prefix.
func (e *integEnv) cleanup(t *testing.T) {
	t.Helper()
	pager := s3.NewListObjectsV2Paginator(e.s3Client, &s3.ListObjectsV2Input{
		Bucket: aws.String(e.bucket),
		Prefix: aws.String(e.prefix),
	})
	for pager.HasMorePages() {
		page, err := pager.NextPage(e.ctx)
		if err != nil {
			return
		}
		for _, obj := range page.Contents {
			_, _ = e.s3Client.DeleteObject(e.ctx, &s3.DeleteObjectInput{
				Bucket: aws.String(e.bucket),
				Key:    obj.Key,
			})
		}
	}
}

// ---------------------------------------------------------------------------
// Synthetic dataset helpers
// ---------------------------------------------------------------------------

// syntheticFile is an in-memory file.
type syntheticFile struct {
	path        string
	data        []byte
	contentHash string
	dvcStage    string
}

// buildDataset creates n deterministic files distributed across DVC stages.
func buildDataset(n int) []syntheticFile {
	stages := []string{"preprocess", "train", "evaluate"}
	rng := rand.New(rand.NewSource(42)) //nolint:gosec
	files := make([]syntheticFile, n)
	for i := range files {
		data := make([]byte, 512+rng.Intn(512))
		for j := range data {
			data[j] = byte('a' + rng.Intn(26))
		}
		sum := md5.Sum(data) //nolint:gosec
		files[i] = syntheticFile{
			path:        fmt.Sprintf("data/file-%04d.bin", i),
			data:        data,
			contentHash: fmt.Sprintf("%x", sum),
			dvcStage:    stages[i%len(stages)],
		}
	}
	return files
}

// buildManifest constructs a Manifest from files split across chunkCount chunks.
func buildManifest(files []syntheticFile, bucket, prefix, uploadID string, chunkCount int) *manifest.Manifest {
	perChunk := len(files) / chunkCount
	if perChunk < 1 {
		perChunk = 1
	}
	m := &manifest.Manifest{
		Version:         manifest.ManifestVersion,
		UploadID:        uploadID,
		Bucket:          bucket,
		Prefix:          prefix,
		CompressionType: "zstd",
		GitMetadata:     &manifest.GitMetadata{Commit: "integ-commit-abc"},
		CreatedAt:       time.Now(),
	}
	chunkKeys := make(map[int]string)
	chunkFileCount := make(map[int]int)
	chunkSize := make(map[int]int64)
	for i, sf := range files {
		chunkID := i / perChunk
		if chunkID >= chunkCount {
			chunkID = chunkCount - 1
		}
		if _, ok := chunkKeys[chunkID]; !ok {
			chunkKeys[chunkID] = fmt.Sprintf("%s/%s/chunk-%02d.tar.zst", prefix, uploadID, chunkID)
		}
		chunkFileCount[chunkID]++
		chunkSize[chunkID] += int64(len(sf.data))
		m.Files = append(m.Files, manifest.FileEntry{
			Path:        sf.path,
			Size:        int64(len(sf.data)),
			ModTime:     time.Now(),
			ContentHash: sf.contentHash,
			ChunkID:     chunkID,
			ShardID:     0,
			S3Key:       chunkKeys[chunkID],
			DVCMetadata: &manifest.DVCMetadata{Stage: sf.dvcStage},
		})
	}
	m.TotalFiles = int64(len(files))

	// Populate the Chunks slice so this is a valid chunked manifest. Without it,
	// len(m.Chunks) == 0 and the restore path treats the manifest as
	// direct-upload mode (writing files by basename), which breaks the
	// path-preserving chunked restore these tests assert. (see #228, #238)
	var totalFiles int64
	var totalSize int64
	for id := 0; id < chunkCount; id++ {
		key, ok := chunkKeys[id]
		if !ok {
			continue // fewer chunks than requested (small dataset)
		}
		m.Chunks = append(m.Chunks, manifest.ChunkEntry{
			ID:               id,
			ShardID:          0,
			S3Key:            key,
			FileCount:        chunkFileCount[id],
			UncompressedSize: chunkSize[id],
			CompressedSize:   chunkSize[id],
			CreatedAt:        time.Now(),
		})
		totalFiles += int64(chunkFileCount[id])
		totalSize += chunkSize[id]
	}
	m.TotalChunks = len(m.Chunks)
	m.ShardCount = 1
	m.Shards = []manifest.ShardEntry{{
		ID:               0,
		Prefix:           fmt.Sprintf("%s/%s/shard-0", prefix, uploadID),
		ChunkCount:       len(m.Chunks),
		FileCount:        totalFiles,
		UncompressedSize: totalSize,
		CompressedSize:   totalSize,
	}}
	return m
}

// ---------------------------------------------------------------------------
// In-memory S3 client (manifest.S3Downloader)
// ---------------------------------------------------------------------------

// memS3 is a thread-safe in-memory S3-like store.
type memS3 struct {
	mu   sync.RWMutex
	data map[string][]byte
}

func newMemS3() *memS3 { return &memS3{data: make(map[string][]byte)} }

func (m *memS3) put(key string, data []byte) {
	m.mu.Lock()
	m.data[key] = append([]byte(nil), data...)
	m.mu.Unlock()
}

// GetObject implements manifest.S3Downloader.
func (m *memS3) GetObject(_ context.Context, in *s3.GetObjectInput, _ ...func(*s3.Options)) (*s3.GetObjectOutput, error) {
	m.mu.RLock()
	data, ok := m.data[*in.Key]
	m.mu.RUnlock()
	if !ok {
		return nil, fmt.Errorf("memS3: key not found: %s", *in.Key)
	}
	return &s3.GetObjectOutput{Body: nopCloser{bytes.NewReader(data)}}, nil
}

type nopCloser struct{ *bytes.Reader }

func (nopCloser) Close() error { return nil }

// ---------------------------------------------------------------------------
// Chunk builder
// ---------------------------------------------------------------------------

// buildChunks populates an in-memory S3 with zstd-compressed tar archives for
// every chunk in m. Files are filled with their synthetic content from byPath.
func buildChunks(t *testing.T, m *manifest.Manifest, files []syntheticFile) *memS3 {
	t.Helper()
	byPath := make(map[string][]byte, len(files))
	for _, sf := range files {
		byPath[sf.path] = sf.data
	}
	// Group entries by S3 key (one archive per chunk).
	keyEntries := make(map[string][]manifest.FileEntry)
	for i := range m.Files {
		fe := m.Files[i]
		keyEntries[fe.S3Key] = append(keyEntries[fe.S3Key], fe)
	}
	client := newMemS3()
	for s3Key, entries := range keyEntries {
		client.put(s3Key, buildZstdTar(t, entries, byPath))
	}
	return client
}

// buildZstdTar builds a zstd-compressed tar archive containing the given
// FileEntries, using byPath for the actual content bytes.
func buildZstdTar(t *testing.T, entries []manifest.FileEntry, byPath map[string][]byte) []byte {
	t.Helper()
	buf := new(bytes.Buffer)
	enc, err := zstd.NewWriter(buf)
	require.NoError(t, err)
	tw := tar.NewWriter(enc)
	for _, fe := range entries {
		content, ok := byPath[fe.Path]
		if !ok {
			content = bytes.Repeat([]byte("x"), int(fe.Size))
		}
		err := tw.WriteHeader(&tar.Header{Name: fe.Path, Size: int64(len(content)), Mode: 0644})
		require.NoError(t, err)
		_, err = tw.Write(content)
		require.NoError(t, err)
	}
	require.NoError(t, tw.Close())
	require.NoError(t, enc.Close())
	return buf.Bytes()
}

// ---------------------------------------------------------------------------
// Test: Incremental sync — detect changed files via content hash
// ---------------------------------------------------------------------------

// TestDVCWorkflow_IncrementalSync verifies that modifying 20% of files causes
// only those files to be flagged for re-upload when comparing hashes.
func TestDVCWorkflow_IncrementalSync(t *testing.T) {
	const (
		total       = 50
		modifyEvery = 5 // modify 20%
	)

	files := buildDataset(total)
	m := buildManifest(files, "test-bucket", "test-prefix", "upload-001", 5)
	mq := manifest.NewManifestQuery(m)

	// Verify every file is indexed by hash.
	for _, sf := range files {
		fe, ok := mq.FindFileByHash(sf.contentHash)
		require.True(t, ok, "hash not found for %s", sf.path)
		assert.Equal(t, sf.path, fe.Path)
	}

	// Modify every 5th file (= 20%).
	var modified int
	for i := range files {
		if i%modifyEvery == 0 {
			files[i].data = append(files[i].data, []byte("changed")...)
			sum := md5.Sum(files[i].data) //nolint:gosec
			files[i].contentHash = fmt.Sprintf("%x", sum)
			modified++
		}
	}

	// Count which files need re-upload vs. can be skipped.
	var toUpload, toSkip int
	for _, sf := range files {
		if _, ok := mq.FindFileByHash(sf.contentHash); ok {
			toSkip++
		} else {
			toUpload++
		}
	}

	assert.Equal(t, modified, toUpload, "only modified files need re-upload")
	assert.Equal(t, total-modified, toSkip, "unchanged files can be skipped")
}

// ---------------------------------------------------------------------------
// Test: Selective restore via hash, git commit, and DVC stage
// ---------------------------------------------------------------------------

// TestDVCWorkflow_SelectiveRestore verifies SelectiveExtractor correctly
// restores files by hash, DVC stage, and git commit.
func TestDVCWorkflow_SelectiveRestore(t *testing.T) {
	const (
		fileCount  = 100
		chunkCount = 10
	)
	files := buildDataset(fileCount)
	m := buildManifest(files, "test-bucket", "test-prefix", "upload-002", chunkCount)
	client := buildChunks(t, m, files)
	se := manifest.NewSelectiveExtractor(m, client, 0)

	t.Run("by_hash", func(t *testing.T) {
		destDir := t.TempDir()
		target := files[7]
		stats, err := se.ExtractFileByHash(context.Background(), target.contentHash, destDir)
		require.NoError(t, err)
		assert.Equal(t, int64(1), stats.Restored)
		assert.Equal(t, int64(1), stats.ChunksDownloaded)

		// Verify content integrity.
		got, err := os.ReadFile(filepath.Join(destDir, target.path))
		require.NoError(t, err)
		assert.Equal(t, target.data, got)
	})

	t.Run("by_dvc_stage", func(t *testing.T) {
		destDir := t.TempDir()
		stats, err := se.BatchRestoreByDVCStage(context.Background(), "preprocess", destDir)
		require.NoError(t, err)
		// Every 3rd file (index 0, 3, 6, …) → ~34 files.
		assert.Greater(t, stats.Restored, int64(30))
	})

	t.Run("by_commit", func(t *testing.T) {
		destDir := t.TempDir()
		stats, err := se.BatchRestoreByCommit(context.Background(), "integ-commit-abc", destDir)
		require.NoError(t, err)
		assert.Equal(t, int64(fileCount), stats.Restored)
	})

	t.Run("minimises_downloads_for_colocated_files", func(t *testing.T) {
		// Files 0-9 share chunk 0 (10 files per chunk).
		// Use a fresh SelectiveExtractor with a zero-capacity cache so that
		// all downloads are counted (no carry-over from sibling subtests).
		freshSE := manifest.NewSelectiveExtractor(m, client, 1) // 1-byte cache = effectively no caching
		destDir := t.TempDir()
		targets := []string{files[0].path, files[3].path, files[5].path}
		stats, err := freshSE.BatchRestore(context.Background(), targets, destDir)
		require.NoError(t, err)
		assert.Equal(t, int64(3), stats.Restored)
		assert.Equal(t, int64(1), stats.ChunksDownloaded, "3 files co-located in one chunk → 1 download")
	})
}

// ---------------------------------------------------------------------------
// Test: Budget enforcement — simulate budget exceeded (chunk download fails)
// ---------------------------------------------------------------------------

// TestDVCWorkflow_BudgetEnforcement verifies that a failed chunk download
// (simulating budget denial / 403) increments Failed and does not prevent
// other chunks from succeeding.
func TestDVCWorkflow_BudgetEnforcement(t *testing.T) {
	files := buildDataset(20)
	m := buildManifest(files, "test-bucket", "test-prefix", "upload-003", 2)

	// Populate only chunk 0; chunk 1 is absent (simulates budget exceeded).
	client := newMemS3()
	byPath := make(map[string][]byte, len(files))
	for _, sf := range files {
		byPath[sf.path] = sf.data
	}
	for _, fe := range m.Files {
		if fe.ChunkID == 0 {
			if _, exists := client.data[fe.S3Key]; !exists {
				var entries []manifest.FileEntry
				for _, f := range m.Files {
					if f.ChunkID == 0 {
						entries = append(entries, f)
					}
				}
				client.put(fe.S3Key, buildZstdTar(t, entries, byPath))
				break
			}
		}
	}

	se := manifest.NewSelectiveExtractor(m, client, 0)
	allPaths := make([]string, len(files))
	for i, sf := range files {
		allPaths[i] = sf.path
	}

	stats, err := se.BatchRestore(context.Background(), allPaths, t.TempDir())
	require.NoError(t, err) // partial failures must not surface as error

	assert.Greater(t, stats.Restored, int64(0), "chunk 0 files should restore")
	assert.Greater(t, stats.Failed, int64(0), "chunk 1 files should fail (not in memS3)")
	assert.Equal(t, int64(1), stats.ChunksDownloaded, "only chunk 0 downloaded")
}

// ---------------------------------------------------------------------------
// Test: Concurrent DVC operations from multiple clients
// ---------------------------------------------------------------------------

// TestDVCWorkflow_ConcurrentClients simulates concurrent restore operations
// on the same manifest, verifying no data races and correct aggregate counts.
func TestDVCWorkflow_ConcurrentClients(t *testing.T) {
	const (
		fileCount  = 150
		chunkCount = 15
		numClients = 5
	)
	files := buildDataset(fileCount)
	m := buildManifest(files, "test-bucket", "test-prefix", "upload-004", chunkCount)
	client := buildChunks(t, m, files)
	stages := []string{"preprocess", "train", "evaluate"}

	var totalRestored int64
	var wg sync.WaitGroup
	for i := 0; i < numClients; i++ {
		wg.Add(1)
		stage := stages[i%len(stages)]
		go func(stage string) {
			defer wg.Done()
			se := manifest.NewSelectiveExtractor(m, client, 0)
			stats, err := se.BatchRestoreByDVCStage(context.Background(), stage, t.TempDir())
			if err == nil {
				atomic.AddInt64(&totalRestored, stats.Restored)
			}
		}(stage)
	}
	wg.Wait()

	assert.Greater(t, totalRestored, int64(100), "concurrent clients should collectively restore files")
}

// ---------------------------------------------------------------------------
// Test: P95 latency benchmark for hash lookup
// ---------------------------------------------------------------------------

// TestDVCWorkflow_P95LatencyBenchmark ensures the P95 hash-lookup latency
// across 1000 files stays below 5ms (in-memory, no I/O).
func TestDVCWorkflow_P95LatencyBenchmark(t *testing.T) {
	const n = 1000
	files := buildDataset(n)
	m := buildManifest(files, "test-bucket", "test-prefix", "upload-bench", 10)
	mq := manifest.NewManifestQuery(m)

	latencies := make([]time.Duration, n)
	for i, sf := range files {
		start := time.Now()
		_, _ = mq.FindFileByHash(sf.contentHash)
		latencies[i] = time.Since(start)
	}
	sortDurations(latencies)
	p95 := latencies[int(float64(n)*0.95)]
	t.Logf("P95 hash-lookup latency: %v (n=%d)", p95, n)
	assert.Less(t, p95.Microseconds(), int64(5000), "P95 hash lookup must be < 5ms")
}

// ---------------------------------------------------------------------------
// Test: Real S3 round-trip (gated by CARGOSHIP_ENABLE_S3_INTEGRATION_TESTS)
// ---------------------------------------------------------------------------

// TestDVCWorkflow_RealS3RoundTrip performs a real upload → restore cycle
// against CARGOSHIP_TEST_BUCKET, verifying content integrity.
func TestDVCWorkflow_RealS3RoundTrip(t *testing.T) {
	env := newIntegEnv(t)

	files := buildDataset(20)
	uploadID := fmt.Sprintf("integ-%d", time.Now().UnixNano())
	m := buildManifest(files, env.bucket, env.prefix, uploadID, 2)

	// Upload synthetic chunks to real S3.
	byPath := make(map[string][]byte, len(files))
	for _, sf := range files {
		byPath[sf.path] = sf.data
	}
	keyEntries := make(map[string][]manifest.FileEntry)
	for i := range m.Files {
		fe := m.Files[i]
		keyEntries[fe.S3Key] = append(keyEntries[fe.S3Key], fe)
	}
	for s3Key, entries := range keyEntries {
		data := buildZstdTar(t, entries, byPath)
		_, err := env.s3Client.PutObject(env.ctx, &s3.PutObjectInput{
			Bucket: aws.String(env.bucket),
			Key:    aws.String(s3Key),
			Body:   bytes.NewReader(data),
		})
		require.NoError(t, err, "upload chunk %s", s3Key)
	}

	// Selective restore via hash using real S3 client.
	se := manifest.NewSelectiveExtractor(m, env.s3Client, 0)
	destDir := t.TempDir()
	target := files[3]
	stats, err := se.ExtractFileByHash(env.ctx, target.contentHash, destDir)
	require.NoError(t, err)
	assert.Equal(t, int64(1), stats.Restored)

	// Verify content integrity.
	got, err := os.ReadFile(filepath.Join(destDir, target.path))
	require.NoError(t, err)
	assert.Equal(t, target.data, got, "extracted content must match original")
	t.Logf("✅ Real S3 round-trip: %d files restored, %d chunks downloaded",
		stats.Restored, stats.ChunksDownloaded)
}

// ---------------------------------------------------------------------------
// Helpers
// ---------------------------------------------------------------------------

// sortDurations sorts in ascending order.
func sortDurations(d []time.Duration) {
	for i := 1; i < len(d); i++ {
		for j := i; j > 0 && d[j] < d[j-1]; j-- {
			d[j], d[j-1] = d[j-1], d[j]
		}
	}
}
