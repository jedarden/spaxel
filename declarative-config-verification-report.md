# Declarative-Config Directory Verification Report

**Date:** 2026-08-28  
**Scope:** Verification of `declarative-config/k8s/iad-ci/argo-workflows/` directory structure

## Executive Summary

The `declarative-config` directory **does not exist within the `/home/coding/spaxel/` repository**. Based on documentation findings, the declarative-config is located in a **separate external repository**: `jedarden/declarative-config`.

## Key Findings

### 1. Directory Location

**Expected Path (external):** `jedarden/declarative-config/k8s/iad-ci/argo-workflows/`  
**Status:** External repository - NOT within spaxel

**Evidence from spaxel documentation:**
- `/home/coding/spaxel/docs/BUILD_PATHS.md` states: "Production uses Kubernetes manifests from `jedarden/declarative-config` (external repository)."
- `/home/coding/spaxel/docs/notes/ci-test-sim-reference-map.md` confirms: "The WorkflowTemplates live in `jedarden/declarative-config` (outside this repo)."

### 2. Argo Workflows Structure (from documentation)

**Expected Structure in external repository:**
```
jedarden/declarative-config/
└── k8s/
    └── iad-ci/
        └── argo-workflows/
            ├── spaxel-build-workflowtemplate.yml
            ├── spaxel-e2e-workflowtemplate.yml
            └── [other WorkflowTemplate files]
```

**Confirmed WorkflowTemplate files from documentation:**
- `spaxel-build-workflowtemplate.yml` (417 lines) - primary CI/CD template
- `spaxel-e2e-workflowtemplate.yml` - end-to-end testing template

### 3. Cluster Integration

**Namespace:** `argo-workflows` (in `iad-ci` cluster)  
**Access Method:** Via ArgoCD sync from declarative-config repository  
**Kubeconfig:** `/home/coding/.kube/iad-ci.kubeconfig` (outside spaxel)

## Search Scope Documentation

**Constraints:** This verification was limited to the `/home/coding/spaxel/` directory only.  
**Search methods used:**
1. Directory listing of `/home/coding/spaxel/`
2. Grep search for "declarative-config" references in documentation
3. Grep search for "argo-workflows" references in documentation

**What was NOT searched (outside constraints):**
- External repositories (`jedarden/declarative-config`)
- Kubernetes cluster (`iad-ci`)
- Home directory kubeconfig location (`/home/coding/.kube/`)

## Conclusion

✅ **CONFIRMED:** The declarative-config directory exists in the external `jedarden/declarative-config` repository  
❌ **NOT FOUND:** No `declarative-config` directory within the spaxel repository  
✅ **DOCUMENTED:** Argo WorkflowTemplates are managed separately and synced via ArgoCD

**Recommendation:** To verify the actual WorkflowTemplate files, access to the `jedarden/declarative-config` repository or the `iad-ci` cluster is required.
