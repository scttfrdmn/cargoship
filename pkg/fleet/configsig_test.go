package fleet

import (
	"strings"
	"testing"
)

func TestConfigSign_VerifyRoundTrip(t *testing.T) {
	privPEM, pubPEM, err := GenerateConfigKeypair()
	if err != nil {
		t.Fatalf("keygen: %v", err)
	}
	priv, err := LoadConfigPrivateKey(privPEM)
	if err != nil {
		t.Fatalf("load private: %v", err)
	}
	pub, err := LoadConfigPublicKey(pubPEM)
	if err != nil {
		t.Fatalf("load public: %v", err)
	}

	data := []byte("id: box-1\nversion: 7\n")
	sig := SignConfigBytes(priv, data)
	if sig.Algorithm != ConfigSigAlgorithm {
		t.Errorf("algorithm = %q", sig.Algorithm)
	}
	if sig.KeyID != ConfigKeyID(pub) {
		t.Errorf("key id mismatch: sig %s vs pub %s", sig.KeyID, ConfigKeyID(pub))
	}
	if err := VerifyConfigBytes(pub, data, sig); err != nil {
		t.Errorf("verify of good signature failed: %v", err)
	}
}

func TestConfigVerify_TamperedBytesFail(t *testing.T) {
	privPEM, pubPEM, _ := GenerateConfigKeypair()
	priv, _ := LoadConfigPrivateKey(privPEM)
	pub, _ := LoadConfigPublicKey(pubPEM)

	sig := SignConfigBytes(priv, []byte("watch_paths:\n  - path: /data\n"))
	if err := VerifyConfigBytes(pub, []byte("watch_paths:\n  - path: /etc\n"), sig); err == nil {
		t.Fatal("verify should fail on tampered bytes")
	}
}

func TestConfigVerify_WrongKeyFails(t *testing.T) {
	priv1PEM, _, _ := GenerateConfigKeypair()
	_, pub2PEM, _ := GenerateConfigKeypair()
	priv1, _ := LoadConfigPrivateKey(priv1PEM)
	pub2, _ := LoadConfigPublicKey(pub2PEM)

	data := []byte("id: box-1\n")
	sig := SignConfigBytes(priv1, data)
	err := VerifyConfigBytes(pub2, data, sig)
	if err == nil {
		t.Fatal("verify should fail against the wrong public key")
	}
	// The key-id guard catches it before the crypto check.
	if !strings.Contains(err.Error(), "key id") {
		t.Errorf("expected a key-id mismatch error, got: %v", err)
	}
}

func TestConfigVerify_UnsupportedAlgorithm(t *testing.T) {
	_, pubPEM, _ := GenerateConfigKeypair()
	pub, _ := LoadConfigPublicKey(pubPEM)
	err := VerifyConfigBytes(pub, []byte("x"), ConfigSignature{Algorithm: "rsa", Signature: "AAAA"})
	if err == nil || !strings.Contains(err.Error(), "unsupported signature algorithm") {
		t.Errorf("want unsupported-algorithm error, got: %v", err)
	}
}

func TestConfigSignature_JSONRoundTrip(t *testing.T) {
	privPEM, _, _ := GenerateConfigKeypair()
	priv, _ := LoadConfigPrivateKey(privPEM)
	sig := SignConfigBytes(priv, []byte("data"))
	b, err := sig.Marshal()
	if err != nil {
		t.Fatalf("marshal: %v", err)
	}
	got, err := ParseConfigSignature(b)
	if err != nil {
		t.Fatalf("parse: %v", err)
	}
	if got != sig {
		t.Errorf("round-trip mismatch: %+v vs %+v", got, sig)
	}
}

func TestLoadKeys_RejectNonPEMAndWrongType(t *testing.T) {
	if _, err := LoadConfigPublicKey([]byte("not pem")); err == nil {
		t.Error("expected error for non-PEM public key")
	}
	if _, err := LoadConfigPrivateKey([]byte("not pem")); err == nil {
		t.Error("expected error for non-PEM private key")
	}
	// A public key fed to the private-key loader must be rejected.
	_, pubPEM, _ := GenerateConfigKeypair()
	if _, err := LoadConfigPrivateKey(pubPEM); err == nil {
		t.Error("expected error loading a public key as a private key")
	}
}
