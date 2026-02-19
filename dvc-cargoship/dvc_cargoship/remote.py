"""CargoShipFileSystem: fsspec-compatible DVC remote for CargoShip.

DVC discovers this class via the ``dvc.fs`` entry point (see ``setup.py``).
Once installed, a remote can be added with::

    dvc remote add myremote cargoship://my-bucket/my-prefix

Performance tuning via ``dvc remote modify``::

    dvc remote modify myremote small_file_threshold 10MB
    dvc remote modify myremote download_workers 8

**Architecture note** — CargoShip is a *bulk* uploader: it archives an entire
directory into compressed, sharded tar.zst chunks.  This adapter translates
DVC's individual-file cache operations (put_file / get_file) into CargoShip
CLI calls by using a temporary staging directory for each operation.

Small files (below *small_file_threshold*) are buffered in a
:class:`~dvc_cargoship.perf.BatchUploadBuffer` and sent as a single
``cargoship upload`` call, eliminating per-file invocation overhead.

Downloads are accelerated with :func:`~dvc_cargoship.perf.parallel_restore`
when :meth:`get_files` is called with multiple targets.

**DVC config-schema limitation** — DVC currently validates remote URLs
against a hard-coded schema (iterative/dvc#9711).  Until that issue is
resolved, using ``cargoship://`` URLs may require a patched DVC build.
"""

from __future__ import annotations

import os
import shutil
import tempfile
from typing import Any, Dict, List, Optional, Tuple

from fsspec.spec import AbstractFileSystem

from .budget import DVCBudgetChecker, DVCBudgetExceededError
from .cli import CargoShipCLI, CargoShipCLIError
from .perf import (
    BatchUploadBuffer,
    _DEFAULT_DOWNLOAD_WORKERS,
    _DEFAULT_SMALL_FILE_THRESHOLD,
    parallel_restore,
    parse_size,
)

_PROTOCOL = "cargoship"


