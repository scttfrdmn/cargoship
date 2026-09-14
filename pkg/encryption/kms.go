// Package encryption provides KMS envelope encryption for CargoShip manifests (Issue #163)
package encryption

import (
	"context"
	"crypto/aes"
	"crypto/cipher"
	"crypto/rand"
	"encoding/base64"
	"fmt"
	"io"
	"sort"

	"github.com/aws/aws-sdk-go-v2/aws"
	"github.com/aws/aws-sdk-go-v2/service/kms"
)

// Binding scheme constants (CSH-SEC-004). CurrentBindingVersion is stamped into
// every newly-encrypted manifest; the app/purpose/scheme values are folded into
// the KMS EncryptionContext and GCM AAD so the ciphertext is bound to the upload
// it belongs to.
const (
	CurrentBindingVersion = 1
	bindingApplication    = "cargoship"
	bindingPurpose        = "manifest"
)

// ManifestIdentity is the archive identity an encrypted manifest is bound to.
// Scheme v1 binds the upload ID only: it is unique per upload and travels with
// the archive, so it defeats cross-upload substitution while surviving a
// legitimate copy/rename to another bucket or prefix (#335). Bucket/prefix are
// deliberately NOT bound.
type ManifestIdentity struct {
	UploadID string
}

// encryptionContext returns the KMS EncryptionContext for this identity. KMS
// enforces that the DEK can only be unwrapped with the identical context, so a
// manifest encrypted for a different upload cannot be decrypted here.
func (id ManifestIdentity) encryptionContext() map[string]string {
	return map[string]string{
		"application": bindingApplication,
		"purpose":     bindingPurpose,
		"v":           fmt.Sprintf("%d", CurrentBindingVersion),
		"upload_id":   id.UploadID,
	}
}

// aad returns the AES-GCM additional authenticated data for this identity: the
// same key/value pairs as the KMS context, serialized canonically (keys sorted,
// "k=v\n") so encrypt and decrypt produce byte-identical AAD. GCM authenticates
// but does not encrypt the AAD; a mismatch fails gcm.Open.
func (id ManifestIdentity) aad() []byte {
	ctx := id.encryptionContext()
	keys := make([]string, 0, len(ctx))
	for k := range ctx {
		keys = append(keys, k)
	}
	sort.Strings(keys)
	var b []byte
	for _, k := range keys {
		b = append(b, k...)
		b = append(b, '=')
		b = append(b, ctx[k]...)
		b = append(b, '\n')
	}
	return b
}

// KMSClient defines the interface for KMS operations needed for encryption
type KMSClient interface {
	GenerateDataKey(ctx context.Context, params *kms.GenerateDataKeyInput, optFns ...func(*kms.Options)) (*kms.GenerateDataKeyOutput, error)
	Decrypt(ctx context.Context, params *kms.DecryptInput, optFns ...func(*kms.Options)) (*kms.DecryptOutput, error)
}

// KMSEncryptor handles KMS envelope encryption for manifests
type KMSEncryptor struct {
	kmsClient KMSClient
	kmsKeyID  string
}

// NewKMSEncryptor creates a new KMS encryptor
func NewKMSEncryptor(kmsClient KMSClient, kmsKeyID string) *KMSEncryptor {
	return &KMSEncryptor{
		kmsClient: kmsClient,
		kmsKeyID:  kmsKeyID,
	}
}

// EncryptedManifest represents an encrypted manifest with envelope encryption
type EncryptedManifest struct {
	// Algorithm used for encryption
	Algorithm string `json:"algorithm"`

	// KMS key ID used to encrypt the data key
	KMSKeyID string `json:"kms_key_id"`

	// Base64-encoded encrypted data encryption key (DEK)
	EncryptedDEK string `json:"encrypted_dek"`

	// Base64-encoded initialization vector (IV/nonce) for AES-GCM
	IV string `json:"iv"`

	// Base64-encoded encrypted manifest data
	EncryptedData string `json:"encrypted_data"`

	// BindingVersion is nonzero when the manifest is cryptographically bound to
	// its archive identity via KMS EncryptionContext + GCM AAD (CSH-SEC-004).
	// Absent/0 marks a legacy unbound manifest, decrypted without a context so
	// archives written before this scheme remain readable.
	BindingVersion int `json:"binding_version,omitempty"`
}

// EncryptManifest encrypts manifest JSON using KMS envelope encryption
//
// Envelope encryption flow:
// 1. Generate a data encryption key (DEK) using KMS GenerateDataKey
// 2. Encrypt the manifest JSON with the DEK using AES-256-GCM
// 3. Store the encrypted DEK (encrypted by KMS) in the output
// 4. Return encrypted manifest with metadata
func (e *KMSEncryptor) EncryptManifest(ctx context.Context, manifestJSON []byte, id ManifestIdentity) (*EncryptedManifest, error) {
	// Step 1: Generate data encryption key using KMS, bound to the archive
	// identity via EncryptionContext so the DEK can only be unwrapped for this
	// upload (CSH-SEC-004).
	generateOutput, err := e.kmsClient.GenerateDataKey(ctx, &kms.GenerateDataKeyInput{
		KeyId:             aws.String(e.kmsKeyID),
		KeySpec:           "AES_256", // 256-bit AES key
		EncryptionContext: id.encryptionContext(),
	})
	if err != nil {
		return nil, fmt.Errorf("failed to generate data key from KMS: %w", err)
	}

	// Plaintext DEK (32 bytes for AES-256)
	plaintextDEK := generateOutput.Plaintext

	// Encrypted DEK (encrypted by KMS using the CMK)
	encryptedDEK := generateOutput.CiphertextBlob

	// Step 2: Encrypt manifest JSON with AES-256-GCM using the DEK
	block, err := aes.NewCipher(plaintextDEK)
	if err != nil {
		return nil, fmt.Errorf("failed to create AES cipher: %w", err)
	}

	gcm, err := cipher.NewGCM(block)
	if err != nil {
		return nil, fmt.Errorf("failed to create GCM mode: %w", err)
	}

	// Generate random IV (nonce) for GCM
	iv := make([]byte, gcm.NonceSize())
	if _, err := io.ReadFull(rand.Reader, iv); err != nil {
		return nil, fmt.Errorf("failed to generate IV: %w", err)
	}

	// Encrypt the manifest JSON. GCM provides authenticated encryption; the
	// identity AAD binds the ciphertext to this upload as a second layer beyond
	// the KMS EncryptionContext (CSH-SEC-004).
	encryptedData := gcm.Seal(nil, iv, manifestJSON, id.aad())

	// Step 3: Return encrypted manifest with metadata
	return &EncryptedManifest{
		Algorithm:      "AES-256-GCM",
		KMSKeyID:       e.kmsKeyID,
		EncryptedDEK:   base64.StdEncoding.EncodeToString(encryptedDEK),
		IV:             base64.StdEncoding.EncodeToString(iv),
		EncryptedData:  base64.StdEncoding.EncodeToString(encryptedData),
		BindingVersion: CurrentBindingVersion,
	}, nil
}

