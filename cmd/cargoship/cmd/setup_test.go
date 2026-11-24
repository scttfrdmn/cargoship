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
		{
			name:     "last line without newline",
			input:    "us-east-2",
			expected: "us-east-2",
		},
		{
			name:     "end of input",
			input:    "",
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

func TestConfirmDefault(t *testing.T) {
	tests := []struct {
		name  string
		input string
		def   bool
		want  bool
	}{
		{"yes", "yes\n", false, true},
		{"y", "y\n", false, true},
		{"YES", "YES\n", false, true},
		{"empty takes default yes", "\n", true, true},
		{"empty takes default no", "\n", false, false},
		{"EOF takes the default", "", false, false},
		{"no", "no\n", true, false},
		{"n", "n\n", true, false},
		{"invalid input is no", "maybe\n", true, false},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := confirmDefault(bufio.NewReader(strings.NewReader(tt.input)), "Test prompt?", tt.def)
			assert.Equal(t, tt.want, got)
		})
	}
}

// TestConfirmTyped pins #909: only an explicit "yes" confirms. An empty answer
// or end of input used to count as yes.
func TestConfirmTyped(t *testing.T) {
	for in, want := range map[string]bool{"yes\n": true, "YES\n": true, "y\n": false, "\n": false, "": false, "no\n": false} {
		assert.Equal(t, want, confirmTyped(bufio.NewReader(strings.NewReader(in)), "Type 'yes':"), "input %q", in)
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

func BenchmarkConfirmDefault(b *testing.B) {
	input := "yes\n"

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		reader := bufio.NewReader(strings.NewReader(input))
		_ = confirmDefault(reader, "Test?", true)
	}
}
