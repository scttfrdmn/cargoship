//go:build e2e

// End-to-end regression for #335: restoring and verifying an archive that has
// been COPIED to a different bucket.
//
// Every restore path resolved objects against the bucket recorded inside the
// manifest at upload time, never the bucket the manifest was just read out of.
// So `cargoship restore s3://archive-copy/...` loaded the manifest from
// archive-copy and then fetched every chunk from the original bucket. The caller
// had supplied a bucket and had no indication it was ignored.
//
// The 404 is the benign case. The dangerous one is a restore or a deep verify
// that SUCCEEDS against the stale original — certifying a copy nobody read,
// which is exactly the claim a `verify --deep` is supposed to make about
// specific bytes in a specific place.
//
// Copying to a new bucket and DELETING the original is what makes this
// unambiguous: with the original gone, any operation that still succeeds must
// have read the copy.
package e2e

import (
	"bytes"
	"context"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/aws/aws-sdk-go-v2/aws"
	"github.com/aws/aws-sdk-go-v2/service/s3"
)

// TestMovedArchive_RestoresAndVerifiesFromTheBucketItWasGiven is the #335
// regression. Chunked layout with a prefix, because the chunk objects are what
// get addressed by bucket — a direct-layout archive stores files as plain
// objects and would not exercise the chunk fetch path.
func TestMovedArchive_RestoresAndVerifiesFromTheBucketItWasGiven(t *testing.T) {
	const (
		origBucket = "moved-archive-origin"
		copyBucket = "moved-archive-copy"
		prefix     = "archives"
	)
	for _, b := range []string{origBucket, copyBucket} {
		if err := createBucket(substrateURL, b); err != nil {
			t.Fatalf("create bucket %s: %v", b, err)
		}
	}

	// Chunked layout needs an average file size that defeats the direct-upload
	// heuristic. Patterned so it compresses small and the test stays fast.
	src := t.TempDir()
	big := make([]byte, 6*1024*1024)
	for i := range big {
		big[i] = byte(i % 251)
	}
	for _, name := range []string{"moved-a.bin", "moved-b.bin"} {
		if err := os.WriteFile(filepath.Join(src, name), big, 0o644); err != nil {
			t.Fatalf("write source: %v", err)
		}
	}

	uploadDest := "s3://" + origBucket + "/" + prefix
	out := runCargoship(t, "upload", src, uploadDest, "--region", "us-east-1")
	uploadID := extractUploadID(t, out)

	// Copy every object to the second bucket under identical keys, then delete
	// the original. Anything that still works from here read the copy.
	keys := copyAllObjects(t, origBucket, copyBucket)
	if len(keys) == 0 {
		t.Fatal("upload produced no objects to copy")
	}
	deleteAllObjects(t, origBucket, keys)

	copyURL := "s3://" + copyBucket + "/" + prefix + "/uploads/" + uploadID

	// 1. verify --deep against the copy must read the copy. Before the fix it
	// fetched from the (now empty) original and reported every chunk missing.
	runCargoship(t, "verify", copyURL, "--deep", "--region", "us-east-1")

	// 2. Restore by the exact manifest path, the same combination #334 covers,
	// so the Glacier pre-flight is exercised rather than skipped.
	restoreDir := t.TempDir()
	runCargoship(t, "restore", copyURL, restoreDir,
		"--region", "us-east-1", "--file", filepath.Join(src, "moved-a.bin"))

	got, err := os.ReadFile(findFileByBase(t, restoreDir, "moved-a.bin"))
	if err != nil {
		t.Fatalf("read restored file: %v", err)
	}
	if len(got) != len(big) {
		t.Fatalf("restored size %d, want %d", len(got), len(big))
	}
	for i := range got {
		if got[i] != big[i] {
			t.Fatalf("restored content differs at byte %d: got %d want %d", i, got[i], big[i])
		}
	}
}

// copyAllObjects copies every object from src to dst under identical keys and
// returns the keys copied.
func copyAllObjects(t *testing.T, srcBucket, dstBucket string) []string {
	t.Helper()
	ctx := context.Background()
	client := fixtureS3Client()

	var keys []string
	var token *string
	for {
		page, err := client.ListObjectsV2(ctx, &s3.ListObjectsV2Input{
			Bucket:            aws.String(srcBucket),
			ContinuationToken: token,
		})
		if err != nil {
			t.Fatalf("list %s: %v", srcBucket, err)
		}
		for _, obj := range page.Contents {
			key := aws.ToString(obj.Key)
			body, getErr := client.GetObject(ctx, &s3.GetObjectInput{
				Bucket: aws.String(srcBucket),
				Key:    aws.String(key),
			})
			if getErr != nil {
				t.Fatalf("get %s: %v", key, getErr)
			}
			data := readAllAndClose(t, body)
			if _, putErr := client.PutObject(ctx, &s3.PutObjectInput{
				Bucket: aws.String(dstBucket),
				Key:    aws.String(key),
				Body:   bytes.NewReader(data),
			}); putErr != nil {
				t.Fatalf("put %s: %v", key, putErr)
			}
			keys = append(keys, key)
		}
		if page.IsTruncated == nil || !*page.IsTruncated {
			break
		}
		token = page.NextContinuationToken
	}
	return keys
}

// deleteAllObjects removes the given keys from bucket, so that a later success
// can only have come from somewhere else.
func deleteAllObjects(t *testing.T, bucket string, keys []string) {
	t.Helper()
	ctx := context.Background()
	client := fixtureS3Client()
	for _, key := range keys {
		if _, err := client.DeleteObject(ctx, &s3.DeleteObjectInput{
			Bucket: aws.String(bucket),
			Key:    aws.String(key),
		}); err != nil {
			t.Fatalf("delete %s/%s: %v", bucket, key, err)
		}
	}

	// Confirm the original really is empty. If the emulator ignored the deletes,
	// the whole test would pass vacuously — a fetch from the original bucket
	// would still succeed and prove nothing.
	page, err := client.ListObjectsV2(ctx, &s3.ListObjectsV2Input{Bucket: aws.String(bucket)})
	if err != nil {
		t.Fatalf("list %s after delete: %v", bucket, err)
	}
	if len(page.Contents) != 0 {
		var remaining []string
		for _, o := range page.Contents {
			remaining = append(remaining, aws.ToString(o.Key))
		}
		t.Fatalf("origin bucket still holds %d object(s) (%s); the test could pass without reading the copy",
			len(page.Contents), strings.Join(remaining, ", "))
	}
}
