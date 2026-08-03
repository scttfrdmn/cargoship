## cargoship create keys

Create a new private and public key pair

```
cargoship create keys [flags]
```

### Options

```
  -b, --bits int       Bit length of the key (default 4096)
  -e, --email string   Email of the key
  -h, --help           help for keys
  -n, --name string    Name of the key
      --type KeyType   key type (rsa, x25519) (default rsa)
```

### Options inherited from parent commands

```
      --context string        Override execution context (local, agent, repl)
  -d, --destination string    Directory to write files in to. If not specified, we'll use an auto generated temp dir
      --memory-limit string   Set a memory limit for the run. This will slow things down, but will less likely to OOM in certain situations. Avoid this unless you are having memory issues.
      --pprof                 Enable runtime profiling HTTP endpoint at localhost:6060
      --pprof-addr string     Address for runtime profiling HTTP endpoint (default "localhost:6060")
      --profile               Enable performance profiling. This will generate profile files in a temp directory
  -t, --trace                 Enable trace messages in output
  -v, --verbose               Enable verbose output
```

