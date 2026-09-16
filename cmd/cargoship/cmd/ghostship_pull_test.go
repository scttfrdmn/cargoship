package cmd

import (
	"strings"
	"testing"
)

func TestGhostshipRun_ConfigURL_RequiresPublicKey(t *testing.T) {
	_, err := runGhostship(t, "run", "--config-url", "s3://b/p", "--writer-id", "w1")
	if err == nil {
		t.Fatal("expected an error when --public-key is missing")
	}
	if !strings.Contains(err.Error(), "public-key is required") {
		t.Errorf("error = %q, want it to require --public-key", err)
	}
}

func TestGhostshipRun_ConfigURL_RequiresWriterID(t *testing.T) {
	t.Setenv("CARGOSHIP_WRITER_ID", "") // ensure no env writer id
	_, err := runGhostship(t, "run", "--config-url", "s3://b/p", "--public-key", "pub.pem")
	if err == nil {
		t.Fatal("expected an error when no writer id can be resolved")
	}
	if !strings.Contains(err.Error(), "writer id") {
		t.Errorf("error = %q, want it to require a writer id", err)
	}
}

func TestGhostshipRun_ConfigURL_MutuallyExclusiveWithConfig(t *testing.T) {
	_, err := runGhostship(t, "run", "--config-url", "s3://b/p", "--config", "box.yaml")
	if err == nil {
		t.Fatal("expected an error combining --config-url with --config")
	}
	if !strings.Contains(err.Error(), "cannot be combined") {
		t.Errorf("error = %q, want a mutual-exclusion error", err)
	}
}
