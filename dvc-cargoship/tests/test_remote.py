"""Tests for CargoShipFileSystem fsspec adapter."""

from __future__ import annotations

import os
import tempfile
from unittest.mock import MagicMock, patch

import pytest

from dvc_cargoship.cli import CargoShipCLIError
from dvc_cargoship.remote import CargoShipFileSystem


# ---------------------------------------------------------------------------
# Helpers
# ---------------------------------------------------------------------------


def _make_fs(bucket: str = "my-bucket", prefix: str = "my-prefix") -> CargoShipFileSystem:
    """Return a filesystem with a mocked CLI."""
    fs = CargoShipFileSystem(bucket=bucket, prefix=prefix)
    fs.cli = MagicMock()
    return fs


def _entry(path: str, size: int = 100, content_hash: str = "abc") -> dict:
    return {"path": path, "size": size, "content_hash": content_hash}


# ---------------------------------------------------------------------------
# Initialisation and URL helpers
# ---------------------------------------------------------------------------


class TestInit:
    def test_strips_trailing_slash_from_bucket(self):
        fs = CargoShipFileSystem(bucket="bucket/")
        assert fs._bucket == "bucket"

    def test_strips_trailing_slash_from_prefix(self):
        fs = CargoShipFileSystem(prefix="/prefix/")
        assert fs._prefix == "prefix"

    def test_cli_property_creates_instance_lazily(self):
        fs = CargoShipFileSystem(bucket="b")
        fs._cli = None  # reset: fsspec may return a cached instance
        with patch("dvc_cargoship.remote.CargoShipCLI") as MockCLI:
            _ = fs.cli
        MockCLI.assert_called_once_with(binary="cargoship")

    def test_cli_property_caches_instance(self):
        fs = CargoShipFileSystem(bucket="b")
        fs._cli = None  # reset: fsspec may return a cached instance
        with patch("dvc_cargoship.remote.CargoShipCLI") as MockCLI:
            a = fs.cli
            b = fs.cli
        assert MockCLI.call_count == 1
        assert a is b

    def test_cli_setter_injects_instance(self):
        fs = CargoShipFileSystem(bucket="b")
        mock_cli = MagicMock()
        fs.cli = mock_cli
        assert fs.cli is mock_cli


class TestURLHelpers:
    def test_strip_protocol_removes_scheme(self):
        fs = CargoShipFileSystem()
        assert fs._strip_protocol("cargoship://bucket/key") == "bucket/key"

    def test_strip_protocol_passthrough(self):
        fs = CargoShipFileSystem()
        assert fs._strip_protocol("bucket/key") == "bucket/key"

    def test_destination_url_with_prefix(self):
        fs = CargoShipFileSystem(bucket="b", prefix="p")
        assert fs._destination_url() == "s3://b/p"

    def test_destination_url_without_prefix(self):
        fs = CargoShipFileSystem(bucket="b", prefix="")
        assert fs._destination_url() == "s3://b"

    def test_parse_path_bucket_and_key(self):
        fs = CargoShipFileSystem(bucket="b")
        bucket, key = fs._parse_path("cargoship://other-bucket/some/key")
        assert bucket == "other-bucket"
        assert key == "some/key"

    def test_parse_path_falls_back_to_self_bucket(self):
        fs = CargoShipFileSystem(bucket="default-bucket")
        bucket, key = fs._parse_path("default-bucket")
        assert bucket == "default-bucket"
        assert key == ""

    def test_relative_key_strips_prefix(self):
        fs = CargoShipFileSystem(bucket="b", prefix="pfx")
        path = "cargoship://b/pfx/data/a.txt"
        assert fs._relative_key(path) == "data/a.txt"

    def test_relative_key_no_prefix(self):
        fs = CargoShipFileSystem(bucket="b", prefix="")
        path = "cargoship://b/data/a.txt"
        assert fs._relative_key(path) == "data/a.txt"


# ---------------------------------------------------------------------------
# ls()
# ---------------------------------------------------------------------------


