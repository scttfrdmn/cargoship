# Idea (RFC): Time Machine compatibility & a Finder-browsable restore mount

> Status: **exploration / not scheduled**. This is a design note, not a committed
> work item — it captures a thought experiment and a recommendation so the reasoning
> isn't lost. It is deliberately *not* a backlog issue.

## Prompt

> "What would it look like to offer a Time Machine-compatible endpoint/mount on macOS
> from CargoShip that then writes the Time Machine files in CargoShip format?"

## TL;DR

- **Emulating a Time Machine *write* target (SMB/AFP share): don't.** What lands on us
  is an opaque sparsebundle disk image, not files — which guts CargoShip's dedup,
  compression, and (most importantly) its per-file trust story, and it forces an
  **inbound listening service** that contradicts the outbound-only agent posture.
- **The idea worth stealing is the inverse:** a **read-only, Finder-browsable restore
  mount** ("enter Time Machine" for CargoShip archives). It plays to our strengths and
  keeps the trust + outbound-only guarantees intact.

## How Time Machine actually works (why the write side is a bad fit)

Time Machine has two target modes:

1. **Network target (SMB/AFP share).** macOS does **not** write your files to the
   share. It creates a **sparsebundle** — a disk image split into ~8 MB "band" files —
   and writes the backup *inside* that image as an APFS/HFS+ filesystem. To be accepted
   as a target, the share must advertise Time Machine support (mDNS `_adisk._tcp` TXT
   records + Samba's `vfs_fruit` SMB extensions). So a network TM endpoint receives
   **opaque band blobs**, not semantic files.
2. **Local / directly-attached target.** APFS snapshots + clonefiles + a specific
   directory structure. Emulating this over a synthetic mount means reproducing the
   APFS semantics TM depends on (snapshots, hardlinks, xattrs).

### Why "TM share → CargoShip format" doesn't pay off

- **Dedup / compression collapse.** Our manifest + chunk engine operates on files.
  Band files are filesystem-internal; a small edit rewrites whole bands, so we'd see
  churn and dedup poorly. We'd effectively be a dumb block store for a disk image.
- **Trust story inverts (project priority #1).** CargoShip's trust anchor is per-file
  manifests + `verify --deep` + byte round-trip. A sparsebundle is one opaque blob; a
  corrupt band can poison the image and individual files can't be independently
  verified. Wrapping TM makes us *inherit TM's opacity*.
- **Breaks outbound-only.** An SMB/AFP endpoint is an **inbound listening service on the
  LAN** — the opposite of the ghostship posture (no inbound port; see the epic and the
  #340 hardening).
- **Largely redundant.** The *experience* TM provides (scheduled, incremental, versioned,
  restorable) is what `ghostship run` already delivers: incremental chains + dataset
  versioning + `restore --version/--as-of`.

### The one non-silly write-side framing

Run a real Samba `vfs_fruit` Time Machine share and have CargoShip **tier the
sparsebundle bands to S3** as cold blocks — i.e. be the *backing store*, not the
format. Legitimate, but it's "cheap S3-backed Time Machine," where CargoShip adds
little over plain S3, and it's high-maintenance: Apple changes TM's accepted feature
surface across releases, so you'd be chasing a spec you don't own.

## The keeper: a read-only, Finder-browsable restore mount

Flip the direction. The compelling "Time Machine feel" is the **browse-and-restore
UX**, and that maps cleanly onto what CargoShip already has:

- A **read-only FUSE / loopback-NFS mount** (macFUSE, or an NFS export whose semantics
  we control) that projects archives as `…/<dataset>/<version>/<original tree>`.
  Finder, `ls`, and `cp` "just work"; dragging a file out restores it.
- Built on the existing chain resolution (`manifest.ResolveChain` / `MergeChain`) and
  the streaming restore path — a natural GUI companion to the `browse` TUI.
- **Preserves the guarantees:** it's a *local read* mount, not a network *write*
  listener (outbound-only in spirit), and every file it shows is a manifest entry that
  can be independently verified (per-file trust preserved).

### Rough sketch (if it were ever picked up)

- Mount provider over the same read APIs `cargoship browse`/`restore` use; lazily fetch
  + decompress chunks on `open`/`read`, size-bounded, LRU-cached.
- Directory tree from the resolved manifest chain; versions as top-level dirs
  (`latest`, `<version-ordinal>`, `as-of/<timestamp>`).
- Read-only, no write path — restore is "copy out," matching the trust model.
- macFUSE is a kernel extension (user-approved); an NFS-loopback variant avoids the
  kext at the cost of more moving parts. Worth prototyping both.

## Recommendation

- **Skip** Time Machine write-target emulation (opaque bands, inbound port, trust
  regression, redundant with ghostship).
- **Consider** the read-side Finder-browsable restore mount as a future enhancement —
  it's additive, trust-preserving, and a strong UX complement to the fleet work. If it
  graduates from "idea" to "planned," it becomes its own scoped issue then.
