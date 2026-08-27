# Kaniko v1.24.0 Stable Release Research

**Date:** 2025-08-27
**Purpose:** Identify current stable Kaniko release for WorkflowTemplate update

## Current Stable Release

**Version:** v1.24.0
**Release Date:** May 21, 2025

## Image Reference

**Standard executor image:**
```
gcr.io/kaniko-project/executor:v1.24.0
```

**Image variants available:**
- Standard: `gcr.io/kaniko-project/executor:v1.24.0`
- Debug: `gcr.io/kaniko-project/executor:debug-v1.24.0`
- Slim: `gcr.io/kaniko-project/executor:slim-v1.24.0`

**Digest:** Not obtained from GitHub releases page (would need to query registry directly)

## Release Notes & Breaking Changes

### Security Fixes
- **CVE-2025-21613:** Upgraded go-git to version 5.13.1 to address security vulnerability

### Bug Fixes
- Fixed panic that occurred when image name and stage alias are the same

### Dependency Updates
- ca-certificates source updated to Debian Bookworm
- containerd updates
- AWS SDK updates
- Various Go library dependency updates

## Important Status Note

**Repository Status:** The Kaniko repository was **archived on June 3, 2025** and is now read-only. This means v1.24.0 is likely the final release from the original GoogleContainerTools organization.

## Recommendation

Use `gcr.io/kaniko-project/executor:v1.24.0` as the standard image reference for the WorkflowTemplate update. The slim variant (`:slim-v1.24.0`) may be preferable for smaller image size if debug features are not required.

No major breaking changes were identified in this release that would impact existing Kaniko usage.
