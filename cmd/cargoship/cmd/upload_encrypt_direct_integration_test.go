//go:build integration

package cmd

import (
	"context"
	"strings"
	"testing"

	"github.com/aws/aws-sdk-go-v2/aws"
	"github.com/aws/aws-sdk-go-v2/service/s3"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/scttfrdmn/cargoship/pkg/manifest"
)

// TestUpload_EncryptManifestHidesFileNames pins #1037. A small tree takes the
// direct-upload fast path by default, which stores each file as its own object
// keyed by its path and puts the path in cargoship-file-path metadata, so under
// --encrypt-manifest (the option for sensitive file names) every name was still
// listable. With --encrypt-manifest every file goes into chunks, and no object
// key or metadata value carries a source path.
func TestUpload_EncryptManifestHidesFileNames(t *testing.T) {
	e := newUploadEmulator(t)
	ctx := context.Background()
	key := e.newKey(t)
	src := writeSourceTree(t, map[string]string{
		"secret-filename.txt":            "cargoship-test secret\n",
		"patients/jane-doe/scan-001.dcm": "cargoship-test scan\n",
	})
	const prefix = "enc-direct"

	_, err := e.upload(t, src, prefix, "--kms-key-id", key, "--encrypt-manifest")
	require.NoError(t, err)

	keys := e.keys(t, prefix)
	require.NotEmpty(t, keys)
	for _, k := range keys {
		for _, name := range []string{"secret-filename", "jane-doe", "scan-001", "patients"} {
			assert.NotContains(t, k, name, "object key %s reveals a source path", k)
		}
		h, err := e.s3.HeadObject(ctx, &s3.HeadObjectInput{Bucket: aws.String(e.bucket), Key: aws.String(k)})
		require.NoError(t, err)
		for mk, mv := range h.Metadata {
			assert.NotContains(t, mv, src, "metadata %s=%s on %s reveals a source path", mk, mv, k)
		}
	}

	id := uploadIDFromKeys(t, prefix, keys)
	m, err := manifest.DownloadFromS3WithDecryption(ctx, e.s3, e.kms, e.bucket, prefix, id)
	require.NoError(t, err)
	assert.Len(t, m.Files, 2)
	assert.Positive(t, m.TotalChunks, "under --encrypt-manifest the files must be packed into chunks")
	for _, f := range m.Files {
		assert.True(t, strings.HasSuffix(f.S3Key, ".tar.zst") || strings.HasSuffix(f.S3Key, ".tar"),
			"%s must be stored in a chunk, not as its own object (%s)", f.Path, f.S3Key)
	}
}

// TestUpload_EncryptManifestRefusesDirectFlags: asking for direct mode and
// --encrypt-manifest together is contradictory, so it is a usage error rather
// than one of the two being silently ignored (#1037).
func TestUpload_EncryptManifestRefusesDirectFlags(t *testing.T) {
	e := newUploadEmulator(t)
	key := e.newKey(t)
	src := writeSourceTree(t, map[string]string{"a.txt": "cargoship-test a\n"})
	for _, flag := range []string{"--direct-upload", "--force-direct-upload"} {
		_, err := e.upload(t, src, "enc-direct-flags", "--kms-key-id", key, "--encrypt-manifest", flag)
		require.Error(t, err, flag)
		assert.ErrorIs(t, err, ErrUsage, flag)
	}
	assert.Empty(t, e.keys(t, "enc-direct-flags"))
}
