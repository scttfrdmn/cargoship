# ML datasets with DVC

**You are:** an ML practitioner versioning datasets and models with
[DVC](https://dvc.org). Your data lives in a DVC cache that you push to S3, and
the native S3 remote is slow on large datasets and wasteful on the thousands of
tiny content-addressed blobs DVC creates.

CargoShip integrates with DVC in two ways:

1. **The Python plugin (`dvc-cargoship`)** — a DVC remote that routes
   `dvc push`/`dvc pull` through CargoShip. Best for teams already living in
   `dvc push`.
2. **The Go `dvc` command + `--dvc-auto`** — annotate a CargoShip upload with
   per-file DVC *stage* metadata, then inspect and restore by stage. Best when
   CargoShip is your primary archiver and you want DVC provenance baked into the
   manifest.

Pick either or use both. This tutorial covers each.

---

## Path A — the `dvc-cargoship` remote plugin

### Step 1 — Install

The plugin is a Python package; CargoShip itself is the Go binary it shells out
to. You need both.

```bash
pip install dvc-cargoship          # DVC 3.x, Python 3.9+
cargoship --version                # binary must be on PATH
```

AWS credentials are read by the CargoShip binary through the standard chain (env
vars, `~/.aws/credentials`, or IAM role).

### Step 2 — Point a DVC remote at CargoShip

The only thing that changes from a native S3 remote is the URL scheme —
`s3://` becomes `cargoship://`; bucket and prefix are identical:

```bash
dvc remote add -d myremote cargoship://my-bucket/dvc-cache
dvc push
```

On another machine, `dvc pull` restores through the same remote.

### Step 3 — Tune for your dataset shape

All options are set with `dvc remote modify`:

```bash
# Batch small blobs into one CargoShip upload (default 10MB threshold)
dvc remote modify myremote small_file_threshold 50MB

# Parallelism for dvc pull
dvc remote modify myremote download_workers 16

# If the binary isn't on PATH
dvc remote modify myremote cargoship_bin /usr/local/bin/cargoship
```

`small_file_threshold` is the key knob: DVC stores every file as a separate
content-addressed blob, so a dataset of thousands of tiny images would mean
thousands of `cargoship upload` invocations. The plugin instead accumulates
sub-threshold files and flushes them as a single batched upload. Set it to `0`
to disable batching for a remote that only holds large files.

### Step 4 — Track DVC cost as its own project

CargoShip's budget system can treat your DVC cache as a project so its spend is
visible separately from direct uploads:

```bash
cargoship budget set my_research_data --cost 500

dvc remote modify myremote project_id my_research_data
dvc remote modify myremote enable_budget_check true
```

With `enable_budget_check true`, the plugin checks the project budget before each
push and raises `DVCBudgetExceededError` if you're over. Every DVC upload is
auto-tagged `dvc_cache=true` and `dvc_operation=push|pull`, so you can separate
DVC from direct spend:

```bash
cargoship budget status my_research_data
cargoship cost projects
```

### Migrating from the native S3 remote

The CargoShip remote is compatible with the underlying S3 objects, so migration
is add-new / push / remove-old:

```bash
pip install dvc-cargoship
dvc remote add cs-remote cargoship://my-bucket/dvc-cache   # same bucket/prefix
dvc remote default cs-remote
dvc push --all-branches --all-tags
dvc remote remove myremote
git add .dvc/config && git commit -m "chore: migrate DVC remote to CargoShip"
```

Rollback is symmetric — re-add the S3 remote at the same bucket/prefix; all
uploaded files remain accessible.

::: warning Plugin limitations
- **Immutable archives.** `dvc gc` is unsupported and `rm()` raises
  `NotImplementedError` — "remove" a file by making a new incremental upload that
  omits it.
- **In-remote `copy()` is not implemented** (DVC dedups by content hash, so it's
  rarely needed).
- DVC validates remote URLs against an internal schema; strict setups may need a
  patched DVC build (see
  [iterative/dvc#9711](https://github.com/iterative/dvc/issues/9711)).
:::

See [Python dvc-cargoship plugin](/guides/dvc/plugin) for the full reference.

---

## Path B — DVC stage metadata in a CargoShip upload

If CargoShip is your primary archiver, you don't need the remote at all. You can
embed DVC *pipeline stage* provenance directly into a normal upload's manifest,
then inspect and restore by stage.

### Step 1 — Upload with `--dvc-auto`

Run from a directory containing a `dvc.yaml`. CargoShip discovers the stages and
tags each file entry with the stage that produced it:

```bash
cargoship upload ./ml-project s3://ml-datasets/project-x/ \
  --dvc-auto \
  --git-metadata \
  --project project-x
```

- `--dvc-auto` reads `dvc.yaml` and annotates each file with its stage name.
- `--git-metadata` embeds the commit/branch/tag/remote in the manifest, so the
  archive records exactly which code revision produced the data.

To pull provenance from one specific stage's `dvc.yaml` + `dvc.lock` instead,
use `--dvc-stage <name>`. To emit `.dvc` sidecar files after upload, add
`--generate-dvc-files`.

### Step 2 — Inspect stages in the archive

```bash
# List pipeline stages and how many files each produced
cargoship dvc stages s3://ml-datasets/project-x/uploads/<id>

# Compare local working files against the archived manifest
cargoship dvc status ./ml-project s3://ml-datasets/project-x/uploads/<id>
cargoship dvc status ./ml-project s3://ml-datasets/project-x/uploads/<id> --stage train
```

`dvc status` reports each file as `unchanged`, `modified`, or `missing` versus
the manifest's content hashes — a quick way to see what's drifted since the
archive was made. Add `--json` to any of these for scripting.

### Step 3 — Restore a whole stage, or a commit

Because the manifest carries stage and git metadata, you can restore by
provenance rather than by path:

```bash
# Everything produced by the "train" stage
cargoship restore s3://ml-datasets/project-x/uploads/<id> ./out --dvc-stage train

# Everything from a specific git commit
cargoship restore s3://ml-datasets/project-x/uploads/<id> ./out --git-commit 9f3a2c1
```

You can still restore by path (`--file`) or content hash (`--hash`) as usual —
see [Restoring files](/guides/restoring).

### Step 4 — Regenerate `.dvc` sidecars from an archive

To reconstruct DVC tracking files from an uploaded manifest (e.g. on a fresh
checkout):

```bash
cargoship dvc export s3://ml-datasets/project-x/uploads/<id> ./dvc-files
```

See [Go `dvc` command](/guides/dvc/command) for the full reference.

---

## Which path should I use?

| You want… | Use |
|-----------|-----|
| `dvc push`/`dvc pull` to just get faster | Path A — the plugin remote |
| DVC spend tracked as a budgeted project | Path A — `project_id` + `enable_budget_check` |
| Provenance (stage, git commit) baked into a CargoShip archive | Path B — `--dvc-auto --git-metadata` |
| Restore data by stage or commit, not path | Path B — `restore --dvc-stage` / `--git-commit` |

## Next steps

- [DVC integration overview](/guides/dvc/) · [Plugin](/guides/dvc/plugin) · [Go command](/guides/dvc/command).
- [Incremental sync](/guides/sync) — for iterative experiment workflows.
- [Budgets & quotas](/guides/cost/budgets) · [Restoring files](/guides/restoring).
