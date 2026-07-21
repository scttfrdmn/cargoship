# Resuming interrupted uploads

Large uploads can be interrupted — a dropped connection, a laptop lid, a Ctrl-C.
CargoShip checkpoints progress to `~/.cargoship/state/` as it goes, so re-running
the same `cargoship upload` command automatically picks up where it left off
instead of starting over. The `cargoship resume` command manages those saved
states.

```bash
cargoship resume list                 # show resumable uploads and their progress
cargoship resume 20260721-a1b2c3      # resume a specific interrupted upload
cargoship resume clean --older-than 24h
```

To deliberately ignore a saved state and start fresh, run the upload with
`--force-restart`:

```bash
cargoship upload ./my-data s3://my-bucket/archives/ --force-restart
```

::: warning Draft
This page is being expanded. For the complete flag list on `resume`,
`resume list`, and `resume clean`, see the
[Uploading & sync command reference](/reference/commands/upload).
:::

## See also

- [Uploading data](/guides/uploading).
- Reference: [Uploading & sync commands](/reference/commands/upload).
