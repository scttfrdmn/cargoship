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
        project_id: Optional[str] = None,
        tags: Optional[Dict[str, str]] = None,
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
        project_id:
            Project ID for cost tracking (``--project <id>``).
            DVC remotes typically use ``"dvc_cache"``.
        tags:
            Custom key/value tags recorded in cost entries
            (``--tag key=value``, one flag per pair).
            DVC remotes automatically add ``dvc_cache=true`` and
            ``dvc_operation=push``.
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
        if project_id is not None:
            args += ["--project", project_id]
        if tags:
            for key, value in tags.items():
                args += ["--tag", f"{key}={value}"]
        self.run(*args)

    def budget_status(
        self,
        project_id: Optional[str] = None,
    ) -> Dict[str, Any]:
        """Return budget status for *project_id* as a dict.

        Calls ``cargoship budget status <project_id> --json``.

        Returns an empty dict when no budget is configured, the project does
        not exist, or the ``cargoship`` binary is unavailable / not
        authenticated.
        """
        if project_id is None:
            return {}
        try:
            result = self.run("budget", "status", project_id, "--json")
        except CargoShipCLIError:
            return {}
        try:
            data = json.loads(result.stdout)
            return data if isinstance(data, dict) else {}
        except json.JSONDecodeError:
            return {}

    def cost_estimate(
        self,
        size_bytes: int,
        storage_class: str = "STANDARD",
        region: str = "us-west-2",
    ) -> Dict[str, Any]:
        """Return a cost estimate for uploading *size_bytes* bytes.

        Calls ``cargoship cost estimate --size <bytes>B --storage-class <sc>
        --region <r> --json``.

        Returns an empty dict on failure (e.g. no AWS credentials or network
        unavailable) so that callers can treat a missing estimate as zero cost.
        """
        args: List[str] = [
            "cost", "estimate",
            "--size", f"{size_bytes}B",
            "--storage-class", storage_class,
            "--region", region,
            "--json",
        ]
        try:
            result = self.run(*args)
        except CargoShipCLIError:
            return {}
        try:
            data = json.loads(result.stdout)
            return data if isinstance(data, dict) else {}
        except json.JSONDecodeError:
            return {}

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

        Implemented on top of ``cargoship info --json``: that command takes the
        S3 URL as a positional argument and emits the full manifest as JSON
        (whose ``files`` array holds the file entries). The ``cargoship list``
        command, by contrast, requires ``--bucket``/``--upload-id`` flags and
        prints human-readable text, not JSON.
        """
        manifest = self.info(destination, upload_id=upload_id)
        files = manifest.get("files", [])
        return files if isinstance(files, list) else []

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
