# Analyzing existing S3 spend

`cargoship analyze` scans an S3 bucket you already have, calculates what it costs
today, and estimates how much re-archiving it with CargoShip's chunked, compressed
format could save. It works against AWS S3 and S3-compatible providers (Wasabi,
Backblaze B2, MinIO).

```bash
cargoship analyze s3://my-bucket
cargoship analyze s3://my-bucket/data --format json
```

For very large buckets, `--sampling` estimates from a subset instead of scanning
every object (tune with `--sample-size`). Point at a non-AWS provider with
`--provider` plus `--endpoint-url`:

```bash
cargoship analyze s3://my-bucket --sampling --sample-size 10000
cargoship analyze s3://my-bucket --provider wasabi \
  --endpoint-url https://s3.wasabisys.com
```

::: warning Draft
This page is being expanded. For the complete flag list, see the
[Cost, budget & alerts command reference](/reference/commands/cost).
:::

## See also

- [Estimating costs](/guides/cost/estimate) — estimate before an upload.
- [Cost management & reporting](/guides/cost/management).
- [Lifecycle & storage classes](/guides/cost/lifecycle).
- Reference: [Cost, budget & alerts](/reference/commands/cost).
