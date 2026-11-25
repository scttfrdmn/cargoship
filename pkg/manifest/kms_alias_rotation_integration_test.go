//go:build integration

package manifest

import (
	"bytes"
	"compress/gzip"
	"context"
	"encoding/json"
	"fmt"
	"strings"
	"testing"
	"time"

	"github.com/aws/aws-sdk-go-v2/aws"
	"github.com/aws/aws-sdk-go-v2/config"
	"github.com/aws/aws-sdk-go-v2/service/kms"
	"github.com/aws/aws-sdk-go-v2/service/s3"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/scttfrdmn/cargoship/pkg/encryption"
)

func kmsTestClients(t *testing.T) (*s3.Client, *kms.Client) {
	t.Helper()
	cfg, err := config.LoadDefaultConfig(context.Background(), config.WithRegion("us-east-1"))
	require.NoError(t, err)
	return s3.NewFromConfig(cfg, func(o *s3.Options) { o.UsePathStyle = substrateURL != "" }), kms.NewFromConfig(cfg)
}

// TestEncryptedManifest_ReadableAfterAliasRotation pins #912. The manifest stored
// the alias the user passed (--kms-key-id alias/x) and Decrypt named it. After
// the alias was re-pointed to a new key (AWS's manual rotation), KMS refused, and
// the failure surfaced as NoSuchKey ("manifest missing") for an intact archive.
func TestEncryptedManifest_ReadableAfterAliasRotation(t *testing.T) {
	ctx := context.Background()
	s3c, kc := kmsTestClients(t)
	k1, err := kc.CreateKey(ctx, &kms.CreateKeyInput{})
	require.NoError(t, err)
	k2, err := kc.CreateKey(ctx, &kms.CreateKeyInput{})
	require.NoError(t, err)
	alias := fmt.Sprintf("alias/cargoship-rotate-%d", time.Now().UnixNano())
	_, err = kc.CreateAlias(ctx, &kms.CreateAliasInput{AliasName: aws.String(alias), TargetKeyId: k1.KeyMetadata.KeyId})
	require.NoError(t, err)

	prefix := fmt.Sprintf("alias-rotation-%d", time.Now().UnixNano())
	m := &Manifest{
		Version: ManifestVersion, UploadID: "u-rotate", Bucket: integTestBucket, Prefix: prefix,
		Files:      []FileEntry{{Path: "a.txt", Size: 1, S3Key: "uploads/u-rotate/a.txt"}},
		Encryption: &EncryptionMetadata{Enabled: true, ManifestEncrypted: true, ManifestKMSKeyID: alias},
	}
	require.NoError(t, m.UploadToS3WithEncryption(ctx, s3c, kc, true))

	// The stored key is the key KMS used, not the alias.
	sealed := readSealed(t, s3c, prefix, "u-rotate")
	assert.Equal(t, aws.ToString(k1.KeyMetadata.Arn), sealed.KMSKeyID, "the manifest must record the key ARN")

	// Rotate: point the alias at a new key.
	_, err = kc.UpdateAlias(ctx, &kms.UpdateAliasInput{AliasName: aws.String(alias), TargetKeyId: k2.KeyMetadata.KeyId})
	require.NoError(t, err)

	got, err := DownloadFromS3WithDecryption(ctx, s3c, kc, integTestBucket, prefix, "u-rotate")
	require.NoError(t, err, "a manifest must stay readable after its alias is re-pointed")
	assert.Equal(t, "a.txt", got.Files[0].Path)
}

// TestEncryptedManifest_LegacyAliasRecordStillDecrypts covers manifests written
// before #912, which recorded the alias. Decrypt omits an alias KeyId, since a
// symmetric ciphertext identifies its own key.
func TestEncryptedManifest_LegacyAliasRecordStillDecrypts(t *testing.T) {
	ctx := context.Background()
	_, kc := kmsTestClients(t)
	k1, err := kc.CreateKey(ctx, &kms.CreateKeyInput{})
	require.NoError(t, err)
	k2, err := kc.CreateKey(ctx, &kms.CreateKeyInput{})
	require.NoError(t, err)
	alias := fmt.Sprintf("alias/cargoship-legacy-%d", time.Now().UnixNano())
	_, err = kc.CreateAlias(ctx, &kms.CreateAliasInput{AliasName: aws.String(alias), TargetKeyId: k1.KeyMetadata.KeyId})
	require.NoError(t, err)

	sealed, err := encryption.EncryptManifestBytes(ctx, kc, alias, []byte(`{"version":"2.0","upload_id":"u-legacy","files":[]}`),
		encryption.ManifestIdentity{UploadID: "u-legacy"})
	require.NoError(t, err)
	sealed.KMSKeyID = alias // what a pre-#912 writer stored
	_, err = kc.UpdateAlias(ctx, &kms.UpdateAliasInput{AliasName: aws.String(alias), TargetKeyId: k2.KeyMetadata.KeyId})
	require.NoError(t, err)

	plain, err := encryption.DecryptManifestBytes(ctx, kc, sealed, encryption.ManifestIdentity{UploadID: "u-legacy"})
	require.NoError(t, err)
	assert.Contains(t, string(plain), "u-legacy")
}

