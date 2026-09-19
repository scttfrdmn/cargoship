//go:build integration && torture

package pipeline

// Ghostship / fleet data-path torture (#654).
//
// The base torture matrix covers the single-writer byte path. These subtests cover
// the FLEET data paths, which until now were only exercised against the in-process
// emulator by tests/e2e/ghostship_test.go (and there by basename, without SHA
// verification). Everything here verifies byte-identity BY RELATIVE PATH — the #486
// discipline — and runs against real S3 when the suite is pointed at it:
//
//	AWS_PROFILE=… AWS_REGION=us-west-2 CARGOSHIP_ENABLE_S3_INTEGRATION_TESTS=1 \
//	CARGOSHIP_TEST_BUCKET=<throwaway> \
//	go test -tags "integration torture" -run 'TestTorture/ghostship' -timeout 30m -count=1 ./pkg/pipeline/
//
// Manual/periodic like the rest of the matrix: the real-AWS CI lane builds with
// `-tags integration` only, so the `torture` tag keeps these out of it.

import (
	"context"
	"fmt"
	"math/rand"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/aws/aws-sdk-go-v2/aws"
	"github.com/aws/aws-sdk-go-v2/config"
	"github.com/aws/aws-sdk-go-v2/service/s3"
	"github.com/stretchr/testify/require"

	"github.com/scttfrdmn/cargoship/pkg/fleet"
	"github.com/scttfrdmn/cargoship/pkg/manifest"
)

// fleetTortureClient builds the S3 client the same way every other torture helper
// does: real AWS when substrateURL is empty, path-style against the emulator when not.
func fleetTortureClient(t *testing.T, region string) *s3.Client {
	t.Helper()
	cfg, err := config.LoadDefaultConfig(context.Background(), config.WithRegion(region))
	require.NoError(t, err)
	var s3Opts []func(*s3.Options)
	if substrateURL != "" {
		s3Opts = append(s3Opts, func(o *s3.Options) { o.UsePathStyle = true })
	}
	return s3.NewFromConfig(cfg, s3Opts...)
}

// writerUpload runs one pipeline upload as a fleet writer would: the prefix is
// writer-folded via WriterPrefix and WriterID is also recorded on the config (the
// two must be applied together — see pipeline.WriterPrefix's contract).
type writerUpload struct {
	bucket, region string
	base           string // shared fleet base prefix (NOT folded)
	writerID       string
	srcDir         string
	s3Client       *s3.Client
}

func (w writerUpload) effPrefix() string { return WriterPrefix(w.base, w.writerID) }

// run performs one cycle. includeOnly/syncType/prevID/deleted thread the incremental
// contract; empty syncType means a full sync.
func (w writerUpload) run(t *testing.T, uploadID string, includeOnly, deleted []string, syncType, prevID, datasetID string, ordinal int, forceChunked bool) *Result {
	t.Helper()
	pc := &PipelineConfig{
		ScannerWorkers: 2, ArchiverWorkers: 2, UploaderWorkers: 2,
		S3Bucket: w.bucket, S3Prefix: w.effPrefix(), S3Region: w.region,
		WriterID:  w.writerID,
		UseRealS3: true, S3Client: w.s3Client, S3PartSize: 5 * 1024 * 1024,
		EnableManifest: true, SourcePath: w.srcDir, UploadID: uploadID,
		EnableMultiPrefix: true, ShardCount: 2, FileChecksums: true,
		DatasetID: datasetID, VersionOrdinal: ordinal,
	}
	if syncType != "" {
		pc.SyncType, pc.PreviousUploadID, pc.IncludeOnlyFiles, pc.DeletedPaths = syncType, prevID, includeOnly, deleted
	}
	if forceChunked {
		pc.DirectUploadMaxFiles = 1 // the constructor resets EnableAutoDirectUpload
	}
	p, err := NewPipeline(pc)
	require.NoError(t, err)
	res, err := p.Run(context.Background(), w.srcDir)
	require.NoError(t, err)
	require.True(t, res.Success, "writer %s upload %s should succeed", w.writerID, uploadID)
	return res
}

