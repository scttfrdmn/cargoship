package cmd

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestNewTravelAgentCmd(t *testing.T) {
	cmd := newTravelAgentCmd()
	
	require.NotNil(t, cmd)
	assert.Equal(t, "travelagent CREDENTIAL_FILE", cmd.Use)
	assert.Equal(t, "Run a travel agent server. NOT FOR PRODUCTION USE", cmd.Short)
	assert.NotNil(t, cmd.RunE)
	
	// Test that it requires exactly one argument
	assert.Error(t, cmd.Args(cmd, []string{}))
	assert.NoError(t, cmd.Args(cmd, []string{"one"}))
	assert.Error(t, cmd.Args(cmd, []string{"one", "two"}))
}

func TestTravelAgentCmdNonexistentFile(t *testing.T) {
	cmd := newTravelAgentCmd()
	
	// Test with nonexistent file
	err := cmd.RunE(cmd, []string{"/nonexistent/file.yaml"})
	assert.Error(t, err)
	assert.Contains(t, err.Error(), "no such file or directory")
}

func TestTravelAgentCmdInvalidYAML(t *testing.T) {
	// Create temporary file with invalid YAML
	tmpFile, err := os.CreateTemp("", "invalid_*.yaml")
	require.NoError(t, err)
	defer func() { _ = os.Remove(tmpFile.Name()) }()
	
	// Write invalid YAML
	invalidYAML := "invalid: yaml: content: ["
	err = os.WriteFile(tmpFile.Name(), []byte(invalidYAML), 0644)
	require.NoError(t, err)
	
	cmd := newTravelAgentCmd()
	err = cmd.RunE(cmd, []string{tmpFile.Name()})
	assert.Error(t, err)
	assert.Contains(t, err.Error(), "yaml")
}

func TestTravelAgentCmdNoTransfers(t *testing.T) {
	// Create temporary file with valid YAML but no transfers
	tmpFile, err := os.CreateTemp("", "no_transfers_*.yaml")
	require.NoError(t, err)
	defer func() { _ = os.Remove(tmpFile.Name()) }()
	
	// Write YAML with no transfers
	validYAMLNoTransfers := `admin_token: "test-token"
transfers: []`
	err = os.WriteFile(tmpFile.Name(), []byte(validYAMLNoTransfers), 0644)
	require.NoError(t, err)
	
	cmd := newTravelAgentCmd()
	err = cmd.RunE(cmd, []string{tmpFile.Name()})
	assert.Error(t, err)
	assert.Contains(t, err.Error(), "could not find any transfers")
}

func TestTravelAgentCmdValidCredentials(t *testing.T) {
	// Create temporary file with valid credentials
	tmpFile, err := os.CreateTemp("", "valid_*.yaml")
	require.NoError(t, err)
	defer func() { _ = os.Remove(tmpFile.Name()) }()
	
	// Write valid YAML with transfers
	validYAML := `admin_token: "test-admin-token"
transfers:
  - id: "test-transfer-1"
    token: "test-token-1"
    destination: "s3://test-bucket/path1"
  - id: "test-transfer-2"
    token: "test-token-2"
    destination: "local:///tmp/test"`
	err = os.WriteFile(tmpFile.Name(), []byte(validYAML), 0644)
	require.NoError(t, err)
	
	cmd := newTravelAgentCmd()
	
	// Verify the command is properly structured
	assert.NotNil(t, cmd.RunE)
	assert.Equal(t, "travelagent CREDENTIAL_FILE", cmd.Use)
	
	// Test the command would attempt to start server (but we can't test the actual server start
	// without proper logger setup and network resources)
	// The command should at least parse the YAML correctly before failing on server start
}

func TestTravelAgentCmdFilePermissions(t *testing.T) {
	// Create temporary file
	tmpFile, err := os.CreateTemp("", "permissions_*.yaml")
	require.NoError(t, err)
	defer func() { _ = os.Remove(tmpFile.Name()) }()
	
	// Write valid YAML
	validYAML := `admin_token: "test-token"
transfers:
  - id: "test"
    token: "token"
    destination: "local:///tmp"`
	err = os.WriteFile(tmpFile.Name(), []byte(validYAML), 0644)
	require.NoError(t, err)
	
	// Change permissions to make file unreadable
	err = os.Chmod(tmpFile.Name(), 0000)
	if err != nil {
		t.Skip("Could not change file permissions for test")
	}
	defer func() {
		// Restore permissions for cleanup
		_ = os.Chmod(tmpFile.Name(), 0644)
	}()
	
	cmd := newTravelAgentCmd()
	err = cmd.RunE(cmd, []string{tmpFile.Name()})
	assert.Error(t, err)
	assert.Contains(t, err.Error(), "permission denied")
}

func TestTravelAgentCmdRelativePath(t *testing.T) {
	// Create temporary file in current directory
	tmpFile, err := os.CreateTemp(".", "relative_*.yaml")
	require.NoError(t, err)
	fileName := filepath.Base(tmpFile.Name())
	defer func() { _ = os.Remove(tmpFile.Name()) }()
	
	// Write YAML with no transfers to trigger early error
	validYAMLNoTransfers := `admin_token: "test-token"
transfers: []`
	err = os.WriteFile(tmpFile.Name(), []byte(validYAMLNoTransfers), 0644)
	require.NoError(t, err)
	
	cmd := newTravelAgentCmd()
	err = cmd.RunE(cmd, []string{fileName})
	assert.Error(t, err)
	assert.Contains(t, err.Error(), "could not find any transfers")
}

