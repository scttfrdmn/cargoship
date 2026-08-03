## cargoship dvc export

Download a manifest and generate .dvc sidecar files

```
cargoship dvc export S3_URL [OUTPUT_DIR] [flags]
```

### Options

```
      --cache-dir string    Local DVC cache directory (recorded in manifest) (default ".dvc/cache")
  -h, --help                help for export
      --output-dir string   Directory to write .dvc files (default: dvc-files)
      --region string       AWS region (default "us-east-1")
```

### Options inherited from parent commands

```
      --context string        Override execution context (local, agent, repl)
      --memory-limit string   Set a memory limit for the run. This will slow things down, but will less likely to OOM in certain situations. Avoid this unless you are having memory issues.
      --pprof                 Enable runtime profiling HTTP endpoint at localhost:6060
      --pprof-addr string     Address for runtime profiling HTTP endpoint (default "localhost:6060")
      --profile               Enable performance profiling. This will generate profile files in a temp directory
  -t, --trace                 Enable trace messages in output
  -v, --verbose               Enable verbose output
```

