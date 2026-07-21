# DVC integration

CargoShip works with [DVC](https://dvc.org) (Data Version Control) in two
complementary ways, so you can choose how deeply to integrate. They are
independent — use either, or both.

## The two integration paths

### A. The Go `dvc` command (built into the CargoShip binary)

CargoShip can record DVC pipeline provenance directly in an upload's manifest.
Upload with `--dvc-auto` and CargoShip auto-discovers stages from `dvc.yaml`,
annotating every file entry with the stage that produced it. Afterward, the
`cargoship dvc` command group reads that metadata back — list stages, compare
local files against the archive, and regenerate `.dvc` sidecar files.

```bash
# Annotate an upload with DVC stage provenance
cargoship upload ./data s3://my-bucket/dataset --dvc-auto
```

This path lives entirely in this repository (`cargoship dvc` plus the `--dvc-auto`
upload flag). See [Go `dvc` command](/guides/dvc/command) for the full workflow.

### B. The Python `dvc-cargoship` remote plugin

The `dvc-cargoship` plugin registers CargoShip as a DVC *remote* (the
`cargoship://` URL scheme) so `dvc push` / `dvc pull` route cache operations
through CargoShip's chunked, compressed, parallel uploads — with optional per-project
budget checks.

```bash
pip install dvc-cargoship
dvc remote add -d myremote cargoship://my-bucket/dvc-cache
dvc push
```

The plugin is documented separately on the
[Python dvc-cargoship plugin](/guides/dvc/plugin) page — see there for install,
configuration, and remote setup.

## Which should I use?

- Want to **version and track cache** through DVC's own `push`/`pull` commands?
  Use the **Python plugin (path B)**.
- Want to **archive a dataset with CargoShip** while preserving which pipeline
  stage produced each file, then inspect or restore by stage? Use the **Go `dvc`
  command (path A)**.

## See also

- [Go `dvc` command](/guides/dvc/command).
- [Python dvc-cargoship plugin](/guides/dvc/plugin).
- [ML datasets with DVC](/tutorials/ml-dvc) — end-to-end tutorial.
- Reference: [DVC commands](/reference/commands/dvc).
