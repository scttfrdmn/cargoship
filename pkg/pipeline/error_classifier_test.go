package pipeline

import (
	"errors"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// TestErrorType_String tests the String method for all ErrorType values
func TestErrorType_String(t *testing.T) {
	tests := []struct {
		name     string
		errType  ErrorType
		expected string
	}{
		{"Unknown", ErrorTypeUnknown, "Unknown"},
		{"Permission", ErrorTypePermission, "Permission"},
		{"Network", ErrorTypeNetwork, "Network"},
		{"Storage", ErrorTypeStorage, "Storage"},
		{"Validation", ErrorTypeValidation, "Validation"},
		{"Quota", ErrorTypeQuota, "Quota"},
		{"Timeout", ErrorTypeTimeout, "Timeout"},
		{"Invalid value", ErrorType(999), "Unknown"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := tt.errType.String()
			assert.Equal(t, tt.expected, got)
		})
	}
}

// TestNewErrorClassifier tests the constructor
func TestNewErrorClassifier(t *testing.T) {
	ec := NewErrorClassifier()
	require.NotNil(t, ec)
}

// TestErrorClassifier_Classify tests the main classification logic
func TestErrorClassifier_Classify(t *testing.T) {
	ec := NewErrorClassifier()

	tests := []struct {
		name         string
		err          error
		expectedType ErrorType
		checkMessage bool
	}{
		{
			name:         "nil error",
			err:          nil,
			expectedType: ErrorTypeUnknown,
		},
		{
			name:         "permission - access denied",
			err:          errors.New("AccessDenied: Access Denied"),
			expectedType: ErrorTypePermission,
			checkMessage: true,
		},
		{
			name:         "permission - forbidden",
			err:          errors.New("403 Forbidden"),
			expectedType: ErrorTypePermission,
			checkMessage: true,
		},
		{
			name:         "permission - unauthorized",
			err:          errors.New("Unauthorized request"),
			expectedType: ErrorTypePermission,
			checkMessage: true,
		},
		{
			name:         "network - connection refused",
			err:          errors.New("connection refused"),
			expectedType: ErrorTypeNetwork,
			checkMessage: true,
		},
		{
			name:         "network - timeout",
			err:          errors.New("i/o timeout"),
			expectedType: ErrorTypeNetwork,
			checkMessage: true,
		},
		{
			name:         "network - EOF",
			err:          errors.New("unexpected EOF"),
			expectedType: ErrorTypeNetwork,
			checkMessage: true,
		},
		{
			name:         "storage - no such bucket",
			err:          errors.New("NoSuchBucket: The specified bucket does not exist"),
			expectedType: ErrorTypeStorage,
			checkMessage: true,
		},
		{
			name:         "storage - slow down",
			err:          errors.New("Slow Down: Please reduce your request rate"),
			expectedType: ErrorTypeStorage,
			checkMessage: true,
		},
		{
			name:         "storage - 503",
			err:          errors.New("503 Service Unavailable"),
			expectedType: ErrorTypeStorage,
			checkMessage: true,
		},
		{
			name:         "validation - invalid parameter",
			err:          errors.New("Invalid parameter: bucket name"),
			expectedType: ErrorTypeValidation,
			checkMessage: true,
		},
		{
			name:         "validation - malformed",
			err:          errors.New("Malformed request"),
			expectedType: ErrorTypeValidation,
			checkMessage: true,
		},
		{
			name:         "validation - 400",
			err:          errors.New("400 Bad Request"),
			expectedType: ErrorTypeValidation,
			checkMessage: true,
		},
		{
			name:         "quota - limit exceeded",
			err:          errors.New("Limit exceeded: too many requests"),
			expectedType: ErrorTypeQuota,
			checkMessage: true,
		},
		{
			name:         "quota - throttled",
			err:          errors.New("Request throttled"),
			expectedType: ErrorTypeQuota,
			checkMessage: true,
		},
		{
			name:         "timeout - deadline exceeded",
			err:          errors.New("context deadline exceeded"),
			expectedType: ErrorTypeTimeout,
			checkMessage: true,
		},
		{
			name:         "timeout - timed out",
			err:          errors.New("operation timed out"),
			expectedType: ErrorTypeTimeout,
			checkMessage: true,
		},
		{
			name:         "unknown error",
			err:          errors.New("some random error"),
			expectedType: ErrorTypeUnknown,
			checkMessage: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := ec.Classify(tt.err)

			if tt.err == nil {
				assert.Nil(t, result)
				return
			}

			require.NotNil(t, result)
			assert.Equal(t, tt.expectedType, result.Type)
			assert.Equal(t, tt.err, result.OriginalError)

			if tt.checkMessage {
				assert.NotEmpty(t, result.UserMessage)
				assert.NotEmpty(t, result.TroubleshootingTips)
			}
		})
	}
}

