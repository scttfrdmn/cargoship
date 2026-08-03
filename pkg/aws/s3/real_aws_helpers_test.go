//go:build integration || performance

// Bucket/object helpers shared by the real-AWS suites in this package.
//
// The build tag is an OR on purpose. These helpers were originally defined in
// real_aws_integration_test.go (`//go:build integration`) but called from
// stress_test.go and extreme_test.go (`//go:build performance`). Those tags are
// never set together, so the callers could never see the callees and the
// performance-tagged files did not compile — for as long as nothing built them
// (#329). Anything used across both suites belongs here, not in one of them.

package s3

import (
	"context"
	"os"

	"github.com/aws/aws-sdk-go-v2/aws"
	"github.com/aws/aws-sdk-go-v2/service/s3"
	"github.com/aws/aws-sdk-go-v2/service/s3/types"
)

// transpTestRegion is the region ensureTestBucket creates buckets in. It is
// shared because both suites create buckets; the bucket NAMES are per-suite and
// stay with their own files (an unused var here trips staticcheck under
// whichever tag doesn't reference it).
var transpTestRegion = firstNonEmpty(os.Getenv("AWS_REGION"), "us-west-2")

// firstNonEmpty returns the first non-empty string, or "" if there is none.
func firstNonEmpty(vals ...string) string {
	for _, v := range vals {
		if v != "" {
			return v
		}
	}
	return ""
}

// ensureTestBucket creates bucket if it does not already exist.
func ensureTestBucket(ctx context.Context, s3Client *s3.Client, bucket string) error {
	_, err := s3Client.HeadBucket(ctx, &s3.HeadBucketInput{
		Bucket: aws.String(bucket),
	})

	if err == nil {
		return nil
	}

	_, err = s3Client.CreateBucket(ctx, &s3.CreateBucketInput{
		Bucket: aws.String(bucket),
		CreateBucketConfiguration: &types.CreateBucketConfiguration{
			LocationConstraint: types.BucketLocationConstraint(transpTestRegion),
		},
	})

	return err
}

// cleanupTestObjects deletes every object under prefix, best-effort.
func cleanupTestObjects(ctx context.Context, s3Client *s3.Client, bucket, prefix string) {
	result, err := s3Client.ListObjectsV2(ctx, &s3.ListObjectsV2Input{
		Bucket: aws.String(bucket),
		Prefix: aws.String(prefix),
	})

	if err != nil {
		return
	}

	for _, obj := range result.Contents {
		_, _ = s3Client.DeleteObject(ctx, &s3.DeleteObjectInput{
			Bucket: aws.String(bucket),
			Key:    obj.Key,
		})
	}
}
