// Package pipeline provides streaming pipeline for CargoShip
package pipeline

import (
	"fmt"
	"strings"
)

// ErrorType represents the category of error
type ErrorType int

const (
	ErrorTypeUnknown ErrorType = iota
	ErrorTypePermission
	ErrorTypeNetwork
	ErrorTypeStorage
	ErrorTypeValidation
	ErrorTypeQuota
	ErrorTypeTimeout
)

// String returns the string representation of ErrorType
func (et ErrorType) String() string {
	switch et {
	case ErrorTypePermission:
		return "Permission"
	case ErrorTypeNetwork:
		return "Network"
	case ErrorTypeStorage:
		return "Storage"
	case ErrorTypeValidation:
		return "Validation"
	case ErrorTypeQuota:
		return "Quota"
	case ErrorTypeTimeout:
		return "Timeout"
	default:
		return "Unknown"
	}
}

// ClassifiedError contains error classification and troubleshooting information
type ClassifiedError struct {
	OriginalError       error
	Type                ErrorType
	UserMessage         string
	TroubleshootingTips []string
}

// ErrorClassifier classifies errors and provides troubleshooting guidance
type ErrorClassifier struct{}

// NewErrorClassifier creates a new error classifier
func NewErrorClassifier() *ErrorClassifier {
	return &ErrorClassifier{}
}

// Classify analyzes an error and returns classification with troubleshooting tips
func (ec *ErrorClassifier) Classify(err error) *ClassifiedError {
	if err == nil {
		return nil
	}

	errStr := strings.ToLower(err.Error())

	// Permission errors
	if ec.isPermissionError(errStr) {
		return ec.classifyPermissionError(err, errStr)
	}

	// Network errors
	if ec.isNetworkError(errStr) {
		return ec.classifyNetworkError(err, errStr)
	}

	// Storage errors
	if ec.isStorageError(errStr) {
		return ec.classifyStorageError(err, errStr)
	}

	// Validation errors
	if ec.isValidationError(errStr) {
		return ec.classifyValidationError(err, errStr)
	}

	// Quota errors
	if ec.isQuotaError(errStr) {
		return ec.classifyQuotaError(err, errStr)
	}

	// Timeout errors
	if ec.isTimeoutError(errStr) {
		return ec.classifyTimeoutError(err, errStr)
	}

	// Unknown error
	return &ClassifiedError{
		OriginalError: err,
		Type:          ErrorTypeUnknown,
		UserMessage:   fmt.Sprintf("Unexpected error: %v", err),
		TroubleshootingTips: []string{
			"Check the error details above for more information",
			"Verify your AWS credentials and configuration",
			"Try running the command again with --verbose for detailed logs",
		},
	}
}

// isPermissionError checks if error is related to permissions
func (ec *ErrorClassifier) isPermissionError(errStr string) bool {
	permissionKeywords := []string{
		"accessdenied",
		"forbidden",
		"access denied",
		"permission denied",
		"unauthorized",
		"403",
		"not authorized",
		"insufficient permissions",
	}

	for _, keyword := range permissionKeywords {
		if strings.Contains(errStr, keyword) {
			return true
		}
	}
	return false
}

// classifyPermissionError provides specific guidance for permission errors
func (ec *ErrorClassifier) classifyPermissionError(err error, errStr string) *ClassifiedError {
	classified := &ClassifiedError{
		OriginalError: err,
		Type:          ErrorTypePermission,
		UserMessage:   "Permission denied: AWS IAM credentials lack required permissions",
	}

	// S3-specific permissions
	if strings.Contains(errStr, "putobject") || strings.Contains(errStr, "upload") {
		classified.TroubleshootingTips = []string{
			"Verify IAM user/role has 's3:PutObject' permission",
			"Check bucket policy allows uploads from your AWS account",
			"Ensure S3 bucket ACLs are not blocking uploads",
			"Required permissions: s3:PutObject, s3:PutObjectAcl (if using ACLs)",
		}
	} else if strings.Contains(errStr, "getobject") || strings.Contains(errStr, "download") {
		classified.TroubleshootingTips = []string{
			"Verify IAM user/role has 's3:GetObject' permission",
			"Check bucket policy allows reads from your AWS account",
			"Required permissions: s3:GetObject, s3:ListBucket",
		}
	} else if strings.Contains(errStr, "bucket") {
		classified.TroubleshootingTips = []string{
			"Verify IAM user/role has bucket-level permissions",
			"Check bucket policy and bucket ACLs",
			"Required permissions: s3:ListBucket, s3:GetBucketLocation",
		}
	} else {
		classified.TroubleshootingTips = []string{
			"Run: aws sts get-caller-identity (verify AWS credentials)",
			"Check IAM policy attached to your user/role",
			"Verify bucket policy allows your AWS account",
			"Required S3 permissions: s3:PutObject, s3:GetObject, s3:ListBucket",
		}
	}

	return classified
}

// isNetworkError checks if error is network-related
func (ec *ErrorClassifier) isNetworkError(errStr string) bool {
	networkKeywords := []string{
		"connection refused",
		"connection reset",
		"network unreachable",
		"no route to host",
		"connection timeout",
		"dial tcp",
		"i/o timeout",
		"eof",
		"broken pipe",
		"connection aborted",
	}

	for _, keyword := range networkKeywords {
		if strings.Contains(errStr, keyword) {
			return true
		}
	}
	return false
}

