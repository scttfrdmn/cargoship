package fleet

import (
	"context"
	"testing"
)

func TestControlPrefix(t *testing.T) {
	if got := ControlPrefix("backups", "w1"); got != "backups/fleet/w1" {
		t.Errorf("ControlPrefix = %q, want backups/fleet/w1", got)
	}
	if got := ConfigKey("backups/fleet/w1"); got != "backups/fleet/w1/config.yaml" {
		t.Errorf("ConfigKey = %q", got)
	}
	if got := ConfigSigKey("backups/fleet/w1"); got != "backups/fleet/w1/config.yaml.sig" {
		t.Errorf("ConfigSigKey = %q", got)
	}
}

// signedConfigObjects returns a fakeReadAPI serving a signed config under
// controlPrefix, plus the public key that verifies it.
func signedConfigObjects(t *testing.T, controlPrefix string, configBytes []byte) (*fakeReadAPI, []byte) {
	t.Helper()
	privPEM, pubPEM, err := GenerateConfigKeypair()
	if err != nil {
		t.Fatal(err)
	}
	priv, _ := LoadConfigPrivateKey(privPEM)
	sig := SignConfigBytes(priv, configBytes)
	sigBytes, _ := sig.Marshal()
	api := &fakeReadAPI{objects: map[string][]byte{
		ConfigKey(controlPrefix):    configBytes,
		ConfigSigKey(controlPrefix): sigBytes,
	}}
	return api, pubPEM
}

func TestPullSignedConfig_HappyPath(t *testing.T) {
	cp := ControlPrefix("backups", "w1")
	want := []byte("id: box-1\nversion: 3\n")
	api, pubPEM := signedConfigObjects(t, cp, want)
	pub, _ := LoadConfigPublicKey(pubPEM)

	got, err := PullSignedConfig(context.Background(), api, "bucket", cp, pub)
	if err != nil {
		t.Fatalf("PullSignedConfig: %v", err)
	}
	if string(got) != string(want) {
		t.Errorf("config bytes = %q, want %q", got, want)
	}
}

func TestPullSignedConfig_TamperedFails(t *testing.T) {
	cp := ControlPrefix("backups", "w1")
	api, pubPEM := signedConfigObjects(t, cp, []byte("id: box-1\n"))
	pub, _ := LoadConfigPublicKey(pubPEM)

	// Swap the config object for different bytes: signature no longer matches.
	api.objects[ConfigKey(cp)] = []byte("id: evil\nwatch_paths:\n  - path: /etc\n")

	if _, err := PullSignedConfig(context.Background(), api, "bucket", cp, pub); err == nil {
		t.Fatal("PullSignedConfig must fail on a tampered config")
	}
}

func TestPullSignedConfig_WrongKeyFails(t *testing.T) {
	cp := ControlPrefix("backups", "w1")
	api, _ := signedConfigObjects(t, cp, []byte("id: box-1\n"))
	// A different key that did not sign the config.
	_, otherPubPEM, _ := GenerateConfigKeypair()
	otherPub, _ := LoadConfigPublicKey(otherPubPEM)

	if _, err := PullSignedConfig(context.Background(), api, "bucket", cp, otherPub); err == nil {
		t.Fatal("PullSignedConfig must fail against the wrong public key")
	}
}

func TestPullSignedConfig_MissingObjectsFail(t *testing.T) {
	cp := ControlPrefix("backups", "w1")
	_, pubPEM, _ := GenerateConfigKeypair()
	pub, _ := LoadConfigPublicKey(pubPEM)

	// No config object at all.
	empty := &fakeReadAPI{objects: map[string][]byte{}}
	if _, err := PullSignedConfig(context.Background(), empty, "bucket", cp, pub); err == nil {
		t.Fatal("missing config should fail")
	}

	// Config present, signature missing.
	cfgOnly := &fakeReadAPI{objects: map[string][]byte{ConfigKey(cp): []byte("id: x\n")}}
	if _, err := PullSignedConfig(context.Background(), cfgOnly, "bucket", cp, pub); err == nil {
		t.Fatal("missing signature should fail")
	}
}
