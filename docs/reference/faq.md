# FAQ

## What format does CargoShip write?

Open, documented, and portable ones. An upload is a set of `tar.zst` objects
(tar archives compressed with Zstandard) plus a JSON manifest that indexes every
file. Nothing is proprietary — the [archive layout](/reference/format/archive-layout)
and [manifest schema](/reference/format/manifest) are fully specified.

## Can I extract my data without CargoShip?

Yes. Because chunks are ordinary `tar.zst` objects, you can download one from S3
and unpack it with standard tools (`zstd -d`, then `tar -xf`) — no CargoShip
binary required. The manifest tells you which chunk holds which file. CargoShip
just makes selective, checksum-verified extraction convenient.

## Is CargoShip a service or a subscription?

Neither. It's a self-contained Go command-line tool (Apache 2.0 licensed) that
runs on your machine and talks directly to your own S3 bucket using your own AWS
credentials. There is no CargoShip server, account, or hosted component in the
data path.

## Does CargoShip delete or modify my source files?

No. CargoShip only reads your source directory — it never modifies or deletes
local files during an upload. Destructive S3-side operations (like `cargoship
delete` or `scuttle`) are separate, explicit commands; see
[Destructive operations](/reference/commands/destructive) and
[Costs & safety guarantees](/intro/costs-and-safety).

## See also

- [Concepts & terminology](/intro/concepts).
- [How it works](/intro/how-it-works).
- [Troubleshooting](/reference/troubleshooting).
- [Glossary](/reference/glossary).
