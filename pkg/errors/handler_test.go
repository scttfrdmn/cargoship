package errors

import (
	"context"
	"errors"
	"fmt"
	"log/slog"
	"os"
	"strings"
	"testing"
	"time"

	"github.com/aws/aws-sdk-go-v2/service/s3/types"
	"github.com/aws/smithy-go"
)

func TestCargoShipError_Error(t *testing.T) {
	tests := []struct {
		name     string
		err      *CargoShipError
		expected string
	}{
		{
			name: "with resource",
			err: &CargoShipError{
				Type:      ErrorTypeNetwork,
				Operation: "upload",
				Resource:  "bucket/key",
				Message:   "timeout occurred",
			},
			expected: "network error in upload for bucket/key: timeout occurred",
		},
		{
			name: "without resource",
			err: &CargoShipError{
				Type:      ErrorTypePermission,
				Operation: "list",
				Message:   "access denied",
			},
			expected: "permission error in list: access denied",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := tt.err.Error(); got != tt.expected {
				t.Errorf("CargoShipError.Error() = %v, want %v", got, tt.expected)
			}
		})
	}
}

func TestCargoShipError_Unwrap(t *testing.T) {
	cause := errors.New("underlying error")
	err := &CargoShipError{
		Cause: cause,
	}

	if got := err.Unwrap(); got != cause {
		t.Errorf("CargoShipError.Unwrap() = %v, want %v", got, cause)
	}
}

func TestCargoShipError_IsRetryable(t *testing.T) {
	tests := []struct {
		name      string
		retryable bool
	}{
		{"retryable error", true},
		{"non-retryable error", false},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := &CargoShipError{Retryable: tt.retryable}
			if got := err.IsRetryable(); got != tt.retryable {
				t.Errorf("CargoShipError.IsRetryable() = %v, want %v", got, tt.retryable)
			}
		})
	}
}

func TestNewErrorHandler(t *testing.T) {
	logger := slog.New(slog.NewTextHandler(os.Stderr, nil))
	handler := NewErrorHandler(logger)

	if handler.maxRetries != 3 {
		t.Errorf("NewErrorHandler() maxRetries = %v, want 3", handler.maxRetries)
	}
	if handler.baseDelay != time.Second {
		t.Errorf("NewErrorHandler() baseDelay = %v, want 1s", handler.baseDelay)
	}
	if handler.maxDelay != 30*time.Second {
		t.Errorf("NewErrorHandler() maxDelay = %v, want 30s", handler.maxDelay)
	}
	if handler.backoffFactor != 2.0 {
		t.Errorf("NewErrorHandler() backoffFactor = %v, want 2.0", handler.backoffFactor)
	}
	if handler.logger != logger {
		t.Errorf("NewErrorHandler() logger mismatch")
	}
}

func TestErrorHandler_WithRetryConfig(t *testing.T) {
	logger := slog.New(slog.NewTextHandler(os.Stderr, nil))
	handler := NewErrorHandler(logger)

	maxRetries := 5
	baseDelay := 2 * time.Second
	maxDelay := 60 * time.Second

	result := handler.WithRetryConfig(maxRetries, baseDelay, maxDelay)

	if result.maxRetries != maxRetries {
		t.Errorf("WithRetryConfig() maxRetries = %v, want %v", result.maxRetries, maxRetries)
	}
	if result.baseDelay != baseDelay {
		t.Errorf("WithRetryConfig() baseDelay = %v, want %v", result.baseDelay, baseDelay)
	}
	if result.maxDelay != maxDelay {
		t.Errorf("WithRetryConfig() maxDelay = %v, want %v", result.maxDelay, maxDelay)
	}
	if result != handler {
		t.Errorf("WithRetryConfig() should return same instance")
	}
}

