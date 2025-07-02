package cmd

import (
	"bytes"
	"encoding/json"
	"os"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"github.com/scttfrdmn/cargoship/pkg/config"
)

func TestNewConfigCmd(t *testing.T) {
	cmd := NewConfigCmd()
	require.NotNil(t, cmd)
	assert.Equal(t, "config", cmd.Use)
	assert.Equal(t, "Manage CargoShip configuration", cmd.Short)
	assert.NotEmpty(t, cmd.Long)
	assert.NotNil(t, cmd.RunE)

	// Test flags
	flags := cmd.Flags()
	assert.True(t, flags.HasFlags())
	
	fileFlag := flags.Lookup("file")
	require.NotNil(t, fileFlag)
	assert.Equal(t, "", fileFlag.DefValue)
	
	generateFlag := flags.Lookup("generate")
	require.NotNil(t, generateFlag)
	assert.Equal(t, "false", generateFlag.DefValue)
	
	editFlag := flags.Lookup("edit")
	require.NotNil(t, editFlag)
	assert.Equal(t, "false", editFlag.DefValue)
	
	validateFlag := flags.Lookup("validate")
	require.NotNil(t, validateFlag)
	assert.Equal(t, "false", validateFlag.DefValue)
	
	showFlag := flags.Lookup("show")
	require.NotNil(t, showFlag)
	assert.Equal(t, "false", showFlag.DefValue)
	
	formatFlag := flags.Lookup("format")
	require.NotNil(t, formatFlag)
	assert.Equal(t, "yaml", formatFlag.DefValue)
}

func TestGenerateConfig(t *testing.T) {
	// Capture stdout
	originalStdout := os.Stdout
	r, w, _ := os.Pipe()
	os.Stdout = w

	err := generateConfig()

	// Restore stdout
	_ = w.Close()
	os.Stdout = originalStdout

	assert.NoError(t, err)

	// Read captured output
	var buf bytes.Buffer
	_, _ = buf.ReadFrom(r)
	output := buf.String()

	// Verify output contains expected content
	assert.Contains(t, output, "# CargoShip Configuration Example")
	assert.Contains(t, output, "# Save this to ~/.cargoship.yaml")
	assert.Contains(t, output, "# To save this configuration:")
	assert.Contains(t, output, "cargoship config --generate > ~/.cargoship.yaml")
}

func TestShowConfigJSON(t *testing.T) {
	// Save original configFormat
	originalFormat := configFormat
	defer func() { configFormat = originalFormat }()
	
	// Set format to JSON
	configFormat = "json"
	
	// Create a temporary config file with valid YAML
	tempDir := t.TempDir()
	configPath := filepath.Join(tempDir, ".cargoship.yaml")
	
	validConfig := `
aws:
  region: us-east-1
  profile: default
storage:
  default_bucket: test-bucket
  default_storage_class: STANDARD
upload:
  max_concurrency: 10
  chunk_size: 64MB
metrics:
  enabled: true
  namespace: CargoShip/Test
logging:
  level: info
`
	
	err := os.WriteFile(configPath, []byte(validConfig), 0644)
	require.NoError(t, err)
	
	// Test with file that doesn't exist (manager should use defaults)
	// Capture stdout
	originalStdout := os.Stdout
	r, w, _ := os.Pipe()
	os.Stdout = w
	
	// Create a new manager and call showConfig  
	manager := config.NewManager()
	err = showConfig(manager)
	
	// Restore stdout
	_ = w.Close()
	os.Stdout = originalStdout
	
	assert.NoError(t, err)
	
	// Read and verify JSON output
	var buf bytes.Buffer
	_, _ = buf.ReadFrom(r)
	output := buf.String()
	
	// Should be valid JSON
	var jsonData map[string]interface{}
	err = json.Unmarshal([]byte(output), &jsonData)
	assert.NoError(t, err)
}

func TestShowConfigUnsupportedFormat(t *testing.T) {
	// Save original configFormat
	originalFormat := configFormat
	defer func() { configFormat = originalFormat }()
	
	// Set unsupported format
	configFormat = "xml"
	
	manager := config.NewManager()
	err := showConfig(manager)
	
	assert.Error(t, err)
	assert.Contains(t, err.Error(), "unsupported format: xml")
}

// Tests for individual functions work better than testing the full RunE
// due to global variable flag binding complexities

