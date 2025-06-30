// Package errors provides production-grade error handling for CargoShip
package errors

import (
	"context"
	"errors"
	"fmt"
	"log/slog"
	"strings"
	"time"

	"github.com/aws/aws-sdk-go-v2/service/s3/types"
	"github.com/aws/smithy-go"
	"github.com/sethvargo/go-retry"
)

// ErrorType categorizes different types of errors
type ErrorType string

const (
	ErrorTypeNetwork     ErrorType = "network"
	ErrorTypePermission  ErrorType = "permission"
	ErrorTypeThrottling  ErrorType = "throttling"
	ErrorTypeValidation  ErrorType = "validation"
	ErrorTypeSystem      ErrorType = "system"
	ErrorTypeUnknown     ErrorType = "unknown"
)

// CargoShipError represents a structured error with context
type CargoShipError struct {
	Type        ErrorType `json:"type"`
	Message     string    `json:"message"`
	Operation   string    `json:"operation"`
	Resource    string    `json:"resource,omitempty"`
	Cause       error     `json:"cause,omitempty"`
	Retryable   bool      `json:"retryable"`
	Timestamp   time.Time `json:"timestamp"`
	Context     map[string]interface{} `json:"context,omitempty"`
}

// Error implements the error interface
func (e *CargoShipError) Error() string {
	if e.Resource != "" {
		return fmt.Sprintf("%s error in %s for %s: %s", e.Type, e.Operation, e.Resource, e.Message)
	}
	return fmt.Sprintf("%s error in %s: %s", e.Type, e.Operation, e.Message)
}

// Unwrap returns the underlying error
func (e *CargoShipError) Unwrap() error {
	return e.Cause
}

// IsRetryable returns whether the error is retryable
func (e *CargoShipError) IsRetryable() bool {
	return e.Retryable
}

// ErrorHandler provides comprehensive error handling and retry logic
type ErrorHandler struct {
	maxRetries    int
	baseDelay     time.Duration
	maxDelay      time.Duration
	backoffFactor float64
	logger        *slog.Logger
}

// NewErrorHandler creates a new error handler
func NewErrorHandler(logger *slog.Logger) *ErrorHandler {
	return &ErrorHandler{
		maxRetries:    3,
		baseDelay:     1 * time.Second,
		maxDelay:      30 * time.Second,
		backoffFactor: 2.0,
		logger:        logger,
	}
}

// WithRetryConfig configures retry behavior
func (h *ErrorHandler) WithRetryConfig(maxRetries int, baseDelay, maxDelay time.Duration) *ErrorHandler {
	h.maxRetries = maxRetries
	h.baseDelay = baseDelay
	h.maxDelay = maxDelay
	return h
}

// WrapError creates a CargoShipError from a generic error
func (h *ErrorHandler) WrapError(err error, operation, resource string) *CargoShipError {
	if err == nil {
		return nil
	}

	// Check if already wrapped
	if csErr, ok := err.(*CargoShipError); ok {
		return csErr
	}

	errorType, retryable := h.categorizeError(err)
	
	return &CargoShipError{
		Type:      errorType,
		Message:   err.Error(),
		Operation: operation,
		Resource:  resource,
		Cause:     err,
		Retryable: retryable,
		Timestamp: time.Now(),
		Context:   make(map[string]interface{}),
	}
}

// categorizeError determines the error type and retry behavior
func (h *ErrorHandler) categorizeError(err error) (ErrorType, bool) {
	errStr := strings.ToLower(err.Error())
	
	// AWS-specific errors using smithy-go
	var apiErr *smithy.GenericAPIError
	if errors.As(err, &apiErr) {
		return h.categorizeAWSError(*apiErr)
	}
	
	// S3-specific errors
	var notFound *types.NotFound
	if errors.As(err, &notFound) {
		return ErrorTypeValidation, false
	}
	
	var noSuchBucket *types.NoSuchBucket
	if errors.As(err, &noSuchBucket) {
		return ErrorTypeValidation, false
	}
	
	// Note: types.AccessDenied was removed in newer AWS SDK versions
	// Handle access denied via string matching instead
	
	// Network errors
	if strings.Contains(errStr, "connection") ||
		strings.Contains(errStr, "timeout") ||
		strings.Contains(errStr, "network") ||
		strings.Contains(errStr, "dns") {
		return ErrorTypeNetwork, true
	}
	
	// Permission errors
	if strings.Contains(errStr, "access denied") ||
		strings.Contains(errStr, "forbidden") ||
		strings.Contains(errStr, "unauthorized") ||
		strings.Contains(errStr, "permission") {
		return ErrorTypePermission, false
	}
	
	// Throttling errors
	if strings.Contains(errStr, "throttle") ||
		strings.Contains(errStr, "rate limit") ||
		strings.Contains(errStr, "too many requests") ||
		strings.Contains(errStr, "slow down") {
		return ErrorTypeThrottling, true
	}
	
	// Validation errors
	if strings.Contains(errStr, "invalid") ||
		strings.Contains(errStr, "bad request") ||
		strings.Contains(errStr, "malformed") {
		return ErrorTypeValidation, false
	}
	
	return ErrorTypeUnknown, true
}

