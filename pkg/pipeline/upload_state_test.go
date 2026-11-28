package pipeline

import (
	"testing"

	"github.com/aws/aws-sdk-go-v2/service/s3/types"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// TestUploadSettings_RoundTrip pins #1011 at the package boundary: what
// NewUploadState records, ApplyUploadSettings restores, and a state from before
// settings were recorded resumes with the upload defaults.
func TestUploadSettings_RoundTrip(t *testing.T) {
	orig := &PipelineConfig{
		UploadID: "u", S3Bucket: "b", S3Prefix: "p", S3Region: "r", S3StorageClass: "GLACIER_IR",
		KMSKeyID: "k", EncryptManifest: true, ForceChunkSizeMB: 16, ShardCount: 3, SourcePath: "/s",
		FileChecksums: false, FrameSize: 0, CompressionLevel: 6, ShardStrategy: "directory",
		ExcludePatterns: []string{"x"}, IncludeOnlyFiles: []string{"/s/a"}, SyncType: "incremental",
		PreviousUploadID: "prev", DeletedPaths: []string{"/s/d"}, DatasetID: "ds", VersionOrdinal: 2,
		WriterID: "w", ProjectID: "proj", Tags: map[string]string{"a": "b"}, EnableDeduplication: true,
		TierChunkingStrategy: "youngest-file",
		TierSelector: &StorageTierSelector{Enabled: true, HotDays: 1, ColdDays: 2, ArchiveDays: 3,
			MaxTier: types.StorageClassDeepArchive, DefaultClass: types.StorageClassStandard},
	}
	st := NewUploadState(orig)
	assert.Equal(t, "u", st.UploadID)
	assert.Equal(t, 16, st.ChunkSizeMB)
	require.Len(t, st.Shards, 3)
	assert.Equal(t, 2, st.Shards[2].ShardID)
	require.NotNil(t, st.Settings)

	got := &PipelineConfig{FileChecksums: true, FrameSize: DefaultFrameSize}
	ApplyUploadSettings(got, st.Settings)
	assert.False(t, got.FileChecksums, "an explicit opt-out survives")
	assert.Zero(t, got.FrameSize, "an explicit --frame-size 0 survives")
	assert.Equal(t, orig.CompressionLevel, got.CompressionLevel)
	assert.Equal(t, orig.ShardStrategy, got.ShardStrategy)
	assert.Equal(t, orig.ExcludePatterns, got.ExcludePatterns)
	assert.Equal(t, orig.IncludeOnlyFiles, got.IncludeOnlyFiles)
	assert.Equal(t, orig.SyncType, got.SyncType)
	assert.Equal(t, orig.PreviousUploadID, got.PreviousUploadID)
	assert.Equal(t, orig.DeletedPaths, got.DeletedPaths)
	assert.Equal(t, orig.DatasetID, got.DatasetID)
	assert.Equal(t, orig.VersionOrdinal, got.VersionOrdinal)
	assert.Equal(t, orig.WriterID, got.WriterID)
	assert.Equal(t, orig.ProjectID, got.ProjectID)
	assert.Equal(t, orig.Tags, got.Tags)
	assert.True(t, got.EnableDeduplication)
	assert.Equal(t, orig.TierChunkingStrategy, got.TierChunkingStrategy)
	assert.Equal(t, orig.TierSelector, got.TierSelector)

	// Copies, not aliases: editing the restored config leaves the record alone.
	got.ExcludePatterns[0] = "changed"
	got.Tags["a"] = "changed"
	assert.Equal(t, []string{"x"}, st.Settings.ExcludePatterns)
	assert.Equal(t, "b", st.Settings.Tags["a"])

	// A disabled selector records no tier settings and restores none.
	orig.TierSelector.Enabled = false
	off := &PipelineConfig{TierSelector: &StorageTierSelector{Enabled: true}}
	ApplyUploadSettings(off, UploadSettingsFromConfig(orig))
	assert.Nil(t, off.TierSelector)

	// No recorded settings (a pre-#1011 state): the upload command's defaults.
	old := &PipelineConfig{}
	ApplyUploadSettings(old, nil)
	assert.True(t, old.FileChecksums)
	assert.Equal(t, DefaultFrameSize, old.FrameSize)

	// No shards when the count is unset (a make with a negative length panics).
	assert.Empty(t, NewUploadState(&PipelineConfig{UploadID: "u", ShardCount: -1}).Shards)
}
