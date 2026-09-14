package manifest

import (
	"fmt"
	"io"
)

// Safety limits bound the memory a single manifest or object read may allocate,
// so a hostile or corrupt archive/manifest cannot force an unbounded allocation
// (CWE-400/409, CSH-SEC-003). They are deliberately generous — far above any
// real workload — but finite.
const (
	// MaxManifestObjectBytes bounds a manifest object as stored in S3 (gzip on
	// the wire). Manifests are JSON metadata; even millions of files stay well
	// under this.
	MaxManifestObjectBytes = 512 << 20 // 512 MiB

	// MaxManifestJSONBytes bounds the manifest JSON after gzip decompression, so
	// a small hostile object cannot decompress into an unbounded allocation
	// (a gzip bomb).
	MaxManifestJSONBytes = 4 << 30 // 4 GiB

	// MaxObjectBytes is the finite fallback cap for a chunk/object read whose
	// expected size the manifest does not declare. When the manifest DOES declare
	// a size, the read is bounded to that (plus a small slack) instead.
	MaxObjectBytes = 8 << 30 // 8 GiB

	// objectSizeSlack is allowed over the manifest's declared object size before
	// a read is rejected, absorbing benign framing/padding differences.
	objectSizeSlack = 1 << 20 // 1 MiB

	// MaxManifestFiles and MaxManifestChunks bound the entry counts a manifest
	// may carry — generous but finite, so a crafted manifest cannot describe an
	// impossibly large dataset that exhausts memory downstream.
	MaxManifestFiles  = 100_000_000
	MaxManifestChunks = 10_000_000
)

// readAllLimited reads all of r but no more than max bytes, returning an
// "exceeds safety limit" error if the source holds more. It reads max+1 through
// an io.LimitReader so an over-limit source is detected, not silently truncated
// (CSH-SEC-003). what names the source in the error.
func readAllLimited(r io.Reader, max int64, what string) ([]byte, error) {
	data, err := io.ReadAll(io.LimitReader(r, max+1))
	if err != nil {
		return nil, err
	}
	if int64(len(data)) > max {
		return nil, fmt.Errorf("%s exceeds safety limit of %d bytes (CSH-SEC-003)", what, max)
	}
	return data, nil
}

// objectReadLimit returns the byte cap for reading an object whose manifest-
// declared size is expected (0 or negative = unknown → the finite MaxObjectBytes
// fallback). A small slack is added so benign padding doesn't trip the cap.
func objectReadLimit(expected int64) int64 {
	if expected <= 0 {
		return MaxObjectBytes
	}
	return expected + objectSizeSlack
}

// validateManifestCounts rejects a manifest whose declared entry counts exceed
// the finite safety limits, so nothing downstream allocates per-entry from an
// impossibly large dataset (CSH-SEC-003). Split out from FromJSON so the
// threshold logic is testable without allocating the entries.
func validateManifestCounts(files, chunks int) error {
	if files > MaxManifestFiles {
		return fmt.Errorf("manifest declares %d files, exceeds safety limit of %d (CSH-SEC-003)", files, MaxManifestFiles)
	}
	if chunks > MaxManifestChunks {
		return fmt.Errorf("manifest declares %d chunks, exceeds safety limit of %d (CSH-SEC-003)", chunks, MaxManifestChunks)
	}
	return nil
}

// checkContentLength rejects an S3 object whose reported ContentLength already
// exceeds limit, before any bytes are read (CSH-SEC-003). A nil ContentLength
// (unset by the server) is not trusted either way — the subsequent bounded read
// still enforces the cap.
func checkContentLength(contentLength *int64, limit int64, what string) error {
	if contentLength != nil && *contentLength > limit {
		return fmt.Errorf("%s: S3 object reports %d bytes, exceeds safety limit of %d (CSH-SEC-003)",
			what, *contentLength, limit)
	}
	return nil
}
