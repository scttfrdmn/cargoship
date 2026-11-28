package cmd

import (
	"encoding/json"
	"reflect"
	"testing"

	"github.com/aws/aws-sdk-go-v2/service/s3"
	"github.com/aws/aws-sdk-go-v2/service/s3/types"
	"github.com/dustin/go-humanize"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/scttfrdmn/cargoship/pkg/pipeline"
	"github.com/scttfrdmn/cargoship/pkg/resume"
)

// loadStateJSON parses a state file body the way resume.LoadState does.
func loadStateJSON(t *testing.T, body string) *resume.UploadState {
	t.Helper()
	var st resume.UploadState
	require.NoError(t, json.Unmarshal([]byte(body), &st))
	return &st
}

// TestNewResumePipelineConfig_OldStateKeepsUploadDefaults pins #1011 for state
// files written before the upload settings were recorded: `cargoship resume`
// rebuilt the config with Go zero values, so a resumed upload stored no per-file
// checksums (restore can't verify those chunks) and dropped the 16 MiB
// random-access frames. A state without settings must resume with the upload
// command's defaults.
func TestNewResumePipelineConfig_OldStateKeepsUploadDefaults(t *testing.T) {
	st := loadStateJSON(t, `{"upload_id":"20260101-old","source_dir":"/data","bucket":"b","prefix":"p","shard_count":8}`)
	cfg := newResumePipelineConfig(st, &s3.Client{}, nil)
	assert.True(t, cfg.FileChecksums, "per-file checksums are on by default for upload; a resume must keep them")

	def, err := humanize.ParseBytes(NewUploadCmd().Flags().Lookup("frame-size").DefValue)
	require.NoError(t, err)
	assert.Equal(t, int64(def), cfg.FrameSize, "frame size must be the upload default")
}

// TestNewResumePipelineConfig_RestoresSavedSettings pins #1011: every upload
// setting the state records must reach the resumed pipeline. Resuming an
// interrupted sync or fleet upload dropped its exclusions (uploading excluded
// files, e.g. secrets or a NAS #recycle), its change set and chain identity,
// writer and project attribution, dedup, and the format knobs.
func TestNewResumePipelineConfig_RestoresSavedSettings(t *testing.T) {
	st := loadStateJSON(t, `{
  "upload_id": "20260101-new", "source_dir": "/data", "bucket": "b", "prefix": "p/writers/nas-1",
  "storage_class": "STANDARD_IA", "shard_count": 6,
  "settings": {
    "file_checksums": false,
    "frame_size": 4194304,
    "compression_level": 9,
    "shard_strategy": "size",
    "exclude_patterns": ["#recycle", "*.secret"],
    "include_only_files": ["/data/a.txt", "/data/sub/b.txt"],
    "sync_type": "incremental",
    "previous_upload_id": "20251231-prev",
    "deleted_paths": ["/data/gone.txt"],
    "dataset_id": "20251201-root",
    "version_ordinal": 4,
    "writer_id": "nas-1",
    "project_id": "proj-7",
    "tags": {"team": "genomics"},
    "enable_deduplication": true,
    "tier_chunking_strategy": "tier-aware",
    "tier": {"hot_days": 10, "cold_days": 60, "archive_days": 200, "max_tier": "GLACIER", "default_class": "STANDARD", "fallback_to_mtime": true}
  }
}`)
	cfg := newResumePipelineConfig(st, &s3.Client{}, nil)

	assert.False(t, cfg.FileChecksums, "an explicit --no-file-checksums must survive resume")
	assert.Equal(t, int64(4<<20), cfg.FrameSize)
	assert.Equal(t, 9, cfg.CompressionLevel)
	assert.Equal(t, "size", cfg.ShardStrategy)
	assert.Equal(t, []string{"#recycle", "*.secret"}, cfg.ExcludePatterns, "exclusions must survive resume")
	assert.Equal(t, []string{"/data/a.txt", "/data/sub/b.txt"}, cfg.IncludeOnlyFiles)
	assert.Equal(t, "incremental", cfg.SyncType)
	assert.Equal(t, "20251231-prev", cfg.PreviousUploadID)
	assert.Equal(t, []string{"/data/gone.txt"}, cfg.DeletedPaths)
	assert.Equal(t, "20251201-root", cfg.DatasetID)
	assert.Equal(t, 4, cfg.VersionOrdinal)
	assert.Equal(t, "nas-1", cfg.WriterID)
	assert.Equal(t, "proj-7", cfg.ProjectID)
	assert.Equal(t, map[string]string{"team": "genomics"}, cfg.Tags)
	assert.True(t, cfg.EnableDeduplication)
	assert.Equal(t, "tier-aware", cfg.TierChunkingStrategy)
	if assert.NotNil(t, cfg.TierSelector, "--auto-tier must survive resume") {
		assert.True(t, cfg.TierSelector.Enabled)
		assert.Equal(t, 10, cfg.TierSelector.HotDays)
		assert.Equal(t, 60, cfg.TierSelector.ColdDays)
		assert.Equal(t, 200, cfg.TierSelector.ArchiveDays)
		assert.Equal(t, types.StorageClassGlacier, cfg.TierSelector.MaxTier)
		assert.Equal(t, types.StorageClassStandard, cfg.TierSelector.DefaultClass)
		assert.True(t, cfg.TierSelector.FallbackToMtime)
	}
}

