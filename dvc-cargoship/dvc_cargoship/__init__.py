"""dvc-cargoship: DVC remote plugin for CargoShip bulk S3 uploads.

Install this package alongside DVC to enable the ``cargoship://`` remote
scheme::

    pip install dvc-cargoship
    dvc remote add myremote cargoship://my-bucket/my-prefix

See https://github.com/scttfrdmn/cargoship for CargoShip documentation.
"""

from ._version import __version__
from .cli import CargoShipCLI, CargoShipCLIError
from .perf import BatchUploadBuffer, parallel_restore, parse_size
from .remote import CargoShipFileSystem

__all__ = [
    "CargoShipFileSystem",
    "CargoShipCLI",
    "CargoShipCLIError",
    "BatchUploadBuffer",
    "parallel_restore",
    "parse_size",
    "__version__",
]
