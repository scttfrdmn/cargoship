package fleet

import (
	"crypto/ed25519"
	"crypto/sha256"
	"crypto/x509"
	"encoding/base64"
	"encoding/hex"
	"encoding/json"
	"encoding/pem"
	"errors"
	"fmt"
)

// ConfigSigAlgorithm is the only signature algorithm the fleet config trust path
// supports (#614): detached ed25519 over the exact config bytes.
const ConfigSigAlgorithm = "ed25519"

// ConfigSignature is the detached signature sidecar for a fleet config object. It is
// stored alongside the config (a `.sig` file locally, or a control-prefix object in
// S3) and carries the algorithm and the signing key's id so a mismatched key is caught
// before the (still cryptographically sound) verify.
type ConfigSignature struct {
	Algorithm string `json:"algorithm"`
	KeyID     string `json:"key_id"`
	Signature string `json:"signature"` // base64 (std) of the raw ed25519 signature
}

// GenerateConfigKeypair generates an ed25519 signing keypair, PEM-encoded (PKCS#8
// private, PKIX public). The operator keeps the private key; only the public key is
// baked into an agent at deploy.
func GenerateConfigKeypair() (privPEM, pubPEM []byte, err error) {
	pub, priv, err := ed25519.GenerateKey(nil)
	if err != nil {
		return nil, nil, fmt.Errorf("generate ed25519 key: %w", err)
	}
	privDER, err := x509.MarshalPKCS8PrivateKey(priv)
	if err != nil {
		return nil, nil, fmt.Errorf("marshal private key: %w", err)
	}
	pubDER, err := x509.MarshalPKIXPublicKey(pub)
	if err != nil {
		return nil, nil, fmt.Errorf("marshal public key: %w", err)
	}
	privPEM = pem.EncodeToMemory(&pem.Block{Type: "PRIVATE KEY", Bytes: privDER})
	pubPEM = pem.EncodeToMemory(&pem.Block{Type: "PUBLIC KEY", Bytes: pubDER})
	return privPEM, pubPEM, nil
}

// LoadConfigPrivateKey parses a PEM PKCS#8 ed25519 private key.
func LoadConfigPrivateKey(privPEM []byte) (ed25519.PrivateKey, error) {
	blk, _ := pem.Decode(privPEM)
	if blk == nil {
		return nil, errors.New("no PEM block in private key")
	}
	k, err := x509.ParsePKCS8PrivateKey(blk.Bytes)
	if err != nil {
		return nil, fmt.Errorf("parse private key: %w", err)
	}
	ed, ok := k.(ed25519.PrivateKey)
	if !ok {
		return nil, fmt.Errorf("not an ed25519 private key (%T)", k)
	}
	return ed, nil
}

// LoadConfigPublicKey parses a PEM PKIX ed25519 public key.
func LoadConfigPublicKey(pubPEM []byte) (ed25519.PublicKey, error) {
	blk, _ := pem.Decode(pubPEM)
	if blk == nil {
		return nil, errors.New("no PEM block in public key")
	}
	k, err := x509.ParsePKIXPublicKey(blk.Bytes)
	if err != nil {
		return nil, fmt.Errorf("parse public key: %w", err)
	}
	ed, ok := k.(ed25519.PublicKey)
	if !ok {
		return nil, fmt.Errorf("not an ed25519 public key (%T)", k)
	}
	return ed, nil
}

// ConfigKeyID is a short, stable identifier for a public key (first 8 bytes of its
// SHA-256, hex). Used to detect a config signed by the wrong key before verifying.
func ConfigKeyID(pub ed25519.PublicKey) string {
	sum := sha256.Sum256(pub)
	return "sha256:" + hex.EncodeToString(sum[:8])
}

// SignConfigBytes produces a detached signature over the exact config bytes.
func SignConfigBytes(priv ed25519.PrivateKey, data []byte) ConfigSignature {
	sig := ed25519.Sign(priv, data)
	return ConfigSignature{
		Algorithm: ConfigSigAlgorithm,
		KeyID:     ConfigKeyID(priv.Public().(ed25519.PublicKey)),
		Signature: base64.StdEncoding.EncodeToString(sig),
	}
}

// VerifyConfigBytes checks a detached signature over data against pub. It fails closed:
// an unsupported algorithm, a key-id mismatch, or a bad signature all return an error.
func VerifyConfigBytes(pub ed25519.PublicKey, data []byte, s ConfigSignature) error {
	if s.Algorithm != ConfigSigAlgorithm {
		return fmt.Errorf("unsupported signature algorithm %q (want %q)", s.Algorithm, ConfigSigAlgorithm)
	}
	sig, err := base64.StdEncoding.DecodeString(s.Signature)
	if err != nil {
		return fmt.Errorf("decode signature: %w", err)
	}
	if want := ConfigKeyID(pub); s.KeyID != "" && s.KeyID != want {
		return fmt.Errorf("signature key id %s does not match the trusted public key %s", s.KeyID, want)
	}
	if !ed25519.Verify(pub, data, sig) {
		return errors.New("signature verification failed")
	}
	return nil
}

// Marshal renders the signature sidecar as indented JSON.
func (s ConfigSignature) Marshal() ([]byte, error) {
	return json.MarshalIndent(s, "", "  ")
}

// ParseConfigSignature parses a signature sidecar.
func ParseConfigSignature(b []byte) (ConfigSignature, error) {
	var s ConfigSignature
	if err := json.Unmarshal(b, &s); err != nil {
		return ConfigSignature{}, fmt.Errorf("parse config signature: %w", err)
	}
	return s, nil
}
