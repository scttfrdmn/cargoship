## cargoship restore jobs

Manage queued Glacier restore jobs

### Synopsis

List, check, download, and clean restore jobs created when Glacier/Deep Archive
objects need time to be retrieved before they can be downloaded.

When 'cargoship restore' requests a Glacier restore without --wait, it saves a
job to ~/.cargoship/restore-jobs/ and prints a job ID. Use these subcommands to
track the job and trigger the download once the objects are ready.

### Options

```
  -h, --help   help for jobs
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

