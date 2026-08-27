# Node Pinning Decision for Spaxel Pod

**Date:** 2026-08-27
**Status:** DECIDED — Continue with nodeSelector
**Related Bead:** spaxel-73aceb04

## Problem Statement

The spaxel mothership pod needs stable network addressing so that:
1. Nodes can reliably reach the mothership for WebSocket connections (`ws://<host>:8080/ws/node`)
2. Nodes can fetch OTA firmware updates from a stable URL
3. The OTA base URL doesn't break silently if the pod reschedules

**Current Risk:** If the pod reschedules to a different node (e.g., from `k3s-agent-minisforum` at `10.20.23.203` to another node at a different IP), any hardcoded IP-based URLs would break.

## Options Evaluated

### Option 1: nodeSelector (Current Approach) ✅ **RECOMMENDED**

**Implementation:**
```yaml
spec:
  nodeSelector:
    kubernetes.io/hostname: k3s-agent-minisforum
```

**How it works:**
- Kubernetes scheduler places the pod ONLY on `k3s-agent-minisforum`
- Pod IP is durably `10.20.23.203` (node's IP)
- If `k3s-agent-minisforum` is unavailable, pod stays in `Pending` state rather than rescheduling elsewhere

**Advantages:**
1. **Simplicity** — One-line configuration, no external DNS infrastructure required
2. **Predictability** — Pod location and IP are known and stable
3. **No new dependencies** — Works with existing k3s cluster, no DNS server setup needed
4. **Explicit coupling** — Makes the hardware dependency visible in manifests
5. **Graceful failure** — If the pinned node is down, pod fails to schedule rather than appearing to work at a wrong IP

**Disadvantages:**
1. **Tight hardware coupling** — Pod cannot run if `k3s-agent-minisforum` is unavailable
2. **Manual intervention required** — If that node needs maintenance, the nodeSelector must be updated manually
3. **Single point of failure** — If `k3s-agent-minisforum` fails, spaxel is down until nodeSelector is changed

**Operational Mitigations:**
- Document the hardware dependency in runbooks
- Have a procedure for updating nodeSelector during planned maintenance
- Monitor node health to detect failures early

### Option 2: LAN-Resolvable DNS Name

**Implementation Options:**

**2A. Add manual DNS A record:**
```
spaxel-minisforum.lan A 10.20.23.203
```
Then use `SPAXEL_ADVERTISED_BASE_URL=http://spaxel-minisforum.lan:8080`

**2B. Use Tailscale hostname:**
```
spaxel.ardenone.com → resolves to node's Tailscale IP
```

**2C. Rely on Kubernetes service DNS:**
```
spaxel.spaxel.svc.cluster.local → ClusterIP → pod (via hostNetwork)
```

**Investigation Results:**

- ✅ **Public HTTPS works:** `spaxel.ardenone.com` resolves to Cloudflare IPs (`172.67.219.132`, `104.21.45.222`)
- ❌ **Node hostname has no DNS:** `k3s-agent-minisforum` returns `NXDOMAIN` (no DNS record exists)
- ⚠️ **Cluster DNS has limitations:** Kubernetes service DNS (`spaxel.spaxel.svc.cluster.local`) only works within the cluster network. ESP32 nodes on the LAN cannot resolve cluster-internal DNS.
- ❌ **Tailscale DNS not verified:** Would require checking if `spaxel` hostname exists in Tailscale network

**Advantages:**
1. **Loose coupling** — Pod could move between nodes while maintaining the same hostname
2. **More flexible operations** — Easier to drain/replace the pinned node
3. **Matches esp32-firmware pattern** — Nodes already use mDNS (`_spaxel._tcp.local`) for discovery

**Disadvantages:**
1. **Requires external DNS infrastructure** — Not available in current setup
2. **Additional moving parts** — DNS server adds complexity and failure modes
3. **No immediate benefit** — Current deployment is single-pod; nodeSelector already provides the same stability
4. **Cluster DNS doesn't work for LAN clients** — ESP32 nodes cannot resolve `*.cluster.local` names
5. **Debugging complexity** — DNS issues can be opaque and time-consuming to troubleshoot

## Current Deployment State

**Actual configuration in `/home/coding/declarative-config/k8s/ardenone-cluster/spaxel/deployment.yml`:**

```yaml
spec:
  hostNetwork: true
  dnsPolicy: ClusterFirstWithHostNet
  nodeSelector:
    kubernetes.io/hostname: k3s-agent-minisforum
  containers:
  - name: mothership
    env:
    - name: SPAXEL_ADVERTISED_BASE_URL
      value: "https://spaxel.ardenone.com"
```

**Key observations:**
1. ✅ **nodeSelector already implemented** — Pod is pinned to `k3s-agent-minisforum`
2. ✅ **Public HTTPS origin used** — `SPAXEL_ADVERTISED_BASE_URL` is `https://spaxel.ardenone.com`, NOT a LAN IP
3. ✅ **hostNetwork: true** — Pod binds directly to node's network interface
4. ✅ **This combination is already production-ready and working**

## Decision

**Continue with current approach: nodeSelector + public HTTPS origin**

### Rationale

1. **Already implemented and working** — No changes needed, deployment is stable
2. **Public HTTPS origin solves the rescheduling problem** — Nodes use `spaxel.ardenone.com` for OTA, which doesn't change if the pod moves
3. **mDNS handles node discovery** — Nodes discover mothership via `_spaxel._tcp.local`, not by hardcoded IP
4. **Simplest solution** — No DNS infrastructure to maintain or debug
5. **Acceptable trade-off** — Tight hardware coupling to one node is acceptable for a single-pod deployment; if that node fails, manual intervention is required regardless

### Why NOT DNS

1. **No DNS infrastructure exists** — `k3s-agent-minisforum` has no DNS record; setting one up is additional work
2. **Cluster DNS is LAN-invisible** — ESP32 nodes cannot resolve cluster-internal DNS
3. **Complexity without benefit** — DNS would add infrastructure but wouldn't improve reliability for single-pod deployment
4. **Can revisit later** — If spaxel scales to multiple pods or nodes, DNS becomes more valuable

## Prerequisites

**None — Current deployment is complete.**

### Future Considerations

**If deployment grows to multiple pods/nodes:**
1. **Revisit DNS approach** — Create `spaxel-internal.lan` DNS record pointing to a virtual IP or use load balancer
2. **Consider removing hostNetwork** — Use Kubernetes Service with `type: LoadBalancer` or `NodePort` instead
3. **Evaluate headless service** — For direct pod-to-pod communication without ClusterIP
4. **Consider service mesh** — If multi-pod service mesh becomes necessary

**If high availability is required:**
1. **Add anti-affinity** — Prevent multiple spaxel pods from landing on same node
2. **Pod disruption budgets** — Ensure at least one pod is available during node upgrades
3. **Multiple nodeSelectors** — Allow pod to run on one of several specific nodes
4. **Remove single point of failure** — DNS-based service discovery becomes critical

## Operational Guidance

**During planned maintenance on k3s-agent-minisforum:**
1. Check which node is currently pinned: `kubectl get deployment spaxel -n spaxel -o yaml | grep nodeSelector`
2. If node must be drained, update nodeSelector to an alternative node first
3. Verify pod is healthy on new node before proceeding with maintenance
4. Update any runbooks or documentation that reference the old node

**Monitoring:**
1. Alert on pod `Pending` state — indicates pinned node is unavailable
2. Alert on node `NotReady` — proactively detect hardware issues
3. Monitor node resource usage — ensure headroom for spaxel pod

**Verification:**
- Pod is running on `k3s-agent-minisforum`: `kubectl get pods -n spaxel -o wide`
- Pod IP should be `10.20.23.203` (node's IP)
- Service endpoint: `kubectl get endpoints spaxel -n spaxel` should show `10.20.23.203:8080`
- OTA URL accessible: `curl -I https://spaxel.ardenone.com/firmware/` should return HTTP headers

## Conclusion

**Status:** ✅ No action required

The current deployment configuration already implements the optimal approach for the current scale:
- **nodeSelector** ensures stable pod placement
- **Public HTTPS origin** provides stable OTA addressing
- **mDNS** handles node discovery
- **hostNetwork** allows direct LAN access for nodes

If and when the deployment scales or high availability becomes a requirement, revisit DNS-based service discovery. For now, simplicity and stability are the priority.