// notResumed lists the PipelineConfig fields a resume deliberately does not
// carry over from the interrupted upload, with the reason. Every other field
// must come back equal (TestResume_RebuiltConfigEqualsOriginal), so a new
// upload setting can't be silently dropped on resume again (#1011, #725).
var notResumed = map[string]string{
	"ScannerWorkers": "tuning", "ArchiverWorkers": "tuning", "UploaderWorkers": "tuning",
	"ChunkBufferSize": "tuning", "ArchiveBufferSize": "tuning", "ResultBufferSize": "tuning",
	"WorkersPerPrefix": "tuning", "S3PartSize": "tuning",
	"S3Client": "runtime client", "KMSClient": "runtime client", "Transporter": "runtime client",
	"Tracer": "runtime", "EnableTracing": "runtime", "EnableProgress": "UI", "ProgressInterval": "UI",
	"StageTimeout":      "runtime",
	"S3SSEKMSKeyId":     "deprecated alias of KMSKeyID",
	"EnableMultiPrefix": "resume always uses the multi-prefix uploader", "EnableArchiverSharding": "internal mode",
	"EnableOptimization": "network tuning", "CongestionControl": "network tuning",
	"EnableCompressedAwareChunking": "not exposed by the CLI", "EnableArchivePadding": "not exposed by the CLI",
	"MaxPaddingRatio": "not exposed by the CLI", "ChunkingConfig": "not exposed by the CLI",
	"MagikaConfig":   "read from the config file on every run",
	"EnableManifest": "resume sets it", "EnablePartialManifest": "resume sets it",
	"PartialManifestSaveInterval": "runtime", "ResumeMode": "resume sets it", "ResumeUploadID": "resume sets it",
	"SkipExisting": "resume option", "EnableLocalState": "runtime", "LocalStateSaveInterval": "runtime",
	"DisableFileHashing": "runtime", "CleanupOnFailure": "a resume keeps what it uploaded",
	"EnableDirectUpload": "routing: a resumed chunked upload stays chunked", "ForceDirectUpload": "routing",
	"EnableAutoDirectUpload": "routing", "DirectUploadThresholdMB": "routing", "DirectUploadMaxFiles": "routing",
	"DirectUploadAvgSizeMB": "routing", "DirectUploadWorkers": "routing",
	"DVCPipelineData": "kept in the partial manifest", "GitMetadataData": "kept in the partial manifest",
}

// TestResume_RebuiltConfigEqualsOriginal pins #1011 end to end through the state
// file: the config `cargoship resume` rebuilds must equal the interrupted
// upload's config in every field not listed in notResumed. A new PipelineConfig
// field fails here until it is either persisted or listed with a reason.
func TestResume_RebuiltConfigEqualsOriginal(t *testing.T) {
	orig := &pipeline.PipelineConfig{
		S3Bucket: "b", S3Prefix: "p/writers/nas-1", S3Region: "us-east-2", UseRealS3: true,
		S3StorageClass: "STANDARD_IA", KMSKeyID: "arn:aws:kms:us-east-1:1:key/k", EncryptManifest: true,
		UploadID: "20260101-orig", WriterID: "nas-1", ShardCount: 6, ForceChunkSizeMB: 32,
		CompressionLevel: 9, ShardStrategy: "hash", FrameSize: 8 << 20,
		SourcePath: "/data", FileChecksums: false, EnableDeduplication: true,
		IncludeOnlyFiles: []string{"/data/a"}, ExcludePatterns: []string{"#recycle"},
		SyncType: "incremental", PreviousUploadID: "20251231-prev", DeletedPaths: []string{"/data/gone"},
		DatasetID: "20251201-root", VersionOrdinal: 3, ProjectID: "proj", Tags: map[string]string{"k": "v"},
		TierChunkingStrategy: "tier-aware",
		TierSelector: &pipeline.StorageTierSelector{Enabled: true, DefaultClass: types.StorageClassStandard,
			HotDays: 5, ColdDays: 50, ArchiveDays: 500, MaxTier: types.StorageClassGlacier, FallbackToMtime: true},
	}
	require.NoError(t, resume.SaveState(pipeline.NewUploadState(orig)))
	st, err := resume.LoadState(orig.UploadID)
	require.NoError(t, err)
	got := newResumePipelineConfig(st, &s3.Client{}, nil)

	ov, gv := reflect.ValueOf(orig).Elem(), reflect.ValueOf(got).Elem()
	compared := 0
	for i := 0; i < ov.NumField(); i++ {
		name := ov.Type().Field(i).Name
		if _, skip := notResumed[name]; skip {
			continue
		}
		compared++
		assert.Equal(t, ov.Field(i).Interface(), gv.Field(i).Interface(),
			"PipelineConfig.%s is not carried over by resume: persist it in resume.UploadSettings, or list it in notResumed with a reason", name)
	}
	assert.Positive(t, compared)
	for name := range notResumed {
		_, ok := ov.Type().FieldByName(name)
		assert.True(t, ok, "notResumed lists %s, which PipelineConfig no longer has", name)
	}
}