class TestLs:
    def test_returns_detail_entries(self):
        fs = _make_fs(bucket="b", prefix="p")
        fs.cli.list_files.return_value = [_entry("file.txt", size=42, content_hash="deadbeef")]
        results = fs.ls("cargoship://b/p", detail=True)
        assert len(results) == 1
        r = results[0]
        assert r["type"] == "file"
        assert r["size"] == 42
        assert r["md5"] == "deadbeef"
        assert "file.txt" in r["name"]

    def test_returns_path_strings_when_no_detail(self):
        fs = _make_fs(bucket="b", prefix="p")
        fs.cli.list_files.return_value = [_entry("file.txt")]
        results = fs.ls("cargoship://b/p", detail=False)
        assert isinstance(results[0], str)
        assert "file.txt" in results[0]

    def test_filters_by_prefix(self):
        fs = _make_fs(bucket="b", prefix="")
        fs.cli.list_files.return_value = [
            _entry("a/x.txt"),
            _entry("b/y.txt"),
        ]
        results = fs.ls("cargoship://b/a", detail=False)
        assert len(results) == 1
        assert "a/x.txt" in results[0]

    def test_returns_empty_list_when_no_entries(self):
        fs = _make_fs()
        fs.cli.list_files.return_value = []
        assert fs.ls("cargoship://my-bucket/my-prefix") == []

    def test_calls_list_files_with_destination(self):
        fs = _make_fs(bucket="b", prefix="p")
        fs.cli.list_files.return_value = []
        fs.ls("cargoship://b/p")
        fs.cli.list_files.assert_called_once_with("s3://b/p")


# ---------------------------------------------------------------------------
# info()
# ---------------------------------------------------------------------------


class TestInfo:
    def test_returns_entry_for_existing_path(self):
        fs = _make_fs(bucket="b", prefix="p")
        fs.cli.list_files.return_value = [_entry("data.csv")]
        result = fs.info("cargoship://b/p/data.csv")
        assert result["type"] == "file"

    def test_raises_file_not_found_for_missing_path(self):
        fs = _make_fs(bucket="b", prefix="p")
        fs.cli.list_files.return_value = []
        with pytest.raises(FileNotFoundError):
            fs.info("cargoship://b/p/missing.csv")


# ---------------------------------------------------------------------------
# exists()
# ---------------------------------------------------------------------------


class TestExists:
    def test_returns_true_when_present(self):
        fs = _make_fs(bucket="b", prefix="p")
        fs.cli.list_files.return_value = [_entry("data.csv")]
        assert fs.exists("cargoship://b/p/data.csv") is True

    def test_returns_false_when_absent(self):
        fs = _make_fs(bucket="b", prefix="p")
        fs.cli.list_files.return_value = []
        assert fs.exists("cargoship://b/p/missing.csv") is False


# ---------------------------------------------------------------------------
# put_file()
# ---------------------------------------------------------------------------


class TestPutFile:
    def test_copies_file_to_staging_and_uploads(self):
        fs = _make_fs(bucket="b", prefix="p")
        with tempfile.NamedTemporaryFile(suffix=".csv", delete=False) as f:
            f.write(b"a,b,c\n")
            local_path = f.name
        try:
            fs.put_file(local_path, "cargoship://b/p/data.csv")
            fs.flush_batch()  # small file is buffered; flush to trigger upload
        finally:
            os.unlink(local_path)
        fs.cli.upload.assert_called_once()
        call_kwargs = fs.cli.upload.call_args
        # destination should be s3://b/p
        assert "s3://b/p" in call_kwargs[0] or "s3://b/p" in str(call_kwargs)

    def test_staging_directory_is_cleaned_up(self):
        fs = _make_fs()
        staging_dirs = []

        def capture_staging(staging, *args, **kwargs):
            staging_dirs.append(staging)

        fs.cli.upload = capture_staging

        with tempfile.NamedTemporaryFile(delete=False) as f:
            f.write(b"data")
            local_path = f.name
        try:
            fs.put_file(local_path, "cargoship://my-bucket/my-prefix/f.txt")
            fs.flush_batch()  # small file is buffered; flush to trigger upload + cleanup
        finally:
            os.unlink(local_path)

        assert len(staging_dirs) == 1
        assert not os.path.exists(staging_dirs[0])


