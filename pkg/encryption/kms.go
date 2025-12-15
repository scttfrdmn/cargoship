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

	"github.com/aws/aws-sdk-go-v2/aws"
	"github.com/aws/aws-sdk-go-v2/service/kms"
)

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
}

// EncryptManifest encrypts manifest JSON using KMS envelope encryption
//
// Envelope encryption flow:
// 1. Generate a data encryption key (DEK) using KMS GenerateDataKey
// 2. Encrypt the manifest JSON with the DEK using AES-256-GCM
// 3. Store the encrypted DEK (encrypted by KMS) in the output
// 4. Return encrypted manifest with metadata
func (e *KMSEncryptor) EncryptManifest(ctx context.Context, manifestJSON []byte) (*EncryptedManifest, error) {
	// Step 1: Generate data encryption key using KMS
	generateOutput, err := e.kmsClient.GenerateDataKey(ctx, &kms.GenerateDataKeyInput{
		KeyId:   aws.String(e.kmsKeyID),
		KeySpec: "AES_256", // 256-bit AES key
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

	// Encrypt the manifest JSON
	// GCM provides authenticated encryption (confidentiality + integrity)
	encryptedData := gcm.Seal(nil, iv, manifestJSON, nil)

	// Step 3: Return encrypted manifest with metadata
	return &EncryptedManifest{
		Algorithm:     "AES-256-GCM",
		KMSKeyID:      e.kmsKeyID,
		EncryptedDEK:  base64.StdEncoding.EncodeToString(encryptedDEK),
		IV:            base64.StdEncoding.EncodeToString(iv),
		EncryptedData: base64.StdEncoding.EncodeToString(encryptedData),
	}, nil
}

// DecryptManifest decrypts an encrypted manifest using KMS envelope encryption
//
// Decryption flow:
// 1. Decrypt the data encryption key (DEK) using KMS Decrypt
// 2. Decrypt the manifest data using the DEK with AES-256-GCM
// 3. Return the plaintext manifest JSON
func (e *KMSEncryptor) DecryptManifest(ctx context.Context, encrypted *EncryptedManifest) ([]byte, error) {
	// Validate algorithm
	if encrypted.Algorithm != "AES-256-GCM" {
		return nil, fmt.Errorf("unsupported encryption algorithm: %s (expected AES-256-GCM)", encrypted.Algorithm)
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

	// Step 1: Decrypt the DEK using KMS
	decryptOutput, err := e.kmsClient.Decrypt(ctx, &kms.DecryptInput{
		CiphertextBlob: encryptedDEK,
		KeyId:          aws.String(encrypted.KMSKeyID), // Optional but recommended for verification
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

	// Decrypt and authenticate
	manifestJSON, err := gcm.Open(nil, iv, encryptedData, nil)
	if err != nil {
		return nil, fmt.Errorf("failed to decrypt manifest data (authentication failed): %w", err)
	}

	return manifestJSON, nil
}

// EncryptManifestBytes is a convenience function that encrypts manifest bytes
func EncryptManifestBytes(ctx context.Context, kmsClient KMSClient, kmsKeyID string, manifestJSON []byte) (*EncryptedManifest, error) {
	encryptor := NewKMSEncryptor(kmsClient, kmsKeyID)
	return encryptor.EncryptManifest(ctx, manifestJSON)
}

// DecryptManifestBytes is a convenience function that decrypts manifest bytes
func DecryptManifestBytes(ctx context.Context, kmsClient KMSClient, encrypted *EncryptedManifest) ([]byte, error) {
	encryptor := NewKMSEncryptor(kmsClient, encrypted.KMSKeyID)
	return encryptor.DecryptManifest(ctx, encrypted)
}
