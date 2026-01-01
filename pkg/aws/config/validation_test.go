package config

import (
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// TestValidationError_Error tests the Error method
func TestValidationError_Error(t *testing.T) {
	tests := []struct {
		name     string
		err      ValidationError
		expected string
	}{
		{
			name: "with value",
			err: ValidationError{
				Field:   "Bucket",
				Value:   "test-bucket",
				Message: "invalid name",
			},
			expected: "validation error for 'Bucket' (value: test-bucket): invalid name",
		},
		{
			name: "without value",
			err: ValidationError{
				Field:   "Bucket",
				Value:   nil,
				Message: "bucket name cannot be empty",
			},
			expected: "validation error for 'Bucket': bucket name cannot be empty",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := tt.err.Error()
			assert.Equal(t, tt.expected, got)
		})
	}
}

// TestValidationErrors_Error tests the Error method for multiple errors
func TestValidationErrors_Error(t *testing.T) {
	tests := []struct {
		name     string
		errs     ValidationErrors
		contains []string
	}{
		{
			name: "no errors",
			errs: ValidationErrors{},
			contains: []string{"no validation errors"},
		},
		{
			name: "single error",
			errs: ValidationErrors{
				{Field: "Bucket", Value: "", Message: "cannot be empty"},
			},
			contains: []string{"1 validation error", "Bucket", "cannot be empty"},
		},
		{
			name: "multiple errors",
			errs: ValidationErrors{
				{Field: "Bucket", Value: "", Message: "cannot be empty"},
				{Field: "Concurrency", Value: 0, Message: "must be at least 1"},
			},
			contains: []string{"2 validation error", "Bucket", "Concurrency"},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := tt.errs.Error()
			for _, substr := range tt.contains {
				assert.Contains(t, got, substr)
			}
		})
	}
}