// categorizeAWSError categorizes AWS-specific errors
func (h *ErrorHandler) categorizeAWSError(awsErr smithy.GenericAPIError) (ErrorType, bool) {
	code := awsErr.Code
	
	switch code {
	case "ThrottlingException", "Throttling", "ProvisionedThroughputExceededException", "SlowDown":
		return ErrorTypeThrottling, true
	case "AccessDenied", "UnauthorizedOperation", "Forbidden":
		return ErrorTypePermission, false
	case "InvalidRequest", "ValidationException", "InvalidParameterValue":
		return ErrorTypeValidation, false
	case "RequestTimeout", "ServiceUnavailable", "InternalError":
		return ErrorTypeNetwork, true
	default:
		// Note: smithy.GenericAPIError doesn't expose HTTPStatusCode
		// We'll categorize based on the error code instead
		return ErrorTypeSystem, true
	}
}

// RetryWithBackoff executes an operation with retry logic
func (h *ErrorHandler) RetryWithBackoff(ctx context.Context, operation string, fn func() error) error {
	backoff := retry.NewExponential(h.baseDelay)
	backoff = retry.WithMaxRetries(uint64(h.maxRetries), backoff)
	backoff = retry.WithCappedDuration(h.maxDelay, backoff)
	
	return retry.Do(ctx, backoff, func(ctx context.Context) error {
		err := fn()
		if err == nil {
			return nil
		}
		
		wrappedErr := h.WrapError(err, operation, "")
		
		h.logger.Error("operation failed",
			"operation", operation,
			"error_type", wrappedErr.Type,
			"retryable", wrappedErr.Retryable,
			"error", wrappedErr.Message)
		
		if !wrappedErr.IsRetryable() {
			return wrappedErr
		}
		
		return retry.RetryableError(wrappedErr)
	})
}

// HandlePanic recovers from panics and converts them to errors
func (h *ErrorHandler) HandlePanic(operation string) func() {
	return func() {
		if r := recover(); r != nil {
			var err error
			switch v := r.(type) {
			case error:
				err = v
			case string:
				err = fmt.Errorf("panic: %s", v)
			default:
				err = fmt.Errorf("panic: %v", v)
			}
			
			wrappedErr := h.WrapError(err, operation, "")
			h.logger.Error("panic recovered",
				"operation", operation,
				"error", wrappedErr.Message,
				"stack_trace", fmt.Sprintf("%+v", r))
		}
	}
}

// LogError logs an error with appropriate level and context
func (h *ErrorHandler) LogError(err error, operation, resource string, errorContext map[string]interface{}) {
	wrappedErr := h.WrapError(err, operation, resource)
	
	// Add additional context
	if wrappedErr.Context == nil {
		wrappedErr.Context = make(map[string]interface{})
	}
	for k, v := range errorContext {
		wrappedErr.Context[k] = v
	}
	
	// Log with appropriate level based on error type
	level := slog.LevelError
	switch wrappedErr.Type {
	case ErrorTypeThrottling, ErrorTypeNetwork:
		level = slog.LevelWarn
	case ErrorTypeValidation:
		level = slog.LevelInfo
	}
	
	h.logger.Log(context.Background(), level, "operation error",
		"operation", wrappedErr.Operation,
		"resource", wrappedErr.Resource,
		"error_type", wrappedErr.Type,
		"retryable", wrappedErr.Retryable,
		"message", wrappedErr.Message,
		"error_context", wrappedErr.Context)
}

