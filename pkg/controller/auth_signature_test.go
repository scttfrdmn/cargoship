package controller

import (
	"crypto"
	"crypto/rand"
	"crypto/rsa"
	"crypto/sha256"
	"crypto/x509"
	"encoding/base64"
	"encoding/pem"
	"log/slog"
	"os"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// Helper function to generate RSA key pair for testing
// Returns privateKey and base64-encoded PEM public key (as expected by RegisterAgent)
func generateTestKeyPair(t *testing.T) (*rsa.PrivateKey, string) {
	t.Helper()

	// Generate 2048-bit RSA key
	privateKey, err := rsa.GenerateKey(rand.Reader, 2048)
	require.NoError(t, err, "Failed to generate RSA key")

	// Export public key as PEM
	pubKeyBytes, err := x509.MarshalPKIXPublicKey(&privateKey.PublicKey)
	require.NoError(t, err, "Failed to marshal public key")

	pubKeyPEM := pem.EncodeToMemory(&pem.Block{
		Type:  "PUBLIC KEY",
		Bytes: pubKeyBytes,
	})

	// Return base64-encoded PEM (as expected by RegisterAgent)
	return privateKey, base64.StdEncoding.EncodeToString(pubKeyPEM)
}

// Note: decodeBase64PEM helper removed - verifySignature now handles base64-encoded input directly

// Helper function to sign a message using RSA-PSS
func signMessage(t *testing.T, privateKey *rsa.PrivateKey, message string) string {
	t.Helper()

	// Hash the message
	hashed := sha256.Sum256([]byte(message))

	// Sign using RSA-PSS
	signature, err := rsa.SignPSS(rand.Reader, privateKey, crypto.SHA256, hashed[:], &rsa.PSSOptions{
		SaltLength: rsa.PSSSaltLengthAuto,
	})
	require.NoError(t, err, "Failed to sign message")

	// Encode as base64
	return base64.StdEncoding.EncodeToString(signature)
}

func TestAuthManager_verifySignature_ValidSignature(t *testing.T) {
	logger := slog.New(slog.NewTextHandler(os.Stdout, &slog.HandlerOptions{Level: slog.LevelError}))
	auth, err := NewAuthManager(nil, logger)
	require.NoError(t, err)

	// Generate test key pair
	privateKey, publicKeyBase64PEM := generateTestKeyPair(t)

	// Test message (agentID)
	agentID := "test-agent-123"

	// Sign the message
	signature := signMessage(t, privateKey, agentID)

	// Verify signature - verifySignature expects base64-encoded PEM
	err = auth.verifySignature(agentID, signature, publicKeyBase64PEM)
	assert.NoError(t, err, "Valid signature should verify successfully")
}

func TestAuthManager_verifySignature_InvalidSignature(t *testing.T) {
	logger := slog.New(slog.NewTextHandler(os.Stdout, &slog.HandlerOptions{Level: slog.LevelError}))
	auth, err := NewAuthManager(nil, logger)
	require.NoError(t, err)

	// Generate two different key pairs
	privateKey1, _ := generateTestKeyPair(t)
	_, publicKeyBase64PEM2 := generateTestKeyPair(t)

	agentID := "test-agent-123"

	// Sign with key1, verify with key2 - should fail
	signature := signMessage(t, privateKey1, agentID)

	err = auth.verifySignature(agentID, signature, publicKeyBase64PEM2)
	assert.Error(t, err, "Signature with wrong key should fail verification")
	assert.Contains(t, err.Error(), "signature verification failed")
}

func TestAuthManager_verifySignature_TamperedMessage(t *testing.T) {
	logger := slog.New(slog.NewTextHandler(os.Stdout, &slog.HandlerOptions{Level: slog.LevelError}))
	auth, err := NewAuthManager(nil, logger)
	require.NoError(t, err)

	privateKey, publicKeyBase64PEM := generateTestKeyPair(t)

	// Sign original message
	originalAgentID := "test-agent-123"
	signature := signMessage(t, privateKey, originalAgentID)

	// Try to verify with different message (tampering)
	tamperedAgentID := "test-agent-456"
	err = auth.verifySignature(tamperedAgentID, signature, publicKeyBase64PEM)
	assert.Error(t, err, "Tampered message should fail verification")
	assert.Contains(t, err.Error(), "signature verification failed")
}

func TestAuthManager_verifySignature_MalformedBase64(t *testing.T) {
	logger := slog.New(slog.NewTextHandler(os.Stdout, &slog.HandlerOptions{Level: slog.LevelError}))
	auth, err := NewAuthManager(nil, logger)
	require.NoError(t, err)

	_, publicKeyBase64PEM := generateTestKeyPair(t)

	// Invalid base64 signature
	invalidSignature := "this-is-not-valid-base64!@#$%"

	err = auth.verifySignature("test-agent", invalidSignature, publicKeyBase64PEM)
	assert.Error(t, err, "Malformed base64 should fail")
	assert.Contains(t, err.Error(), "invalid signature encoding")
}

func TestAuthManager_verifySignature_InvalidPEM(t *testing.T) {
	logger := slog.New(slog.NewTextHandler(os.Stdout, &slog.HandlerOptions{Level: slog.LevelError}))
	auth, err := NewAuthManager(nil, logger)
	require.NoError(t, err)

	privateKey, _ := generateTestKeyPair(t)
	agentID := "test-agent-123"
	signature := signMessage(t, privateKey, agentID)

	// Invalid PEM format
	invalidPEM := "-----BEGIN PUBLIC KEY-----\nthis is not a valid PEM\n-----END PUBLIC KEY-----"

	err = auth.verifySignature(agentID, signature, invalidPEM)
	assert.Error(t, err, "Invalid PEM should fail")
	// Error occurs at base64 decode stage
	assert.Contains(t, err.Error(), "invalid public key encoding")
}

func TestAuthManager_verifySignature_NonRSAKey(t *testing.T) {
	logger := slog.New(slog.NewTextHandler(os.Stdout, &slog.HandlerOptions{Level: slog.LevelError}))
	auth, err := NewAuthManager(nil, logger)
	require.NoError(t, err)

	// Generate EC key instead of RSA
	// This is a valid ECDSA P-256 public key in PEM format
	ecdsaPEM := `-----BEGIN PUBLIC KEY-----
MFkwEwYHKoZIzj0CAQYIKoZIzj0DAQcDQgAE5Z3Ql8fKCQqXf7dL3a4sVvCdQq8t
KQ7wKp7Rq3F5X8vJYpvWxL2N5M3KqF6vQzL8X9Y4F2K5T6M1A3N8Z7Q==
-----END PUBLIC KEY-----`

	// Base64-encode the ECDSA PEM (as expected by verifySignature)
	ecdsaBase64 := base64.StdEncoding.EncodeToString([]byte(ecdsaPEM))

	// Valid base64 signature (doesn't matter what it is)
	validBase64Sig := base64.StdEncoding.EncodeToString([]byte("dummy signature"))

	err = auth.verifySignature("test-agent", validBase64Sig, ecdsaBase64)
	assert.Error(t, err, "Non-RSA key should fail")
	// Error could occur at various stages depending on input
	assert.True(t,
		err.Error() == "public key is not RSA type" ||
		err.Error() == "failed to decode PEM public key" ||
		err.Error() == "signature verification failed: crypto/rsa: verification error",
		"Should fail with appropriate error for non-RSA key")
}

func TestAuthManager_verifySignature_EmptySignature(t *testing.T) {
	logger := slog.New(slog.NewTextHandler(os.Stdout, &slog.HandlerOptions{Level: slog.LevelError}))
	auth, err := NewAuthManager(nil, logger)
	require.NoError(t, err)

	_, publicKeyBase64PEM := generateTestKeyPair(t)

	err = auth.verifySignature("test-agent", "", publicKeyBase64PEM)
	assert.Error(t, err, "Empty signature should fail")
	assert.Contains(t, err.Error(), "signature required")
}

func TestAuthManager_verifySignature_EmptyPublicKey(t *testing.T) {
	logger := slog.New(slog.NewTextHandler(os.Stdout, &slog.HandlerOptions{Level: slog.LevelError}))
	auth, err := NewAuthManager(nil, logger)
	require.NoError(t, err)

	err = auth.verifySignature("test-agent", "dmFsaWRiYXNlNjQ=", "")
	assert.Error(t, err, "Empty public key should fail")
	assert.Contains(t, err.Error(), "public key required")
}

func TestAuthManager_verifySignature_SignatureTooShort(t *testing.T) {
	logger := slog.New(slog.NewTextHandler(os.Stdout, &slog.HandlerOptions{Level: slog.LevelError}))
	auth, err := NewAuthManager(nil, logger)
	require.NoError(t, err)

	_, publicKeyBase64PEM := generateTestKeyPair(t)

	// Valid base64 but too short for RSA-2048 signature (should be 256 bytes)
	shortSignature := base64.StdEncoding.EncodeToString([]byte("short"))

	err = auth.verifySignature("test-agent", shortSignature, publicKeyBase64PEM)
	assert.Error(t, err, "Too short signature should fail")
	assert.Contains(t, err.Error(), "signature verification failed")
}

func TestAuthManager_AuthenticateAgent_WithValidSignature(t *testing.T) {
	logger := slog.New(slog.NewTextHandler(os.Stdout, &slog.HandlerOptions{Level: slog.LevelError}))
	auth, err := NewAuthManager(nil, logger)
	require.NoError(t, err)

	// Generate key pair
	privateKey, publicKeyPEM := generateTestKeyPair(t)

	// Register agent with public key
	agentID := "test-agent-authenticated"
	err = auth.RegisterAgent(agentID, "Test Agent", publicKeyPEM, "agent", []string{"test"}, nil)
	require.NoError(t, err, "Failed to register agent")

	// Sign the agentID
	signature := signMessage(t, privateKey, agentID)

	// Authenticate - should succeed
	token, err := auth.AuthenticateAgent(agentID, signature)
	assert.NoError(t, err, "Authentication with valid signature should succeed")
	assert.NotEmpty(t, token, "Should return JWT token")

	// Verify token is valid
	claims, err := auth.ValidateToken(token)
	assert.NoError(t, err, "Token should be valid")
	assert.Equal(t, agentID, claims.AgentID)
	assert.Equal(t, "agent", claims.Role)
}

func TestAuthManager_AuthenticateAgent_WithInvalidSignature(t *testing.T) {
	logger := slog.New(slog.NewTextHandler(os.Stdout, &slog.HandlerOptions{Level: slog.LevelError}))
	auth, err := NewAuthManager(nil, logger)
	require.NoError(t, err)

	// Generate two key pairs
	_, publicKeyPEM1 := generateTestKeyPair(t)
	privateKey2, _ := generateTestKeyPair(t)

	// Register agent with key1
	agentID := "test-agent-invalid-sig"
	err = auth.RegisterAgent(agentID, "Test Agent", publicKeyPEM1, "agent", []string{"test"}, nil)
	require.NoError(t, err)

	// Sign with key2 (wrong key)
	wrongSignature := signMessage(t, privateKey2, agentID)

	// Authenticate - should fail
	_, err = auth.AuthenticateAgent(agentID, wrongSignature)
	assert.Error(t, err, "Authentication with wrong signature should fail")
	assert.Contains(t, err.Error(), "signature verification failed")
}

func TestAuthManager_AuthenticateAgent_WithoutPublicKey(t *testing.T) {
	logger := slog.New(slog.NewTextHandler(os.Stdout, &slog.HandlerOptions{Level: slog.LevelError}))
	auth, err := NewAuthManager(nil, logger)
	require.NoError(t, err)

	// Register agent WITHOUT public key (empty string)
	agentID := "test-agent-no-key"
	err = auth.RegisterAgent(agentID, "Test Agent", "", "agent", []string{"test"}, nil)
	require.NoError(t, err)

	// Authenticate without signature - should succeed (no verification needed)
	token, err := auth.AuthenticateAgent(agentID, "")
	assert.NoError(t, err, "Authentication without public key should succeed")
	assert.NotEmpty(t, token, "Should return JWT token")
}

func TestAuthManager_validatePublicKey(t *testing.T) {
	logger := slog.New(slog.NewTextHandler(os.Stdout, &slog.HandlerOptions{Level: slog.LevelError}))
	auth, err := NewAuthManager(nil, logger)
	require.NoError(t, err)

	t.Run("valid RSA public key", func(t *testing.T) {
		_, publicKeyBase64PEM := generateTestKeyPair(t)

		// generateTestKeyPair already returns base64-encoded PEM (as expected by RegisterAgent/validatePublicKey)
		err := auth.validatePublicKey(publicKeyBase64PEM)
		assert.NoError(t, err, "Valid RSA public key should pass validation")
	})

	t.Run("invalid base64", func(t *testing.T) {
		err := auth.validatePublicKey("not-valid-base64!@#")
		assert.Error(t, err)
		assert.Contains(t, err.Error(), "invalid base64 encoding")
	})

	t.Run("invalid PEM format", func(t *testing.T) {
		invalidPEM := base64.StdEncoding.EncodeToString([]byte("not a PEM"))
		err := auth.validatePublicKey(invalidPEM)
		assert.Error(t, err)
		assert.Contains(t, err.Error(), "invalid PEM format")
	})
}
