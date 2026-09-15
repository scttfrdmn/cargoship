package pipeline

import (
	"strings"
	"testing"
)

func TestWriterPrefix(t *testing.T) {
	tests := []struct {
		name, base, id, want string
	}{
		{"empty id is legacy passthrough", "backups/data", "", "backups/data"},
		{"empty id and empty base", "", "", ""},
		{"folds under base", "backups/data", "lab-nas-1", "backups/data/writers/lab-nas-1"},
		{"empty base yields bare segment", "", "lab-nas-1", "writers/lab-nas-1"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := WriterPrefix(tt.base, tt.id); got != tt.want {
				t.Errorf("WriterPrefix(%q,%q)=%q, want %q", tt.base, tt.id, got, tt.want)
			}
		})
	}
	// Composes with the uploads/<id> layout: a single fold produces the #520 key.
	if got := WriterPrefix("p", "w1") + "/uploads/u1"; got != "p/writers/w1/uploads/u1" {
		t.Errorf("composed key = %q, want p/writers/w1/uploads/u1", got)
	}
}

func TestSanitizeWriterID(t *testing.T) {
	valid := []string{"lab-nas-1", "w-a3f9c2d1", "home_server", "Synology.Photos", strings.Repeat("x", 63)}
	for _, v := range valid {
		if got, err := SanitizeWriterID(v); err != nil || got != v {
			t.Errorf("SanitizeWriterID(%q)=%q,%v; want it accepted unchanged", v, got, err)
		}
	}
	invalid := []string{"", "a/b", `a\b`, "..", "a..b", "has space", "tab\there", "ctrl\x01", strings.Repeat("x", 64)}
	for _, v := range invalid {
		if _, err := SanitizeWriterID(v); err == nil {
			t.Errorf("SanitizeWriterID(%q) should have errored", v)
		}
	}
}

func TestDeriveWriterID(t *testing.T) {
	a, b := DeriveWriterID(), DeriveWriterID()
	if a != b {
		t.Errorf("DeriveWriterID not deterministic: %q vs %q", a, b)
	}
	if !strings.HasPrefix(a, "w-") || len(a) != len("w-")+8 {
		t.Errorf("DeriveWriterID shape = %q, want w-<8hex>", a)
	}
	if got, err := SanitizeWriterID(a); err != nil || got != a {
		t.Errorf("derived id %q must pass SanitizeWriterID: %v", a, err)
	}
}

func TestResolveWriterID(t *testing.T) {
	// Empty flag + no env => legacy "" (no isolation segment).
	t.Setenv(writerIDEnvVar, "")
	if got, err := ResolveWriterID(""); err != nil || got != "" {
		t.Errorf("empty => %q,%v; want \"\",nil", got, err)
	}
	// Explicit flag value.
	if got, err := ResolveWriterID("lab-nas-1"); err != nil || got != "lab-nas-1" {
		t.Errorf("flag => %q,%v; want lab-nas-1", got, err)
	}
	// Env fallback when flag empty.
	t.Setenv(writerIDEnvVar, "env-writer")
	if got, err := ResolveWriterID(""); err != nil || got != "env-writer" {
		t.Errorf("env => %q,%v; want env-writer", got, err)
	}
	// Flag beats env.
	if got, _ := ResolveWriterID("flag-writer"); got != "flag-writer" {
		t.Errorf("flag should beat env, got %q", got)
	}
	// "auto" derives the opaque id.
	if got, err := ResolveWriterID("auto"); err != nil || got != DeriveWriterID() {
		t.Errorf("auto => %q,%v; want derived id", got, err)
	}
	// Invalid explicit id is an error (never silently rewritten).
	if _, err := ResolveWriterID("bad/id"); err == nil {
		t.Error("invalid id should error")
	}
}
