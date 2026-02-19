"""Tests for dvc_cargoship.perf — parse_size, BatchUploadBuffer, parallel_restore."""

from __future__ import annotations

import os
import tempfile
from unittest.mock import MagicMock, call, patch

import pytest

from dvc_cargoship.cli import CargoShipCLIError
from dvc_cargoship.perf import (
    BatchUploadBuffer,
    _DEFAULT_DOWNLOAD_WORKERS,
    _DEFAULT_SMALL_FILE_THRESHOLD,
    parallel_restore,
    parse_size,
)


# ---------------------------------------------------------------------------
# parse_size
# ---------------------------------------------------------------------------


class TestParseSize:
    def test_megabytes(self):
        assert parse_size("10MB") == 10 * 1024**2

    def test_gigabytes(self):
        assert parse_size("1GB") == 1024**3

    def test_kilobytes(self):
        assert parse_size("512KB") == 512 * 1024

    def test_bytes_suffix(self):
        assert parse_size("1024B") == 1024

    def test_bare_integer(self):
        assert parse_size("1048576") == 1048576

    def test_case_insensitive(self):
        assert parse_size("10mb") == parse_size("10MB")
        assert parse_size("1gb") == parse_size("1GB")

    def test_fractional(self):
        assert parse_size("1.5MB") == int(1.5 * 1024**2)

    def test_whitespace_stripped(self):
        assert parse_size("  10 MB  ") == 10 * 1024**2

    def test_zero(self):
        assert parse_size("0") == 0

    def test_invalid_raises(self):
        with pytest.raises(ValueError):
            parse_size("not-a-size")

    def test_invalid_number_with_unit_raises(self):
        with pytest.raises(ValueError):
            parse_size("xMB")

    def test_terabytes(self):
        assert parse_size("2TB") == 2 * 1024**4


# ---------------------------------------------------------------------------
# BatchUploadBuffer
# ---------------------------------------------------------------------------


def _mock_cli():
    cli = MagicMock()
    cli.upload = MagicMock()
    return cli


def _write_file(directory: str, name: str, size: int) -> str:
    """Write a file of *size* bytes and return its path."""
    path = os.path.join(directory, name)
    with open(path, "wb") as f:
        f.write(b"x" * size)
    return path


class TestBatchUploadBufferShouldBuffer:
    def test_small_file_returns_true(self):
        buf = BatchUploadBuffer(_mock_cli(), "s3://b/p", threshold=10 * 1024**2)
        assert buf.should_buffer(5 * 1024**2) is True

    def test_large_file_returns_false(self):
        buf = BatchUploadBuffer(_mock_cli(), "s3://b/p", threshold=10 * 1024**2)
        assert buf.should_buffer(20 * 1024**2) is False

    def test_exact_threshold_returns_false(self):
        buf = BatchUploadBuffer(_mock_cli(), "s3://b/p", threshold=10 * 1024**2)
        assert buf.should_buffer(10 * 1024**2) is False

    def test_zero_threshold_disables_batching(self):
        buf = BatchUploadBuffer(_mock_cli(), "s3://b/p", threshold=0)
        assert buf.should_buffer(0) is False
        assert buf.should_buffer(1) is False


class TestBatchUploadBufferAdd:
    def test_add_increments_file_count(self):
        cli = _mock_cli()
        with tempfile.TemporaryDirectory() as d:
            f = _write_file(d, "a.txt", 100)
            buf = BatchUploadBuffer(cli, "s3://b/p")
            buf.add(f, "a.txt")
            assert buf.file_count == 1

    def test_add_increments_buffered_bytes(self):
        cli = _mock_cli()
        with tempfile.TemporaryDirectory() as d:
            f = _write_file(d, "a.txt", 200)
            buf = BatchUploadBuffer(cli, "s3://b/p")
            buf.add(f, "a.txt")
            assert buf.buffered_bytes == 200

    def test_add_creates_symlink_in_staging(self):
        cli = _mock_cli()
        with tempfile.TemporaryDirectory() as d:
            f = _write_file(d, "data.csv", 50)
            buf = BatchUploadBuffer(cli, "s3://b/p")
            buf.add(f, "data.csv")
            staging = buf._staging_path
            assert staging is not None
            assert os.path.exists(os.path.join(staging, "data.csv"))
            buf.close()

    def test_add_preserves_relative_directory_structure(self):
        cli = _mock_cli()
        with tempfile.TemporaryDirectory() as d:
            f = _write_file(d, "a.csv", 10)
            buf = BatchUploadBuffer(cli, "s3://b/p")
            buf.add(f, "subdir/nested/a.csv")
            staging = buf._staging_path
            assert os.path.exists(os.path.join(staging, "subdir", "nested", "a.csv"))
            buf.close()

    def test_add_strips_leading_slash_from_key(self):
        cli = _mock_cli()
        with tempfile.TemporaryDirectory() as d:
            f = _write_file(d, "a.csv", 10)
            buf = BatchUploadBuffer(cli, "s3://b/p")
            buf.add(f, "/a.csv")
            staging = buf._staging_path
            assert os.path.exists(os.path.join(staging, "a.csv"))
            buf.close()

    def test_auto_flush_on_size_threshold(self):
        cli = _mock_cli()
        with tempfile.TemporaryDirectory() as d:
            f1 = _write_file(d, "a.bin", 30)
            f2 = _write_file(d, "b.bin", 30)
            buf = BatchUploadBuffer(
                cli, "s3://b/p", threshold=100, auto_flush_bytes=50
            )
            buf.add(f1, "a.bin")
            assert cli.upload.call_count == 0
            buf.add(f2, "b.bin")
            # total = 60 >= auto_flush_bytes=50 → should have auto-flushed
            assert cli.upload.call_count == 1
            assert buf.file_count == 0


