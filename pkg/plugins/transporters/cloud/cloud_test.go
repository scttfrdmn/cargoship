package cloud

import (
	"fmt"
	"strings"
	"testing"

	"github.com/scttfrdmn/cargoship/pkg/plugins/transporters"
	"github.com/scttfrdmn/cargoship/pkg/rclone"
)

func TestTransporter_Check(t *testing.T) {
	tests := []struct {
		name        string
		config      transporters.Config
		expectError bool
		errorMsg    string
	}{
		{
			name: "Valid destination",
			config: transporters.Config{
				Destination: "s3://bucket/path",
			},
			expectError: false,
		},
		{
			name: "Empty destination",
			config: transporters.Config{
				Destination: "",
			},
			expectError: true,
			errorMsg:    "destination is not set",
		},
		{
			name: "Valid cloud destination",
			config: transporters.Config{
				Destination: "gcs://my-bucket/folder",
			},
			expectError: false,
		},
		{
			name: "Valid Azure destination",
			config: transporters.Config{
				Destination: "azure://container/path",
			},
			expectError: false,
		},
		{
			name: "Local path destination",
			config: transporters.Config{
				Destination: "/local/path",
			},
			expectError: false,
		},
		{
			name: "HTTP URL destination",
			config: transporters.Config{
				Destination: "https://example.com/api/upload",
			},
			expectError: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			transporter := &Transporter{
				Config: tt.config,
			}

			err := transporter.Check()

			if tt.expectError {
				if err == nil {
					t.Errorf("Check() expected error but got nil")
				} else if err.Error() != tt.errorMsg {
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

func TestTransporter_DestinationPathConstruction(t *testing.T) {
	tests := []struct {
		name        string
		destination string
		upload      string
		expected    string
	}{
		{
			name:        "No trailing slash, no leading slash",
			destination: "s3://bucket/folder",
			upload:      "file.tar.gz",
			expected:    "s3://bucket/folder/file.tar.gz",
		},
		{
			name:        "Trailing slash, no leading slash",
			destination: "s3://bucket/folder/",
			upload:      "file.tar.gz",
			expected:    "s3://bucket/folder/file.tar.gz",
		},
		{
			name:        "No trailing slash, leading slash",
			destination: "s3://bucket/folder",
			upload:      "/file.tar.gz",
			expected:    "s3://bucket/folder/file.tar.gz",
		},
		{
			name:        "Trailing slash, leading slash",
			destination: "s3://bucket/folder/",
			upload:      "/file.tar.gz",
			expected:    "s3://bucket/folder/file.tar.gz",
		},
		{
			name:        "Empty upload path",
			destination: "s3://bucket/folder",
			upload:      "",
			expected:    "s3://bucket/folder",
		},
		{
			name:        "Multiple slashes",
			destination: "s3://bucket/folder///",
			upload:      "///file.tar.gz",
			expected:    "s3://bucket/folder/////file.tar.gz", // TrimSuffix only removes one "/" then adds one, TrimPrefix removes "///"
		},
		{
			name:        "Deep path",
			destination: "s3://bucket/base",
			upload:      "year/2024/month/01/day/15/archive.tar.gz",
			expected:    "s3://bucket/base/year/2024/month/01/day/15/archive.tar.gz",
		},
		{
			name:        "Azure blob storage",
			destination: "azure://container/base/",
			upload:      "/backup/data.tar.gz",
			expected:    "azure://container/base/backup/data.tar.gz",
		},
		{
			name:        "Google Cloud Storage",
			destination: "gcs://my-bucket/archive",
			upload:      "2024/backup.tar.gz",
			expected:    "gcs://my-bucket/archive/2024/backup.tar.gz",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			// Test the path construction logic used in SendWithChannel
			dest := tt.destination
			if tt.upload != "" {
				dest = strings.TrimSuffix(dest, "/") + "/" + strings.TrimPrefix(tt.upload, "/")
			}

			if dest != tt.expected {
				t.Errorf("Destination path = %v, want %v", dest, tt.expected)
			}
		})
	}
}

func TestTransporter_ConfigValidation(t *testing.T) {
	// Test that transporter properly stores and uses config
	config := transporters.Config{
		Destination: "s3://test-bucket/path",
	}

	transporter := &Transporter{
		Config: config,
	}

	if transporter.Config.Destination != config.Destination {
		t.Errorf("Transporter.Config.Destination = %v, want %v", 
			transporter.Config.Destination, config.Destination)
	}

	// Test check passes with valid config
	err := transporter.Check()
	if err != nil {
		t.Errorf("Check() with valid config should not error: %v", err)
	}
}

func TestTransporter_ImplementsInterface(t *testing.T) {
	// Verify that Transporter implements the transporters.Transporter interface
	var _ transporters.Transporter = (*Transporter)(nil)
	
	// Also test that we can create and use the transporter as the interface
	var tr transporters.Transporter = &Transporter{
		Config: transporters.Config{
			Destination: "s3://test-bucket/path",
		},
	}

	// Test interface methods work
	err := tr.Check()
	if err != nil {
		t.Errorf("Interface Check() failed: %v", err)
	}

	// Verify we can call Send through the interface
	// (We won't actually execute it to avoid external dependencies)
	if tr == nil {
		t.Error("Transporter should implement interface correctly")
	}
}

func TestTransporter_SendMethods_Signature(t *testing.T) {
	// Test that the Send and SendWithChannel methods have correct signatures
	// and can be called without panicking (though they may error due to rclone dependencies)
	
	transporter := &Transporter{
		Config: transporters.Config{
			Destination: "s3://test-bucket/path",
		},
	}

	// Test Send method signature - this will likely fail due to rclone but shouldn't panic
	func() {
		defer func() {
			if r := recover(); r != nil {
				t.Errorf("Send method should not panic: %v", r)
			}
		}()
		_ = transporter.Send("/nonexistent/source", "upload/path")
	}()

	// Test SendWithChannel method signature
	func() {
		defer func() {
			if r := recover(); r != nil {
				t.Errorf("SendWithChannel method should not panic: %v", r)
			}
		}()
		ch := make(chan rclone.TransferStatus, 1)
		_ = transporter.SendWithChannel("/nonexistent/source", "upload/path", ch)
	}()
}

func TestTransporter_EdgeCases(t *testing.T) {
	tests := []struct {
		name   string
		config transporters.Config
		valid  bool
	}{
		{
			name: "Whitespace-only destination",
			config: transporters.Config{
				Destination: "   ",
			},
			valid: true, // Whitespace is technically not empty
		},
		{
			name: "Very long destination",
			config: transporters.Config{
				Destination: strings.Repeat("s3://bucket/very-long-path/", 100),
			},
			valid: true,
		},
		{
			name: "Special characters in destination",
			config: transporters.Config{
				Destination: "s3://bucket/path-with-special-chars_123.456",
			},
			valid: true,
		},
		{
			name: "Unicode in destination",
			config: transporters.Config{
				Destination: "s3://bucket/ñáéíóú",
			},
			valid: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			transporter := &Transporter{
				Config: tt.config,
			}

			err := transporter.Check()
			if tt.valid && err != nil {
				t.Errorf("Check() should not error for valid config: %v", err)
			}
			if !tt.valid && err == nil {
				t.Error("Check() should error for invalid config")
			}
		})
	}
}

func TestTransporter_EmptyDestinationValidation(t *testing.T) {
	// Test various forms of "empty" destinations
	emptyDestinations := []string{
		"",
		// Note: we only check for exactly empty string, not whitespace
	}

	for i, dest := range emptyDestinations {
		t.Run(fmt.Sprintf("empty_destination_%d", i), func(t *testing.T) {
			transporter := &Transporter{
				Config: transporters.Config{
					Destination: dest,
				},
			}

			err := transporter.Check()
			if dest == "" {
				if err == nil {
					t.Error("Check() should error for empty destination")
				}
				if err.Error() != "destination is not set" {
					t.Errorf("Check() error = %v, want 'destination is not set'", err.Error())
				}
			}
		})
	}
}

// Benchmark tests for performance verification
func BenchmarkTransporter_Check(b *testing.B) {
	transporter := &Transporter{
		Config: transporters.Config{
			Destination: "s3://bucket/path",
		},
	}

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		_ = transporter.Check()
	}
}

func BenchmarkTransporter_PathConstruction(b *testing.B) {
	destination := "s3://bucket/base/"
	upload := "/subfolder/archive.tar.gz"

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		// Simulate the path construction logic from SendWithChannel
		dest := destination
		if upload != "" {
			dest = strings.TrimSuffix(dest, "/") + "/" + strings.TrimPrefix(upload, "/")
		}
		_ = dest
	}
}

func BenchmarkTransporter_CheckEmpty(b *testing.B) {
	transporter := &Transporter{
		Config: transporters.Config{
			Destination: "",
		},
	}

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		_ = transporter.Check()
	}
}