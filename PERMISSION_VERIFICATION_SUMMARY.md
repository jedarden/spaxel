# Permission Verification Summary

**Date:** 2026-09-02

## Verification Scope

This document tracks permission verification for the Spaxel WiFi CSI-based indoor positioning system. It records:

- System-level permissions granted and verified
- Resource access rights confirmed through testing
- Security boundaries validated
- OAuth/approval scopes and their current status

### Verification Categories

#### Mothership (Go Backend)
- Database access permissions
- Network binding and socket permissions
- File system access (data directory, firmware storage)
- Process ownership and user rights

#### Firmware (ESP32-S3)
- WiFi peripheral access
- CSI capture permissions
- BLE scanning capabilities
- Flash/NVS read/write rights
- OTA update permissions

#### Dashboard (Web UI)
- WebSocket connection permissions
- API access control
- Session management
- Cross-origin resource sharing (CORS)

#### Infrastructure
- Docker container privileges
- Kubernetes RBAC policies
- mDNS advertisement rights
- TLS certificate handling

---

*Last verified: 2026-09-02*
