# Estimating costs

Before you commit to a large upload, `cargoship estimate` tells you what it will
cost to store on S3 — across storage classes, with CargoShip's compression and
chunking factored in. It's read-only and touches no data, so run it as often as
you like.

```bash
cargoship estimate ./my-data
```

CargoShip scans the directory, models how it will [compress](/guides/features/compression)
and [chunk](/guides/features/sharding), and prints monthly storage, request, and
transfer costs. For the full flag list, see the
[cost command reference](/reference/commands/cost).

## Compare naive upload vs. CargoShip

The most useful thing `estimate` does is show what CargoShip's chunking and
compression save you versus uploading files as-is:

```bash
cargoship estimate ./my-data --show-comparison
```

This contrasts a "naive" upload (one S3 object per file, uncompressed) against
CargoShip's approach (compressed `tar.zst` chunks). For datasets with many small
files, the difference in **request costs** alone is often dramatic — thousands of
PUT requests collapse into a handful of chunk uploads.

::: tip Why request count matters
S3 bills per 1,000 PUT/GET requests. Ten thousand tiny files uploaded
individually is ten thousand PUTs; the same files packed into a dozen chunks is a
dozen PUTs. `--show-comparison` makes that visible before you pay for it.
:::

## Pick a storage class

```bash
cargoship estimate ./my-data --storage-class GLACIER
```

Pass any S3 storage class to see its monthly cost. By default the estimate also
prints recommendations across classes so you can weigh storage price against
retrieval cost and latency. Common choices:

| Class | Best for | Relative storage cost |
|-------|----------|-----------------------|
| `STANDARD` | Frequent access | Baseline |
| `INTELLIGENT_TIERING` | Unknown / mixed access | Auto-tiers |
| `STANDARD_IA` | Monthly access | Lower |
| `GLACIER_IR` | Archive, occasional instant retrieval | Much lower |
| `GLACIER` | Cold archive (minutes to retrieve) | Very low |
| `DEEP_ARCHIVE` | Compliance, rarely retrieved (hours) | Lowest |

See [Lifecycle & storage classes](/guides/cost/lifecycle) for choosing between
them and automating transitions.

## Real-time pricing

By default, estimates use CargoShip's built-in pricing table (fast, no
credentials needed). To pull current rates from the AWS Pricing API for your
exact region:

```bash
cargoship estimate ./my-data --storage-class GLACIER_IR --real-time-pricing --region us-west-2
```

::: warning Requires AWS credentials
`--real-time-pricing` calls the AWS Pricing API, so valid credentials and network
access are required. Without the flag, CargoShip falls back to its bundled
pricing — accurate enough for planning, but not penny-exact for negotiated or
enterprise rates.
:::

## JSON output for scripts

```bash
cargoship estimate ./my-data --storage-class GLACIER_IR --format json
```

Machine-readable output for wiring estimates into CI gates, dashboards, or
approval workflows. The default `--format table` is meant for humans.

## Other options

- `--region` — region for pricing (default `us-east-1`); match your target bucket.
- `--bandwidth` — network bandwidth in MB/s to model upload time (0 = auto-detect).
- `--show-recommendations` / `--show-parallel` / `--show-upload-optimization` — on by
  default; each adds an advisory section (storage-class picks, parallel-upload
  tuning, chunk sizing). Disable any you don't want in scripted output.

## Best practices

::: tip
- **Always estimate before a first large upload** — it's free and takes seconds.
- **Lead with `--show-comparison`** so stakeholders see the chunking/compression win.
- **Set `--region` to your bucket's region** — pricing varies by region.
- **Reserve `--real-time-pricing` for final sign-off**; the built-in table is fine
  for exploration and needs no credentials.
- **Feed estimates into budgets**: size a project budget from the estimate, then
  enforce it — see [Budgets & volume quotas](/guides/cost/budgets).
:::

## See also

- [Cost management & reporting](/guides/cost/management) — track actual spend after uploading.
- [Budgets & volume quotas](/guides/cost/budgets) — turn an estimate into an enforced limit.
- [Lifecycle & storage classes](/guides/cost/lifecycle) — reduce storage cost over time.
- Reference: [Cost, budget & alerts commands](/reference/commands/cost).