// TestS3Config_ValidateStrict tests comprehensive validation
func TestS3Config_ValidateStrict(t *testing.T) {
	tests := []struct {
		name      string
		config    S3Config
		wantErr   bool
		errFields []string
	}{
		{
			name: "valid config",
			config: S3Config{
				Bucket:              "my-test-bucket",
				StorageClass:        StorageClassStandard,
				MultipartThreshold:  10 * 1024 * 1024, // 10MB
				MultipartChunkSize:  10 * 1024 * 1024, // 10MB
				Concurrency:         4,
			},
			wantErr: false,
		},
		{
			name: "empty bucket",
			config: S3Config{
				StorageClass:        StorageClassStandard,
				MultipartThreshold:  10 * 1024 * 1024,
				MultipartChunkSize:  10 * 1024 * 1024,
				Concurrency:         4,
			},
			wantErr:   true,
			errFields: []string{"Bucket"},
		},
		{
			name: "invalid bucket name",
			config: S3Config{
				Bucket:              "Invalid_Bucket",
				StorageClass:        StorageClassStandard,
				MultipartThreshold:  10 * 1024 * 1024,
				MultipartChunkSize:  10 * 1024 * 1024,
				Concurrency:         4,
			},
			wantErr:   true,
			errFields: []string{"Bucket"},
		},
		{
			name: "invalid storage class",
			config: S3Config{
				Bucket:              "my-bucket",
				StorageClass:        "INVALID_CLASS",
				MultipartThreshold:  10 * 1024 * 1024,
				MultipartChunkSize:  10 * 1024 * 1024,
				Concurrency:         4,
			},
			wantErr:   true,
			errFields: []string{"StorageClass"},
		},
		{
			name: "multipart threshold too small",
			config: S3Config{
				Bucket:              "my-bucket",
				StorageClass:        StorageClassStandard,
				MultipartThreshold:  1024 * 1024, // 1MB - too small
				MultipartChunkSize:  10 * 1024 * 1024,
				Concurrency:         4,
			},
			wantErr:   true,
			errFields: []string{"MultipartThreshold"},
		},
		{
			name: "multipart threshold too large",
			config: S3Config{
				Bucket:              "my-bucket",
				StorageClass:        StorageClassStandard,
				MultipartThreshold:  6 * 1024 * 1024 * 1024, // 6GB - too large
				MultipartChunkSize:  10 * 1024 * 1024,
				Concurrency:         4,
			},
			wantErr:   true,
			errFields: []string{"MultipartThreshold"},
		},
		{
			name: "chunk size too small",
			config: S3Config{
				Bucket:              "my-bucket",
				StorageClass:        StorageClassStandard,
				MultipartThreshold:  10 * 1024 * 1024,
				MultipartChunkSize:  1024 * 1024, // 1MB - too small
				Concurrency:         4,
			},
			wantErr:   true,
			errFields: []string{"MultipartChunkSize"},
		},
		{
			name: "chunk size too large",
			config: S3Config{
				Bucket:              "my-bucket",
				StorageClass:        StorageClassStandard,
				MultipartThreshold:  10 * 1024 * 1024,
				MultipartChunkSize:  6 * 1024 * 1024 * 1024, // 6GB - too large
				Concurrency:         4,
			},
			wantErr:   true,
			errFields: []string{"MultipartChunkSize"},
		},
		{
			name: "concurrency too low",
			config: S3Config{
				Bucket:              "my-bucket",
				StorageClass:        StorageClassStandard,
				MultipartThreshold:  10 * 1024 * 1024,
				MultipartChunkSize:  10 * 1024 * 1024,
				Concurrency:         0,
			},
			wantErr:   true,
			errFields: []string{"Concurrency"},
		},
		{
			name: "concurrency too high",
			config: S3Config{
				Bucket:              "my-bucket",
				StorageClass:        StorageClassStandard,
				MultipartThreshold:  10 * 1024 * 1024,
				MultipartChunkSize:  10 * 1024 * 1024,
				Concurrency:         10001,
			},
			wantErr:   true,
			errFields: []string{"Concurrency"},
		},
		{
			name: "invalid KMS key ID",
			config: S3Config{
				Bucket:              "my-bucket",
				StorageClass:        StorageClassStandard,
				MultipartThreshold:  10 * 1024 * 1024,
				MultipartChunkSize:  10 * 1024 * 1024,
				Concurrency:         4,
				KMSKeyID:            "invalid-key",
			},
			wantErr:   true,
			errFields: []string{"KMSKeyID"},
		},
		{
			name: "multiple validation errors",
			config: S3Config{
				Bucket:              "",
				StorageClass:        "INVALID",
				MultipartThreshold:  1024,
				MultipartChunkSize:  1024,
				Concurrency:         0,
			},
			wantErr:   true,
			errFields: []string{"Bucket", "StorageClass", "MultipartThreshold", "MultipartChunkSize", "Concurrency"},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := tt.config.ValidateStrict()

			if tt.wantErr {
				require.Error(t, err)
				errMsg := err.Error()
				for _, field := range tt.errFields {
					assert.Contains(t, errMsg, field, "expected error to mention field: %s", field)
				}
			} else {
				require.NoError(t, err)
			}
		})
	}
}

// TestS3Config_Validate tests basic validation
func TestS3Config_Validate(t *testing.T) {
	tests := []struct {
		name    string
		config  S3Config
		wantErr bool
		errMsg  string
	}{
		{
			name: "valid config",
			config: S3Config{
				Bucket:              "my-bucket",
				MultipartThreshold:  10 * 1024 * 1024,
				MultipartChunkSize:  10 * 1024 * 1024,
				Concurrency:         4,
			},
			wantErr: false,
		},
		{
			name: "empty bucket",
			config: S3Config{
				MultipartThreshold: 10 * 1024 * 1024,
				MultipartChunkSize: 10 * 1024 * 1024,
				Concurrency:        4,
			},
			wantErr: true,
			errMsg:  "bucket name cannot be empty",
		},
		{
			name: "low concurrency",
			config: S3Config{
				Bucket:              "my-bucket",
				MultipartThreshold:  10 * 1024 * 1024,
				MultipartChunkSize:  10 * 1024 * 1024,
				Concurrency:         0,
			},
			wantErr: true,
			errMsg:  "concurrency must be at least 1",
		},
		{
			name: "low multipart threshold",
			config: S3Config{
				Bucket:              "my-bucket",
				MultipartThreshold:  1024 * 1024, // 1MB - too small
				MultipartChunkSize:  10 * 1024 * 1024,
				Concurrency:         4,
			},
			wantErr: true,
			errMsg:  "multipart threshold must be at least 5MB",
		},
		{
			name: "low chunk size",
			config: S3Config{
				Bucket:              "my-bucket",
				MultipartThreshold:  10 * 1024 * 1024,
				MultipartChunkSize:  1024 * 1024, // 1MB - too small
				Concurrency:         4,
			},
			wantErr: true,
			errMsg:  "multipart chunk size must be at least 5MB",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := tt.config.Validate()

			if tt.wantErr {
				require.Error(t, err)
				if tt.errMsg != "" {
					assert.Contains(t, err.Error(), tt.errMsg)
				}
			} else {
				require.NoError(t, err)
			}
		})
	}
}

