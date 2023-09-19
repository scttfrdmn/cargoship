# Inventory Metadata

Arbitrary metadata can be included in the inventory file as well. This allows
for data owners to add some special text describing the data. By default,
anything in one of the top level directories that matches the glob
`suitcase-meta*` will be included in the metadata. This is configurable with the
`--internal-metadata-glob 'new-glob*'` flag.

You can also include metadata outside of the target directories with
`--external-metadata-file /tmp/some-file.txt`. This argument can be used
multiple times.

This metadata is stored inside the inventory file with the key
`internal_metadata` or `external_metadata`:

```yaml
...
internal_metadata:
    /Users/drews/Desktop/example-suitcase/suitcase-meta.txt: |
        This is a test bit of metadata
...
```
