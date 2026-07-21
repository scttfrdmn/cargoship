"""Integration tests: full push/pull workflow against a mock cargoship binary.

These tests create a real temporary cargoship "remote" backed by a fake
binary script that stores files in a local directory, validating the complete
``put_file`` → ``list`` → ``get_file`` pipeline without a real S3 bucket or
the actual ``cargoship`` binary.

The mock binary supports the subset of commands used by CargoShipCLI:
- ``upload <source_dir> <dest_url> [flags...]``
- ``restore <dest_url> <staging_dir> [--file <path>]``
- ``info <dest_url> --json`` (manifest with a ``files`` array; backs list_files)
"""

from __future__ import annotations

import json
import os
import stat
import tempfile
import textwrap
from typing import Tuple

import pytest

from dvc_cargoship import CargoShipFileSystem


# ---------------------------------------------------------------------------
# Fixture: mock cargoship binary
# ---------------------------------------------------------------------------


@pytest.fixture
def mock_cargoship(tmp_path) -> Tuple[str, str]:
    """Return ``(binary_path, store_dir)`` for a mock cargoship installation.

    The binary stores uploaded files under ``store_dir`` and serves them back
    on ``restore``.  A ``list`` call returns a JSON array of all stored files.
    """
    store_dir = tmp_path / "store"
    store_dir.mkdir()

    script = textwrap.dedent(
        f"""\
        #!/usr/bin/env python3
        \"\"\"Mock cargoship binary for integration tests.\"\"\"
        import json, os, shutil, sys

        STORE = {str(store_dir)!r}


        def cmd_upload(argv):
            # argv: <source_dir> <s3://...> [--quiet] [--project <id>] [--tag k=v] ...
            source_dir = argv[0]
            for root, _dirs, files in os.walk(source_dir):
                for fname in files:
                    src = os.path.join(root, fname)
                    rel = os.path.relpath(src, source_dir)
                    dst = os.path.join(STORE, rel)
                    os.makedirs(os.path.dirname(dst) if os.path.dirname(rel) else STORE,
                                exist_ok=True)
                    shutil.copy2(src, dst)


        def cmd_restore(argv):
            # argv: <s3://...> <staging_dir> [--file <path>] [--hash <hash>] ...
            staging = argv[1]
            file_path = None
            i = 2
            while i < len(argv):
                if argv[i] == "--file" and i + 1 < len(argv):
                    file_path = argv[i + 1]
                    i += 2
                else:
                    i += 1
            if file_path:
                src = os.path.join(STORE, file_path)
                if not os.path.exists(src):
                    print(f"not found: {{file_path}}", file=sys.stderr)
                    sys.exit(1)
                dst = os.path.join(staging, os.path.basename(file_path))
                shutil.copy2(src, dst)


        def cmd_info(argv):
            # argv: <s3://...> --json [--upload-id <id>]
            # Emit the manifest as JSON, matching the real binary's
            # `cargoship info --json` (manifest.ToJSON): an object whose "files"
            # array holds the file entries.
            entries = []
            for root, _dirs, files in os.walk(STORE):
                for fname in files:
                    full = os.path.join(root, fname)
                    rel = os.path.relpath(full, STORE)
                    entries.append({{
                        "path": rel,
                        "size": os.path.getsize(full),
                        "content_hash": "",
                    }})
            print(json.dumps({{
                "version": "2.0",
                "total_files": len(entries),
                "files": entries,
            }}))


        COMMANDS = {{"upload": cmd_upload, "restore": cmd_restore, "info": cmd_info}}
        cmd = sys.argv[1] if len(sys.argv) > 1 else ""
        handler = COMMANDS.get(cmd)
        if handler is None:
            print(f"unknown command: {{cmd}}", file=sys.stderr)
            sys.exit(1)
        handler(sys.argv[2:])
        """
    )

    bin_path = tmp_path / "cargoship"
    bin_path.write_text(script)
    bin_path.chmod(bin_path.stat().st_mode | stat.S_IEXEC | stat.S_IXGRP | stat.S_IXOTH)
    return str(bin_path), str(store_dir)


def _fs(bin_path: str, bucket: str = "test-bucket", prefix: str = "cache") -> CargoShipFileSystem:
    """Return a CargoShipFileSystem that uses the mock binary.

    ``small_file_threshold=0`` disables batching so every ``put_file`` call
    triggers an immediate ``cargoship upload``.
    """
    # Use a unique storage_options dict to avoid fsspec instance caching
    # across tests that share bucket/prefix.
    return CargoShipFileSystem(
        bucket=bucket,
        prefix=prefix,
        cargoship_bin=bin_path,
        small_file_threshold=0,
        skip_instance_cache=True,
    )


# ---------------------------------------------------------------------------
# TestPutThenGetFile
# ---------------------------------------------------------------------------


class TestPutThenGetFile:
    def test_single_file_roundtrip(self, mock_cargoship, tmp_path):
        bin_path, _store = mock_cargoship
        src = tmp_path / "data.csv"
        src.write_text("col1,col2\n1,2\n3,4\n")

        fs = _fs(bin_path)
        fs.put_file(str(src), "cargoship://test-bucket/cache/data.csv")

        dst = tmp_path / "restored.csv"
        fs.get_file("cargoship://test-bucket/cache/data.csv", str(dst))

        assert dst.exists()
        assert dst.read_text() == src.read_text()

    def test_binary_file_roundtrip(self, mock_cargoship, tmp_path):
        bin_path, _store = mock_cargoship
        src = tmp_path / "weights.bin"
        src.write_bytes(bytes(range(256)) * 100)

        fs = _fs(bin_path)
        fs.put_file(str(src), "cargoship://test-bucket/cache/weights.bin")

        dst = tmp_path / "restored.bin"
        fs.get_file("cargoship://test-bucket/cache/weights.bin", str(dst))

        assert dst.read_bytes() == src.read_bytes()

    def test_get_missing_file_raises(self, mock_cargoship, tmp_path):
        bin_path, _store = mock_cargoship
        fs = _fs(bin_path)
        with pytest.raises((FileNotFoundError, Exception)):
            fs.get_file(
                "cargoship://test-bucket/cache/missing.txt",
                str(tmp_path / "out.txt"),
            )


