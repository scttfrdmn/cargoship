package config

import (
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// isolate points HOME and the working directory at fresh temp dirs so the
// search never sees the developer's real config.
func isolate(t *testing.T) (home, cwd string) {
	t.Helper()
	home, cwd = t.TempDir(), t.TempDir()
	t.Setenv("HOME", home)
	t.Chdir(cwd)
	return home, cwd
}

func touch(t *testing.T, p string) {
	t.Helper()
	if err := os.MkdirAll(filepath.Dir(p), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(p, []byte("aws:\n  region: us-west-2\n"), 0o600); err != nil {
		t.Fatal(err)
	}
}

func TestResolveConfigFile_Explicit(t *testing.T) {
	isolate(t)
	tests := []struct {
		path    string
		wantErr bool
	}{
		{"/etc/cargoship.yaml", false},
		{"/etc/cargoship.YML", false},
		{"/etc/cargoship", false}, // no extension: read as YAML
		{"/etc/cargoship.toml", true},
		{"/etc/cargoship.json", true},
	}
	for _, tt := range tests {
		got, _, err := ResolveConfigFile(tt.path)
		if tt.wantErr {
			if !errors.Is(err, ErrUnsupportedConfigFormat) {
				t.Errorf("%s: err = %v, want ErrUnsupportedConfigFormat", tt.path, err)
			}
			continue
		}
		if err != nil || got != tt.path {
			t.Errorf("%s: got %q, %v", tt.path, got, err)
		}
	}
}

func TestResolveConfigFile_SearchOrderAndYAMLOnly(t *testing.T) {
	home, cwd := isolate(t)

	if p, w, err := ResolveConfigFile(""); p != "" || w != "" || err != nil {
		t.Fatalf("empty dirs: got %q %q %v", p, w, err)
	}

	// A TOML config alone is reported as ignored, not used (#890).
	touch(t, filepath.Join(home, ".cargoship.toml"))
	p, w, _ := ResolveConfigFile("")
	if p != "" || !strings.Contains(w, ".cargoship.toml") || !strings.Contains(w, "YAML") {
		t.Fatalf("toml only: got path %q, warning %q", p, w)
	}

	// The working directory is searched last...
	touch(t, filepath.Join(cwd, ".cargoship.yaml"))
	if p, _, _ = ResolveConfigFile(""); p != filepath.Join(".", ".cargoship.yaml") {
		t.Fatalf("cwd: got %q", p)
	}
	// ...after ~/.config/cargoship...
	cfgDir := filepath.Join(home, ".config", "cargoship", ".cargoship.yml")
	touch(t, cfgDir)
	if p, _, _ = ResolveConfigFile(""); p != cfgDir {
		t.Fatalf("~/.config: got %q", p)
	}
	// ...and the home directory wins, preferring .yaml over .yml.
	touch(t, filepath.Join(home, ".cargoship.yml"))
	touch(t, filepath.Join(home, ".cargoship.yaml"))
	if p, w, _ = ResolveConfigFile(""); p != filepath.Join(home, ".cargoship.yaml") || w != "" {
		t.Fatalf("home: got %q, warning %q (a YAML file must win over the ignored TOML silently)", p, w)
	}
}

// TestLoadConfig_IgnoresNonYAMLConfig is the #890 repro at the Manager level:
// a lone ~/.cargoship.toml used to be found and then decoded as YAML, failing
// every `config` command.
func TestLoadConfig_IgnoresNonYAMLConfig(t *testing.T) {
	home, _ := isolate(t)
	if err := os.WriteFile(filepath.Join(home, ".cargoship.toml"), []byte("[aws]\nregion = \"us-west-2\"\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	m := NewManager()
	if err := m.LoadConfig(""); err != nil {
		t.Fatalf("a non-YAML config must be ignored, not fail the load: %v", err)
	}
	if err := NewManager().LoadConfig(filepath.Join(home, ".cargoship.toml")); !errors.Is(err, ErrUnsupportedConfigFormat) {
		t.Fatalf("an explicit TOML path must be rejected, got %v", err)
	}
}
