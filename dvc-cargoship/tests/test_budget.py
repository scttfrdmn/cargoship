"""Tests for dvc_cargoship.budget — DVCBudgetExceededError, DVCBudgetChecker."""

from __future__ import annotations

from unittest.mock import MagicMock

import pytest

from dvc_cargoship.budget import DVCBudgetChecker, DVCBudgetExceededError


# ---------------------------------------------------------------------------
# DVCBudgetExceededError
# ---------------------------------------------------------------------------


class TestDVCBudgetExceededError:
    def test_message_includes_project_id(self):
        err = DVCBudgetExceededError("my-proj", 100.0, 95.0)
        assert "my-proj" in str(err)

    def test_message_includes_max_and_current(self):
        err = DVCBudgetExceededError("p", 100.0, 95.0)
        assert "100.00" in str(err)
        assert "95.00" in str(err)

    def test_estimated_cost_included_when_nonzero(self):
        err = DVCBudgetExceededError("p", 100.0, 90.0, estimated_cost=0.05)
        assert "0.0500" in str(err)

    def test_estimated_cost_omitted_when_zero(self):
        err = DVCBudgetExceededError("p", 100.0, 90.0, estimated_cost=0.0)
        assert "estimated" not in str(err)

    def test_attributes_set(self):
        err = DVCBudgetExceededError("myp", 50.0, 40.0, 5.0, "push")
        assert err.project_id == "myp"
        assert err.max_budget == 50.0
        assert err.current_spend == 40.0
        assert err.estimated_cost == 5.0
        assert err.operation == "push"

    def test_is_exception(self):
        assert issubclass(DVCBudgetExceededError, Exception)


# ---------------------------------------------------------------------------
# DVCBudgetChecker — helpers
# ---------------------------------------------------------------------------


def _make_cli(status: dict | None = None, estimate: dict | None = None) -> MagicMock:
    """Return a mock CLI with configurable budget_status and cost_estimate."""
    cli = MagicMock()
    cli.budget_status.return_value = status or {}
    cli.cost_estimate.return_value = estimate or {}
    return cli


# ---------------------------------------------------------------------------
# get_status
# ---------------------------------------------------------------------------


class TestGetStatus:
    def test_delegates_to_cli_budget_status(self):
        cli = _make_cli({"max_budget": 100.0})
        checker = DVCBudgetChecker(cli, project_id="dvc_cache")
        result = checker.get_status()
        cli.budget_status.assert_called_once_with("dvc_cache")
        assert result == {"max_budget": 100.0}

    def test_returns_empty_when_cli_returns_empty(self):
        cli = _make_cli({})
        checker = DVCBudgetChecker(cli)
        assert checker.get_status() == {}


# ---------------------------------------------------------------------------
# is_over_budget
# ---------------------------------------------------------------------------


class TestIsOverBudget:
    def test_true_when_over_budget_flag_set(self):
        cli = _make_cli({"over_budget": True})
        assert DVCBudgetChecker(cli).is_over_budget() is True

    def test_true_when_over_volume_flag_set(self):
        cli = _make_cli({"over_volume": True})
        assert DVCBudgetChecker(cli).is_over_budget() is True

    def test_false_when_neither_flag_set(self):
        cli = _make_cli({"max_budget": 100.0, "current_spend": 50.0})
        assert DVCBudgetChecker(cli).is_over_budget() is False

    def test_false_when_status_empty(self):
        cli = _make_cli({})
        assert DVCBudgetChecker(cli).is_over_budget() is False


# ---------------------------------------------------------------------------
# check_upload — passes (no raise)
# ---------------------------------------------------------------------------


