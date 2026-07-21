# Observability & tracing

CargoShip can emit OpenTelemetry traces and Prometheus metrics during an upload so
you can see where time goes across the scan, chunk, archive, and upload stages and
watch throughput on a dashboard. Both are off by default; turn them on per run with
flags on `cargoship upload`.

```bash
cargoship upload ./my-data s3://my-bucket/archives/ \
  --tracing --tracing-exporter otlp \
  --tracing-endpoint http://localhost:4318 \
  --prometheus-addr :9090
```

## Distributed tracing

Tracing instruments the pipeline with OpenTelemetry spans so you can follow a run
stage by stage in a tracing backend.

- `--tracing` — enable distributed tracing.
- `--tracing-exporter` — where spans go: `stdout` (default), `jaeger`, `otlp`, or
  `none`.
- `--tracing-endpoint` — collector URL; **required** for the `jaeger` and `otlp`
  exporters (for example `http://localhost:4318` for an OTLP/HTTP collector).
- `--tracing-sample-rate` — fraction of traces to record, `0.0`–`1.0` (default
  `1.0`, meaning trace every run). Lower it for high-volume automation.

```bash
# Send spans straight to a local Jaeger instance
cargoship upload ./my-data s3://my-bucket/archives/ \
  --tracing --tracing-exporter jaeger \
  --tracing-endpoint http://localhost:14268/api/traces
```

## Prometheus metrics

`--prometheus-addr` starts an HTTP endpoint that exposes CargoShip's metrics in
Prometheus format for the duration of the run. Point a Prometheus scrape or a
Grafana dashboard at it:

```bash
cargoship upload ./my-data s3://my-bucket/archives/ --prometheus-addr :9090
# metrics available at http://localhost:9090/metrics
```

## CloudWatch metrics

Separately from per-run tracing, the top-level `cargoship metrics` command tests
CargoShip's CloudWatch integration so you can confirm metrics reach AWS monitoring:

```bash
# Publish test metrics to CloudWatch
cargoship metrics --test

# Use a custom namespace and region
cargoship metrics --test --namespace "CargoShip/Prod" --region us-east-1
```

- `--test` — publish test metrics to CloudWatch.
- `--namespace` — CloudWatch namespace (default `CargoShip/Test`).
- `--region` — AWS region for CloudWatch (default `us-west-2`).

## See also

- [Performance tuning](/guides/features/optimization).
- [Benchmarking](/guides/features/benchmarking).
- Reference: [Uploading & sync commands](/reference/commands/upload).
- Reference: [Diagnostics & utilities](/reference/commands/diagnostics).
