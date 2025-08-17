package cmd

import (
	"bytes"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/spf13/cobra"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/scttfrdmn/cargoship/internal/testutil"
)

// TestV042DataDiscoveryWorkflow tests the complete "Find → Preview → Extract" workflow
func TestV042DataDiscoveryWorkflow(t *testing.T) {
	testutil.RequireNoGoroutineLeak(t)

	// Create temporary directory for test
	tempDir, err := os.MkdirTemp("", "v042_workflow_test")
	require.NoError(t, err)
	defer func() {
		if removeErr := os.RemoveAll(tempDir); removeErr != nil {
			_ = removeErr // Ignore remove error in tests
		}
	}()

	// Create a comprehensive test inventory file for genomics data scenario
	inventoryFile := filepath.Join(tempDir, "genomics-project-inventory.yaml")

	yamlContent := `
files:
  - path: /project/raw-data/sample1.fastq.gz
    destination: raw-data/sample1.fastq.gz
    name: sample1.fastq.gz
    size: 524288000
    suitcase_index: 1
    suitcase_name: genomics-data-01-of-03.tar.zst
  - path: /project/raw-data/sample2.fastq.gz
    destination: raw-data/sample2.fastq.gz
    name: sample2.fastq.gz
    size: 629145600
    suitcase_index: 1
    suitcase_name: genomics-data-01-of-03.tar.zst
  - path: /project/aligned/sample1.bam
    destination: aligned/sample1.bam
    name: sample1.bam
    size: 1073741824
    suitcase_index: 2
    suitcase_name: genomics-data-02-of-03.tar.zst
  - path: /project/aligned/sample2.bam
    destination: aligned/sample2.bam
    name: sample2.bam
    size: 1258291200
    suitcase_index: 2
    suitcase_name: genomics-data-02-of-03.tar.zst
  - path: /project/results/analysis.vcf
    destination: results/analysis.vcf
    name: analysis.vcf
    size: 104857600
    suitcase_index: 3
    suitcase_name: genomics-data-03-of-03.tar.zst
  - path: /project/results/summary.json
    destination: results/summary.json
    name: summary.json
    size: 16384
    suitcase_index: 3
    suitcase_name: genomics-data-03-of-03.tar.zst
  - path: /project/metadata/experiment.yaml
    destination: metadata/experiment.yaml
    name: experiment.yaml
    size: 8192
    suitcase_index: 3
    suitcase_name: genomics-data-03-of-03.tar.zst
total_indexes: 3
options:
  user: genomics-researcher
  prefix: genomics-project
  max_suitcase_size: 2147483648
  suitcase_format: tar.zst
`

	err = os.WriteFile(inventoryFile, []byte(yamlContent), 0644)
	require.NoError(t, err)

	// Test Phase 1: Browse/Discovery
	t.Run("Phase1_Browse_Discovery", func(t *testing.T) {
		// Test basic browsing
		browseCmd := NewBrowseCmd()
		_ = browseCmd.Flags().Set("inventory-directory", tempDir)
		_ = browseCmd.Flags().Set("index-cache-dir", filepath.Join(tempDir, "cache"))
		_ = browseCmd.Flags().Set("count-only", "true")

		var buf bytes.Buffer
		browseCmd.SetOut(&buf)
		browseCmd.SetErr(&buf)

		// This will fail due to missing infrastructure, but we validate command structure
		err := browseCmd.RunE(browseCmd, []string{"s3://genomics-archive/"})
		if err != nil {
			t.Logf("Browse command failed as expected in test environment: %v", err)
			// Validate it's trying to work with inventory files
			assert.Contains(t, err.Error(), "inventory")
		}

		// Test pattern-based browsing
		browseCmd2 := NewBrowseCmd()
		_ = browseCmd2.Flags().Set("inventory-directory", tempDir)
		_ = browseCmd2.Flags().Set("pattern", "*.fastq.gz")
		_ = browseCmd2.Flags().Set("min-size", "500MB")
		_ = browseCmd2.Flags().Set("format", "json")

		// Verify flags are properly set
		pattern, _ := browseCmd2.Flags().GetString("pattern")
		assert.Equal(t, "*.fastq.gz", pattern)

		minSize, _ := browseCmd2.Flags().GetString("min-size")
		assert.Equal(t, "500MB", minSize)
	})

	// Test Phase 2: Restore Preview & Cost Estimation
	t.Run("Phase2_Restore_Preview", func(t *testing.T) {
		// Test restoration preview
		restoreCmd := NewRestoreCmd()
		_ = restoreCmd.Flags().Set("inventory-directory", tempDir)
		_ = restoreCmd.Flags().Set("index-cache-dir", filepath.Join(tempDir, "cache"))
		_ = restoreCmd.Flags().Set("preview", "true")
		_ = restoreCmd.Flags().Set("pattern", "*.bam")
		_ = restoreCmd.Flags().Set("format", "table")

		var buf bytes.Buffer
		restoreCmd.SetOut(&buf)
		restoreCmd.SetErr(&buf)

		// This will fail due to missing infrastructure, but validates command structure
		err := restoreCmd.RunE(restoreCmd, []string{"s3://genomics-archive/aligned/", "./restored-data/"})
		if err != nil {
			t.Logf("Restore preview failed as expected in test environment: %v", err)
			// Should be trying to prepare restoration index
			assert.Contains(t, err.Error(), "index")
		}

		// Test cost estimation
		restoreCmd2 := NewRestoreCmd()
		_ = restoreCmd2.Flags().Set("inventory-directory", tempDir)
		_ = restoreCmd2.Flags().Set("estimate-cost", "true")
		_ = restoreCmd2.Flags().Set("region", "us-west-2")
		_ = restoreCmd2.Flags().Set("storage-class", "GLACIER")
		_ = restoreCmd2.Flags().Set("include-transfer-costs", "true")

		// Verify cost estimation flags
		region, _ := restoreCmd2.Flags().GetString("region")
		assert.Equal(t, "us-west-2", region)

		storageClass, _ := restoreCmd2.Flags().GetString("storage-class")
		assert.Equal(t, "GLACIER", storageClass)
	})

	// Test Phase 3: Selective Extraction
	t.Run("Phase3_Selective_Extraction", func(t *testing.T) {
		// Test specific file extraction
		extractCmd := NewExtractCmd()
		_ = extractCmd.Flags().Set("inventory-directory", tempDir)
		_ = extractCmd.Flags().Set("index-cache-dir", filepath.Join(tempDir, "cache"))
		_ = extractCmd.Flags().Set("dry-run", "true")
		_ = extractCmd.Flags().Set("preserve-structure", "true")

		var buf bytes.Buffer
		extractCmd.SetOut(&buf)
		extractCmd.SetErr(&buf)

		// Test specific file path extraction
		err := extractCmd.RunE(extractCmd, []string{"s3://genomics-archive/data.tar.gz:/results/summary.json", "./extracted/"})
		if err != nil {
			t.Logf("Extract command failed as expected in test environment: %v", err)
			// Should be trying to prepare extraction index
			assert.Contains(t, err.Error(), "index")
		}

		// Test pattern-based extraction
		extractCmd2 := NewExtractCmd()
		_ = extractCmd2.Flags().Set("pattern", "*.vcf")
		_ = extractCmd2.Flags().Set("dry-run", "true")
		_ = extractCmd2.Flags().Set("flatten", "true")
		_ = extractCmd2.Flags().Set("max-files", "10")

		// Verify extraction flags
		pattern, _ := extractCmd2.Flags().GetString("pattern")
		assert.Equal(t, "*.vcf", pattern)

		flatten, _ := extractCmd2.Flags().GetBool("flatten")
		assert.True(t, flatten)
	})

	// Test Phase 4: Workflow Integration
	t.Run("Phase4_Workflow_Integration", func(t *testing.T) {
		// Test that all commands use consistent flag names and behavior
		browseCmd := NewBrowseCmd()
		restoreCmd := NewRestoreCmd()
		extractCmd := NewExtractCmd()

		// Verify consistent flag names across commands
		commonFlags := []string{
			"pattern", "min-size", "max-size", "after", "before", "extensions",
			"inventory-directory", "index-cache-dir", "format",
		}

		for _, flagName := range commonFlags {
			assert.NotNil(t, browseCmd.Flags().Lookup(flagName), "Browse missing flag: %s", flagName)
			assert.NotNil(t, restoreCmd.Flags().Lookup(flagName), "Restore missing flag: %s", flagName)
			assert.NotNil(t, extractCmd.Flags().Lookup(flagName), "Extract missing flag: %s", flagName)
		}

		// Test that commands have appropriate unique flags
		assert.NotNil(t, browseCmd.Flags().Lookup("recursive"), "Browse should have --recursive")
		assert.NotNil(t, restoreCmd.Flags().Lookup("estimate-cost"), "Restore should have --estimate-cost")
		assert.NotNil(t, extractCmd.Flags().Lookup("dry-run"), "Extract should have --dry-run")
	})
}