// TestS3Config_ValidateWithDefaults tests validation with defaults
func TestS3Config_ValidateWithDefaults(t *testing.T) {
	tests := []struct {
		name              string
		config            S3Config
		expectedThreshold int64
		expectedChunkSize int64
		expectedConcur    int
		expectedStorage   StorageClass
		wantErr           bool
	}{
		{
			name: "apply all defaults",
			config: S3Config{
				Bucket: "my-bucket",
			},
			expectedThreshold: 100 * 1024 * 1024,
			expectedChunkSize: 10 * 1024 * 1024,
			expectedConcur:    4,
			expectedStorage:   StorageClassIntelligentTiering,
			wantErr:           false,
		},
		{
			name: "preserve existing values",
			config: S3Config{
				Bucket:              "my-bucket",
				MultipartThreshold:  50 * 1024 * 1024,
				MultipartChunkSize:  20 * 1024 * 1024,
				Concurrency:         8,
				StorageClass:        StorageClassStandard,
			},
			expectedThreshold: 50 * 1024 * 1024,
			expectedChunkSize: 20 * 1024 * 1024,
			expectedConcur:    8,
			expectedStorage:   StorageClassStandard,
			wantErr:           false,
		},
		{
			name: "partial defaults",
			config: S3Config{
				Bucket:             "my-bucket",
				MultipartThreshold: 25 * 1024 * 1024,
			},
			expectedThreshold: 25 * 1024 * 1024,
			expectedChunkSize: 10 * 1024 * 1024,
			expectedConcur:    4,
			expectedStorage:   StorageClassIntelligentTiering,
			wantErr:           false,
		},
		{
			name:    "empty bucket fails even with defaults",
			config:  S3Config{},
			wantErr: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := tt.config.ValidateWithDefaults()

			if tt.wantErr {
				require.Error(t, err)
			} else {
				require.NoError(t, err)
				assert.Equal(t, tt.expectedThreshold, tt.config.MultipartThreshold)
				assert.Equal(t, tt.expectedChunkSize, tt.config.MultipartChunkSize)
				assert.Equal(t, tt.expectedConcur, tt.config.Concurrency)
				assert.Equal(t, tt.expectedStorage, tt.config.StorageClass)
			}
		})
	}
}

// Test_validateBucketName tests bucket name validation
func Test_validateBucketName(t *testing.T) {
	tests := []struct {
		name    string
		bucket  string
		wantErr bool
		errMsg  string
	}{
		{"valid simple", "my-bucket", false, ""},
		{"valid with dots", "my.test.bucket", false, ""},
		{"valid with numbers", "my-bucket-123", false, ""},
		{"too short", "ab", true, "between 3 and 63 characters"},
		{"too long", strings.Repeat("a", 64), true, "between 3 and 63 characters"},
		{"uppercase", "My-Bucket", true, "lowercase letters"},
		{"starts with hyphen", "-my-bucket", true, "lowercase letters"},
		{"ends with hyphen", "my-bucket-", true, "lowercase letters"},
		{"consecutive dots", "my..bucket", true, "consecutive dots"},
		{"IP address format", "192.168.1.1", true, "IP address"},
		{"underscore", "my_bucket", true, "lowercase letters"},
		{"starts with dot", ".my-bucket", true, "lowercase letters"},
		{"ends with dot", "my-bucket.", true, "lowercase letters"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := validateBucketName(tt.bucket)

			if tt.wantErr {
				require.Error(t, err)
				if tt.errMsg != "" {
					assert.Contains(t, err.Error(), tt.errMsg)
				}
			} else {
				require.NoError(t, err)
			}
		})
	}
}

