package encryption

import (
	"context"
	"crypto/aes"
	"crypto/cipher"
	"encoding/base64"
	"testing"

	"github.com/aws/aws-sdk-go-v2/service/kms"
	"github.com/aws/aws-sdk-go-v2/service/kms/types"
)

// MockKMSClient implements a mock KMS client for testing. It records the
// EncryptionContext passed to GenerateDataKey and Decrypt (CSH-SEC-004) so tests
// can assert the KMS binding layer is wired; the GCM AAD layer (real crypto)
// independently enforces substitution rejection.
type MockKMSClient struct {
	generateDataKeyFunc func(ctx context.Context, params *kms.GenerateDataKeyInput, optFns ...func(*kms.Options)) (*kms.GenerateDataKeyOutput, error)
	decryptFunc         func(ctx context.Context, params *kms.DecryptInput, optFns ...func(*kms.Options)) (*kms.DecryptOutput, error)

	lastGenContext     map[string]string
	lastDecryptContext map[string]string
}

func (m *MockKMSClient) GenerateDataKey(ctx context.Context, params *kms.GenerateDataKeyInput, optFns ...func(*kms.Options)) (*kms.GenerateDataKeyOutput, error) {
	m.lastGenContext = params.EncryptionContext
	if m.generateDataKeyFunc != nil {
		return m.generateDataKeyFunc(ctx, params, optFns...)
	}
	// Default: return a mock 256-bit key
	plaintextKey := make([]byte, 32) // 256 bits
	for i := range plaintextKey {
		plaintextKey[i] = byte(i)
	}
	encryptedKey := append([]byte("encrypted:"), plaintextKey...)
	return &kms.GenerateDataKeyOutput{
		Plaintext:      plaintextKey,
		CiphertextBlob: encryptedKey,
		KeyId:          params.KeyId,
	}, nil
}

func (m *MockKMSClient) Decrypt(ctx context.Context, params *kms.DecryptInput, optFns ...func(*kms.Options)) (*kms.DecryptOutput, error) {
	m.lastDecryptContext = params.EncryptionContext
	if m.decryptFunc != nil {
		return m.decryptFunc(ctx, params, optFns...)
	}
	// Default: extract the plaintext from the mock encrypted format
	// Assumes the format "encrypted:<plaintext>"
	if len(params.CiphertextBlob) > 10 {
		plaintextKey := params.CiphertextBlob[10:] // Skip "encrypted:" prefix
		return &kms.DecryptOutput{
			Plaintext: plaintextKey,
			KeyId:     params.KeyId,
		}, nil
	}
	return nil, &types.InvalidCiphertextException{Message: stringPtr("invalid ciphertext")}
}

func stringPtr(s string) *string {
	return &s
}

func TestEncryptDecryptManifest(t *testing.T) {
	ctx := context.Background()
	mockKMS := &MockKMSClient{}
	kmsKeyID := "arn:aws:kms:us-west-2:123456789012:key/12345678-1234-1234-1234-123456789012"

	encryptor := NewKMSEncryptor(mockKMS, kmsKeyID)

	// Test data
	manifestJSON := []byte(`{"version":"1.0","upload_id":"test-123","files":[]}`)

	// Encrypt
	id := ManifestIdentity{UploadID: "test-123"}
	encrypted, err := encryptor.EncryptManifest(ctx, manifestJSON, id)
	if err != nil {
		t.Fatalf("EncryptManifest failed: %v", err)
	}

	// Verify encrypted manifest structure
	if encrypted.Algorithm != "AES-256-GCM" {
		t.Errorf("Expected algorithm AES-256-GCM, got %s", encrypted.Algorithm)
	}
	if encrypted.KMSKeyID != kmsKeyID {
		t.Errorf("Expected KMS key ID %s, got %s", kmsKeyID, encrypted.KMSKeyID)
	}
	if encrypted.EncryptedDEK == "" {
		t.Error("EncryptedDEK is empty")
	}
	if encrypted.IV == "" {
		t.Error("IV is empty")
	}
	if encrypted.EncryptedData == "" {
		t.Error("EncryptedData is empty")
	}

	// Verify base64 encoding
	if _, err := base64.StdEncoding.DecodeString(encrypted.EncryptedDEK); err != nil {
		t.Errorf("EncryptedDEK is not valid base64: %v", err)
	}
	if _, err := base64.StdEncoding.DecodeString(encrypted.IV); err != nil {
		t.Errorf("IV is not valid base64: %v", err)
	}
	if _, err := base64.StdEncoding.DecodeString(encrypted.EncryptedData); err != nil {
		t.Errorf("EncryptedData is not valid base64: %v", err)
	}

	// Decrypt
	decrypted, err := encryptor.DecryptManifest(ctx, encrypted, id)
	if err != nil {
		t.Fatalf("DecryptManifest failed: %v", err)
	}

	// Verify decrypted data matches original
	if string(decrypted) != string(manifestJSON) {
		t.Errorf("Decrypted manifest does not match original.\nOriginal:  %s\nDecrypted: %s", manifestJSON, decrypted)
	}
}

