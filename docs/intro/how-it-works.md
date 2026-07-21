# How it works

CargoShip turns a directory on disk into a set of compressed archive objects in
S3, plus a **manifest** that records exactly where every file went. Understanding
the pipeline makes every other page — flags, costs, restores — easier to reason
about.

## The streaming pipeline

An upload flows through four stages, connected by in-memory pipes
(`io.Pipe`) rather than temporary files on disk:

```
  ┌──────────┐   ┌──────────┐   ┌───────────┐   ┌──────────┐
  │ Scanner  │──▶│ Chunker  │──▶│ Archiver  │──▶│ Uploader │──▶ S3
  └──────────┘   └──────────┘   └───────────┘   └──────────┘
   walk files     group into      tar + zstd      parallel,
   + detect       chunks &        per chunk       multi-prefix
   metadata       shards                          PutObject
```

1. **Scanner** walks the source directory, collecting each file's path, size,
   modification time, and (optionally) a content hash and AI-detected type.
2. **Chunker** groups files into *chunks* — the unit that becomes one S3 object —
   and assigns each chunk to a *shard* (an S3 key prefix) for parallelism.
3. **Archiver** streams each chunk into a `tar` container, compressing with
   Zstandard at a level chosen for the chunk's content type.
4. **Uploader** sends chunks to S3 concurrently, spread across shard prefixes,
   and records the result in the manifest.

Because stages are connected by pipes, a chunk is being uploaded while the next
one is still being archived and the scanner is still walking — nothing waits for
the whole dataset, and **nothing is staged to local disk**. Memory use is bounded
by roughly `chunk_size × workers`, not by the dataset size.

## What lands in S3

A completed upload has a predictable layout under a single **upload ID**:

```
s3://my-bucket/archives/uploads/20260721-a1b2c3/
├── manifest.json.gz                     # the index of everything
├── shard-0/
│   ├── chunk-0.tar.zst
│   └── chunk-8.tar.zst
├── shard-1/
│   ├── chunk-1.tar.zst
│   └── chunk-9.tar.zst
└── … (shards 2..N)
```

- Each **chunk** is a standard `tar.zst` object (or plain `.tar` when its contents
  are already-compressed formats).
- Chunks are spread across **shards** (`shard-0`, `shard-1`, …) so uploads and
  restores parallelize across S3 prefixes.
- The **manifest** is a JSON document mapping every original file to its chunk,
  shard, and S3 key, along with sizes, hashes, and compression details.

This layout is a documented, open format — see the
[Archive & Manifest Format Spec](/reference/format/). You can extract your data
with standard `tar`/`zstd` tooling even if CargoShip isn't installed.

## Why sharding matters

S3 scales request throughput per key **prefix**. By writing chunks under multiple
prefixes (`shard-0`, `shard-1`, …) instead of one, CargoShip parallelizes the
request rate rather than serializing through a single prefix. The
[shard count is chosen adaptively](/guides/features/sharding) (4–32) from the
workload's file count, size, and your machine's resources — or you can set it
manually.

## The manifest is the source of truth

Every later operation reads the manifest:

- [`cargoship list` / `info`](/guides/inspecting) — see what's in an upload without downloading.
- [`cargoship verify`](/guides/verifying) — check integrity against recorded checksums.
- [`cargoship restore` / `download`](/guides/restoring) — pull specific files back,
  resolving each to its chunk and shard.
- [`cargoship shell` / `browse`](/guides/browsing) — navigate the archive interactively.

## Next

You have the mental model. Now get a working upload:

- [Quick Start](/start/quickstart) — zero to a verified upload in minutes.
- [Concepts & terminology](/intro/concepts) — precise definitions of upload ID,
  chunk, shard, manifest, and more.
