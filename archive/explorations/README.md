# Exploration Archive

This directory contains exploratory documents and proofs of concept that informed CargoShip's design and features.

## Contents

### Cost & Economics Analysis
- **`economic_framework_tco_analysis.md`** - Total Cost of Ownership (TCO) analysis comparing cloud storage to on-premises infrastructure
- **`compute_optimization_proof.md`** - Cost comparison framework: cloud pay-as-you-go vs on-prem unused capacity
- **`storage_optimization_proof.md`** - Storage cost optimization strategies and calculations

### Domain-Centric Storage Policies
- **`domain_specific_lifecycle_policies.md`** - Exploration of inferring storage tiering policies from data domain knowledge
- **`domain-proof.zip`** - Supporting materials for domain-centric storage research

## Key Insights

### 1. Cloud vs On-Premises Cost Model
These explorations analyzed the fundamental economics of cloud storage versus traditional on-premises infrastructure:
- **Cloud advantage**: Pay only for what you use, elastic scaling
- **On-prem challenge**: Capital expenditure for peak capacity that sits idle most of the time
- **Cost crossover points**: When cloud becomes more economical than maintaining physical infrastructure

### 2. Domain-Centric Tiering
Investigated how knowledge of data domains (e.g., "compliance records", "media assets", "logs") could automatically inform:
- Storage tier selection (hot/cold/archive)
- Retention policies
- Access patterns
- Compression strategies

This research contributed to:
- Issue #32: Intelligent storage tier selection
- Issue #164: Tier-aware chunking strategies
- Future work on automatic optimization based on workload classification

## Status

These documents represent **completed exploratory research** that informed product decisions but are not active development work. They are archived here for reference and historical context.

## Related Features

Features that emerged from these explorations:
- ✅ Automatic storage tier selection (v0.4.0)
- ✅ Budget management and cost tracking (v0.6.0)
- ⏳ Domain-aware optimization (future roadmap consideration)
