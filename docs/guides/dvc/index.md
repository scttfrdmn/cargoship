# DVC integration

CargoShip works with [DVC](https://dvc.org) (Data Version Control) in two
complementary ways, so you can choose how deeply to integrate:

- **The Go `dvc` command** (built into the CargoShip binary) reads DVC pipeline
  metadata that has been embedded in an upload's manifest — list stages, compare
  local files against the archive, and generate `.dvc` sidecar files. Upload with
  `--dvc-auto` to record each file's stage from `dvc.yaml`. See
  [Go `dvc` command](/guides/dvc/command).

- **The Python `dvc-cargoship` plugin** registers CargoShip as a DVC *remote*
  (the `cargoship://` URL scheme) so `dvc push` / `dvc pull` route cache
  operations through CargoShip's chunked, compressed, parallel uploads — with
  optional per-project budget checks. See
  [Python dvc-cargoship plugin](/guides/dvc/plugin).

```bash
# Annotate an upload with DVC stage provenance (Go command)
cargoship upload ./data s3://my-bucket/dataset --dvc-auto

# Or use CargoShip as a DVC remote (Python plugin)
pip install dvc-cargoship
dvc remote add -d myremote cargoship://my-bucket/dvc-cache
dvc push
```

::: warning Draft
This overview page is being expanded. For flags, see the
[DVC command reference](/reference/commands/dvc); for a full walkthrough, see
[ML datasets with DVC](/tutorials/ml-dvc).
:::

## See also

- [Go `dvc` command](/guides/dvc/command).
- [Python dvc-cargoship plugin](/guides/dvc/plugin).
- [ML datasets with DVC](/tutorials/ml-dvc) — end-to-end tutorial.
- Reference: [DVC commands](/reference/commands/dvc).
