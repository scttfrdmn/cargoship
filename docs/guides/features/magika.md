# Magika AI file detection

Magika is Google's deep-learning file-type detector. When enabled, CargoShip uses
it during the scan stage to identify content by what a file actually *is* rather
than by its extension — so it recognizes source code in a `.bin`, a misnamed file
with no extension, and 200+ content types that extension matching misses. Better
detection means CargoShip picks a smarter compression level per file.

Magika is **opt-in** and requires the Magika CLI on your PATH:

```bash
pip install magika   # or: pipx install magika
magika --version
```

Then enable it in configuration:

```yaml
magika:
  enabled: true
  batch_size: 100
  timeout: "30s"
```

::: info Graceful fallback
Magika never blocks an upload. If it is not installed or fails, CargoShip logs a
warning and falls back to extension-based detection — uploads always work either
way.
:::

::: warning Draft
This page is being expanded. See [Compression & content-aware](/guides/features/compression)
for how detected types map to compression levels, and the
[Configuration schema](/reference/configuration) for all Magika settings.
:::

## See also

- [Compression & content-aware](/guides/features/compression).
- [Concepts: file entry](/intro/concepts#file-entry) — where the detected type is stored.
- Reference: [Configuration schema](/reference/configuration).
