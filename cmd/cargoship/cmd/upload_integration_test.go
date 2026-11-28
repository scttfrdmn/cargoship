//go:build integration

package cmd

import (
	"bytes"
	"context"
	"fmt"
	"os"
	"path/filepath"
	"strings"
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

// uploadEmulator is an emulator bucket plus clients for driving the real
// `upload` command in-process.
type uploadEmulator struct {
	s3     *s3.Client
	kms    *kms.Client
	bucket string
}

// newUploadEmulator points the CLI's AWS config at the emulator for this test
// and creates a fresh bucket.
func newUploadEmulator(t *testing.T) *uploadEmulator {
	t.Helper()
	if substrateURL == "" {
		t.Skip("needs the emulator: these tests drive the upload command against an in-process S3/KMS")
	}
	t.Setenv("AWS_ENDPOINT_URL", substrateURL)
	t.Setenv("AWS_ACCESS_KEY_ID", "test")
	t.Setenv("AWS_SECRET_ACCESS_KEY", "test")
	t.Setenv("AWS_REGION", testRegion)
	cfg := aws.Config{Region: testRegion, BaseEndpoint: aws.String(substrateURL),
		Credentials: credentials.NewStaticCredentialsProvider("test", "test", "")}
	e := &uploadEmulator{
		s3:     s3.NewFromConfig(cfg, func(o *s3.Options) { o.UsePathStyle = true }),
		kms:    kms.NewFromConfig(cfg),
		bucket: fmt.Sprintf("upload-cmd-%d", time.Now().UnixNano()),
	}
	_, err := e.s3.CreateBucket(context.Background(), &s3.CreateBucketInput{Bucket: aws.String(e.bucket)})
	require.NoError(t, err)
	return e
}

// newKey creates an emulator KMS key and returns its ARN.
func (e *uploadEmulator) newKey(t *testing.T) string {
	t.Helper()
	out, err := e.kms.CreateKey(context.Background(), &kms.CreateKeyInput{})
	require.NoError(t, err)
	return aws.ToString(out.KeyMetadata.Arn)
}

// upload runs `cargoship upload SRC s3://bucket/prefix ARGS...` in-process and
// returns what the command wrote through cobra (its warnings).
func (e *uploadEmulator) upload(t *testing.T, src, prefix string, args ...string) (string, error) {
	t.Helper()
	var out bytes.Buffer
	c := NewUploadCmd()
	c.SetArgs(append([]string{src, "s3://" + e.bucket + "/" + prefix,
		"--region", testRegion, "--quiet", "--ignore-budget"}, args...))
	c.SetOut(&out)
	c.SetErr(&out)
	c.SilenceUsage = true
	err := c.Execute()
	return out.String(), err
}

// keys lists every object key under prefix.
func (e *uploadEmulator) keys(t *testing.T, prefix string) []string {
	t.Helper()
	var keys []string
	p := s3.NewListObjectsV2Paginator(e.s3, &s3.ListObjectsV2Input{Bucket: aws.String(e.bucket), Prefix: aws.String(prefix + "/")})
	for p.HasMorePages() {
		page, err := p.NextPage(context.Background())
		require.NoError(t, err)
		for _, o := range page.Contents {
			keys = append(keys, aws.ToString(o.Key))
		}
	}
	return keys
}

// writeSourceTree writes files (relative path -> content) under a temp dir.
func writeSourceTree(t *testing.T, files map[string]string) string {
	t.Helper()
	src := t.TempDir()
	for rel, body := range files {
		p := filepath.Join(src, filepath.FromSlash(rel))
		require.NoError(t, os.MkdirAll(filepath.Dir(p), 0o750))
		require.NoError(t, os.WriteFile(p, []byte(body), 0o600))
	}
	return src
}

// uploadIDFromKeys returns the single upload ID under prefix/uploads/.
func uploadIDFromKeys(t *testing.T, prefix string, keys []string) string {
	t.Helper()
	ids := map[string]bool{}
	for _, k := range keys {
		rest, ok := strings.CutPrefix(k, prefix+"/uploads/")
		if !ok {
			continue
		}
		if id, _, ok := strings.Cut(rest, "/"); ok {
			ids[id] = true
		}
	}
	require.Len(t, ids, 1, "want exactly one upload under %s: %v", prefix, keys)
	for id := range ids {
		return id
	}
	return ""
}

// TestUpload_EncryptManifestDVCAutoWritesNoPlaintextManifest pins #1001.
// --dvc-auto annotated the finalized manifest and re-uploaded it with the
// plaintext UploadToS3, whatever --encrypt-manifest said, so an encrypted upload
// ended with manifest.encrypted.json.gz AND a plaintext manifest.json.gz that
// listed every path, readable without KMS.
func TestUpload_EncryptManifestDVCAutoWritesNoPlaintextManifest(t *testing.T) {
	e := newUploadEmulator(t)
	key := e.newKey(t)
	src := writeSourceTree(t, map[string]string{
		"dvc.yaml":                     "stages:\n  prepare:\n    cmd: echo prepare\n    outs:\n      - data\n",
		"data/secret-name-0.txt":       "cargoship-test secret 0\n",
		"data/secret-name-1.txt":       "cargoship-test secret 1\n",
		"notes/also-sensitive-name.md": "cargoship-test notes\n",
	})
	prefix := "enc-dvc"

	out, err := e.upload(t, src, prefix, "--kms-key-id", key, "--encrypt-manifest", "--dvc-auto")
	require.NoError(t, err)
	assert.NotContains(t, out, "re-upload failed", "the --dvc-auto rewrite must succeed, encrypted")

	keys := e.keys(t, prefix)
	var encrypted int
	for _, k := range keys {
		base := k[strings.LastIndex(k, "/")+1:]
		assert.NotContains(t, []string{manifest.ManifestFileNameGZ, manifest.ManifestFileName}, base,
			"%s is a plaintext manifest under --encrypt-manifest", k)
		if strings.HasPrefix(base, "manifest.encrypted.json") {
			encrypted++
		}
	}
	require.Equal(t, 1, encrypted, "the encrypted manifest must exist: %v", keys)

	id := uploadIDFromKeys(t, prefix, keys)
	m, err := manifest.DownloadFromS3WithDecryption(context.Background(), e.s3, e.kms, e.bucket, prefix, id)
	require.NoError(t, err)
	assert.Len(t, m.Files, 4)
}

// TestRewriteFinalManifest pins the rewrite half of #1001: a post-upload edit of
// an --encrypt-manifest upload's manifest is written encrypted (and decrypts to
// the edit), or not at all; a plaintext upload's is written in plaintext.
func TestRewriteFinalManifest(t *testing.T) {
	e := newUploadEmulator(t)
	ctx := context.Background()
	key := e.newKey(t)
	newManifest := func(prefix string) *manifest.Manifest {
		return &manifest.Manifest{Version: manifest.ManifestVersion, UploadID: "u-1", Bucket: e.bucket, Prefix: prefix,
			Files: []manifest.FileEntry{{Path: "/src/secret-name.txt", Size: 1}}}
	}
	annotate := func(m *manifest.Manifest) { m.Files[0].DVCMetadata = &manifest.DVCMetadata{Stage: "prepare"} }

	t.Run("encrypted upload is rewritten encrypted", func(t *testing.T) {
		m := newManifest("rw-enc")
		m.Encryption = &manifest.EncryptionMetadata{Enabled: true, DataKMSKeyID: key, ManifestEncrypted: true, ManifestKMSKeyID: key}
		annotate(m)
		require.NoError(t, rewriteFinalManifest(ctx, e.s3, e.kms, m, true))
		assert.Equal(t, []string{"rw-enc/uploads/u-1/manifest.encrypted.json.gz"}, e.keys(t, "rw-enc"))
		got, err := manifest.DownloadFromS3WithDecryption(ctx, e.s3, e.kms, e.bucket, "rw-enc", "u-1")
		require.NoError(t, err)
		require.NotNil(t, got.Files[0].DVCMetadata)
		assert.Equal(t, "prepare", got.Files[0].DVCMetadata.Stage)
	})

	t.Run("encrypted upload without a KMS client is refused, not written in plaintext", func(t *testing.T) {
		m := newManifest("rw-nokms")
		m.Encryption = &manifest.EncryptionMetadata{Enabled: true, DataKMSKeyID: key, ManifestEncrypted: true, ManifestKMSKeyID: key}
		require.Error(t, rewriteFinalManifest(ctx, e.s3, nil, m, true))
		assert.Empty(t, e.keys(t, "rw-nokms"))
	})

	t.Run("plaintext upload is rewritten in plaintext", func(t *testing.T) {
		m := newManifest("rw-plain")
		annotate(m)
		require.NoError(t, rewriteFinalManifest(ctx, e.s3, nil, m, false))
		assert.Equal(t, []string{"rw-plain/uploads/u-1/manifest.json.gz"}, e.keys(t, "rw-plain"))
	})
}