# ---------------------------------------------------------------------------
# get_file()
# ---------------------------------------------------------------------------


class TestGetFile:
    def test_restores_file_to_lpath(self):
        fs = _make_fs(bucket="b", prefix="p")

        def fake_restore(dest, staging, *, file_path=None, **kwargs):
            # Simulate cargoship writing the file to staging
            out = os.path.join(staging, os.path.basename(file_path))
            with open(out, "w") as fh:
                fh.write("restored content")

        fs.cli.restore = fake_restore

        with tempfile.TemporaryDirectory() as outdir:
            lpath = os.path.join(outdir, "result.csv")
            fs.get_file("cargoship://b/p/data/result.csv", lpath)
            assert os.path.exists(lpath)
            with open(lpath) as fh:
                assert fh.read() == "restored content"

    def test_raises_if_restore_does_not_produce_file(self):
        fs = _make_fs(bucket="b", prefix="p")
        fs.cli.restore = MagicMock()  # does nothing, file not created

        with tempfile.TemporaryDirectory() as outdir:
            lpath = os.path.join(outdir, "out.csv")
            with pytest.raises(FileNotFoundError, match="cargoship restore did not produce"):
                fs.get_file("cargoship://b/p/out.csv", lpath)


# ---------------------------------------------------------------------------
# rm() and copy()
# ---------------------------------------------------------------------------


class TestNotSupported:
    def test_rm_raises_not_implemented(self):
        fs = _make_fs()
        with pytest.raises(NotImplementedError, match="immutable"):
            fs.rm("cargoship://b/p/file.txt")

    def test_copy_raises_not_implemented(self):
        fs = _make_fs()
        with pytest.raises(NotImplementedError, match="copy"):
            fs.copy("cargoship://b/p/a.txt", "cargoship://b/p/b.txt")


# ---------------------------------------------------------------------------
# list_cache_paths() — DVC 2.x compat
# ---------------------------------------------------------------------------


class TestListCachePaths:
    def test_returns_all_paths(self):
        fs = _make_fs()
        fs.cli.list_files.return_value = [
            _entry("a.txt"),
            _entry("b/c.txt"),
        ]
        paths = fs.list_cache_paths()
        assert set(paths) == {"a.txt", "b/c.txt"}

    def test_filters_by_prefix(self):
        fs = _make_fs()
        fs.cli.list_files.return_value = [
            _entry("data/a.txt"),
            _entry("meta/b.txt"),
        ]
        paths = fs.list_cache_paths(prefix="data/")
        assert paths == ["data/a.txt"]

    def test_invokes_progress_callback(self):
        fs = _make_fs()
        fs.cli.list_files.return_value = [_entry("a.txt"), _entry("b.txt")]
        counts = []
        fs.list_cache_paths(progress_callback=lambda n: counts.append(n))
        assert counts == [1, 2]

    def test_skips_entries_with_empty_path(self):
        fs = _make_fs()
        fs.cli.list_files.return_value = [{"path": "", "size": 0}]
        assert fs.list_cache_paths() == []


# ---------------------------------------------------------------------------
# DVC 2.x upload / download / remove compat wrappers
# ---------------------------------------------------------------------------


class TestDVC2Compat:
    def test_upload_delegates_to_put_file(self):
        fs = _make_fs()
        fs.put_file = MagicMock()
        fs.upload("/local/file.txt", "cargoship://b/p/file.txt")
        fs.put_file.assert_called_once_with("/local/file.txt", "cargoship://b/p/file.txt")

    def test_download_delegates_to_get_file(self):
        fs = _make_fs()
        fs.get_file = MagicMock()
        fs.download("cargoship://b/p/file.txt", "/local/file.txt")
        fs.get_file.assert_called_once_with("cargoship://b/p/file.txt", "/local/file.txt")

    def test_remove_delegates_to_rm(self):
        fs = _make_fs()
        fs.rm = MagicMock(side_effect=NotImplementedError)
        with pytest.raises(NotImplementedError):
            fs.remove("cargoship://b/p/file.txt")
        fs.rm.assert_called_once_with("cargoship://b/p/file.txt")


