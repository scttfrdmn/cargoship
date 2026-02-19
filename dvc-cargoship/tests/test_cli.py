"""Tests for CargoShipCLI subprocess wrapper."""

from __future__ import annotations

import json
import subprocess
from unittest.mock import MagicMock, patch

import pytest

from dvc_cargoship.cli import CargoShipCLI, CargoShipCLIError


# ---------------------------------------------------------------------------
# Helpers
# ---------------------------------------------------------------------------


def _make_completed(returncode: int = 0, stdout: str = "", stderr: str = "") -> subprocess.CompletedProcess:
    result = MagicMock(spec=subprocess.CompletedProcess)
    result.returncode = returncode
    result.stdout = stdout
    result.stderr = stderr
    return result


# ---------------------------------------------------------------------------
# CargoShipCLI.find()
# ---------------------------------------------------------------------------


class TestFind:
    def test_find_returns_cli_when_binary_on_path(self):
        with patch("dvc_cargoship.cli.shutil.which", return_value="/usr/local/bin/cargoship"):
            cli = CargoShipCLI.find()
        assert cli.binary == "/usr/local/bin/cargoship"

    def test_find_raises_when_binary_missing(self):
        with patch("dvc_cargoship.cli.shutil.which", return_value=None):
            with pytest.raises(FileNotFoundError, match="cargoship binary not found"):
                CargoShipCLI.find()

    def test_find_custom_binary_name(self):
        with patch("dvc_cargoship.cli.shutil.which", return_value="/opt/bin/cs") as mock_which:
            cli = CargoShipCLI.find(binary="cs")
        mock_which.assert_called_once_with("cs")
        assert cli.binary == "/opt/bin/cs"


# ---------------------------------------------------------------------------
# CargoShipCLI.run()
# ---------------------------------------------------------------------------


class TestRun:
    def test_run_success(self):
        cli = CargoShipCLI(binary="cargoship")
        completed = _make_completed(returncode=0, stdout="ok")
        with patch("dvc_cargoship.cli.subprocess.run", return_value=completed) as mock_run:
            result = cli.run("upload", "src", "s3://b/p")
        mock_run.assert_called_once()
        cmd_arg = mock_run.call_args[0][0]
        assert cmd_arg == ["cargoship", "upload", "src", "s3://b/p"]
        assert result is completed

    def test_run_raises_on_nonzero_exit(self):
        cli = CargoShipCLI(binary="cargoship")
        completed = _make_completed(returncode=1, stderr="boom")
        with patch("dvc_cargoship.cli.subprocess.run", return_value=completed):
            with pytest.raises(CargoShipCLIError, match="exit 1"):
                cli.run("upload", "src", "s3://b/p")

    def test_run_check_false_no_raise(self):
        cli = CargoShipCLI(binary="cargoship")
        completed = _make_completed(returncode=1)
        with patch("dvc_cargoship.cli.subprocess.run", return_value=completed):
            result = cli.run("list", "s3://b/p", check=False)
        assert result.returncode == 1

    def test_run_merges_env(self):
        cli = CargoShipCLI(binary="cargoship")
        completed = _make_completed()
        with patch("dvc_cargoship.cli.subprocess.run", return_value=completed) as mock_run:
            with patch("dvc_cargoship.cli.os.environ", {"PATH": "/usr/bin"}):
                cli.run("list", "s3://b/p", env={"MY_VAR": "1"})
        call_kwargs = mock_run.call_args[1]
        assert call_kwargs["env"]["MY_VAR"] == "1"
        assert call_kwargs["env"]["PATH"] == "/usr/bin"

    def test_run_pipes_stdout_and_stderr(self):
        cli = CargoShipCLI(binary="cargoship")
        completed = _make_completed()
        with patch("dvc_cargoship.cli.subprocess.run", return_value=completed) as mock_run:
            cli.run("list", "s3://b/p")
        call_kwargs = mock_run.call_args[1]
        assert call_kwargs["stdout"] == subprocess.PIPE
        assert call_kwargs["stderr"] == subprocess.PIPE
        assert call_kwargs["text"] is True


# ---------------------------------------------------------------------------
# CargoShipCLI.upload()
# ---------------------------------------------------------------------------


class TestUpload:
    def _cli(self):
        cli = CargoShipCLI(binary="cargoship")
        cli.run = MagicMock(return_value=_make_completed())
        return cli

    def test_basic_upload(self):
        cli = self._cli()
        cli.upload("/data", "s3://b/p")
        cli.run.assert_called_once_with("upload", "/data", "s3://b/p", "--quiet")

    def test_incremental_flag(self):
        cli = self._cli()
        cli.upload("/data", "s3://b/p", incremental=True)
        args = cli.run.call_args[0]
        assert "--incremental" in args

    def test_prev_manifest_flag(self):
        cli = self._cli()
        cli.upload("/data", "s3://b/p", prev_manifest="/tmp/mf.json")
        args = cli.run.call_args[0]
        assert "--prev-manifest" in args
        assert "/tmp/mf.json" in args

    def test_generate_dvc_files_flag(self):
        cli = self._cli()
        cli.upload("/data", "s3://b/p", generate_dvc_files=True)
        args = cli.run.call_args[0]
        assert "--generate-dvc-files" in args

    def test_quiet_false(self):
        cli = self._cli()
        cli.upload("/data", "s3://b/p", quiet=False)
        args = cli.run.call_args[0]
        assert "--quiet" not in args


