# Documentation Organization

<div align="center">
  <img src="../assets/images/logo.png" alt="CargoShip Logo" width="120" height="120">
</div>

This document describes the current documentation organization for CargoShip and the rationale behind the structure.

## Overview

CargoShip's documentation has been reorganized from 25+ scattered root-level files into a logical, hierarchical structure that improves discoverability and maintenance.

## Documentation Structure

```
cargoship/
├── README.md                 # Main project overview
├── CONTRIBUTING.md           # Contribution guidelines  
├── CHANGELOG.md              # Version history
├── ROADMAP.md               # Future plans
├── SECURITY.md              # Security policy
└── docs/                    # All documentation
    ├── index.md             # Documentation home page
    ├── install.md           # Installation guide
    ├── wizard.md            # Quick start wizard
    ├── USER_GUIDE.md        # Complete user guide
    ├── ARCHITECTURE.md      # System architecture
    ├── cost-management.md   # Cost optimization
    ├── ATTRIBUTION.md       # Project attribution
    ├── BINARY_RELEASE_SUMMARY.md
    │
    ├── advanced/            # Advanced features
    │   ├── autocomplete.md
    │   ├── benchmarks.md
    │   ├── defaults_overrides.md
    │   ├── gpg_encryption.md
    │   ├── inventory_schema.md
    │   └── travelagent.md
    │
    ├── components/          # Core components
    │   ├── cli_metadata.md
    │   ├── cli_output.md
    │   ├── hashes.md
    │   ├── inventory.md
    │   ├── inventory_metadata.md
    │   └── suitcase.md
    │
    ├── plugins/             # Plugin system
    │   └── transport/
    │       ├── cloud.md
    │       └── shell.md
    │
    ├── development/         # Development docs
    │   ├── README.md
    │   ├── DEVELOPMENT_RULES.md
    │   ├── DOCKER_ENVIRONMENT_STATUS.md
    │   ├── INTEGRATION_TEST_RESULTS.md
    │   ├── MODULARIZATION_PLAN.md
    │   ├── PROJECT_STATUS.md
    │   ├── REAL_AWS_TESTING_PLAN.md
    │   ├── RELEASE_PROCESS.md
    │   ├── REMAINING_TEST_ISSUES.md
    │   ├── TEST_ANALYSIS_REPORT.md
    │   └── VERSION_TRACKING.md
    │
    ├── deployment/          # Deployment guides
    │   ├── README.md
    │   ├── GHOST_SHIP_DEBUG_RESOLUTION.md
    │   ├── GHOST_SHIP_DEPLOYMENT_GUIDE.md
    │   └── README_LAUNCH.md
    │
    ├── architecture/        # Technical architecture
    │   ├── README.md
    │   ├── CARGOSHIP_PROJECT_PLAN.md
    │   ├── CARGOSHIP_TRANSPORTER_ENHANCEMENTS.md
    │   ├── MULTIREGION_S3_OPTIMIZATION_ENHANCEMENT.md
    │   ├── S3_OPTIMIZATION_INTEGRATION_COMPLETE.md
    │   ├── S3_OPTIMIZATION_MODULARIZATION_COMPLETE.md
    │   └── TRANSFER_ARCHITECTURE.md
    │
    ├── testing/             # Test documentation
    │   └── tui-coverage-progress.md
    │
    ├── assets/              # Images and media
    │   └── cargoship-logo.svg
    │
    └── vhs/                 # Demo recordings
        ├── demo.gif
        ├── demo.tape
        ├── travel-agent.gif
        ├── travel-agent.tape
        ├── wizard.gif
        └── wizard.tape
```

## Categorization Logic

### Root Level (Essential Files Only)
Files that users need immediate access to:
- `README.md` - Project overview and quick start
- `CONTRIBUTING.md` - How to contribute
- `CHANGELOG.md` - Version history
- `ROADMAP.md` - Future plans
- `SECURITY.md` - Security information

### docs/ - User Documentation
Primary documentation for end users:
- Getting started guides
- Feature documentation
- Configuration guides
- AWS integration

### docs/development/ - Internal Development
Documentation for contributors and maintainers:
- Development rules and standards
- Test results and analysis
- Project management docs
- Release processes