func TestEncryptManifestWithEmptyData(t *testing.T) {
	ctx := context.Background()
	mockKMS := &MockKMSClient{}
	kmsKeyID := "test-key"

	encryptor := NewKMSEncryptor(mockKMS, kmsKeyID)

	// Empty manifest
	manifestJSON := []byte("")

	id := ManifestIdentity{UploadID: "empty-1"}
	encrypted, err := encryptor.EncryptManifest(ctx, manifestJSON, id)
	if err != nil {
		t.Fatalf("EncryptManifest with empty data failed: %v", err)
	}

	// Decrypt
	decrypted, err := encryptor.DecryptManifest(ctx, encrypted, id)
	if err != nil {
		t.Fatalf("DecryptManifest with empty data failed: %v", err)
	}

	if string(decrypted) != "" {
		t.Errorf("Expected empty decrypted data, got: %s", decrypted)
	}
}

func TestDecryptManifestWithInvalidAlgorithm(t *testing.T) {
	ctx := context.Background()
	mockKMS := &MockKMSClient{}

	encryptor := NewKMSEncryptor(mockKMS, "test-key")

	encrypted := &EncryptedManifest{
		Algorithm:     "INVALID-ALGORITHM",
		KMSKeyID:      "test-key",
		EncryptedDEK:  "dGVzdA==",
		IV:            "dGVzdA==",
		EncryptedData: "dGVzdA==",
	}

	_, err := encryptor.DecryptManifest(ctx, encrypted, ManifestIdentity{})
	if err == nil {
		t.Error("Expected error for invalid algorithm, got nil")
	}
	if err != nil && err.Error() != "unsupported encryption algorithm: INVALID-ALGORITHM (expected AES-256-GCM)" {
		t.Errorf("Unexpected error message: %v", err)
	}
}

func TestDecryptManifestWithInvalidBase64(t *testing.T) {
	ctx := context.Background()
	mockKMS := &MockKMSClient{}

	encryptor := NewKMSEncryptor(mockKMS, "test-key")

	tests := []struct {
		name      string
		encrypted *EncryptedManifest
		errMsg    string
	}{
		{
			name: "Invalid EncryptedDEK",
			encrypted: &EncryptedManifest{
				Algorithm:     "AES-256-GCM",
				KMSKeyID:      "test-key",
				EncryptedDEK:  "not-valid-base64!!!",
				IV:            "dGVzdA==",
				EncryptedData: "dGVzdA==",
			},
			errMsg: "failed to decode encrypted DEK",
		},
		{
			name: "Invalid IV",
			encrypted: &EncryptedManifest{
				Algorithm:     "AES-256-GCM",
				KMSKeyID:      "test-key",
				EncryptedDEK:  "dGVzdA==",
				IV:            "not-valid-base64!!!",
				EncryptedData: "dGVzdA==",
			},
			errMsg: "failed to decode IV",
		},
		{
			name: "Invalid EncryptedData",
			encrypted: &EncryptedManifest{
				Algorithm:     "AES-256-GCM",
				KMSKeyID:      "test-key",
				EncryptedDEK:  "dGVzdA==",
				IV:            "dGVzdA==",
				EncryptedData: "not-valid-base64!!!",
			},
			errMsg: "failed to decode encrypted data",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			_, err := encryptor.DecryptManifest(ctx, tt.encrypted, ManifestIdentity{})
			if err == nil {
				t.Error("Expected error for invalid base64, got nil")
			}
			if err != nil && len(tt.errMsg) > 0 {
				// Check that error message contains expected substring
				if len(err.Error()) < len(tt.errMsg) || err.Error()[:len(tt.errMsg)] != tt.errMsg {
					t.Errorf("Expected error containing %q, got %q", tt.errMsg, err.Error())
				}
			}
		})
	}
}

