# Browsing archives

You can explore an upload interactively before restoring anything. CargoShip
offers two ways in: a filesystem-style shell and a full terminal UI, both reading
the manifest so navigation is fast and no chunks download until you extract a file.

## Shell

`cargoship shell` opens an interactive prompt against an archive. It behaves like
a tiny Unix shell over the manifest:

```bash
cargoship shell s3://my-bucket/archives/uploads/20260721-a1b2c3
```

Commands inside the shell:

- `ls [path]` — list files and directories
- `cd DIR` / `pwd` — move around
- `cat FILE` / `head FILE [n]` — stream file contents
- `stat FILE` — size, hash, chunk, DVC stage, Git commit
- `find PATTERN` — glob search (e.g. `*.csv`, `data/*.parquet`)
- `stage list` / `stage NAME` — inspect DVC pipeline stages
- `get FILE [dst]` — extract a single file locally

## Browse (TUI)

`cargoship browse` opens a full-screen terminal UI to navigate the file list,
select files (space), search (`/`), filter by DVC stage or Git commit, and confirm
a restore (enter). It supports the same Glacier options as `restore`
(`--tier`, `--wait`, `--max-restore-cost`).

```bash
cargoship browse s3://my-bucket/archives/uploads/20260721-a1b2c3 ./restored
```

::: warning Draft
This page is being expanded. For the complete flag list on `shell` and `browse`,
see the [Inspection & retrieval command reference](/reference/commands/inspect).
:::

## See also

- [Restoring files (incl. Glacier)](/guides/restoring).
- [Downloading & extracting](/guides/downloading).
- Reference: [Inspection & retrieval](/reference/commands/inspect).
