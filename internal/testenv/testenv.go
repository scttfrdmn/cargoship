// Package testenv isolates a test binary from the developer's real home
// directory and AWS configuration (#1048).
//
// Before this existed, `go test ./...` wrote ~/.cargoship.yaml,
// ~/.cargoship-context and ~/.cargoship/state on the developer's machine, and an
// emulator-less test could pick up the real AWS profile. Every test package
// whose code can reach a home-directory path calls Isolate from an init
// function in a _test.go file; scripts/ci/check-test-home-isolation.sh enforces
// that.
//
// Isolate runs in init rather than TestMain so it works under every build-tag
// combination (several packages have integration-only TestMains) and before any
// test or TestMain body runs.
package testenv

import (
	"fmt"
	"os"
	"path/filepath"
	"sync"
)

// realAWSOptIns are the environment switches that opt a test run into real AWS.
// When any is set, the AWS credential chain is left alone (only its file paths
// are pinned to the developer's real ~/.aws so a profile keeps working under the
// isolated HOME).
var realAWSOptIns = map[string]string{
	"CARGOSHIP_ENABLE_S3_INTEGRATION_TESTS":  "1",
	"CARGOSHIP_ENABLE_AWS_INTEGRATION_TESTS": "true",
	"CARGOSHIP_ENABLE_MULTIREGION_TESTS":     "",
}

// awsCredentialVars are cleared when no real-AWS opt-in is set, so a test that
// forgets to point at the emulator cannot fall through to real credentials.
var awsCredentialVars = []string{
	"AWS_PROFILE",
	"AWS_DEFAULT_PROFILE",
	"AWS_ACCESS_KEY_ID",
	"AWS_SECRET_ACCESS_KEY",
	"AWS_SESSION_TOKEN",
	"AWS_WEB_IDENTITY_TOKEN_FILE",
	"AWS_ROLE_ARN",
}

var (
	once    sync.Once
	homeDir string
	initErr error
)

// Isolate points HOME, USERPROFILE and the XDG base directories at a fresh
// temporary directory and, unless the run opted into real AWS, points the AWS
// shared config and credentials files at empty files and clears AWS credential
// variables. Go's build and module caches are pinned to their real locations
// first, so tests that invoke the go tool don't rebuild or re-download.
//
// It is idempotent and panics on failure: a test binary that can't isolate
// itself must not run against the real home directory.
func Isolate() {
	once.Do(func() { homeDir, initErr = isolate() })
	if initErr != nil {
		panic(fmt.Sprintf("testenv: isolate test environment: %v", initErr))
	}
}

// HomeDir returns the isolated home directory (empty before Isolate).
func HomeDir() string { return homeDir }

// RealAWS reports whether the run opted into real AWS.
func RealAWS() bool {
	for name, want := range realAWSOptIns {
		v, ok := os.LookupEnv(name)
		if !ok || v == "" {
			continue
		}
		if want == "" || v == want {
			return true
		}
	}
	return false
}

func isolate() (string, error) {
	realHome, err := os.UserHomeDir()
	if err != nil {
		return "", fmt.Errorf("resolve real home: %w", err)
	}
	realCache, err := os.UserCacheDir()
	if err != nil {
		return "", fmt.Errorf("resolve real cache dir: %w", err)
	}
	pinIfUnset("GOCACHE", filepath.Join(realCache, "go-build"))
	pinIfUnset("GOPATH", filepath.Join(realHome, "go"))
	pinIfUnset("GOMODCACHE", filepath.Join(os.Getenv("GOPATH"), "pkg", "mod"))

	dir, err := os.MkdirTemp("", "cargoship-testhome-")
	if err != nil {
		return "", fmt.Errorf("create isolated home: %w", err)
	}
	set := map[string]string{
		"HOME":            dir,
		"USERPROFILE":     dir,
		"XDG_CONFIG_HOME": filepath.Join(dir, ".config"),
		"XDG_CACHE_HOME":  filepath.Join(dir, ".cache"),
		"XDG_DATA_HOME":   filepath.Join(dir, ".local", "share"),
		"XDG_STATE_HOME":  filepath.Join(dir, ".local", "state"),
	}
	if RealAWS() {
		// Keep the developer's profile resolvable under the new HOME.
		pinIfUnset("AWS_CONFIG_FILE", filepath.Join(realHome, ".aws", "config"))
		pinIfUnset("AWS_SHARED_CREDENTIALS_FILE", filepath.Join(realHome, ".aws", "credentials"))
	} else {
		awsDir := filepath.Join(dir, ".aws")
		if err := os.MkdirAll(awsDir, 0o700); err != nil {
			return "", fmt.Errorf("create isolated aws dir: %w", err)
		}
		for _, name := range []string{"config", "credentials"} {
			if err := os.WriteFile(filepath.Join(awsDir, name), nil, 0o600); err != nil {
				return "", fmt.Errorf("create empty aws %s: %w", name, err)
			}
		}
		set["AWS_CONFIG_FILE"] = filepath.Join(awsDir, "config")
		set["AWS_SHARED_CREDENTIALS_FILE"] = filepath.Join(awsDir, "credentials")
		set["AWS_EC2_METADATA_DISABLED"] = "true"
		for _, name := range awsCredentialVars {
			if err := os.Unsetenv(name); err != nil {
				return "", fmt.Errorf("unset %s: %w", name, err)
			}
		}
	}
	for k, v := range set {
		if err := os.Setenv(k, v); err != nil {
			return "", fmt.Errorf("set %s: %w", k, err)
		}
	}
	return dir, nil
}

func pinIfUnset(name, value string) {
	if os.Getenv(name) == "" {
		_ = os.Setenv(name, value)
	}
}
