package cmd

import (
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// TestParseS3URL_Download tests S3 URL parsing for download command (Issue #96)
func TestParseS3URL_Download(t *testing.T) {
	tests := []struct {
		name         string
		s3URL        string
		expectBucket string
		expectPrefix string
		expectError  bool
	}{
		{
			name:         "bucket with full upload path",
			s3URL:        "s3://my-bucket/uploads/20231208-123456-abcd1234",
			expectBucket: "my-bucket",
			expectPrefix: "uploads/20231208-123456-abcd1234",
			expectError:  false,
		},
		{
			name:         "bucket with nested prefix",
			s3URL:        "s3://prod-bucket/data/backups/uploads/20231208-123456",
			expectBucket: "prod-bucket",
			expectPrefix: "data/backups/uploads/20231208-123456",
			expectError:  false,
		},
		{
			name:         "bucket only",
			s3URL:        "s3://my-bucket",
			expectBucket: "my-bucket",
			expectPrefix: "",
			expectError:  false,
		},
		{
			name:         "bucket with single level prefix",
			s3URL:        "s3://my-bucket/uploads",
			expectBucket: "my-bucket",
			expectPrefix: "uploads",
			expectError:  false,
		},
		{
			name:         "missing s3:// prefix",
			s3URL:        "my-bucket/uploads/upload-id",
			expectBucket: "",
			expectPrefix: "",
			expectError:  true,
		},
		{
			name:         "empty string",
			s3URL:        "",
			expectBucket: "",
			expectPrefix: "",
			expectError:  true,
		},
		{
			name:         "invalid protocol",
			s3URL:        "https://my-bucket/key",
			expectBucket: "",
			expectPrefix: "",
			expectError:  true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			bucket, prefix, err := parseS3URL(tt.s3URL)

			if tt.expectError {
				assert.Error(t, err, "Expected error for invalid S3 URL")
			} else {
				require.NoError(t, err, "Should parse valid S3 URL")
				assert.Equal(t, tt.expectBucket, bucket, "Bucket mismatch")
				assert.Equal(t, tt.expectPrefix, prefix, "Prefix mismatch")
			}
		})
	}
}

// TestDownloadCmd_Creation tests command creation (Issue #96)
func TestDownloadCmd_Creation(t *testing.T) {
	cmd := NewDownloadCmd()

	assert.NotNil(t, cmd)
	assert.Equal(t, "download", cmd.Use[:8])
	assert.Contains(t, cmd.Short, "Download")
	assert.Contains(t, cmd.Long, "selective extraction")
}

// TestDownloadCmd_Flags tests flag definitions (Issue #96)
func TestDownloadCmd_Flags(t *testing.T) {
	cmd := NewDownloadCmd()

	tests := []struct {
		flagName     string
		expectExists bool
	}{
		{"pattern", true},
		{"files", true},
		{"shard-ids", true},
		{"region", true},
		{"verbose", true},
		{"dry-run", true},
		{"workers", true},
		{"bucket", false},     // Should not exist (using positional arg)
		{"upload-id", false},  // Should not exist (using positional arg)
		{"output-dir", false}, // Should not exist (using positional arg)
	}

	for _, tt := range tests {
		t.Run(tt.flagName, func(t *testing.T) {
			flag := cmd.Flags().Lookup(tt.flagName)
			if tt.expectExists {
				assert.NotNil(t, flag, "Flag %s should exist", tt.flagName)
			} else {
				assert.Nil(t, flag, "Flag %s should not exist", tt.flagName)
			}
		})
	}
}

// TestDownloadCmd_PatternFlag tests pattern flag configuration (Issue #96)
func TestDownloadCmd_PatternFlag(t *testing.T) {
	cmd := NewDownloadCmd()

	flag := cmd.Flags().Lookup("pattern")
	require.NotNil(t, flag, "pattern flag should exist")

	assert.Equal(t, "", flag.DefValue, "pattern should default to empty")
	assert.Contains(t, flag.Usage, "glob", "Flag usage should mention glob pattern")
}

// TestDownloadCmd_FilesFlag tests files flag configuration (Issue #96)
func TestDownloadCmd_FilesFlag(t *testing.T) {
	cmd := NewDownloadCmd()

	flag := cmd.Flags().Lookup("files")
	require.NotNil(t, flag, "files flag should exist")

	assert.Contains(t, flag.Usage, "exact file paths", "Flag usage should mention exact file paths")
}

// TestDownloadCmd_ShardIDsFlag tests shard-ids flag configuration (Issue #96)
func TestDownloadCmd_ShardIDsFlag(t *testing.T) {
	cmd := NewDownloadCmd()

	flag := cmd.Flags().Lookup("shard-ids")
	require.NotNil(t, flag, "shard-ids flag should exist")

	assert.Contains(t, flag.Usage, "shard IDs", "Flag usage should mention shard IDs")
}

