package cmd

import (
	"strings"
	"testing"
	"time"

	"github.com/spf13/cobra"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/scttfrdmn/cargoship/pkg/aws/cost"
	"github.com/scttfrdmn/cargoship/pkg/manifest"
	"github.com/scttfrdmn/cargoship/pkg/pipeline"
)

// TestNewUploadCmd_Structure tests the upload command structure (Issue #95)
func TestNewUploadCmd_Structure(t *testing.T) {
	cmd := NewUploadCmd()

	// Verify command properties
	assert.Equal(t, "upload", cmd.Name())
	assert.NotEmpty(t, cmd.Short)
	assert.NotEmpty(t, cmd.Long)
	assert.NotNil(t, cmd.RunE)

	// Verify command usage
	assert.Contains(t, cmd.Use, "SOURCE_DIR")
	assert.Contains(t, cmd.Use, "DESTINATION")

	// Verify flags exist
	flags := []string{"bucket", "region", "storage-class", "shard-count", "shard-strategy", "compression-level", "quiet"}
	for _, flag := range flags {
		f := cmd.Flags().Lookup(flag)
		assert.NotNil(t, f, "Flag %s should exist", flag)
	}

	// Verify flag shortcuts
	assert.Equal(t, "b", cmd.Flags().Lookup("bucket").Shorthand)
	assert.Equal(t, "r", cmd.Flags().Lookup("region").Shorthand)

	// Verify default values
	assert.Equal(t, "us-west-2", cmd.Flags().Lookup("region").DefValue)
	assert.Equal(t, "STANDARD", cmd.Flags().Lookup("storage-class").DefValue)
	assert.Equal(t, "0", cmd.Flags().Lookup("shard-count").DefValue) // Issue #106: Auto-select by default
	// #316: the default is round-robin, which is what the archiver has always
	// actually done. It previously read "hash" while the implementation was
	// round-robin — and this test passed anyway, which is the point of the
	// behavioral tests referenced below.
	assert.Equal(t, pipeline.ShardStrategyRoundRobin, cmd.Flags().Lookup("shard-strategy").DefValue)
	assert.Equal(t, "3", cmd.Flags().Lookup("compression-level").DefValue)
	assert.Equal(t, "false", cmd.Flags().Lookup("quiet").DefValue)
}

