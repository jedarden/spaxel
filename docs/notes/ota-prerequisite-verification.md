# OTA Prerequisite Verification

**Date**: 2026-08-24
**Bead**: spaxel-15f7f9da
**Task**: Verify prerequisites for public firmware OTA

## Summary

**BLOCKED**: Server-side enforcement is implemented but NOT DEPLOYED. Cannot verify node firmware versions without a running deployment.

## Prerequisites

### 1. Server-Side Enforcement (bf-5kuen)
**Requirement**: The route must be authenticated before we exempt /firmware, or we publish every firmware image to the open internet.

**Status**: ⚠️ CODE IMPLEMENTED, NOT DEPLOYED

**Findings**:
- Authentication code exists in `mothership/internal/ota/server.go` (lines 286-313)
- Token validation logic properly implemented:
  - Checks `X-Spaxel-MAC` and `X-Spaxel-Token` headers
  - Validates tokens using `provSrv.ValidateToken`
  - Returns 404 (not 401) to avoid leaking firmware filenames
  - Supports migration window via `SetMigrationDeadline()`
- Main.go wires up token validator: `otaSrv.SetTokenValidator(provSrv.ValidateToken)` (line 4822)
- **NO spaxel deployment found** in iad-ci or ardenone-manager clusters

**Risk**: HIGH - If `/firmware` is exempted at ingress before authentication is live, all firmware images become publicly accessible without access control.

### 2. Node Firmware Versions (≥0.2.19 with wss:// support)
**Requirement**: All nodes must run firmware 0.2.19+ with wss:// support - the public origin is HTTPS-only.

**Status**: ⚠️ CANNOT VERIFY - No running deployment

**Findings**:
- Current codebase version: **0.2.79** (from VERSION file)
- Version 0.2.79 exceeds the 0.2.19 threshold
- Documentation confirms 0.2.19 includes wss:// support
- **No actual deployment or nodes found** to verify runtime versions
- Cannot confirm if any nodes exist on older firmware

**Risk**: MEDIUM - Nodes on firmware < 0.2.19 cannot download over TLS, but verification is impossible without a deployment.

## Cluster Verification

Checked clusters:
- `iad-ci`: No spaxel pods found
- `ardenone-manager`: No spaxel pods found
- No workflows or deployments detected

## Code Verification

✅ Authentication implementation is present and correct:
- Token validation in `internal/ota/server.go:HandleServe()`
- Proper wiring in `mothership/cmd/mothership/main.go:4822`
- Migration window support implemented
- 404 responses prevent information leakage

## Blocking Issues

1. **No deployment exists** - Authentication code cannot be verified in a live environment
2. **No nodes to check** - Cannot confirm firmware versions in actual fleet
3. **Production exposure risk** - `/firmware` endpoint cannot be safely exempted at ingress

## Recommendation

**DO NOT PROCEED** with public firmware OTA deployment until:

1. Deploy spaxel to a cluster with authentication enabled
2. Verify authentication is working on `/firmware` endpoint
3. Confirm actual node firmware versions ≥ 0.2.19
4. Test end-to-end OTA flow with authentication

## Next Steps

1. Deploy spaxel (likely to ardenone-manager or appropriate cluster)
2. Verify `GET /firmware/<filename>` requires valid headers
3. Check node registry for firmware versions
4. Document production deployment architecture

## References

- ADR-006: Firmware download authentication
- Bead chain: `bf-b61zo` (firmware headers) → `bf-5kuen` (server enforcement) → `bf-3doto` (ingress exemption)
