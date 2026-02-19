"""Budget checking for DVC operations.

Before each DVC push/pull the :class:`DVCBudgetChecker` queries the
CargoShip cost manager via ``cargoship budget status <project_id> --json``
and raises :class:`DVCBudgetExceededError` when the project has already
exceeded its budget or when the pending upload would tip it over.

Usage::

    from dvc_cargoship.budget import DVCBudgetChecker

    checker = DVCBudgetChecker(cli, project_id="dvc_cache")
    checker.check_upload(size_bytes=50_000_000, operation="push")
"""

from __future__ import annotations

from typing import TYPE_CHECKING, Any, Dict, Optional

if TYPE_CHECKING:
    from .cli import CargoShipCLI


class DVCBudgetExceededError(Exception):
    """Raised when a DVC operation would exceed the configured budget.

    Attributes
    ----------
    project_id:
        The project whose budget was exceeded.
    max_budget:
        The configured maximum budget in USD.
    current_spend:
        Amount already spent this period in USD.
    estimated_cost:
        Estimated cost of the pending operation in USD (0 when unknown).
    operation:
        Human-readable operation name (e.g. ``"push"`` or ``"pull"``).
    """

    def __init__(
        self,
        project_id: str,
        max_budget: float,
        current_spend: float,
        estimated_cost: float = 0.0,
        operation: str = "push",
    ) -> None:
        self.project_id = project_id
        self.max_budget = max_budget
        self.current_spend = current_spend
        self.estimated_cost = estimated_cost
        self.operation = operation

        remaining = max(0.0, max_budget - current_spend)
        msg = (
            f"DVC budget exceeded for project {project_id!r} "
            f"(operation: {operation}): "
            f"max=${max_budget:.2f}, spent=${current_spend:.2f}, "
            f"remaining=${remaining:.2f}"
        )
        if estimated_cost:
            msg += f", estimated=${estimated_cost:.4f}"
        super().__init__(msg)


class DVCBudgetChecker:
    """Pre-flight budget checker for DVC operations.

    Wraps ``cargoship budget status <project_id> --json`` and
    ``cargoship cost estimate --size <bytes> --json`` to enforce cost
    limits before DVC uploads proceed.

    The checker is intentionally lenient about failures: when the
    ``cargoship`` binary is unavailable, AWS credentials are missing, or
    no budget has been configured for the project it does **nothing**
    rather than blocking the upload.  Configure an explicit budget via::

        cargoship budget set dvc_cache --cost 100 --volume 500

    Parameters
    ----------
    cli:
        :class:`~dvc_cargoship.cli.CargoShipCLI` instance.
    project_id:
        CargoShip project ID used for DVC cost tracking.
        Defaults to ``"dvc_cache"``, matching the DVC remote's default.
    """

    def __init__(
        self,
        cli: "CargoShipCLI",
        project_id: str = "dvc_cache",
    ) -> None:
        self._cli = cli
        self.project_id = project_id

    # ------------------------------------------------------------------
    # Public API
    # ------------------------------------------------------------------

    def get_status(self) -> Dict[str, Any]:
        """Return the budget status dict for this project.

        Returns an empty dict when no budget is configured or when the
        ``cargoship`` binary is unavailable.
        """
        return self._cli.budget_status(self.project_id)

    def is_over_budget(self) -> bool:
        """Return *True* when the project has already exceeded its budget."""
        status = self.get_status()
        return bool(status.get("over_budget") or status.get("over_volume"))

    def check_upload(
        self,
        size_bytes: int = 0,
        operation: str = "push",
    ) -> None:
        """Raise :class:`DVCBudgetExceededError` if the upload is not allowed.

        Checks two conditions in order:

        1. Is the project *currently* over budget?  If so, raise immediately.
        2. If *size_bytes* > 0, estimate the cost via
           ``cargoship cost estimate`` and check whether
           ``current_spend + estimated_cost > max_budget``.

        When no budget is configured for the project (``get_status()``
        returns an empty dict or ``max_budget == 0``) the method does
        nothing.

        Parameters
        ----------
        size_bytes:
            Estimated size of the pending upload in bytes.  When zero the
            cost estimate step is skipped.
        operation:
            Human-readable operation name included in any raised error
            (e.g. ``"push"`` or ``"pull"``).

        Raises
        ------
        DVCBudgetExceededError
            When the project is over budget or the pending upload would
            exceed the configured maximum.
        """
        status = self.get_status()
        if not status:
            # No budget configured — nothing to enforce.
            return

        max_budget: float = float(status.get("max_budget") or 0)
        current_spend: float = float(status.get("current_spend") or 0)

        # Already over budget?
        if status.get("over_budget"):
            raise DVCBudgetExceededError(
                project_id=self.project_id,
                max_budget=max_budget,
                current_spend=current_spend,
                operation=operation,
            )

        # Would the pending upload tip us over?
        if max_budget > 0 and size_bytes > 0:
            estimate = self._cli.cost_estimate(size_bytes)
            estimated_cost: float = float(estimate.get("total_cost") or 0)
            if estimated_cost and current_spend + estimated_cost > max_budget:
                raise DVCBudgetExceededError(
                    project_id=self.project_id,
                    max_budget=max_budget,
                    current_spend=current_spend,
                    estimated_cost=estimated_cost,
                    operation=operation,
                )
