"""CargoShipCLI: subprocess wrapper around the ``cargoship`` binary.

All public methods translate to ``cargoship <subcommand>`` invocations and
raise :class:`CargoShipCLIError` on non-zero exit codes.
"""

from __future__ import annotations

import json
import os
import shutil
import subprocess
from typing import Any, Dict, List, Optional


class CargoShipCLIError(Exception):
    """Raised when a ``cargoship`` subprocess command exits with a non-zero code."""


class CargoShipCLI:
    """Locate and invoke the ``cargoship`` binary.

    Usage::

        cli = CargoShipCLI.find()               # discovers binary on PATH
        cli.upload("/data", "s3://my-bucket/pfx")
        entries = cli.list_files("s3://my-bucket/pfx")

    All methods raise :class:`CargoShipCLIError` on non-zero exit codes.
    Pass ``check=False`` to :meth:`run` to suppress that behaviour and
    inspect the return code yourself.
    """

    def __init__(self, binary: str = "cargoship") -> None:
        self.binary = binary

    # ------------------------------------------------------------------
    # Discovery
    # ------------------------------------------------------------------

    @classmethod
    def find(cls, binary: str = "cargoship") -> "CargoShipCLI":
        """Return a :class:`CargoShipCLI` pointing at the binary on PATH.

        Raises :class:`FileNotFoundError` when the binary cannot be found.
        """
        resolved = shutil.which(binary)
        if resolved is None:
            raise FileNotFoundError(
                f"cargoship binary not found on PATH. "
                f"Install it from https://github.com/scttfrdmn/cargoship/releases"
            )
        return cls(binary=resolved)

    # ------------------------------------------------------------------
    # Raw execution
    # ------------------------------------------------------------------

    def run(
        self,
        *args: str,
        check: bool = True,
        env: Optional[Dict[str, str]] = None,
    ) -> subprocess.CompletedProcess:
        """Execute ``cargoship <args>`` and return the :class:`subprocess.CompletedProcess`.

        Parameters
        ----------
        *args:
            Arguments forwarded to the ``cargoship`` binary.
        check:
            When *True* (default) raise :class:`CargoShipCLIError` if the
            process exits with a non-zero code.
        env:
            Extra environment variables merged on top of the current process
            environment.
        """
        cmd = [self.binary, *args]
        merged_env: Dict[str, str] = {**os.environ, **(env or {})}
        result = subprocess.run(
            cmd,
            stdout=subprocess.PIPE,
            stderr=subprocess.PIPE,
            text=True,
            env=merged_env,
        )
        if check and result.returncode != 0:
            raise CargoShipCLIError(
                f"cargoship {' '.join(args)} failed "
                f"(exit {result.returncode}):\n"
                f"stdout: {result.stdout}\n"
                f"stderr: {result.stderr}"
            )
        return result

    # ------------------------------------------------------------------
    # High-level commands
    # ------------------------------------------------------------------

    def upload(
        self,
        source_dir: str,
        destination: str,
        *,
        incremental: bool = False,
        prev_manifest: Optional[str] = None,
        generate_dvc_files: bool = False,
        quiet: bool = True,
    ) -> None:
        """Upload *source_dir* to *destination* (``s3://bucket/prefix``).

        Parameters
        ----------
        source_dir:
            Local directory to upload.
        destination:
            S3 destination URL (``s3://bucket/prefix``).
        incremental:
            Pass ``--incremental`` to skip unchanged files.
        prev_manifest:
            Path to previous manifest for incremental comparison
            (``--prev-manifest <path>``).
        generate_dvc_files:
            Pass ``--generate-dvc-files`` to emit ``.dvc`` sidecars.
        quiet:
            Pass ``--quiet`` to suppress progress output.
        """
        args: List[str] = ["upload", source_dir, destination]
        if incremental:
            args.append("--incremental")
        if prev_manifest is not None:
            args += ["--prev-manifest", prev_manifest]
        if generate_dvc_files:
            args.append("--generate-dvc-files")
        if quiet:
            args.append("--quiet")
        self.run(*args)

    def list_files(
        self,
        destination: str,
        upload_id: Optional[str] = None,
    ) -> List[Dict[str, Any]]:
        """Return file entries from the latest manifest at *destination*.

        Each entry is a dict with at least ``path``, ``size``, and
        ``content_hash`` keys (matching CargoShip's ``--json`` output).

        Returns an empty list when the remote has no uploads or the command
        fails so that callers can treat a missing remote as an empty cache.
        """
        args: List[str] = ["list", destination, "--json"]
        if upload_id is not None:
            args += ["--upload-id", upload_id]
        try:
            result = self.run(*args)
        except CargoShipCLIError:
            return []
        try:
            data = json.loads(result.stdout)
            return data if isinstance(data, list) else []
        except json.JSONDecodeError:
            return []

    def info(
        self,
        destination: str,
        upload_id: Optional[str] = None,
    ) -> Dict[str, Any]:
        """Return upload metadata for *destination* as a dict.

        Returns an empty dict when the command fails.
        """
        args: List[str] = ["info", destination, "--json"]
        if upload_id is not None:
            args += ["--upload-id", upload_id]
        try:
            result = self.run(*args)
        except CargoShipCLIError:
            return {}
        try:
            return json.loads(result.stdout)
        except json.JSONDecodeError:
            return {}

    def restore(
        self,
        destination: str,
        target_dir: str,
        *,
        file_path: Optional[str] = None,
        content_hash: Optional[str] = None,
        upload_id: Optional[str] = None,
    ) -> None:
        """Restore files from *destination* into *target_dir*.

        Parameters
        ----------
        destination:
            S3 source URL (``s3://bucket/prefix``).
        target_dir:
            Local directory to restore files into.
        file_path:
            Restore a specific file by its relative path (``--file <path>``).
        content_hash:
            Restore the file matching this MD5 hash (``--hash <md5>``).
        upload_id:
            Restore from a specific upload ID (``--upload-id <id>``).
        """
        args: List[str] = ["restore", destination, target_dir]
        if file_path is not None:
            args += ["--file", file_path]
        if content_hash is not None:
            args += ["--hash", content_hash]
        if upload_id is not None:
            args += ["--upload-id", upload_id]
        self.run(*args)