// TestErrorClassifier_isPermissionError tests permission detection
func TestErrorClassifier_isPermissionError(t *testing.T) {
	ec := NewErrorClassifier()

	tests := []struct {
		name     string
		errStr   string
		expected bool
	}{
		{"access denied lowercase", "accessdenied: access denied", true},
		{"forbidden", "403 forbidden", true},
		{"permission denied", "permission denied", true},
		{"unauthorized", "unauthorized request", true},
		{"not authorized", "not authorized to perform", true},
		{"insufficient permissions", "insufficient permissions", true},
		{"non-permission error", "network error occurred", false},
		{"empty string", "", false},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := ec.isPermissionError(tt.errStr)
			assert.Equal(t, tt.expected, got)
		})
	}
}

// TestErrorClassifier_classifyPermissionError tests permission error classification
func TestErrorClassifier_classifyPermissionError(t *testing.T) {
	ec := NewErrorClassifier()

	tests := []struct {
		name              string
		err               error
		errStr            string
		expectedTipsCount int
	}{
		{
			name:              "putobject error",
			err:               errors.New("AccessDenied on PutObject"),
			errStr:            "accessdenied on putobject",
			expectedTipsCount: 4,
		},
		{
			name:              "upload error",
			err:               errors.New("Cannot upload file"),
			errStr:            "cannot upload file",
			expectedTipsCount: 4,
		},
		{
			name:              "getobject error",
			err:               errors.New("AccessDenied on GetObject"),
			errStr:            "accessdenied on getobject",
			expectedTipsCount: 3,
		},
		{
			name:              "download error",
			err:               errors.New("Cannot download file"),
			errStr:            "cannot download file",
			expectedTipsCount: 3,
		},
		{
			name:              "bucket error",
			err:               errors.New("Cannot access bucket"),
			errStr:            "cannot access bucket",
			expectedTipsCount: 3,
		},
		{
			name:              "generic permission error",
			err:               errors.New("Access Denied"),
			errStr:            "access denied",
			expectedTipsCount: 4,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := ec.classifyPermissionError(tt.err, tt.errStr)

			require.NotNil(t, result)
			assert.Equal(t, ErrorTypePermission, result.Type)
			assert.Equal(t, tt.err, result.OriginalError)
			assert.NotEmpty(t, result.UserMessage)
			assert.Len(t, result.TroubleshootingTips, tt.expectedTipsCount)
		})
	}
}

// TestErrorClassifier_isNetworkError tests network error detection
func TestErrorClassifier_isNetworkError(t *testing.T) {
	ec := NewErrorClassifier()

	tests := []struct {
		name     string
		errStr   string
		expected bool
	}{
		{"connection refused", "connection refused", true},
		{"connection reset", "connection reset by peer", true},
		{"network unreachable", "network unreachable", true},
		{"no route to host", "no route to host", true},
		{"connection timeout", "connection timeout", true},
		{"dial tcp", "dial tcp: connection failed", true},
		{"i/o timeout", "i/o timeout", true},
		{"EOF", "unexpected eof", true},
		{"broken pipe", "write: broken pipe", true},
		{"connection aborted", "connection aborted", true},
		{"non-network error", "access denied", false},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := ec.isNetworkError(tt.errStr)
			assert.Equal(t, tt.expected, got)
		})
	}
}

