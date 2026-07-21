# Observability & tracing

CargoShip can emit OpenTelemetry traces and Prometheus metrics during an upload so
you can see where time goes across the scan → chunk → archive → upload pipeline and
watch throughput in a dashboard. This is off by default; turn it on per run with
flags on `cargoship upload`.

```bash
cargoship upload ./my-data s3://my-bucket/archives/ \
  --tracing --tracing-exporter otlp \
  --tracing-endpoint http://localhost:4318 \
  --prometheus-addr :9090
```

- `--tracing` — enable distributed tracing.
- `--tracing-exporter` — `stdout` (default), `jaeger`, `otlp`, or `none`.
- `--tracing-endpoint` — collector URL (required for `jaeger` / `otlp`).
- `--tracing-sample-rate` — `0.0`–`1.0` (default `1.0` = trace everything).
- `--prometheus-addr` — expose metrics on this HTTP address (e.g. `:9090`).

::: warning Draft
This page is being expanded. For the complete flag list, see the
[Uploading & sync command reference](/reference/commands/upload).
:::

## See also

- [Performance tuning](/guides/features/optimization).
- [Benchmarking](/guides/features/benchmarking).
- Reference: [Uploading & sync commands](/reference/commands/upload).
