package testenv

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestIsolatePointsHomeAtTempDir(t *testing.T) {
	Isolate()

	home, err := os.UserHomeDir()
	if err != nil {
		t.Fatalf("UserHomeDir: %v", err)
	}
	if home != HomeDir() {
		t.Fatalf("UserHomeDir = %q, want the isolated %q", home, HomeDir())
	}
	if !strings.HasPrefix(filepath.Base(home), "cargoship-testhome-") {
		t.Fatalf("isolated home %q is not a testenv temp dir", home)
	}
	if got := os.Getenv("USERPROFILE"); got != home {
		t.Fatalf("USERPROFILE = %q, want %q (Windows reads it, #893)", got, home)
	}
	cfg, err := os.UserConfigDir()
	if err != nil {
		t.Fatalf("UserConfigDir: %v", err)
	}
	if !strings.HasPrefix(cfg, home) {
		t.Fatalf("UserConfigDir %q escapes the isolated home %q", cfg, home)
	}
}

func TestIsolateClearsAWSUnlessOptedIn(t *testing.T) {
	Isolate()
	if RealAWS() {
		t.Skip("real-AWS opt-in set; the credential chain is intentionally kept")
	}
	for _, name := range awsCredentialVars {
		if v, ok := os.LookupEnv(name); ok {
			t.Errorf("%s still set (%d bytes) without a real-AWS opt-in", name, len(v))
		}
	}
	for _, name := range []string{"AWS_CONFIG_FILE", "AWS_SHARED_CREDENTIALS_FILE"} {
		p := os.Getenv(name)
		if !strings.HasPrefix(p, HomeDir()) {
			t.Errorf("%s = %q, want a file under the isolated home", name, p)
			continue
		}
		fi, err := os.Stat(p)
		if err != nil || fi.Size() != 0 {
			t.Errorf("%s should be an empty file: %v", name, err)
		}
	}
}

func TestRealAWS(t *testing.T) {
	for name := range realAWSOptIns {
		t.Setenv(name, "")
	}
	if RealAWS() {
		t.Fatal("RealAWS with no opt-in set")
	}
	tests := []struct {
		name, value string
		want        bool
	}{
		{"CARGOSHIP_ENABLE_S3_INTEGRATION_TESTS", "1", true},
		{"CARGOSHIP_ENABLE_S3_INTEGRATION_TESTS", "0", false},
		{"CARGOSHIP_ENABLE_AWS_INTEGRATION_TESTS", "true", true},
		{"CARGOSHIP_ENABLE_AWS_INTEGRATION_TESTS", "1", false},
		{"CARGOSHIP_ENABLE_MULTIREGION_TESTS", "yes", true},
	}
	for _, tt := range tests {
		t.Run(tt.name+"="+tt.value, func(t *testing.T) {
			t.Setenv(tt.name, tt.value)
			if got := RealAWS(); got != tt.want {
				t.Fatalf("RealAWS() = %v, want %v", got, tt.want)
			}
		})
	}
}

func TestIsolateCoversUserCacheDir(t *testing.T) {
	Isolate()
	cache, err := os.UserCacheDir()
	if err != nil {
		t.Fatalf("UserCacheDir: %v", err)
	}
	if !strings.HasPrefix(cache, HomeDir()) {
		t.Fatalf("UserCacheDir %q escapes the isolated home %q", cache, HomeDir())
	}
}
