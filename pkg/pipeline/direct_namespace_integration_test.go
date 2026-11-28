//go:build integration

package pipeline

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/aws/aws-sdk-go-v2/aws"
	"github.com/aws/aws-sdk-go-v2/config"
	"github.com/aws/aws-sdk-go-v2/service/s3"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/scttfrdmn/cargoship/pkg/manifest"
)

// directNamespaceFixture is a source tree whose root holds files named like
// CargoShip's own manifest objects.
var directNamespaceFixture = map[string]string{
	"manifest.json.gz":                   "cargoship-test user file named like the final manifest\n",
	"manifest.json":                      "cargoship-test user file named like the plain manifest\n",
	"manifest.partial.json.gz":           "cargoship-test user file named like the partial manifest\n",
	"manifest.encrypted.json":            "cargoship-test user file named like the encrypted manifest\n",
	"manifest.encrypted.json.gz":         "cargoship-test user file named like the gzip encrypted manifest\n",
	"manifest.partial.encrypted.json.gz": "cargoship-test user file named like the encrypted partial manifest\n",
	"sub/data.txt":                       "cargoship-test nested file\n",
}

func directNamespaceClient(t *testing.T) (*s3.Client, string, string) {
	t.Helper()
	bucket := os.Getenv("CARGOSHIP_TEST_BUCKET")
	if bucket == "" {
		bucket = "cargoship-pipeline-test"
	}
	region := os.Getenv("AWS_REGION")
	if region == "" {
		region = "us-east-1"
	}
	cfg, err := config.LoadDefaultConfig(context.Background(), config.WithRegion(region))
	require.NoError(t, err)
	return s3.NewFromConfig(cfg, func(o *s3.Options) { o.UsePathStyle = substrateURL != "" }), bucket, region
}

// uploadDirectFixture writes the fixture tree, uploads it in direct mode and
// returns the source dir, prefix and upload ID.
func uploadDirectFixture(t *testing.T, client *s3.Client, bucket, region, name string) (string, string, string) {
	t.Helper()
	src := t.TempDir()
	for rel, body := range directNamespaceFixture {
		p := filepath.Join(src, filepath.FromSlash(rel))
		require.NoError(t, os.MkdirAll(filepath.Dir(p), 0o750))
		require.NoError(t, os.WriteFile(p, []byte(body), 0o600))
	}
	prefix := fmt.Sprintf("%s-%d", name, time.Now().UnixNano())
	uploadID := fmt.Sprintf("u-%d", time.Now().UnixNano())
	p, err := NewPipeline(&PipelineConfig{
		ScannerWorkers: 1, ArchiverWorkers: 1, UploaderWorkers: 1,
		S3Bucket: bucket, S3Prefix: prefix, S3Region: region,
		UseRealS3: true, S3Client: client, S3PartSize: 5 * 1024 * 1024,
		UploadID: uploadID, EnableManifest: true, EnablePartialManifest: true,
		SourcePath: src, FileChecksums: true, ForceDirectUpload: true,
		EnableMultiPrefix: true, ShardCount: 4, WorkersPerPrefix: 1,
	})
	require.NoError(t, err)
	r, err := p.Run(context.Background(), src)
	require.NoError(t, err)
	require.True(t, r.Success, "errors: %v", r.Errors)
	return src, prefix, uploadID
}

// restoreAndCompare restores every fixture file from m and asserts each is
// byte-identical to its source.
func restoreAndCompare(t *testing.T, client *s3.Client, m *manifest.Manifest, src string) {
	t.Helper()
	require.Len(t, m.Files, len(directNamespaceFixture))
	targets := make([]string, 0, len(m.Files))
	for _, f := range m.Files {
		targets = append(targets, f.Path)
	}
	out := t.TempDir()
	stats, err := manifest.NewSelectiveExtractor(m, client, 0).BatchRestore(context.Background(), targets, out)
	require.NoError(t, err)
	require.Zero(t, stats.Failed, "every file must restore")
	byBase := indexFilesByBase(t, out)
	for rel, want := range directNamespaceFixture {
		p, ok := byBase[filepath.Base(rel)]
		require.True(t, ok, "%s was not restored", rel)
		got, err := os.ReadFile(p)
		require.NoError(t, err)
		assert.Equal(t, want, string(got), "%s must round-trip byte-identical", rel)
	}
	_ = src
}