class TestBatchUploadBufferFlush:
    def test_flush_calls_cli_upload(self):
        cli = _mock_cli()
        with tempfile.TemporaryDirectory() as d:
            f = _write_file(d, "x.csv", 10)
            buf = BatchUploadBuffer(cli, "s3://b/p")
            buf.add(f, "x.csv")
            buf.flush()
        cli.upload.assert_called_once()
        call_args = cli.upload.call_args
        assert call_args[0][1] == "s3://b/p"

    def test_flush_with_empty_buffer_does_nothing(self):
        cli = _mock_cli()
        buf = BatchUploadBuffer(cli, "s3://b/p")
        buf.flush()
        cli.upload.assert_not_called()

    def test_flush_resets_counts(self):
        cli = _mock_cli()
        with tempfile.TemporaryDirectory() as d:
            f = _write_file(d, "x.csv", 10)
            buf = BatchUploadBuffer(cli, "s3://b/p")
            buf.add(f, "x.csv")
            buf.flush()
        assert buf.file_count == 0
        assert buf.buffered_bytes == 0

    def test_multiple_flushes_call_upload_each_time(self):
        cli = _mock_cli()
        with tempfile.TemporaryDirectory() as d:
            f1 = _write_file(d, "a.csv", 5)
            f2 = _write_file(d, "b.csv", 5)
            buf = BatchUploadBuffer(cli, "s3://b/p")
            buf.add(f1, "a.csv")
            buf.flush()
            buf.add(f2, "b.csv")
            buf.flush()
        assert cli.upload.call_count == 2


class TestBatchUploadBufferClose:
    def test_close_flushes_remaining_files(self):
        cli = _mock_cli()
        with tempfile.TemporaryDirectory() as d:
            f = _write_file(d, "x.csv", 10)
            buf = BatchUploadBuffer(cli, "s3://b/p")
            buf.add(f, "x.csv")
            buf.close()
        cli.upload.assert_called_once()

    def test_close_is_idempotent(self):
        cli = _mock_cli()
        buf = BatchUploadBuffer(cli, "s3://b/p")
        buf.close()
        buf.close()  # should not raise

    def test_context_manager_closes_on_exit(self):
        cli = _mock_cli()
        with tempfile.TemporaryDirectory() as d:
            f = _write_file(d, "x.csv", 10)
            with BatchUploadBuffer(cli, "s3://b/p") as buf:
                buf.add(f, "x.csv")
        cli.upload.assert_called_once()


# ---------------------------------------------------------------------------
# parallel_restore
# ---------------------------------------------------------------------------


