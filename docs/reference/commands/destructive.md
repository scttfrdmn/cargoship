# Destructive operations

::: danger These commands permanently remove data from S3
`delete` and `scuttle` cannot be undone. Always run with `--dry-run` first to
preview exactly what will be removed. `--force` skips confirmation — reserve it
for automation you trust, and prefer reviewing a `--dry-run` in the same script
before the real run.
:::

- **`delete`** removes a **single upload** (by upload ID). Scoped — it won't touch
  other uploads in the same bucket.
- **`scuttle`** removes **all** CargoShip data under a bucket/prefix. It requires
  triple confirmation and is intended for tearing down an entire archive location.

See [Clean up](/start/cleanup) for the recommended workflow.

::: tip Generated reference
Flag tables below are generated from the CLI and kept in sync by a drift check.
:::

[[toc]]

<!-- @include: ../../gen/cli/cargoship_delete.md -->

<!-- @include: ../../gen/cli/cargoship_scuttle.md -->