func TestRunConfigShowHelp(t *testing.T) {
	// Save original flag values
	originalGenerate := configGenerate
	originalShow := configShow
	originalValidate := configValidate
	originalEdit := configEdit
	defer func() { 
		configGenerate = originalGenerate
		configShow = originalShow
		configValidate = originalValidate
		configEdit = originalEdit
	}()
	
	// Reset all flags
	configGenerate = false
	configShow = false
	configValidate = false
	configEdit = false
	
	// Capture stdout
	originalStdout := os.Stdout
	r, w, _ := os.Pipe()
	os.Stdout = w
	
	cmd := NewConfigCmd()
	err := cmd.RunE(cmd, []string{})
	
	// Restore stdout
	_ = w.Close()
	os.Stdout = originalStdout
	
	assert.NoError(t, err)
	
	// Verify help output
	var buf bytes.Buffer
	_, _ = buf.ReadFrom(r)
	output := buf.String()
	assert.Contains(t, output, "Manage CargoShip configuration")
}

func TestValidateConfig(t *testing.T) {
	// Capture stdout
	originalStdout := os.Stdout
	r, w, _ := os.Pipe()
	os.Stdout = w
	
	manager := config.NewManager()
	err := validateConfig(manager)
	
	// Restore stdout
	_ = w.Close()
	os.Stdout = originalStdout
	
	assert.NoError(t, err)
	
	// Verify validation output
	var buf bytes.Buffer
	_, _ = buf.ReadFrom(r)
	output := buf.String()
	
	assert.Contains(t, output, "✅ Configuration is valid!")
	assert.Contains(t, output, "Configuration summary:")
	assert.Contains(t, output, "AWS Region:")
	assert.Contains(t, output, "Storage Class:")
	assert.Contains(t, output, "Upload Concurrency:")
	assert.Contains(t, output, "Metrics Enabled:")
	assert.Contains(t, output, "Log Level:")
}

// Tests use real config.Manager for integration testing

func TestEditConfigNoEditor(t *testing.T) {
	// Save original environment
	originalEditor := os.Getenv("EDITOR")
	originalVisual := os.Getenv("VISUAL")
	defer func() {
		_ = os.Setenv("EDITOR", originalEditor)
		_ = os.Setenv("VISUAL", originalVisual)
	}()
	
	// Clear editor environment variables
	_ = os.Unsetenv("EDITOR")
	_ = os.Unsetenv("VISUAL")
	
	// Save original PATH to restore later
	originalPath := os.Getenv("PATH")
	defer func() { _ = os.Setenv("PATH", originalPath) }()
	
	// Set empty PATH to ensure no editors are found
	_ = os.Setenv("PATH", "")
	
	err := editConfig()
	assert.Error(t, err)
	assert.Contains(t, err.Error(), "no editor found")
}

func TestConfigFlagHandling(t *testing.T) {
	// Test flag state management
	cmd := NewConfigCmd()
	
	// Test setting flags
	_ = cmd.Flags().Set("generate", "true")
	_ = cmd.Flags().Set("format", "json")
	_ = cmd.Flags().Set("file", "/tmp/test.yaml")
	
	// Flags should be accessible
	generateFlag, _ := cmd.Flags().GetBool("generate")
	assert.True(t, generateFlag)
	
	formatFlag, _ := cmd.Flags().GetString("format")
	assert.Equal(t, "json", formatFlag)
	
	fileFlag, _ := cmd.Flags().GetString("file")
	assert.Equal(t, "/tmp/test.yaml", fileFlag)
}

func TestShowConfigYAML(t *testing.T) {
	// Save original configFormat
	originalFormat := configFormat
	defer func() { configFormat = originalFormat }()
	
	// Set format to YAML
	configFormat = "yaml"
	
	// Save original home directory behavior
	// Create temporary directory to act as home
	tempDir := t.TempDir()
	
	// Create mock config manager with default values
	manager := config.NewManager()
	
	// Capture stdout
	originalStdout := os.Stdout
	r, w, _ := os.Pipe()
	os.Stdout = w
	
	// Temporarily change HOME for this test
	originalHome := os.Getenv("HOME")
	_ = os.Setenv("HOME", tempDir)
	defer func() { _ = os.Setenv("HOME", originalHome) }()
	
	err := showConfig(manager)
	
	// Restore stdout
	_ = w.Close()
	os.Stdout = originalStdout
	
	assert.NoError(t, err)
	
	// Read captured output
	var buf bytes.Buffer
	_, _ = buf.ReadFrom(r)
	output := buf.String()
	
	// Should contain YAML content
	assert.Contains(t, output, "aws:")
	assert.Contains(t, output, "region:")
	assert.Contains(t, output, "storage:")
	assert.Contains(t, output, "upload:")
}