// TestErrorClassifier_classifyNetworkError tests network error classification
func TestErrorClassifier_classifyNetworkError(t *testing.T) {
	ec := NewErrorClassifier()

	err := errors.New("connection refused")
	result := ec.classifyNetworkError(err, "connection refused")

	require.NotNil(t, result)
	assert.Equal(t, ErrorTypeNetwork, result.Type)
	assert.Equal(t, err, result.OriginalError)
	assert.Contains(t, result.UserMessage, "Network connectivity")
	assert.NotEmpty(t, result.TroubleshootingTips)
	assert.GreaterOrEqual(t, len(result.TroubleshootingTips), 5)
}

// TestErrorClassifier_isStorageError tests storage error detection
func TestErrorClassifier_isStorageError(t *testing.T) {
	ec := NewErrorClassifier()

	tests := []struct {
		name     string
		errStr   string
		expected bool
	}{
		{"nosuchbucket lowercase", "nosuchbucket", true},
		{"no such bucket", "no such bucket", true},
		{"bucket not found", "bucket not found", true},
		{"nosuchkey", "nosuchkey: the key does not exist", true},
		{"no such key", "no such key", true},
		{"service unavailable", "service unavailable", true},
		{"503 error", "503 service error", true},
		{"slow down", "slow down: reduce request rate", true},
		{"500 error", "500 internal error", true},
		{"internal error", "internal error occurred", true},
		{"non-storage error", "permission denied", false},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := ec.isStorageError(tt.errStr)
			assert.Equal(t, tt.expected, got)
		})
	}
}

// TestErrorClassifier_classifyStorageError tests storage error classification
func TestErrorClassifier_classifyStorageError(t *testing.T) {
	ec := NewErrorClassifier()

	tests := []struct {
		name              string
		err               error
		errStr            string
		expectedMsgPart   string
		expectedTipsCount int
	}{
		{
			name:              "nosuchbucket",
			err:               errors.New("NoSuchBucket"),
			errStr:            "nosuchbucket",
			expectedMsgPart:   "does not exist",
			expectedTipsCount: 4,
		},
		{
			name:              "bucket not found",
			err:               errors.New("Bucket not found"),
			errStr:            "bucket not found",
			expectedMsgPart:   "does not exist",
			expectedTipsCount: 4,
		},
		{
			name:              "slow down",
			err:               errors.New("SlowDown"),
			errStr:            "slow down",
			expectedMsgPart:   "request rate exceeded",
			expectedTipsCount: 4,
		},
		{
			name:              "503 service unavailable",
			err:               errors.New("503 Service Unavailable"),
			errStr:            "503 service unavailable",
			expectedMsgPart:   "temporarily unavailable",
			expectedTipsCount: 4,
		},
		{
			name:              "service unavailable text",
			err:               errors.New("Service Unavailable"),
			errStr:            "service unavailable",
			expectedMsgPart:   "temporarily unavailable",
			expectedTipsCount: 4,
		},
		{
			name:              "generic storage error",
			err:               errors.New("500 Internal Error"),
			errStr:            "500 internal error",
			expectedMsgPart:   "storage error",
			expectedTipsCount: 3,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := ec.classifyStorageError(tt.err, tt.errStr)

			require.NotNil(t, result)
			assert.Equal(t, ErrorTypeStorage, result.Type)
			assert.Equal(t, tt.err, result.OriginalError)
			assert.Contains(t, result.UserMessage, tt.expectedMsgPart)
			assert.Len(t, result.TroubleshootingTips, tt.expectedTipsCount)
		})
	}
}

