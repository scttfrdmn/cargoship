package pipeline

import (
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"os"
	"os/user"
	"path"
	"strings"
)

// Writer isolation (#520): a per-agent writer id that scopes an upload's S3 keys
// under a writers/<id>/ segment so a fleet sharing one bucket/prefix never
// collides. Everything here is opt-in — an empty writer id yields the exact
// pre-#520 layout.
const (
	writerSegment        = "writers"
	maxWriterIDLen       = 63
	autoWriterIDSentinel = "auto"
	writerIDEnvVar       = "CARGOSHIP_WRITER_ID"
)

// WriterPrefix folds a per-writer isolation segment into an S3 key prefix (#520).
// With an empty writerID it returns base unchanged (the pre-#520 layout, byte for
// byte). Otherwise it returns base/writers/<writerID>, so an upload's objects live
// under a writer-scoped subtree. writerID is assumed already validated (see
// SanitizeWriterID/ResolveWriterID). Callers must apply this exactly once per code
// path — it does not detect an already-folded prefix.
func WriterPrefix(base, writerID string) string {
	if writerID == "" {
		return base
	}
	seg := path.Join(writerSegment, writerID)
	if base == "" {
		return seg
	}
	return path.Join(base, seg)
}

// SanitizeWriterID validates an operator-supplied writer id for use in an S3 key
// segment (#520). It rejects — rather than silently rewrites — ids containing a
// path separator, "..", whitespace, or control characters, and ids longer than
// maxWriterIDLen, so a bad value fails loudly instead of writing to an unexpected
// location. The returned value is safe to embed via WriterPrefix.
func SanitizeWriterID(id string) (string, error) {
	if id == "" {
		return "", fmt.Errorf("writer id is empty")
	}
	if len(id) > maxWriterIDLen {
		return "", fmt.Errorf("writer id %q too long (max %d chars)", id, maxWriterIDLen)
	}
	if strings.ContainsAny(id, `/\`) {
		return "", fmt.Errorf("writer id %q must not contain a path separator", id)
	}
	if strings.Contains(id, "..") {
		return "", fmt.Errorf("writer id %q must not contain '..'", id)
	}
	for _, r := range id {
		if r <= ' ' || r == 0x7f {
			return "", fmt.Errorf("writer id %q must not contain whitespace or control characters", id)
		}
	}
	return id, nil
}

// DeriveWriterID returns a stable, opaque per-machine writer id of the form
// w-<first8hex(sha256(hostname \x00 osuser))> (#520). It keeps the raw hostname out
// of the S3 key while staying deterministic for a given host+user, so a box that
// re-runs `--writer-id auto` reuses the same id. The result always passes
// SanitizeWriterID.
func DeriveWriterID() string {
	hostname, err := os.Hostname()
	if err != nil || hostname == "" {
		hostname = "unknown-host"
	}
	osuser := "unknown-user"
	if u, err := user.Current(); err == nil && u.Username != "" {
		osuser = u.Username
	} else if e := os.Getenv("USER"); e != "" {
		osuser = e
	}
	sum := sha256.Sum256([]byte(hostname + "\x00" + osuser))
	return "w-" + hex.EncodeToString(sum[:])[:8]
}

// ResolveWriterID resolves the effective writer id for an upload (#520): the flag
// value, else the CARGOSHIP_WRITER_ID environment variable, else "" (legacy
// single-writer, no isolation segment). The sentinel "auto" (from either source)
// derives a stable opaque id via DeriveWriterID. Any explicit id is validated with
// SanitizeWriterID and a bad value is an error, never silently rewritten.
func ResolveWriterID(flagVal string) (string, error) {
	v := flagVal
	if v == "" {
		v = os.Getenv(writerIDEnvVar)
	}
	if v == "" {
		return "", nil
	}
	if v == autoWriterIDSentinel {
		return DeriveWriterID(), nil
	}
	return SanitizeWriterID(v)
}
