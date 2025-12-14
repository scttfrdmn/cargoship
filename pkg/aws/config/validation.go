package config

import (
	"fmt"
	"regexp"
	"strings"
)

// ValidationError represents a configuration validation error
type ValidationError struct {
	Field   string      // Field that failed validation
	Value   interface{} // Invalid value
	Message string      // Human-readable error message
}

func (e *ValidationError) Error() string {
	if e.Value != nil {
		return fmt.Sprintf("validation error for '%s' (value: %v): %s", e.Field, e.Value, e.Message)
	}
	return fmt.Sprintf("validation error for '%s': %s", e.Field, e.Message)
}

// ValidationErrors is a collection of validation errors
type ValidationErrors []ValidationError

func (errs ValidationErrors) Error() string {
	if len(errs) == 0 {
		return "no validation errors"
	}

	var msgs []string
	for _, err := range errs {
		msgs = append(msgs, err.Error())
	}
	return fmt.Sprintf("%d validation error(s):\n  - %s", len(errs), strings.Join(msgs, "\n  - "))
}

// ValidateStrict performs comprehensive validation with all checks
func (c *S3Config) ValidateStrict() error {
	var errs ValidationErrors

	// Bucket validation
	if c.Bucket == "" {
		errs = append(errs, ValidationError{
			Field:   "Bucket",
			Value:   c.Bucket,
			Message: "bucket name cannot be empty",
		})
	} else if err := validateBucketName(c.Bucket); err != nil {
		errs = append(errs, ValidationError{
			Field:   "Bucket",
			Value:   c.Bucket,
			Message: err.Error(),
		})
	}

	// Storage class validation
	if err := validateStorageClass(c.StorageClass); err != nil {
		errs = append(errs, ValidationError{
			Field:   "StorageClass",
			Value:   c.StorageClass,
			Message: err.Error(),
		})
	}

	// Multipart threshold validation
	if c.MultipartThreshold < 5*1024*1024 { // 5MB minimum (AWS requirement)
		errs = append(errs, ValidationError{
			Field:   "MultipartThreshold",
			Value:   c.MultipartThreshold,
			Message: fmt.Sprintf("must be at least 5MB (got %d bytes)", c.MultipartThreshold),
		})
	}
	if c.MultipartThreshold > 5*1024*1024*1024 { // 5GB max (AWS limit)
		errs = append(errs, ValidationError{
			Field:   "MultipartThreshold",
			Value:   c.MultipartThreshold,
			Message: fmt.Sprintf("must be at most 5GB (got %d bytes)", c.MultipartThreshold),
		})
	}

	// Chunk size validation
	if c.MultipartChunkSize < 5*1024*1024 { // 5MB minimum (AWS requirement)
		errs = append(errs, ValidationError{
			Field:   "MultipartChunkSize",
			Value:   c.MultipartChunkSize,
			Message: fmt.Sprintf("must be at least 5MB (got %d bytes)", c.MultipartChunkSize),
		})
	}
	if c.MultipartChunkSize > 5*1024*1024*1024 { // 5GB max (AWS limit)
		errs = append(errs, ValidationError{
			Field:   "MultipartChunkSize",
			Value:   c.MultipartChunkSize,
			Message: fmt.Sprintf("must be at most 5GB (got %d bytes)", c.MultipartChunkSize),
		})
	}

	// Concurrency validation
	if c.Concurrency < 1 {
		errs = append(errs, ValidationError{
			Field:   "Concurrency",
			Value:   c.Concurrency,
			Message: "must be at least 1",
		})
	}
	if c.Concurrency > 10000 { // Reasonable upper limit
		errs = append(errs, ValidationError{
			Field:   "Concurrency",
			Value:   c.Concurrency,
			Message: fmt.Sprintf("must be at most 10000 (got %d)", c.Concurrency),
		})
	}

	// KMS Key ID validation (if provided)
	if c.KMSKeyID != "" {
		if err := validateKMSKeyID(c.KMSKeyID); err != nil {
			errs = append(errs, ValidationError{
				Field:   "KMSKeyID",
				Value:   c.KMSKeyID,
				Message: err.Error(),
			})
		}
	}

	if len(errs) > 0 {
		return errs
	}
	return nil
}

// Validate performs basic validation (backward compatible)
func (c *S3Config) Validate() error {
	if c.Bucket == "" {
		return fmt.Errorf("bucket name cannot be empty")
	}

	if c.Concurrency < 1 {
		return fmt.Errorf("concurrency must be at least 1 (got %d)", c.Concurrency)
	}

	if c.MultipartThreshold < 5*1024*1024 {
		return fmt.Errorf("multipart threshold must be at least 5MB (got %d bytes)", c.MultipartThreshold)
	}

	if c.MultipartChunkSize < 5*1024*1024 {
		return fmt.Errorf("multipart chunk size must be at least 5MB (got %d bytes)", c.MultipartChunkSize)
	}

	return nil
}

// ValidateWithDefaults validates and applies default values for zero fields
func (c *S3Config) ValidateWithDefaults() error {
	// Apply defaults for zero values
	if c.MultipartThreshold == 0 {
		c.MultipartThreshold = 100 * 1024 * 1024 // 100MB default
	}

	if c.MultipartChunkSize == 0 {
		c.MultipartChunkSize = 10 * 1024 * 1024 // 10MB default
	}

	if c.Concurrency == 0 {
		c.Concurrency = 4 // 4 parallel uploads default
	}

	if c.StorageClass == "" {
		c.StorageClass = StorageClassIntelligentTiering
	}

	// Validate after applying defaults
	return c.Validate()
}