// fetchWriterManifest reads a manifest from under the writer-folded prefix.
func (w writerUpload) fetchManifest(t *testing.T, uploadID string) *manifest.Manifest {
	t.Helper()
	m, err := manifest.DownloadFromS3(context.Background(), w.s3Client, w.bucket, w.effPrefix(), uploadID)
	require.NoError(t, err, "fetch manifest %s for writer %s", uploadID, w.writerID)
	return m
}

// requireBytesByRelPath restores the given manifest and asserts every expected file
// comes back byte-identical, keyed by RELATIVE PATH (never basename — #480/#475/#486).
func requireBytesByRelPath(t *testing.T, m *manifest.Manifest, s3Client *s3.Client, bucket string, want map[string]genFile, label string) string {
	t.Helper()
	outDir := t.TempDir()
	targets := make([]string, 0, len(want))
	for rel := range want {
		targets = append(targets, rel)
	}
	se := manifest.NewSelectiveExtractor(m, s3Client, 0).SetBucket(bucket)
	stats, err := se.BatchRestore(context.Background(), targets, outDir)
	require.NoError(t, err, "%s: restore failed", label)
	require.Equal(t, int64(len(want)), stats.Restored, "%s: every file should restore (failed=%d)", label, stats.Failed)
	require.Zero(t, stats.Failed, "%s: no restore failures expected", label)

	for rel, wf := range want {
		got, err := os.ReadFile(filepath.Join(outDir, filepath.FromSlash(rel)))
		require.NoError(t, err, "%s: restored file missing for %s", label, rel)
		require.Equal(t, wf.size, len(got), "%s: size mismatch for %s", label, rel)
		require.Equal(t, wf.sum, sha256hex(got),
			"%s: BYTE MISMATCH after round-trip for %s (integrity invariant failed)", label, rel)
	}
	return outDir
}

func corpusMap(corpus []genFile) map[string]genFile {
	m := make(map[string]genFile, len(corpus))
	for _, f := range corpus {
		m[f.relPath] = f
	}
	return m
}

