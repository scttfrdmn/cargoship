package manifest

import (
	"bytes"
	"context"
	"crypto/rand"
	"errors"
	"fmt"
	"sort"
	"strings"
	"testing"

	"github.com/aws/aws-sdk-go-v2/aws"
	"github.com/aws/aws-sdk-go-v2/service/kms"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// fakeKMS is an in-memory KMS that, like the real one, refuses to unwrap a data
// key under a different encryption context.
type fakeKMS struct{ keys map[string][]byte }

func ctxString(c map[string]string) string {
	var parts []string
	for k, v := range c {
		parts = append(parts, k+"="+v)
	}
	sort.Strings(parts)
	return strings.Join(parts, ",")
}

func (f *fakeKMS) GenerateDataKey(_ context.Context, in *kms.GenerateDataKeyInput, _ ...func(*kms.Options)) (*kms.GenerateDataKeyOutput, error) {
	if f.keys == nil {
		f.keys = map[string][]byte{}
	}
	plain := make([]byte, 32)
	_, _ = rand.Read(plain)
	blob := []byte(fmt.Sprintf("blob-%d", len(f.keys)))
	f.keys[string(blob)+"|"+ctxString(in.EncryptionContext)] = plain
	return &kms.GenerateDataKeyOutput{Plaintext: plain, CiphertextBlob: blob, KeyId: aws.String("arn:aws:kms:us-east-1:111122223333:key/k")}, nil
}

func (f *fakeKMS) Decrypt(_ context.Context, in *kms.DecryptInput, _ ...func(*kms.Options)) (*kms.DecryptOutput, error) {
	plain, ok := f.keys[string(in.CiphertextBlob)+"|"+ctxString(in.EncryptionContext)]
	if !ok {
		return nil, errors.New("InvalidCiphertextException: encryption context mismatch")
	}
	return &kms.DecryptOutput{Plaintext: plain}, nil
}

// TestEncryptedPartialManifest_RoundTripAndBinding pins #913's partial manifest:
// it is encrypted (file names are not readable), decrypts for its own upload, and
// is bound to a partial identity distinct from the final manifest's.
func TestEncryptedPartialManifest_RoundTripAndBinding(t *testing.T) {
	ctx := context.Background()
	k := &fakeKMS{}
	m := &Manifest{Version: ManifestVersion, UploadID: "u1", Files: []FileEntry{{Path: "very/secret/name.txt", Size: 3, S3Key: "uploads/u1/c"}}}

	data, err := m.EncryptedPartialBytes(ctx, k, "arn:aws:kms:us-east-1:111122223333:key/k")
	require.NoError(t, err)
	assert.False(t, bytes.Contains(data, []byte("very/secret")), "the compressed wrapper must not carry readable names")

	got, err := DecryptPartialManifest(ctx, k, data, "u1")
	require.NoError(t, err)
	require.Len(t, got.Files, 1)
	assert.Equal(t, "very/secret/name.txt", got.Files[0].Path)

	_, err = DecryptPartialManifest(ctx, k, data, "u2")
	assert.Error(t, err, "a partial manifest must not decrypt as another upload's")

	_, err = m.EncryptedPartialBytes(ctx, nil, "k")
	assert.Error(t, err, "no KMS client is an error, not a plaintext fallback")
	_, err = m.EncryptedPartialBytes(ctx, k, "")
	assert.Error(t, err)

	_, err = DecryptPartialManifest(ctx, k, []byte("not gzip"), "u1")
	assert.Error(t, err)
}

// TestNewBuilderFromExisting_KeepsChainAndEncryption pins #913: the resumed
// builder dropped Encryption and the chain and dataset fields.
func TestNewBuilderFromExisting_KeepsChainAndEncryption(t *testing.T) {
	enc := &EncryptionMetadata{Enabled: true, ManifestEncrypted: true, ManifestKMSKeyID: "k", DataKMSKeyID: "k"}
	b, err := NewBuilderFromExisting(&Manifest{
		UploadID: "v2", PreviousManifestID: "v1", SyncType: SyncTypeIncremental,
		DatasetID: "v1", VersionOrdinal: 2, WriterID: "nas-1", DeletedPaths: []string{"gone.txt"},
		Encryption: enc, Deduplication: &ManifestDeduplication{DuplicateFiles: 3},
	})
	require.NoError(t, err)
	m := b.Build()
	assert.Equal(t, "v1", m.PreviousManifestID)
	assert.Equal(t, SyncTypeIncremental, m.SyncType)
	assert.Equal(t, "v1", m.DatasetID)
	assert.Equal(t, 2, m.VersionOrdinal)
	assert.Equal(t, "nas-1", m.WriterID)
	assert.Equal(t, []string{"gone.txt"}, m.DeletedPaths)
	require.NotNil(t, m.Encryption)
	assert.True(t, m.Encryption.ManifestEncrypted)
	assert.NotSame(t, enc, m.Encryption, "the encryption block is copied, not shared")
	require.NotNil(t, m.Deduplication)
}
