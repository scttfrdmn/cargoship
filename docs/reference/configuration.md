# Configuration schema

CargoShip reads settings from a YAML configuration file so you don't have to
repeat flags on every command. Anything in the file can still be overridden by an
[environment variable](/reference/environment-variables) or a command-line flag —
see [Config files & precedence](/guides/config/files).

```bash
cargoship config --generate            # write an annotated example config
cargoship config --show                # print the resolved configuration
cargoship config --validate            # validate the active config
cargoship config --validate-detailed   # also check AWS connectivity & bucket access
```

## File location

Config files are discovered by name (`.cargoship.yaml`) in this order:

1. `$HOME/.cargoship.yaml`
2. `$HOME/.config/cargoship/.cargoship.yaml`
3. `./.cargoship.yaml` (current directory)

The [interactive setup wizard](/guides/config/setup) (`cargoship setup`) writes
`~/.cargoship.yaml` for you. You can also point at any file with `--file`.

## Schema

The file has eight top-level sections. Defaults below are what CargoShip uses when
a key is omitted.

### `aws`

| Key | Type | Default | Notes |
|-----|------|---------|-------|
| `region` | string | `us-west-2` | **Required.** Default region for S3 operations. |
| `profile` | string | — | Named AWS profile. |
| `access_key_id` | string | — | Prefer the credential chain; avoid committing keys. |
| `secret_access_key` | string | — | |
| `session_token` | string | — | For temporary credentials. |
| `s3_endpoint` | string | — | S3-compatible endpoint override (Wasabi, B2, MinIO). |
| `use_path_style` | bool | `false` | Path-style S3 addressing (needed by some S3-compatible providers). |
| `max_retries` | int | `3` | SDK retry attempts. |
| `retry_max_delay` | duration | `30s` | Max backoff between retries. |
| `request_timeout` | duration | `5m` | Per-request timeout. |

### `storage`

| Key | Type | Default | Notes |
|-----|------|---------|-------|
| `default_bucket` | string | — | Used when a command omits the bucket. |
| `default_storage_class` | string | `INTELLIGENT_TIERING` | Must be one of the valid classes (see `security`). |
| `kms_key_id` | string | — | KMS key for SSE-KMS / manifest encryption. |
| `sse_encryption` | bool | `true` | Server-side encryption on by default. |
| `object_tagging` | map | — | Tags applied to uploaded objects. |
| `metadata_directive` | string | `REPLACE` | S3 metadata directive. |

### `upload`

| Key | Type | Default | Notes |
|-----|------|---------|-------|
| `max_concurrency` | int | `8` | Parallel upload workers (1–100). |
| `chunk_size` | size | `16MB` | Target chunk size. |
| `enable_adaptive_sizing` | bool | `true` | Auto-tune chunk size to the workload. |
| `max_prefixes` | int | `8` | Upper bound on parallel S3 prefixes. |
| `prefix_pattern` | string | `hash` | Prefix distribution pattern. |
| `compression_type` | string | `zstd` | Archive compression algorithm. |
| `compression_level` | int | `3` | Zstandard level (1–22). |
| `checksum_algorithm` | string | `SHA256` | Integrity checksum. |
| `memory_limit` | size | — | Optional cap on pipeline memory. |

### `metrics`

| Key | Type | Default | Notes |
|-----|------|---------|-------|
| `enabled` | bool | `true` | Publish CloudWatch metrics. |
| `namespace` | string | `CargoShip/Production` | **Required when enabled.** |
| `flush_interval` | duration | `30s` | How often metrics are flushed. |
| `batch_size` | int | `20` | Metrics per batch. |
| `region` | string | — | Defaults to `aws.region`. |
| `dry_run` | bool | `false` | Compute but don't publish metrics. |

### `logging`

| Key | Type | Default | Notes |
|-----|------|---------|-------|
| `level` | string | `info` | One of `debug`, `info`, `warn`, `error`. |
| `structured` | bool | `false` | JSON structured logs. |
| `timestamp` | bool | `true` | Include timestamps. |
| `caller` | bool | `false` | Include caller file/line. |
| `output` | string | — | Log destination (default stderr). |

### `security`

| Key | Type | Default | Notes |
|-----|------|---------|-------|
| `require_encryption` | bool | `false` | Refuse unencrypted uploads. |
| `allowed_regions` | list | — | Restrict which regions may be used. |
| `allowed_storage_classes` | list | all six (below) | Restrict selectable classes. |
| `max_file_size` | size | — | Reject files larger than this. |
| `blocked_extensions` | list | — | Reject these file extensions. |

Valid storage classes: `STANDARD`, `STANDARD_IA`, `ONEZONE_IA`,
`INTELLIGENT_TIERING`, `GLACIER`, `DEEP_ARCHIVE`.

### `cargohold`

Sharding subsystem — see [Multi-prefix sharding](/guides/features/sharding).

| Key | Type | Default | Notes |
|-----|------|---------|-------|
| `enable` | bool | `true` | Use CargoHold sharding. |
| `shard_count` | int | `10` | Shards (1–100). The `upload` command defaults to adaptive (`--shard-count 0`). |
| `shard_strategy` | string | `hash` | One of `hash`, `size`, `type`, `directory`. |
| `compression_level` | int | `3` | Zstandard level (1–22). |

### `magika`

AI file-type detection — see [Magika AI file detection](/guides/features/magika).

| Key | Type | Default | Notes |
|-----|------|---------|-------|
| `enabled` | bool | `false` | Opt-in; requires `pip install magika`. |
| `binary_path` | string | — | Auto-discovered on `PATH` if empty. |
| `batch_size` | int | `100` | Files per batch (1–10000). |
| `timeout` | duration | `30s` | Per-batch timeout. |
| `enable_cache` | bool | `true` | Cache detection results. |
| `use_mime_type` | bool | `false` | Report MIME type instead of content label. |
| `include_scores` | bool | `false` | Include confidence scores. |

## Validation

`cargoship config --validate` enforces: `aws.region` is present;
`storage.default_storage_class` is one of the six valid classes;
`upload.max_concurrency` is 1–100; `metrics.namespace` is set when metrics are
enabled; `logging.level` is one of debug/info/warn/error; `cargohold.shard_count`
is 1–100 and `shard_strategy` is one of hash/size/type/directory;
`cargohold.compression_level` is 1–22; `magika.batch_size` is 1–10000 and
`magika.timeout` parses as a duration.

::: tip
`cargoship config --generate` writes a fully-commented example with every section,
so you can start from a working file rather than build one by hand.
:::

## See also

- [Config files & precedence](/guides/config/files)
- [Environment variables](/reference/environment-variables)
- [Interactive setup wizard](/guides/config/setup)
- Reference: [Configuration & context commands](/reference/commands/config)