// runGhostshipMultiWriterIsolation proves that several writers backing up
// CONCURRENTLY to one bucket under one shared base prefix stay fully isolated: each
// restores its own tree byte-identically, each manifest records its own writer id,
// and no writer's objects appear under another's prefix. Nothing in the suite
// exercised concurrent multi-writer before (#520/#604).
func runGhostshipMultiWriterIsolation(t *testing.T, rng *rand.Rand) {
	bucket := tortureEnv("CARGOSHIP_TEST_BUCKET", "cargoship-pipeline-test")
	region := tortureEnv("AWS_REGION", "us-east-1")
	s3Client := fleetTortureClient(t, region)
	ctx := context.Background()

	base := fmt.Sprintf("torture-fleet-multi-%d", time.Now().UnixNano())
	writerIDs := []string{"lab-nas-1", "lab-nas-2", "workstation-7"}

	// Each writer gets a DISTINCT corpus so a cross-writer leak shows up as a byte
	// mismatch, not a coincidental match.
	type wctx struct {
		up     writerUpload
		corpus []genFile
		upload string
	}
	writers := make([]*wctx, 0, len(writerIDs))
	for i, id := range writerIDs {
		src := t.TempDir()
		// Vary shape per writer: hostile paths, many small files, then hostile again.
		var corpus []genFile
		if i == 1 {
			corpus = plantManyFiles(t, src, rng, 60)
		} else {
			corpus = plantHostileCorpus(t, src, rng, 2048)
		}
		require.NotEmpty(t, corpus)
		writers = append(writers, &wctx{
			up:     writerUpload{bucket: bucket, region: region, base: base, writerID: id, srcDir: src, s3Client: s3Client},
			corpus: corpus,
			upload: fmt.Sprintf("%d-%s", time.Now().UnixNano(), id),
		})
	}

	// Upload all writers CONCURRENTLY — the interleaving is the point.
	var wg sync.WaitGroup
	for _, w := range writers {
		wg.Add(1)
		go func(w *wctx) {
			defer wg.Done()
			w.up.run(t, w.upload, nil, nil, "", "", "", 0, false)
		}(w)
	}
	wg.Wait()

	// Every writer restores its own tree byte-identically, and its manifest records
	// its own identity under its own folded prefix.
	for _, w := range writers {
		m := w.up.fetchManifest(t, w.upload)
		require.Equal(t, w.up.writerID, m.WriterID, "manifest should record writer %s", w.up.writerID)
		require.Equal(t, int64(len(w.corpus)), m.TotalFiles, "writer %s file count", w.up.writerID)
		requireBytesByRelPath(t, m, s3Client, bucket, corpusMap(w.corpus), "writer "+w.up.writerID)
	}

	// Physical isolation: every object under a writer's prefix belongs to that writer,
	// and each writer's subtree is non-empty.
	for _, w := range writers {
		pfx := w.up.effPrefix() + "/"
		var found int
		pager := s3.NewListObjectsV2Paginator(s3Client, &s3.ListObjectsV2Input{
			Bucket: aws.String(bucket), Prefix: aws.String(pfx),
		})
		for pager.HasMorePages() {
			page, err := pager.NextPage(ctx)
			require.NoError(t, err)
			for _, o := range page.Contents {
				key := aws.ToString(o.Key)
				found++
				for _, other := range writers {
					if other.up.writerID == w.up.writerID {
						continue
					}
					require.NotContains(t, key, "/writers/"+other.up.writerID+"/",
						"writer %s wrote into %s's subtree: %s", w.up.writerID, other.up.writerID, key)
				}
			}
		}
		require.Positive(t, found, "writer %s should have objects under %s", w.up.writerID, pfx)
	}

	// Breadcrumb for grepping a manual run log. Deliberately NOT wired into
	// scripts/ci/verification-report.sh: that report is generated by the real-AWS lane,
	// which builds with `-tags integration` only, so no torture marker can ever reach
	// it. Wiring a parser for a line that cannot appear would be misleading.
	t.Logf("VERIFICATION_FLEET_MULTIWRITER writers=%d base=%s", len(writers), base)
	t.Logf("ghostship multi-writer isolation OK: %d writers concurrent, each byte-identical and prefix-isolated", len(writers))
}

