//go:build integration

package pipeline

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"math/rand"
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/aws/aws-sdk-go-v2/aws"
	"github.com/aws/aws-sdk-go-v2/config"
	"github.com/aws/aws-sdk-go-v2/service/s3"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/scttfrdmn/cargoship/pkg/manifest"
)

// genFile describes one planted file, with the SHA-256 of the exact bytes
// written so we can assert byte-identity after restore.
type genFile struct {
	relPath string
	base    string
	sum     string
	size    int
}

// TestRoundTripProperty is the whole-pipeline integrity invariant (#270 leg 3):
//
//	for any source tree:  upload → download → restore  ⇒  every file is
//	byte-identical to the original (same SHA-256), none missing or extra.
//
// It plants a deliberately hostile corpus — empty files, a large file, deep
// nesting, unicode/spaces/dotfiles in names, and both highly-compressible and
// incompressible content (the compressible case exposed the #275 tail-
// truncation bug). It runs the REAL pipeline against the emulator and restores
// through the REAL SelectiveExtractor.BatchRestore, exercising the true
// upload/restore path end to end.
//
// Two sub-tests cover both storage paths the pipeline can choose: a small
// corpus (average file < threshold → direct upload, TotalChunks == 0) and a
// large corpus (→ chunked tar.zst objects). The invariant must hold for both.
func TestRoundTripProperty(t *testing.T) {
	cases := []struct {
		name        string
		baseSize    int // size floor applied to every corpus file
		wantChunked bool
	}{
		// Tiny files → average below the 5MB threshold → direct upload.
		{"direct_upload", 0, false},
		// Every file padded to ≥6MB → average above threshold → chunked
		// tar.zst objects (mirrors manifest_integration_test.go's sizing).
		{"chunked_upload", 6 * 1024 * 1024, true},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			runRoundTrip(t, tc.baseSize, tc.wantChunked)
		})
	}
}

func runRoundTrip(t *testing.T, baseSize int, wantChunked bool) {
	bucket := os.Getenv("CARGOSHIP_TEST_BUCKET")
	if bucket == "" {
		bucket = "cargoship-pipeline-test"
	}
	region := os.Getenv("AWS_REGION")
	if region == "" {
		region = "us-east-1"
	}

	cfg, err := config.LoadDefaultConfig(context.Background(), config.WithRegion(region))
	require.NoError(t, err)
	var s3Opts []func(*s3.Options)
	if substrateURL != "" {
		s3Opts = append(s3Opts, func(o *s3.Options) { o.UsePathStyle = true })
	}
	s3Client := s3.NewFromConfig(cfg, s3Opts...)
	ctx := context.Background()

	rng := rand.New(rand.NewSource(0xCA6005417))
	srcDir := t.TempDir()
	corpus := plantHostileCorpus(t, srcDir, rng, baseSize)
	require.NotEmpty(t, corpus)

	testPrefix := fmt.Sprintf("roundtrip-%d", time.Now().UnixNano())
	uploadID := fmt.Sprintf("%d-rt", time.Now().UnixNano())
	pc := &PipelineConfig{
		ScannerWorkers: 2, ArchiverWorkers: 4, UploaderWorkers: 2,
		S3Bucket: bucket, S3Prefix: testPrefix, S3Region: region,
		UseRealS3: true, S3Client: s3Client, S3PartSize: 5 * 1024 * 1024,
		EnableManifest: true, SourcePath: srcDir, UploadID: uploadID,
		EnableMultiPrefix: true, ShardCount: 4, FileChecksums: true,
	}

	p, err := NewPipeline(pc)
	require.NoError(t, err)
	result, err := p.Run(ctx, srcDir)
	require.NoError(t, err)
	require.True(t, result.Success, "upload should succeed")

	// Download + parse the manifest.
	manifestKey := fmt.Sprintf("%s/uploads/%s/manifest.json.gz", testPrefix, uploadID)
	obj, err := s3Client.GetObject(ctx, &s3.GetObjectInput{Bucket: aws.String(bucket), Key: aws.String(manifestKey)})
	require.NoError(t, err, "manifest should be uploaded")
	mBytes, err := readAll(obj.Body)
	require.NoError(t, err)
	_ = obj.Body.Close()
	m, err := manifest.FromJSONCompressed(mBytes)
	require.NoError(t, err)

	require.Equal(t, int64(len(corpus)), m.TotalFiles, "manifest file count should match corpus")
	t.Logf("mode: TotalChunks=%d, files=%d (baseSize=%d bytes)", m.TotalChunks, m.TotalFiles, baseSize)
	// Confirm we actually exercised the intended storage path, so this test
	// keeps covering BOTH direct and chunked restore if thresholds change.
	if wantChunked {
		require.Greater(t, m.TotalChunks, 0, "expected chunked upload but got direct (TotalChunks=0)")
	} else {
		require.Equal(t, 0, m.TotalChunks, "expected direct upload but got chunked")
	}

	// Restore every file via the real BatchRestore path (handles both direct
	// and chunked manifests). It writes each file to destDir by BASENAME.
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

	// Index restored files by basename. BatchRestore's output layout differs by
	// mode (direct writes the basename into destDir; chunked preserves the
	// source path under destDir), so locate each file by basename wherever it
	// landed rather than assuming a layout.
	restoredByBase := indexFilesByBase(t, outDir)

	// Assert byte-identity for each planted file.
	for _, want := range corpus {
		path, ok := restoredByBase[want.base]
		require.True(t, ok, "restored file not found for %s", want.relPath)
		got, err := os.ReadFile(path)
		require.NoError(t, err)
		assert.Equal(t, want.size, len(got), "size mismatch for %s", want.relPath)
		assert.Equal(t, want.sum, sha256hex(got),
			"BYTE MISMATCH after round-trip for %s (integrity invariant failed)", want.relPath)
	}
	t.Logf("round-trip OK: %d files byte-identical across upload→restore", len(corpus))
}

