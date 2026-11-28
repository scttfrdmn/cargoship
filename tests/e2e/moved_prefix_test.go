//go:build e2e

// End-to-end regression for #1032, the prefix half of #335: verifying and
// restoring an archive that has been COPIED to a different prefix.
//
// The manifest records the prefix it was written under, and every object key
// was resolved against that recorded prefix rather than the one the manifest
// was just read from. So `verify --deep` of a copy read the ORIGINAL's chunks:
// with the copy's chunks overwritten by garbage it still reported PASS, which is
// the one thing an integrity check must not do.
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

// uploadChunkedArchive uploads two patterned 6 MiB files (big enough to defeat
// the direct-upload heuristic, so the archive has chunks) to bucket/prefix and
// returns the upload ID, the source dir, and the file content.
func uploadChunkedArchive(t *testing.T, bucket, prefix string) (uploadID, src string, content []byte) {
	t.Helper()
	src = t.TempDir()
	content = make([]byte, 6*1024*1024)
	for i := range content {
		content[i] = byte(i % 251)
	}
	for _, name := range []string{"prefix-a.bin", "prefix-b.bin"} {
		if err := os.WriteFile(filepath.Join(src, name), content, 0o644); err != nil {
			t.Fatalf("write source: %v", err)
		}
	}
	out := runCargoship(t, "upload", src, "s3://"+bucket+"/"+prefix, "--region", "us-east-1")
	return extractUploadID(t, out), src, content
}

// copyPrefix copies every object under from/ to the same relative key under
// to/ in bucket and returns the destination keys.
func copyPrefix(t *testing.T, bucket, from, to string) []string {
	t.Helper()
	ctx := context.Background()
	client := fixtureS3Client()
	page, err := client.ListObjectsV2(ctx, &s3.ListObjectsV2Input{Bucket: aws.String(bucket), Prefix: aws.String(from + "/")})
	if err != nil {
		t.Fatalf("list %s/%s: %v", bucket, from, err)
	}
	var keys []string
	for _, obj := range page.Contents {
		key := aws.ToString(obj.Key)
		body, err := client.GetObject(ctx, &s3.GetObjectInput{Bucket: aws.String(bucket), Key: aws.String(key)})
		if err != nil {
			t.Fatalf("get %s: %v", key, err)
		}
		data := readAllAndClose(t, body)
		dst := to + strings.TrimPrefix(key, from)
		if _, err := client.PutObject(ctx, &s3.PutObjectInput{Bucket: aws.String(bucket), Key: aws.String(dst), Body: bytes.NewReader(data)}); err != nil {
			t.Fatalf("put %s: %v", dst, err)
		}
		keys = append(keys, dst)
	}
	if len(keys) == 0 {
		t.Fatalf("nothing under %s/%s to copy", bucket, from)
	}
	return keys
}

// corruptChunks overwrites every non-manifest object among keys with garbage.
func corruptChunks(t *testing.T, bucket string, keys []string) {
	t.Helper()
	client := fixtureS3Client()
	n := 0
	for _, key := range keys {
		if strings.Contains(key, "manifest") {
			continue
		}
		garbage := bytes.Repeat([]byte("cargoship-corrupted-copy-"), 4096)
		if _, err := client.PutObject(context.Background(), &s3.PutObjectInput{
			Bucket: aws.String(bucket), Key: aws.String(key), Body: bytes.NewReader(garbage),
		}); err != nil {
			t.Fatalf("corrupt %s: %v", key, err)
		}
		n++
	}
	if n == 0 {
		t.Fatal("the copy has no chunk objects to corrupt; the test would prove nothing")
	}
}

// TestMovedPrefix_DeepVerifyReadsTheCopy is the #1032 regression: a copy at a
// new prefix, with every chunk corrupted and the original left intact, must
// fail verify --deep and restore. On the bug both passed by reading the
// original.
func TestMovedPrefix_DeepVerifyReadsTheCopy(t *testing.T) {
	const bucket = "moved-prefix-corrupt"
	if err := createBucket(substrateURL, bucket); err != nil {
		t.Fatalf("create bucket: %v", err)
	}
	uploadID, src, _ := uploadChunkedArchive(t, bucket, "orig")
	keys := copyPrefix(t, bucket, "orig", "copied/archive")
	copyURL := "s3://" + bucket + "/copied/archive/uploads/" + uploadID

	// The pristine copy verifies.
	runCargoship(t, "verify", copyURL, "--deep", "--region", "us-east-1")

	corruptChunks(t, bucket, keys)

	if out, err := runCargoshipAllowErr(t, "verify", copyURL, "--deep", "--region", "us-east-1"); err == nil {
		t.Fatalf("verify --deep of a copy with every chunk corrupted PASSED, so it read the original:\n%s", out)
	}
	if out, err := runCargoshipAllowErr(t, "restore", copyURL, t.TempDir(), "--region", "us-east-1",
		"--file", filepath.Join(src, "prefix-a.bin")); err == nil {
		t.Fatalf("restore from a copy with every chunk corrupted succeeded, so it read the original:\n%s", out)
	}
}

// TestMovedPrefix_RestoresFromTheCopy: with the original deleted, verify and
// restore of the copy succeed, so they can only have read the copy.
func TestMovedPrefix_RestoresFromTheCopy(t *testing.T) {
	const bucket = "moved-prefix-delete"
	if err := createBucket(substrateURL, bucket); err != nil {
		t.Fatalf("create bucket: %v", err)
	}
	uploadID, src, content := uploadChunkedArchive(t, bucket, "orig")
	copyPrefix(t, bucket, "orig", "moved")

	client := fixtureS3Client()
	page, err := client.ListObjectsV2(context.Background(), &s3.ListObjectsV2Input{Bucket: aws.String(bucket), Prefix: aws.String("orig/")})
	if err != nil {
		t.Fatalf("list original: %v", err)
	}
	var orig []string
	for _, o := range page.Contents {
		orig = append(orig, aws.ToString(o.Key))
	}
	for _, key := range orig {
		if _, err := client.DeleteObject(context.Background(), &s3.DeleteObjectInput{Bucket: aws.String(bucket), Key: aws.String(key)}); err != nil {
			t.Fatalf("delete %s: %v", key, err)
		}
	}

	copyURL := "s3://" + bucket + "/moved/uploads/" + uploadID
	runCargoship(t, "verify", copyURL, "--deep", "--region", "us-east-1")
	restoreDir := t.TempDir()
	runCargoship(t, "restore", copyURL, restoreDir, "--region", "us-east-1", "--file", filepath.Join(src, "prefix-a.bin"))
	got, err := os.ReadFile(findFileByBase(t, restoreDir, "prefix-a.bin"))
	if err != nil {
		t.Fatalf("read restored file: %v", err)
	}
	if !bytes.Equal(got, content) {
		t.Fatalf("restored file differs from the source (%d bytes vs %d)", len(got), len(content))
	}
}