func TestErrorHandler_WrapError(t *testing.T) {
	logger := slog.New(slog.NewTextHandler(os.Stderr, nil))
	handler := NewErrorHandler(logger)

	tests := []struct {
		name      string
		err       error
		operation string
		resource  string
		expected  *CargoShipError
	}{
		{
			name:      "nil error",
			err:       nil,
			operation: "test",
			resource:  "resource",
			expected:  nil,
		},
		{
			name:      "already wrapped error",
			err:       &CargoShipError{Type: ErrorTypeNetwork, Message: "test"},
			operation: "test",
			resource:  "resource",
			expected:  nil, // WrapError returns same instance, so we'll check separately
		},
		{
			name:      "network error",
			err:       errors.New("connection timeout"),
			operation: "upload",
			resource:  "bucket/key",
			expected: &CargoShipError{
				Type:      ErrorTypeNetwork,
				Message:   "connection timeout",
				Operation: "upload",
				Resource:  "bucket/key",
				Retryable: true,
			},
		},
		{
			name:      "permission error",
			err:       errors.New("access denied"),
			operation: "list",
			resource:  "bucket",
			expected: &CargoShipError{
				Type:      ErrorTypePermission,
				Message:   "access denied",
				Operation: "list",
				Resource:  "bucket",
				Retryable: false,
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := handler.WrapError(tt.err, tt.operation, tt.resource)

			if tt.name == "nil error" {
				if got != nil {
					t.Errorf("WrapError() = %v, want nil", got)
				}
				return
			}

			if tt.name == "already wrapped error" {
				// For already wrapped errors, should return the same instance
				if got != tt.err {
					t.Errorf("WrapError() should return same instance for already wrapped error")
				}
				return
			}

			if got == nil {
				t.Errorf("WrapError() = nil, want %v", tt.expected)
				return
			}

			if got.Type != tt.expected.Type {
				t.Errorf("WrapError() Type = %v, want %v", got.Type, tt.expected.Type)
			}
			if got.Message != tt.expected.Message {
				t.Errorf("WrapError() Message = %v, want %v", got.Message, tt.expected.Message)
			}
			if got.Operation != tt.expected.Operation {
				t.Errorf("WrapError() Operation = %v, want %v", got.Operation, tt.expected.Operation)
			}
			if got.Resource != tt.expected.Resource {
				t.Errorf("WrapError() Resource = %v, want %v", got.Resource, tt.expected.Resource)
			}
			if got.Retryable != tt.expected.Retryable {
				t.Errorf("WrapError() Retryable = %v, want %v", got.Retryable, tt.expected.Retryable)
			}
			if got.Cause != tt.err {
				t.Errorf("WrapError() Cause = %v, want %v", got.Cause, tt.err)
			}
		})
	}
}

func TestErrorHandler_categorizeError(t *testing.T) {
	logger := slog.New(slog.NewTextHandler(os.Stderr, nil))
	handler := NewErrorHandler(logger)

	tests := []struct {
		name          string
		err           error
		expectedType  ErrorType
		expectedRetry bool
	}{
		{
			name:          "network timeout",
			err:           errors.New("connection timeout"),
			expectedType:  ErrorTypeNetwork,
			expectedRetry: true,
		},
		{
			name:          "network connection",
			err:           errors.New("connection refused"),
			expectedType:  ErrorTypeNetwork,
			expectedRetry: true,
		},
		{
			name:          "dns error",
			err:           errors.New("dns lookup failed"),
			expectedType:  ErrorTypeNetwork,
			expectedRetry: true,
		},
		{
			name:          "permission denied",
			err:           errors.New("access denied"),
			expectedType:  ErrorTypePermission,
			expectedRetry: false,
		},
		{
			name:          "forbidden",
			err:           errors.New("forbidden"),
			expectedType:  ErrorTypePermission,
			expectedRetry: false,
		},
		{
			name:          "throttling",
			err:           errors.New("throttle limit exceeded"),
			expectedType:  ErrorTypeThrottling,
			expectedRetry: true,
		},
		{
			name:          "rate limit",
			err:           errors.New("rate limit exceeded"),
			expectedType:  ErrorTypeThrottling,
			expectedRetry: true,
		},
		{
			name:          "validation error",
			err:           errors.New("invalid parameter"),
			expectedType:  ErrorTypeValidation,
			expectedRetry: false,
		},
		{
			name:          "bad request",
			err:           errors.New("bad request"),
			expectedType:  ErrorTypeValidation,
			expectedRetry: false,
		},
		{
			name:          "unknown error",
			err:           errors.New("something went wrong"),
			expectedType:  ErrorTypeUnknown,
			expectedRetry: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			gotType, gotRetry := handler.categorizeError(tt.err)
			if gotType != tt.expectedType {
				t.Errorf("categorizeError() type = %v, want %v", gotType, tt.expectedType)
			}
			if gotRetry != tt.expectedRetry {
				t.Errorf("categorizeError() retry = %v, want %v", gotRetry, tt.expectedRetry)
			}
		})
	}
}

