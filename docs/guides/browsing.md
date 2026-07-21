# Browsing archives

You can explore an upload interactively before restoring anything. CargoShip
offers two ways in: a filesystem-style shell and a full terminal UI. Both read
the manifest, so navigation is instant and no chunks download until you extract a
file. The full flag lists live in the [command reference](/reference/commands/inspect).

## Shell

`cargoship shell` opens an interactive prompt against an archive. It behaves like
a tiny Unix shell over the manifest:

```bash
cargoship shell s3://my-bucket/archives/uploads/20260721-a1b2c3
```

Commands available at the prompt:

| Command | What it does |
|---------|--------------|
| `ls [path]` | List files and directories |
| `cd DIR` / `pwd` | Change / print the current directory |
| `cat FILE` | Stream a file's contents to stdout |
| `head FILE [n]` | Print the first `n` lines (default 10) |
| `stat FILE` | Show size, hash, chunk, DVC stage, Git commit |
| `find PATTERN` | Glob search (e.g. `*.csv`, `data/*.parquet`) |
| `stage list` | List all DVC pipeline stages and their file counts |
| `stage NAME` | List files belonging to a DVC stage |
| `get FILE [dst]` | Extract a single file locally (default: current directory) |
| `help` | Show the command list |
| `exit` / `quit` | Leave the shell |

A short session looks like this:

```
cargoship:/$ ls data
train.csv   test.csv   features/
cargoship:/$ stat data/train.csv
  path:   data/train.csv
  size:   214 MB
  hash:   d8e8fca2dc0f896fd7cb4cb0031ba249
  chunk:  shard-03/chunk-012
  stage:  preprocess
cargoship:/$ get data/train.csv ./
extracted data/train.csv → ./train.csv (214 MB)
cargoship:/$ exit
```

Flags: `-r`/`--region` (default `us-east-1`) and `--cache-gb` to size the LRU
chunk cache (default 10 GB). Called without an S3 URL, `cargoship shell` starts
the generic CargoShip REPL instead of an archive shell.

## Browse (TUI)

`cargoship browse` opens a full-screen terminal UI to navigate the file list,
select files, search, filter by DVC stage or Git commit, and confirm a restore:

```bash
cargoship browse s3://my-bucket/archives/uploads/20260721-a1b2c3 ./restored
```

Keyboard controls:

| Key | Action |
|-----|--------|
| `↑` / `↓` | Move through the file list |
| `space` | Toggle selection on the highlighted file |
| `enter` | Confirm restore of selected files |
| `/` | Incremental search |
| `d` | Cycle the DVC-stage filter |
| `g` | Cycle the Git-commit filter |
| `a` | Select all visible files |
| `c` | Clear the selection |
| `q` / `ctrl+c` | Quit without restoring |

Because `browse` restores, it supports the same Glacier options as
[`restore`](/guides/restoring#glacier) — retrieval works even when files are in
Glacier or Deep Archive:

```bash
# Larger cache for a big dataset
cargoship browse s3://my-bucket/.../20260721-a1b2c3 ./restored --cache-gb 20

# Restore from Glacier, standard tier, wait for the thaw
cargoship browse s3://my-bucket/.../20260721-a1b2c3 --tier standard --wait
```

- `--tier` — `expedited`, `standard` (default), `bulk`.
- `--wait` — block until the Glacier thaw completes, then download.
- `--restore-days` — days to keep the restored copy (default 7).
- `--max-restore-cost` — abort if the estimated retrieval cost exceeds this USD limit.
- `--cache-gb`, `-r`/`--region` — as for `shell`.

## Which one to use

- **`shell`** — scripting-adjacent, precise: inspect metadata, `cat`/`head` to
  peek at contents, and `get` individual files. Best when you know roughly what
  you're after.
- **`browse`** — visual, multi-select: skim a large file list, filter by
  stage/commit, and restore a batch in one confirm. Best for exploratory picks.

## Best practices

::: tip
- **Browse before a big restore** to select exactly the files you need and avoid
  paying to retrieve chunks you'll discard.
- **Set `--max-restore-cost`** in `browse` when the archive may be in Glacier —
  it's your guard against a costly accidental thaw.
- **Bump `--cache-gb`** for large archives so repeated `cat`/`get` calls reuse
  already-downloaded chunks.
- **Use `stat` / `stage list`** in the shell to confirm provenance before you
  extract.
:::

## See also

- [Restoring files (incl. Glacier)](/guides/restoring).
- [Downloading & extracting](/guides/downloading).
- [Concepts: manifest](/intro/concepts#manifest).
- Reference: [Inspection & retrieval](/reference/commands/inspect).
