# Repository Structure Verification Findings

**Date:** 2026-09-01  
**Task:** Document repository structure and verify declarative-config location

## Executive Summary

✅ **CONFIRMED:** `declarative-config` exists as a **separate repository**, not as a subdirectory within spaxel. Both repositories reside at `/home/coding/` as sibling directories.

## Repository Locations and Remote URLs

### Primary Repository: spaxel

**Local Path:** `/home/coding/spaxel/`

**Git Remote:**
```
origin	https://git.ardenone.com/jedarden/spaxel.git (fetch)
origin	https://git.ardenone.com/jedarden/spaxel.git (push)
```

**Description:** WiFi CSI-based indoor positioning system for self-hosted homes. Contains the ESP32-S3 firmware, Go mothership backend, and JavaScript dashboard.

### Separate Repository: declarative-config

**Local Path:** `/home/coding/declarative-config/`

**Git Remotes:**
```
github	https://github.com/jedarden/declarative-config.git (fetch)
github	https://github.com/jedarden/declarative-config.git (push)
origin	https://git.ardenone.com/jedarden/declarative-config.git (fetch)
origin	https://git.ardenone.com/jedarden/declarative-config.git (push)
```

**Description:** Kubernetes manifests and infrastructure configuration. Contains Argo Workflows templates, cluster configurations, and deployment manifests for the entire fleet.

**Key Structure:**
- `k8s/` - Kubernetes manifests for various clusters
- `k8s/iad-ci/argo-workflows/` - Argo WorkflowTemplates (including spaxel-build, spaxel-e2e)
- `containers/` - Container build configurations
- `nixos/` - NixOS system configurations

## Verification Results

### 1. declarative-config is NOT a subdirectory of spaxel

**Status:** ✅ **SEPARATE REPOSITORY**

The `declarative-config` directory exists at `/home/coding/declarative-config/` as a standalone Git repository, completely independent from the spaxel repository. This is confirmed by:

1. **Separate directory structure:** Both repositories are siblings under `/home/coding/`
2. **Separate Git repositories:** Each has its own `.git` directory and remote configuration
3. **Separate remotes:** declarative-config has dual remotes (GitHub + Forgejo), while spaxel uses only Forgejo
4. **No nested structure:** `/home/coding/spaxel/declarative-config/` does not exist

### 2. ArgoCD Integration

**Cluster:** `iad-ci`  
**Namespace:** `argo-workflows`  
**Access Method:** ArgoCD syncs from the declarative-config repository  
**Kubeconfig:** `/home/coding/.kube/iad-ci.kubeconfig` (external to both repos)

**Relevant WorkflowTemplates (from external repo):**
- `spaxel-build-workflowtemplate.yml` - Primary CI/CD build template
- `spaxel-e2e-workflowtemplate.yml` - End-to-end testing template

### 3. Cross-Repository References

**spaxel references declarative-config in documentation:**
- `docs/BUILD_PATHS.md` states: "Production uses Kubernetes manifests from jedarden/declarative-config (external repository)."
- `docs/notes/ci-test-sim-reference-map.md` confirms: "The WorkflowTemplates live in jedarden/declarative-config (outside this repo)."

## Anomalies and Issues

### ⚠️ Dual Remote Configuration

**Issue:** The declarative-config repository has **two separate remotes** configured:
- `github` → GitHub mirror
- `origin` → Forgejo (git.ardenone.com) primary

**Impact:** This is actually the intended design per the Git Hosting section of CLAUDE.md. Forgejo is the source of truth, with GitHub as a read-only mirror synced automatically via Forgejo's server-side push mirror.

**Status:** ✅ **Working as designed** - No action needed

### ✅ No Issues Detected

- Both repositories are properly configured with correct remotes
- Documentation accurately reflects the repository structure
- Previous verification report (2026-08-28) findings remain accurate
- No inconsistencies between documented and actual structure

## Conclusion

The repository structure is clean and well-organized:

1. **spaxel** - Application code (firmware, mothership, dashboard)
2. **declarative-config** - Infrastructure and deployment manifests

This separation follows GitOps best practices, keeping application logic separate from infrastructure configuration. The dual-remote setup for declarative-config (Forgejo primary + GitHub mirror) is intentional and functioning correctly.

---

**Verification Method:**
- Directory inspection of `/home/coding/`
- Git remote verification for both repositories
- Cross-reference with existing documentation
- Comparison with previous verification report (2026-08-28)

**Previous Reference:** See `declarative-config-verification-report.md` (2026-08-28) for detailed Argo Workflows structure analysis.
