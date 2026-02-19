"""setup.py — kept for editable installs (pip install -e .).

All metadata is declared in pyproject.toml.
"""
from setuptools import setup

setup(
    entry_points={
        # DVC discovers filesystem plugins under the `dvc.fs` group.
        # The key is the URL scheme; the value is the fully-qualified class.
        # After installation, `dvc remote add myremote cargoship://bucket/prefix`
        # will route to CargoShipFileSystem.
        #
        # Note: DVC config-schema validation (iterative/dvc#9711) may require
        # a patched DVC build until the dynamic-schema mechanism is upstreamed.
        "dvc.fs": [
            "cargoship = dvc_cargoship:CargoShipFileSystem",
        ],
    },
)
