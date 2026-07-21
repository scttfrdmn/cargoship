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

## Inspecting stages

```bash
# List pipeline stages and their file counts
cargoship dvc stages s3://my-bucket/dataset/uploads/20260721-a1b2c3

# Compare local files against the archive (unchanged / modified / missing)
cargoship dvc status ./data s3://my-bucket/dataset/uploads/20260721-a1b2c3 --stage train

# Download the manifest and regenerate .dvc sidecar files
cargoship dvc export s3://my-bucket/dataset/uploads/20260721-a1b2c3 ./dvc-files
```

Once stages are recorded, you can also restore everything a stage produced with
[`cargoship restore --dvc-stage`](/guides/restoring).

::: warning Draft
This page is being expanded. For the complete flag list on `dvc stages`,
`dvc status`, and `dvc export`, see the [DVC command reference](/reference/commands/dvc).
:::

## See also

- [DVC integration overview](/guides/dvc/).
- [Python dvc-cargoship plugin](/guides/dvc/plugin).
- [Restoring files](/guides/restoring) — restore by `--dvc-stage`.
- Reference: [DVC commands](/reference/commands/dvc).
