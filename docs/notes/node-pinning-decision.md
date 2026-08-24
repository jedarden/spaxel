# Node Pinning Decision for Spaxel Pod

**Date:** 2026-08-23
**Status:** ACCEPTED
**Bead:** spaxel-73aceb04

## Context

The spaxel mothership pod currently runs on a k3s cluster with the following characteristics:

- **Cluster Node:** `k3s-agent-minisforum` at IP `10.20.23.203`
- **Public Domain:** `spaxel.ardenone.com` (routed through Cloudflare)
- **Current Network Configuration:** Uses `hostNetwork: true` for mDNS support
- **Current Issue:** `SPAXEL_ADVERTISED_BASE_URL` is set to `http://10.20.23.203:8080` (hardcoded node IP)
- **Problem:** The pod is NOT pinned to the node, so if it reschedules, the OTA URL and mothership address break silently

## Analysis of Options

### Option 1: nodeSelector/Affinity Pinning

**Approach:** Add `nodeSelector` or `nodeAffinity` to pin the spaxel pod to `k3s-agent-minisforum`

**Pros:**
- Simple to implement (single YAML change)
- Makes current hardcoded IP approach durable
- Pods stay on the known node with stable IP
- No additional infrastructure required

**Cons:**
- Relocates the problem rather than solving it (just moves the fragility)
- Ties deployment to specific node hardware
- Creates single point of failure
- Maintenance burden if node needs to be replaced or upgraded
- Reduces Kubernetes scheduling flexibility
- Complicates cluster maintenance and upgrades

**Implementation:**
```yaml
spec:
  template:
    spec:
      nodeSelector:
        kubernetes.io/hostname: k3s-agent-minisforum
      # OR using affinity:
      affinity:
        nodeAffinity:
          requiredDuringSchedulingIgnoredDuringExecution:
            nodeSelectorTerms:
            - matchExpressions:
              - key: kubernetes.io/hostname
                operator: In
                values:
                - k3s-agent-minisforum
```

### Option 2: LAN-Resolvable DNS Name

**Approach:** Create a stable DNS name that resolves to the spaxel service regardless of which node the pod runs on

**Pros:**
- Survives deliberate pod movement and rescheduling
- More robust and flexible architecture
- Decouples service identity from node identity
- Better for multi-node clusters and scaling
- Aligns with Kubernetes service discovery best practices
- Enables future cluster expansion without reconfiguration

**Cons:**
- Requires DNS infrastructure setup
- More complex initial setup
- Requires internal DNS or Kubernetes Service with stable endpoint

**Implementation Approaches:**

1. **Kubernetes Service (ClusterIP)**
   - Create a Service to expose the pod
   - Use `spaxel.home.svc.cluster.local` or similar
   - Requires updating node provisioning to use DNS name

2. **Internal DNS Record**
   - Create A record in internal DNS (e.g., `spaxel.home.lan`)
   - Point to current pod IP (requires dynamic updates or short TTL)
   - Could use external-dns with DNS controller

3. **Use mDNS Service Name**
   - Already implemented: `_spaxel._tcp.local`
   - Nodes already discover mothership via mDNS
   - Extend to use mDNS name for OTA URL
   - Limitation: mDNS may not cross VLAN/subnet boundaries

**Existing DNS Infrastructure:**
- Public domain: `spaxel.ardenone.com` (already configured)
- mDNS service: `_spaxel._tcp.local` (already implemented)
- Node fallback: `ms_ip` NVS key for manual IP configuration

## Decision

**Decision:** **Use LAN-Resolvable DNS Name (Option 2)** with a phased implementation approach

### Rationale

1. **Architectural Superiority:** DNS-based service discovery is the standard pattern for Kubernetes deployments and aligns with cloud-native best practices

2. **Future-Proofing:** As the cluster grows or changes, the DNS approach remains valid without reconfiguration

3. **Separation of Concerns:** Decouples the service identity from infrastructure specifics, making the system more maintainable

4. **Leverages Existing Infrastructure:** The system already has mDNS service discovery implemented; extending this pattern is natural

5. **Avoids Technical Debt:** nodeSelector pinning creates ongoing maintenance burden and operational fragility

## Implementation Plan