func TestErrorHandler_categorizeError_S3Types(t *testing.T) {
	logger := slog.New(slog.NewTextHandler(os.Stderr, nil))
	handler := NewErrorHandler(logger)

	tests := []struct {
		name          string
		err           error
		expectedType  ErrorType
		expectedRetry bool
	}{
		{
			name:          "S3 NotFound",
			err:           &types.NotFound{},
			expectedType:  ErrorTypeValidation,
			expectedRetry: false,
		},
		{
			name:          "S3 NoSuchBucket",
			err:           &types.NoSuchBucket{},
			expectedType:  ErrorTypeValidation,
			expectedRetry: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			gotType, gotRetry := handler.categorizeError(tt.err)
			if gotType != tt.expectedType {
				t.Errorf("categorizeError() type = %v, want %v", gotType, tt.expectedType)
			}
			if gotRetry != tt.expectedRetry {
				t.Errorf("categorizeError() retry = %v, want %v", gotRetry, tt.expectedRetry)
			}
		})
	}
}

func TestErrorHandler_categorizeAWSError(t *testing.T) {
	logger := slog.New(slog.NewTextHandler(os.Stderr, nil))
	handler := NewErrorHandler(logger)

	tests := []struct {
		name          string
		code          string
		expectedType  ErrorType
		expectedRetry bool
	}{
		{
			name:          "throttling exception",
			code:          "ThrottlingException",
			expectedType:  ErrorTypeThrottling,
			expectedRetry: true,
		},
		{
			name:          "slow down",
			code:          "SlowDown",
			expectedType:  ErrorTypeThrottling,
			expectedRetry: true,
		},
		{
			name:          "access denied",
			code:          "AccessDenied",
			expectedType:  ErrorTypePermission,
			expectedRetry: false,
		},
		{
			name:          "forbidden",
			code:          "Forbidden",
			expectedType:  ErrorTypePermission,
			expectedRetry: false,
		},
		{
			name:          "validation exception",
			code:          "ValidationException",
			expectedType:  ErrorTypeValidation,
			expectedRetry: false,
		},
		{
			name:          "invalid parameter",
			code:          "InvalidParameterValue",
			expectedType:  ErrorTypeValidation,
			expectedRetry: false,
		},
		{
			name:          "request timeout",
			code:          "RequestTimeout",
			expectedType:  ErrorTypeNetwork,
			expectedRetry: true,
		},
		{
			name:          "service unavailable",
			code:          "ServiceUnavailable",
			expectedType:  ErrorTypeNetwork,
			expectedRetry: true,
		},
		{
			name:          "unknown code",
			code:          "UnknownError",
			expectedType:  ErrorTypeSystem,
			expectedRetry: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			awsErr := smithy.GenericAPIError{
				Code: tt.code,
			}
			gotType, gotRetry := handler.categorizeAWSError(awsErr)
			if gotType != tt.expectedType {
				t.Errorf("categorizeAWSError() type = %v, want %v", gotType, tt.expectedType)
			}
			if gotRetry != tt.expectedRetry {
				t.Errorf("categorizeAWSError() retry = %v, want %v", gotRetry, tt.expectedRetry)
			}
		})
	}
}