// TestDownloadCmd_DryRunFlag tests dry-run flag configuration (Issue #96)
func TestDownloadCmd_DryRunFlag(t *testing.T) {
	cmd := NewDownloadCmd()

	flag := cmd.Flags().Lookup("dry-run")
	require.NotNil(t, flag, "dry-run flag should exist")

	assert.Equal(t, "false", flag.DefValue, "dry-run should default to false")
	assert.Contains(t, flag.Usage, "would be downloaded", "Flag usage should describe dry-run behavior")
}

// TestDownloadCmd_RegionFlag tests region flag configuration (Issue #96)
func TestDownloadCmd_RegionFlag(t *testing.T) {
	cmd := NewDownloadCmd()

	flag := cmd.Flags().Lookup("region")
	require.NotNil(t, flag, "region flag should exist")

	assert.Equal(t, "us-west-2", flag.DefValue, "region should default to us-west-2")
	assert.Equal(t, "r", flag.Shorthand, "region should have -r shorthand")
}

// TestDownloadCmd_Examples tests command examples (Issue #96)
func TestDownloadCmd_Examples(t *testing.T) {
	cmd := NewDownloadCmd()

	// Verify examples demonstrate key features
	assert.Contains(t, cmd.Long, "--pattern", "Should show pattern example")
	assert.Contains(t, cmd.Long, "--files", "Should show files example")
	assert.Contains(t, cmd.Long, "--shard-ids", "Should show shard-ids example")
	assert.Contains(t, cmd.Long, "--dry-run", "Should show dry-run example")
	assert.Contains(t, cmd.Long, "s3://", "Should show S3 URL format")
}

// TestDownloadCmd_ArgsValidation tests argument count validation (Issue #96)
func TestDownloadCmd_ArgsValidation(t *testing.T) {
	cmd := NewDownloadCmd()

	tests := []struct {
		name        string
		args        []string
		expectError bool
	}{
		{
			name:        "no arguments",
			args:        []string{},
			expectError: true, // Requires at least S3 URL
		},
		{
			name:        "one argument (S3 URL only, dry-run)",
			args:        []string{"s3://bucket/uploads/id"},
			expectError: false, // Valid for dry-run
		},
		{
			name:        "two arguments (S3 URL + output dir)",
			args:        []string{"s3://bucket/uploads/id", "./output"},
			expectError: false, // Valid
		},
		{
			name:        "three arguments (too many)",
			args:        []string{"s3://bucket/uploads/id", "./output", "extra"},
			expectError: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := cmd.Args(cmd, tt.args)
			if tt.expectError {
				assert.Error(t, err, "Should reject invalid arg count")
			} else {
				assert.NoError(t, err, "Should accept valid arg count")
			}
		})
	}
}

// TestDownloadCmd_UsageOutput tests help/usage output (Issue #96)
func TestDownloadCmd_UsageOutput(t *testing.T) {
	cmd := NewDownloadCmd()

	// Get usage string
	usage := cmd.UsageString()

	// Verify key elements in usage
	assert.True(t, strings.Contains(usage, "S3_URL") || strings.Contains(usage, "download"),
		"Usage should show S3_URL argument")
	assert.True(t, strings.Contains(usage, "OUTPUT_DIR") || strings.Contains(usage, "output"),
		"Usage should show OUTPUT_DIR argument")
	assert.Contains(t, usage, "--pattern", "Usage should list pattern flag")
	assert.Contains(t, usage, "--files", "Usage should list files flag")
	assert.Contains(t, usage, "--shard-ids", "Usage should list shard-ids flag")
}

// TestDownloadCmd_SafetyFlagParity pins download's safety surface to restore's.
// download used to carry its own tar-extraction loop that bypassed the shared
// SelectiveExtractor, and so silently lacked verify-on-restore (#283) and the
// dataset-relative layout (#287) that restore had. Both commands now route
// through the same extractor, so they must expose the same controls over it —
// if one grows a flag the other doesn't, they have diverged again. (#311)
func TestDownloadCmd_SafetyFlagParity(t *testing.T) {
	dl := NewDownloadCmd()
	rs := NewRestoreCmd()

	for _, name := range []string{"no-verify", "flatten"} {
		t.Run(name, func(t *testing.T) {
			d := dl.Flags().Lookup(name)
			require.NotNil(t, d, "download must expose --%s", name)
			r := rs.Flags().Lookup(name)
			require.NotNil(t, r, "restore must expose --%s", name)
			// Same default, so the same command line means the same thing.
			assert.Equal(t, r.DefValue, d.DefValue,
				"--%s default differs between download and restore", name)
		})
	}
}

// TestDownloadCmd_VerifiesByDefault asserts integrity checking is opt-out, not
// opt-in: a download that silently writes corrupt bytes is worse than a slow
// one. (#283 / #311)
func TestDownloadCmd_VerifiesByDefault(t *testing.T) {
	f := NewDownloadCmd().Flags().Lookup("no-verify")
	require.NotNil(t, f)
	assert.Equal(t, "false", f.DefValue, "verification must be on by default")
}
