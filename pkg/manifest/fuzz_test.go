package manifest

import (
	"net/url"
	"path/filepath"
	"strings"
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

// FuzzRestorePath fuzzes the restore path sanitizer, which is a SECURITY
// boundary: manifests record absolute source paths and a crafted (or corrupt)
// manifest can contain absolute paths, `..` segments, or Windows volume names.
// #282 was a real path traversal here — `filepath.Join(destDir, entry.Path)`
// escaped destDir — so this is exactly the code where table tests cover the
// cases someone thought of and miss the ones they didn't.
//
// The invariant is absolute and holds for every input:
//
//	restorePath either returns an error, or returns a path inside destDir.
//
// A violation is a path-traversal vulnerability, not merely a wrong answer.
// Both layout modes (dataset-relative and --flatten) and both empty and
// populated SourcePath roots are fuzzed, since the sanitizer's behavior depends
// on all of them.
func FuzzRestorePath(f *testing.F) {
	// Seeds: benign, plus the traversal shapes #282 was about.
	f.Add("data/a.txt", "", false)
	f.Add("/home/u/project/data/a.txt", "/home/u/project", false)
	f.Add("../../../etc/passwd", "", false)
	f.Add("/etc/passwd", "", false)
	f.Add("..", "", false)
	f.Add("../", "", true)
	f.Add("a/../../b", "/a", false)
	f.Add(`C:\Windows\system32`, "", false)
	f.Add("//../..//x", "", false)
	f.Add("", "", false)
	f.Add(".", "", true)
	f.Add("日本語/../../x", "", false)

	f.Fuzz(func(t *testing.T, entryPath, sourcePath string, flatten bool) {
		se := &SelectiveExtractor{
			manifest: &Manifest{SourcePath: sourcePath},
			flatten:  flatten,
		}

		// A fixed, absolute destination so containment is meaningful.
		destDir := t.TempDir()

		got, err := se.restorePath(destDir, entryPath)
		if err != nil {
			return // rejecting input is always an acceptable outcome
		}

		// THE invariant: an accepted path must stay inside destDir.
		absDest, aerr := filepath.Abs(destDir)
		if aerr != nil {
			t.Skipf("cannot resolve destDir: %v", aerr)
		}
		absGot, aerr := filepath.Abs(got)
		if aerr != nil {
			t.Fatalf("restorePath returned an unresolvable path %q (entry=%q): %v", got, entryPath, aerr)
		}
		if absGot != absDest && !strings.HasPrefix(absGot, absDest+string(filepath.Separator)) {
			t.Fatalf("PATH TRAVERSAL: entryPath=%q sourcePath=%q flatten=%v escaped destDir\n  destDir: %s\n  result:  %s",
				entryPath, sourcePath, flatten, absDest, absGot)
		}

		// A path equal to destDir itself is not a file location; the sanitizer
		// should have rejected it rather than returning the directory.
		if absGot == absDest {
			t.Fatalf("restorePath returned destDir itself for entryPath=%q (no filename component)", entryPath)
		}

		// In flatten mode the result must live directly in destDir — that is the
		// whole contract of --flatten (basename into the output dir).
		if flatten && filepath.Dir(absGot) != absDest {
			t.Fatalf("flatten=true produced a nested path: entryPath=%q -> %q (want direct child of %q)",
				entryPath, absGot, absDest)
		}
	})
}

// FuzzResolveObjectKey fuzzes the S3 key normalizer, which has caused two real
// bugs: #273 (uploaders clobbered manifest S3Keys with the SDK's full-URL
// Location) and #281 (chunked restore passed a full-URL key to GetObject
// verbatim and failed every file). It is pure string surgery over prefix,
// bucket, and key — a good fuzz target.
//
// Invariants for any inputs:
//   - the result is never itself a URL (a URL reaching GetObject is #273/#281)
//   - when prefix is non-empty, the result is prefix-scoped
//   - the function never panics
//
// Note the first invariant is "is not a URL", not "contains no '://'": S3 permits
// ':' in object keys, so a file named "weird://name.txt" is a legitimate key and
// must survive unmangled. net/url is used as an INDEPENDENT oracle here rather
// than the resolver's own scheme check, so the test can't agree with a buggy
// implementation by construction.
func FuzzResolveObjectKey(f *testing.F) {
	f.Add("prefix", "bucket", "prefix/uploads/x/chunk-0.tar.zst")
	f.Add("prefix", "bucket", "https://bucket.s3.us-east-1.amazonaws.com/prefix/uploads/x/c.tar.zst")
	f.Add("prefix", "bucket", "bucket/prefix/uploads/x/c.tar.zst")
	f.Add("", "", "")
	f.Add("p", "b", "://")
	f.Add("p", "b", "s3://")
	f.Add("p", "b", "://nohost")
	f.Add("prefix", "prefix", "prefix/prefix/k")
	f.Add("prefix", "bucket", "prefix/uploads/x/weird://name.txt")

	f.Fuzz(func(t *testing.T, prefix, bucket, s3Key string) {
		// A manifest Prefix is an S3 key prefix, never a URL — only the stored
		// S3Key can arrive URL-shaped, which is the bug class this covers. A
		// URL-shaped prefix is garbage-in, so restrict the domain to reality
		// rather than assert on nonsense.
		if strings.Contains(prefix, "://") {
			t.Skip()
		}

		got := ResolveObjectKey(prefix, bucket, s3Key)

		// A resolved key is an S3 object key, never a URL. If a scheme+host
		// survives, GetObject is handed a URL — the #273/#281 failure mode.
		if u, err := url.Parse(got); err == nil && u.Scheme != "" && u.Host != "" {
			t.Fatalf("resolved key is still a URL: prefix=%q bucket=%q s3Key=%q -> %q",
				prefix, bucket, s3Key, got)
		}

		// With a non-empty prefix the key must be prefix-scoped, otherwise the
		// object is addressed outside the upload's namespace. Scoping is checked
		// against the slash-trimmed prefix, since surrounding slashes are not
		// part of an S3 prefix ("archives/" and "archives" name the same
		// namespace, and "archives//key" would be a distinct object).
		if trimmed := strings.Trim(prefix, "/"); trimmed != "" && got != "" {
			if got != trimmed && !strings.HasPrefix(got, trimmed+"/") {
				t.Fatalf("resolved key is not prefix-scoped: prefix=%q bucket=%q s3Key=%q -> %q",
					prefix, bucket, s3Key, got)
			}
			// Surrounding slashes on a prefix are not significant — "archives/"
			// and "archives" name the same S3 namespace — so resolving with
			// either must give the same key. Without this, a manifest written
			// from a user-typed "s3://bucket/archives/" resolved to
			// "archives//key", a distinct object that was never written.
			if alt := ResolveObjectKey(trimmed, bucket, s3Key); alt != got {
				t.Fatalf("prefix slash sensitivity: prefix=%q vs %q, bucket=%q s3Key=%q -> %q vs %q",
					prefix, trimmed, bucket, s3Key, got, alt)
			}
		}

		// A resolved key must never have a leading slash: S3 would treat
		// "/key" and "key" as different objects.
		if strings.HasPrefix(got, "/") {
			t.Fatalf("resolved key has a leading slash: prefix=%q bucket=%q s3Key=%q -> %q",
				prefix, bucket, s3Key, got)
		}

		// Resolution is idempotent: feeding an already-resolved key back in must
		// be a no-op, or a key could pick up the prefix twice as it passes
		// through layers (verify -> restore) that each resolve.
		//
		// Excluded: an empty prefix combined with a key whose first segment is
		// the bucket name. "bucket/x" is then genuinely ambiguous — either a
		// bucket-qualified key or a key with a top-level "bucket" directory —
		// and no resolver can be idempotent over an ambiguous encoding. With a
		// prefix (the real-world case) the ambiguity disappears.
		ambiguous := prefix == "" && bucket != "" && strings.HasPrefix(got, bucket+"/")
		if again := ResolveObjectKey(prefix, bucket, got); !ambiguous && again != got {
			t.Fatalf("not idempotent: prefix=%q bucket=%q s3Key=%q -> %q -> %q",
				prefix, bucket, s3Key, got, again)
		}
	})
}

// FuzzValidateAgainstSchema differentially fuzzes the hand-rolled draft-07
// subset validator (schema_validate.go) against the real struct unmarshal.
// Hand-written validators are a canonical fuzz target, but a crash-only fuzz
// would miss the more valuable class of bug: DRIFT between what the product
// accepts and what the published schema claims.
//
// Invariants:
//   - never panics on arbitrary bytes (it is fed manifests downloaded from S3)
//   - a manifest that FromJSON accepts, re-serialized canonically by the product
//     itself, must validate cleanly against the schema. Any violation there means
//     the product writes manifests its own published schema rejects (#274).
func FuzzValidateAgainstSchema(f *testing.F) {
	f.Add([]byte(`{}`))
	f.Add([]byte(`{"version":"2.0","upload_id":"u","files":[]}`))
	f.Add([]byte(`{"version":"2.0","files":[{"path":"a.txt","size":1}]}`))
	f.Add([]byte(`{"files":"not-an-array"}`))
	f.Add([]byte(`{"total_files":"not-a-number"}`))
	f.Add([]byte(`[]`))
	f.Add([]byte(`null`))
	f.Add([]byte(`not json`))
	f.Add([]byte(``))

	f.Fuzz(func(t *testing.T, data []byte) {
		// 1. Must never panic on arbitrary input.
		_, _ = ValidateAgainstSchema(data)

		// 2. Differential check. Only proceed for input the product itself
		//    accepts as a manifest.
		m, err := FromJSON(data)
		if err != nil || m == nil {
			return
		}

		// Re-serialize through the product's own writer. This is the exact byte
		// shape CargoShip uploads, so it MUST satisfy the published schema —
		// regardless of how odd the fuzzer's field values are. (Fuzzing the raw
		// input directly would flag inputs that merely tolerate extra fields;
		// what matters is that what we WRITE complies.)
		out, err := m.ToJSON()
		if err != nil {
			return
		}

		violations, err := ValidateAgainstSchema(out)
		if err != nil {
			t.Fatalf("schema validation errored on product-generated manifest: %v\njson: %s", err, out)
		}
		if len(violations) > 0 {
			t.Fatalf("product-generated manifest violates its own published schema (#274):\n  violations: %v\n  json: %s",
				violations, out)
		}
	})
}
