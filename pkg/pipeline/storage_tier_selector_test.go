package pipeline

import (
	"testing"
	"time"

	"github.com/aws/aws-sdk-go-v2/service/s3/types"
)

// TestStorageTierSelector_Disabled tests that selector uses default class when disabled
func TestStorageTierSelector_Disabled(t *testing.T) {
	selector := &StorageTierSelector{
		Enabled:      false,
		DefaultClass: types.StorageClassStandard,
		HotDays:      30,
		ColdDays:     90,
		ArchiveDays:  180,
	}

	// Even with very old atime, should return default class when disabled
	veryOldTime := time.Now().Add(-1000 * 24 * time.Hour) // ~3 years old
	recentTime := time.Now()

	tier := selector.SelectTier(veryOldTime, recentTime)
	if tier != types.StorageClassStandard {
		t.Errorf("SelectTier() with disabled = %v, want %v", tier, types.StorageClassStandard)
	}
}

// TestStorageTierSelector_HotFiles tests STANDARD tier selection for recently accessed files
func TestStorageTierSelector_HotFiles(t *testing.T) {
	selector := NewStorageTierSelector(true, types.StorageClassStandard)

	tests := []struct {
		name         string
		daysOld      int
		expectedTier types.StorageClass
	}{
		{"Just created", 0, types.StorageClassStandard},
		{"1 day old", 1, types.StorageClassStandard},
		{"15 days old", 15, types.StorageClassStandard},
		{"29 days old", 29, types.StorageClassStandard},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			atime := time.Now().Add(-time.Duration(tt.daysOld) * 24 * time.Hour)
			mtime := atime

			tier := selector.SelectTier(atime, mtime)
			if tier != tt.expectedTier {
				t.Errorf("SelectTier(%d days old) = %v, want %v", tt.daysOld, tier, tt.expectedTier)
			}
		})
	}
}

// TestStorageTierSelector_StandardIA tests STANDARD_IA tier selection
func TestStorageTierSelector_StandardIA(t *testing.T) {
	selector := NewStorageTierSelector(true, types.StorageClassStandard)

	tests := []struct {
		name         string
		daysOld      int
		expectedTier types.StorageClass
	}{
		{"30 days old exactly", 30, types.StorageClassStandardIa}, // Boundary
		{"31 days old", 31, types.StorageClassStandardIa},
		{"60 days old", 60, types.StorageClassStandardIa},
		{"89 days old", 89, types.StorageClassStandardIa},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			atime := time.Now().Add(-time.Duration(tt.daysOld) * 24 * time.Hour)
			mtime := atime

			tier := selector.SelectTier(atime, mtime)
			if tier != tt.expectedTier {
				t.Errorf("SelectTier(%d days old) = %v, want %v", tt.daysOld, tier, tt.expectedTier)
			}
		})
	}
}

// TestStorageTierSelector_Glacier tests GLACIER tier selection
func TestStorageTierSelector_Glacier(t *testing.T) {
	selector := NewStorageTierSelector(true, types.StorageClassStandard)

	tests := []struct {
		name         string
		daysOld      int
		expectedTier types.StorageClass
	}{
		{"90 days old exactly", 90, types.StorageClassGlacier}, // Boundary
		{"91 days old", 91, types.StorageClassGlacier},
		{"120 days old", 120, types.StorageClassGlacier},
		{"179 days old", 179, types.StorageClassGlacier},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			atime := time.Now().Add(-time.Duration(tt.daysOld) * 24 * time.Hour)
			mtime := atime

			tier := selector.SelectTier(atime, mtime)
			if tier != tt.expectedTier {
				t.Errorf("SelectTier(%d days old) = %v, want %v", tt.daysOld, tier, tt.expectedTier)
			}
		})
	}
}

// TestStorageTierSelector_DeepArchive tests DEEP_ARCHIVE tier selection
func TestStorageTierSelector_DeepArchive(t *testing.T) {
	selector := NewStorageTierSelector(true, types.StorageClassStandard)

	tests := []struct {
		name         string
		daysOld      int
		expectedTier types.StorageClass
	}{
		{"180 days old exactly", 180, types.StorageClassDeepArchive}, // Boundary
		{"181 days old", 181, types.StorageClassDeepArchive},
		{"365 days old", 365, types.StorageClassDeepArchive},
		{"730 days old", 730, types.StorageClassDeepArchive},
		{"1000 days old", 1000, types.StorageClassDeepArchive},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			atime := time.Now().Add(-time.Duration(tt.daysOld) * 24 * time.Hour)
			mtime := atime

			tier := selector.SelectTier(atime, mtime)
			if tier != tt.expectedTier {
				t.Errorf("SelectTier(%d days old) = %v, want %v", tt.daysOld, tier, tt.expectedTier)
			}
		})
	}
}

