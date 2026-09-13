//go:build integration && torture

package pipeline

import (
	"context"
	"fmt"
	"math/rand"
	"os"
	"path/filepath"
	"strconv"
	"strings"
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

	// #480/#475: files that share a basename across directories (README.md,
	// index.js) must each restore their OWN bytes. Small corpus → direct-upload
	// path, where keying by basename silently corrupted one file. The relative-
	// path verification in tortureRoundTrip is what makes this assertion real.
	t.Run("repeated_basenames", func(t *testing.T) {
		src := t.TempDir()
		corpus := plantRepeatedBasenames(t, src)
		tortureRoundTrip(t, corpus, src, nil)
	})

	// #481: content-duplicate files under --enable-dedup must all restore. The
	// duplicate entries carried a placeholder empty S3Key until patched post-
	// upload; unpatched, restore aborted entirely.
	t.Run("dedup_duplicates", func(t *testing.T) {
		src := t.TempDir()
		corpus := plantDuplicateContent(t, src, rng)
		tortureRoundTrip(t, corpus, src, func(pc *PipelineConfig) {
			pc.EnableDeduplication = true
		})
	})

	// #468: a chunked upload spanning MORE THAN ONE scan batch (batchSize=1000)
	// must not collide chunk IDs/S3 keys across batches. Force the packed path
	// (negative threshold) with >1000 files so ≥2 batches run.
	t.Run("multi_batch_packed", func(t *testing.T) {
		src := t.TempDir()
		corpus := plantManyFiles(t, src, rng, 1500)
		tortureRoundTrip(t, corpus, src, func(pc *PipelineConfig) {
			pc.DirectUploadThresholdMB = -1 // never take the direct path → chunked, multi-batch
		})
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

	// #436: the format-2.1 frame index. A small --frame-size against compressible
	// files larger than it forces multiple frames per chunk; every file must
	// still round-trip byte-identically, the manifest must advertise the frame
	// index, and a single-file restore must fetch only that file's frame.
	t.Run("frames_random_access", func(t *testing.T) {
		// Files with a >5 MiB average steer the pipeline onto the chunked tar.zst
		// path (not the small-dataset direct-upload fast path, which never frames).
		src := t.TempDir()
		corpus := plantCompressibleFiles(t, src, []int{8 * 1024 * 1024, 6 * 1024 * 1024, 10 * 1024 * 1024})
		m := tortureRoundTrip(t, corpus, src, func(pc *PipelineConfig) {
			pc.FrameSize = 1024 * 1024 // small vs the files: forces several frames per chunk
		})

		require.Contains(t, m.FormatFeatures, manifest.FormatFeatureFrames,
			"manifest must advertise the frame index")
		framedChunks, multiFrame := 0, 0
		for _, c := range m.Chunks {
			if len(c.Frames) > 0 {
				framedChunks++
			}
			if len(c.Frames) > 1 {
				multiFrame++
			}
		}
		require.Positive(t, framedChunks, "at least one chunk must carry a frame index")
		require.Positive(t, multiFrame, "a 64KiB frame size over >=150KiB files must cut multiple frames")

		// #502: sub-frame cutting means a single file LARGER than frameSize spans
		// many frames. Pre-#502 the framer cut only at file boundaries, so this
		// 24 MiB corpus over 1 MiB frames gave ~3 frames (one per file); now each
		// large file is cut into several, so the total far exceeds the file count.
		totalFrames := 0
		for _, c := range m.Chunks {
			totalFrames += len(c.Frames)
		}
		require.Greater(t, totalFrames, len(corpus)*3,
			"a large file must be sub-framed into many frames, not one frame per file (#502)")
		for _, f := range m.Files {
			require.Positive(t, f.ArchiveOffset, "file %s must have a recorded archive offset", f.Path)
		}
		// CompressedSize is now the true streamed byte count, not the uncompressed
		// estimate — for this highly-compressible corpus it must be well under the
		// uncompressed size, and equal the sum of the chunk's frame sizes.
		for _, c := range m.Chunks {
			require.Positive(t, c.CompressedSize)
			require.Less(t, c.CompressedSize, c.UncompressedSize, "compressible chunk must record a real (smaller) compressed size")
			if len(c.Frames) > 0 {
				var framesTotal int64
				for _, fr := range c.Frames {
					framesTotal += fr.CompressedSize
				}
				require.Equal(t, c.CompressedSize, framesTotal, "chunk compressed size must equal the sum of its frame sizes")
				// #522: a framed chunk carries NO whole-object checksum — its
				// per-frame checksums are the integrity. Pre-#522 this was a
				// redundant non-empty SHA-256 over the same bytes.
				require.Empty(t, c.Checksum, "framed chunk must omit the redundant whole-object checksum (#522)")
				for _, fr := range c.Frames {
					require.NotEmpty(t, fr.Checksum, "each frame must carry its own checksum")
				}
			}
		}

		// Single-file restore exercises the ranged frame fast path end-to-end.
		region := tortureEnv("AWS_REGION", "us-east-1")
		cfg, err := config.LoadDefaultConfig(context.Background(), config.WithRegion(region))
		require.NoError(t, err)
		var s3Opts []func(*s3.Options)
		if substrateURL != "" {
			s3Opts = append(s3Opts, func(o *s3.Options) { o.UsePathStyle = true })
		}
		s3Client := s3.NewFromConfig(cfg, s3Opts...)
		outDir := t.TempDir()
		se := manifest.NewSelectiveExtractor(m, s3Client, 0)
		stats, err := se.BatchRestore(context.Background(), []string{corpus[0].relPath}, outDir)
		require.NoError(t, err)
		require.Equal(t, int64(1), stats.Restored)
		require.Zero(t, stats.Failed)
		got, err := os.ReadFile(filepath.Join(outDir, corpus[0].relPath))
		require.NoError(t, err)
		require.Equal(t, corpus[0].sum, sha256hex(got), "single-file frame restore must be byte-identical")
	})

	// #452: a MIXED-compressibility corpus produces both .tar.zst and plain .tar
	// chunks under one manifest compression_type. Every file must round-trip —
	// the bug (decoding by the top-level field) left files in the plain .tar
	// chunks unrestorable, which homogeneous corpora never exposed.
	t.Run("mixed_compressibility", func(t *testing.T) {
		src := t.TempDir()
		// Compressible (.log) + already-compressed extensions (.zip, which the
		// detector skips → plain .tar). Files ~6 MiB with a 4 MiB forced chunk
		// size → each file is its own chunk, so the upload yields BOTH .tar.zst
		// (the .log files, which also set the manifest's top-level type to zstd)
		// and plain .tar chunks (the .zip files) — the #452 mixed case.
		corpus := plantCompressibleFiles(t, src, []int{6 * 1024 * 1024, 6 * 1024 * 1024})
		corpus = append(corpus, plantAlreadyCompressedFiles(t, src, rng, []int{6 * 1024 * 1024, 6 * 1024 * 1024})...)
		// tortureRoundTrip restores ALL files and asserts byte-identity, so if the
		// chunker produces a plain .tar chunk under a zstd manifest (the #452
		// shape), a regression there fails here. The deterministic guarantee of
		// the decode-by-extension fix lives in the manifest unit test
		// (TestRestoreDecodesPlainTarChunkInMixedUpload); the chunker's exact
		// chunk-kind split isn't controllable from here, so we log it, not assert.
		m := tortureRoundTrip(t, corpus, src, func(pc *PipelineConfig) {
			pc.ForceChunkSizeMB = 4
		})
		var zst, plain int
		for _, c := range m.Chunks {
			switch {
			case strings.HasSuffix(c.S3Key, ".tar.zst"):
				zst++
				// #522: framed .tar.zst chunks omit the whole-object checksum.
				require.Empty(t, c.Checksum, "framed .tar.zst chunk must omit the redundant whole-object checksum (#522)")
			case strings.HasSuffix(c.S3Key, ".tar"):
				plain++
				// #522: plain (unframed) chunks keep the whole-object checksum as
				// their only object-level integrity (no frames cover them).
				require.NotEmpty(t, c.Checksum, "plain .tar chunk must keep its whole-object checksum (#522)")
			}
		}
		t.Logf("mixed corpus chunk kinds: %d compressed (.tar.zst), %d plain (.tar)", zst, plain)
	})

	// #119: resume must skip chunks already uploaded and never corrupt the data.
	t.Run("resume_skips_completed", func(t *testing.T) {
		runResumeTorture(t, rng)
	})

	t.Run("idempotent_rerun", func(t *testing.T) {
		// Uploading the same corpus twice must round-trip byte-identically both
		// times (no partial/duplicated state corrupting the second run).
		src := t.TempDir()
		corpus := plantHostileCorpus(t, src, rng, 0)
		tortureRoundTrip(t, corpus, src, nil)
		tortureRoundTrip(t, corpus, src, nil)
	})

	// #552: after an incremental sync, restoring the LATEST version must recover
	// the full dataset — unchanged files (which live only in the parent upload)
	// included — via the PreviousManifestID chain merge (manifest.ResolveEffective).
	t.Run("incremental_chain_restore", func(t *testing.T) {
		// Cover both upload modes: direct (FileEntry.S3Key is the raw object) and
		// chunked (files packed into tar.zst chunks whose ChunkEntry must be
		// carried forward by the merge).
		t.Run("direct", func(t *testing.T) { runIncrementalChainTorture(t, rng, false) })
		t.Run("chunked", func(t *testing.T) { runIncrementalChainTorture(t, rng, true) })
	})
}

// runIncrementalChainTorture is the end-to-end #552 trust proof: a full sync
// then an incremental sync (delta-only upload chained via PreviousManifestID),
// then a chain-resolved restore of the LATEST version that must reproduce the
// COMPLETE current dataset byte-for-byte — unchanged files (stored only in the
// parent upload) included. It also asserts the newest manifest alone is a
// partial (delta) view, so the merge is doing real work.
func runIncrementalChainTorture(t *testing.T, rng *rand.Rand, forceChunked bool) {
	t.Helper()

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

	srcDir := t.TempDir()
	testPrefix := fmt.Sprintf("torture-incr-%d", time.Now().UnixNano())

	// upload runs one pipeline pass against the shared bucket/prefix. includeOnly
	// (relative paths) restricts an incremental pass to the delta; prevID chains
	// the manifest.
	upload := func(uploadID string, includeOnly []string, syncType, prevID string) {
		pc := &PipelineConfig{
			ScannerWorkers: 4, ArchiverWorkers: 4, UploaderWorkers: 4,
			S3Bucket: bucket, S3Prefix: testPrefix, S3Region: region,
			UseRealS3: true, S3Client: s3Client, S3PartSize: 5 * 1024 * 1024,
			EnableManifest: true, SourcePath: srcDir, UploadID: uploadID,
			EnableMultiPrefix: true, ShardCount: 4, FileChecksums: true,
			IncludeOnlyFiles: includeOnly, SyncType: syncType, PreviousUploadID: prevID,
		}
		if forceChunked {
			// Route through the archiver (tar.zst chunks) instead of the
			// small-file direct-upload fast path, so the merge must carry
			// ancestor ChunkEntry+archive_offset forward. NewPipeline resets
			// EnableAutoDirectUpload to true, so force the chunked path via the
			// max-files gate instead (a nonzero value survives the constructor).
			pc.DirectUploadMaxFiles = 1
		}
		p, err := NewPipeline(pc)
		require.NoError(t, err)
		result, err := p.Run(ctx, srcDir)
		require.NoError(t, err)
		require.True(t, result.Success, "upload %s should succeed", uploadID)
	}

	fetchManifest := func(uploadID string) *manifest.Manifest {
		key := fmt.Sprintf("%s/uploads/%s/manifest.json.gz", testPrefix, uploadID)
		obj, err := s3Client.GetObject(ctx, &s3.GetObjectInput{Bucket: aws.String(bucket), Key: aws.String(key)})
		require.NoError(t, err, "manifest for %s should exist", uploadID)
		b, err := readAll(obj.Body)
		require.NoError(t, err)
		_ = obj.Body.Close()
		m, err := manifest.FromJSONCompressed(b)
		require.NoError(t, err)
		return m
	}

	// v1: full upload of a small corpus.
	v1corpus := plantManyFiles(t, srcDir, rng, 40)
	v1ID := fmt.Sprintf("%d-v1", time.Now().UnixNano())
	upload(v1ID, nil, "full", "")

	// Mutate the tree: rewrite content of a subset (modified) and add a few new
	// files; the rest stay unchanged. Build the expected FINAL dataset by path.
	final := make(map[string]genFile, len(v1corpus))
	for _, f := range v1corpus {
		final[f.relPath] = f
	}
	var delta []string
	for i, f := range v1corpus {
		if i%4 != 0 { // modify every 4th file
			continue
		}
		content := make([]byte, 1+rng.Intn(8192))
		_, _ = rng.Read(content)
		require.NoError(t, os.WriteFile(filepath.Join(srcDir, filepath.FromSlash(f.relPath)), content, 0644))
		final[f.relPath] = genFile{relPath: f.relPath, base: f.base, sum: sha256hex(content), size: len(content)}
		delta = append(delta, f.relPath)
	}
	for i := 0; i < 5; i++ { // add new files
		rel := filepath.ToSlash(filepath.Join("added", fmt.Sprintf("n%03d.dat", i)))
		content := make([]byte, 1+rng.Intn(8192))
		_, _ = rng.Read(content)
		abs := filepath.Join(srcDir, filepath.FromSlash(rel))
		require.NoError(t, os.MkdirAll(filepath.Dir(abs), 0755))
		require.NoError(t, os.WriteFile(abs, content, 0644))
		final[rel] = genFile{relPath: rel, base: filepath.Base(rel), sum: sha256hex(content), size: len(content)}
		delta = append(delta, rel)
	}

	// v2: incremental upload of ONLY the delta, chained to v1.
	v2ID := fmt.Sprintf("%d-v2", time.Now().UnixNano())
	upload(v2ID, delta, "incremental", v1ID)

	// The newest manifest alone is a partial (delta) view — this is the bug's root.
	m2 := fetchManifest(v2ID)
	require.Equal(t, v1ID, m2.PreviousManifestID, "v2 must chain to v1")
	require.Less(t, m2.TotalFiles, int64(len(final)),
		"v2 manifest should hold only the delta (%d), not the full dataset (%d)", m2.TotalFiles, len(final))

	// Resolve the chain into the full-dataset effective view.
	eff, err := manifest.ResolveEffective(ctx, m2, func(_ context.Context, id string) (*manifest.Manifest, error) {
		return fetchManifest(id), nil
	})
	require.NoError(t, err)
	require.Equal(t, int64(len(final)), eff.TotalFiles,
		"effective view must describe the FULL dataset (%d files)", len(final))

	// Restore the effective view and assert byte-identity of the CURRENT state:
	// unchanged files come from v1's chunks, modified/new from v2's.
	outDir := t.TempDir()
	se := manifest.NewSelectiveExtractor(eff, s3Client, 0)
	targets := make([]string, 0, len(final))
	for rel := range final {
		targets = append(targets, rel)
	}
	stats, err := se.BatchRestore(ctx, targets, outDir)
	require.NoError(t, err)
	require.Equal(t, int64(len(final)), stats.Restored, "every current file must restore (failed=%d)", stats.Failed)
	require.Zero(t, stats.Failed)

	for rel, want := range final {
		got, err := os.ReadFile(filepath.Join(outDir, filepath.FromSlash(rel)))
		require.NoError(t, err, "restored file not found for %s", rel)
		require.Equal(t, want.size, len(got), "size mismatch for %s", rel)
		require.Equal(t, want.sum, sha256hex(got),
			"BYTE MISMATCH after incremental-chain restore for %s (#552 invariant failed)", rel)
	}
	t.Logf("incremental-chain restore OK: %d files byte-identical (v2 delta=%d)", len(final), m2.TotalFiles)
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

	// #493: each chunk object appears exactly once — no duplicate ChunkEntry per
	// s3_key. Staging snapshots or chunk-ID collisions (#468) would add partial
	// extras that a strict manifest reader (lith) rejects.
	seenKey := map[string]bool{}
	for _, c := range m.Chunks {
		require.False(t, seenKey[c.S3Key], "duplicate ChunkEntry for s3_key %q (#493)", c.S3Key)
		seenKey[c.S3Key] = true
	}
	// #492: in a chunked upload, every file records its archive_offset — including
	// files in a plain .tar chunk — so a random-access reader can range-GET a file
	// without walking tar headers. (Direct uploads have no chunks; split parts use
	// the whole-chunk path.)
	if len(m.Chunks) > 0 {
		for _, f := range m.Files {
			if f.IsDuplicate || f.TotalParts > 1 {
				continue
			}
			require.Positive(t, f.ArchiveOffset, "file %s missing archive_offset (#492)", f.Path)
		}
	}

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

	// Verify by RELATIVE PATH, not basename: restore preserves directory
	// structure (restorePath), and keying on basename hid same-basename
	// collisions (#480/#475) — the exact blind spot this suite must not have.
	for _, want := range corpus {
		got, err := os.ReadFile(filepath.Join(outDir, filepath.FromSlash(want.relPath)))
		require.NoError(t, err, "restored file not found for %s", want.relPath)
		require.Equal(t, want.size, len(got), "size mismatch for %s", want.relPath)
		require.Equal(t, want.sum, sha256hex(got),
			"BYTE MISMATCH after round-trip for %s (integrity invariant failed)", want.relPath)
	}
	t.Logf("torture round-trip OK: %d files byte-identical (chunks=%d)", len(corpus), m.TotalChunks)
	return m
}

// runResumeTorture proves the #119 wiring produces a working, non-corrupting
// resume: a resume-mode run of a prior upload (same UploadID) completes and the
// data still restores byte-identically. It does NOT assert skip efficiency —
// demonstrating a partial skip deterministically requires an interrupted run,
// and seeding an artificial partial manifest exposed a separate #157 rough edge
// (files re-added / ChunksSkipped not reflected in Result) tracked separately.
func runResumeTorture(t *testing.T, rng *rand.Rand) {
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

	src := t.TempDir()
	corpus := plantHostileCorpus(t, src, rng, 0)
	prefix := fmt.Sprintf("resume-%d", time.Now().UnixNano())
	uploadID := fmt.Sprintf("%d-resume", time.Now().UnixNano())

	newCfg := func(resume bool) *PipelineConfig {
		resumeID := ""
		if resume {
			resumeID = uploadID
		}
		return &PipelineConfig{
			ScannerWorkers: 2, ArchiverWorkers: 4, UploaderWorkers: 4,
			S3Bucket: bucket, S3Prefix: prefix, S3Region: region,
			UseRealS3: true, S3Client: s3Client, S3PartSize: 5 * 1024 * 1024,
			EnableManifest: true, EnablePartialManifest: true, SourcePath: src,
			UploadID: uploadID, EnableMultiPrefix: true, ShardCount: 4, FileChecksums: true,
			ResumeMode: resume, ResumeUploadID: resumeID,
		}
	}

	// Run 1: full upload.
	p1, err := NewPipeline(newCfg(false))
	require.NoError(t, err)
	r1, err := p1.Run(ctx, src)
	require.NoError(t, err)
	require.True(t, r1.Success)

	// Run 2: resume mode (same UploadID). Must complete without error.
	p2, err := NewPipeline(newCfg(true))
	require.NoError(t, err)
	r2, err := p2.Run(ctx, src)
	require.NoError(t, err)
	require.True(t, r2.Success, "resume-mode run must complete")

	// After the resume, the data must still restore byte-identically.
	finalKey := fmt.Sprintf("%s/uploads/%s/manifest.json.gz", prefix, uploadID)
	obj, err := s3Client.GetObject(ctx, &s3.GetObjectInput{Bucket: aws.String(bucket), Key: aws.String(finalKey)})
	require.NoError(t, err)
	mBytes, err := readAll(obj.Body)
	require.NoError(t, err)
	_ = obj.Body.Close()
	m, err := manifest.FromJSONCompressed(mBytes)
	require.NoError(t, err)

	outDir := t.TempDir()
	se := manifest.NewSelectiveExtractor(m, s3Client, 0)
	targets := make([]string, len(corpus))
	for i, f := range corpus {
		targets[i] = f.relPath
	}
	stats, err := se.BatchRestore(ctx, targets, outDir)
	require.NoError(t, err)
	require.Zero(t, stats.Failed)
	byBase := indexFilesByBase(t, outDir)
	for _, want := range corpus {
		path, ok := byBase[want.base]
		require.True(t, ok, "restored file not found for %s", want.relPath)
		got, err := os.ReadFile(path)
		require.NoError(t, err)
		require.Equal(t, want.sum, sha256hex(got), "BYTE MISMATCH after resume for %s", want.relPath)
	}
	t.Logf("resume-mode round-trip OK: %d files byte-identical (run1 chunks=%d, resume chunks=%d)", len(corpus), r1.ChunksUploaded, r2.ChunksUploaded)
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

// plantRepeatedBasenames writes files that deliberately SHARE a basename across
// different directories with different content — the shape that direct-upload
// keyed by basename silently corrupted (#480), and that basename-indexed verify
// couldn't even check (#475).
func plantRepeatedBasenames(t *testing.T, root string) []genFile {
	t.Helper()
	var out []genFile
	for _, dir := range []string{"dirA", "dirB", "dirC"} {
		for _, base := range []string{"README.md", "index.js"} {
			rel := filepath.Join(dir, base)
			abs := filepath.Join(root, rel)
			require.NoError(t, os.MkdirAll(filepath.Dir(abs), 0755))
			content := []byte(fmt.Sprintf("distinct content for %s\n", rel))
			require.NoError(t, os.WriteFile(abs, content, 0644))
			out = append(out, genFile{relPath: rel, base: base, sum: sha256hex(content), size: len(content)})
		}
	}
	return out
}

// plantDuplicateContent writes several files with IDENTICAL content (unique
// basenames) plus one unique file, to exercise the --enable-dedup path (#481).
func plantDuplicateContent(t *testing.T, root string, rng *rand.Rand) []genFile {
	t.Helper()
	shared := make([]byte, 4096)
	_, _ = rng.Read(shared)
	uniq := make([]byte, 4096)
	_, _ = rng.Read(uniq)
	specs := []struct {
		rel     string
		content []byte
	}{
		{"a/dup1.bin", shared}, {"b/dup2.bin", shared}, {"c/dup3.bin", shared}, {"unique.bin", uniq},
	}
	var out []genFile
	for _, s := range specs {
		abs := filepath.Join(root, s.rel)
		require.NoError(t, os.MkdirAll(filepath.Dir(abs), 0755))
		require.NoError(t, os.WriteFile(abs, s.content, 0644))
		out = append(out, genFile{relPath: s.rel, base: filepath.Base(s.rel), sum: sha256hex(s.content), size: len(s.content)})
	}
	return out
}

// plantCompressibleFiles writes files of compressible (patterned) content with a
// .log extension so the archiver takes the zstd path — required for the #436
// frame index to be emitted. Each file gets a unique prefix so their bytes (and
// checksums) differ while staying highly compressible.
func plantCompressibleFiles(t *testing.T, root string, sizes []int) []genFile {
	t.Helper()
	out := make([]genFile, 0, len(sizes))
	for i, sz := range sizes {
		base := fmt.Sprintf("frames%02d.log", i)
		abs := filepath.Join(root, base)
		content := make([]byte, sz)
		pattern := []byte(fmt.Sprintf("cargoship frame-index test line %02d — the quick brown fox\n", i))
		for j := range content {
			content[j] = pattern[j%len(pattern)]
		}
		require.NoError(t, os.WriteFile(abs, content, 0644))
		out = append(out, genFile{relPath: base, base: base, sum: sha256hex(content), size: sz})
	}
	return out
}

// plantAlreadyCompressedFiles writes random-content files with a .zip extension,
// which the compression detector treats as already-compressed → the archiver
// writes them as a plain .tar chunk (no zstd). Used to exercise the mixed
// .tar.zst + .tar case (#452).
func plantAlreadyCompressedFiles(t *testing.T, root string, rng *rand.Rand, sizes []int) []genFile {
	t.Helper()
	out := make([]genFile, 0, len(sizes))
	for i, sz := range sizes {
		base := fmt.Sprintf("blob%02d.zip", i)
		abs := filepath.Join(root, base)
		content := make([]byte, sz)
		_, _ = rng.Read(content)
		require.NoError(t, os.WriteFile(abs, content, 0644))
		out = append(out, genFile{relPath: base, base: base, sum: sha256hex(content), size: sz})
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