# ---------------------------------------------------------------------------
# Performance config via constructor (dvc remote modify)
# ---------------------------------------------------------------------------


class TestPerfConfig:
    def test_default_small_file_threshold(self):
        fs = CargoShipFileSystem(bucket="b")
        assert fs._small_file_threshold == 10 * 1024 * 1024

    def test_string_threshold_parsed(self):
        fs = CargoShipFileSystem(bucket="b", small_file_threshold="5MB")
        assert fs._small_file_threshold == 5 * 1024 * 1024

    def test_integer_threshold_accepted(self):
        fs = CargoShipFileSystem(bucket="b", small_file_threshold=1024)
        assert fs._small_file_threshold == 1024

    def test_zero_threshold_disables_batching(self):
        fs = CargoShipFileSystem(bucket="b", small_file_threshold="0")
        assert fs._small_file_threshold == 0

    def test_download_workers_string(self):
        fs = CargoShipFileSystem(bucket="b", download_workers="8")
        assert fs._download_workers == 8

    def test_download_workers_int(self):
        fs = CargoShipFileSystem(bucket="b", download_workers=2)
        assert fs._download_workers == 2


# ---------------------------------------------------------------------------
# put_file — small file batching
# ---------------------------------------------------------------------------


class TestPutFileSmallFileBatching:
    def _make_fs(self, threshold: int = 10 * 1024 * 1024) -> CargoShipFileSystem:
        fs = CargoShipFileSystem(
            bucket="b", prefix="p", small_file_threshold=threshold
        )
        fs.cli = MagicMock()
        fs.cli.upload = MagicMock()
        return fs

    def test_small_file_does_not_call_upload_immediately(self):
        fs = self._make_fs(threshold=10 * 1024 * 1024)
        with tempfile.NamedTemporaryFile(delete=False) as f:
            f.write(b"x" * 100)  # 100 bytes < 10MB
            lpath = f.name
        try:
            fs.put_file(lpath, "cargoship://b/p/tiny.bin")
            fs.cli.upload.assert_not_called()
        finally:
            os.unlink(lpath)
            fs.close()

    def test_small_file_uploaded_after_flush(self):
        fs = self._make_fs(threshold=10 * 1024 * 1024)
        with tempfile.NamedTemporaryFile(delete=False) as f:
            f.write(b"x" * 100)
            lpath = f.name
        try:
            fs.put_file(lpath, "cargoship://b/p/tiny.bin")
            fs.flush_batch()
            fs.cli.upload.assert_called_once()
        finally:
            os.unlink(lpath)

    def test_large_file_uploaded_immediately(self):
        fs = self._make_fs(threshold=100)  # 100-byte threshold
        with tempfile.NamedTemporaryFile(delete=False) as f:
            f.write(b"x" * 200)  # 200 bytes > 100-byte threshold
            lpath = f.name
        try:
            fs.put_file(lpath, "cargoship://b/p/big.bin")
            fs.cli.upload.assert_called_once()
        finally:
            os.unlink(lpath)
            fs.close()

    def test_multiple_small_files_batched_in_one_upload(self):
        fs = self._make_fs(threshold=10 * 1024 * 1024)
        paths = []
        try:
            for i in range(3):
                f = tempfile.NamedTemporaryFile(delete=False, suffix=".bin")
                f.write(b"x" * 10)
                f.close()
                paths.append(f.name)
                fs.put_file(f.name, f"cargoship://b/p/file{i}.bin")
            fs.flush_batch()
            # All 3 files should be uploaded in a single call
            assert fs.cli.upload.call_count == 1
        finally:
            for p in paths:
                os.unlink(p)

    def test_close_flushes_pending_small_files(self):
        fs = self._make_fs(threshold=10 * 1024 * 1024)
        with tempfile.NamedTemporaryFile(delete=False) as f:
            f.write(b"y" * 50)
            lpath = f.name
        try:
            fs.put_file(lpath, "cargoship://b/p/pending.bin")
            fs.close()
            fs.cli.upload.assert_called_once()
        finally:
            os.unlink(lpath)


