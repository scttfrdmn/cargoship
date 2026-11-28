package manifest

import (
	"context"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// TestUploadToS3_RefusesEncryptedManifest pins #1001's safety net: a manifest
// marked ManifestEncrypted must never be written in plaintext, whichever caller
// reaches UploadToS3. The --dvc-auto rewrite did exactly that, leaving a
// readable manifest.json.gz next to the encrypted one. The refusal comes before
// any S3 call, so a nil client is never touched.
func TestUploadToS3_RefusesEncryptedManifest(t *testing.T) {
	m := &Manifest{Version: ManifestVersion, UploadID: "u-1", Bucket: "b", Prefix: "p",
		Encryption: &EncryptionMetadata{Enabled: true, DataKMSKeyID: "k", ManifestEncrypted: true, ManifestKMSKeyID: "k"}}
	for _, compress := range []bool{true, false} {
		err := m.UploadToS3(context.Background(), nil, compress)
		require.Error(t, err)
		assert.Contains(t, err.Error(), "plaintext")
	}

	// UploadToS3WithEncryption with no manifest key used to fall back to the
	// plaintext upload; it must fail instead.
	m.Encryption.ManifestKMSKeyID = ""
	err := m.UploadToS3WithEncryption(context.Background(), nil, nil, true)
	require.Error(t, err)
	assert.Contains(t, err.Error(), "plaintext")
}