// Test_validateStorageClass tests storage class validation
func Test_validateStorageClass(t *testing.T) {
	tests := []struct {
		name    string
		class   StorageClass
		wantErr bool
	}{
		{"STANDARD", StorageClassStandard, false},
		{"STANDARD_IA", StorageClassStandardIA, false},
		{"ONEZONE_IA", StorageClassOneZoneIA, false},
		{"INTELLIGENT_TIERING", StorageClassIntelligentTiering, false},
		{"GLACIER", StorageClassGlacier, false},
		{"DEEP_ARCHIVE", StorageClassDeepArchive, false},
		{"empty (allowed)", "", false},
		{"invalid", "INVALID_CLASS", true},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := validateStorageClass(tt.class)

			if tt.wantErr {
				require.Error(t, err)
				assert.Contains(t, err.Error(), "invalid storage class")
			} else {
				require.NoError(t, err)
			}
		})
	}
}

// Test_validateKMSKeyID tests KMS key ID validation
func Test_validateKMSKeyID(t *testing.T) {
	tests := []struct {
		name    string
		keyID   string
		wantErr bool
		errMsg  string
	}{
		{
			name:    "valid UUID",
			keyID:   "12345678-1234-1234-1234-123456789abc",
			wantErr: false,
		},
		{
			name:    "valid ARN",
			keyID:   "arn:aws:kms:us-west-2:123456789012:key/12345678-1234-1234-1234-123456789abc",
			wantErr: false,
		},
		{
			name:    "invalid UUID - too short",
			keyID:   "12345678-1234-1234-1234",
			wantErr: true,
			errMsg:  "invalid KMS key ID format",
		},
		{
			name:    "invalid UUID - uppercase",
			keyID:   "12345678-1234-1234-1234-123456789ABC",
			wantErr: true,
			errMsg:  "invalid KMS key ID format",
		},
		{
			name:    "invalid ARN - missing region",
			keyID:   "arn:aws:kms::123456789012:key/12345678-1234-1234-1234-123456789abc",
			wantErr: true,
			errMsg:  "invalid KMS key ARN format",
		},
		{
			name:    "invalid ARN - wrong format",
			keyID:   "arn:aws:kms:us-west-2:account:wrongkey",
			wantErr: true,
			errMsg:  "invalid KMS key ARN format",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := validateKMSKeyID(tt.keyID)

			if tt.wantErr {
				require.Error(t, err)
				if tt.errMsg != "" {
					assert.Contains(t, err.Error(), tt.errMsg)
				}
			} else {
				require.NoError(t, err)
			}
		})
	}
}

// TestValidateConcurrency tests concurrency validation
func TestValidateConcurrency(t *testing.T) {
	tests := []struct {
		name        string
		concurrency int
		fileSize    int64
		wantErr     bool
		errMsg      string
	}{
		{
			name:        "valid for small file",
			concurrency: 2,
			fileSize:    50 * 1024 * 1024, // 50MB
			wantErr:     false,
		},
		{
			name:        "valid for large file",
			concurrency: 10,
			fileSize:    10 * 1024 * 1024 * 1024, // 10GB
			wantErr:     false,
		},
		{
			name:        "zero concurrency",
			concurrency: 0,
			fileSize:    100 * 1024 * 1024,
			wantErr:     true,
			errMsg:      "must be at least 1",
		},
		{
			name:        "too high for small file",
			concurrency: 10,
			fileSize:    50 * 1024 * 1024, // 50MB
			wantErr:     true,
			errMsg:      "too high for file size",
		},
		{
			name:        "excessive concurrency",
			concurrency: 150,
			fileSize:    10 * 1024 * 1024 * 1024,
			wantErr:     true,
			errMsg:      "excessive",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := ValidateConcurrency(tt.concurrency, tt.fileSize)

			if tt.wantErr {
				require.Error(t, err)
				if tt.errMsg != "" {
					assert.Contains(t, err.Error(), tt.errMsg)
				}
			} else {
				require.NoError(t, err)
			}
		})
	}
}