// IsRetryableError checks if an error is retryable
func IsRetryableError(err error) bool {
	if csErr, ok := err.(*CargoShipError); ok {
		return csErr.IsRetryable()
	}
	
	// Default categorization for non-wrapped errors
	errStr := strings.ToLower(err.Error())
	return strings.Contains(errStr, "connection") ||
		strings.Contains(errStr, "timeout") ||
		strings.Contains(errStr, "throttle") ||
		strings.Contains(errStr, "service unavailable")
}

// GetErrorType returns the error type if it's a CargoShipError
func GetErrorType(err error) ErrorType {
	if csErr, ok := err.(*CargoShipError); ok {
		return csErr.Type
	}
	return ErrorTypeUnknown
}

// RecoveryOptions provides suggestions for error recovery
type RecoveryOptions struct {
	SuggestedActions []string          `json:"suggested_actions"`
	Documentation    string            `json:"documentation"`
	ContactSupport   bool              `json:"contact_support"`
	Context          map[string]string `json:"context"`
}

// GetRecoveryOptions provides actionable suggestions for error recovery
func (h *ErrorHandler) GetRecoveryOptions(err error) *RecoveryOptions {
	wrappedErr := h.WrapError(err, "", "")
	
	options := &RecoveryOptions{
		Context: make(map[string]string),
	}
	
	switch wrappedErr.Type {
	case ErrorTypePermission:
		options.SuggestedActions = []string{
			"Check AWS credentials are configured correctly",
			"Verify IAM permissions for the required S3 operations",
			"Ensure the AWS profile has access to the target bucket",
			"Check if MFA or additional authentication is required",
		}
		options.Documentation = "https://docs.aws.amazon.com/IAM/latest/UserGuide/reference_policies_examples_s3.html"
		
	case ErrorTypeNetwork:
		options.SuggestedActions = []string{
			"Check internet connectivity",
			"Verify AWS service endpoints are accessible",
			"Try reducing concurrency or chunk size",
			"Check for firewall or proxy configuration issues",
		}
		options.Documentation = "https://docs.aws.amazon.com/general/latest/gr/s3.html"
		
	case ErrorTypeThrottling:
		options.SuggestedActions = []string{
			"Reduce upload concurrency",
			"Implement exponential backoff (automatically handled)",
			"Consider using smaller chunk sizes",
			"Monitor AWS service health dashboard",
		}
		options.Documentation = "https://docs.aws.amazon.com/AmazonS3/latest/userguide/optimizing-performance.html"
		
	case ErrorTypeValidation:
		options.SuggestedActions = []string{
			"Check bucket name and region configuration",
			"Verify file paths and object keys are valid",
			"Ensure storage class is appropriate for the operation",
			"Check lifecycle policy configuration syntax",
		}
		options.Documentation = "https://docs.aws.amazon.com/AmazonS3/latest/userguide/"
		
	case ErrorTypeSystem:
		options.SuggestedActions = []string{
			"Check AWS service health dashboard",
			"Retry the operation after a brief delay",
			"Consider using a different AWS region temporarily",
			"Monitor AWS service announcements",
		}
		options.ContactSupport = true
		options.Documentation = "https://status.aws.amazon.com/"
		
	default:
		options.SuggestedActions = []string{
			"Check CargoShip logs for detailed error information",
			"Verify AWS configuration and credentials",
			"Try the operation with verbose logging enabled",
			"Check GitHub issues for similar problems",
		}
		options.ContactSupport = true
		options.Documentation = "https://github.com/scttfrdmn/cargoship/issues"
	}
	
	return options
}


// ErrorMetrics tracks error statistics for monitoring
type ErrorMetrics struct {
	TotalErrors     int64                    `json:"total_errors"`
	ErrorsByType    map[ErrorType]int64      `json:"errors_by_type"`
	ErrorsByOp      map[string]int64         `json:"errors_by_operation"`
	RetryAttempts   int64                    `json:"retry_attempts"`
	LastError       time.Time                `json:"last_error"`
	ErrorRate       float64                  `json:"error_rate"` // errors per operation
}

// NewErrorMetrics creates a new error metrics tracker
func NewErrorMetrics() *ErrorMetrics {
	return &ErrorMetrics{
		ErrorsByType: make(map[ErrorType]int64),
		ErrorsByOp:   make(map[string]int64),
	}
}

// RecordError records an error in the metrics
func (m *ErrorMetrics) RecordError(err *CargoShipError) {
	m.TotalErrors++
	m.ErrorsByType[err.Type]++
	m.ErrorsByOp[err.Operation]++
	m.LastError = time.Now()
}