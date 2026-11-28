package pipeline

import (
	"maps"
	"slices"
	"time"

	"github.com/aws/aws-sdk-go-v2/service/s3/types"

	"github.com/scttfrdmn/cargoship/pkg/resume"
)

// DefaultFrameSize is the upload command's --frame-size default (16 MiB). A
// resume from a state that predates recorded settings uses it (#1011).
const DefaultFrameSize int64 = 16 << 20

// NewUploadState returns the local resume state (#119) for an upload run with
// config, including the settings `cargoship resume` needs to rebuild the same
// upload (#1011).
func NewUploadState(config *PipelineConfig) *resume.UploadState {
	now := time.Now()
	state := &resume.UploadState{
		UploadID:        config.UploadID,
		StartTime:       now,
		LastSave:        now,
		SourceDir:       config.SourcePath,
		Bucket:          config.S3Bucket,
		Prefix:          config.S3Prefix,
		Region:          config.S3Region,
		StorageClass:    config.S3StorageClass,
		KMSKeyID:        config.KMSKeyID,
		EncryptManifest: config.EncryptManifest,
		ChunkSizeMB:     config.ForceChunkSizeMB,
		ShardCount:      config.ShardCount,
		Settings:        UploadSettingsFromConfig(config),
	}
	if config.ShardCount > 0 {
		state.Shards = make([]resume.ShardState, config.ShardCount)
		for i := range state.Shards {
			state.Shards[i] = resume.ShardState{ShardID: i}
		}
	}
	return state
}

// UploadSettingsFromConfig records the settings of config that a resumed upload
// must reproduce (#1011). Slices and maps are copied.
func UploadSettingsFromConfig(config *PipelineConfig) *resume.UploadSettings {
	s := &resume.UploadSettings{
		FileChecksums:        config.FileChecksums,
		FrameSize:            config.FrameSize,
		CompressionLevel:     config.CompressionLevel,
		ShardStrategy:        config.ShardStrategy,
		ExcludePatterns:      slices.Clone(config.ExcludePatterns),
		IncludeOnlyFiles:     slices.Clone(config.IncludeOnlyFiles),
		SyncType:             config.SyncType,
		PreviousUploadID:     config.PreviousUploadID,
		DeletedPaths:         slices.Clone(config.DeletedPaths),
		DatasetID:            config.DatasetID,
		VersionOrdinal:       config.VersionOrdinal,
		WriterID:             config.WriterID,
		ProjectID:            config.ProjectID,
		Tags:                 maps.Clone(config.Tags),
		EnableDeduplication:  config.EnableDeduplication,
		TierChunkingStrategy: config.TierChunkingStrategy,
	}
	if ts := config.TierSelector; ts != nil && ts.Enabled {
		s.Tier = &resume.TierSettings{
			HotDays:         ts.HotDays,
			ColdDays:        ts.ColdDays,
			ArchiveDays:     ts.ArchiveDays,
			MaxTier:         string(ts.MaxTier),
			DefaultClass:    string(ts.DefaultClass),
			FallbackToMtime: ts.FallbackToMtime,
		}
	}
	return s
}

// ApplyUploadSettings sets the resumable settings of config from s (#1011). A
// nil s comes from a state saved before settings were recorded; it applies the
// upload command's defaults instead of Go zero values: per-file checksums on
// and DefaultFrameSize frames.
func ApplyUploadSettings(config *PipelineConfig, s *resume.UploadSettings) {
	if s == nil {
		config.FileChecksums = true
		config.FrameSize = DefaultFrameSize
		return
	}
	config.FileChecksums = s.FileChecksums
	config.FrameSize = s.FrameSize
	config.CompressionLevel = s.CompressionLevel
	config.ShardStrategy = s.ShardStrategy
	config.ExcludePatterns = slices.Clone(s.ExcludePatterns)
	config.IncludeOnlyFiles = slices.Clone(s.IncludeOnlyFiles)
	config.SyncType = s.SyncType
	config.PreviousUploadID = s.PreviousUploadID
	config.DeletedPaths = slices.Clone(s.DeletedPaths)
	config.DatasetID = s.DatasetID
	config.VersionOrdinal = s.VersionOrdinal
	config.WriterID = s.WriterID
	config.ProjectID = s.ProjectID
	config.Tags = maps.Clone(s.Tags)
	config.EnableDeduplication = s.EnableDeduplication
	config.TierChunkingStrategy = s.TierChunkingStrategy
	config.TierSelector = nil
	if t := s.Tier; t != nil {
		config.TierSelector = &StorageTierSelector{
			Enabled:         true,
			HotDays:         t.HotDays,
			ColdDays:        t.ColdDays,
			ArchiveDays:     t.ArchiveDays,
			MaxTier:         types.StorageClass(t.MaxTier),
			DefaultClass:    types.StorageClass(t.DefaultClass),
			FallbackToMtime: t.FallbackToMtime,
		}
	}
}