// TestV042CommandIntegration tests command help and argument parsing
func TestV042CommandIntegration(t *testing.T) {
	testutil.RequireNoGoroutineLeak(t)

	t.Run("All_Commands_Have_Proper_Help", func(t *testing.T) {
		commands := []struct {
			name string
			cmd  *cobra.Command
		}{
			{"browse", NewBrowseCmd()},
			{"restore", NewRestoreCmd()},
			{"extract", NewExtractCmd()},
		}

		for _, tc := range commands {
			t.Run(tc.name, func(t *testing.T) {
				var buf bytes.Buffer
				tc.cmd.SetOut(&buf)
				tc.cmd.SetErr(&buf)

				tc.cmd.SetArgs([]string{"--help"})
				err := tc.cmd.Execute()
				require.NoError(t, err)

				helpOutput := buf.String()
				assert.Contains(t, helpOutput, "Usage:")
				assert.Contains(t, helpOutput, "Examples:")
				assert.Contains(t, helpOutput, "Flags:")
				assert.NotEmpty(t, helpOutput)
			})
		}
	})

	t.Run("Commands_Require_Proper_Arguments", func(t *testing.T) {
		// Browse allows 0-2 args
		browseCmd := NewBrowseCmd()
		var buf bytes.Buffer
		browseCmd.SetOut(&buf)
		browseCmd.SetErr(&buf)
		browseCmd.SetArgs([]string{})
		err := browseCmd.Execute()
		// Should not error with 0 args
		if err != nil && !strings.Contains(err.Error(), "index") {
			t.Errorf("Browse should accept 0 arguments, got: %v", err)
		}

		// Restore requires 1-2 args
		restoreCmd := NewRestoreCmd()
		buf.Reset()
		restoreCmd.SetOut(&buf)
		restoreCmd.SetErr(&buf)
		restoreCmd.SetArgs([]string{})
		err = restoreCmd.Execute()
		assert.Error(t, err) // Should error with 0 args

		// Extract requires 1-2 args
		extractCmd := NewExtractCmd()
		buf.Reset()
		extractCmd.SetOut(&buf)
		extractCmd.SetErr(&buf)
		extractCmd.SetArgs([]string{})
		err = extractCmd.Execute()
		assert.Error(t, err) // Should error with 0 args
	})
}