func TestErrorHandler_RetryWithBackoff(t *testing.T) {
	logger := slog.New(slog.NewTextHandler(os.Stderr, nil))
	handler := NewErrorHandler(logger).WithRetryConfig(2, 10*time.Millisecond, 100*time.Millisecond)

	t.Run("successful operation", func(t *testing.T) {
		callCount := 0
		err := handler.RetryWithBackoff(context.Background(), "test", func() error {
			callCount++
			return nil
		})
		if err != nil {
			t.Errorf("RetryWithBackoff() error = %v, want nil", err)
		}
		if callCount != 1 {
			t.Errorf("RetryWithBackoff() callCount = %v, want 1", callCount)
		}
	})

	t.Run("retryable error", func(t *testing.T) {
		callCount := 0
		err := handler.RetryWithBackoff(context.Background(), "test", func() error {
			callCount++
			if callCount < 3 {
				return errors.New("timeout")
			}
			return nil
		})
		if err != nil {
			t.Errorf("RetryWithBackoff() error = %v, want nil", err)
		}
		if callCount != 3 {
			t.Errorf("RetryWithBackoff() callCount = %v, want 3", callCount)
		}
	})

	t.Run("non-retryable error", func(t *testing.T) {
		callCount := 0
		err := handler.RetryWithBackoff(context.Background(), "test", func() error {
			callCount++
			return errors.New("access denied")
		})
		if err == nil {
			t.Errorf("RetryWithBackoff() error = nil, want error")
		}
		if callCount != 1 {
			t.Errorf("RetryWithBackoff() callCount = %v, want 1", callCount)
		}
	})
}

func TestErrorHandler_HandlePanic(t *testing.T) {
	logger := slog.New(slog.NewTextHandler(os.Stderr, nil))
	handler := NewErrorHandler(logger)

	t.Run("panic with error", func(t *testing.T) {
		defer handler.HandlePanic("test")()
		panic(errors.New("test panic"))
	})

	t.Run("panic with string", func(t *testing.T) {
		defer handler.HandlePanic("test")()
		panic("test panic string")
	})

	t.Run("panic with other type", func(t *testing.T) {
		defer handler.HandlePanic("test")()
		panic(42)
	})

	t.Run("no panic", func(t *testing.T) {
		defer handler.HandlePanic("test")()
		// Normal execution
	})
}

func TestErrorHandler_LogError(t *testing.T) {
	logger := slog.New(slog.NewTextHandler(os.Stderr, nil))
	handler := NewErrorHandler(logger)

	err := errors.New("test error")
	handler.LogError(err, "test-operation", "test-resource", map[string]interface{}{
		"key": "value",
	})

	// Test that the error is properly wrapped and logged
	// This is mainly for coverage since we can't easily test log output
}

func TestIsRetryableError(t *testing.T) {
	tests := []struct {
		name     string
		err      error
		expected bool
	}{
		{
			name:     "CargoShipError retryable",
			err:      &CargoShipError{Retryable: true},
			expected: true,
		},
		{
			name:     "CargoShipError non-retryable",
			err:      &CargoShipError{Retryable: false},
			expected: false,
		},
		{
			name:     "connection error",
			err:      errors.New("connection refused"),
			expected: true,
		},
		{
			name:     "timeout error",
			err:      errors.New("timeout occurred"),
			expected: true,
		},
		{
			name:     "throttle error",
			err:      errors.New("throttle limit"),
			expected: true,
		},
		{
			name:     "service unavailable error",
			err:      errors.New("service unavailable"),
			expected: true,
		},
		{
			name:     "other error",
			err:      errors.New("other error"),
			expected: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := IsRetryableError(tt.err); got != tt.expected {
				t.Errorf("IsRetryableError() = %v, want %v", got, tt.expected)
			}
		})
	}
}

func TestGetErrorType(t *testing.T) {
	tests := []struct {
		name     string
		err      error
		expected ErrorType
	}{
		{
			name:     "CargoShipError",
			err:      &CargoShipError{Type: ErrorTypeNetwork},
			expected: ErrorTypeNetwork,
		},
		{
			name:     "other error",
			err:      errors.New("other error"),
			expected: ErrorTypeUnknown,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := GetErrorType(tt.err); got != tt.expected {
				t.Errorf("GetErrorType() = %v, want %v", got, tt.expected)
			}
		})
	}
}

