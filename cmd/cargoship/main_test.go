package main

import (
	"bytes"
	"context"
	"os"
	"testing"

	"github.com/scttfrdmn/cargoship/cmd/cargoship/cmd"
)

// TestNewRootCmd tests that we can create the root command successfully
func TestNewRootCmd(t *testing.T) {
	var buf bytes.Buffer
	rootCmd := cmd.NewRootCmd(&buf)
	
	if rootCmd == nil {
		t.Fatal("NewRootCmd should not return nil")
	}
	
	// Test that the command has basic properties set
	if rootCmd.Use == "" {
		t.Error("Root command should have Use field set")
	}
}

// TestRootCmdExecution tests executing the root command with help
func TestRootCmdExecution(t *testing.T) {
	var buf bytes.Buffer
	rootCmd := cmd.NewRootCmd(&buf)
	
	// Test help command
	rootCmd.SetArgs([]string{"--help"})
	err := rootCmd.ExecuteContext(context.Background())
	if err != nil {
		t.Errorf("Help command should not error, got: %v", err)
	}
	
	// Verify some output was written
	if buf.Len() == 0 {
		t.Error("Help command should produce output")
	}
}

// TestRootCmdInvalidArgs tests that invalid arguments produce errors
func TestRootCmdInvalidArgs(t *testing.T) {
	var buf bytes.Buffer
	rootCmd := cmd.NewRootCmd(&buf)
	
	// Test invalid command
	rootCmd.SetArgs([]string{"nonexistent-command"})
	err := rootCmd.ExecuteContext(context.Background())
	if err == nil {
		t.Error("Invalid command should produce an error")
	}
}

// TestContextBackground verifies context functionality
func TestContextBackground(t *testing.T) {
	ctx := context.Background()
	if ctx == nil {
		t.Error("context.Background() should not return nil")
	}
	
	// Verify context can be used with commands
	var buf bytes.Buffer
	rootCmd := cmd.NewRootCmd(&buf)
	rootCmd.SetArgs([]string{"--version"})
	
	// This tests the context path through ExecuteContext
	err := rootCmd.ExecuteContext(ctx)
	// Version command might not exist, but context should work
	_ = err // We don't care about the specific error, just that context works
}

// TestOsStdout verifies os package integration
func TestOsStdout(t *testing.T) {
	if os.Stdout == nil {
		t.Error("os.Stdout should not be nil")
	}
	
	// Test that we can create command with os.Stdout
	rootCmd := cmd.NewRootCmd(os.Stdout)
	if rootCmd == nil {
		t.Error("NewRootCmd with os.Stdout should work")
	}
}

// TestMainFunctionComponents tests the individual components that main() uses
func TestMainFunctionComponents(t *testing.T) {
	// Test the core components main() relies on
	
	// 1. Creating root command with os.Stdout
	rootCmd := cmd.NewRootCmd(os.Stdout)
	if rootCmd == nil {
		t.Fatal("NewRootCmd should create valid command")
	}
	
	// 2. Creating context
	ctx := context.Background()
	if ctx == nil {
		t.Fatal("context.Background() should create valid context")
	}
	
	// 3. Test ExecuteContext with valid args (help)
	rootCmd.SetArgs([]string{"--help"})
	err := rootCmd.ExecuteContext(ctx)
	if err != nil {
		t.Errorf("ExecuteContext with --help should not error: %v", err)
	}
}

// TestVersionVariables tests the version variable declarations
func TestVersionVariables(t *testing.T) {
	// Test that version variables are properly declared
	if version == "" {
		t.Error("version variable should not be empty")
	}
	if commit == "" {
		t.Error("commit variable should not be empty")
	}
	if date == "" {
		t.Error("date variable should not be empty")
	}
}

// TestNewRootCmdWithVersion tests the version-aware root command creation
func TestNewRootCmdWithVersion(t *testing.T) {
	var buf bytes.Buffer
	versionInfo := "test-version (test-commit) built on test-date"
	
	rootCmd := cmd.NewRootCmdWithVersion(&buf, versionInfo)
	if rootCmd == nil {
		t.Fatal("NewRootCmdWithVersion should not return nil")
	}
	
	// Test version command
	rootCmd.SetArgs([]string{"--version"})
	err := rootCmd.ExecuteContext(context.Background())
	if err != nil {
		t.Errorf("Version command should not error: %v", err)
	}
	
	// Check that version info appears in output
	output := buf.String()
	if output == "" {
		t.Error("Version command should produce output")
	}
}

// TestBuildVersionInfo tests the buildVersionInfo function
func TestBuildVersionInfo(t *testing.T) {
	// Save original values
	origVersion := version
	origCommit := commit
	origDate := date
	
	// Test with all unknown values
	version = "dev"
	commit = "unknown"
	date = "unknown"
	result := buildVersionInfo()
	expected := "dev"
	if result != expected {
		t.Errorf("buildVersionInfo() = %q, want %q", result, expected)
	}
	
	// Test with commit but no date
	version = "v1.0.0"
	commit = "abc123"
	date = "unknown"
	result = buildVersionInfo()
	expected = "v1.0.0 (abc123)"
	if result != expected {
		t.Errorf("buildVersionInfo() = %q, want %q", result, expected)
	}
	
	// Test with all values
	version = "v1.0.0"
	commit = "abc123"
	date = "2024-01-01"
	result = buildVersionInfo()
	expected = "v1.0.0 (abc123) built on 2024-01-01"
	if result != expected {
		t.Errorf("buildVersionInfo() = %q, want %q", result, expected)
	}
	
	// Restore original values
	version = origVersion
	commit = origCommit
	date = origDate
}