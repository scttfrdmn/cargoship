# Python dvc-cargoship plugin

`dvc-cargoship` is a [DVC](https://dvc.org) remote plugin that routes cache
operations through CargoShip. It registers a `cargoship://` URL scheme (via
[fsspec](https://filesystem-spec.readthedocs.io/)) so `dvc push` / `dvc pull`
archive data into compressed, sharded `tar.zst` chunks and stream them to S3 with
parallel uploads and deduplication — and incremental syncs skip unchanged files.

## Requirements

- Python 3.9+
- DVC 3.x (`pip install dvc`)
- The CargoShip binary on PATH ([releases](https://github.com/scttfrdmn/cargoship/releases))
- AWS credentials accessible to the CargoShip binary (env vars, `~/.aws/credentials`, or an IAM role)

## Installation

```bash
pip install dvc-cargoship
```

## Quick start

```bash
# 1. Initialize a DVC project (if not already done)
dvc init

# 2. Add the CargoShip remote
dvc remote add -d myremote cargoship://my-bucket/dvc-cache

# 3. Push and pull data
dvc push
dvc pull
```

## Configuration

Set options with `dvc remote modify myremote OPTION VALUE`:

| Option | Default | Description |
|--------|---------|-------------|
| `small_file_threshold` | `10MB` | Files smaller than this are batched into a single CargoShip upload. Accepts bytes or a size string (`10MB`, `512KB`, `1GB`). Set to `0` to disable batching. |
| `download_workers` | `4` | Parallel threads for `dvc pull` operations. |
| `cargoship_bin` | `cargoship` | Name or absolute path of the CargoShip binary (useful when it is not on PATH). |
| `project_id` | *(none)* | Project ID for cost tracking. Uploads tagged with this ID appear in `cargoship budget status`. |
| `enable_budget_check` | `false` | When `true`, checks the project budget before each upload and raises an error if exceeded. Requires `project_id`. |

```bash
dvc remote modify myremote small_file_threshold 50MB
dvc remote modify myremote download_workers 8
dvc remote modify myremote cargoship_bin /usr/local/bin/cargoship
```

## Migrating from the native S3 remote

The bucket and prefix stay the same — only the URL scheme changes from `s3://`
to `cargoship://`, and the plugin is fully compatible with the underlying S3
objects (so rollback is just re-adding the S3 remote).

```bash
pip install dvc-cargoship
dvc remote add cs-remote cargoship://my-bucket/dvc-cache
dvc remote default cs-remote
dvc push --all-branches --all-tags
dvc remote remove myremote
git add .dvc/config && git commit -m "chore: migrate DVC remote to CargoShip"
```

## Budget integration

CargoShip can track S3 costs per project and stop uploads when a budget is
exceeded. Set a budget, then point the remote at that project:

```bash
cargoship budget set --project my_research_data --max-budget 500.00
dvc remote modify myremote project_id my_research_data
dvc remote modify myremote enable_budget_check true
```

Before each upload the plugin checks `cargoship budget status PROJECT --json`; if
the project is over budget it raises `DVCBudgetExceededError`. All DVC uploads are
tagged `dvc_cache=true`, `dvc_operation=push|pull`, and (if set)
`dvc_project=PROJECT`, so you can separate DVC spend from direct uploads:

```bash
cargoship budget status my_research_data
cargoship cost projects
```

## Limitations

- **Immutable archives** — CargoShip archives cannot be modified, so `dvc gc` is
  not supported and `rm()` raises `NotImplementedError`. To "remove" a file,
  create a new incremental upload that omits it.
- **In-remote copy** — `copy()` is not implemented; DVC's content-hash
  deduplication rarely needs it.
- **DVC URL validation** — DVC validates remote URLs against an internal schema
  ([iterative/dvc#9711](https://github.com/iterative/dvc/issues/9711)); strict
  validation may require a patched DVC build until that is resolved.

## See also

- [DVC integration overview](/guides/dvc/).
- [Go `dvc` command](/guides/dvc/command).
- [Budgets & volume quotas](/guides/cost/budgets).
- Reference: [DVC commands](/reference/commands/dvc).
