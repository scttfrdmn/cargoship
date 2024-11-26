# Overview

## Global Defaults

Users can use a `~/.suitcasectl.yaml` file for storing default options. Options
in this file will be applied to all commands. The format of the options names
should match their command line counterparts, minus the `--`, so if you want to
set `--suitcase-format ".tar.gz"` on every run, put this in your
`~/.suitcasectl.yaml` file: `suitcase-format: tar.gz`.

## Project Defaults (Overrides)

Users can include a `suitcasectl.yaml` file in the root of their
directory which will automatically be used by suitcasectl for certain options.
This could be useful for things like ignore patterns or other settings.

Example:

```yaml
ignore-glob:
  - "*.out"
  - "*.swp"
```

Currently project defaults can only be used on the following options:

* follow-symlinks
* ignore-glob
* inventory-format
* internal-metadata-glob
* max-suitcase-size
* prefix
* user
* suitcase-format

Under the covers, both of these approcaches use the
[viper](https://github.com/spf13/viper) library. Their documentation will be
useful when tracking down issues.