// runGhostshipSyncCycles drives REAL sync cycles at the library level — the
// composition `cargoship sync`/`ghostship run` performs
// (FindLatestManifestForSource → ScanLocalFiles → ComputeDelta → NextVersion →
// pipeline) — under a writer-folded prefix, then restores the chain-resolved latest
// version and byte-verifies the whole current dataset. No existing test composes
// those primitives, which is exactly the seam #624 broke (delta keyed by the wrong
// path form silently re-uploaded everything).
func runGhostshipSyncCycles(t *testing.T, rng *rand.Rand, forceChunked bool) {
	bucket := tortureEnv("CARGOSHIP_TEST_BUCKET", "cargoship-pipeline-test")
	region := tortureEnv("AWS_REGION", "us-east-1")
	s3Client := fleetTortureClient(t, region)
	ctx := context.Background()

	base := fmt.Sprintf("torture-fleet-sync-%d", time.Now().UnixNano())
	const writerID = "sync-writer-1"
	srcDir := t.TempDir()
	up := writerUpload{bucket: bucket, region: region, base: base, writerID: writerID, srcDir: srcDir, s3Client: s3Client}

	corpus := plantManyFiles(t, srcDir, rng, 40)
	require.NotEmpty(t, corpus)
	want := corpusMap(corpus)

	// cycle performs one real sync cycle and returns (uploadID, uploaded, syncType).
	// An empty uploadID means "no changes" — the cycle correctly uploaded nothing.
	var prevID string
	cycle := func(label string) (string, bool, string) {
		t.Helper()
		prevManifest, err := manifest.FindLatestManifestForSource(ctx, s3Client, bucket, up.effPrefix(), srcDir)
		syncType := manifest.SyncTypeFull
		if err == nil && prevManifest != nil {
			syncType = manifest.SyncTypeIncremental
		}
		local, err := manifest.ScanLocalFiles(srcDir)
		require.NoError(t, err, "%s: scan", label)
		delta, err := manifest.ComputeDelta(local, prevManifest, &manifest.SyncOptions{TrackDeletes: true})
		require.NoError(t, err, "%s: delta", label)
		if !delta.HasChanges() {
			return "", false, syncType
		}
		include := make([]string, 0, len(delta.GetChangedFiles()))
		for _, f := range delta.GetChangedFiles() {
			include = append(include, f.Path)
		}
		datasetID, ordinal := manifest.NextVersion(ctx, prevManifest, func(fctx context.Context, id string) (*manifest.Manifest, error) {
			return manifest.DownloadFromS3(fctx, s3Client, bucket, up.effPrefix(), id)
		})
		uploadID := fmt.Sprintf("%d-%s", time.Now().UnixNano(), label)
		st := ""
		if syncType == manifest.SyncTypeIncremental {
			st = syncType
		}
		up.run(t, uploadID, include, delta.Deleted, st, prevID, datasetID, ordinal, forceChunked)
		prevID = uploadID
		return uploadID, true, syncType
	}

	// Cycle 1 — nothing prior, so a full sync of everything.
	v1, uploaded, syncType := cycle("v1")
	require.True(t, uploaded, "first cycle must upload")
	require.Equal(t, manifest.SyncTypeFull, syncType, "first cycle should be a full sync")
	m1 := up.fetchManifest(t, v1)
	require.Equal(t, int64(len(corpus)), m1.TotalFiles, "full sync should carry every file")
	requireBytesByRelPath(t, m1, s3Client, bucket, want, "after v1")

	// Cycle 2 — NO local changes. The delta must be empty: this is the #624 guard.
	_, uploaded, syncType = cycle("v2-nochange")
	require.False(t, uploaded, "an unchanged tree must upload NOTHING (#624 regression guard)")
	require.Equal(t, manifest.SyncTypeIncremental, syncType, "second cycle should see the previous manifest")

	// Cycle 3 — modify some files, add some, delete one.
	changed := 0
	for i, f := range corpus {
		if i%7 != 0 {
			continue
		}
		body := []byte(fmt.Sprintf("rewritten %s at %d\n", f.relPath, time.Now().UnixNano()))
		require.NoError(t, os.WriteFile(filepath.Join(srcDir, filepath.FromSlash(f.relPath)), body, 0o600))
		want[f.relPath] = genFile{relPath: f.relPath, base: filepath.Base(f.relPath), sum: sha256hex(body), size: len(body)}
		changed++
	}
	require.Positive(t, changed, "test should modify at least one file")
	for i := 0; i < 5; i++ {
		rel := fmt.Sprintf("added/new%03d.dat", i)
		require.NoError(t, os.MkdirAll(filepath.Join(srcDir, "added"), 0o750))
		body := []byte(fmt.Sprintf("added file %d\n", i))
		require.NoError(t, os.WriteFile(filepath.Join(srcDir, filepath.FromSlash(rel)), body, 0o600))
		want[rel] = genFile{relPath: rel, base: filepath.Base(rel), sum: sha256hex(body), size: len(body)}
	}
	// Delete one file that is NOT in the modified set so it lives only in v1.
	var deletedRel string
	for i, f := range corpus {
		if i%7 == 0 {
			continue
		}
		deletedRel = f.relPath
		break
	}
	require.NotEmpty(t, deletedRel)
	require.NoError(t, os.Remove(filepath.Join(srcDir, filepath.FromSlash(deletedRel))))
	delete(want, deletedRel)

	v3, uploaded, _ := cycle("v3")
	require.True(t, uploaded, "a changed tree must upload")
	m3 := up.fetchManifest(t, v3)
	require.Equal(t, v1, m3.PreviousManifestID, "v3 should chain to v1 (v2 uploaded nothing)")
	require.Less(t, m3.TotalFiles, int64(len(want)), "an incremental manifest should be a partial view")

	// The chain-resolved latest version must reproduce the COMPLETE current dataset,
	// including unchanged files that live only in v1.
	eff, err := manifest.ResolveEffective(ctx, m3, func(fctx context.Context, id string) (*manifest.Manifest, error) {
		return manifest.DownloadFromS3(fctx, s3Client, bucket, up.effPrefix(), id)
	})
	require.NoError(t, err, "chain resolve")
	require.Equal(t, int64(len(want)), eff.TotalFiles,
		"effective manifest should describe the full current dataset (%d files)", len(want))

	outDir := requireBytesByRelPath(t, eff, s3Client, bucket, want, "after v3 (chain-resolved)")

	// The deleted file must be gone from the effective view and not restored.
	_, statErr := os.Stat(filepath.Join(outDir, filepath.FromSlash(deletedRel)))
	require.True(t, os.IsNotExist(statErr), "tombstoned file %s must not be restored", deletedRel)

	t.Logf("ghostship sync cycles OK: full→no-change→incremental under writers/%s, %d files byte-identical (chunked=%t)",
		writerID, len(want), forceChunked)
}