func TestShowConfigYMLFormat(t *testing.T) {
	// Save original configFormat
	originalFormat := configFormat
	defer func() { configFormat = originalFormat }()
	
	// Test yml alias for yaml
	configFormat = "yml"
	
	tempDir := t.TempDir()
	originalHome := os.Getenv("HOME")
	_ = os.Setenv("HOME", tempDir)
	defer func() { _ = os.Setenv("HOME", originalHome) }()
	
	manager := config.NewManager()
	
	// Capture stdout
	originalStdout := os.Stdout
	r, w, _ := os.Pipe()
	os.Stdout = w
	
	err := showConfig(manager)
	
	// Restore stdout
	_ = w.Close()
	os.Stdout = originalStdout
	
	assert.NoError(t, err)
	
	// Read captured output
	var buf bytes.Buffer
	_, _ = buf.ReadFrom(r)
	output := buf.String()
	
	// Should contain YAML content
	assert.Contains(t, output, "aws:")
}

func TestEditConfigWithEditor(t *testing.T) {
	if testing.Short() {
		t.Skip("Skipping editor integration test in short mode")
	}
	
	// Save original environment
	originalEditor := os.Getenv("EDITOR")
	originalConfigFile := configFile
	defer func() {
		_ = os.Setenv("EDITOR", originalEditor)
		configFile = originalConfigFile
	}()
	
	// Create temp directory and config file
	tempDir := t.TempDir()
	testConfig := filepath.Join(tempDir, "test-config.yaml")
	configFile = testConfig
	
	// Create initial config content
	initialConfig := `aws:
  region: us-west-2
storage:
  default_bucket: test
`
	err := os.WriteFile(testConfig, []byte(initialConfig), 0644)
	require.NoError(t, err)
	
	// Use 'true' command as a no-op editor for testing
	_ = os.Setenv("EDITOR", "true")
	
	// Capture stdout
	originalStdout := os.Stdout
	r, w, _ := os.Pipe()
	os.Stdout = w
	
	err = editConfig()
	
	// Restore stdout
	_ = w.Close()
	os.Stdout = originalStdout
	
	assert.NoError(t, err)
	
	// Read captured output
	var buf bytes.Buffer
	_, _ = buf.ReadFrom(r)
	output := buf.String()
	
	assert.Contains(t, output, "Opening")
	assert.Contains(t, output, "with true")
	assert.Contains(t, output, "✅ Configuration saved and validated successfully!")
}

func TestEditConfigCreatesNewFile(t *testing.T) {
	if testing.Short() {
		t.Skip("Skipping editor integration test in short mode")
	}
	
	// Save original environment
	originalEditor := os.Getenv("EDITOR")
	originalConfigFile := configFile
	defer func() {
		_ = os.Setenv("EDITOR", originalEditor)
		configFile = originalConfigFile
	}()
	
	// Create temp directory for new config file
	tempDir := t.TempDir()
	testConfig := filepath.Join(tempDir, "new-config.yaml")
	configFile = testConfig
	
	// Use 'true' command as a no-op editor for testing
	_ = os.Setenv("EDITOR", "true")
	
	// Capture stdout
	originalStdout := os.Stdout
	r, w, _ := os.Pipe()
	os.Stdout = w
	
	err := editConfig()
	
	// Restore stdout
	_ = w.Close()
	os.Stdout = originalStdout
	
	assert.NoError(t, err)
	
	// Verify file was created
	_, err = os.Stat(testConfig)
	assert.NoError(t, err)
	
	// Read captured output
	var buf bytes.Buffer
	_, _ = buf.ReadFrom(r)
	output := buf.String()
	
	assert.Contains(t, output, "Creating new configuration file")
	assert.Contains(t, output, "✅ Configuration saved and validated successfully!")
}

func TestEditConfigWithVISUALEditor(t *testing.T) {
	if testing.Short() {
		t.Skip("Skipping editor integration test in short mode")
	}
	
	// Save original environment
	originalEditor := os.Getenv("EDITOR")
	originalVisual := os.Getenv("VISUAL")
	originalConfigFile := configFile
	defer func() {
		_ = os.Setenv("EDITOR", originalEditor)
		_ = os.Setenv("VISUAL", originalVisual)
		configFile = originalConfigFile
	}()
	
	// Clear EDITOR but set VISUAL
	_ = os.Unsetenv("EDITOR")
	_ = os.Setenv("VISUAL", "true")
	
	tempDir := t.TempDir()
	testConfig := filepath.Join(tempDir, "visual-config.yaml")
	configFile = testConfig
	
	// Capture stdout
	originalStdout := os.Stdout
	r, w, _ := os.Pipe()
	os.Stdout = w
	
	err := editConfig()
	
	// Restore stdout
	_ = w.Close()
	os.Stdout = originalStdout
	
	assert.NoError(t, err)
	
	// Read captured output
	var buf bytes.Buffer
	_, _ = buf.ReadFrom(r)
	output := buf.String()
	
	assert.Contains(t, output, "Opening")
	assert.Contains(t, output, "with true")
}