func TestConvenienceFunctions(t *testing.T) {
	ctx := context.Background()
	mockKMS := &MockKMSClient{}
	kmsKeyID := "test-key"

	manifestJSON := []byte(`{"test":"data"}`)

	// Test EncryptManifestBytes
	id := ManifestIdentity{UploadID: "conv-1"}
	encrypted, err := EncryptManifestBytes(ctx, mockKMS, kmsKeyID, manifestJSON, id)
	if err != nil {
		t.Fatalf("EncryptManifestBytes failed: %v", err)
	}

	// Test DecryptManifestBytes
	decrypted, err := DecryptManifestBytes(ctx, mockKMS, encrypted, id)
	if err != nil {
		t.Fatalf("DecryptManifestBytes failed: %v", err)
	}

	if string(decrypted) != string(manifestJSON) {
		t.Errorf("Decrypted data does not match original.\nOriginal:  %s\nDecrypted: %s", manifestJSON, decrypted)
	}
}

func TestEncryptDecryptLargeManifest(t *testing.T) {
	ctx := context.Background()
	mockKMS := &MockKMSClient{}
	kmsKeyID := "test-key"

	encryptor := NewKMSEncryptor(mockKMS, kmsKeyID)

	// Create a large manifest (1 MB)
	largeData := make([]byte, 1024*1024)
	for i := range largeData {
		largeData[i] = byte(i % 256)
	}

	// Encrypt
	id := ManifestIdentity{UploadID: "large-1"}
	encrypted, err := encryptor.EncryptManifest(ctx, largeData, id)
	if err != nil {
		t.Fatalf("EncryptManifest with large data failed: %v", err)
	}

	// Decrypt
	decrypted, err := encryptor.DecryptManifest(ctx, encrypted, id)
	if err != nil {
		t.Fatalf("DecryptManifest with large data failed: %v", err)
	}

	// Verify
	if len(decrypted) != len(largeData) {
		t.Errorf("Decrypted data length mismatch. Expected %d, got %d", len(largeData), len(decrypted))
	}

	// Check first and last bytes
	if decrypted[0] != largeData[0] {
		t.Errorf("First byte mismatch. Expected %d, got %d", largeData[0], decrypted[0])
	}
	if decrypted[len(decrypted)-1] != largeData[len(largeData)-1] {
		t.Errorf("Last byte mismatch. Expected %d, got %d", largeData[len(largeData)-1], decrypted[len(decrypted)-1])
	}
}

// --- CSH-SEC-004: identity binding ---

// TestEncryptManifest_BindsIdentity confirms a newly-encrypted manifest is marked
// bound and that the upload identity reached the KMS EncryptionContext.
func TestEncryptManifest_BindsIdentity(t *testing.T) {
	ctx := context.Background()
	mockKMS := &MockKMSClient{}
	enc := NewKMSEncryptor(mockKMS, "k")

	m, err := enc.EncryptManifest(ctx, []byte(`{"x":1}`), ManifestIdentity{UploadID: "A"})
	if err != nil {
		t.Fatalf("encrypt: %v", err)
	}
	if m.BindingVersion != CurrentBindingVersion {
		t.Errorf("BindingVersion = %d, want %d", m.BindingVersion, CurrentBindingVersion)
	}
	if got := mockKMS.lastGenContext["upload_id"]; got != "A" {
		t.Errorf("GenerateDataKey EncryptionContext upload_id = %q, want A", got)
	}
	if mockKMS.lastGenContext["application"] != "cargoship" || mockKMS.lastGenContext["purpose"] != "manifest" {
		t.Errorf("EncryptionContext missing application/purpose: %v", mockKMS.lastGenContext)
	}
}