// classifyNetworkError provides guidance for network errors
func (ec *ErrorClassifier) classifyNetworkError(err error, errStr string) *ClassifiedError {
	return &ClassifiedError{
		OriginalError: err,
		Type:          ErrorTypeNetwork,
		UserMessage:   "Network connectivity issue: Unable to reach AWS S3",
		TroubleshootingTips: []string{
			"Check your internet connection",
			"Verify AWS region is correct (current region might be unreachable)",
			"Check firewall/proxy settings blocking AWS endpoints",
			"Try a different AWS region closer to your location",
			"Verify VPN or corporate network policies allow AWS access",
			"Test connectivity: curl https://s3.amazonaws.com",
		},
	}
}

// isStorageError checks if error is storage-related
func (ec *ErrorClassifier) isStorageError(errStr string) bool {
	storageKeywords := []string{
		"nosuchbucket",
		"no such bucket",
		"bucketnotfound",
		"bucket not found",
		"nosuchkey",
		"no such key",
		"service unavailable",
		"503",
		"slow down",
		"500",
		"internal error",
	}

	for _, keyword := range storageKeywords {
		if strings.Contains(errStr, keyword) {
			return true
		}
	}
	return false
}

// classifyStorageError provides guidance for storage errors
func (ec *ErrorClassifier) classifyStorageError(err error, errStr string) *ClassifiedError {
	classified := &ClassifiedError{
		OriginalError: err,
		Type:          ErrorTypeStorage,
	}

	if strings.Contains(errStr, "nosuchbucket") || strings.Contains(errStr, "bucket not found") {
		classified.UserMessage = "S3 bucket does not exist or is in a different region"
		classified.TroubleshootingTips = []string{
			"Verify bucket name is correct (no typos)",
			"Check bucket exists: aws s3 ls s3://your-bucket-name",
			"Verify bucket is in the specified region",
			"Create bucket if needed: cargoship setup (interactive wizard)",
		}
	} else if strings.Contains(errStr, "slow down") {
		classified.UserMessage = "S3 request rate exceeded: Too many requests to S3"
		classified.TroubleshootingTips = []string{
			"S3 automatically throttles high request rates",
			"CargoShip uses 8 parallel shards to mitigate this",
			"Wait a few seconds and retry the upload",
			"Consider using exponential backoff (built into retry logic)",
		}
	} else if strings.Contains(errStr, "503") || strings.Contains(errStr, "service unavailable") {
		classified.UserMessage = "S3 service temporarily unavailable"
		classified.TroubleshootingTips = []string{
			"AWS S3 is experiencing temporary issues",
			"Wait 30-60 seconds and retry",
			"Check AWS Service Health Dashboard: https://status.aws.amazon.com",
			"CargoShip will automatically retry failed uploads",
		}
	} else {
		classified.UserMessage = "S3 storage error occurred"
		classified.TroubleshootingTips = []string{
			"Check S3 bucket status and configuration",
			"Verify bucket exists and is accessible",
			"Check AWS Service Health Dashboard for S3 issues",
		}
	}

	return classified
}

// isValidationError checks if error is validation-related
func (ec *ErrorClassifier) isValidationError(errStr string) bool {
	validationKeywords := []string{
		"invalid",
		"malformed",
		"bad request",
		"400",
		"validation",
		"invalid parameter",
	}

	for _, keyword := range validationKeywords {
		if strings.Contains(errStr, keyword) {
			return true
		}
	}
	return false
}

// classifyValidationError provides guidance for validation errors
func (ec *ErrorClassifier) classifyValidationError(err error, errStr string) *ClassifiedError {
	return &ClassifiedError{
		OriginalError: err,
		Type:          ErrorTypeValidation,
		UserMessage:   "Invalid request parameters or configuration",
		TroubleshootingTips: []string{
			"Verify S3 bucket name follows naming rules (lowercase, no underscores)",
			"Check region name is valid (us-west-2, us-east-1, etc.)",
			"Validate storage class (STANDARD, INTELLIGENT_TIERING, etc.)",
			"Review command arguments for typos or invalid values",
		},
	}
}

// isQuotaError checks if error is quota-related
func (ec *ErrorClassifier) isQuotaError(errStr string) bool {
	quotaKeywords := []string{
		"quota exceeded",
		"limit exceeded",
		"too many requests",
		"throttl",
		"rate limit",
	}

	for _, keyword := range quotaKeywords {
		if strings.Contains(errStr, keyword) {
			return true
		}
	}
	return false
}

// classifyQuotaError provides guidance for quota errors
func (ec *ErrorClassifier) classifyQuotaError(err error, errStr string) *ClassifiedError {
	return &ClassifiedError{
		OriginalError: err,
		Type:          ErrorTypeQuota,
		UserMessage:   "AWS quota or rate limit exceeded",
		TroubleshootingTips: []string{
			"AWS has rate limits on S3 API requests",
			"Wait a few minutes before retrying",
			"Consider requesting quota increase from AWS Support",
			"Check Service Quotas in AWS Console",
		},
	}
}

// isTimeoutError checks if error is timeout-related
func (ec *ErrorClassifier) isTimeoutError(errStr string) bool {
	timeoutKeywords := []string{
		"timeout",
		"deadline exceeded",
		"context deadline",
		"timed out",
	}

	for _, keyword := range timeoutKeywords {
		if strings.Contains(errStr, keyword) {
			return true
		}
	}
	return false
}

// classifyTimeoutError provides guidance for timeout errors
func (ec *ErrorClassifier) classifyTimeoutError(err error, errStr string) *ClassifiedError {
	return &ClassifiedError{
		OriginalError: err,
		Type:          ErrorTypeTimeout,
		UserMessage:   "Request timed out: Operation took too long to complete",
		TroubleshootingTips: []string{
			"Check your internet connection speed",
			"Large files may take longer to upload",
			"Consider increasing timeout values if uploads consistently fail",
			"Verify network is stable (no packet loss or high latency)",
			"Try uploading during off-peak hours for better performance",
		},
	}
}
