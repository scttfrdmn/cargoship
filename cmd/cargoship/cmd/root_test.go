package cmd

import (
	"bytes"
	"log/slog"
	"testing"
	"time"

	"github.com/charmbracelet/log"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestNewRootCmd(t *testing.T) {
	var buf bytes.Buffer
	cmd := NewRootCmd(&buf)

	require.NotNil(t, cmd)
	assert.Equal(t, "cargoship", cmd.Use)
	assert.Equal(t, "Enterprise data archiving for AWS", cmd.Short)
	assert.Equal(t, "dev", cmd.Version)
	assert.NotEmpty(t, cmd.Long)
	assert.True(t, cmd.SilenceUsage)
	assert.True(t, cmd.SilenceErrors)

	// Test that the command has expected subcommands
	subCommands := make(map[string]bool)
	for _, subcmd := range cmd.Commands() {
		subCommands[subcmd.Use] = true
	}

	// Test that some essential commands are present
	essentialCommands := []string{"create", "config", "benchmark", "wizard", "schema"}
	for _, expected := range essentialCommands {
		assert.True(t, subCommands[expected], "Expected essential subcommand %s not found", expected)
	}

	// Verify we have some subcommands (should be > 5)
	assert.Greater(t, len(cmd.Commands()), 5, "Should have multiple subcommands")
}

func TestNewRootCmdWithVersion(t *testing.T) {
	var buf bytes.Buffer
	testVersion := "v1.2.3"

	cmd := NewRootCmdWithVersion(&buf, testVersion)

	require.NotNil(t, cmd)
	assert.Equal(t, "cargoship", cmd.Use)
	assert.Equal(t, testVersion, cmd.Version)
	assert.NotEmpty(t, cmd.Long)

	// Test that output writer is set correctly
	assert.Equal(t, &buf, cmd.OutOrStdout())

	// Test flags are set up
	flags := cmd.PersistentFlags()
	assert.True(t, flags.HasFlags())

	verboseFlag := flags.Lookup("verbose")
	require.NotNil(t, verboseFlag)
	assert.Equal(t, "false", verboseFlag.DefValue)

	traceFlag := flags.Lookup("trace")
	require.NotNil(t, traceFlag)
	assert.Equal(t, "false", traceFlag.DefValue)

	profileFlag := flags.Lookup("profile")
	require.NotNil(t, profileFlag)
	assert.Equal(t, "false", profileFlag.DefValue)

	memoryLimitFlag := flags.Lookup("memory-limit")
	require.NotNil(t, memoryLimitFlag)
	assert.Equal(t, "", memoryLimitFlag.DefValue)
}

// Note: checkErr calls os.Exit() so it's difficult to test directly
// We test it indirectly through its error handling logic

func TestNewLoggerOpts(t *testing.T) {
	// Save original values
	originalVerbose := Verbose
	originalTrace := trace
	defer func() {
		Verbose = originalVerbose
		trace = originalTrace
	}()

	// Test default options
	Verbose = false
	trace = false
	opts := newLoggerOpts()

	assert.True(t, opts.ReportTimestamp)
	assert.Equal(t, time.Kitchen, opts.TimeFormat)
	assert.Equal(t, "cargoship 🚢 ", opts.Prefix)
	assert.Equal(t, log.InfoLevel, opts.Level)
	assert.False(t, opts.ReportCaller)

	// Test verbose mode
	Verbose = true
	trace = false
	opts = newLoggerOpts()
	assert.Equal(t, log.DebugLevel, opts.Level)
	assert.False(t, opts.ReportCaller)

	// Test trace mode
	Verbose = false
	trace = true
	opts = newLoggerOpts()
	assert.Equal(t, log.InfoLevel, opts.Level)
	assert.True(t, opts.ReportCaller)

	// Test both verbose and trace
	Verbose = true
	trace = true
	opts = newLoggerOpts()
	assert.Equal(t, log.DebugLevel, opts.Level)
	assert.True(t, opts.ReportCaller)
}

func TestNewJSONLoggerOpts(t *testing.T) {
	// Save original values
	originalVerbose := Verbose
	originalTrace := trace
	defer func() {
		Verbose = originalVerbose
		trace = originalTrace
	}()

	// Test default options
	Verbose = false
	trace = false
	opts := newJSONLoggerOpts()

	assert.True(t, opts.ReportTimestamp)
	assert.Equal(t, "cargoship", opts.Prefix)
	assert.Equal(t, log.InfoLevel, opts.Level)
	assert.False(t, opts.ReportCaller)
	assert.Equal(t, log.JSONFormatter, opts.Formatter)

	// Test verbose mode
	Verbose = true
	opts = newJSONLoggerOpts()
	assert.Equal(t, log.DebugLevel, opts.Level)

	// Test trace mode
	trace = true
	opts = newJSONLoggerOpts()
	assert.True(t, opts.ReportCaller)
}

func TestSetupLogging(t *testing.T) {
	var buf bytes.Buffer

	// Test normal setup
	setupLogging(&buf)

	// Verify logger is set
	assert.NotNil(t, logger)
	assert.NotNil(t, slog.Default())

	// Test that panic occurs with nil writer
	assert.Panics(t, func() {
		setupLogging(nil)
	}, "Should panic with nil writer")
}

func TestSetupLoggingOutput(t *testing.T) {
	var buf bytes.Buffer
	setupLogging(&buf)

	// Test that logging actually writes to the buffer
	slog.Info("test message")

	output := buf.String()
	assert.Contains(t, output, "test message")
	assert.Contains(t, output, "cargoship 🚢")
}

func TestToPTR(t *testing.T) {
	// Test with int
	intVal := 42
	intPtr := toPTR(intVal)
	require.NotNil(t, intPtr)
	assert.Equal(t, intVal, *intPtr)

	// Test with string
	strVal := "hello"
	strPtr := toPTR(strVal)
	require.NotNil(t, strPtr)
	assert.Equal(t, strVal, *strPtr)

	// Test with bool
	boolVal := true
	boolPtr := toPTR(boolVal)
	require.NotNil(t, boolPtr)
	assert.Equal(t, boolVal, *boolPtr)

	// Test with struct
	type testStruct struct {
		Field string
	}
	structVal := testStruct{Field: "test"}
	structPtr := toPTR(structVal)
	require.NotNil(t, structPtr)
	assert.Equal(t, structVal, *structPtr)

	// Test that pointer is different from original
	newIntVal := 123
	*intPtr = newIntVal
	assert.NotEqual(t, intVal, newIntVal, "Original value should be unchanged")
	assert.Equal(t, newIntVal, *intPtr, "Pointer should point to new value")
}

// Note: GlobalPersistentPreRun and PostRun functions interact with external state
// and are tested indirectly through command execution

func TestRootCommandIntegration(t *testing.T) {
	// Test that the root command can be created and has basic functionality
	var buf bytes.Buffer
	cmd := NewRootCmd(&buf)

	// Test version output
	cmd.SetArgs([]string{"--version"})
	err := cmd.Execute()
	assert.NoError(t, err)

	output := buf.String()
	assert.Contains(t, output, "dev")
}

func TestRootCommandFlags(t *testing.T) {
	var buf bytes.Buffer
	cmd := NewRootCmd(&buf)

	// Test setting flags
	cmd.SetArgs([]string{"--verbose", "--help"})
	err := cmd.Execute()
	assert.NoError(t, err)

	// Verify flag was processed
	verbose, err := cmd.Flags().GetBool("verbose")
	assert.NoError(t, err)
	assert.True(t, verbose)
}

func TestLoggerOptionsFormats(t *testing.T) {
	// Test that logger options produce valid configurations
	opts := newLoggerOpts()

	// Verify time format is valid
	now := time.Now()
	formatted := now.Format(opts.TimeFormat)
	assert.NotEmpty(t, formatted)

	// Test JSON logger options
	jsonOpts := newJSONLoggerOpts()
	assert.NotNil(t, jsonOpts.Formatter)
}

func TestUtilityFunctionsCoverage(t *testing.T) {
	// Test edge cases for utility functions

	// Test toPTR with zero values
	zeroInt := 0
	zeroPtr := toPTR(zeroInt)
	assert.Equal(t, 0, *zeroPtr)

	emptyString := ""
	emptyPtr := toPTR(emptyString)
	assert.Equal(t, "", *emptyPtr)

	// Test logger setup edge cases
	var buf bytes.Buffer
	setupLogging(&buf)

	// Verify logger configuration
	assert.NotNil(t, logger)

	// Test logging functionality
	slog.Info("test coverage message")
	output := buf.String()
	assert.Contains(t, output, "test coverage message")
}