// runGhostshipWriteOnlyFullSync models the strict write-only agent: it has no read
// access to its own data, so it can never find its previous manifest and every cycle
// is a FULL sync to the same writer prefix (the documented #613 trade-off, confirmed
// against real IAM in #655). Repeated full uploads into one prefix must not corrupt
// each other — every version must still restore byte-identically.
func runGhostshipWriteOnlyFullSync(t *testing.T, rng *rand.Rand) {
	bucket := tortureEnv("CARGOSHIP_TEST_BUCKET", "cargoship-pipeline-test")
	region := tortureEnv("AWS_REGION", "us-east-1")
	s3Client := fleetTortureClient(t, region)

	base := fmt.Sprintf("torture-fleet-wo-%d", time.Now().UnixNano())
	const writerID = "write-only-1"
	srcDir := t.TempDir()
	up := writerUpload{bucket: bucket, region: region, base: base, writerID: writerID, srcDir: srcDir, s3Client: s3Client}

	corpus := plantHostileCorpus(t, srcDir, rng, 1024)
	require.NotEmpty(t, corpus)

	// Three cycles, each a FULL sync (never consulting a previous manifest), with the
	// tree mutating between them — exactly what a delete-free, read-free agent does.
	type snap struct {
		uploadID string
		want     map[string]genFile
	}
	snaps := make([]snap, 0, 3)
	want := corpusMap(corpus)
	for cycle := 1; cycle <= 3; cycle++ {
		if cycle > 1 {
			// Mutate one file and add one, so each cycle's dataset is distinct.
			f := corpus[cycle%len(corpus)]
			body := []byte(fmt.Sprintf("cycle %d rewrite of %s\n", cycle, f.relPath))
			require.NoError(t, os.WriteFile(filepath.Join(srcDir, filepath.FromSlash(f.relPath)), body, 0o600))
			want[f.relPath] = genFile{relPath: f.relPath, base: filepath.Base(f.relPath), sum: sha256hex(body), size: len(body)}

			rel := fmt.Sprintf("cycle%d.dat", cycle)
			nb := []byte(fmt.Sprintf("new in cycle %d\n", cycle))
			require.NoError(t, os.WriteFile(filepath.Join(srcDir, filepath.FromSlash(rel)), nb, 0o600))
			want[rel] = genFile{relPath: rel, base: rel, sum: sha256hex(nb), size: len(nb)}
		}
		uploadID := fmt.Sprintf("%d-wo%d", time.Now().UnixNano(), cycle)
		// Full sync: no syncType/prevID/includeOnly — the write-only shape.
		up.run(t, uploadID, nil, nil, "", "", "", 0, false)

		cur := make(map[string]genFile, len(want))
		for k, v := range want {
			cur[k] = v
		}
		snaps = append(snaps, snap{uploadID: uploadID, want: cur})
	}

	// EVERY full-sync version must independently restore byte-identically — a later
	// full upload to the same prefix must not corrupt an earlier one.
	for i, s := range snaps {
		m := up.fetchManifest(t, s.uploadID)
		require.Empty(t, m.PreviousManifestID, "cycle %d should be a standalone full sync", i+1)
		require.Equal(t, int64(len(s.want)), m.TotalFiles, "cycle %d file count", i+1)
		requireBytesByRelPath(t, m, s3Client, bucket, s.want, fmt.Sprintf("write-only cycle %d", i+1))
	}

	t.Logf("ghostship write-only full-sync OK: %d independent full cycles under writers/%s, all byte-identical",
		len(snaps), writerID)
}