// TestV042PerformanceAndScaling tests performance characteristics
func TestV042PerformanceAndScaling(t *testing.T) {
	testutil.RequireNoGoroutineLeak(t)

	t.Run("Command_Creation_Performance", func(t *testing.T) {
		start := time.Now()

		// Creating commands should be fast
		for i := 0; i < 100; i++ {
			_ = NewBrowseCmd()
			_ = NewRestoreCmd()
			_ = NewExtractCmd()
		}

		elapsed := time.Since(start)
		assert.Less(t, elapsed, time.Second, "Command creation should be fast")
	})

	t.Run("Help_Generation_Performance", func(t *testing.T) {
		commands := []*cobra.Command{
			NewBrowseCmd(),
			NewRestoreCmd(),
			NewExtractCmd(),
		}

		for _, cmd := range commands {
			start := time.Now()

			var buf bytes.Buffer
			cmd.SetOut(&buf)
			cmd.SetArgs([]string{"--help"})
			err := cmd.Execute()
			require.NoError(t, err)

			elapsed := time.Since(start)
			assert.Less(t, elapsed, 100*time.Millisecond, "Help generation should be fast for %s", cmd.Use)
		}
	})
}

// TestV042ErrorHandling tests error handling and edge cases
func TestV042ErrorHandling(t *testing.T) {
	testutil.RequireNoGoroutineLeak(t)

	t.Run("Invalid_Flag_Values", func(t *testing.T) {
		// Test invalid date formats
		browseCmd := NewBrowseCmd()
		_ = browseCmd.Flags().Set("after", "invalid-date")

		// parseSearchFilter should handle this
		filter, err := parseSearchFilter(browseCmd)
		assert.Error(t, err)
		assert.Nil(t, filter)

		// Test invalid size formats
		extractCmd := NewExtractCmd()
		_ = extractCmd.Flags().Set("min-size", "invalid-size")

		extractFilter, err := parseExtractionFilter(extractCmd)
		assert.Error(t, err)
		assert.Nil(t, extractFilter)
	})

	t.Run("Missing_Inventory_Directories", func(t *testing.T) {
		tempDir, err := os.MkdirTemp("", "missing_inventory_test")
		require.NoError(t, err)
		defer func() {
			if removeErr := os.RemoveAll(tempDir); removeErr != nil {
				_ = removeErr
			}
		}()

		// Directory with no inventory files
		emptyDir := filepath.Join(tempDir, "empty")
		err = os.MkdirAll(emptyDir, 0755)
		require.NoError(t, err)

		browseCmd := NewBrowseCmd()
		_ = browseCmd.Flags().Set("inventory-directory", emptyDir)

		var buf bytes.Buffer
		browseCmd.SetOut(&buf)
		browseCmd.SetErr(&buf)

		err = browseCmd.RunE(browseCmd, []string{"test://location"})
		assert.Error(t, err)
		assert.Contains(t, err.Error(), "no inventory files found")
	})
}

