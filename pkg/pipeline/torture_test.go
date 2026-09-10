//go:build integration && torture

package pipeline

import (
	"context"
	"fmt"
	"math/rand"
	"os"
	"path/filepath"
	"strconv"
	"testing"
	"time"

	"github.com/aws/aws-sdk-go-v2/aws"
	"github.com/aws/aws-sdk-go-v2/config"
	"github.com/aws/aws-sdk-go-v2/service/s3"
	"github.com/stretchr/testify/require"

	"github.com/scttfrdmn/cargoship/pkg/manifest"
)

// TestTorture is the byte-exact round-trip torture matrix (trust program). Each
// subtest plants an adversarial corpus, runs the REAL pipeline against the
// in-process Substrate emulator (see TestMain in s3_integration_test.go),
// restores every file through the REAL SelectiveExtractor.BatchRestore, and
// asserts every restored file is byte-identical (SHA-256) to the original.
//
// It is SELECTABLE: run one dimension without the whole suite via
//
//	make torture RUN=large_multipart
//	  → go test -tags "integration torture" -run 'TestTorture/large_multipart' ...
//
// Emulator-only by default (TestMain stands up Substrate unless
// CARGOSHIP_ENABLE_S3_INTEGRATION_TESTS=1 points it at real S3). Reuses the
// round-trip helpers from roundtrip_property_test.go (genFile, plantHostileCorpus,
// indexFilesByBase, sha256hex) and readAll from manifest_integration_test.go.
func TestTorture(t *testing.T) {
	rng := rand.New(rand.NewSource(0x707C7E)) // deterministic corpora

	t.Run("hostile_paths", func(t *testing.T) {
		src := t.TempDir()
		corpus := plantHostileCorpus(t, src, rng, 0)
		tortureRoundTrip(t, corpus, src, nil)
	})

	t.Run("many_small_files", func(t *testing.T) {
		n := 1000 // scaled for the emulator; raise via TORTURE_FILE_COUNT for heavy/live runs
		if v := os.Getenv("TORTURE_FILE_COUNT"); v != "" {
			if parsed, err := strconv.Atoi(v); err == nil && parsed > 0 {
				n = parsed
			}
		}
		src := t.TempDir()
		corpus := plantManyFiles(t, src, rng, n)
		tortureRoundTrip(t, corpus, src, nil)
	})

	t.Run("large_multipart", func(t *testing.T) {
		// Files well above the 5 MiB part size force multipart uploads; random
		// content is incompressible so the stored objects are genuinely large.
		src := t.TempDir()
		corpus := plantSizedFiles(t, src, rng, []int{12 * 1024 * 1024, 7 * 1024 * 1024})
		tortureRoundTrip(t, corpus, src, func(pc *PipelineConfig) {
			pc.S3PartSize = 5 * 1024 * 1024
		})
	})

	t.Run("empty_and_tiny", func(t *testing.T) {
		src := t.TempDir()
		corpus := plantSizedFiles(t, src, rng, []int{0, 0, 1, 2, 3})
		tortureRoundTrip(t, corpus, src, nil)
	})

	// Every shard strategy must preserve bytes — the strategy only changes which
	// prefix a chunk lands under, never its content.
	for _, strat := range []string{
		ShardStrategyRoundRobin, ShardStrategyHash, ShardStrategySize,
		ShardStrategyType, ShardStrategyDirectory,
	} {
		strat := strat
		t.Run("shard_strategy_"+strat, func(t *testing.T) {
			src := t.TempDir()
			corpus := plantHostileCorpus(t, src, rng, 0)
			tortureRoundTrip(t, corpus, src, func(pc *PipelineConfig) {
				pc.ShardStrategy = strat
			})
		})
	}

	t.Run("compression_level_override_chunked", func(t *testing.T) {
		// Pin max compression and steer to the chunked tar.zst path.
		src := t.TempDir()
		corpus := plantHostileCorpus(t, src, rng, 6*1024*1024)
		tortureRoundTrip(t, corpus, src, func(pc *PipelineConfig) {
			pc.CompressionLevel = 19
		})
	})

	// #424: the wired congestion pacer must move bytes intact. Run the real
	// BBR-fed pacer on the upload path (chunked + multipart) and assert
	// byte-identity for each algorithm.
	for _, algo := range []string{"bbr", "cubic", "auto"} {
		algo := algo
		t.Run("congestion_"+algo, func(t *testing.T) {
			src := t.TempDir()
			corpus := plantHostileCorpus(t, src, rng, 6*1024*1024)
			tortureRoundTrip(t, corpus, src, func(pc *PipelineConfig) {
				pc.EnableOptimization = true
				pc.CongestionControl = algo
			})
		})
	}

	t.Run("idempotent_rerun", func(t *testing.T) {
		// Uploading the same corpus twice must round-trip byte-identically both
		// times (no partial/duplicated state corrupting the second run).
		src := t.TempDir()
		corpus := plantHostileCorpus(t, src, rng, 0)
		tortureRoundTrip(t, corpus, src, nil)
		tortureRoundTrip(t, corpus, src, nil)
	})
}