// validateBucketName validates S3 bucket naming rules
func validateBucketName(bucket string) error {
	// S3 bucket naming rules:
	// - 3-63 characters
	// - lowercase letters, numbers, hyphens, dots
	// - must start and end with letter or number
	// - cannot contain consecutive dots
	// - cannot be IP address format

	if len(bucket) < 3 || len(bucket) > 63 {
		return fmt.Errorf("bucket name must be between 3 and 63 characters (got %d)", len(bucket))
	}

	// Check for valid characters
	validBucket := regexp.MustCompile(`^[a-z0-9][a-z0-9.-]*[a-z0-9]$`)
	if !validBucket.MatchString(bucket) {
		return fmt.Errorf("bucket name must contain only lowercase letters, numbers, hyphens, and dots")
	}

	// Check for consecutive dots
	if strings.Contains(bucket, "..") {
		return fmt.Errorf("bucket name cannot contain consecutive dots")
	}

	// Check if it looks like an IP address
	if regexp.MustCompile(`^\d+\.\d+\.\d+\.\d+$`).MatchString(bucket) {
		return fmt.Errorf("bucket name cannot be formatted as an IP address")
	}

	return nil
}

// validateStorageClass validates the storage class value
func validateStorageClass(class StorageClass) error {
	validClasses := map[StorageClass]bool{
		StorageClassStandard:           true,
		StorageClassStandardIA:         true,
		StorageClassOneZoneIA:          true,
		StorageClassIntelligentTiering: true,
		StorageClassGlacier:            true,
		StorageClassDeepArchive:        true,
	}

	if class == "" {
		return nil // Empty is okay, will use default
	}

	if !validClasses[class] {
		return fmt.Errorf("invalid storage class '%s', must be one of: STANDARD, STANDARD_IA, ONEZONE_IA, INTELLIGENT_TIERING, GLACIER, DEEP_ARCHIVE", class)
	}

	return nil
}

// validateKMSKeyID validates KMS key ID format
func validateKMSKeyID(keyID string) error {
	// KMS Key ARN format: arn:aws:kms:region:account-id:key/key-id
	// Or just key ID: uuid format

	if strings.HasPrefix(keyID, "arn:") {
		// Full ARN
		arnPattern := regexp.MustCompile(`^arn:aws:kms:[a-z0-9-]+:\d{12}:key/[a-f0-9-]+$`)
		if !arnPattern.MatchString(keyID) {
			return fmt.Errorf("invalid KMS key ARN format")
		}
	} else {
		// UUID format
		uuidPattern := regexp.MustCompile(`^[a-f0-9]{8}-[a-f0-9]{4}-[a-f0-9]{4}-[a-f0-9]{4}-[a-f0-9]{12}$`)
		if !uuidPattern.MatchString(keyID) {
			return fmt.Errorf("invalid KMS key ID format (expected UUID or ARN)")
		}
	}

	return nil
}

// ValidateConcurrency checks if concurrency value is reasonable for the given file size
func ValidateConcurrency(concurrency int, fileSize int64) error {
	if concurrency < 1 {
		return fmt.Errorf("concurrency must be at least 1")
	}

	// For small files, high concurrency doesn't help
	if fileSize < 100*1024*1024 && concurrency > 4 {
		return fmt.Errorf("concurrency of %d is too high for file size %.2f MB (recommended: 1-4)",
			concurrency, float64(fileSize)/(1024*1024))
	}

	// For large files, reasonable upper limit
	if concurrency > 100 {
		return fmt.Errorf("concurrency of %d is excessive (recommended: 4-20)", concurrency)
	}

	return nil
}

// ValidateChunkSize checks if chunk size is appropriate for file size
func ValidateChunkSize(chunkSize, fileSize int64) error {
	if chunkSize < 5*1024*1024 {
		return fmt.Errorf("chunk size must be at least 5MB")
	}

	if chunkSize > 5*1024*1024*1024 {
		return fmt.Errorf("chunk size must be at most 5GB")
	}

	// Check if file will have too many parts
	// AWS S3 max parts: 10,000
	maxParts := fileSize / chunkSize
	if maxParts > 10000 {
		minChunkSize := fileSize / 10000
		return fmt.Errorf("chunk size %d bytes will result in %d parts (max 10000), minimum chunk size for this file: %d bytes",
			chunkSize, maxParts, minChunkSize)
	}

	return nil
}

// SuggestOptimalConfig suggests optimal configuration for a given file size
func SuggestOptimalConfig(fileSize int64) S3Config {
	config := S3Config{
		StorageClass: StorageClassIntelligentTiering,
	}

	switch {
	case fileSize < 10*1024*1024: // < 10MB
		config.MultipartThreshold = 100 * 1024 * 1024 // Don't use multipart
		config.MultipartChunkSize = 5 * 1024 * 1024
		config.Concurrency = 1

	case fileSize < 100*1024*1024: // < 100MB
		config.MultipartThreshold = 10 * 1024 * 1024
		config.MultipartChunkSize = 10 * 1024 * 1024
		config.Concurrency = 2

	case fileSize < 1024*1024*1024: // < 1GB
		config.MultipartThreshold = 10 * 1024 * 1024
		config.MultipartChunkSize = 10 * 1024 * 1024
		config.Concurrency = 4

	case fileSize < 10*1024*1024*1024: // < 10GB
		config.MultipartThreshold = 10 * 1024 * 1024
		config.MultipartChunkSize = 50 * 1024 * 1024
		config.Concurrency = 8

	default: // >= 10GB
		config.MultipartThreshold = 10 * 1024 * 1024
		config.MultipartChunkSize = 100 * 1024 * 1024
		config.Concurrency = 10
	}

	return config
}