func TestErrorHandler_GetRecoveryOptions(t *testing.T) {
	logger := slog.New(slog.NewTextHandler(os.Stderr, nil))
	handler := NewErrorHandler(logger)

	tests := []struct {
		name        string
		err         error
		expectActions int
		expectSupport bool
	}{
		{
			name:          "permission error",
			err:           errors.New("access denied"),
			expectActions: 4,
			expectSupport: false,
		},
		{
			name:          "network error",
			err:           errors.New("connection timeout"),
			expectActions: 4,
			expectSupport: false,
		},
		{
			name:          "throttling error",
			err:           errors.New("throttle limit"),
			expectActions: 4,
			expectSupport: false,
		},
		{
			name:          "validation error",
			err:           errors.New("invalid parameter"),
			expectActions: 4,
			expectSupport: false,
		},
		{
			name:          "system error",
			err:           &CargoShipError{Type: ErrorTypeSystem},
			expectActions: 4,
			expectSupport: true,
		},
		{
			name:          "unknown error",
			err:           errors.New("mysterious error"),
			expectActions: 4,
			expectSupport: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			options := handler.GetRecoveryOptions(tt.err)
			
			if len(options.SuggestedActions) != tt.expectActions {
				t.Errorf("GetRecoveryOptions() actions count = %v, want %v", len(options.SuggestedActions), tt.expectActions)
			}
			if options.ContactSupport != tt.expectSupport {
				t.Errorf("GetRecoveryOptions() ContactSupport = %v, want %v", options.ContactSupport, tt.expectSupport)
			}
			if options.Documentation == "" {
				t.Errorf("GetRecoveryOptions() Documentation is empty")
			}
			if options.Context == nil {
				t.Errorf("GetRecoveryOptions() Context is nil")
			}
		})
	}
}

func TestNewErrorMetrics(t *testing.T) {
	metrics := NewErrorMetrics()
	
	if metrics.TotalErrors != 0 {
		t.Errorf("NewErrorMetrics() TotalErrors = %v, want 0", metrics.TotalErrors)
	}
	if metrics.ErrorsByType == nil {
		t.Errorf("NewErrorMetrics() ErrorsByType is nil")
	}
	if metrics.ErrorsByOp == nil {
		t.Errorf("NewErrorMetrics() ErrorsByOp is nil")
	}
	if metrics.RetryAttempts != 0 {
		t.Errorf("NewErrorMetrics() RetryAttempts = %v, want 0", metrics.RetryAttempts)
	}
}

func TestErrorMetrics_RecordError(t *testing.T) {
	metrics := NewErrorMetrics()
	
	err := &CargoShipError{
		Type:      ErrorTypeNetwork,
		Operation: "upload",
	}
	
	metrics.RecordError(err)
	
	if metrics.TotalErrors != 1 {
		t.Errorf("RecordError() TotalErrors = %v, want 1", metrics.TotalErrors)
	}
	if metrics.ErrorsByType[ErrorTypeNetwork] != 1 {
		t.Errorf("RecordError() ErrorsByType[Network] = %v, want 1", metrics.ErrorsByType[ErrorTypeNetwork])
	}
	if metrics.ErrorsByOp["upload"] != 1 {
		t.Errorf("RecordError() ErrorsByOp[upload] = %v, want 1", metrics.ErrorsByOp["upload"])
	}
	if metrics.LastError.IsZero() {
		t.Errorf("RecordError() LastError is zero")
	}
	
	// Record another error
	err2 := &CargoShipError{
		Type:      ErrorTypeNetwork,
		Operation: "download",
	}
	metrics.RecordError(err2)
	
	if metrics.TotalErrors != 2 {
		t.Errorf("RecordError() TotalErrors = %v, want 2", metrics.TotalErrors)
	}
	if metrics.ErrorsByType[ErrorTypeNetwork] != 2 {
		t.Errorf("RecordError() ErrorsByType[Network] = %v, want 2", metrics.ErrorsByType[ErrorTypeNetwork])
	}
	if metrics.ErrorsByOp["download"] != 1 {
		t.Errorf("RecordError() ErrorsByOp[download] = %v, want 1", metrics.ErrorsByOp["download"])
	}
}