# ---------------------------------------------------------------------------
# get_files — parallel restore
# ---------------------------------------------------------------------------


class TestGetFiles:
    def _make_fs(self) -> CargoShipFileSystem:
        fs = CargoShipFileSystem(bucket="b", prefix="p", download_workers=2)
        fs.cli = MagicMock()
        return fs

    def _make_restoring_cli(self) -> MagicMock:
        def _restore(dest, staging, *, file_path=None, **kwargs):
            out = os.path.join(staging, os.path.basename(file_path))
            with open(out, "w") as fh:
                fh.write("content")

        cli = MagicMock()
        cli.restore.side_effect = _restore
        return cli

    def test_get_files_restores_all(self):
        fs = self._make_fs()
        fs.cli = self._make_restoring_cli()
        with tempfile.TemporaryDirectory() as outdir:
            rpaths = [
                "cargoship://b/p/a.txt",
                "cargoship://b/p/b.txt",
            ]
            lpaths = [
                os.path.join(outdir, "a.txt"),
                os.path.join(outdir, "b.txt"),
            ]
            fs.get_files(rpaths, lpaths)
            assert os.path.exists(lpaths[0])
            assert os.path.exists(lpaths[1])

    def test_get_files_raises_on_partial_failure(self):
        fs = self._make_fs()
        fs.cli.restore = MagicMock()  # does nothing — no files created
        with tempfile.TemporaryDirectory() as outdir:
            rpaths = ["cargoship://b/p/missing.txt"]
            lpaths = [os.path.join(outdir, "missing.txt")]
            with pytest.raises(RuntimeError, match="failed to restore"):
                fs.get_files(rpaths, lpaths)

    def test_get_files_invokes_callback(self):
        fs = self._make_fs()
        fs.cli = self._make_restoring_cli()
        progress = []
        with tempfile.TemporaryDirectory() as outdir:
            rpaths = ["cargoship://b/p/x.txt"]
            lpaths = [os.path.join(outdir, "x.txt")]
            fs.get_files(rpaths, lpaths, callback=progress.append)
        assert progress == [1]


# ---------------------------------------------------------------------------
# Budget integration (Issue #183)
# ---------------------------------------------------------------------------


class TestBudgetConfig:
    def test_project_id_default_is_empty(self):
        fs = CargoShipFileSystem(bucket="b", prefix="p")
        assert fs._project_id == ""

    def test_project_id_accepted(self):
        fs = CargoShipFileSystem(bucket="b", prefix="p", project_id="dvc_cache")
        assert fs._project_id == "dvc_cache"

    def test_enable_budget_check_default_is_false(self):
        fs = CargoShipFileSystem(bucket="b", prefix="p")
        assert fs._enable_budget_check is False

    def test_enable_budget_check_true(self):
        fs = CargoShipFileSystem(bucket="b", prefix="p", enable_budget_check=True)
        assert fs._enable_budget_check is True

    def test_enable_budget_check_string_true(self):
        fs = CargoShipFileSystem(bucket="b", prefix="p", enable_budget_check="true")
        assert fs._enable_budget_check is True

    def test_enable_budget_check_string_false(self):
        fs = CargoShipFileSystem(bucket="b", prefix="p", enable_budget_check="false")
        assert fs._enable_budget_check is False

    def test_budget_checker_none_when_disabled(self):
        fs = CargoShipFileSystem(bucket="b", prefix="p", project_id="dvc_cache", enable_budget_check=False)
        assert fs._budget_checker is None

    def test_budget_checker_none_when_no_project(self):
        fs = CargoShipFileSystem(bucket="b", prefix="p", enable_budget_check=True)
        assert fs._budget_checker is None

    def test_budget_checker_created_when_enabled(self):
        from dvc_cargoship.budget import DVCBudgetChecker
        fs = CargoShipFileSystem(bucket="b", prefix="p", project_id="dvc_cache", enable_budget_check=True)
        fs.cli = MagicMock()
        checker = fs._budget_checker
        assert isinstance(checker, DVCBudgetChecker)
        assert checker.project_id == "dvc_cache"