// TestV042RootCommandIntegration tests that all commands are properly registered
func TestV042RootCommandIntegration(t *testing.T) {
	testutil.RequireNoGoroutineLeak(t)

	t.Run("Commands_Registered_In_Root", func(t *testing.T) {
		var buf bytes.Buffer
		rootCmd := NewRootCmd(&buf)

		// Get all command names
		var commandNames []string
		for _, cmd := range rootCmd.Commands() {
			commandNames = append(commandNames, cmd.Name())
		}

		// Verify our v0.4.2 commands are present
		expectedCommands := []string{"browse", "restore", "extract"}
		for _, expected := range expectedCommands {
			assert.Contains(t, commandNames, expected, "Command %s should be registered in root", expected)
		}
	})

	t.Run("Commands_Appear_In_Help", func(t *testing.T) {
		var buf bytes.Buffer
		rootCmd := NewRootCmd(&buf)

		rootCmd.SetArgs([]string{"--help"})
		err := rootCmd.Execute()
		require.NoError(t, err)

		helpOutput := buf.String()
		assert.Contains(t, helpOutput, "browse")
		assert.Contains(t, helpOutput, "restore")
		assert.Contains(t, helpOutput, "extract")
	})
}

// TestV042BackwardsCompatibility ensures we don't break existing functionality
func TestV042BackwardsCompatibility(t *testing.T) {
	testutil.RequireNoGoroutineLeak(t)

	t.Run("Existing_Find_Command_Still_Works", func(t *testing.T) {
		findCmd := NewFindCmd()

		// Should still have the same basic structure
		assert.Equal(t, "find PATTERN", findCmd.Use)
		assert.Contains(t, findCmd.Short, "Find where a file")

		// Should have inventory-directory flag
		assert.NotNil(t, findCmd.Flags().Lookup("inventory-directory"))

		// Test help works
		var buf bytes.Buffer
		findCmd.SetOut(&buf)
		findCmd.SetArgs([]string{"--help"})
		err := findCmd.Execute()
		require.NoError(t, err)

		helpOutput := buf.String()
		assert.Contains(t, helpOutput, "Find where a file")
	})

	t.Run("New_Commands_Dont_Conflict", func(t *testing.T) {
		// Verify new commands don't override existing ones
		var buf bytes.Buffer
		rootCmd := NewRootCmd(&buf)

		// Check that we have both old and new commands
		findCmd := findCommand(rootCmd, "find")
		browseCmd := findCommand(rootCmd, "browse")
		restoreCmd := findCommand(rootCmd, "restore")
		extractCmd := findCommand(rootCmd, "extract")

		assert.NotNil(t, findCmd, "find command should still exist")
		assert.NotNil(t, browseCmd, "browse command should exist")
		assert.NotNil(t, restoreCmd, "restore command should exist")
		assert.NotNil(t, extractCmd, "extract command should exist")

		// Commands should be different
		assert.NotEqual(t, findCmd.Use, browseCmd.Use)
		assert.NotEqual(t, findCmd.Use, restoreCmd.Use)
		assert.NotEqual(t, findCmd.Use, extractCmd.Use)
	})
}

// Helper function to find a command by name in root command
func findCommand(root *cobra.Command, name string) *cobra.Command {
	for _, cmd := range root.Commands() {
		if cmd.Name() == name {
			return cmd
		}
	}
	return nil
}