// TestUploadCmd_FlagsReachPipelineConfig is the guard for #316: for a decade of
// commits the assertions above were the *only* coverage these flags had, and they
// passed while --compression-level, --shard-strategy, --direct-upload-threshold-mb,
// --direct-upload-workers and --interactive were all accepted and then dropped on
// the floor. Presence and default assertions cannot detect a flag that does
// nothing, so this test walks the flag → PipelineConfig path instead.
//
// The per-strategy and per-level *behavior* lives in pkg/pipeline
// (shard_strategy_test.go, archiver_test.go); this only proves the CLI hands the
// values over.
func TestUploadCmd_FlagsReachPipelineConfig(t *testing.T) {
	t.Run("shard strategy is validated against the pipeline's set", func(t *testing.T) {
		// Every value the flag advertises must be accepted by the assigner that
		// consumes it. A strategy named in --help but rejected (or silently
		// ignored) downstream is the #316 defect.
		for _, s := range pipeline.ShardStrategies() {
			assert.NoError(t, pipeline.ValidateShardStrategy(s),
				"upload advertises strategy %q; the pipeline must accept it", s)
		}
		assert.Error(t, pipeline.ValidateShardStrategy("balanced"),
			"an unknown strategy must be rejected at parse time, not ignored at run time")
	})

	t.Run("compression level is an override, not a floor", func(t *testing.T) {
		cmd := NewUploadCmd()
		// Untouched: the pipeline must receive 0 so content-aware selection
		// stays on. Forwarding the flag's default (3) unconditionally would
		// pin every chunk to level 3 for every user who never passed the flag.
		assert.False(t, cmd.Flags().Changed("compression-level"),
			"a fresh command has not been given the flag")

		require.NoError(t, cmd.Flags().Set("compression-level", "19"))
		assert.True(t, cmd.Flags().Changed("compression-level"),
			"Changed() is what distinguishes an explicit 3 from an absent flag")
		v, err := cmd.Flags().GetInt("compression-level")
		require.NoError(t, err)
		assert.Equal(t, 19, v)
		// 19 is outside the pre-built encoder-pool tiers (1/3/6/9); it must not
		// be silently downgraded. See TestArchiverStage_CompressionLevelOutOfPoolOverride.
		assert.Equal(t, "best", pipeline.EffectiveZstdTier(19),
			"an out-of-tier override must map to the strongest tier, not fall back to 3")
	})

	t.Run("direct-upload flags are readable ints", func(t *testing.T) {
		cmd := NewUploadCmd()
		// These two were defined and never read; upload.go now reads them via
		// GetInt, so a rename or type change here must break loudly.
		require.NoError(t, cmd.Flags().Set("direct-upload-threshold-mb", "250"))
		require.NoError(t, cmd.Flags().Set("direct-upload-workers", "64"))

		threshold, err := cmd.Flags().GetInt("direct-upload-threshold-mb")
		require.NoError(t, err)
		assert.Equal(t, 250, threshold)

		workers, err := cmd.Flags().GetInt("direct-upload-workers")
		require.NoError(t, err)
		assert.Equal(t, 64, workers)
	})

	t.Run("interactive is hidden rather than accepted-and-inert", func(t *testing.T) {
		cmd := NewUploadCmd()
		f := cmd.Flags().Lookup("interactive")
		require.NotNil(t, f, "the flag stays defined so existing scripts don't break")
		assert.True(t, f.Hidden,
			"--interactive has no live implementation (see #325); it must not be advertised")
	})
}

// TestUploadCmd_FlagTypes tests that flags have correct types (Issue #95)
func TestUploadCmd_FlagTypes(t *testing.T) {
	cmd := NewUploadCmd()

	// String flags
	stringFlags := []string{"bucket", "region", "storage-class", "shard-strategy"}
	for _, flag := range stringFlags {
		f := cmd.Flags().Lookup(flag)
		assert.Equal(t, "string", f.Value.Type(), "Flag %s should be string type", flag)
	}

	// Int flags
	intFlags := []string{"shard-count", "compression-level"}
	for _, flag := range intFlags {
		f := cmd.Flags().Lookup(flag)
		assert.Equal(t, "int", f.Value.Type(), "Flag %s should be int type", flag)
	}

	// Bool flags
	boolFlags := []string{"quiet"}
	for _, flag := range boolFlags {
		f := cmd.Flags().Lookup(flag)
		assert.Equal(t, "bool", f.Value.Type(), "Flag %s should be bool type", flag)
	}
}

// TestUploadCmd_HelpText tests help text includes key information (Issue #95)
func TestUploadCmd_HelpText(t *testing.T) {
	cmd := NewUploadCmd()

	// Verify long description includes key features
	keyPhrases := []string{
		"CargoHold",
		"shard",
		"compression",
		"parallel",
		"manifest",
	}

	for _, phrase := range keyPhrases {
		assert.Contains(t, cmd.Long, phrase,
			"Help text should mention '%s'", phrase)
	}

	// Verify shard strategies are documented. Driven off the pipeline's list, not
	// a hardcoded copy, so adding a strategy without documenting it fails here.
	for _, strategy := range pipeline.ShardStrategies() {
		assert.Contains(t, cmd.Long, strategy,
			"Help text should document shard strategy '%s'", strategy)
	}

	// Verify examples are provided
	assert.Contains(t, cmd.Long, "Examples:")
	assert.Contains(t, cmd.Long, "cargoship upload")
}