func TestEditConfigInvalidConfiguration(t *testing.T) {
	if testing.Short() {
		t.Skip("Skipping editor integration test in short mode")
	}
	
	// Save original environment
	originalEditor := os.Getenv("EDITOR")
	originalConfigFile := configFile
	defer func() {
		_ = os.Setenv("EDITOR", originalEditor)
		configFile = originalConfigFile
	}()
	
	// Create temp directory and invalid config file
	tempDir := t.TempDir()
	testConfig := filepath.Join(tempDir, "invalid-config.yaml")
	configFile = testConfig
	
	// Create invalid config content that will fail validation
	invalidConfig := `aws:
  region: invalid-region-that-should-fail
  totally_invalid_key: invalid_value
invalid_yaml: [unclosed bracket
`
	err := os.WriteFile(testConfig, []byte(invalidConfig), 0644)
	require.NoError(t, err)
	
	// Use 'true' command as a no-op editor for testing
	_ = os.Setenv("EDITOR", "true")
	
	// Capture stdout
	originalStdout := os.Stdout
	r, w, _ := os.Pipe()
	os.Stdout = w
	
	err = editConfig()
	
	// Restore stdout
	_ = w.Close()
	os.Stdout = originalStdout
	
	// Should not return error even if validation fails
	assert.NoError(t, err)
	
	// Read captured output
	var buf bytes.Buffer
	_, _ = buf.ReadFrom(r)
	output := buf.String()
	
	assert.Contains(t, output, "⚠️ Configuration validation failed")
	assert.Contains(t, output, "Please fix the errors and try again")
}

// Additional tests for runConfig function coverage improvement

func TestRunConfigWithGenerate(t *testing.T) {
	// Capture stdout
	originalStdout := os.Stdout
	r, w, _ := os.Pipe()
	os.Stdout = w
	
	cmd := NewConfigCmd()
	// Set flag through command
	err := cmd.Flags().Set("generate", "true")
	require.NoError(t, err)
	
	err = cmd.RunE(cmd, []string{})
	
	// Restore stdout
	_ = w.Close()
	os.Stdout = originalStdout
	
	assert.NoError(t, err)
	
	// Verify generate output
	var buf bytes.Buffer
	_, _ = buf.ReadFrom(r)
	output := buf.String()
	assert.Contains(t, output, "# CargoShip Configuration Example")
}

func TestRunConfigWithValidate(t *testing.T) {
	// Capture stdout
	originalStdout := os.Stdout
	r, w, _ := os.Pipe()
	os.Stdout = w
	
	cmd := NewConfigCmd()
	// Set flag through command
	err := cmd.Flags().Set("validate", "true")
	require.NoError(t, err)
	
	err = cmd.RunE(cmd, []string{})
	
	// Restore stdout
	_ = w.Close()
	os.Stdout = originalStdout
	
	assert.NoError(t, err)
	
	// Verify validation output
	var buf bytes.Buffer
	_, _ = buf.ReadFrom(r)
	output := buf.String()
	assert.Contains(t, output, "✅ Configuration is valid!")
}

func TestRunConfigWithShow(t *testing.T) {
	// Capture stdout
	originalStdout := os.Stdout
	r, w, _ := os.Pipe()
	os.Stdout = w
	
	cmd := NewConfigCmd()
	// Set flags through command
	err := cmd.Flags().Set("show", "true")
	require.NoError(t, err)
	err = cmd.Flags().Set("format", "yaml")
	require.NoError(t, err)
	
	err = cmd.RunE(cmd, []string{})
	
	// Restore stdout
	_ = w.Close()
	os.Stdout = originalStdout
	
	assert.NoError(t, err)
	
	// Verify show output
	var buf bytes.Buffer
	_, _ = buf.ReadFrom(r)
	output := buf.String()
	assert.Contains(t, output, "aws:")
}

