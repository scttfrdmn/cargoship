package pipeline

import (
	"time"

	"github.com/aws/aws-sdk-go-v2/service/s3/types"
)

// StorageTierSelector determines optimal S3 storage class based on file metadata.
// It analyzes file access time (atime) to automatically select cost-effective storage tiers.
type StorageTierSelector struct {
	// Enabled indicates whether automatic tier selection is active
	Enabled bool

	// DefaultClass is the fallback storage class when tier selection is disabled or fails
	DefaultClass types.StorageClass

	// HotDays is the threshold (in days since last access) for STANDARD storage
	// Files accessed within this period are considered "hot" and stored in STANDARD
	// Default: 30 days
	HotDays int

	// ColdDays is the threshold (in days since last access) for GLACIER storage
	// Files not accessed within this period are considered "cold" and moved to GLACIER
	// Default: 90 days
	ColdDays int

	// ArchiveDays is the threshold (in days since last access) for DEEP_ARCHIVE storage
	// Files not accessed within this period are archived to DEEP_ARCHIVE
	// Default: 180 days
	ArchiveDays int

	// FallbackToMtime enables using modification time as a fallback when atime is unavailable
	// This is useful for filesystems mounted with noatime or when atime extraction fails
	FallbackToMtime bool
}

// SelectTier analyzes file access time and returns the optimal S3 storage class.
//
// The selection algorithm:
//   - 0-HotDays: STANDARD (frequently accessed)
//   - HotDays-ColdDays: STANDARD_IA (infrequently accessed)
//   - ColdDays-ArchiveDays: GLACIER (rarely accessed)
//   - ArchiveDays+: DEEP_ARCHIVE (archived/compliance)
//
// Parameters:
//   - atime: Last access time of the file
//   - mtime: Last modification time of the file (used as fallback)
//
// Returns the appropriate S3 storage class as a string.
func (s *StorageTierSelector) SelectTier(atime, mtime time.Time) types.StorageClass {
	// If tier selection is disabled, use default class
	if !s.Enabled {
		return s.DefaultClass
	}

	// Determine which timestamp to use for tier selection
	accessTime := atime
	if atime.IsZero() {
		if s.FallbackToMtime {
			accessTime = mtime
		} else {
			// No time information available and fallback disabled
			return s.DefaultClass
		}
	}

	// If still zero (both atime and mtime unavailable), use default
	if accessTime.IsZero() {
		return s.DefaultClass
	}

	// Calculate days since last access
	daysSinceAccess := time.Since(accessTime).Hours() / 24

	// Select storage tier based on access age
	switch {
	case daysSinceAccess >= float64(s.ArchiveDays):
		// Very old files (>=180 days default): Deep Archive
		return types.StorageClassDeepArchive

	case daysSinceAccess >= float64(s.ColdDays):
		// Old files (>=90 days default): Glacier
		return types.StorageClassGlacier

	case daysSinceAccess >= float64(s.HotDays):
		// Moderately old files (>=30 days default): Standard-IA
		return types.StorageClassStandardIa

	default:
		// Recently accessed files: Standard
		return types.StorageClassStandard
	}
}

// FileWithTimes represents a file with its associated timestamps for tier analysis.
type FileWithTimes struct {
	Path  string
	Size  int64
	Atime time.Time
	Mtime time.Time
}

// TierStats provides statistics about tier distribution across a set of files.
type TierStats struct {
	// TotalFiles is the total number of files analyzed
	TotalFiles int64

	// TotalSize is the total size in bytes across all files
	TotalSize int64

	// Tiers maps storage class to total size in bytes for that tier
	Tiers map[types.StorageClass]int64
}

// GetTierStats analyzes a batch of files and returns tier distribution statistics.
// This is useful for estimating cost savings before upload or generating reports.
func (s *StorageTierSelector) GetTierStats(files []FileWithTimes) TierStats {
	stats := TierStats{
		Tiers: make(map[types.StorageClass]int64),
	}

	for _, file := range files {
		tier := s.SelectTier(file.Atime, file.Mtime)
		stats.Tiers[tier] += file.Size
		stats.TotalFiles++
		stats.TotalSize += file.Size
	}

	return stats
}

// NewStorageTierSelector creates a new StorageTierSelector with default thresholds.
//
// Default thresholds:
//   - HotDays: 30 (files accessed within 30 days → STANDARD)
//   - ColdDays: 90 (files accessed 30-90 days ago → STANDARD_IA)
//   - ArchiveDays: 180 (files accessed 90-180 days ago → GLACIER)
//   - 180+ days → DEEP_ARCHIVE
func NewStorageTierSelector(enabled bool, defaultClass types.StorageClass) *StorageTierSelector {
	return &StorageTierSelector{
		Enabled:         enabled,
		DefaultClass:    defaultClass,
		HotDays:         30,
		ColdDays:        90,
		ArchiveDays:     180,
		FallbackToMtime: true,
	}
}
