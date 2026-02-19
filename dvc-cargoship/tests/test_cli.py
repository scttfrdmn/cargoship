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


# ---------------------------------------------------------------------------
# CargoShipCLI.upload() — project_id and tags (Issue #183)
# ---------------------------------------------------------------------------


class TestUploadProjectAndTags:
    def _cli(self):
        cli = CargoShipCLI(binary="cargoship")
        cli.run = MagicMock(return_value=MagicMock(returncode=0))
        return cli

    def test_project_id_flag(self):
        cli = self._cli()
        cli.upload("/src", "s3://b/p", project_id="dvc_cache")
        args = cli.run.call_args[0]
        assert "--project" in args
        assert "dvc_cache" in args

    def test_no_project_flag_when_omitted(self):
        cli = self._cli()
        cli.upload("/src", "s3://b/p")
        args = cli.run.call_args[0]
        assert "--project" not in args

    def test_single_tag(self):
        cli = self._cli()
        cli.upload("/src", "s3://b/p", tags={"dvc_cache": "true"})
        args = cli.run.call_args[0]
        assert "--tag" in args
        assert "dvc_cache=true" in args

    def test_multiple_tags(self):
        cli = self._cli()
        cli.upload("/src", "s3://b/p", tags={"dvc_cache": "true", "dvc_operation": "push"})
        args = cli.run.call_args[0]
        assert args.count("--tag") == 2
        assert "dvc_cache=true" in args
        assert "dvc_operation=push" in args

    def test_no_tag_flags_when_omitted(self):
        cli = self._cli()
        cli.upload("/src", "s3://b/p")
        args = cli.run.call_args[0]
        assert "--tag" not in args


# ---------------------------------------------------------------------------
# CargoShipCLI.budget_status() (Issue #183)
# ---------------------------------------------------------------------------


class TestBudgetStatus:
    def _cli(self, stdout="{}"):
        cli = CargoShipCLI(binary="cargoship")
        cli.run = MagicMock(return_value=MagicMock(returncode=0, stdout=stdout))
        return cli

    def test_returns_empty_when_project_id_is_none(self):
        cli = self._cli()
        assert cli.budget_status(None) == {}
        cli.run.assert_not_called()

    def test_calls_budget_status_with_json_flag(self):
        cli = self._cli('{"max_budget": 100}')
        cli.budget_status("dvc_cache")
        args = cli.run.call_args[0]
        assert args == ("budget", "status", "dvc_cache", "--json")

    def test_returns_parsed_dict(self):
        cli = self._cli('{"max_budget": 100.0, "current_spend": 42.5}')
        result = cli.budget_status("proj")
        assert result == {"max_budget": 100.0, "current_spend": 42.5}

    def test_returns_empty_on_cli_error(self):
        cli = CargoShipCLI(binary="cargoship")
        cli.run = MagicMock(side_effect=CargoShipCLIError("no budget"))
        assert cli.budget_status("proj") == {}

    def test_returns_empty_on_invalid_json(self):
        cli = self._cli("not-json")
        assert cli.budget_status("proj") == {}

    def test_returns_empty_when_json_is_not_dict(self):
        cli = self._cli("[1, 2, 3]")
        assert cli.budget_status("proj") == {}


# ---------------------------------------------------------------------------
# CargoShipCLI.cost_estimate() (Issue #183)
# ---------------------------------------------------------------------------


class TestCostEstimate:
    def _cli(self, stdout="{}"):
        cli = CargoShipCLI(binary="cargoship")
        cli.run = MagicMock(return_value=MagicMock(returncode=0, stdout=stdout))
        return cli

    def test_passes_size_as_bytes_with_B_suffix(self):
        cli = self._cli('{"total_cost": 0.01}')
        cli.cost_estimate(10_485_760)
        args = cli.run.call_args[0]
        assert "--size" in args
        size_idx = args.index("--size")
        assert args[size_idx + 1] == "10485760B"

    def test_passes_storage_class(self):
        cli = self._cli("{}")
        cli.cost_estimate(1024, storage_class="GLACIER")
        args = cli.run.call_args[0]
        assert "--storage-class" in args
        sc_idx = args.index("--storage-class")
        assert args[sc_idx + 1] == "GLACIER"

    def test_passes_region(self):
        cli = self._cli("{}")
        cli.cost_estimate(1024, region="eu-west-1")
        args = cli.run.call_args[0]
        assert "--region" in args
        r_idx = args.index("--region")
        assert args[r_idx + 1] == "eu-west-1"

    def test_passes_json_flag(self):
        cli = self._cli("{}")
        cli.cost_estimate(1024)
        args = cli.run.call_args[0]
        assert "--json" in args

    def test_returns_parsed_dict(self):
        cli = self._cli('{"total_cost": 0.0042, "currency": "USD"}')
        result = cli.cost_estimate(1024)
        assert result == {"total_cost": 0.0042, "currency": "USD"}

    def test_returns_empty_on_cli_error(self):
        cli = CargoShipCLI(binary="cargoship")
        cli.run = MagicMock(side_effect=CargoShipCLIError("no pricing"))
        assert cli.cost_estimate(1024) == {}

    def test_returns_empty_on_invalid_json(self):
        cli = self._cli("bad json")
        assert cli.cost_estimate(1024) == {}

    def test_default_storage_class_is_standard(self):
        cli = self._cli("{}")
        cli.cost_estimate(1024)
        args = cli.run.call_args[0]
        sc_idx = args.index("--storage-class")
        assert args[sc_idx + 1] == "STANDARD"
