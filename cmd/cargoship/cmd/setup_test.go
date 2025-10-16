package cmd

import (
	"bufio"
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestNewSetupCmd(t *testing.T) {
	cmd := NewSetupCmd()

	assert.NotNil(t, cmd)
	assert.Equal(t, "setup", cmd.Use)
	assert.Contains(t, cmd.Short, "Interactive setup wizard")

	// Check flags are registered
	assert.NotNil(t, cmd.Flags().Lookup("non-interactive"))
	assert.NotNil(t, cmd.Flags().Lookup("output"))
}

func TestSetupCmdHelp(t *testing.T) {
	cmd := NewSetupCmd()

	help := cmd.Long

	// Verify help text contains key information
	assert.Contains(t, help, "Interactive setup wizard")
	assert.Contains(t, help, "AWS credentials")
	assert.Contains(t, help, "S3 bucket")
	assert.Contains(t, help, "upload parameters")
}

func TestReadLine(t *testing.T) {
	tests := []struct {
		name     string
		input    string
		expected string
	}{
		{
			name:     "simple input",
			input:    "test\n",
			expected: "test",
		},
		{
			name:     "input with spaces",
			input:    "  hello world  \n",
			expected: "hello world",
		},
		{
			name:     "empty input",
			input:    "\n",
			expected: "",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			reader := bufio.NewReader(strings.NewReader(tt.input))
			result := readLine(reader)
			assert.Equal(t, tt.expected, result)
		})
	}
}

func TestConfirmYesNo(t *testing.T) {
	tests := []struct {
		name     string
		input    string
		expected bool
	}{
		{
			name:     "yes lowercase",
			input:    "yes\n",
			expected: true,
		},
		{
			name:     "y lowercase",
			input:    "y\n",
			expected: true,
		},
		{
			name:     "YES uppercase",
			input:    "YES\n",
			expected: true,
		},
		{
			name:     "Y uppercase",
			input:    "Y\n",
			expected: true,
		},
		{
			name:     "empty default yes",
			input:    "\n",
			expected: true,
		},
		{
			name:     "no",
			input:    "no\n",
			expected: false,
		},
		{
			name:     "n",
			input:    "n\n",
			expected: false,
		},
		{
			name:     "invalid input",
			input:    "maybe\n",
			expected: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			reader := bufio.NewReader(strings.NewReader(tt.input))
			result := confirmYesNo(reader, "Test prompt?")
			assert.Equal(t, tt.expected, result)
		})
	}
}

func TestVerifyAWSCredentials(t *testing.T) {
	t.Skip("Skipping AWS integration test - requires real AWS credentials")

	// This test requires actual AWS credentials
	// In a real environment, you would test with valid credentials
	err := verifyAWSCredentials("us-west-2", "")

	// We expect this to either succeed (if credentials are available)
	// or fail with a specific error (if not)
	if err != nil {
		t.Logf("AWS verification failed as expected in test environment: %v", err)
	}
}

func TestVerifyBucketAccess(t *testing.T) {
	t.Skip("Skipping S3 integration test - requires real AWS credentials and bucket")

	// This test requires actual AWS credentials and a bucket
	// In a real environment, you would test with valid credentials and bucket
	err := verifyBucketAccess("us-west-2", "", "test-bucket")

	// We expect this to either succeed (if bucket exists and accessible)
	// or fail with a specific error (if not)
	if err != nil {
		t.Logf("Bucket verification failed as expected in test environment: %v", err)
	}
}

func TestSetupCommandFlags(t *testing.T) {
	cmd := NewSetupCmd()

	// Test non-interactive flag
	nonInteractiveFlag := cmd.Flags().Lookup("non-interactive")
	require.NotNil(t, nonInteractiveFlag)
	assert.Equal(t, "false", nonInteractiveFlag.DefValue)

	// Test output flag
	outputFlag := cmd.Flags().Lookup("output")
	require.NotNil(t, outputFlag)
	assert.Equal(t, "", outputFlag.DefValue)
}

func TestSetupCommandExamples(t *testing.T) {
	cmd := NewSetupCmd()

	// Verify examples are provided in the help text
	assert.Contains(t, cmd.Long, "Examples:")
	assert.Contains(t, cmd.Long, "cargoship setup")
}

// Benchmark tests
func BenchmarkReadLine(b *testing.B) {
	input := "test input\n"

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		reader := bufio.NewReader(strings.NewReader(input))
		_ = readLine(reader)
	}
}

func BenchmarkConfirmYesNo(b *testing.B) {
	input := "yes\n"

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		reader := bufio.NewReader(strings.NewReader(input))
		_ = confirmYesNo(reader, "Test?")
	}
}