// TestErrorClassifier_isValidationError tests validation error detection
func TestErrorClassifier_isValidationError(t *testing.T) {
	ec := NewErrorClassifier()

	tests := []struct {
		name     string
		errStr   string
		expected bool
	}{
		{"invalid", "invalid parameter", true},
		{"malformed", "malformed request", true},
		{"bad request", "bad request", true},
		{"400 error", "400 bad request", true},
		{"validation", "validation failed", true},
		{"invalid parameter", "invalid parameter: bucket", true},
		{"non-validation error", "network timeout", false},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := ec.isValidationError(tt.errStr)
			assert.Equal(t, tt.expected, got)
		})
	}
}

// TestErrorClassifier_classifyValidationError tests validation error classification
func TestErrorClassifier_classifyValidationError(t *testing.T) {
	ec := NewErrorClassifier()

	err := errors.New("Invalid parameter")
	result := ec.classifyValidationError(err, "invalid parameter")

	require.NotNil(t, result)
	assert.Equal(t, ErrorTypeValidation, result.Type)
	assert.Equal(t, err, result.OriginalError)
	assert.Contains(t, result.UserMessage, "Invalid request")
	assert.NotEmpty(t, result.TroubleshootingTips)
	assert.GreaterOrEqual(t, len(result.TroubleshootingTips), 4)
}

// TestErrorClassifier_isQuotaError tests quota error detection
func TestErrorClassifier_isQuotaError(t *testing.T) {
	ec := NewErrorClassifier()

	tests := []struct {
		name     string
		errStr   string
		expected bool
	}{
		{"quota exceeded", "quota exceeded", true},
		{"limit exceeded", "limit exceeded", true},
		{"too many requests", "too many requests", true},
		{"throttled", "request throttled", true},
		{"throttling", "throttling error", true},
		{"rate limit", "rate limit exceeded", true},
		{"non-quota error", "access denied", false},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := ec.isQuotaError(tt.errStr)
			assert.Equal(t, tt.expected, got)
		})
	}
}

// TestErrorClassifier_classifyQuotaError tests quota error classification
func TestErrorClassifier_classifyQuotaError(t *testing.T) {
	ec := NewErrorClassifier()

	err := errors.New("Quota exceeded")
	result := ec.classifyQuotaError(err, "quota exceeded")

	require.NotNil(t, result)
	assert.Equal(t, ErrorTypeQuota, result.Type)
	assert.Equal(t, err, result.OriginalError)
	assert.Contains(t, result.UserMessage, "quota")
	assert.NotEmpty(t, result.TroubleshootingTips)
	assert.GreaterOrEqual(t, len(result.TroubleshootingTips), 4)
}

// TestErrorClassifier_isTimeoutError tests timeout error detection
func TestErrorClassifier_isTimeoutError(t *testing.T) {
	ec := NewErrorClassifier()

	tests := []struct {
		name     string
		errStr   string
		expected bool
	}{
		{"timeout", "connection timeout", true},
		{"deadline exceeded", "context deadline exceeded", true},
		{"context deadline", "context deadline", true},
		{"timed out", "operation timed out", true},
		{"non-timeout error", "access denied", false},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := ec.isTimeoutError(tt.errStr)
			assert.Equal(t, tt.expected, got)
		})
	}
}

// TestErrorClassifier_classifyTimeoutError tests timeout error classification
func TestErrorClassifier_classifyTimeoutError(t *testing.T) {
	ec := NewErrorClassifier()

	err := errors.New("context deadline exceeded")
	result := ec.classifyTimeoutError(err, "context deadline exceeded")

	require.NotNil(t, result)
	assert.Equal(t, ErrorTypeTimeout, result.Type)
	assert.Equal(t, err, result.OriginalError)
	assert.Contains(t, result.UserMessage, "timed out")
	assert.NotEmpty(t, result.TroubleshootingTips)
	assert.GreaterOrEqual(t, len(result.TroubleshootingTips), 5)
}

