// Package manifest provides the S3 read interface shared by restore, deep
// verify, and the archive browser.
package manifest

import (
	"context"

	"github.com/aws/aws-sdk-go-v2/service/s3"
)

// S3Downloader is the subset of the S3 API the manifest package needs to read
// stored objects. Restore, deep verify, and the archive browser all accept one,
// so tests can supply an in-memory implementation instead of a real client.
type S3Downloader interface {
	GetObject(ctx context.Context, input *s3.GetObjectInput, opts ...func(*s3.Options)) (*s3.GetObjectOutput, error)
}
