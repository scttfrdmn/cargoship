package shell

import (
	"os"
	"testing"

	"github.com/scttfrdmn/cargoship/pkg/plugins/transporters"
	"github.com/scttfrdmn/cargoship/pkg/rclone"
)

func TestTransporter_Configure(t *testing.T) {
	transporter := &Transporter{}
	
	// Configure should always return nil (no-op implementation)
	config := transporters.Config{
		Destination: "echo 'test'",
	}
	
	err := transporter.Configure(config)
	if err != nil {
		t.Errorf("Configure() should not return error, got: %v", err)
	}
}

func TestTransporter_Check(t *testing.T) {
	tests := []struct {
		name        string
		config      transporters.Config
		checkScript string
		expectError bool
		errorMsg    string
	}{
		{
			name: "Valid destination, no check script",
			config: transporters.Config{
				Destination: "echo 'upload complete'",
			},
			checkScript: "",
			expectError: false,
		},
		{
			name: "Empty destination",
			config: transporters.Config{
				Destination: "",
			},
			checkScript: "",
			expectError: true,
			errorMsg:    "must set a non empty destination",
		},
		{
			name: "Valid destination with successful check script",
			config: transporters.Config{
				Destination: "scp file.tar.gz user@server:/path/",
			},
			checkScript: "/bin/echo", // echo command should always succeed
			expectError: false,
		},
		{
			name: "Valid destination with failing check script",
			config: transporters.Config{
				Destination: "rsync file.tar.gz user@server:/path/",
			},
			checkScript: "false", // false command always fails
			expectError: true,
		},
		{
			name: "Valid destination with nonexistent check script",
			config: transporters.Config{
				Destination: "cp file.tar.gz /backup/",
			},
			checkScript: "/nonexistent/script",
			expectError: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			transporter := &Transporter{
				Config:      tt.config,
				checkScript: tt.checkScript,
			}

			err := transporter.Check()

			if tt.expectError {
				if err == nil {
					t.Errorf("Check() expected error but got nil")
				} else if tt.errorMsg != "" && err.Error() != tt.errorMsg {
					t.Errorf("Check() error = %v, want %v", err.Error(), tt.errorMsg)
				}
			} else {
				if err != nil {
					t.Errorf("Check() unexpected error: %v", err)
				}
			}
		})
	}
}

func TestTransporter_CheckScriptValidation(t *testing.T) {
	// Test that empty check script is handled correctly
	transporter := &Transporter{
		Config: transporters.Config{
			Destination: "valid-command",
		},
		checkScript: "",
	}

	err := transporter.Check()
	if err != nil {
		t.Errorf("Check() with empty checkScript should not error: %v", err)
	}

	// Test that non-empty check script is attempted
	transporter.checkScript = "/bin/echo"
	err = transporter.Check()
	if err != nil {
		t.Errorf("Check() with valid echo command should not error: %v", err)
	}
}

func TestTransporter_Send(t *testing.T) {
	// Test the Send method signature and basic functionality
	transporter := Transporter{
		Config: transporters.Config{
			Destination: "/bin/echo", // Use echo command which should succeed
		},
	}

	// Test Send method - it should not panic
	func() {
		defer func() {
			if r := recover(); r != nil {
				t.Errorf("Send method should not panic: %v", r)
			}
		}()
		
		// Create a temporary file to "send"
		tmpFile := "/tmp/test-cargoship-send"
		file, err := os.Create(tmpFile)
		if err != nil {
			t.Skipf("Could not create temp file for test: %v", err)
		}
		file.Close()
		defer os.Remove(tmpFile)

		err = transporter.Send(tmpFile, "upload/path")
		if err != nil {
			// Error is expected since echo command doesn't handle files properly
			// but the method should not panic
			t.Logf("Send() returned error (expected): %v", err)
		}
	}()
}

func TestTransporter_SendWithChannel(t *testing.T) {
	// Test the SendWithChannel method
	transporter := Transporter{
		Config: transporters.Config{
			Destination: "/bin/echo", // Use echo command which should succeed
		},
	}

	// Create test channel
	testChannel := make(chan rclone.TransferStatus, 1)

	// Test SendWithChannel method - it should not panic
	func() {
		defer func() {
			if r := recover(); r != nil {
				t.Errorf("SendWithChannel method should not panic: %v", r)
			}
		}()

		// Create a temporary file to "send"
		tmpFile := "/tmp/test-cargoship-sendwithchannel"
		file, err := os.Create(tmpFile)
		if err != nil {
			t.Skipf("Could not create temp file for test: %v", err)
		}
		file.Close()
		defer os.Remove(tmpFile)

		err = transporter.SendWithChannel(tmpFile, "upload/path", testChannel)
		if err != nil {
			// Error is expected since echo command doesn't handle files properly
			t.Logf("SendWithChannel() returned error (expected): %v", err)
		}

		// Verify the environment variable was set
		envValue := os.Getenv("SUITCASECTL_FILE")
		if envValue != tmpFile {
			t.Errorf("SUITCASECTL_FILE environment variable = %v, want %v", envValue, tmpFile)
		}
	}()
}