// DecryptManifest decrypts an encrypted manifest using KMS envelope encryption
//
// Decryption flow:
// 1. Decrypt the data encryption key (DEK) using KMS Decrypt
// 2. Decrypt the manifest data using the DEK with AES-256-GCM
// 3. Return the plaintext manifest JSON
func (e *KMSEncryptor) DecryptManifest(ctx context.Context, encrypted *EncryptedManifest, expected ManifestIdentity) ([]byte, error) {
	// Validate algorithm
	if encrypted.Algorithm != "AES-256-GCM" {
		return nil, fmt.Errorf("unsupported encryption algorithm: %s (expected AES-256-GCM)", encrypted.Algorithm)
	}

	// CSH-SEC-004: a bound manifest is decrypted only under the identity the
	// caller independently expects (the upload it is fetching). A bound manifest
	// with no expected upload_id can't be verified — fail closed rather than fall
	// back to an unbound decrypt.
	var kmsContext map[string]string
	var aad []byte
	if encrypted.BindingVersion >= 1 {
		if expected.UploadID == "" {
			return nil, fmt.Errorf("encrypted manifest is identity-bound (v%d) but no expected upload identity was provided", encrypted.BindingVersion)
		}
		kmsContext = expected.encryptionContext()
		aad = expected.aad()
	}

	// Decode base64-encoded fields
	encryptedDEK, err := base64.StdEncoding.DecodeString(encrypted.EncryptedDEK)
	if err != nil {
		return nil, fmt.Errorf("failed to decode encrypted DEK: %w", err)
	}

	iv, err := base64.StdEncoding.DecodeString(encrypted.IV)
	if err != nil {
		return nil, fmt.Errorf("failed to decode IV: %w", err)
	}

	encryptedData, err := base64.StdEncoding.DecodeString(encrypted.EncryptedData)
	if err != nil {
		return nil, fmt.Errorf("failed to decode encrypted data: %w", err)
	}

	// Step 1: Decrypt the DEK using KMS. For a bound manifest the same
	// EncryptionContext used at encrypt time is required; KMS refuses the unwrap
	// if it doesn't match, so a substituted manifest from another upload fails.
	decryptOutput, err := e.kmsClient.Decrypt(ctx, &kms.DecryptInput{
		CiphertextBlob:    encryptedDEK,
		KeyId:             aws.String(encrypted.KMSKeyID), // Optional but recommended for verification
		EncryptionContext: kmsContext,
	})
	if err != nil {
		return nil, fmt.Errorf("failed to decrypt DEK using KMS: %w", err)
	}

	plaintextDEK := decryptOutput.Plaintext

	// Step 2: Decrypt the manifest data using AES-256-GCM
	block, err := aes.NewCipher(plaintextDEK)
	if err != nil {
		return nil, fmt.Errorf("failed to create AES cipher: %w", err)
	}

	gcm, err := cipher.NewGCM(block)
	if err != nil {
		return nil, fmt.Errorf("failed to create GCM mode: %w", err)
	}

	// Decrypt and authenticate. aad is nil for a legacy (unbound) manifest and
	// the identity AAD for a bound one; a mismatch fails authentication here even
	// if the DEK unwrapped.
	manifestJSON, err := gcm.Open(nil, iv, encryptedData, aad)
	if err != nil {
		return nil, fmt.Errorf("failed to decrypt manifest data (authentication failed): %w", err)
	}

	return manifestJSON, nil
}

// EncryptManifestBytes is a convenience function that encrypts manifest bytes
// bound to id (CSH-SEC-004).
func EncryptManifestBytes(ctx context.Context, kmsClient KMSClient, kmsKeyID string, manifestJSON []byte, id ManifestIdentity) (*EncryptedManifest, error) {
	encryptor := NewKMSEncryptor(kmsClient, kmsKeyID)
	return encryptor.EncryptManifest(ctx, manifestJSON, id)
}

// DecryptManifestBytes is a convenience function that decrypts manifest bytes.
// expected is the archive identity the caller independently knows (from the
// location it is fetching); a bound manifest is decrypted only under it
// (CSH-SEC-004).
func DecryptManifestBytes(ctx context.Context, kmsClient KMSClient, encrypted *EncryptedManifest, expected ManifestIdentity) ([]byte, error) {
	encryptor := NewKMSEncryptor(kmsClient, encrypted.KMSKeyID)
	return encryptor.DecryptManifest(ctx, encrypted, expected)
}