// TestDirectUpload_ManifestNamedSourceFilesRoundTrip pins #1020. Direct-mode
// objects were keyed at <prefix>/uploads/<id>/<relpath>, the namespace of
// CargoShip's own manifest objects, so a root-level manifest.json.gz was
// overwritten by the manifest, manifest.partial.json.gz was deleted with the
// partial manifest, and a manifest.encrypted.json[.gz] made the manifest reader
// try to decrypt the user's file. Direct objects now live under
// <prefix>/uploads/<id>/files/<relpath>.
func TestDirectUpload_ManifestNamedSourceFilesRoundTrip(t *testing.T) {
	client, bucket, region := directNamespaceClient(t)
	src, prefix, uploadID := uploadDirectFixture(t, client, bucket, region, "direct-ns")
	ctx := context.Background()

	m, err := manifest.DownloadFromS3(ctx, client, bucket, prefix, uploadID)
	require.NoError(t, err)
	filesBase := fmt.Sprintf("%s/uploads/%s/files/", prefix, uploadID)
	for _, f := range m.Files {
		assert.True(t, strings.HasPrefix(f.S3Key, filesBase), "direct object %s must live under %s", f.S3Key, filesBase)
	}
	restoreAndCompare(t, client, m, src)

	// The reader that tries the encrypted names first must find no encrypted
	// manifest (none was written) and fall back to the real, plaintext one.
	m, err = manifest.DownloadFromS3WithDecryption(ctx, client, nil, bucket, prefix, uploadID)
	require.NoError(t, err)
	assert.Equal(t, uploadID, m.UploadID)
}

// TestDirectUpload_OldLayoutStillRestores proves #1020's layout change is
// read-compatible: a direct upload written before it (objects at
// <prefix>/uploads/<id>/<relpath>) restores, because readers resolve each
// file's recorded S3Key rather than rebuilding it. The old layout is made by
// moving a current upload's objects to the old keys and recording those keys.
func TestDirectUpload_OldLayoutStillRestores(t *testing.T) {
	client, bucket, region := directNamespaceClient(t)
	src, prefix, uploadID := uploadDirectFixture(t, client, bucket, region, "direct-old")
	ctx := context.Background()

	m, err := manifest.DownloadFromS3(ctx, client, bucket, prefix, uploadID)
	require.NoError(t, err)
	base := fmt.Sprintf("%s/uploads/%s/", prefix, uploadID)
	for i, f := range m.Files {
		rel, ok := strings.CutPrefix(f.S3Key, base+"files/")
		if !ok {
			rel, ok = strings.CutPrefix(f.S3Key, base)
			require.True(t, ok, "unexpected key %s", f.S3Key)
		}
		if strings.HasPrefix(rel, "manifest.") {
			continue // the old layout couldn't hold these: the manifest took the key
		}
		oldKey := base + rel
		if oldKey != f.S3Key {
			_, err := client.CopyObject(ctx, &s3.CopyObjectInput{Bucket: aws.String(bucket),
				CopySource: aws.String(bucket + "/" + f.S3Key), Key: aws.String(oldKey)})
			require.NoError(t, err)
			_, err = client.DeleteObject(ctx, &s3.DeleteObjectInput{Bucket: aws.String(bucket), Key: aws.String(f.S3Key)})
			require.NoError(t, err)
		}
		m.Files[i].S3Key = oldKey
	}
	// Keep only the files the old layout could store.
	kept := m.Files[:0]
	for _, f := range m.Files {
		if !strings.HasPrefix(filepath.Base(f.Path), "manifest.") {
			kept = append(kept, f)
		}
	}
	m.Files = kept
	require.Len(t, m.Files, 1)
	require.NoError(t, m.UploadToS3(ctx, client, true))

	old, err := manifest.DownloadFromS3(ctx, client, bucket, prefix, uploadID)
	require.NoError(t, err)
	require.Equal(t, base+"sub/data.txt", old.Files[0].S3Key)
	out := t.TempDir()
	stats, err := manifest.NewSelectiveExtractor(old, client, 0).BatchRestore(ctx, []string{old.Files[0].Path}, out)
	require.NoError(t, err)
	require.Zero(t, stats.Failed)
	got, err := os.ReadFile(indexFilesByBase(t, out)["data.txt"])
	require.NoError(t, err)
	assert.Equal(t, directNamespaceFixture["sub/data.txt"], string(got))
	_ = src
}
