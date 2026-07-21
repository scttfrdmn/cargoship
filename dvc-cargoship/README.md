# dvc-cargoship

A [DVC](https://dvc.org) remote plugin that routes cache operations through
[CargoShip](https://github.com/scttfrdmn/cargoship) for high-performance
bulk S3 uploads.

CargoShip archives data into compressed, sharded `tar.zst` chunks and streams
them to S3 with parallel uploads and intelligent deduplication.  When used as
a DVC remote, large datasets push 5–8× faster than the native S3 remote and
incremental syncs skip unchanged files entirely.

## Requirements

- Python 3.9+
- DVC 3.x (`pip install dvc`)
- CargoShip binary on PATH ([download releases](https://github.com/scttfrdmn/cargoship/releases))
- AWS credentials accessible to the CargoShip binary (env vars, `~/.aws/credentials`, or IAM role)

## Installation

```bash
pip install dvc-cargoship
```

For a development editable install:

```bash
git clone https://github.com/scttfrdmn/cargoship
cd cargoship/dvc-cargoship
pip install -e ".[dev]"
```

## Quick Start

```bash
# 1. Initialize a DVC project (if not already done)
dvc init

# 2. Add the CargoShip remote
dvc remote add -d myremote cargoship://my-bucket/dvc-cache

# 3. Push data
dvc push

# 4. Pull data on another machine
dvc pull
```

## Configuration

All options are set via `dvc remote modify`:

```bash
dvc remote modify myremote <option> <value>
```

### Available Options

| Option | Default | Description |
|--------|---------|-------------|
| `small_file_threshold` | `10MB` | Files smaller than this are batched into a single CargoShip upload. Accepts bytes or a size string (`10MB`, `512KB`, `1GB`). Set to `0` to disable batching. |
| `download_workers` | `4` | Number of parallel threads for `dvc pull` operations. |
| `cargoship_bin` | `cargoship` | Name or absolute path of the CargoShip binary. Useful when the binary is not on PATH. |
| `project_id` | *(none)* | Project ID for cost tracking. Uploads tagged with this ID appear in `cargoship budget status`. |
| `enable_budget_check` | `false` | When `true`, checks the project budget before each upload and raises an error if the budget is exceeded. Requires `project_id`. |

### Example

```bash
# Tune batch threshold and parallelism
dvc remote modify myremote small_file_threshold 50MB
dvc remote modify myremote download_workers 8

# Use a specific CargoShip binary
dvc remote modify myremote cargoship_bin /usr/local/bin/cargoship

# Enable budget tracking for a research project
dvc remote modify myremote project_id my_research_data
dvc remote modify myremote enable_budget_check true
```

## Migration Guide

### From the Native DVC S3 Remote

If you are currently using `dvc remote add myremote s3://my-bucket/dvc-cache`:

**Step 1 — Install the plugin:**

```bash
pip install dvc-cargoship
```

**Step 2 — Add a new CargoShip remote pointing at the same bucket:**

```bash
dvc remote add cs-remote cargoship://my-bucket/dvc-cache
dvc remote default cs-remote
```

> The bucket and prefix path are identical; only the URL scheme changes from
> `s3://` to `cargoship://`.

**Step 3 — Push all data through the new remote:**

```bash
dvc push --all-branches --all-tags
```

**Step 4 — Remove the old S3 remote:**

```bash
dvc remote remove myremote
```

**Step 5 — Commit the updated `.dvc/config`:**

```bash
git add .dvc/config
git commit -m "chore: migrate DVC remote to CargoShip"
```

### Verifying the Migration

```bash
# Check the remote is reachable and has files
dvc remote list
dvc status --cloud
```

### Rollback

The CargoShip remote is fully compatible with the underlying S3 objects.  If
you need to roll back, re-add the S3 remote and configure it with the same
bucket and prefix — all previously uploaded files remain accessible.

## Budget Integration

CargoShip can track S3 costs per project and stop uploads when a budget is
exceeded.

### Setup

```bash
# Set a budget for the DVC cache project
cargoship budget set my_research_data --cost 500.00 --volume 500

# Configure the DVC remote to track against this project
dvc remote modify myremote project_id my_research_data
dvc remote modify myremote enable_budget_check true
```

### How It Works

Before each upload, `dvc-cargoship` calls `cargoship budget status
<project_id> --json` to check the current spend.  If the project is over
budget (or the estimated cost of the pending upload would exceed the budget),
`DVCBudgetExceededError` is raised with a detailed message:

```
DVCBudgetExceededError: DVC budget exceeded for project 'my_research_data'
(operation: push): max=$500.00, spent=$510.50, remaining=$0.00
```

All DVC uploads are automatically tagged with:

| Tag | Value |
|-----|-------|
| `dvc_cache` | `true` |
| `dvc_operation` | `push` or `pull` |
| `dvc_project` | the configured `project_id` (if set) |

This allows you to view DVC vs. direct upload costs separately:

```bash
cargoship budget status my_research_data
cargoship cost projects
```

## Performance Tuning

### Small File Batching

DVC's cache layout stores each file as a separate content-addressed blob.
For datasets with many small files (e.g. thousands of 10–100 KB images),
calling `cargoship upload` once per file creates too much per-invocation
overhead.

`dvc-cargoship` addresses this by accumulating small files in an in-memory
staging area and flushing them as a single `cargoship upload` call.

```bash
# Increase threshold for workloads with many medium-sized files
dvc remote modify myremote small_file_threshold 50MB

# Disable batching for a remote with only large files
dvc remote modify myremote small_file_threshold 0
```

### Parallel Downloads

```bash
# Match worker count to available CPU cores for large restores
dvc remote modify myremote download_workers 16
```

### Incremental Uploads

For iterative workflows where only a subset of files change between
experiments, CargoShip's incremental mode skips files that haven't changed:

```bash
# Not directly configurable via dvc remote modify today.
# Use cargoship upload --incremental directly for manual incremental transfers.
cargoship upload ./data s3://my-bucket/dvc-cache --incremental
```

## Architecture

```
DVC push/pull
     │
     ▼
CargoShipFileSystem (fsspec)
     │
     ├─ Small files (<threshold) ──► BatchUploadBuffer ──► cargoship upload (batched)
     │
     └─ Large files (≥threshold) ──► cargoship upload (immediate)

DVC pull / get_files
     │
     ▼
parallel_restore (ThreadPoolExecutor)
     │
     └─ cargoship restore --file <path> (N workers)
```

The plugin is registered as a [fsspec](https://filesystem-spec.readthedocs.io/)
filesystem under the `cargoship://` URL scheme via the `dvc.fs` entry point.
DVC discovers it automatically when the package is installed.

## Limitations

- **Immutable archives** — CargoShip archives cannot be modified.  `dvc gc`
  (garbage collection) is not supported; `rm()` raises `NotImplementedError`.
  To "remove" a file, create a new incremental upload that omits it.
- **DVC URL validation** — DVC validates remote URLs against an internal
  schema (see [iterative/dvc#9711](https://github.com/iterative/dvc/issues/9711)).
  Until that is resolved, a patched DVC build may be needed for strict
  URL validation.
- **Copy within remote** — `copy()` is not implemented.  DVC's de-duplication
  works through content hashes so in-remote copies are rarely needed.

## Development

```bash
# Install with dev extras
cd dvc-cargoship
pip install -e ".[dev]"

# Run tests
pytest tests/ -v

# Run a specific test class
pytest tests/test_remote.py::TestPutFile -v
```

### Project Structure

```
dvc-cargoship/
├── dvc_cargoship/
│   ├── __init__.py      # Public API surface
│   ├── _version.py      # Package version
│   ├── remote.py        # CargoShipFileSystem (fsspec + DVC integration)
│   ├── cli.py           # CargoShipCLI subprocess wrapper
│   ├── perf.py          # BatchUploadBuffer, parallel_restore, parse_size
│   └── budget.py        # DVCBudgetChecker, DVCBudgetExceededError
├── tests/
│   ├── test_remote.py
│   ├── test_cli.py
│   ├── test_perf.py
│   ├── test_budget.py
│   └── test_integration.py  # End-to-end mock workflow tests
├── pyproject.toml
├── setup.py
└── requirements.txt
```

## License

MIT — see [LICENSE](../LICENSE) in the repository root.
