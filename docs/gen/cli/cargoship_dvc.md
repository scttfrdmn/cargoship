## cargoship dvc

Inspect and export DVC stage metadata from a CargoShip archive

### Synopsis

Commands for working with DVC (Data Version Control) stage metadata
embedded in CargoShip manifests.

Use 'cargoship upload --dvc-auto' to annotate uploads with per-file stage
information from dvc.yaml before using these commands.

### Options

```
  -h, --help   help for dvc
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