class TestParallelRestore:
    def _make_cli_that_writes(self, content: bytes = b"data") -> MagicMock:
        """Return a mock CLI whose restore() writes *content* to the staging dir."""

        def _restore(dest, staging, *, file_path=None, **kwargs):
            out = os.path.join(staging, os.path.basename(file_path))
            with open(out, "wb") as fh:
                fh.write(content)

        cli = MagicMock()
        cli.restore.side_effect = _restore
        return cli

    def test_restores_all_files(self):
        cli = self._make_cli_that_writes()
        with tempfile.TemporaryDirectory() as outdir:
            files = [
                ("a.txt", os.path.join(outdir, "a.txt")),
                ("b.txt", os.path.join(outdir, "b.txt")),
            ]
            results = parallel_restore(cli, "s3://b/p", files)
            assert results == {"a.txt": None, "b.txt": None}
            assert os.path.exists(os.path.join(outdir, "a.txt"))
            assert os.path.exists(os.path.join(outdir, "b.txt"))

    def test_returns_exception_on_failure(self):
        cli = MagicMock()
        cli.restore.side_effect = CargoShipCLIError("not found")
        with tempfile.TemporaryDirectory() as outdir:
            files = [("missing.txt", os.path.join(outdir, "missing.txt"))]
            results = parallel_restore(cli, "s3://b/p", files)
        assert isinstance(results["missing.txt"], CargoShipCLIError)

    def test_partial_failure_does_not_abort_others(self):
        call_count = {"n": 0}

        def _restore(dest, staging, *, file_path=None, **kwargs):
            call_count["n"] += 1
            if file_path == "bad.txt":
                raise CargoShipCLIError("fail")
            out = os.path.join(staging, os.path.basename(file_path))
            with open(out, "wb") as fh:
                fh.write(b"ok")

        cli = MagicMock()
        cli.restore.side_effect = _restore
        with tempfile.TemporaryDirectory() as outdir:
            files = [
                ("good.txt", os.path.join(outdir, "good.txt")),
                ("bad.txt", os.path.join(outdir, "bad.txt")),
            ]
            results = parallel_restore(cli, "s3://b/p", files, workers=2)
        assert results["good.txt"] is None
        assert isinstance(results["bad.txt"], CargoShipCLIError)
        assert call_count["n"] == 2

    def test_progress_callback_is_invoked(self):
        cli = self._make_cli_that_writes()
        progress = []
        with tempfile.TemporaryDirectory() as outdir:
            files = [
                ("a.txt", os.path.join(outdir, "a.txt")),
                ("b.txt", os.path.join(outdir, "b.txt")),
            ]
            parallel_restore(
                cli, "s3://b/p", files, progress_callback=progress.append
            )
        assert sorted(progress) == [1, 2]

    def test_empty_file_list_returns_empty_dict(self):
        cli = MagicMock()
        results = parallel_restore(cli, "s3://b/p", [])
        assert results == {}
        cli.restore.assert_not_called()

    def test_raises_file_not_found_when_restore_produces_nothing(self):
        cli = MagicMock()
        cli.restore = MagicMock()  # does nothing — file not created
        with tempfile.TemporaryDirectory() as outdir:
            files = [("phantom.txt", os.path.join(outdir, "phantom.txt"))]
            results = parallel_restore(cli, "s3://b/p", files)
        assert isinstance(results["phantom.txt"], FileNotFoundError)

    def test_workers_default(self):
        """parallel_restore uses _DEFAULT_DOWNLOAD_WORKERS by default."""
        cli = MagicMock()
        cli.restore = MagicMock()
        with patch("dvc_cargoship.perf.ThreadPoolExecutor") as MockPool:
            MockPool.return_value.__enter__ = lambda s: s
            MockPool.return_value.__exit__ = MagicMock(return_value=False)
            MockPool.return_value.submit = MagicMock(return_value=MagicMock())
            MockPool.return_value.submit.return_value.result = MagicMock(
                return_value=("k", None)
            )
            # Just verify it's called with the default workers
            parallel_restore(cli, "s3://b/p", [])
        MockPool.assert_called_once_with(max_workers=_DEFAULT_DOWNLOAD_WORKERS)


# ---------------------------------------------------------------------------
# BatchUploadBuffer — project_id and tags (Issue #183)
# ---------------------------------------------------------------------------


class TestBatchUploadBufferProjectAndTags:
    def test_flush_passes_project_id(self):
        cli = _mock_cli()
        with tempfile.TemporaryDirectory() as d:
            f = _write_file(d, "a.csv", 10)
            buf = BatchUploadBuffer(cli, "s3://b/p", project_id="dvc_cache")
            buf.add(f, "a.csv")
            buf.flush()
        call_kwargs = cli.upload.call_args[1]
        assert call_kwargs.get("project_id") == "dvc_cache"

    def test_flush_passes_tags(self):
        cli = _mock_cli()
        tags = {"dvc_cache": "true", "dvc_operation": "push"}
        with tempfile.TemporaryDirectory() as d:
            f = _write_file(d, "a.csv", 10)
            buf = BatchUploadBuffer(cli, "s3://b/p", tags=tags)
            buf.add(f, "a.csv")
            buf.flush()
        call_kwargs = cli.upload.call_args[1]
        assert call_kwargs.get("tags") == tags

    def test_flush_passes_none_when_no_tags(self):
        cli = _mock_cli()
        with tempfile.TemporaryDirectory() as d:
            f = _write_file(d, "a.csv", 10)
            buf = BatchUploadBuffer(cli, "s3://b/p")
            buf.add(f, "a.csv")
            buf.flush()
        call_kwargs = cli.upload.call_args[1]
        assert call_kwargs.get("tags") is None

    def test_flush_passes_none_project_id_when_not_set(self):
        cli = _mock_cli()
        with tempfile.TemporaryDirectory() as d:
            f = _write_file(d, "a.csv", 10)
            buf = BatchUploadBuffer(cli, "s3://b/p")
            buf.add(f, "a.csv")
            buf.flush()
        call_kwargs = cli.upload.call_args[1]
        assert call_kwargs.get("project_id") is None