### docs/deployment/ - Operations
Documentation for deployment and operations:
- Production deployment guides
- Distributed agent setup
- Troubleshooting guides

### docs/architecture/ - Technical Design
Deep technical documentation:
- Architecture decisions
- Design documents
- Implementation details
- Enhancement specifications

## Benefits of This Organization

### 1. Improved Discoverability
- Clear hierarchy makes finding relevant docs easier
- Related documents are grouped together
- README files in each directory explain contents

### 2. Better Maintenance
- Similar documents are co-located
- Easier to keep related docs in sync
- Clear ownership of different doc types

### 3. Cleaner Root Directory
- Only essential files remain at root level
- Reduced visual clutter
- Easier project navigation

### 4. Enhanced Navigation
- MkDocs site has comprehensive navigation
- Logical grouping in documentation site
- Cross-references work correctly

### 5. Role-Based Access
- Users find user docs quickly
- Developers find dev docs efficiently  
- Operators find deployment docs easily

## Migration Summary

### Files Moved to docs/development/
- DEVELOPMENT_RULES.md
- DOCKER_ENVIRONMENT_STATUS.md
- INTEGRATION_TEST_RESULTS.md
- MODULARIZATION_PLAN.md
- PROJECT_STATUS.md
- REAL_AWS_TESTING_PLAN.md
- RELEASE_PROCESS.md
- REMAINING_TEST_ISSUES.md
- TEST_ANALYSIS_REPORT.md
- VERSION_TRACKING.md

### Files Moved to docs/deployment/
- GHOST_SHIP_DEBUG_RESOLUTION.md
- GHOST_SHIP_DEPLOYMENT_GUIDE.md
- README_LAUNCH.md

### Files Moved to docs/architecture/
- CARGOSHIP_PROJECT_PLAN.md
- CARGOSHIP_TRANSPORTER_ENHANCEMENTS.md
- MULTIREGION_S3_OPTIMIZATION_ENHANCEMENT.md
- S3_OPTIMIZATION_INTEGRATION_COMPLETE.md
- S3_OPTIMIZATION_MODULARIZATION_COMPLETE.md
- TRANSFER_ARCHITECTURE.md

### Files Moved to docs/
- ATTRIBUTION.md
- BINARY_RELEASE_SUMMARY.md

## Updated References

### MkDocs Navigation
The `mkdocs.yml` file has been completely updated to include:
- All reorganized documentation
- Logical navigation hierarchy
- Proper cross-references
- Role-based grouping

### README.md Updates
The main README has been updated with:
- Better organized documentation links
- Correct paths to moved files
- Enhanced documentation section structure

## Future Maintenance

### Adding New Documentation
- **User guides** → `docs/`
- **Development docs** → `docs/development/`
- **Deployment guides** → `docs/deployment/`
- **Architecture docs** → `docs/architecture/`
- **Test reports** → `docs/testing/`

### Keeping Navigation Updated
- Update `mkdocs.yml` when adding new docs
- Update README section for user-facing docs
- Add entries to directory README files
- Ensure cross-references are updated

### Documentation Standards
- Use clear, descriptive filenames
- Include brief description in directory READMEs
- Maintain consistent formatting
- Keep cross-references up to date

## Tools and Automation

### MkDocs Site Generation
```bash
mkdocs serve  # Local development
mkdocs build  # Production build
```

### Documentation Validation
- Links are validated in CI
- MkDocs build must pass
- Cross-references checked automatically

## Conclusion

This reorganization significantly improves CargoShip's documentation experience by:

1. **Reducing complexity** - Clean root directory with only essential files
2. **Improving navigation** - Logical hierarchy and comprehensive site navigation
3. **Enhancing discoverability** - Role-based organization helps users find what they need
4. **Simplifying maintenance** - Related docs grouped together, easier to keep in sync
5. **Supporting growth** - Clear patterns for adding future documentation

The new structure supports both the current documentation needs and provides a scalable foundation for future growth as CargoShip continues to evolve.

---

*This organization was implemented in Q3 2025 as part of the project documentation cleanup initiative.*