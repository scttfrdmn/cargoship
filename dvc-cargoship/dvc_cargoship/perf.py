"""Performance helpers for the CargoShip DVC remote.

- :func:`parse_size` — convert human-readable size strings to bytes.
- :class:`BatchUploadBuffer` — aggregate small files via symlinks, then flush
  as a single ``cargoship upload`` operation.
- :func:`parallel_restore` — restore multiple files from an S3 remote
  concurrently using a thread-pool.
"""

from __future__ import annotations

import os
import shutil
import tempfile
from concurrent.futures import ThreadPoolExecutor, as_completed
from typing import TYPE_CHECKING, Callable, Dict, List, Optional, Tuple

if TYPE_CHECKING:
    from .cli import CargoShipCLI

# ---------------------------------------------------------------------------
# Size parsing
# ---------------------------------------------------------------------------

_SIZE_UNITS: Dict[str, int] = {
    "TB": 1024**4,
    "GB": 1024**3,
    "MB": 1024**2,
    "KB": 1024,
    "B": 1,
}

_DEFAULT_SMALL_FILE_THRESHOLD: int = 10 * 1024 * 1024  # 10 MB
_DEFAULT_AUTO_FLUSH_BYTES: int = 50 * 1024 * 1024  # 50 MB
_DEFAULT_DOWNLOAD_WORKERS: int = 4


def parse_size(s: str) -> int:
    """Parse a human-readable size string into bytes.

    Accepts formats such as ``"10MB"``, ``"1.5 GB"``, ``"512kb"``, or a plain
    integer string ``"1048576"``.  The unit suffix is case-insensitive.

    Parameters
    ----------
    s:
        Size string to parse.

    Returns
    -------
    int
        Size in bytes (always a whole number; fractional bytes are truncated).

    Raises
    ------
    ValueError
        When the string cannot be parsed.
    """
    s = s.strip()
    upper = s.upper()
    for unit, factor in _SIZE_UNITS.items():
        if upper.endswith(unit):
            number_part = s[: -len(unit)].strip()
            try:
                return int(float(number_part) * factor)
            except ValueError:
                raise ValueError(f"Cannot parse size: {s!r}")
    # bare integer (no unit)
    try:
        return int(s)
    except ValueError:
        raise ValueError(f"Cannot parse size: {s!r}")


# ---------------------------------------------------------------------------
# Batch upload buffer
# ---------------------------------------------------------------------------


