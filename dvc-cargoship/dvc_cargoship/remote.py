"""CargoShipFileSystem: fsspec-compatible DVC remote for CargoShip.

DVC discovers this class via the ``dvc.fs`` entry point (see ``setup.py``).
Once installed, a remote can be added with::

    dvc remote add myremote cargoship://my-bucket/my-prefix

**Architecture note** — CargoShip is a *bulk* uploader: it archives an entire
directory into compressed, sharded tar.zst chunks.  This adapter translates
DVC's individual-file cache operations (put_file / get_file) into CargoShip
CLI calls by using a temporary staging directory for each operation.

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

from .cli import CargoShipCLI, CargoShipCLIError

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
    """

    protocol = _PROTOCOL
    root_marker = ""

    def __init__(
        self,
        bucket: str = "",
        prefix: str = "",
        cargoship_bin: str = "cargoship",
        **kwargs: Any,
    ) -> None:
        super().__init__(**kwargs)
        self._bucket = bucket.strip("/")
        self._prefix = prefix.strip("/")
        self._cli: Optional[CargoShipCLI] = None
        self._cargoship_bin = cargoship_bin

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

        CargoShip is a bulk uploader; this method stages the file in a
        temporary directory and calls ``cargoship upload`` on that directory.
        The resulting upload is associated with this remote's bucket and prefix.
        """
        with tempfile.TemporaryDirectory(prefix="dvc-cargoship-put-") as staging:
            dest_name = os.path.basename(lpath)
            shutil.copy2(lpath, os.path.join(staging, dest_name))
            self.cli.upload(staging, self._destination_url(), quiet=True)

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