func TestTransporter_EnvironmentVariableHandling(t *testing.T) {
	transporter := Transporter{
		Config: transporters.Config{
			Destination: "/usr/bin/env", // env command to check environment variables
		},
	}

	// Save original environment
	originalEnv := os.Getenv("SUITCASECTL_FILE")
	defer func() {
		if originalEnv != "" {
			os.Setenv("SUITCASECTL_FILE", originalEnv)
		} else {
			os.Unsetenv("SUITCASECTL_FILE")
		}
	}()

	testFile := "/test/file/path.tar.gz"
	testChannel := make(chan rclone.TransferStatus, 1)

	err := transporter.SendWithChannel(testFile, "upload", testChannel)
	// Error is expected from env command, but environment should be set
	_ = err

	// Verify environment variable was set
	envValue := os.Getenv("SUITCASECTL_FILE")
	if envValue != testFile {
		t.Errorf("SUITCASECTL_FILE environment variable = %v, want %v", envValue, testFile)
	}
}

func TestTransporter_ImplementsInterface(t *testing.T) {
	// Verify that Transporter implements the transporters.Transporter interface
	var _ transporters.Transporter = (*Transporter)(nil)
	
	// Also test that we can create and use the transporter as the interface
	var tr transporters.Transporter = &Transporter{
		Config: transporters.Config{
			Destination: "echo 'test'",
		},
	}

	// Test interface methods work
	err := tr.Check()
	if err != nil {
		t.Errorf("Interface Check() failed: %v", err)
	}

	// Test Configure method directly on the concrete type
	concreteTransporter := tr.(*Transporter)
	err = concreteTransporter.Configure(transporters.Config{Destination: "test"})
	if err != nil {
		t.Errorf("Configure() failed: %v", err)
	}
}

func TestTransporter_ConfigurationHandling(t *testing.T) {
	tests := []struct {
		name   string
		config transporters.Config
		valid  bool
	}{
		{
			name: "Valid shell command",
			config: transporters.Config{
				Destination: "cp source.tar.gz /backup/",
			},
			valid: true,
		},
		{
			name: "Valid rsync command",
			config: transporters.Config{
				Destination: "rsync -av source.tar.gz user@host:/path/",
			},
			valid: true,
		},
		{
			name: "Valid scp command",
			config: transporters.Config{
				Destination: "scp source.tar.gz user@host:/remote/path/",
			},
			valid: true,
		},
		{
			name: "Complex shell pipeline",
			config: transporters.Config{
				Destination: "gzip -c source.tar | ssh user@host 'cat > /remote/file.tar.gz'",
			},
			valid: true,
		},
		{
			name: "Empty destination",
			config: transporters.Config{
				Destination: "",
			},
			valid: false,
		},
		{
			name: "Whitespace-only destination",
			config: transporters.Config{
				Destination: "   ",
			},
			valid: true, // Only checks for empty string, not whitespace
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			transporter := &Transporter{
				Config: tt.config,
			}

			err := transporter.Check()
			if tt.valid && err != nil && err.Error() == "must set a non empty destination" {
				t.Errorf("Check() should not error for valid config: %v", err)
			}
			if !tt.valid && err == nil {
				t.Error("Check() should error for invalid config")
			}
		})
	}
}

func TestTransporter_EdgeCases(t *testing.T) {
	// Test various edge cases
	transporter := &Transporter{}

	// Test with uninitialized config
	err := transporter.Check()
	if err == nil {
		t.Error("Check() should error with uninitialized config")
	}

	// Test with partially initialized config
	transporter.Config.Destination = "echo 'test'"
	err = transporter.Check()
	if err != nil {
		t.Errorf("Check() should not error with valid destination: %v", err)
	}
}

func TestTransporter_CheckScriptExecutionBehavior(t *testing.T) {
	tests := []struct {
		name        string
		checkScript string
		expectError bool
		description string
	}{
		{
			name:        "Empty check script",
			checkScript: "",
			expectError: false,
			description: "Should succeed when no check script is provided",
		},
		{
			name:        "True command",
			checkScript: "true",
			expectError: false,
			description: "Should succeed with true command",
		},
		{
			name:        "False command", 
			checkScript: "false",
			expectError: true,
			description: "Should fail with false command",
		},
		{
			name:        "Echo command",
			checkScript: "/bin/echo",
			expectError: false,
			description: "Should succeed with echo command",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			transporter := &Transporter{
				Config: transporters.Config{
					Destination: "valid-destination",
				},
				checkScript: tt.checkScript,
			}

			err := transporter.Check()
			
			if tt.expectError && err == nil {
				t.Errorf("Check() expected error for %s", tt.description)
			}
			if !tt.expectError && err != nil {
				t.Errorf("Check() unexpected error for %s: %v", tt.description, err)
			}
		})
	}
}

// Benchmark tests
func BenchmarkTransporter_Check(b *testing.B) {
	transporter := &Transporter{
		Config: transporters.Config{
			Destination: "echo 'test'",
		},
		checkScript: "",
	}

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		_ = transporter.Check()
	}
}

func BenchmarkTransporter_Configure(b *testing.B) {
	transporter := &Transporter{}
	config := transporters.Config{
		Destination: "echo 'test'",
	}

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		_ = transporter.Configure(config)
	}
}

func BenchmarkTransporter_CheckWithScript(b *testing.B) {
	transporter := &Transporter{
		Config: transporters.Config{
			Destination: "echo 'test'",
		},
		checkScript: "true",
	}

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		_ = transporter.Check()
	}
}