# Shell Transport Plugin

Use `--shell-destination=$YOUR_SHELL_SCRIPT`

The file to copy will be accessible with the `$SUITCASECTL_FILE` variable, and can be used in a script like this:

```bash
#!/usr/bin/env bash

if [[ -z "${SUITCASECTL_FILE}" ]]; then
    echo "must set SUITCASECTL_FILE" before running 1>&2
    exit 2
fi

if [[ ! -e "${SUITCASECTL_FILE}" ]]; then
    echo "SUITCASECTL_FILE must be a file" 2>&2
    exit 3
fi

rsync -va "${SUITCASECTL_FILE}" foo:/bar/
```
