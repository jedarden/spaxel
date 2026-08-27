# Kaniko Stable Release Research

**Date Researched:** 2026-08-27

## Current Stable Release

**Version:** v1.24.0  
**Release Date:** May 21, 2025  
**Commit:** 1d2bff595903b1887220a522a5a43c67db2da553

## Image Reference

```
gcr.io/kaniko-project/executor:v1.24.0
```

### Alternative Tags
- `gcr.io/kaniko-project/executor:latest` (points to v1.24.0)
- `gcr.io/kaniko-project/executor:debug` (debug build)
- `gcr.io/kaniko-project/executor:slim` (slim variant)

## Release Notes

### Key Updates in v1.24.0
- **Security Fix:** Addresses CVE-2025-21613 by upgrading go-git to v5.13.1
- **Bug Fix:** Prevents panic when image name and stage alias are the same

### Recommendations
1. Use the specific version tag `v1.24.0` rather than `latest` for reproducible builds
2. Consider using the digest for maximum security (obtainable via `docker pull gcr.io/kaniko-project/executor:v1.24.0` followed by `docker inspect`)
3. The security fix for CVE-2025-21613 makes this release important for security-conscious deployments

## Breaking Changes
No breaking changes were noted in the v1.24.0 release notes.
