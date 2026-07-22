# CargoShip vs. other tools

CargoShip isn't a general-purpose S3 client. It's built specifically for
archiving **large directory trees of many files** to S3 cheaply, verifiably, and
reversibly. This page is an honest guide to when it's the right tool and when a
simpler one is.

## At a glance

| | `aws s3 cp/sync` | `rclone` | `tar` → `aws s3 cp` | DVC remote | **CargoShip** |
|---|---|---|---|---|---|
| One object per file | yes | yes | no (one blob) | via cache | **no — packs into chunks** |
| Handles many small files well | request-heavy | request-heavy | yes (opaque) | ok | **yes — chunked** |
| Compression | no | no | manual | no | **yes (content-aware zstd)** |
| Selective single-file restore | yes | yes | **no — full download** | yes | **yes (from a chunk)** |
| Integrity manifest + `verify` | no | check flags | no | hashes | **yes** |
| Cost estimate before upload | no | no | no | no | **yes** |
| Storage-class / tier automation | flags | flags | manual | no | **yes** |
| Resumable / incremental | sync | yes | no | yes | **yes** |
| Open, tool-independent format | yes (raw) | yes (raw) | yes (tar) | opaque cache | **yes (tar.zst + JSON manifest)** |

## When to use each

**`aws s3 cp` / `aws s3 sync`** — the right choice for a handful of files, or when
you want each file to remain an individually-addressable S3 object. Simple,
ubiquitous, no packing. Downsides at scale: one PUT per file (request cost and
throughput), no compression, no cost preview.

**`rclone`** — excellent general-purpose sync across many cloud backends, with
filtering and bandwidth control. Reach for it for ongoing folder sync or
multi-cloud. It doesn't pack/compress into archives or produce a verifiable
manifest, so per-file request overhead and retrieval guarantees are on you.

**`tar` piped to `aws s3 cp`** — fine for "one dataset, one blob, restore the whole
thing." You lose selective restore (you must download the entire archive to get
one file), get no manifest/index, and manage compression and multipart yourself.

**DVC remotes** — the right tool for versioning ML datasets and wiring data into
pipelines. CargoShip complements rather than replaces it: the
[`dvc-cargoship` plugin](/guides/dvc/plugin) can serve as a faster DVC remote, and
`cargoship upload --dvc-auto` records DVC stage provenance in the manifest.

**CargoShip** — the right choice when you have **large trees of many files** to
land in S3 as compressed, indexed, integrity-checked archives, want to **estimate
cost first**, need **selective restore without downloading everything**, and care
that the result stays **readable with standard tools**. Its sweet spot is research
and technical datasets (genomics, imaging, sensor, analytics output) and bulk
archival — cases where per-file copy tools create excessive requests, give weak
recovery guarantees, or make cost hard to predict.

## Migrating

Already using rclone or the AWS CLI? See
[Migrating from rclone / aws cli](/tutorials/migrating).
