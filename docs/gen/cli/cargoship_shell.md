## cargoship shell

Navigate a CargoShip archive or start an interactive shell

### Synopsis

When called with an S3 URL, opens an interactive filesystem shell for
browsing and inspecting a CargoShip archive without downloading files.

  cargoship shell s3://my-bucket/uploads/20240101-abc123

When called without arguments, starts the generic CargoShip REPL.

Archive shell commands:
  ls [path]         List files and directories
  cd &lt;dir&gt;          Change current directory
  pwd               Print current directory
  cat &lt;file&gt;        Stream file content to stdout
  head &lt;file&gt; [n]   Print first n lines (default 10)
  stat &lt;file&gt;       Show file metadata (size, hash, chunk, DVC stage, git commit)
  find &lt;pattern&gt;    Find files by glob pattern (e.g. *.csv, data/*.parquet)
  stage list        List all DVC pipeline stages and their file counts
  stage &lt;name&gt;      List files belonging to a DVC stage
  get &lt;file&gt; [dst]  Extract file to a local path (default: current directory)
  help              Show this help
  exit / quit       Exit the shell

```
cargoship shell [S3_URL] [flags]
```

### Options

```
      --cache-gb int    LRU chunk cache size in GB (default 10)
  -h, --help            help for shell
  -r, --region string   AWS region (default "us-east-1")
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