class CargoShipFileSystem(AbstractFileSystem):
    """fsspec filesystem backed by a CargoShip S3 remote.

    Parameters
    ----------
    bucket:
        S3 bucket name (extracted from the ``cargoship://bucket/prefix`` URL).
    prefix:
        S3 key prefix (may be empty).
    cargoship_bin:
        Name or absolute path of the ``cargoship`` binary.  Defaults to
        ``"cargoship"`` which will be resolved via PATH at first use.
    small_file_threshold:
        Files smaller than this value (bytes or size string such as ``"10MB"``)
        are aggregated into a single CargoShip upload rather than being sent
        one at a time.  Defaults to 10 MB.  Set to ``"0"`` to disable batching.
    download_workers:
        Number of parallel threads used by :meth:`get_files`.  Defaults to 4.
    """

    protocol = _PROTOCOL
    root_marker = ""

    def __init__(
        self,
        bucket: str = "",
        prefix: str = "",
        cargoship_bin: str = "cargoship",
        small_file_threshold: Any = _DEFAULT_SMALL_FILE_THRESHOLD,
        download_workers: Any = _DEFAULT_DOWNLOAD_WORKERS,
        project_id: str = "",
        enable_budget_check: Any = False,
        **kwargs: Any,
    ) -> None:
        super().__init__(**kwargs)
        self._bucket = bucket.strip("/")
        self._prefix = prefix.strip("/")
        self._cli: Optional[CargoShipCLI] = None
        self._cargoship_bin = cargoship_bin

        # Parse size/int config values (DVC passes them as strings)
        self._small_file_threshold: int = (
            parse_size(str(small_file_threshold))
            if isinstance(small_file_threshold, str)
            else int(small_file_threshold)
        )
        self._download_workers: int = int(download_workers)

        # Issue #183: Budget integration
        self._project_id: str = project_id
        self._enable_budget_check: bool = str(enable_budget_check).lower() not in (
            "false", "0", ""
        ) if isinstance(enable_budget_check, str) else bool(enable_budget_check)
        self.__budget: Optional[DVCBudgetChecker] = None

        # Lazy batch buffer — created on first small-file put_file
        self._batch: Optional[BatchUploadBuffer] = None

    # ------------------------------------------------------------------
    # Lazy CLI accessor (enables unit-test injection)
    # ------------------------------------------------------------------

    @property
    def cli(self) -> CargoShipCLI:
        """Return the :class:`CargoShipCLI` instance, creating it on first access."""
        if self._cli is None:
            self._cli = CargoShipCLI(binary=self._cargoship_bin)
        return self._cli

    @cli.setter
    def cli(self, value: CargoShipCLI) -> None:
        """Inject a :class:`CargoShipCLI` (used in unit tests)."""
        self._cli = value
        # Reset the batch buffer and budget checker so they pick up the new CLI.
        if self._batch is not None:
            self._batch.close()
            self._batch = None
        self.__budget = None

    # ------------------------------------------------------------------
    # Budget helpers (Issue #183)
    # ------------------------------------------------------------------

    @property
    def _budget_checker(self) -> Optional[DVCBudgetChecker]:
        """Return the :class:`DVCBudgetChecker`, or *None* when disabled.

        Budget checking is skipped when *enable_budget_check* is False or
        when no *project_id* is configured.
        """
        if not self._enable_budget_check or not self._project_id:
            return None
        if self.__budget is None:
            self.__budget = DVCBudgetChecker(self.cli, self._project_id)
        return self.__budget

    def _dvc_tags(self, operation: str = "push") -> Dict[str, str]:
        """Return the standard DVC cost-record tags for *operation*."""
        tags: Dict[str, str] = {"dvc_cache": "true", "dvc_operation": operation}
        if self._project_id:
            tags["dvc_project"] = self._project_id
        return tags

    # ------------------------------------------------------------------
    # Batch buffer helpers
    # ------------------------------------------------------------------

    @property
    def _batch_buffer(self) -> BatchUploadBuffer:
        """Return (or lazily create) the per-instance batch upload buffer."""
        if self._batch is None:
            self._batch = BatchUploadBuffer(
                self.cli,
                self._destination_url(),
                threshold=self._small_file_threshold,
                project_id=self._project_id or None,
                tags=self._dvc_tags("push"),
            )
        return self._batch

    def flush_batch(self) -> None:
        """Flush any pending small-file uploads.

        Called automatically by :meth:`close` and the destructor.  May also be
        called explicitly after a sequence of :meth:`put_file` calls.
        """
        if self._batch is not None:
            self._batch.flush()

    def close(self) -> None:
        """Flush pending uploads and release resources."""
        if self._batch is not None:
            self._batch.close()
            self._batch = None

    def __del__(self) -> None:
        try:
            self.close()
        except Exception:
            pass

    # ------------------------------------------------------------------
    # URL / path helpers
    # ------------------------------------------------------------------

    @classmethod
    def _strip_protocol(cls, path: str) -> str:
        """Remove the ``cargoship://`` prefix and return the bare bucket/key path."""
        if isinstance(path, str) and path.startswith(f"{_PROTOCOL}://"):
            path = path[len(f"{_PROTOCOL}://"):]
        return path.lstrip("/")

    def _destination_url(self) -> str:
        """Return the canonical S3 destination URL for this remote."""
        if self._prefix:
            return f"s3://{self._bucket}/{self._prefix}"
        return f"s3://{self._bucket}"

    def _parse_path(self, path: str) -> Tuple[str, str]:
        """Split a remote path into ``(bucket, key)``."""
        stripped = self._strip_protocol(path)
        parts = stripped.split("/", 1)
        bucket = parts[0] if parts else self._bucket
        key = parts[1] if len(parts) > 1 else ""
        return bucket, key

    def _relative_key(self, path: str) -> str:
        """Return the key portion of *path* relative to this remote's prefix."""
        _, key = self._parse_path(path)
        if self._prefix and key.startswith(self._prefix):
            key = key[len(self._prefix):].lstrip("/")
        return key

    # ------------------------------------------------------------------
    # fsspec AbstractFileSystem interface
    # ------------------------------------------------------------------

    def ls(
        self,
        path: str,
        detail: bool = True,
        **kwargs: Any,
    ) -> List[Any]:
        """List files under *path* by querying the latest CargoShip manifest.

        Returns a list of dicts (when *detail=True*) or path strings.
        Returns an empty list when the remote has no uploads or is unreachable.
        """
        entries = self.cli.list_files(self._destination_url())
        filter_prefix = self._relative_key(path)

        results: List[Any] = []
        for entry in entries:
            entry_key = entry.get("path", "")
            if filter_prefix and not entry_key.startswith(filter_prefix):
                continue
            full_path = (
                f"{_PROTOCOL}://{self._bucket}"
                + (f"/{self._prefix}" if self._prefix else "")
                + f"/{entry_key}"
            )
            if detail:
                results.append(
                    {
                        "name": full_path,
                        "size": entry.get("size", 0),
                        "type": "file",
                        "md5": entry.get("content_hash", ""),
                    }
                )
            else:
                results.append(full_path)
        return results

    def info(self, path: str, **kwargs: Any) -> Dict[str, Any]:
        """Return metadata dict for *path*.

        Raises :class:`FileNotFoundError` when the path is not found.
        """
        entries = self.ls(path, detail=True)
        target = self._strip_protocol(path)
        for entry in entries:
            if self._strip_protocol(entry.get("name", "")) == target:
                return entry
        raise FileNotFoundError(f"No such file in CargoShip remote: {path!r}")

    def exists(self, path: str, **kwargs: Any) -> bool:
        """Return *True* when *path* is present in the latest CargoShip manifest."""
        try:
            self.info(path)
            return True
        except FileNotFoundError:
            return False

    def put_file(self, lpath: str, rpath: str, **kwargs: Any) -> None:
        """Upload the local file at *lpath* to the remote at *rpath*.

        **Budget check** — when *enable_budget_check* is True and a
        *project_id* is configured, the budget is verified before the upload
        proceeds.  :class:`~dvc_cargoship.budget.DVCBudgetExceededError` is
        raised if the project is over budget.

        **Small files** (below *small_file_threshold*) are symlinked into a
        shared staging directory and deferred until the buffer is flushed via
        :meth:`flush_batch` or :meth:`close`.  The batch upload is tagged
        ``dvc_cache=true, dvc_operation=push``.

        **Large files** are uploaded immediately via a per-file
        ``cargoship upload`` call, also tagged with DVC metadata.
        """
        file_size = os.path.getsize(lpath)

        # Budget pre-check (Issue #183)
        checker = self._budget_checker
        if checker is not None:
            checker.check_upload(size_bytes=file_size, operation="push")

        if self._batch_buffer.should_buffer(file_size):
            rel_key = self._relative_key(rpath) or os.path.basename(lpath)
            self._batch_buffer.add(lpath, rel_key)
        else:
            with tempfile.TemporaryDirectory(prefix="dvc-cargoship-put-") as staging:
                dest_name = os.path.basename(lpath)
                shutil.copy2(lpath, os.path.join(staging, dest_name))
                self.cli.upload(
                    staging,
                    self._destination_url(),
                    quiet=True,
                    project_id=self._project_id or None,
                    tags=self._dvc_tags("push"),
                )

    def get_file(self, rpath: str, lpath: str, **kwargs: Any) -> None:
        """Download the remote file at *rpath* to *lpath*.

        Calls ``cargoship restore`` with the file's relative key.  The binary
        must have the ``restore --file <path>`` subcommand available.
        """
        rel_key = self._relative_key(rpath)
        with tempfile.TemporaryDirectory(prefix="dvc-cargoship-get-") as staging:
            self.cli.restore(
                self._destination_url(),
                staging,
                file_path=rel_key,
            )
            restored = os.path.join(staging, os.path.basename(rel_key))
            if not os.path.exists(restored):
                raise FileNotFoundError(
                    f"cargoship restore did not produce expected file: {restored!r}"
                )
            os.makedirs(os.path.dirname(os.path.abspath(lpath)), exist_ok=True)
            shutil.move(restored, lpath)

    def get_files(
        self,
        rpaths: List[str],
        lpaths: List[str],
        callback: Any = None,
    ) -> None:
        """Download multiple remote files in parallel.

        Uses :func:`~dvc_cargoship.perf.parallel_restore` with
        *download_workers* threads.  Any failures are re-raised as a combined
        error after all files have been attempted.

        Parameters
        ----------
        rpaths:
            Remote paths (``cargoship://`` URLs or bare keys).
        lpaths:
            Corresponding local destination paths.
        callback:
            Optional progress callback ``(completed: int) -> None``.
        """
        pairs = [
            (self._relative_key(rp), lp) for rp, lp in zip(rpaths, lpaths)
        ]
        results = parallel_restore(
            self.cli,
            self._destination_url(),
            pairs,
            workers=self._download_workers,
            progress_callback=callback,
        )
        errors = {k: v for k, v in results.items() if v is not None}
        if errors:
            msgs = "\n".join(f"  {k}: {v}" for k, v in errors.items())
            raise RuntimeError(
                f"{len(errors)} file(s) failed to restore:\n{msgs}"
            )

    def rm(self, path: str, recursive: bool = False, **kwargs: Any) -> None:
        """Not supported — CargoShip archives are immutable.

        Raises :class:`NotImplementedError`.  To effectively "remove" a file,
        create a new incremental upload that omits the file.
        """
        raise NotImplementedError(
            "CargoShip archives are immutable; individual file removal is not "
            "supported.  Create a new upload that omits the file you want to remove."
        )

    def copy(self, path1: str, path2: str, **kwargs: Any) -> None:
        """Not supported — copying within a CargoShip remote is not implemented."""
        raise NotImplementedError(
            "copy() is not supported for CargoShip remotes."
        )

    # ------------------------------------------------------------------
    # DVC 2.x compatibility helpers
    # ------------------------------------------------------------------

    def list_cache_paths(
        self,
        prefix: Optional[str] = None,
        progress_callback: Any = None,
    ) -> List[str]:
        """Return all cached file paths in this remote.

        Used by ``dvc gc`` and ``dvc status -c`` to reconcile the remote cache
        against the local DVC cache.

        Parameters
        ----------
        prefix:
            Optional path prefix filter.
        progress_callback:
            Optional callable ``(count: int) -> None`` invoked periodically
            with the number of paths yielded so far.
        """
        entries = self.cli.list_files(self._destination_url())
        paths: List[str] = []
        for entry in entries:
            p = entry.get("path", "")
            if not p:
                continue
            if prefix and not p.startswith(prefix):
                continue
            paths.append(p)
            if progress_callback is not None:
                progress_callback(len(paths))
        return paths

    def upload(
        self,
        from_file: str,
        to_info: Any,
        name: Optional[str] = None,
        no_progress_bar: bool = False,
    ) -> None:
        """DVC 2.x upload interface — delegates to :meth:`put_file`."""
        self.put_file(from_file, str(to_info))

    def download(
        self,
        from_info: Any,
        to_file: str,
        name: Optional[str] = None,
        no_progress_bar: bool = False,
    ) -> None:
        """DVC 2.x download interface — delegates to :meth:`get_file`."""
        self.get_file(str(from_info), to_file)

    def remove(self, path_info: Any) -> None:
        """DVC 2.x remove interface — delegates to :meth:`rm`."""
        self.rm(str(path_info))