class BatchUploadBuffer:
    """Accumulate small files via symlinks and upload them as one batch.

    Instead of invoking ``cargoship upload`` once per file (expensive for small
    files), files are symlinked into a shared staging directory.  When the
    buffer is flushed — either explicitly via :meth:`flush` / :meth:`close` or
    automatically when *auto_flush_bytes* is exceeded — a single
    ``cargoship upload`` call transfers all buffered files.

    Parameters
    ----------
    cli:
        :class:`~dvc_cargoship.cli.CargoShipCLI` instance used to invoke the
        upload.
    destination:
        S3 destination URL (``s3://bucket/prefix``).
    threshold:
        Files whose size is *below* this value (bytes) are eligible for
        batching.  Files at or above this size should be uploaded directly.
    auto_flush_bytes:
        Flush the buffer automatically when the total staged size reaches this
        value.  Defaults to 50 MB.

    Usage::

        buf = BatchUploadBuffer(cli, "s3://my-bucket/prefix")
        for local_path, rel_key in small_files:
            buf.add(local_path, rel_key)
        buf.close()   # uploads all staged files in one call
    """

    def __init__(
        self,
        cli: CargoShipCLI,
        destination: str,
        *,
        threshold: int = _DEFAULT_SMALL_FILE_THRESHOLD,
        auto_flush_bytes: int = _DEFAULT_AUTO_FLUSH_BYTES,
    ) -> None:
        self._cli = cli
        self._destination = destination
        self.threshold = threshold
        self._auto_flush_bytes = auto_flush_bytes
        self._staging: Optional[tempfile.TemporaryDirectory] = None
        self._staging_path: Optional[str] = None
        self._buffered_bytes: int = 0
        self._file_count: int = 0

    # ------------------------------------------------------------------
    # Public API
    # ------------------------------------------------------------------

    def should_buffer(self, file_size: int) -> bool:
        """Return *True* when *file_size* qualifies for batch buffering."""
        return file_size < self.threshold

    def add(self, local_path: str, relative_key: str) -> None:
        """Stage *local_path* under *relative_key* for the next batch upload.

        Creates a symlink (or copy on filesystems that do not support symlinks)
        in the shared staging directory, preserving the relative directory
        structure of *relative_key*.

        Auto-flushes the buffer if the total staged bytes reach
        *auto_flush_bytes*.

        Parameters
        ----------
        local_path:
            Absolute path to the local file.
        relative_key:
            Key path relative to this remote's prefix (e.g.
            ``"data/train/a.csv"``).
        """
        staging = self._ensure_staging()
        rel = relative_key.lstrip("/")
        dest = os.path.join(staging, rel)
        dest_dir = os.path.dirname(dest)
        if dest_dir:
            os.makedirs(dest_dir, exist_ok=True)

        abs_src = os.path.abspath(local_path)
        try:
            os.symlink(abs_src, dest)
        except (OSError, NotImplementedError):
            shutil.copy2(local_path, dest)

        self._buffered_bytes += os.path.getsize(local_path)
        self._file_count += 1

        if self._buffered_bytes >= self._auto_flush_bytes:
            self.flush()

    @property
    def file_count(self) -> int:
        """Number of files currently buffered (not yet flushed)."""
        return self._file_count

    @property
    def buffered_bytes(self) -> int:
        """Total size (bytes) of files currently buffered."""
        return self._buffered_bytes

    def flush(self) -> None:
        """Upload all buffered files as a single ``cargoship upload`` call.

        Does nothing when the buffer is empty.
        """
        if self._staging is None or self._file_count == 0:
            return
        staging_path = self._staging_path
        self._cli.upload(staging_path, self._destination, quiet=True)
        self._reset()

    def close(self) -> None:
        """Flush remaining files and release the staging directory."""
        self.flush()
        if self._staging is not None:
            try:
                self._staging.cleanup()
            except OSError:
                pass
            self._staging = None
            self._staging_path = None

    def __enter__(self) -> "BatchUploadBuffer":
        return self

    def __exit__(self, *_: object) -> None:
        self.close()

    def __del__(self) -> None:
        try:
            self.close()
        except Exception:
            pass

    # ------------------------------------------------------------------
    # Internal helpers
    # ------------------------------------------------------------------

    def _ensure_staging(self) -> str:
        if self._staging is None:
            self._staging = tempfile.TemporaryDirectory(
                prefix="dvc-cargoship-batch-"
            )
            self._staging_path = self._staging.name
        return self._staging_path  # type: ignore[return-value]

    def _reset(self) -> None:
        """Cleanup and reset the staging area without flushing."""
        if self._staging is not None:
            try:
                self._staging.cleanup()
            except OSError:
                pass
        self._staging = None
        self._staging_path = None
        self._buffered_bytes = 0
        self._file_count = 0


# ---------------------------------------------------------------------------
# Parallel restore
# ---------------------------------------------------------------------------


def parallel_restore(
    cli: CargoShipCLI,
    destination: str,
    files: List[Tuple[str, str]],
    *,
    workers: int = _DEFAULT_DOWNLOAD_WORKERS,
    progress_callback: Optional[Callable[[int], None]] = None,
) -> Dict[str, Optional[Exception]]:
    """Restore multiple files from *destination* concurrently.

    Each ``(rel_key, local_path)`` pair is restored in a separate thread.
    Files that fail are recorded in the returned dict rather than raising.

    Parameters
    ----------
    cli:
        :class:`~dvc_cargoship.cli.CargoShipCLI` instance.
    destination:
        S3 source URL (``s3://bucket/prefix``).
    files:
        List of ``(relative_key, local_path)`` pairs to restore.
    workers:
        Maximum number of concurrent restore threads.
    progress_callback:
        Optional callable invoked with the running total of completed
        files (both successes and failures) after each file finishes.

    Returns
    -------
    dict
        Maps each *relative_key* to ``None`` (success) or the
        :class:`Exception` that was raised during restoration.
    """

    def _restore_one(
        rel_key: str,
        local_path: str,
    ) -> Tuple[str, Optional[Exception]]:
        with tempfile.TemporaryDirectory(prefix="dvc-cargoship-par-") as staging:
            try:
                cli.restore(destination, staging, file_path=rel_key)
                restored = os.path.join(staging, os.path.basename(rel_key))
                if not os.path.exists(restored):
                    raise FileNotFoundError(
                        f"cargoship restore did not produce expected file: {restored!r}"
                    )
                os.makedirs(
                    os.path.dirname(os.path.abspath(local_path)), exist_ok=True
                )
                shutil.move(restored, local_path)
                return rel_key, None
            except Exception as exc:  # noqa: BLE001
                return rel_key, exc

    results: Dict[str, Optional[Exception]] = {}
    completed = 0

    with ThreadPoolExecutor(max_workers=workers) as pool:
        future_map = {
            pool.submit(_restore_one, rel_key, local_path): rel_key
            for rel_key, local_path in files
        }
        for future in as_completed(future_map):
            rel_key, exc = future.result()
            results[rel_key] = exc
            completed += 1
            if progress_callback is not None:
                progress_callback(completed)

    return results