// TestStorageTierSelector_FallbackToMtime tests fallback behavior when atime is zero
func TestStorageTierSelector_FallbackToMtime(t *testing.T) {
	selector := NewStorageTierSelector(true, types.StorageClassStandard)
	selector.FallbackToMtime = true

	// Zero atime, but valid mtime (200 days old)
	zeroAtime := time.Time{}
	oldMtime := time.Now().Add(-200 * 24 * time.Hour)

	tier := selector.SelectTier(zeroAtime, oldMtime)

	// Should fall back to mtime and select DEEP_ARCHIVE (>180 days)
	if tier != types.StorageClassDeepArchive {
		t.Errorf("SelectTier() with fallback = %v, want %v", tier, types.StorageClassDeepArchive)
	}
}

// TestStorageTierSelector_NoFallback tests behavior when fallback is disabled
func TestStorageTierSelector_NoFallback(t *testing.T) {
	selector := NewStorageTierSelector(true, types.StorageClassStandard)
	selector.FallbackToMtime = false

	// Zero atime, but valid mtime
	zeroAtime := time.Time{}
	oldMtime := time.Now().Add(-200 * 24 * time.Hour)

	tier := selector.SelectTier(zeroAtime, oldMtime)

	// Should return default class (no fallback)
	if tier != types.StorageClassStandard {
		t.Errorf("SelectTier() with no fallback = %v, want %v", tier, types.StorageClassStandard)
	}
}

// TestStorageTierSelector_BothTimesZero tests behavior when both atime and mtime are zero
func TestStorageTierSelector_BothTimesZero(t *testing.T) {
	selector := NewStorageTierSelector(true, types.StorageClassGlacier)

	zeroTime := time.Time{}
	tier := selector.SelectTier(zeroTime, zeroTime)

	// Should return default class
	if tier != types.StorageClassGlacier {
		t.Errorf("SelectTier() with zero times = %v, want %v", tier, types.StorageClassGlacier)
	}
}

// TestStorageTierSelector_CustomThresholds tests tier selection with custom thresholds
func TestStorageTierSelector_CustomThresholds(t *testing.T) {
	// Use aggressive thresholds: 14/45/180 days
	selector := &StorageTierSelector{
		Enabled:         true,
		DefaultClass:    types.StorageClassStandard,
		HotDays:         14,
		ColdDays:        45,
		ArchiveDays:     180,
		FallbackToMtime: true,
	}

	tests := []struct {
		daysOld      int
		expectedTier types.StorageClass
	}{
		{10, types.StorageClassStandard},     // <14 days
		{20, types.StorageClassStandardIa},   // 14-45 days
		{50, types.StorageClassGlacier},      // 45-180 days
		{200, types.StorageClassDeepArchive}, // >180 days
	}

	for _, tt := range tests {
		atime := time.Now().Add(-time.Duration(tt.daysOld) * 24 * time.Hour)
		tier := selector.SelectTier(atime, time.Time{})

		if tier != tt.expectedTier {
			t.Errorf("SelectTier(%d days old) = %v, want %v", tt.daysOld, tier, tt.expectedTier)
		}
	}
}

// TestStorageTierSelector_GetTierStats tests tier statistics aggregation
func TestStorageTierSelector_GetTierStats(t *testing.T) {
	selector := NewStorageTierSelector(true, types.StorageClassStandard)

	now := time.Now()
	files := []FileWithTimes{
		// Recent files (STANDARD)
		{Path: "file1.txt", Size: 1000, Atime: now.Add(-10 * 24 * time.Hour), Mtime: now},
		{Path: "file2.txt", Size: 2000, Atime: now.Add(-20 * 24 * time.Hour), Mtime: now},

		// Infrequent files (STANDARD_IA)
		{Path: "file3.txt", Size: 3000, Atime: now.Add(-60 * 24 * time.Hour), Mtime: now},

		// Rare files (GLACIER)
		{Path: "file4.txt", Size: 4000, Atime: now.Add(-120 * 24 * time.Hour), Mtime: now},

		// Archive files (DEEP_ARCHIVE)
		{Path: "file5.txt", Size: 5000, Atime: now.Add(-200 * 24 * time.Hour), Mtime: now},
		{Path: "file6.txt", Size: 6000, Atime: now.Add(-365 * 24 * time.Hour), Mtime: now},
	}

	stats := selector.GetTierStats(files)

	// Verify total counts
	if stats.TotalFiles != 6 {
		t.Errorf("TotalFiles = %d, want 6", stats.TotalFiles)
	}

	expectedTotalSize := int64(1000 + 2000 + 3000 + 4000 + 5000 + 6000)
	if stats.TotalSize != expectedTotalSize {
		t.Errorf("TotalSize = %d, want %d", stats.TotalSize, expectedTotalSize)
	}

	// Verify tier distributions
	expectedStandard := int64(1000 + 2000)
	if stats.Tiers[types.StorageClassStandard] != expectedStandard {
		t.Errorf("STANDARD size = %d, want %d",
			stats.Tiers[types.StorageClassStandard], expectedStandard)
	}

	expectedStandardIA := int64(3000)
	if stats.Tiers[types.StorageClassStandardIa] != expectedStandardIA {
		t.Errorf("STANDARD_IA size = %d, want %d",
			stats.Tiers[types.StorageClassStandardIa], expectedStandardIA)
	}

	expectedGlacier := int64(4000)
	if stats.Tiers[types.StorageClassGlacier] != expectedGlacier {
		t.Errorf("GLACIER size = %d, want %d",
			stats.Tiers[types.StorageClassGlacier], expectedGlacier)
	}

	expectedDeepArchive := int64(5000 + 6000)
	if stats.Tiers[types.StorageClassDeepArchive] != expectedDeepArchive {
		t.Errorf("DEEP_ARCHIVE size = %d, want %d",
			stats.Tiers[types.StorageClassDeepArchive], expectedDeepArchive)
	}
}