// tortureRoundTrip uploads srcDir through the real pipeline, restores every file
// via the real SelectiveExtractor, and asserts byte-identity. mutate lets a
// scenario tweak the pipeline config (shard count/strategy, part size,
// compression). It returns the parsed manifest for any extra assertions.
func tortureRoundTrip(t *testing.T, corpus []genFile, srcDir string, mutate func(*PipelineConfig)) *manifest.Manifest {
	t.Helper()
	require.NotEmpty(t, corpus)

	bucket := tortureEnv("CARGOSHIP_TEST_BUCKET", "cargoship-pipeline-test")
	region := tortureEnv("AWS_REGION", "us-east-1")

	cfg, err := config.LoadDefaultConfig(context.Background(), config.WithRegion(region))
	require.NoError(t, err)
	var s3Opts []func(*s3.Options)
	if substrateURL != "" {
		s3Opts = append(s3Opts, func(o *s3.Options) { o.UsePathStyle = true })
	}
	s3Client := s3.NewFromConfig(cfg, s3Opts...)
	ctx := context.Background()

	testPrefix := fmt.Sprintf("torture-%d", time.Now().UnixNano())
	uploadID := fmt.Sprintf("%d-tort", time.Now().UnixNano())
	pc := &PipelineConfig{
		ScannerWorkers: 4, ArchiverWorkers: 4, UploaderWorkers: 4,
		S3Bucket: bucket, S3Prefix: testPrefix, S3Region: region,
		UseRealS3: true, S3Client: s3Client, S3PartSize: 5 * 1024 * 1024,
		EnableManifest: true, SourcePath: srcDir, UploadID: uploadID,
		EnableMultiPrefix: true, ShardCount: 4, FileChecksums: true,
	}
	if mutate != nil {
		mutate(pc)
	}

	p, err := NewPipeline(pc)
	require.NoError(t, err)
	result, err := p.Run(ctx, srcDir)
	require.NoError(t, err)
	require.True(t, result.Success, "upload should succeed")

	// Fetch + parse the manifest, and confirm it accounts for every file.
	manifestKey := fmt.Sprintf("%s/uploads/%s/manifest.json.gz", testPrefix, uploadID)
	obj, err := s3Client.GetObject(ctx, &s3.GetObjectInput{Bucket: aws.String(bucket), Key: aws.String(manifestKey)})
	require.NoError(t, err, "manifest should be uploaded")
	mBytes, err := readAll(obj.Body)
	require.NoError(t, err)
	_ = obj.Body.Close()
	m, err := manifest.FromJSONCompressed(mBytes)
	require.NoError(t, err)
	require.Equal(t, int64(len(corpus)), m.TotalFiles, "manifest file count should match corpus")

	// Restore everything and assert byte-identity by basename.
	outDir := t.TempDir()
	se := manifest.NewSelectiveExtractor(m, s3Client, 0)
	targets := make([]string, len(corpus))
	for i, f := range corpus {
		targets[i] = f.relPath
	}
	stats, err := se.BatchRestore(ctx, targets, outDir)
	require.NoError(t, err)
	require.Equal(t, int64(len(corpus)), stats.Restored, "every file should restore (failed=%d)", stats.Failed)
	require.Zero(t, stats.Failed)

	byBase := indexFilesByBase(t, outDir)
	for _, want := range corpus {
		path, ok := byBase[want.base]
		require.True(t, ok, "restored file not found for %s", want.relPath)
		got, err := os.ReadFile(path)
		require.NoError(t, err)
		require.Equal(t, want.size, len(got), "size mismatch for %s", want.relPath)
		require.Equal(t, want.sum, sha256hex(got),
			"BYTE MISMATCH after round-trip for %s (integrity invariant failed)", want.relPath)
	}
	t.Logf("torture round-trip OK: %d files byte-identical (chunks=%d)", len(corpus), m.TotalChunks)
	return m
}

func tortureEnv(key, def string) string {
	if v := os.Getenv(key); v != "" {
		return v
	}
	return def
}

// plantManyFiles writes n small random files with unique basenames, spread
// across subdirectories so no single directory is enormous.
func plantManyFiles(t *testing.T, root string, rng *rand.Rand, n int) []genFile {
	t.Helper()
	out := make([]genFile, 0, n)
	for i := 0; i < n; i++ {
		base := fmt.Sprintf("f%06d.dat", i)
		rel := filepath.Join(fmt.Sprintf("d%03d", i/500), base)
		abs := filepath.Join(root, rel)
		require.NoError(t, os.MkdirAll(filepath.Dir(abs), 0755))
		size := 1 + rng.Intn(4096)
		content := make([]byte, size)
		_, _ = rng.Read(content)
		require.NoError(t, os.WriteFile(abs, content, 0644))
		out = append(out, genFile{relPath: rel, base: base, sum: sha256hex(content), size: size})
	}
	return out
}

// plantSizedFiles writes one incompressible file per requested size, with unique
// basenames. Size 0 produces an empty file.
func plantSizedFiles(t *testing.T, root string, rng *rand.Rand, sizes []int) []genFile {
	t.Helper()
	out := make([]genFile, 0, len(sizes))
	for i, sz := range sizes {
		base := fmt.Sprintf("sized%02d.bin", i)
		abs := filepath.Join(root, base)
		content := make([]byte, sz)
		_, _ = rng.Read(content)
		require.NoError(t, os.WriteFile(abs, content, 0644))
		out = append(out, genFile{relPath: base, base: base, sum: sha256hex(content), size: sz})
	}
	return out
}