// runGhostshipHeartbeatCoexistence proves the heartbeat object a fleet writer PUTs
// into its own data prefix (writers/<id>/status.json, #615) does not disturb the data
// path: manifest discovery, the effective dataset, and restore must all ignore it.
func runGhostshipHeartbeatCoexistence(t *testing.T, rng *rand.Rand) {
	bucket := tortureEnv("CARGOSHIP_TEST_BUCKET", "cargoship-pipeline-test")
	region := tortureEnv("AWS_REGION", "us-east-1")
	s3Client := fleetTortureClient(t, region)
	ctx := context.Background()

	base := fmt.Sprintf("torture-fleet-hb-%d", time.Now().UnixNano())
	const writerID = "hb-writer-1"
	srcDir := t.TempDir()
	up := writerUpload{bucket: bucket, region: region, base: base, writerID: writerID, srcDir: srcDir, s3Client: s3Client}

	corpus := plantManyFiles(t, srcDir, rng, 20)
	want := corpusMap(corpus)

	// Cycle 1, then a heartbeat, exactly as the daemon orders it.
	v1 := fmt.Sprintf("%d-hb1", time.Now().UnixNano())
	up.run(t, v1, nil, nil, "", "", "", 0, false)
	require.NoError(t, fleet.WriteStatus(ctx, s3Client, bucket, up.effPrefix(), fleet.WriterStatus{
		WriterID: writerID, Hostname: "torture", InstanceID: "inst-1",
		CargoshipVersion: "torture", UpdatedAt: time.Now(),
		Sources: []fleet.SourceStatus{{Path: srcDir, OK: true, UploadID: v1, Files: int64(len(corpus))}},
	}), "heartbeat write should succeed")

	// The heartbeat is physically inside the data prefix.
	hbKey := fleet.StatusKey(up.effPrefix())
	require.True(t, strings.HasPrefix(hbKey, up.effPrefix()+"/"), "heartbeat should live under the writer prefix")
	_, err := s3Client.HeadObject(ctx, &s3.HeadObjectInput{Bucket: aws.String(bucket), Key: aws.String(hbKey)})
	require.NoError(t, err, "heartbeat object should exist at %s", hbKey)

	// Manifest discovery must still find the upload with the heartbeat present.
	found, err := manifest.FindLatestManifestForSource(ctx, s3Client, bucket, up.effPrefix(), srcDir)
	require.NoError(t, err, "manifest discovery must ignore status.json")
	require.NotNil(t, found)
	require.Equal(t, v1, found.UploadID, "discovery should find the real upload, not the heartbeat")

	// A delta computed against that manifest must see NO changes (the heartbeat must
	// not register as a data file).
	local, err := manifest.ScanLocalFiles(srcDir)
	require.NoError(t, err)
	delta, err := manifest.ComputeDelta(local, found, &manifest.SyncOptions{TrackDeletes: true})
	require.NoError(t, err)
	require.False(t, delta.HasChanges(),
		"heartbeat must not appear as a change (new=%d modified=%d deleted=%d)",
		len(delta.New), len(delta.Modified), len(delta.Deleted))

	// And the data still restores byte-identically.
	requireBytesByRelPath(t, found, s3Client, bucket, want, "with heartbeat present")

	// The heartbeat itself is still readable + parseable by the control side.
	statuses, err := fleet.ListWriterStatuses(ctx, s3Client, bucket, base)
	require.NoError(t, err)
	require.Len(t, statuses, 1, "control side should see exactly one writer")
	require.Equal(t, writerID, statuses[0].WriterID)
	require.True(t, statuses[0].Healthy(), "writer should report healthy")

	t.Logf("ghostship heartbeat coexistence OK: status.json in the data prefix disturbs neither discovery, delta, nor restore")
}