func TestRunConfigWithEdit(t *testing.T) {
	if testing.Short() {
		t.Skip("Skipping editor integration test in short mode")
	}
	
	// Save original environment
	originalEditor := os.Getenv("EDITOR")
	defer func() { 
		_ = os.Setenv("EDITOR", originalEditor)
	}()
	
	// Set editor
	_ = os.Setenv("EDITOR", "true")
	
	// Create temp config file with initial content
	tempDir := t.TempDir()
	testConfig := filepath.Join(tempDir, "edit-test.yaml")
	initialConfig := `aws:
  region: us-west-2
storage:
  default_bucket: test
`
	err := os.WriteFile(testConfig, []byte(initialConfig), 0644)
	require.NoError(t, err)
	
	// Capture stdout
	originalStdout := os.Stdout
	r, w, _ := os.Pipe()
	os.Stdout = w
	
	cmd := NewConfigCmd()
	// Set flags through command
	err = cmd.Flags().Set("edit", "true")
	require.NoError(t, err)
	err = cmd.Flags().Set("file", testConfig)
	require.NoError(t, err)
	
	err = cmd.RunE(cmd, []string{})
	
	// Restore stdout
	_ = w.Close()
	os.Stdout = originalStdout
	
	assert.NoError(t, err)
	
	// Verify edit output
	var buf bytes.Buffer
	_, _ = buf.ReadFrom(r)
	output := buf.String()
	assert.Contains(t, output, "Opening")
}

func TestRunConfigLoadConfigError(t *testing.T) {
	cmd := NewConfigCmd()
	// Set flags through command
	err := cmd.Flags().Set("validate", "true")
	require.NoError(t, err)
	err = cmd.Flags().Set("file", "/nonexistent/path/config.yaml")
	require.NoError(t, err)
	
	err = cmd.RunE(cmd, []string{})
	
	// Should return error when config loading fails
	assert.Error(t, err)
	assert.Contains(t, err.Error(), "failed to load config")
}

func TestRunConfigShowWithInvalidFile(t *testing.T) {
	cmd := NewConfigCmd()
	// Set flags through command
	err := cmd.Flags().Set("show", "true")
	require.NoError(t, err)
	err = cmd.Flags().Set("file", "/nonexistent/directory/config.yaml")
	require.NoError(t, err)
	
	err = cmd.RunE(cmd, []string{})
	
	// Should return error when config loading fails for show
	assert.Error(t, err)
	assert.Contains(t, err.Error(), "failed to load config")
}

func TestRunConfigWithValidFileLoad(t *testing.T) {
	// Create a valid config file
	tempDir := t.TempDir()
	testConfig := filepath.Join(tempDir, "valid-config.yaml")
	validConfig := `aws:
  region: us-east-1
  profile: default
storage:
  default_bucket: test-bucket
`
	err := os.WriteFile(testConfig, []byte(validConfig), 0644)
	require.NoError(t, err)
	
	// Capture stdout
	originalStdout := os.Stdout
	r, w, _ := os.Pipe()
	os.Stdout = w
	
	cmd := NewConfigCmd()
	// Set flags through command
	err = cmd.Flags().Set("show", "true")
	require.NoError(t, err)
	err = cmd.Flags().Set("file", testConfig)
	require.NoError(t, err)
	err = cmd.Flags().Set("format", "yaml")
	require.NoError(t, err)
	
	err = cmd.RunE(cmd, []string{})
	
	// Restore stdout
	_ = w.Close()
	os.Stdout = originalStdout
	
	assert.NoError(t, err)
	
	// Verify output contains configuration (even if merged with defaults)
	var buf bytes.Buffer
	_, _ = buf.ReadFrom(r)
	output := buf.String()
	assert.Contains(t, output, "aws:")
	// The configuration system merges with defaults, so just verify structure
	assert.Contains(t, output, "region:")
	assert.Contains(t, output, "storage:")
}

func TestRunConfigFlagPrecedence(t *testing.T) {
	// Capture stdout
	originalStdout := os.Stdout
	r, w, _ := os.Pipe()
	os.Stdout = w
	
	cmd := NewConfigCmd()
	// Set multiple flags - generate should take precedence
	err := cmd.Flags().Set("generate", "true")
	require.NoError(t, err)
	err = cmd.Flags().Set("show", "true") // This should be ignored
	require.NoError(t, err)
	err = cmd.Flags().Set("validate", "true") // This should be ignored
	require.NoError(t, err)
	
	err = cmd.RunE(cmd, []string{})
	
	// Restore stdout
	_ = w.Close()
	os.Stdout = originalStdout
	
	assert.NoError(t, err)
	
	// Should only run generate, not other actions
	var buf bytes.Buffer
	_, _ = buf.ReadFrom(r)
	output := buf.String()
	assert.Contains(t, output, "# CargoShip Configuration Example")
	// Should not contain validation output
	assert.NotContains(t, output, "✅ Configuration is valid!")
}