// TestDecryptManifest_RejectsSubstitution is the CSH-SEC-004 regression: a
// manifest bound to upload A must NOT decrypt under a different expected upload
// B. Enforced here by the GCM AAD layer (real crypto); real KMS additionally
// refuses the DEK unwrap on the mismatched EncryptionContext.
func TestDecryptManifest_RejectsSubstitution(t *testing.T) {
	ctx := context.Background()
	mockKMS := &MockKMSClient{}
	enc := NewKMSEncryptor(mockKMS, "k")

	m, err := enc.EncryptManifest(ctx, []byte(`{"secret":"A"}`), ManifestIdentity{UploadID: "A"})
	if err != nil {
		t.Fatalf("encrypt: %v", err)
	}

	// Correct identity decrypts.
	if _, err := enc.DecryptManifest(ctx, m, ManifestIdentity{UploadID: "A"}); err != nil {
		t.Fatalf("decrypt with correct identity should succeed: %v", err)
	}
	// A different upload's identity must fail.
	if _, err := enc.DecryptManifest(ctx, m, ManifestIdentity{UploadID: "B"}); err == nil {
		t.Fatal("decrypt under a different upload identity must fail (substitution)")
	}
	// The decrypt attempt must have carried the EXPECTED identity into KMS, not
	// the ciphertext's.
	if got := mockKMS.lastDecryptContext["upload_id"]; got != "B" {
		t.Errorf("Decrypt EncryptionContext upload_id = %q, want B (the expected identity)", got)
	}
}

// TestDecryptManifest_FailsClosedWithoutIdentity confirms a bound manifest is
// refused when the caller cannot supply the expected upload identity.
func TestDecryptManifest_FailsClosedWithoutIdentity(t *testing.T) {
	ctx := context.Background()
	mockKMS := &MockKMSClient{}
	enc := NewKMSEncryptor(mockKMS, "k")

	m, err := enc.EncryptManifest(ctx, []byte(`{}`), ManifestIdentity{UploadID: "A"})
	if err != nil {
		t.Fatalf("encrypt: %v", err)
	}
	if _, err := enc.DecryptManifest(ctx, m, ManifestIdentity{}); err == nil {
		t.Fatal("a bound manifest with no expected identity must fail closed")
	}
}

// TestDecryptManifest_ReadsLegacyUnbound confirms backward read support: an
// archive written before the binding scheme (BindingVersion 0, nil AAD/context)
// still decrypts regardless of the expected identity.
func TestDecryptManifest_ReadsLegacyUnbound(t *testing.T) {
	ctx := context.Background()
	mockKMS := &MockKMSClient{}
	enc := NewKMSEncryptor(mockKMS, "k")
	plaintext := []byte(`{"legacy":true}`)

	legacy := legacyEncrypted(t, mockKMS, "k", plaintext)
	if legacy.BindingVersion != 0 {
		t.Fatalf("fixture should be unbound, got BindingVersion=%d", legacy.BindingVersion)
	}
	// Even with a (non-matching) expected identity, an unbound manifest reads.
	got, err := enc.DecryptManifest(ctx, legacy, ManifestIdentity{UploadID: "whatever"})
	if err != nil {
		t.Fatalf("legacy unbound manifest must still decrypt: %v", err)
	}
	if string(got) != string(plaintext) {
		t.Errorf("legacy decrypt = %q, want %q", got, plaintext)
	}
}

// legacyEncrypted reconstructs a pre-CSH-SEC-004 (unbound) EncryptedManifest:
// DEK from KMS with no EncryptionContext, AES-GCM with nil AAD, BindingVersion 0.
func legacyEncrypted(t *testing.T, mockKMS *MockKMSClient, keyID string, plaintext []byte) *EncryptedManifest {
	t.Helper()
	out, err := mockKMS.GenerateDataKey(context.Background(), &kms.GenerateDataKeyInput{
		KeyId: &keyID, KeySpec: "AES_256", // no EncryptionContext
	})
	if err != nil {
		t.Fatalf("gen key: %v", err)
	}
	block, err := aes.NewCipher(out.Plaintext)
	if err != nil {
		t.Fatalf("cipher: %v", err)
	}
	gcm, err := cipher.NewGCM(block)
	if err != nil {
		t.Fatalf("gcm: %v", err)
	}
	iv := make([]byte, gcm.NonceSize())
	data := gcm.Seal(nil, iv, plaintext, nil) // nil AAD = legacy
	return &EncryptedManifest{
		Algorithm:     "AES-256-GCM",
		KMSKeyID:      keyID,
		EncryptedDEK:  base64.StdEncoding.EncodeToString(out.CiphertextBlob),
		IV:            base64.StdEncoding.EncodeToString(iv),
		EncryptedData: base64.StdEncoding.EncodeToString(data),
		// BindingVersion left 0
	}
}
