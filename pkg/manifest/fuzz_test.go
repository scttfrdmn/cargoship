package manifest

import (
	"testing"
	"time"
	"unicode/utf8"
)

// FuzzFromJSON checks that deserializing arbitrary bytes never panics. FromJSON
// is fed manifest data downloaded from S3 / read from disk, so it must degrade
// to an error on garbage input rather than crash.
func FuzzFromJSON(f *testing.F) {
	f.Add([]byte(`{}`))
	f.Add([]byte(`{"version":"2.0","files":[]}`))
	f.Add([]byte(`{"total_files":9223372036854775807}`))
	f.Add([]byte(`not json`))
	f.Add([]byte(``))
	f.Add([]byte(`{"files":[{"path":"a","size":-1}]}`))

	f.Fuzz(func(t *testing.T, data []byte) {
		// Must not panic. An error return is fine; a *Manifest is fine.
		_, _ = FromJSON(data)
	})
}

// FuzzFromJSONAuto checks the auto-detecting deserializer against arbitrary
// bytes, including inputs that begin with the gzip magic number (0x1f 0x8b) but
// carry a malformed gzip body — the path most likely to mishandle input.
func FuzzFromJSONAuto(f *testing.F) {
	f.Add([]byte(`{"version":"2.0"}`))
	f.Add([]byte{GzipMagicNumber1, GzipMagicNumber2})             // gzip magic, truncated body
	f.Add([]byte{GzipMagicNumber1, GzipMagicNumber2, 0x08, 0x00}) // gzip magic + junk
	f.Add([]byte{})

	f.Fuzz(func(t *testing.T, data []byte) {
		_, _ = FromJSONAuto(data)
	})
}

// FuzzManifestRoundTrip verifies the JSON round-trip is stable: a manifest built
// from fuzzed inputs, once serialized and parsed back, re-serializes to the same
// bytes. Catches field-tag/marshaling drift and lossy round-trips.
func FuzzManifestRoundTrip(f *testing.F) {
	f.Add("2.0", "upload-1", "bucket", "prefix", "us-east-1", int64(3), int64(1024), "file.txt")
	f.Add("", "", "", "", "", int64(0), int64(0), "")
	f.Add("2.0", "id", "b", "p", "eu-west-1", int64(-5), int64(1<<62), "π/日本語/file")

	f.Fuzz(func(t *testing.T, version, uploadID, bucket, prefix, region string,
		totalFiles, totalBytes int64, filePath string) {

		// The round-trip is byte-stable only for valid UTF-8: encoding/json
		// replaces invalid UTF-8 bytes with U+FFFD on marshal, so an invalid
		// byte wouldn't survive a second ToJSON. Every string the product puts
		// in a manifest (filesystem paths, generated IDs, region names) is
		// valid UTF-8, so restrict the fuzz domain to match reality.
		for _, s := range []string{version, uploadID, bucket, prefix, region, filePath} {
			if !utf8.ValidString(s) {
				t.Skip()
			}
		}

		m := &Manifest{
			Version:    version,
			UploadID:   uploadID,
			Bucket:     bucket,
			Prefix:     prefix,
			Region:     region,
			TotalFiles: totalFiles,
			TotalBytes: totalBytes,
			// Fixed timestamps so the round-trip is deterministic (UTC avoids
			// zone-offset formatting differences between marshal passes).
			CreatedAt: time.Unix(0, 0).UTC(),
			Files: []FileEntry{
				{Path: filePath, Size: totalBytes},
			},
		}

		data1, err := m.ToJSON()
		if err != nil {
			t.Skip() // some inputs (e.g. non-UTF-8) aren't JSON-encodable; not our concern here
		}

		m2, err := FromJSON(data1)
		if err != nil {
			t.Fatalf("FromJSON failed to parse output of ToJSON: %v\ndata: %q", err, data1)
		}

		data2, err := m2.ToJSON()
		if err != nil {
			t.Fatalf("re-serialize failed: %v", err)
		}

		if string(data1) != string(data2) {
			t.Fatalf("round-trip not stable:\nfirst:  %q\nsecond: %q", data1, data2)
		}
	})
}

// FuzzParseS3URL checks the S3 URL parser never panics and preserves its core
// invariant: for any accepted s3:// URL, "s3://" + bucket + "/" + prefix (or
// just "s3://" + bucket when prefix is empty) reconstructs the original.
func FuzzParseS3URL(f *testing.F) {
	f.Add("s3://bucket/prefix/key")
	f.Add("s3://bucket")
	f.Add("s3://bucket/")
	f.Add("s3://")
	f.Add("http://not-s3")
	f.Add("")
	f.Add("s3://b/p/with spaces/日本語")

	f.Fuzz(func(t *testing.T, url string) {
		bucket, prefix, err := ParseS3URL(url)
		if err != nil {
			return // rejected input; nothing more to check
		}

		var reconstructed string
		if prefix == "" {
			// Ambiguous: both "s3://bucket" and "s3://bucket/" parse to empty
			// prefix. Accept either form.
			if url != "s3://"+bucket && url != "s3://"+bucket+"/" {
				t.Fatalf("empty-prefix reconstruct mismatch: url=%q bucket=%q", url, bucket)
			}
			return
		}
		reconstructed = "s3://" + bucket + "/" + prefix
		if reconstructed != url {
			t.Fatalf("reconstruct mismatch: url=%q -> bucket=%q prefix=%q -> %q",
				url, bucket, prefix, reconstructed)
		}
	})
}