### Phase 1: Immediate (Interim) - Use Public Origin with Authentication
**Timeline:** Before enabling node pinning

1. Implement ADR-006 firmware authentication (already tracked as `bf-b61zo`)
2. Configure `SPAXEL_ADVERTISED_BASE_URL=https://spaxel.ardenone.com`
3. This provides immediate benefit without node pinning

### Phase 2: Short-Term (DNS Setup) - Internal Service Discovery
**Timeline:** Within 1-2 sprints

1. **Create Kubernetes Service:**
   ```yaml
   apiVersion: v1
   kind: Service
   metadata:
     name: spaxel
     namespace: home
   spec:
     selector:
       app: spaxel
     ports:
     - port: 8080
       targetPort: 8080
       name: http
   ```

2. **Update Configuration:**
   - Set `SPAXEL_ADVERTISED_BASE_URL` to use service DNS
   - Example: `http://spaxel.home.svc.cluster.local:8080`
   - For LAN nodes: `http://spaxel.home:8080` (if using ClusterIP with external DNS)

3. **Update Node Provisioning:**
   - Modify provisioning payload to include DNS name
   - Nodes can use DNS name instead of hardcoded IP

### Phase 3: Long-Term (Enhanced DNS) - Robust Service Discovery
**Timeline:** Future enhancement

1. **Extend mDNS for OTA:**
   - Configure mothership to advertise mDNS name in OTA URL
   - Nodes already query mDNS for mothership discovery
   - Leverage existing `_spaxel._tcp.local` service

2. **Add DNS Fallback Chain:**
   - Try mDNS first
   - Fall back to DNS service name
   - Fall back to cached `ms_ip` NVS key

3. **External DNS Integration (Optional):**
   - Consider using external-dns or similar for dynamic DNS updates
   - Only if using A-record based approach

## Prerequisites

### For Phase 1 (Immediate):
- ✅ Already tracked: `bf-b61zo` (firmware authentication)
- ✅ Public domain already configured: `spaxel.ardenone.com`
- ✅ Cloudflare ingress already operational

### For Phase 2 (DNS Setup):
- Kubernetes cluster with Service support
- Ability to update Deployment manifests
- Testing environment with multiple nodes (if scaling)
- DNS resolution from nodes to cluster (either internal DNS or external DNS)

### For Phase 3 (Enhanced DNS):
- mDNS infrastructure testing across VLAN/subnet boundaries
- DNS controller if using external-dns approach
- Node firmware updates to support DNS-based OTA URLs

## Why Not nodeSelector Pinning

While nodeSelector pinning would solve the immediate problem, it was rejected because:

1. **It relocates rather than resolves the problem** - the fragility remains, just moves to a different layer
2. **Creates operational debt** - adds maintenance burden for cluster operations
3. **Reduces flexibility** - complicates future scaling and cluster management
4. **Against Kubernetes best practices** - pods should be treated as ephemeral and scheduled freely
5. **Single point of failure** - entire service depends on one physical node

The DNS-based approach, while requiring more upfront work, provides a more robust and maintainable foundation for the long term.

## Related Documentation

- **ADR-006:** Authenticate firmware downloads with existing node token
- **docs/plan/plan.md:** Complete architecture and ADR reference
- **docs/deployment/wifi-configuration.md:** Kubernetes deployment examples
- **docs/notes/mdns-override.md:** mDNS troubleshooting and fallback mechanisms
- **docker-compose.yml:** Current deployment configuration using hostNetwork
- **mothership/internal/config/config.go:** SPAXEL_ADVERTISED_BASE_URL configuration logic

## Follow-up Actions

1. ✅ Document this decision in `docs/notes/node-pinning-decision.md`
2. Track Phase 1 implementation: firmware authentication (`bf-b61zo` → `bf-5kuen` → `bf-3doto`)
3. Create new beads for Phase 2 (DNS Service setup) if not already tracked
4. Update deployment documentation to reflect DNS-based approach
5. Consider deprecating hardcoded IP addresses in favor of DNS names

---

**Decision Date:** 2026-08-23
**Status:** ACCEPTED - DNS-based service discovery is the chosen approach
**Applies To:** Spaxel mothership deployment on k3s cluster
