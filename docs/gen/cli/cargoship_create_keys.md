## cargoship create keys

Create a new private and public key pair

### Synopsis

Create a new GPG private and public key pair.

The private key is encrypted with a passphrase. Supply it interactively when
prompted, or set CARGOSHIP_GPG_PASSPHRASE for non-interactive use. To generate an
unencrypted private key, pass --no-passphrase explicitly.

Keys are written to the current directory unless --destination is given. An
existing private.key or public.key is never overwritten.

```
cargoship create keys [flags]
```

### Options

```
  -b, --bits int        Bit length of the key (default 4096)
  -e, --email string    Email of the key
  -h, --help            help for keys
  -n, --name string     Name of the key
      --no-passphrase   Generate an UNENCRYPTED private key (not recommended; the file is plaintext key material)
      --type KeyType    key type (rsa, x25519) (default rsa)
```

### Options inherited from parent commands

```
      --context string        Override execution context (local, agent, repl)
  -d, --destination string    Directory to write files in to. Defaults to the current directory
      --memory-limit string   Set a memory limit for the run. This will slow things down, but will less likely to OOM in certain situations. Avoid this unless you are having memory issues.
      --pprof                 Enable runtime profiling HTTP endpoint at localhost:6060
      --pprof-addr string     Address for runtime profiling HTTP endpoint (default "localhost:6060")
      --profile               Enable performance profiling. This will generate profile files in a temp directory
  -t, --trace                 Enable trace messages in output
  -v, --verbose               Enable verbose output
```