// TestStorageTierSelector_EmptyFileList tests stats with empty file list
func TestStorageTierSelector_EmptyFileList(t *testing.T) {
	selector := NewStorageTierSelector(true, types.StorageClassStandard)

	stats := selector.GetTierStats([]FileWithTimes{})

	if stats.TotalFiles != 0 {
		t.Errorf("TotalFiles = %d, want 0", stats.TotalFiles)
	}
	if stats.TotalSize != 0 {
		t.Errorf("TotalSize = %d, want 0", stats.TotalSize)
	}
	if len(stats.Tiers) != 0 {
		t.Errorf("Tiers map has %d entries, want 0", len(stats.Tiers))
	}
}

// TestNewStorageTierSelector tests default constructor
func TestNewStorageTierSelector(t *testing.T) {
	selector := NewStorageTierSelector(true, types.StorageClassStandard)

	if !selector.Enabled {
		t.Error("Expected Enabled=true")
	}
	if selector.DefaultClass != types.StorageClassStandard {
		t.Errorf("DefaultClass = %v, want %v", selector.DefaultClass, types.StorageClassStandard)
	}
	if selector.HotDays != 30 {
		t.Errorf("HotDays = %d, want 30", selector.HotDays)
	}
	if selector.ColdDays != 90 {
		t.Errorf("ColdDays = %d, want 90", selector.ColdDays)
	}
	if selector.ArchiveDays != 180 {
		t.Errorf("ArchiveDays = %d, want 180", selector.ArchiveDays)
	}
	if !selector.FallbackToMtime {
		t.Error("Expected FallbackToMtime=true")
	}
}

// TestStorageTierSelector_BoundaryConditions tests exact boundary values
func TestStorageTierSelector_BoundaryConditions(t *testing.T) {
	selector := NewStorageTierSelector(true, types.StorageClassStandard)

	tests := []struct {
		hoursOld     float64
		expectedTier types.StorageClass
		description  string
	}{
		// Just before 30 days
		{30*24 - 1, types.StorageClassStandard, "29.96 days"},
		// Exactly 30 days (boundary: >= 30 → STANDARD_IA)
		{30 * 24, types.StorageClassStandardIa, "30 days exactly"},
		// Just after 30 days
		{30*24 + 1, types.StorageClassStandardIa, "30.04 days"},

		// Just before 90 days
		{90*24 - 1, types.StorageClassStandardIa, "89.96 days"},
		// Exactly 90 days (boundary: >= 90 → GLACIER)
		{90 * 24, types.StorageClassGlacier, "90 days exactly"},
		// Just after 90 days
		{90*24 + 1, types.StorageClassGlacier, "90.04 days"},

		// Just before 180 days
		{180*24 - 1, types.StorageClassGlacier, "179.96 days"},
		// Exactly 180 days (boundary: >= 180 → DEEP_ARCHIVE)
		{180 * 24, types.StorageClassDeepArchive, "180 days exactly"},
		// Just after 180 days
		{180*24 + 1, types.StorageClassDeepArchive, "180.04 days"},
	}

	for _, tt := range tests {
		t.Run(tt.description, func(t *testing.T) {
			atime := time.Now().Add(-time.Duration(tt.hoursOld) * time.Hour)
			tier := selector.SelectTier(atime, time.Time{})

			if tier != tt.expectedTier {
				t.Errorf("SelectTier(%s) = %v, want %v",
					tt.description, tier, tt.expectedTier)
			}
		})
	}
}