// TestErrorClassifier_Classify_EdgeCases tests additional edge cases
func TestErrorClassifier_Classify_EdgeCases(t *testing.T) {
	ec := NewErrorClassifier()

	tests := []struct {
		name         string
		err          error
		expectedType ErrorType
	}{
		{"mixed case AccessDenied", errors.New("AccessDenied: User lacks permissions"), ErrorTypePermission},
		{"permission with upload context", errors.New("putobject access denied"), ErrorTypePermission},
		{"permission with download context", errors.New("getobject forbidden"), ErrorTypePermission},
		{"permission with bucket context", errors.New("bucket access denied"), ErrorTypePermission},
		{"network with dial context", errors.New("dial tcp: connection refused"), ErrorTypeNetwork},
		{"network broken pipe", errors.New("write: broken pipe"), ErrorTypeNetwork},
		{"network connection aborted", errors.New("connection aborted by peer"), ErrorTypeNetwork},
		{"storage nosuchkey", errors.New("NoSuchKey: key not found"), ErrorTypeStorage},
		{"storage 500 internal", errors.New("500 Internal Server Error"), ErrorTypeStorage},
		{"validation bad request", errors.New("400: Bad Request - invalid bucket name"), ErrorTypeValidation},
		{"validation malformed XML", errors.New("MalformedXML in request"), ErrorTypeValidation},
		{"quota too many requests", errors.New("TooManyRequests: rate limit exceeded"), ErrorTypeQuota},
		{"quota throttling", errors.New("Throttling exception occurred"), ErrorTypeQuota},
		{"timeout context deadline", errors.New("context deadline exceeded"), ErrorTypeTimeout},
		{"timeout operation timed out", errors.New("operation timed out after 30s"), ErrorTypeTimeout},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := ec.Classify(tt.err)
			require.NotNil(t, result)
			assert.Equal(t, tt.expectedType, result.Type)
			assert.NotEmpty(t, result.UserMessage)
			assert.NotEmpty(t, result.TroubleshootingTips)
		})
	}
}

// TestErrorClassifier_ClassifyPermissionError_AllBranches tests all permission error branches
func TestErrorClassifier_ClassifyPermissionError_AllBranches(t *testing.T) {
	ec := NewErrorClassifier()

	tests := []struct {
		name          string
		errStr        string
		expectedTips  int
	}{
		{"putobject specific", "access denied on putobject operation", 4},
		{"upload generic", "cannot upload to s3", 4},
		{"getobject specific", "getobject permission denied", 3},
		{"download generic", "failed to download from s3", 3},
		{"bucket operations", "bucket access denied", 3},
		{"generic permission", "unauthorized access", 4},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := ec.classifyPermissionError(errors.New(tt.errStr), tt.errStr)
			require.NotNil(t, result)
			assert.Equal(t, ErrorTypePermission, result.Type)
			assert.Len(t, result.TroubleshootingTips, tt.expectedTips)
		})
	}
}

// TestErrorClassifier_ClassifyStorageError_AllBranches tests all storage error branches  
func TestErrorClassifier_ClassifyStorageError_AllBranches(t *testing.T) {
	ec := NewErrorClassifier()

	tests := []struct {
		name         string
		errStr       string
		msgContains  string
		tipsCount    int
	}{
		{"nosuchbucket variant 1", "nosuchbucket error", "does not exist", 4},
		{"nosuchbucket variant 2", "bucket not found", "does not exist", 4},
		{"slow down", "slow down request", "request rate exceeded", 4},
		{"503 error", "503 service unavailable", "temporarily unavailable", 4},
		{"service unavailable text", "service unavailable", "temporarily unavailable", 4},
		{"500 generic", "500 internal server error", "storage error", 3},
		{"internal error generic", "internal error", "storage error", 3},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := ec.classifyStorageError(errors.New(tt.errStr), tt.errStr)
			require.NotNil(t, result)
			assert.Equal(t, ErrorTypeStorage, result.Type)
			assert.Contains(t, result.UserMessage, tt.msgContains)
			assert.Len(t, result.TroubleshootingTips, tt.tipsCount)
		})
	}
}