// TestEncryptedManifest_UnreadableIsNotReportedMissing: an encrypted manifest
// that exists but can't be decrypted must report that, not NoSuchKey.
func TestEncryptedManifest_UnreadableIsNotReportedMissing(t *testing.T) {
	ctx := context.Background()
	s3c, kc := kmsTestClients(t)
	prefix := fmt.Sprintf("unreadable-%d", time.Now().UnixNano())
	bogus, err := json.Marshal(encryption.EncryptedManifest{Algorithm: "AES-256-GCM", EncryptedDEK: "bm90LWEta2V5", IV: "AAAA", EncryptedData: "AAAA", BindingVersion: 1})
	require.NoError(t, err)
	var buf bytes.Buffer
	zw := gzip.NewWriter(&buf)
	_, _ = zw.Write(bogus)
	require.NoError(t, zw.Close())
	_, err = s3c.PutObject(ctx, &s3.PutObjectInput{
		Bucket: aws.String(integTestBucket), Key: aws.String(prefix + "/uploads/u-bad/manifest.encrypted.json.gz"), Body: bytes.NewReader(buf.Bytes()),
	})
	require.NoError(t, err)

	_, err = DownloadFromS3WithDecryption(ctx, s3c, kc, integTestBucket, prefix, "u-bad")
	require.Error(t, err)
	assert.Contains(t, err.Error(), "exists but could not be read")
	assert.False(t, strings.Contains(err.Error(), "NoSuchKey"), "must not be reported as a missing manifest: %v", err)
}

func readSealed(t *testing.T, s3c *s3.Client, prefix, uploadID string) *encryption.EncryptedManifest {
	t.Helper()
	obj, err := s3c.GetObject(context.Background(), &s3.GetObjectInput{
		Bucket: aws.String(integTestBucket), Key: aws.String(prefix + "/uploads/" + uploadID + "/manifest.encrypted.json.gz"),
	})
	require.NoError(t, err)
	defer func() { _ = obj.Body.Close() }()
	zr, err := gzip.NewReader(obj.Body)
	require.NoError(t, err)
	var sealed encryption.EncryptedManifest
	require.NoError(t, json.NewDecoder(zr).Decode(&sealed))
	return &sealed
}

// TestDownloadFromS3_CorruptManifestIsNotReportedMissing pins #919's reader half:
// a manifest.json.gz that exists but can't be parsed fell through to the
// uncompressed name and reported NoSuchKey, so a damaged manifest looked absent
// and the delete chain guard skipped that version.
func TestDownloadFromS3_CorruptManifestIsNotReportedMissing(t *testing.T) {
	ctx := context.Background()
	s3c, _ := kmsTestClients(t)
	prefix := fmt.Sprintf("corrupt-%d", time.Now().UnixNano())
	_, err := s3c.PutObject(ctx, &s3.PutObjectInput{
		Bucket: aws.String(integTestBucket), Key: aws.String(prefix + "/uploads/u-x/manifest.json.gz"),
		Body: bytes.NewReader([]byte("not gzip")),
	})
	require.NoError(t, err)

	_, err = DownloadFromS3(ctx, s3c, integTestBucket, prefix, "u-x")
	require.Error(t, err)
	assert.False(t, IsNotFound(err), "a corrupt manifest must not read as missing: %v", err)
	assert.Contains(t, err.Error(), "could not be parsed")

	_, err = DownloadFromS3(ctx, s3c, integTestBucket, prefix, "u-absent")
	assert.True(t, IsNotFound(err), "an absent manifest is still not-found: %v", err)
}
