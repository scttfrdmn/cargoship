package manifest

import (
	"bytes"
	"compress/gzip"
	"context"
	"encoding/json"
	"errors"
	"fmt"

	"github.com/aws/aws-sdk-go-v2/service/s3"
	"github.com/aws/aws-sdk-go-v2/service/s3/types"
	"github.com/aws/smithy-go"

	"github.com/scttfrdmn/cargoship/pkg/encryption"
)

const (
	// PartialManifestFileNameGZ is the plaintext partial manifest a running
	// upload saves for resume (#157).
	PartialManifestFileNameGZ = "manifest.partial.json.gz"
	// PartialEncryptedManifestFileNameGZ is the partial manifest of an upload
	// with --encrypt-manifest (#913). It used to be written in plaintext, so an
	// interrupted encrypted upload left its full file list readable in S3.
	PartialEncryptedManifestFileNameGZ = "manifest.partial.encrypted.json.gz"
)

// partialIdentity binds an encrypted partial manifest to its upload, distinctly
// from the final manifest's identity, so neither can be substituted for the other.
func partialIdentity(uploadID string) encryption.ManifestIdentity {
	return encryption.ManifestIdentity{UploadID: uploadID + "#partial"}
}

// EncryptedPartialBytes returns m as an encrypted partial manifest object
// (gzip of the encrypted-manifest wrapper), sealed under kmsKeyID (#913).
func (m *Manifest) EncryptedPartialBytes(ctx context.Context, kmsClient encryption.KMSClient, kmsKeyID string) ([]byte, error) {
	if kmsClient == nil || kmsKeyID == "" {
		return nil, fmt.Errorf("encrypt partial manifest: a KMS client and key are required")
	}
	plain, err := m.ToJSON()
	if err != nil {
		return nil, fmt.Errorf("serialize partial manifest: %w", err)
	}
	sealed, err := encryption.NewKMSEncryptor(kmsClient, kmsKeyID).EncryptManifest(ctx, plain, partialIdentity(m.UploadID))
	if err != nil {
		return nil, fmt.Errorf("encrypt partial manifest: %w", err)
	}
	wrapper, err := json.Marshal(sealed)
	if err != nil {
		return nil, fmt.Errorf("marshal encrypted partial manifest: %w", err)
	}
	var buf bytes.Buffer
	zw := gzip.NewWriter(&buf)
	if _, err := zw.Write(wrapper); err != nil {
		return nil, fmt.Errorf("compress encrypted partial manifest: %w", err)
	}
	if err := zw.Close(); err != nil {
		return nil, fmt.Errorf("compress encrypted partial manifest: %w", err)
	}
	return buf.Bytes(), nil
}

// DecryptPartialManifest parses an object written by EncryptedPartialBytes for
// uploadID.
func DecryptPartialManifest(ctx context.Context, kmsClient encryption.KMSClient, data []byte, uploadID string) (*Manifest, error) {
	zr, err := gzip.NewReader(bytes.NewReader(data))
	if err != nil {
		return nil, fmt.Errorf("decompress encrypted partial manifest: %w", err)
	}
	defer func() { _ = zr.Close() }()
	wrapper, err := readAllLimited(zr, MaxManifestJSONBytes, "decompressed encrypted partial manifest")
	if err != nil {
		return nil, err
	}
	var sealed encryption.EncryptedManifest
	if err := json.Unmarshal(wrapper, &sealed); err != nil {
		return nil, fmt.Errorf("parse encrypted partial manifest: %w", err)
	}
	plain, err := encryption.DecryptManifestBytes(ctx, kmsClient, &sealed, partialIdentity(uploadID))
	if err != nil {
		return nil, fmt.Errorf("decrypt partial manifest: %w", err)
	}
	return parseManifestJSON(plain)
}

// DownloadPartialManifestFromS3WithDecryption loads the partial manifest of an
// upload that used --encrypt-manifest. It reads the encrypted partial, and falls
// back to the plaintext one only when no encrypted partial exists (one written
// before #913). A partial that exists but can't be decrypted is an error, never
// a silent fallback.
func DownloadPartialManifestFromS3WithDecryption(ctx context.Context, s3Client *s3.Client, kmsClient encryption.KMSClient, bucket, prefix, uploadID string) (*Manifest, error) {
	key := fmt.Sprintf("%s/uploads/%s/%s", prefix, uploadID, PartialEncryptedManifestFileNameGZ)
	data, err := downloadS3Object(ctx, s3Client, bucket, key)
	if err == nil {
		return DecryptPartialManifest(ctx, kmsClient, data, uploadID)
	}
	if !isNotFound(err) {
		return nil, fmt.Errorf("download encrypted partial manifest: %w", err)
	}
	return DownloadPartialManifestFromS3(ctx, s3Client, bucket, prefix, uploadID)
}

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