// TestValidateChunkSize tests chunk size validation
func TestValidateChunkSize(t *testing.T) {
	tests := []struct {
		name      string
		chunkSize int64
		fileSize  int64
		wantErr   bool
		errMsg    string
	}{
		{
			name:      "valid chunk size",
			chunkSize: 10 * 1024 * 1024,    // 10MB
			fileSize:  100 * 1024 * 1024,   // 100MB
			wantErr:   false,
		},
		{
			name:      "too small",
			chunkSize: 1 * 1024 * 1024,     // 1MB
			fileSize:  100 * 1024 * 1024,   // 100MB
			wantErr:   true,
			errMsg:    "at least 5MB",
		},
		{
			name:      "too large",
			chunkSize: 6 * 1024 * 1024 * 1024, // 6GB
			fileSize:  10 * 1024 * 1024 * 1024,
			wantErr:   true,
			errMsg:    "at most 5GB",
		},
		{
			name:      "too many parts",
			chunkSize: 5 * 1024 * 1024, // 5MB
			fileSize:  100 * 1024 * 1024 * 1024, // 100GB = 20,000 parts at 5MB each
			wantErr:   true,
			errMsg:    "will result in",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := ValidateChunkSize(tt.chunkSize, tt.fileSize)

			if tt.wantErr {
				require.Error(t, err)
				if tt.errMsg != "" {
					assert.Contains(t, err.Error(), tt.errMsg)
				}
			} else {
				require.NoError(t, err)
			}
		})
	}
}

// TestSuggestOptimalConfig tests optimal configuration suggestions
func TestSuggestOptimalConfig(t *testing.T) {
	tests := []struct {
		name               string
		fileSize           int64
		expectedConcur     int
		expectedChunkSize  int64
		expectedThreshold  int64
	}{
		{
			name:               "small file < 10MB",
			fileSize:           5 * 1024 * 1024,
			expectedConcur:     1,
			expectedChunkSize:  5 * 1024 * 1024,
			expectedThreshold:  100 * 1024 * 1024,
		},
		{
			name:               "medium file < 100MB",
			fileSize:           50 * 1024 * 1024,
			expectedConcur:     2,
			expectedChunkSize:  10 * 1024 * 1024,
			expectedThreshold:  10 * 1024 * 1024,
		},
		{
			name:               "large file < 1GB",
			fileSize:           500 * 1024 * 1024,
			expectedConcur:     4,
			expectedChunkSize:  10 * 1024 * 1024,
			expectedThreshold:  10 * 1024 * 1024,
		},
		{
			name:               "very large file < 10GB",
			fileSize:           5 * 1024 * 1024 * 1024,
			expectedConcur:     8,
			expectedChunkSize:  50 * 1024 * 1024,
			expectedThreshold:  10 * 1024 * 1024,
		},
		{
			name:               "huge file >= 10GB",
			fileSize:           20 * 1024 * 1024 * 1024,
			expectedConcur:     10,
			expectedChunkSize:  100 * 1024 * 1024,
			expectedThreshold:  10 * 1024 * 1024,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			config := SuggestOptimalConfig(tt.fileSize)

			assert.Equal(t, tt.expectedConcur, config.Concurrency)
			assert.Equal(t, tt.expectedChunkSize, config.MultipartChunkSize)
			assert.Equal(t, tt.expectedThreshold, config.MultipartThreshold)
			assert.Equal(t, StorageClassIntelligentTiering, config.StorageClass)
		})
	}
}