class TestDVCTags:
    def test_push_tags(self):
        fs = CargoShipFileSystem(bucket="b", prefix="p", project_id="dvc_cache")
        tags = fs._dvc_tags("push")
        assert tags["dvc_cache"] == "true"
        assert tags["dvc_operation"] == "push"
        assert tags["dvc_project"] == "dvc_cache"

    def test_pull_tags(self):
        fs = CargoShipFileSystem(bucket="b", prefix="p", project_id="dvc_cache")
        tags = fs._dvc_tags("pull")
        assert tags["dvc_operation"] == "pull"

    def test_no_project_tag_when_project_id_empty(self):
        fs = CargoShipFileSystem(bucket="b", prefix="p")
        tags = fs._dvc_tags("push")
        assert "dvc_project" not in tags


class TestPutFileBudgetCheck:
    def _make_fs(self, project_id="dvc_cache", enable_budget_check=True):
        fs = CargoShipFileSystem(
            bucket="b", prefix="p",
            project_id=project_id,
            enable_budget_check=enable_budget_check,
            small_file_threshold=0,  # force direct upload path
        )
        fs.cli = MagicMock()
        return fs

    def test_budget_check_not_called_when_disabled(self):
        from unittest.mock import patch
        fs = self._make_fs(enable_budget_check=False)
        with tempfile.NamedTemporaryFile(delete=False) as f:
            f.write(b"data")
            lpath = f.name
        try:
            with patch("dvc_cargoship.remote.DVCBudgetChecker") as MockChecker:
                fs.put_file(lpath, "cargoship://b/p/data.bin")
                MockChecker.assert_not_called()
        finally:
            os.unlink(lpath)

    def test_budget_check_raises_propagates(self):
        from dvc_cargoship.budget import DVCBudgetExceededError
        fs = self._make_fs()
        # inject a checker that always raises
        mock_checker = MagicMock()
        mock_checker.check_upload.side_effect = DVCBudgetExceededError(
            "dvc_cache", 100.0, 105.0
        )
        fs._CargoShipFileSystem__budget = mock_checker
        with tempfile.NamedTemporaryFile(delete=False) as f:
            f.write(b"data")
            lpath = f.name
        try:
            with pytest.raises(DVCBudgetExceededError):
                fs.put_file(lpath, "cargoship://b/p/data.bin")
        finally:
            os.unlink(lpath)

    def test_large_file_upload_includes_dvc_tags(self):
        fs = self._make_fs(enable_budget_check=False)
        with tempfile.NamedTemporaryFile(delete=False) as f:
            f.write(b"x" * 100)
            lpath = f.name
        try:
            fs.put_file(lpath, "cargoship://b/p/big.bin")
        finally:
            os.unlink(lpath)
        call_kwargs = fs.cli.upload.call_args[1]
        assert call_kwargs.get("tags", {}).get("dvc_cache") == "true"
        assert call_kwargs.get("tags", {}).get("dvc_operation") == "push"

    def test_large_file_upload_includes_project_id(self):
        fs = self._make_fs(enable_budget_check=False)
        with tempfile.NamedTemporaryFile(delete=False) as f:
            f.write(b"x" * 100)
            lpath = f.name
        try:
            fs.put_file(lpath, "cargoship://b/p/big.bin")
        finally:
            os.unlink(lpath)
        call_kwargs = fs.cli.upload.call_args[1]
        assert call_kwargs.get("project_id") == "dvc_cache"