func TestErrorHandler_categorizeError_WithSmithyError(t *testing.T) {
	logger := slog.New(slog.NewTextHandler(os.Stderr, nil))
	handler := NewErrorHandler(logger)

	// Create a mock smithy error
	smithyErr := &smithy.GenericAPIError{
		Code:    "ThrottlingException",
		Message: "Rate exceeded",
	}

	// Wrap it to test the error handling
	wrappedErr := fmt.Errorf("wrapped error: %w", smithyErr)

	errorType, retryable := handler.categorizeError(wrappedErr)

	if errorType != ErrorTypeThrottling {
		t.Errorf("categorizeError() with smithy error type = %v, want %v", errorType, ErrorTypeThrottling)
	}
	if !retryable {
		t.Errorf("categorizeError() with smithy error retryable = %v, want true", retryable)
	}
}

func TestErrorTypes(t *testing.T) {
	tests := []struct {
		name string
		et   ErrorType
		str  string
	}{
		{"network", ErrorTypeNetwork, "network"},
		{"permission", ErrorTypePermission, "permission"},
		{"throttling", ErrorTypeThrottling, "throttling"},
		{"validation", ErrorTypeValidation, "validation"},
		{"system", ErrorTypeSystem, "system"},
		{"unknown", ErrorTypeUnknown, "unknown"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if string(tt.et) != tt.str {
				t.Errorf("ErrorType string = %v, want %v", string(tt.et), tt.str)
			}
		})
	}
}

func TestCargoShipError_FieldValues(t *testing.T) {
	now := time.Now()
	context := map[string]interface{}{
		"key": "value",
	}
	
	err := &CargoShipError{
		Type:      ErrorTypeNetwork,
		Message:   "test message",
		Operation: "test operation",
		Resource:  "test resource",
		Cause:     errors.New("cause"),
		Retryable: true,
		Timestamp: now,
		Context:   context,
	}

	if err.Type != ErrorTypeNetwork {
		t.Errorf("CargoShipError.Type = %v, want %v", err.Type, ErrorTypeNetwork)
	}
	if err.Message != "test message" {
		t.Errorf("CargoShipError.Message = %v, want 'test message'", err.Message)
	}
	if err.Operation != "test operation" {
		t.Errorf("CargoShipError.Operation = %v, want 'test operation'", err.Operation)
	}
	if err.Resource != "test resource" {
		t.Errorf("CargoShipError.Resource = %v, want 'test resource'", err.Resource)
	}
	if err.Cause.Error() != "cause" {
		t.Errorf("CargoShipError.Cause = %v, want 'cause'", err.Cause)
	}
	if !err.Retryable {
		t.Errorf("CargoShipError.Retryable = %v, want true", err.Retryable)
	}
	if err.Timestamp != now {
		t.Errorf("CargoShipError.Timestamp = %v, want %v", err.Timestamp, now)
	}
	if err.Context["key"] != "value" {
		t.Errorf("CargoShipError.Context['key'] = %v, want 'value'", err.Context["key"])
	}
}

func TestErrorHandler_LogError_WithNilContext(t *testing.T) {
	logger := slog.New(slog.NewTextHandler(os.Stderr, nil))
	handler := NewErrorHandler(logger)

	err := &CargoShipError{
		Type:      ErrorTypeNetwork,
		Message:   "test error",
		Operation: "test",
		Resource:  "resource",
		Context:   nil, // Test nil context
	}

	// This should not panic and should initialize context
	handler.LogError(err, "test-operation", "test-resource", map[string]interface{}{
		"key": "value",
	})
}

func TestErrorHandler_WrapError_PreservesContext(t *testing.T) {
	logger := slog.New(slog.NewTextHandler(os.Stderr, nil))
	handler := NewErrorHandler(logger)

	originalErr := &CargoShipError{
		Type:      ErrorTypeNetwork,
		Message:   "original error",
		Operation: "original op",
		Resource:  "original resource",
		Retryable: true,
		Context:   map[string]interface{}{"original": "context"},
	}

	// WrapError should return the same error if already wrapped
	result := handler.WrapError(originalErr, "new op", "new resource")

	if result != originalErr {
		t.Errorf("WrapError() should return same instance for already wrapped error")
	}
	if result.Operation != "original op" {
		t.Errorf("WrapError() should preserve original operation")
	}
	if result.Context["original"] != "context" {
		t.Errorf("WrapError() should preserve original context")
	}
}