class TestCheckUploadPasses:
    def test_no_raise_when_no_budget_configured(self):
        cli = _make_cli({})
        DVCBudgetChecker(cli).check_upload(size_bytes=1_000_000)
        # Should not raise

    def test_no_raise_when_max_budget_zero(self):
        cli = _make_cli({"max_budget": 0, "current_spend": 0})
        DVCBudgetChecker(cli).check_upload(size_bytes=1_000_000)

    def test_no_raise_when_under_budget_and_small_upload(self):
        cli = _make_cli(
            {"max_budget": 100.0, "current_spend": 50.0},
            {"total_cost": 0.01},
        )
        DVCBudgetChecker(cli).check_upload(size_bytes=1_000_000)

    def test_no_raise_when_size_bytes_zero_skips_estimate(self):
        # size_bytes=0 means we don't call cost_estimate at all
        cli = _make_cli({"max_budget": 100.0, "current_spend": 99.99})
        DVCBudgetChecker(cli).check_upload(size_bytes=0)
        cli.cost_estimate.assert_not_called()

    def test_no_raise_when_estimate_is_zero(self):
        # cost_estimate returns empty dict — treat as zero cost
        cli = _make_cli(
            {"max_budget": 100.0, "current_spend": 99.99},
            {},
        )
        DVCBudgetChecker(cli).check_upload(size_bytes=5_000_000)

    def test_no_raise_when_estimate_would_not_exceed(self):
        cli = _make_cli(
            {"max_budget": 100.0, "current_spend": 50.0},
            {"total_cost": 10.0},  # 50 + 10 = 60 < 100
        )
        DVCBudgetChecker(cli).check_upload(size_bytes=10_000_000)


# ---------------------------------------------------------------------------
# check_upload — raises
# ---------------------------------------------------------------------------


class TestCheckUploadRaises:
    def test_raises_when_over_budget(self):
        cli = _make_cli({"over_budget": True, "max_budget": 100.0, "current_spend": 105.0})
        with pytest.raises(DVCBudgetExceededError) as exc_info:
            DVCBudgetChecker(cli).check_upload(size_bytes=0, operation="push")
        assert exc_info.value.project_id == "dvc_cache"
        assert exc_info.value.operation == "push"

    def test_raises_when_estimated_cost_would_exceed(self):
        cli = _make_cli(
            {"max_budget": 100.0, "current_spend": 95.0},
            {"total_cost": 10.0},  # 95 + 10 = 105 > 100
        )
        with pytest.raises(DVCBudgetExceededError) as exc_info:
            DVCBudgetChecker(cli).check_upload(size_bytes=5_000_000)
        err = exc_info.value
        assert err.estimated_cost == 10.0
        assert err.current_spend == 95.0

    def test_raises_with_custom_operation(self):
        cli = _make_cli({"over_budget": True, "max_budget": 50.0, "current_spend": 55.0})
        with pytest.raises(DVCBudgetExceededError) as exc_info:
            DVCBudgetChecker(cli, project_id="my_dvc").check_upload(operation="pull")
        assert exc_info.value.operation == "pull"
        assert exc_info.value.project_id == "my_dvc"

    def test_error_message_has_remaining(self):
        cli = _make_cli({"over_budget": True, "max_budget": 100.0, "current_spend": 110.0})
        with pytest.raises(DVCBudgetExceededError) as exc_info:
            DVCBudgetChecker(cli).check_upload()
        # remaining = max(0, 100 - 110) = 0
        assert "remaining=$0.00" in str(exc_info.value)


# ---------------------------------------------------------------------------
# Interaction with CargoShipCLI methods
# ---------------------------------------------------------------------------


class TestCLIInteraction:
    def test_cost_estimate_called_with_correct_size(self):
        cli = _make_cli(
            {"max_budget": 100.0, "current_spend": 50.0},
            {},
        )
        DVCBudgetChecker(cli).check_upload(size_bytes=12_345_678)
        cli.cost_estimate.assert_called_once_with(12_345_678)

    def test_budget_status_uses_project_id(self):
        cli = _make_cli({})
        DVCBudgetChecker(cli, project_id="research_data").check_upload()
        cli.budget_status.assert_called_once_with("research_data")

    def test_cost_estimate_not_called_when_no_budget(self):
        cli = _make_cli({})
        DVCBudgetChecker(cli).check_upload(size_bytes=1_000_000)
        cli.cost_estimate.assert_not_called()
