---
prev:
  text: Verify & restore it
  link: /start/verify-and-restore
---

# Clean up

If your first upload was just a test, here's how to remove it — safely.

## Preview first

Every destructive command supports `--dry-run`, which shows exactly what would be
removed without touching anything:

```bash
cargoship delete s3://my-bucket/archives/uploads/20260721-a1b2c3 --dry-run
```

## Delete one upload

```bash
cargoship delete s3://my-bucket/archives/uploads/20260721-a1b2c3
```

This removes the chunks and manifest for that single upload ID. You'll be prompted
to confirm. `delete` is scoped to one upload — it won't touch other uploads in the
same bucket.

::: danger scuttle removes everything
[`cargoship scuttle`](/reference/commands/destructive) deletes **all** CargoShip
data under a bucket/prefix, not just one upload. It requires triple confirmation
and should be reserved for tearing down a whole archive location. Always
`--dry-run` it first.
:::

## Automation

For scripted teardown, `--force` skips the confirmation prompt. Only use it when
you're certain of the target — prefer running a `--dry-run` in the same script and
reviewing its output before the real run.

## You're done with the basics

You've uploaded, verified, restored, and cleaned up. From here:

- Pick a [use-case tutorial](/tutorials/) for your kind of data.
- Read [Uploading data](/guides/uploading) for the full set of upload options.
- Set up [budgets and cost tracking](/guides/cost/budgets) before archiving at scale.
