# Overrides

Users can include a `suitcasectl.[yaml|toml|json]` file in the root of their
directory which will automatically be used by suitcasectl for certain options.
This could be useful for things like ignore patterns or other settings.

Example:

```yaml
ignore-glob:
  - "*.out"
  - "*.swp"
```

Under the covers, the [viper](https://github.com/spf13/viper) library is doing
this. Their documentation will be useful when tracking down issues.
