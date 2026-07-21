# Go `dvc` command

The CargoShip binary includes a `dvc` command group for working with DVC
(Data Version Control) metadata embedded in an upload's manifest. It does not
replace DVC — it lets CargoShip record and read which pipeline stage produced each
file, so you can inspect provenance and drive selective restores by stage.

## Recording stage metadata on upload

Upload with `--dvc-auto` to auto-discover stages from `dvc.yaml` and annotate every
file entry with its stage name:

```bash
cargoship upload ./data s3://my-bucket/dataset --dvc-auto
```

CargoShip parses `dvc.yaml` (and `dvc.lock`), maps each output back to the stage
that produced it, and stores that stage name on the file entry in the manifest.
Once recorded, the `dvc` subcommands below can read it back.

## Inspecting stages

### `dvc stages` — list pipeline stages

```bash
cargoship dvc stages s3://my-bucket/dataset/uploads/20260721-a1b2c3
```

Lists each DVC stage found in the manifest with its file count. Add `--json` for
machine-readable output. Use `--region` if the bucket is not in the default
region (this command defaults to `us-east-1`).

### `dvc status` — compare local files against the archive

```bash
cargoship dvc status ./data s3://my-bucket/dataset/uploads/20260721-a1b2c3 --stage train
```

Reports which local files are unchanged, modified, or missing relative to the
archived manifest. `--stage` filters the comparison to files from one DVC stage;
`--json` and `--region` behave as above.

### `dvc export` — regenerate `.dvc` sidecar files

```bash
cargoship dvc export s3://my-bucket/dataset/uploads/20260721-a1b2c3 ./dvc-files
```

Downloads the manifest and writes `.dvc` sidecar files. The output directory can
be given as the second argument or with `--output-dir` (default `dvc-files`);
`--cache-dir` sets the DVC cache directory recorded in the sidecars (default
`.dvc/cache`).

## Restoring by stage

Once stages are recorded, you can restore everything a stage produced with
[`cargoship restore --dvc-stage`](/guides/restoring).

## See also

- [DVC integration overview](/guides/dvc/).
- [Python dvc-cargoship plugin](/guides/dvc/plugin).
- [Restoring files](/guides/restoring) — restore by `--dvc-stage`.
- Reference: [DVC commands](/reference/commands/dvc).
