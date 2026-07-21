# Reading archives (Go library)

CargoShip's manifest and encryption packages are importable, so a Go tool can
parse manifests, query files, download from S3, decrypt, and extract from chunks
without shelling out to the CLI. This page shows the real, current APIs against
the source structs on the other pages.

```go
import "github.com/scttfrdmn/cargoship/pkg/manifest"
```

::: warning A note on older example docs
The `examples/library-usage` README mentions `manifest.ExtractFiles(...)` and a
`query.FindByPattern(...)` method. Those names do **not** exist in the current
`pkg/manifest` package. Use the real APIs documented here: `ListFiles` for glob
matching, `FindFile` for exact lookup, and a small `tar`/`zstd` loop for
extraction (shown below). This page is the accurate reference.
:::

## Loading a manifest

### From the local filesystem

`LoadManifestFromFile` reads a manifest from disk, transparently handling gzip
for `.gz` paths:

```go
m, err := manifest.LoadManifestFromFile("manifest.json.gz") // or manifest.json
if err != nil {
	return err
}
fmt.Printf("upload %s: %d files, %d chunks\n",
	m.UploadID, m.TotalFiles, m.TotalChunks)
```

Lower-level parsers are available when you already have the bytes:

```go
m, err := manifest.FromJSON(jsonBytes)            // plain JSON
m, err := manifest.FromJSONCompressed(gzipBytes)  // gzip-compressed JSON
```

### From S3

`DownloadFromS3` fetches the manifest for an upload, trying `manifest.json.gz`
first and falling back to `manifest.json`:

```go
cfg, err := config.LoadDefaultConfig(ctx)
if err != nil {
	return err
}
s3Client := s3.NewFromConfig(cfg)

m, err := manifest.DownloadFromS3(ctx, s3Client, "my-bucket", "production", "20260721-123456-abcd1234")
if err != nil {
	return err
}
```

The arguments are `bucket`, `prefix` (empty string if none), and `uploadID`.

### From S3, encrypted

For KMS-envelope-encrypted manifests, use the decrypting variant with a KMS
client. It probes the encrypted keys first, then falls back to the plaintext
download:

```go
kmsClient := kms.NewFromConfig(cfg)
m, err := manifest.DownloadFromS3WithDecryption(
	ctx, s3Client, kmsClient, "my-bucket", "production", "20260721-123456-abcd1234")
```

See [Encryption](/reference/format/encryption) for the envelope flow.

## Querying files

`NewManifestQuery` builds O(1) indices over the manifest for lookups. It indexes
by path, by `ContentHash`, by Git commit, and by DVC stage.

```go
q := manifest.NewManifestQuery(m)
```

### Exact path lookup

```go
f := q.FindFile("src/main.go") // *manifest.FileEntry, or nil if not found
if f != nil {
	fmt.Printf("%s is in chunk %d (shard %d): %s\n",
		f.Path, f.ChunkID, f.ShardID, f.S3Key)
}
```

### Glob matching

`ListFiles` returns entries matching a `filepath.Match` glob; it tries the full
path and then the basename, so `*.log` matches `dir/app.log`. An empty pattern
returns all files.

```go
logs := q.ListFiles("*.log")       // all .log files, anywhere
csvs := q.ListFiles("data/*.csv")  // CSVs directly under data/
all  := q.ListFiles("")            // every file
```

::: info `filepath.Match` semantics
`*` and `?` do not cross path separators, and `**` is not supported. For
recursive matching, filter the full `ListFiles("")` slice yourself.
:::

### By location, hash, commit, or DVC stage

```go
shardFiles := q.FilesInShard(0)  // []FileEntry in shard 0
chunkFiles := q.FilesInChunk(3)  // []FileEntry in chunk 3

f, ok := q.FindFileByHash("9e107d9d372bb6826bd81d3542a419d6") // by ContentHash (MD5)
byCommit := q.FindFilesByCommit("<40-char-sha>")              // []*FileEntry
byStage  := q.FindFilesByDVCStage("preprocess")              // []*FileEntry
```

### Summary and rollups

```go
s := q.GetSummary()  // ManifestSummary: totals, ratio, upload ID, timestamps
n := q.CountFiles()  // int64
sz := q.TotalSize()  // int64 (uncompressed bytes)
sh := q.GetShard(2)  // *ShardEntry, or nil
```

## Downloading a chunk

The manifest gives you the exact `s3_key` for any file's chunk — fetch it
directly:

```go
f := q.FindFile("src/main.go")
out, err := s3Client.GetObject(ctx, &s3.GetObjectInput{
	Bucket: aws.String(m.Bucket),
	Key:    aws.String(f.S3Key),
})
if err != nil {
	return err
}
defer out.Body.Close()
// out.Body is the .tar.zst (or .tar) chunk stream
```

## Extracting a file from a chunk

A chunk is a `tar` stream, optionally wrapped in a single zstd frame. Branch on
the key extension (see [Compression](/reference/format/compression)), decode, and
scan the tar for your target entry. The pattern below mirrors CargoShip's own
example program:

```go
import (
	"archive/tar"
	"io"
	"strings"

	"github.com/klauspost/compress/zstd"
)

// extractFile writes the tar entry `name` from a chunk reader into `w`.
// `compressed` should be true for a .tar.zst chunk, false for a plain .tar.
func extractFile(chunk io.Reader, compressed bool, name string, w io.Writer) error {
	var tr *tar.Reader
	if compressed {
		zr, err := zstd.NewReader(chunk)
		if err != nil {
			return err
		}
		defer zr.Close()
		tr = tar.NewReader(zr)
	} else {
		tr = tar.NewReader(chunk)
	}

	for {
		hdr, err := tr.Next()
		if err == io.EOF {
			return fmt.Errorf("file not found in chunk: %s", name)
		}
		if err != nil {
			return err
		}
		if hdr.Name == name {
			_, err := io.Copy(w, tr)
			return err
		}
	}
}
```

::: tip Split files use `.part{N}` entry names
For a file with `TotalParts > 1`, the tar entry names are `{path}.part0`,
`{path}.part1`, … and each carries `CARGOSHIP.*` PAX records (`hdr.PAXRecords`)
describing the offset and part index. Collect the parts and reassemble in offset
order — see [Split files](/reference/format/split-files).
:::

## Verifying integrity

- `ChunkEntry.Checksum` is the **SHA256 of the compressed chunk object** — hash
  the downloaded bytes and compare before extracting.
- `FileEntry.Checksum` is a per-file **SHA256** when present.
- `FileEntry.ContentHash` is **MD5** and exists only for DVC compatibility — do
  not use it as an integrity check.

## Decrypting a manifest directly

If you have an `EncryptedManifest` wrapper (from `manifest.encrypted.json`), the
`pkg/encryption` package decrypts it given a KMS client:

```go
import "github.com/scttfrdmn/cargoship/pkg/encryption"

var wrapper encryption.EncryptedManifest
if err := json.Unmarshal(data, &wrapper); err != nil {
	return err
}
plaintextJSON, err := encryption.DecryptManifestBytes(ctx, kmsClient, &wrapper)
if err != nil {
	return err // GCM auth failure = tampered data or wrong key
}
m, err := manifest.FromJSON(plaintextJSON)
```

## Related

- [Manifest schema](/reference/format/manifest) — the structs these APIs return.
- [Archive layout](/reference/format/archive-layout) — key scheme and container.
- [Encryption](/reference/format/encryption) — the envelope flow in detail.
