# Inventory

Inventory is a yaml or json file that contains a list of files that will be
added to the suitcase, along with some additional metadata around them.

This will be a human readable file that can be used for future file retrieval.
It is also used to generate the actual suitcase archive files.

Both `yaml` and `json` formats are supported, but we have found `yaml` to be
the best balance of readability and performance, so it is the default.

Example Inventory, created with `suitcasectl create suitcase --only-inventory ~/Desktop/example-suitcase --destination=/tmp/foo --max-suitcase-size=5Mb`

```yaml
files:
    - path: /Users/drews/Desktop/example-suitcase/bad.tar
      destination: bad.tar
      name: bad.tar
      size: 3154432
      suitcase_index: 1
      suitcase_name: demo-drews-01-of-03.tar.zst
    - path: /Users/drews/Desktop/example-suitcase/good.tar
      destination: good.tar
      name: good.tar
      size: 3154432
      suitcase_index: 2
      suitcase_name: demo-drews-02-of-03.tar.zst
    - path: /Users/drews/Desktop/example-suitcase/thing
      destination: thing
      name: thing
      size: 3145728
      suitcase_index: 3
      suitcase_name: demo-drews-03-of-03.tar.zst
    - path: /Users/drews/Desktop/example-suitcase/20210922_105206.jpeg
      destination: 20210922_105206.jpeg
      name: 20210922_105206.jpeg
      size: 238296
      suitcase_index: 1
      suitcase_name: demo-drews-01-of-03.tar.zst
    - path: /Users/drews/Desktop/example-suitcase/20220221_100626.jpeg
      destination: 20220221_100626.jpeg
      name: 20220221_100626.jpeg
      size: 225122
      suitcase_index: 2
      suitcase_name: demo-drews-02-of-03.tar.zst
    - path: /Users/drews/Desktop/example-suitcase/OAShapeTypes.zip
      destination: OAShapeTypes.zip
      name: OAShapeTypes.zip
      size: 132988
      suitcase_index: 1
      suitcase_name: demo-drews-01-of-03.tar.zst
    - path: /Users/drews/Desktop/example-suitcase/fake-zip
      destination: fake-zip
      name: fake-zip
      size: 132988
      suitcase_index: 1
      suitcase_name: demo-drews-01-of-03.tar.zst
    - path: /Users/drews/Desktop/example-suitcase/20220221_100626.jpeg alias 4
      destination: 20220221_100626.jpeg alias 4
      name: 20220221_100626.jpeg alias 4
      size: 868
      suitcase_index: 1
      suitcase_name: demo-drews-01-of-03.tar.zst
    - path: /Users/drews/Desktop/example-suitcase/20220221_100626.jpeg alias
      destination: 20220221_100626.jpeg alias
      name: 20220221_100626.jpeg alias
      size: 868
      suitcase_index: 1
      suitcase_name: demo-drews-01-of-03.tar.zst
    - path: /Users/drews/Desktop/example-suitcase/20220221_100626.jpeg alias 2
      destination: 20220221_100626.jpeg alias 2
      name: 20220221_100626.jpeg alias 2
      size: 868
      suitcase_index: 1
      suitcase_name: demo-drews-01-of-03.tar.zst
    - path: /Users/drews/Desktop/example-suitcase/20220221_100626.jpeg alias 3
      destination: 20220221_100626.jpeg alias 3
      name: 20220221_100626.jpeg alias 3
      size: 868
      suitcase_index: 1
      suitcase_name: demo-drews-01-of-03.tar.zst
    - path: /Users/drews/Desktop/example-suitcase/suitcasectl.yaml
      destination: suitcasectl.yaml
      name: suitcasectl.yaml
      size: 67
      suitcase_index: 2
      suitcase_name: demo-drews-02-of-03.tar.zst
    - path: /Users/drews/Desktop/example-suitcase/data.txt
      destination: data.txt
      name: data.txt
      size: 48
      suitcase_index: 1
      suitcase_name: demo-drews-01-of-03.tar.zst
    - path: /Users/drews/Desktop/example-suitcase/suitcase-meta.txt
      destination: suitcase-meta.txt
      name: suitcase-meta.txt
      size: 31
      suitcase_index: 1
      suitcase_name: demo-drews-01-of-03.tar.zst
    - path: /Users/drews/Desktop/example-suitcase/.hidden-file.txt
      destination: .hidden-file.txt
      name: .hidden-file.txt
      size: 15
      suitcase_index: 1
      suitcase_name: demo-drews-01-of-03.tar.zst
options:
    user: drews
    prefix: demo
    top_level_directories:
        - /Users/drews/Desktop/example-suitcase/
    size_considered_large: 0
    max_suitcase_size: 5000000
    internal_metadata_glob: suitcase-meta*
    ignore_globs:
        - '*.out'
        - '*.swp'
    encrypt_inner: false
    hash_inner: false
    limit_file_count: 0
    suitcase_format: tar.zst
    inventory_format: yaml
    follow_symlinks: false
    hash_algorithm: 1
    include_archive_toc: false
    include_archive_toc_deep: false
    transport_plugin: null
total_indexes: 3
index_summaries:
    1:
        count: 11
        size: 3662270
        human_size: 3.7 MB
    2:
        count: 3
        size: 3379621
        human_size: 3.4 MB
    3:
        count: 1
        size: 3145728
        human_size: 3.1 MB
internal_metadata:
    /Users/drews/Desktop/example-suitcase/suitcase-meta.txt: |
        This is a test bit of metadata
external_metadata: {}
cli_meta:
    date: 2023-09-19T09:40:00.683167-04:00
    version: v0.16.0
```