# ---------------------------------------------------------------------------
# CargoShipCLI.list_files()
# ---------------------------------------------------------------------------


class TestListFiles:
    def _cli(self, stdout: str = "[]", returncode: int = 0):
        cli = CargoShipCLI(binary="cargoship")
        cli.run = MagicMock(return_value=_make_completed(returncode=returncode, stdout=stdout))
        return cli

    def test_returns_parsed_list(self):
        entries = [{"path": "a.txt", "size": 100, "content_hash": "abc"}]
        cli = self._cli(stdout=json.dumps(entries))
        result = cli.list_files("s3://b/p")
        assert result == entries

    def test_returns_empty_list_on_cli_error(self):
        cli = CargoShipCLI(binary="cargoship")
        cli.run = MagicMock(side_effect=CargoShipCLIError("fail"))
        assert cli.list_files("s3://b/p") == []

    def test_returns_empty_list_on_invalid_json(self):
        cli = self._cli(stdout="not-json")
        assert cli.list_files("s3://b/p") == []

    def test_returns_empty_list_when_json_not_list(self):
        cli = self._cli(stdout='{"key": "value"}')
        assert cli.list_files("s3://b/p") == []

    def test_includes_upload_id(self):
        cli = self._cli()
        cli.list_files("s3://b/p", upload_id="u-123")
        args = cli.run.call_args[0]
        assert "--upload-id" in args
        assert "u-123" in args

    def test_basic_args(self):
        cli = self._cli()
        cli.list_files("s3://b/p")
        args = cli.run.call_args[0]
        assert args == ("list", "s3://b/p", "--json")


# ---------------------------------------------------------------------------
# CargoShipCLI.info()
# ---------------------------------------------------------------------------


class TestInfo:
    def _cli(self, stdout: str = "{}", returncode: int = 0):
        cli = CargoShipCLI(binary="cargoship")
        cli.run = MagicMock(return_value=_make_completed(returncode=returncode, stdout=stdout))
        return cli

    def test_returns_parsed_dict(self):
        data = {"upload_id": "u-1", "file_count": 5}
        cli = self._cli(stdout=json.dumps(data))
        result = cli.info("s3://b/p")
        assert result == data

    def test_returns_empty_dict_on_cli_error(self):
        cli = CargoShipCLI(binary="cargoship")
        cli.run = MagicMock(side_effect=CargoShipCLIError("fail"))
        assert cli.info("s3://b/p") == {}

    def test_returns_empty_dict_on_invalid_json(self):
        cli = self._cli(stdout="bad-json")
        assert cli.info("s3://b/p") == {}

    def test_includes_upload_id(self):
        cli = self._cli()
        cli.info("s3://b/p", upload_id="u-42")
        args = cli.run.call_args[0]
        assert "--upload-id" in args
        assert "u-42" in args


# ---------------------------------------------------------------------------
# CargoShipCLI.restore()
# ---------------------------------------------------------------------------


class TestRestore:
    def _cli(self):
        cli = CargoShipCLI(binary="cargoship")
        cli.run = MagicMock(return_value=_make_completed())
        return cli

    def test_basic_restore(self):
        cli = self._cli()
        cli.restore("s3://b/p", "/tmp/out")
        args = cli.run.call_args[0]
        assert args == ("restore", "s3://b/p", "/tmp/out")

    def test_file_path_flag(self):
        cli = self._cli()
        cli.restore("s3://b/p", "/tmp/out", file_path="data/a.txt")
        args = cli.run.call_args[0]
        assert "--file" in args
        assert "data/a.txt" in args

    def test_content_hash_flag(self):
        cli = self._cli()
        cli.restore("s3://b/p", "/tmp/out", content_hash="deadbeef")
        args = cli.run.call_args[0]
        assert "--hash" in args
        assert "deadbeef" in args

    def test_upload_id_flag(self):
        cli = self._cli()
        cli.restore("s3://b/p", "/tmp/out", upload_id="u-99")
        args = cli.run.call_args[0]
        assert "--upload-id" in args
        assert "u-99" in args

    def test_raises_on_failure(self):
        cli = CargoShipCLI(binary="cargoship")
        cli.run = MagicMock(side_effect=CargoShipCLIError("not found"))
        with pytest.raises(CargoShipCLIError):
            cli.restore("s3://b/p", "/tmp/out", file_path="missing.txt")
