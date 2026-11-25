//go:build integration

package cmd

import (
	"bytes"
	"context"
	"fmt"
	"testing"
	"time"

	"github.com/aws/aws-sdk-go-v2/aws"
	"github.com/aws/aws-sdk-go-v2/credentials"
	"github.com/aws/aws-sdk-go-v2/service/kms"
	"github.com/aws/aws-sdk-go-v2/service/s3"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/scttfrdmn/cargoship/pkg/manifest"
)

// TestListUploadManifestsChecked_ReportsUnreadable pins #919. The chain guard
// skipped any manifest it couldn't read, so delete and prune decided they were
// safe from the readable set alone and could strand a version they couldn't see.
// Unreadable manifests are now reported separately; an upload with no final
// manifest (only a partial) is absent, not unreadable.
func TestListUploadManifestsChecked_ReportsUnreadable(t *testing.T) {
	ctx := context.Background()
	cfg := aws.Config{Region: "us-east-1", BaseEndpoint: aws.String(substrateURL),
		Credentials: credentials.NewStaticCredentialsProvider("test", "test", "")}
	s3c := s3.NewFromConfig(cfg, func(o *s3.Options) { o.UsePathStyle = true })
	kmsc := kms.NewFromConfig(cfg)
	bucket := fmt.Sprintf("guard-%d", time.Now().UnixNano())
	_, err := s3c.CreateBucket(ctx, &s3.CreateBucketInput{Bucket: aws.String(bucket)})
	require.NoError(t, err)
	prefix := "archive"

	good := &manifest.Manifest{Version: manifest.ManifestVersion, UploadID: "u-good", Bucket: bucket, Prefix: prefix}
	require.NoError(t, good.UploadToS3(ctx, s3c, true))
	put := func(key string, body []byte) {
		_, err := s3c.PutObject(ctx, &s3.PutObjectInput{Bucket: aws.String(bucket), Key: aws.String(key), Body: bytes.NewReader(body)})
		require.NoError(t, err)
	}
	put(prefix+"/uploads/u-broken/manifest.json.gz", []byte("not gzip at all"))             // exists, unreadable
	put(prefix+"/uploads/u-partial/manifest.partial.json.gz", []byte("in-progress upload")) // no final manifest

	ms, unreadable, err := listUploadManifestsChecked(ctx, s3c, kmsc, bucket, prefix)
	require.NoError(t, err)
	require.Len(t, ms, 1)
	assert.Equal(t, "u-good", ms[0].UploadID)
	require.Len(t, unreadable, 1, "the broken manifest must be reported, not skipped")
	assert.Equal(t, "u-broken", unreadable[0].UploadID)
	assert.Contains(t, unreadableSummary(unreadable), "u-broken")

	// The read-only listing still skips it.
	all, err := listUploadManifests(ctx, s3c, kmsc, bucket, prefix)
	require.NoError(t, err)
	assert.Len(t, all, 1)
}
