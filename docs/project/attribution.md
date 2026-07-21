# Attribution & license

## License

CargoShip is released under the [Apache 2.0 License](https://github.com/scttfrdmn/cargoship/blob/main/LICENSE).

## Built on SuitcaseCTL

CargoShip is built on the foundation of **SuitcaseCTL**, developed by Duke
University's DevOps team and released under the MIT License. We're grateful for
their work in research data archiving, which made this project possible.

- **Original project:** SuitcaseCTL
- **Author:** Duke University DevOps Team
- **License:** MIT
- **Repository:** [gitlab.oit.duke.edu/devil-ops/suitcasectl](https://gitlab.oit.duke.edu/devil-ops/suitcasectl)

## Concepts inherited from SuitcaseCTL

Several architectural patterns carried over from SuitcaseCTL and were evolved for
AWS-native operation:

- **Porter pattern** — central orchestration of archiving with a functional-options
  API.
- **Inventory system** — file discovery and metadata collection, extended with cost
  estimation and AWS metadata.
- **Suitcase metaphor** — breaking large datasets into manageable compressed chunks,
  now tuned for S3 object sizing.
- **Pluggable transport** — a modular transport layer, evolved into native AWS SDK
  integration.
- **Travel-agent pattern** — cloud-based orchestration and monitoring.
- **Hierarchical configuration** — Viper-based config with environment-variable
  support.

## Acknowledgments

CargoShip stands on the work of the Duke University DevOps team, the Go community,
AWS and its SDKs, and the many open-source libraries the project depends on. We aim
to keep contributing back in the same open-source spirit.

## See also

- [Architecture](/project/architecture).
- [API stability & versioning](/project/versioning).