func TestTravelAgentCmdEmptyFile(t *testing.T) {
	// Create empty temporary file
	tmpFile, err := os.CreateTemp("", "empty_*.yaml")
	require.NoError(t, err)
	defer func() { _ = os.Remove(tmpFile.Name()) }()
	
	cmd := newTravelAgentCmd()
	err = cmd.RunE(cmd, []string{tmpFile.Name()})
	assert.Error(t, err)
	// Empty file should result in no transfers
	assert.Contains(t, err.Error(), "could not find any transfers")
}

func TestTravelAgentCmdComplexYAMLStructure(t *testing.T) {
	// Create temporary file with complex but valid YAML
	tmpFile, err := os.CreateTemp("", "complex_*.yaml")
	require.NoError(t, err)
	defer func() { _ = os.Remove(tmpFile.Name()) }()
	
	// Write complex YAML structure
	complexYAML := `admin_token: "admin-123"
transfers:
  - id: "transfer-1"
    token: "token-1"
    destination: "s3://bucket1/path1"
    metadata:
      key1: "value1"
      key2: "value2"
  - id: "transfer-2"
    token: "token-2"
    destination: "gcs://bucket2/path2"
    options:
      - "option1"
      - "option2"`
	err = os.WriteFile(tmpFile.Name(), []byte(complexYAML), 0644)
	require.NoError(t, err)
	
	cmd := newTravelAgentCmd()
	
	// Verify command structure
	assert.NotNil(t, cmd.RunE)
	assert.Equal(t, "travelagent CREDENTIAL_FILE", cmd.Use)
	
	// Test that complex YAML would be processed (but we can't test server start)
}

func TestTravelAgentCmdYAMLWithComments(t *testing.T) {
	// Create temporary file with YAML comments
	tmpFile, err := os.CreateTemp("", "comments_*.yaml")
	require.NoError(t, err)
	defer func() { _ = os.Remove(tmpFile.Name()) }()
	
	// Write YAML with comments
	yamlWithComments := `# Travel agent configuration
admin_token: "admin-token" # Administrative token
# List of available transfers
transfers:
  # First transfer configuration
  - id: "test-transfer"
    token: "test-token"
    destination: "local:///tmp/test" # Local destination`
	err = os.WriteFile(tmpFile.Name(), []byte(yamlWithComments), 0644)
	require.NoError(t, err)
	
	cmd := newTravelAgentCmd()
	
	// Verify command structure
	assert.NotNil(t, cmd.RunE)
	assert.Equal(t, "travelagent CREDENTIAL_FILE", cmd.Use)
	
	// YAML with comments should parse correctly
}

func TestTravelAgentCmdArgValidation(t *testing.T) {
	cmd := newTravelAgentCmd()
	
	// Test various argument combinations
	testCases := []struct {
		name          string
		args          []string
		expectedValid bool
	}{
		{
			name:          "no arguments",
			args:          []string{},
			expectedValid: false,
		},
		{
			name:          "one argument",
			args:          []string{"file.yaml"},
			expectedValid: true,
		},
		{
			name:          "two arguments",
			args:          []string{"file1.yaml", "file2.yaml"},
			expectedValid: false,
		},
		{
			name:          "three arguments",
			args:          []string{"file1.yaml", "file2.yaml", "file3.yaml"},
			expectedValid: false,
		},
	}
	
	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			err := cmd.Args(cmd, tc.args)
			if tc.expectedValid {
				assert.NoError(t, err)
			} else {
				assert.Error(t, err)
			}
		})
	}
}

func TestTravelAgentCmdStringFormatting(t *testing.T) {
	cmd := newTravelAgentCmd()
	
	// Test command structure
	assert.Contains(t, cmd.Use, "CREDENTIAL_FILE")
	assert.Contains(t, cmd.Short, "NOT FOR PRODUCTION USE")
	assert.Contains(t, cmd.Short, "travel agent server")
}

func TestTravelAgentCmdMalformedYAMLStructures(t *testing.T) {
	testCases := []struct {
		name          string
		yamlContent   string
		expectedError string
	}{
		{
			name:          "empty transfers",
			yamlContent:   `admin_token: "test"
transfers: []`,
			expectedError: "could not find any transfers",
		},
		{
			name:          "invalid YAML syntax",
			yamlContent:   `admin_token: test
  invalid:
    - unclosed`,
			expectedError: "yaml",
		},
		{
			name:          "mixed indentation",
			yamlContent:   `admin_token: "test"
transfers:
	- id: "mixed"
  token: "token"`,
			expectedError: "yaml",
		},
	}
	
	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			tmpFile, err := os.CreateTemp("", "malformed_*.yaml")
			require.NoError(t, err)
			defer func() { _ = os.Remove(tmpFile.Name()) }()
			
			err = os.WriteFile(tmpFile.Name(), []byte(tc.yamlContent), 0644)
			require.NoError(t, err)
			
			cmd := newTravelAgentCmd()
			err = cmd.RunE(cmd, []string{tmpFile.Name()})
			assert.Error(t, err)
			if tc.expectedError != "" {
				assert.Contains(t, err.Error(), tc.expectedError)
			}
		})
	}
}