// indexFilesByBase walks dir and maps each regular file's basename to its full
// path (basenames are unique in the test corpus).
func indexFilesByBase(t *testing.T, dir string) map[string]string {
	t.Helper()
	out := map[string]string{}
	require.NoError(t, filepath.Walk(dir, func(path string, info os.FileInfo, err error) error {
		if err != nil {
			return err
		}
		if !info.IsDir() {
			out[filepath.Base(path)] = path
		}
		return nil
	}))
	return out
}

// plantHostileCorpus writes a deliberately awkward set of files under root with
// unique basenames (BatchRestore flattens to basename) and returns their paths
// + content hashes. baseSize is a per-file size floor: 0 keeps the tiny/edge
// sizes (→ direct upload); a large floor pads every file above it so the
// average clears the direct-upload threshold (→ chunked upload). Files with an
// intrinsic size above the floor keep it.
func plantHostileCorpus(t *testing.T, root string, rng *rand.Rand, baseSize int) []genFile {
	t.Helper()
	specs := []struct {
		rel  string
		size int
		mode string // "zero", "random", "compressible"
	}{
		{"empty.dat", 0, "zero"},
		{"one-byte.dat", 1, "random"},
		{"small.txt", 4096, "compressible"},
		{"incompressible.bin", 512 * 1024, "random"},
		{"large-compressible.log", 512 * 1024, "compressible"}, // the #275 case
		{"nested/a/b/c/deep.txt", 2048, "compressible"},
		{"unicode/日本語/файл.dat", 1024, "random"},
		{"has spaces/my file (1).txt", 512, "compressible"},
		{"dir/.hidden", 256, "random"},
		{"dir2/UPPER.DAT", 3000, "random"},
	}

	seenBase := map[string]bool{}
	var out []genFile
	for _, s := range specs {
		base := filepath.Base(filepath.FromSlash(s.rel))
		require.False(t, seenBase[base], "corpus basenames must be unique for basename-flattened restore: %s", base)
		seenBase[base] = true

		abs := filepath.Join(root, filepath.FromSlash(s.rel))
		require.NoError(t, os.MkdirAll(filepath.Dir(abs), 0755))

		size := s.size
		if baseSize > size {
			size = baseSize // pad up to steer toward chunked upload
		}
		content := make([]byte, size)
		switch s.mode {
		case "zero":
			// zeros
		case "random":
			_, _ = rng.Read(content)
		case "compressible":
			pattern := []byte("cargoship-roundtrip-")
			for i := range content {
				content[i] = pattern[i%len(pattern)]
			}
		}
		require.NoError(t, os.WriteFile(abs, content, 0644))
		out = append(out, genFile{relPath: s.rel, base: base, sum: sha256hex(content), size: size})
	}
	return out
}

func sha256hex(b []byte) string {
	s := sha256.Sum256(b)
	return hex.EncodeToString(s[:])
}
