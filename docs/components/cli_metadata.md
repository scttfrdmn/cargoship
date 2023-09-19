# CLI Metadata

This is a yaml file that contains information about what was passed to the CLI.
It will be created as `cli-invocation-meta.yaml` in the output directory.

Example:

```yaml
username: drews
hostname: idrew2019.lan
started_at: 2023-09-19T09:40:00.682449-04:00
completed_at: 2023-09-19T09:40:00.683922-04:00
arguments:
    - /Users/drews/Desktop/example-suitcase
active_flags:
    destination: /tmp/foo
    max-suitcase-size: 5Mb
    only-inventory: true
default_flags:
    archive-toc: false
    archive-toc-deep: false
    buffer-size: 1024
    cloud-destination: ""
    concurrency: 10
    encrypt-inner: false
    exclude-systems-pubkeys: false
    external-metadata-file: {}
    follow-symlinks: false
    hash-algorithm: 1
    hash-inner: false
    hash-outer: true
    help: false
    ignore-glob: {}
    internal-metadata-glob: suitcase-meta*
    inventory-file: ""
    inventory-format: 0
    limit-file-count: 0
    memory-limit: ""
    prefix: suitcase
    profile: false
    public-key: {}
    shell-destination: ""
    suitcase-format: 0
    trace: false
    user: ""
    verbose: false
viper_config:
    follow-symlinks: false
    ignore-glob:
        - '*.out'
        - '*.swp'
    internal-metadata-glob: suitcase-meta*
    inventory-format: ""
    max-suitcase-size: 5Mb
    prefix: demo
    suitcase-format: ""
    user: drews
version: "dev"
```