# ---------------------------------------------------------------------------
# TestListFiles
# ---------------------------------------------------------------------------


class TestListFiles:
    def test_empty_remote_returns_empty_list(self, mock_cargoship):
        bin_path, _store = mock_cargoship
        fs = _fs(bin_path)
        result = fs.ls("cargoship://test-bucket/cache", detail=False)
        assert result == []

    def test_list_shows_uploaded_file(self, mock_cargoship, tmp_path):
        bin_path, _store = mock_cargoship
        src = tmp_path / "model.pkl"
        src.write_bytes(b"fake-model-data")

        fs = _fs(bin_path)
        fs.put_file(str(src), "cargoship://test-bucket/cache/model.pkl")

        paths = fs.ls("cargoship://test-bucket/cache", detail=False)
        assert any("model.pkl" in p for p in paths)

    def test_list_detail_true_returns_dicts(self, mock_cargoship, tmp_path):
        bin_path, _store = mock_cargoship
        src = tmp_path / "a.txt"
        src.write_text("hello")

        fs = _fs(bin_path)
        fs.put_file(str(src), "cargoship://test-bucket/cache/a.txt")

        entries = fs.ls("cargoship://test-bucket/cache", detail=True)
        assert len(entries) == 1
        assert entries[0]["type"] == "file"
        assert entries[0]["size"] == len("hello")


# ---------------------------------------------------------------------------
# TestExistsAndInfo
# ---------------------------------------------------------------------------


class TestExistsAndInfo:
    def test_exists_false_when_empty(self, mock_cargoship):
        bin_path, _store = mock_cargoship
        fs = _fs(bin_path)
        assert fs.exists("cargoship://test-bucket/cache/nope.txt") is False

    def test_exists_true_after_upload(self, mock_cargoship, tmp_path):
        bin_path, _store = mock_cargoship
        src = tmp_path / "x.txt"
        src.write_text("x")

        fs = _fs(bin_path)
        fs.put_file(str(src), "cargoship://test-bucket/cache/x.txt")

        assert fs.exists("cargoship://test-bucket/cache/x.txt") is True

    def test_info_raises_for_missing(self, mock_cargoship):
        bin_path, _store = mock_cargoship
        fs = _fs(bin_path)
        with pytest.raises(FileNotFoundError):
            fs.info("cargoship://test-bucket/cache/ghost.txt")


# ---------------------------------------------------------------------------
# TestBatchUpload (small_file_threshold enabled)
# ---------------------------------------------------------------------------


class TestBatchUpload:
    def test_batch_flushes_multiple_small_files(self, mock_cargoship, tmp_path):
        bin_path, store = mock_cargoship

        # Use a high threshold so all test files are batched
        fs = CargoShipFileSystem(
            bucket="test-bucket",
            prefix="cache",
            cargoship_bin=bin_path,
            small_file_threshold=10 * 1024 * 1024,  # 10 MB
            skip_instance_cache=True,
        )

        files = []
        for i in range(3):
            f = tmp_path / f"file{i}.txt"
            f.write_text(f"content {i}")
            files.append(f)
            fs.put_file(str(f), f"cargoship://test-bucket/cache/file{i}.txt")

        # Files should not be in the store yet (still buffered)
        stored_before = list(os.listdir(store))

        # Flush the batch
        fs.flush_batch()

        # Now all files should be in the store
        for i in range(3):
            assert os.path.exists(os.path.join(store, f"file{i}.txt")), \
                f"file{i}.txt not found in store after flush"

    def test_close_flushes_pending_batch(self, mock_cargoship, tmp_path):
        bin_path, store = mock_cargoship
        src = tmp_path / "data.csv"
        src.write_text("a,b\n1,2\n")

        fs = CargoShipFileSystem(
            bucket="test-bucket",
            prefix="cache",
            cargoship_bin=bin_path,
            small_file_threshold=10 * 1024 * 1024,
            skip_instance_cache=True,
        )
        fs.put_file(str(src), "cargoship://test-bucket/cache/data.csv")
        fs.close()

        # After close(), batch should be flushed
        assert os.path.exists(os.path.join(store, "data.csv"))


# ---------------------------------------------------------------------------
# TestGetFiles (parallel restore)
# ---------------------------------------------------------------------------


class TestGetFiles:
    def test_get_multiple_files_in_parallel(self, mock_cargoship, tmp_path):
        bin_path, _store = mock_cargoship

        # Upload several files
        fs_put = _fs(bin_path)
        for i in range(4):
            f = tmp_path / f"src{i}.txt"
            f.write_text(f"file {i} data")
            fs_put.put_file(str(f), f"cargoship://test-bucket/cache/src{i}.txt")

        # Restore all in parallel using get_files
        fs_get = _fs(bin_path)
        rpaths = [f"cargoship://test-bucket/cache/src{i}.txt" for i in range(4)]
        lpaths = [str(tmp_path / f"dst{i}.txt") for i in range(4)]
        fs_get.get_files(rpaths, lpaths)

        for i in range(4):
            dst = tmp_path / f"dst{i}.txt"
            assert dst.exists(), f"dst{i}.txt not restored"
            assert dst.read_text() == f"file {i} data"
