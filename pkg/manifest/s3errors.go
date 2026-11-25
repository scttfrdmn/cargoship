package manifest

import (
	"errors"

	"github.com/aws/aws-sdk-go-v2/service/s3/types"
	"github.com/aws/smithy-go"
)

// isNotFound reports whether err is S3's "no such object".
func isNotFound(err error) bool {
	var nsk *types.NoSuchKey
	if errors.As(err, &nsk) {
		return true
	}
	var apiErr smithy.APIError
	if errors.As(err, &apiErr) {
		switch apiErr.ErrorCode() {
		case "NoSuchKey", "NotFound", "404":
			return true
		}
	}
	return false
}

// IsNotFound reports whether err (from a manifest download) means the object
// doesn't exist, as opposed to existing but being unreadable (#919).
func IsNotFound(err error) bool { return isNotFound(err) }