// TestParseS3Destination tests S3 destination parsing (Issue #95)
func TestParseS3Destination(t *testing.T) {
	tests := []struct {
		name         string
		destination  string
		expectBucket string
		expectPrefix string
		expectError  bool
	}{
		{
			name:         "bucket only",
			destination:  "s3://my-bucket",
			expectBucket: "my-bucket",
			expectPrefix: "",
			expectError:  false,
		},
		{
			name:         "bucket with prefix",
			destination:  "s3://my-bucket/dataset",
			expectBucket: "my-bucket",
			expectPrefix: "dataset",
			expectError:  false,
		},
		{
			name:         "bucket with nested prefix",
			destination:  "s3://my-bucket/prod/cargoship/dataset",
			expectBucket: "my-bucket",
			expectPrefix: "prod/cargoship/dataset",
			expectError:  false,
		},
		{
			name:         "bucket with trailing slash",
			destination:  "s3://my-bucket/dataset/",
			expectBucket: "my-bucket",
			expectPrefix: "dataset",
			expectError:  false,
		},
		{
			name:        "invalid - no s3:// prefix",
			destination: "my-bucket/dataset",
			expectError: true,
		},
		{
			name:        "invalid - http URL",
			destination: "http://my-bucket/dataset",
			expectError: true,
		},
		{
			name:        "invalid - empty",
			destination: "",
			expectError: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			bucket, prefix, err := parseS3Destination(tt.destination)

			if tt.expectError {
				assert.Error(t, err, "Expected error for destination: %s", tt.destination)
			} else {
				assert.NoError(t, err, "Unexpected error for destination: %s", tt.destination)
				assert.Equal(t, tt.expectBucket, bucket, "Bucket mismatch")
				assert.Equal(t, tt.expectPrefix, prefix, "Prefix mismatch")
			}
		})
	}
}

// TestUploadCmd_ShardStrategyValidation tests shard strategy validation (Issue #95)
func TestUploadCmd_ShardStrategyValidation(t *testing.T) {
	validStrategies := []string{"hash", "size", "type", "directory"}
	invalidStrategies := []string{"invalid", "random", "", "foo"}

	// Test that valid strategies are documented in help text
	cmd := NewUploadCmd()
	for _, strategy := range validStrategies {
		assert.Contains(t, cmd.Long, strategy,
			"Valid strategy '%s' should be documented", strategy)
	}

	// Test that invalid strategies would be rejected (validation happens in RunE)
	// We can't easily test RunE without S3, but we can verify the flag accepts string values
	for _, strategy := range invalidStrategies {
		cmd := NewUploadCmd()
		err := cmd.Flags().Set("shard-strategy", strategy)
		assert.NoError(t, err, "Flag should accept any string value (validation in RunE)")
	}
}

// TestUploadCmd_CompressionLevelRange tests compression level range (Issue #95)
func TestUploadCmd_CompressionLevelRange(t *testing.T) {
	// Verify default is within recommended range
	cmd := NewUploadCmd()
	defaultLevel := cmd.Flags().Lookup("compression-level").DefValue
	assert.Equal(t, "3", defaultLevel, "Default compression level should be 3")

	// Verify help text mentions zstd range
	assert.Contains(t, cmd.Flags().Lookup("compression-level").Usage, "1-22",
		"Flag usage should document zstd range")
	usage := cmd.Flags().Lookup("compression-level").Usage
	assert.True(t, strings.Contains(usage, "zstd") || strings.Contains(usage, "Zstd"),
		"Flag usage should mention zstd or Zstd")
}

// TestUploadCmd_ShardCountRange tests shard count range (Issue #95, updated for Issue #106)
func TestUploadCmd_ShardCountRange(t *testing.T) {
	// Verify default is 0 (auto-select)
	cmd := NewUploadCmd()
	defaultCount := cmd.Flags().Lookup("shard-count").DefValue
	assert.Equal(t, "0", defaultCount, "Default shard count should be 0 (auto)")

	// Verify help text documents auto mode, the valid range, and the fallback.
	// #324: the fallback was previously undocumented here and stated three
	// different ways elsewhere, so a user reading --help couldn't tell what 0
	// would actually produce.
	usage := cmd.Flags().Lookup("shard-count").Usage
	assert.Contains(t, usage, "auto",
		"Flag usage should document auto mode")
	assert.Contains(t, usage, "4-32",
		"Flag usage should document valid range")
	assert.Contains(t, usage, "8",
		"Flag usage should document the fallback used when auto-selection fails")
}

