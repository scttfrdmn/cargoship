# Exit codes

Every `cargoship` command exits with one of three codes. They are the contract
for scripts and CI, and are covered by tests so they don't drift.

| Code | Meaning | When |
|------|---------|------|
| `0` | Success | The command completed. |
| `1` | Runtime error | The command was invoked correctly but failed — network error, missing S3 object, verification mismatch, permission denied. |
| `2` | Usage error | The invocation itself was wrong — unknown command or flag, missing required flag, bad flag value, wrong number of arguments. |

The distinction between `1` and `2` is the useful part: `2` means fix your
command line, `1` means the command line was fine and something else went wrong.
Retrying is only ever sensible for `1`.

## In scripts

```bash
cargoship verify s3://my-bucket/archives/uploads/20260721-a1b2c3 --deep
case $? in
  0) echo "verified" ;;
  1) echo "verification failed or could not run — investigate, maybe retry" ; exit 1 ;;
  2) echo "bad invocation — this is a bug in the script" ; exit 2 ;;
esac
```

Because `2` signals a malformed command rather than a transient failure, a retry
loop should treat it as fatal:

```bash
for attempt in 1 2 3; do
  cargoship upload ./data s3://my-bucket/archives/ && break
  status=$?
  [ "$status" -eq 2 ] && { echo "not retryable"; exit 2; }
  sleep $((attempt * 5))
done
```

## Notes

- **`verify` failing a check exits `1`**, not `2` — the command ran and did its
  job; the answer was "this archive does not match". See
  [Verifying uploads](/guides/verifying).
- Commands that print their own diagnostics (again, `verify`) report failure
  through the exit code without an extra error line. The code is still `1`.
- Interrupting with `Ctrl-C` follows the shell's usual signal convention
  (`130` for `SIGINT`) rather than these codes.

::: warning Changed in v0.23.0
Earlier versions exited **`3`** for every failure — runtime errors and malformed
invocations alike. `3` was never documented, and a caller could not tell the two
apart. If a script checks for `3`, update it to check `1` and `2` (see
[#401](https://github.com/scttfrdmn/cargoship/issues/401)).
:::

## See also

- [Verifying uploads](/guides/verifying) — the exit code as a CI gate.
- [Troubleshooting](/reference/troubleshooting) — diagnosing a `1`.
- [Global flags](/reference/commands/global-flags).
