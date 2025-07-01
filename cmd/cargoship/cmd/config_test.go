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
	w.Close()
	os.Stdout = originalStdout

	assert.NoError(t, err)

	// Read captured output
	var buf bytes.Buffer
	buf.ReadFrom(r)
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
	w.Close()
	os.Stdout = originalStdout
	
	assert.NoError(t, err)
	
	// Read and verify JSON output
	var buf bytes.Buffer
	buf.ReadFrom(r)
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
	w.Close()
	os.Stdout = originalStdout
	
	assert.NoError(t, err)
	
	// Verify help output
	var buf bytes.Buffer
	buf.ReadFrom(r)
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
	w.Close()
	os.Stdout = originalStdout
	
	assert.NoError(t, err)
	
	// Verify validation output
	var buf bytes.Buffer
	buf.ReadFrom(r)
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
		os.Setenv("EDITOR", originalEditor)
		os.Setenv("VISUAL", originalVisual)
	}()
	
	// Clear editor environment variables
	os.Unsetenv("EDITOR")
	os.Unsetenv("VISUAL")
	
	// Save original PATH to restore later
	originalPath := os.Getenv("PATH")
	defer func() { os.Setenv("PATH", originalPath) }()
	
	// Set empty PATH to ensure no editors are found
	os.Setenv("PATH", "")
	
	err := editConfig()
	assert.Error(t, err)
	assert.Contains(t, err.Error(), "no editor found")
}

func TestConfigFlagHandling(t *testing.T) {
	// Test flag state management
	cmd := NewConfigCmd()
	
	// Test setting flags
	cmd.Flags().Set("generate", "true")
	cmd.Flags().Set("format", "json")
	cmd.Flags().Set("file", "/tmp/test.yaml")
	
	// Flags should be accessible
	generateFlag, _ := cmd.Flags().GetBool("generate")
	assert.True(t, generateFlag)
	
	formatFlag, _ := cmd.Flags().GetString("format")
	assert.Equal(t, "json", formatFlag)
	
	fileFlag, _ := cmd.Flags().GetString("file")
	assert.Equal(t, "/tmp/test.yaml", fileFlag)
}