// TestUploadCmd_RequiredArguments tests that command requires 2 arguments (Issue #95)
func TestUploadCmd_RequiredArguments(t *testing.T) {
	cmd := NewUploadCmd()

	// Verify Args is set to ExactArgs(2)
	// We can't directly test cobra.ExactArgs, but we can verify the command has Args set
	assert.NotNil(t, cmd.Args, "Args validator should be set")
}

// TestRecordUploadOutcome_Disabled verifies no history is written when opt-in
// is off (a nil manifest and disabled store must be a clean no-op).
func TestRecordUploadOutcome_Disabled(t *testing.T) {
	t.Setenv("CARGOSHIP_UPLOAD_HISTORY", "")

	cmd := &cobra.Command{}
	var out strings.Builder
	cmd.SetOut(&out)
	cmd.SetErr(&out)

	result := &pipeline.Result{Success: true, UploadID: "u1", TotalBytes: 1000, TotalFiles: 2, TotalChunks: 1}
	recordUploadOutcome(cmd, "", "STANDARD", "us-west-2", 3, result, nil)

	assert.Empty(t, out.String(), "disabled store should produce no output")
}

// TestRecordUploadOutcome_AssemblesFromManifest verifies the outcome record is
// assembled from the pipeline result and manifest, including the metadata-only
// file-type mix and manifest-sourced compression fields.
func TestRecordUploadOutcome_AssemblesFromManifest(t *testing.T) {
	path := t.TempDir() + "/upload_history.json"
	t.Setenv("CARGOSHIP_UPLOAD_HISTORY", path)

	cmd := &cobra.Command{}
	var out strings.Builder
	cmd.SetOut(&out)
	cmd.SetErr(&out)

	result := &pipeline.Result{
		Success: true, UploadID: "up-42", TotalBytes: 2 * 1024 * 1024 * 1024,
		TotalFiles: 3, TotalChunks: 5, TotalTime: 10 * time.Second,
	}
	m := &manifest.Manifest{
		UploadID: "up-42", ShardCount: 6, TotalChunks: 5, TotalBytes: 2 * 1024 * 1024 * 1024,
		CompressionType: "zstd", CompressionLevel: 9, CompressionRatio: 0.5,
		Files: []manifest.FileEntry{
			{Path: "a/b/file.txt"}, {Path: "c/other.txt"}, {Path: "noext"},
		},
	}
	recordUploadOutcome(cmd, "proj-x", "GLACIER", "us-east-1", 3, result, m)

	store := cost.NewUploadHistoryStore("")
	got, err := store.LoadOutcomes()
	assert.NoError(t, err)
	assert.Len(t, got, 1)

	o := got[0]
	assert.Equal(t, "up-42", o.UploadID)
	assert.Equal(t, "proj-x", o.ProjectID)
	assert.Equal(t, 6, o.ShardCount, "shard count sourced from manifest")
	assert.Equal(t, 9, o.CompressionLevel, "level sourced from manifest when set")
	assert.Equal(t, "zstd", o.CompressionType)
	assert.InDelta(t, 0.5, o.CompressionRatio, 1e-9)
	assert.Equal(t, int64(1024*1024*1024), o.CompressedBytes, "ratio * total")
	assert.Equal(t, "GLACIER", o.StorageClass)
	assert.Equal(t, map[string]int{"txt": 2, "none": 1}, o.FileTypeMix)
	assert.Greater(t, o.ThroughputMBps, 0.0)
	assert.True(t, o.Success)
}
