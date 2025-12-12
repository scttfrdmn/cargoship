# CargoShip Blog

Welcome to the CargoShip blog! Here we share technical deep dives, performance insights, and practical guides for optimizing large-scale S3 uploads.

---

## Introducing CargoShip (5-Part Series)

A comprehensive series exploring CargoShip's architecture, performance optimizations, and cost management features.

### [Part 1: Why We Built CargoShip - Solving the S3 Upload Bottleneck](post-1-why-we-built-cargoship.md)
**Published**: December 10, 2025 • **Reading Time**: 5 minutes

We tried to upload 2TB of genomics data to S3. It took 18 hours and crashed twice. This is the story of why we built CargoShip—and how it transforms large-scale S3 uploads from painful to effortless.

**Key Topics**: Performance problems, disk bottlenecks, cost optimization mistakes, the discoveries that led to 7× speedup and 95% cost savings.

---

### [Part 2: Zero-Disk Streaming - How CargoShip Uploads 100GB Without Staging](post-2-zero-disk-streaming.md)
**Published**: December 17, 2025 • **Reading Time**: 7 minutes

What if you could upload 100GB without writing a single byte to disk? Unix pipes solved this 50 years ago. CargoShip brings that elegance to S3 uploads with a four-stage streaming pipeline.

**Key Topics**: Streaming architecture, io.Pipe internals, Scanner → Chunker → Archiver → S3 pipeline, hands-on tutorial with real examples.

---

### [Part 3: 8x S3 Performance - The Multi-Prefix Sharding Deep Dive](post-3-multi-prefix-sharding.md)
**Published**: December 24, 2025 • **Reading Time**: 10 minutes

S3's hidden performance feature: 3,500 PUT/s per prefix. Shard across 8 prefixes, get 28,000 PUT/s capacity. Here's the complete implementation with benchmarks showing 71× speedup on real workloads.

**Key Topics**: S3 request rate limits explained, PrefixRouter implementation with Go code, distribution strategies (round-robin, least-loaded, hash-based), benchmark results, edge cases and tuning.

---

### [Part 4: Save 90% on S3 Costs with Intelligent Storage Class Selection](post-4-cost-optimization.md)
**Published**: December 31, 2025 • **Reading Time**: 9 minutes

We reduced S3 costs from $3,318/year to $147/year (95.6% savings) by choosing the right storage class. This guide covers cost estimation, lifecycle automation, budget enforcement, and ML-powered forecasting.

**Key Topics**: Storage class comparison ($0.023/GB to $0.00099/GB), v0.6.0 cost features (estimation, tracking, budget enforcement), lifecycle policies, hands-on scenarios, ROI calculations.

---

### [Part 5: Open Format, Open Source - Building on CargoShip](post-5-open-source.md)
**Published**: January 7, 2026 • **Reading Time**: 10 minutes

Your data shouldn't be locked in proprietary formats. CargoShip uses standard tar+zstd archives—extract without CargoShip using only aws-cli, zstd, and tar. Open source, open format, open future.

**Key Topics**: Open format philosophy, emergency recovery scripts (no CargoShip required), integration examples (Go library, validation, selective recovery), roadmap (v0.7.0-v1.0.0), community participation.

---

## Subscribe & Share

- **GitHub**: [Star the repo](https://github.com/scttfrdmn/cargoship) for updates
- **Discussions**: Join the conversation on [GitHub Discussions](https://github.com/scttfrdmn/cargoship/discussions)
- **Share Your Story**: Using CargoShip? Email scott@cargoship.dev

## About the Author

**Scott Friedman** is the creator of CargoShip. This project started from frustration with existing backup tools during large-scale genomics data uploads at Duke University. CargoShip is built on the foundation of [SuitcaseCTL](https://gitlab.oit.duke.edu/devil-ops/suitcasectl), Duke's original cloud-native backup system.

---

*More blog posts coming soon. Topics: Zero-copy I/O optimizations, HTTP/2 and TCP tuning, distributed tracing, Kubernetes operator, and more.*