func TestRecoveryOptions_FieldValues(t *testing.T) {
	options := &RecoveryOptions{
		SuggestedActions: []string{"action1", "action2"},
		Documentation:    "https://example.com/docs",
		ContactSupport:   true,
		Context:          map[string]string{"key": "value"},
	}

	if len(options.SuggestedActions) != 2 {
		t.Errorf("RecoveryOptions.SuggestedActions length = %v, want 2", len(options.SuggestedActions))
	}
	if options.SuggestedActions[0] != "action1" {
		t.Errorf("RecoveryOptions.SuggestedActions[0] = %v, want 'action1'", options.SuggestedActions[0])
	}
	if options.Documentation != "https://example.com/docs" {
		t.Errorf("RecoveryOptions.Documentation = %v, want 'https://example.com/docs'", options.Documentation)
	}
	if !options.ContactSupport {
		t.Errorf("RecoveryOptions.ContactSupport = %v, want true", options.ContactSupport)
	}
	if options.Context["key"] != "value" {
		t.Errorf("RecoveryOptions.Context['key'] = %v, want 'value'", options.Context["key"])
	}
}

func TestErrorMetrics_FieldValues(t *testing.T) {
	now := time.Now()
	metrics := &ErrorMetrics{
		TotalErrors:  10,
		ErrorsByType: map[ErrorType]int64{ErrorTypeNetwork: 5, ErrorTypePermission: 3},
		ErrorsByOp:   map[string]int64{"upload": 7, "download": 3},
		RetryAttempts: 15,
		LastError:    now,
		ErrorRate:    0.25,
	}

	if metrics.TotalErrors != 10 {
		t.Errorf("ErrorMetrics.TotalErrors = %v, want 10", metrics.TotalErrors)
	}
	if metrics.ErrorsByType[ErrorTypeNetwork] != 5 {
		t.Errorf("ErrorMetrics.ErrorsByType[Network] = %v, want 5", metrics.ErrorsByType[ErrorTypeNetwork])
	}
	if metrics.ErrorsByOp["upload"] != 7 {
		t.Errorf("ErrorMetrics.ErrorsByOp['upload'] = %v, want 7", metrics.ErrorsByOp["upload"])
	}
	if metrics.RetryAttempts != 15 {
		t.Errorf("ErrorMetrics.RetryAttempts = %v, want 15", metrics.RetryAttempts)
	}
	if metrics.LastError != now {
		t.Errorf("ErrorMetrics.LastError = %v, want %v", metrics.LastError, now)
	}
	if metrics.ErrorRate != 0.25 {
		t.Errorf("ErrorMetrics.ErrorRate = %v, want 0.25", metrics.ErrorRate)
	}
}

// Test edge cases and error conditions
func TestErrorHandler_RetryWithBackoff_ContextCancellation(t *testing.T) {
	logger := slog.New(slog.NewTextHandler(os.Stderr, nil))
	handler := NewErrorHandler(logger)

	ctx, cancel := context.WithCancel(context.Background())
	cancel() // Cancel immediately

	err := handler.RetryWithBackoff(ctx, "test", func() error {
		return errors.New("should not be called")
	})

	if err == nil {
		t.Errorf("RetryWithBackoff() with cancelled context should return error")
	}
	if !strings.Contains(err.Error(), "context canceled") {
		t.Errorf("RetryWithBackoff() error should mention context cancellation, got: %v", err)
	}
}

func TestErrorHandler_categorizeError_CaseInsensitive(t *testing.T) {
	logger := slog.New(slog.NewTextHandler(os.Stderr, nil))
	handler := NewErrorHandler(logger)

	// Test that error categorization is case-insensitive
	tests := []struct {
		name string
		err  error
		want ErrorType
	}{
		{"uppercase CONNECTION", errors.New("CONNECTION failed"), ErrorTypeNetwork},
		{"mixed case Access Denied", errors.New("Access Denied"), ErrorTypePermission},
		{"lowercase throttle", errors.New("throttle limit"), ErrorTypeThrottling},
		{"uppercase INVALID", errors.New("INVALID request"), ErrorTypeValidation},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, _ := handler.categorizeError(tt.err)
			if got != tt.want {
				t.Errorf("categorizeError() = %v, want %v", got, tt.want)
			}
		})
	}
}