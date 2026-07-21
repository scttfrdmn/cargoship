# Magika AI file detection

Magika is Google's deep-learning file-type detector. When enabled, CargoShip runs
it during the scan stage to identify content by what a file actually *is* rather
than by its extension — so it recognizes source code in a `.bin`, a misnamed file
with no extension, and 200+ content types that extension matching misses. Better
detection means CargoShip picks a smarter compression level per file.

Magika is **opt-in** and disabled by default. It requires the Magika CLI on your
PATH:

```bash
pip install magika   # or: pipx install magika
magika --version
```

## Enabling Magika

Turn it on in your CargoShip configuration file:

```yaml
magika:
  enabled: true          # default: false (opt-in)
  batch_size: 100        # files per Magika invocation
  timeout: "30s"         # per-batch execution timeout
  enable_cache: true     # cache results by path within a run
```

CargoShip discovers the `magika` binary on your PATH automatically. Point at a
specific binary with `binary_path` if it lives outside PATH:

```yaml
magika:
  enabled: true
  binary_path: /opt/magika/bin/magika
```

All settings live in the [configuration schema](/reference/configuration).

| Setting | Default | Purpose |
|---------|---------|---------|
| `enabled` | `false` | Master switch (opt-in). |
| `binary_path` | *(auto-discover)* | Path to the `magika` binary; empty resolves via PATH. |
| `batch_size` | `100` | Files sent per Magika invocation. |
| `timeout` | `30s` | Per-batch execution timeout. |
| `enable_cache` | `true` | Cache detection results by path. |
| `use_mime_type` | `false` | Key compression on MIME type rather than the content label. |
| `include_scores` | `false` | Retain Magika's confidence scores in results. |

## How detection drives compression

CargoShip batches scanned files, runs `magika --json` over each batch, and stores
the detected content label with the file. The compression selector then maps that
label to a content class and a compression level:

| Detected content | Class | Compression |
|------------------|-------|-------------|
| `python`, `go`, `javascript`, `json`, `yaml`, `sql`, ... | Code | Highest (already-text data compresses well) |
| `txt`, `markdown`, `csv`, `log`, `toml`, ... | Text | High |
| `pdf`, `docx`, `xlsx`, `epub`, ... | Document | Moderate |
| `jpeg`, `png`, `webp`, `heic`, ... | Image | Minimal (already compressed) |
| `mp4`, `mp3`, and other media / archives | Media / Archive | Skipped (already compressed) |

Detection identifies content that extensions miss — code inside a `.bin`, or a
file with no extension at all — so CargoShip spends CPU only where compression
actually helps. See [Compression](/reference/format/compression) for the full
class-to-level mapping.

::: info Graceful fallback
Magika never blocks an upload. If the binary is missing, fails, or times out,
CargoShip logs a warning and falls back to extension-based detection — uploads
always work either way.
:::

## See also

- [Compression & content-aware](/guides/features/compression).
- [Concepts: file entry](/intro/concepts#file-entry) — where the detected type is stored.
- Reference: [Compression format](/reference/format/compression).
- Reference: [Configuration schema](/reference/configuration).
