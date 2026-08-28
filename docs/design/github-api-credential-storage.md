# GitHub API Credential Storage Design

**Design Date:** 2026-08-28  
**Status:** Design — No Implementation Yet  
**Bead:** spaxel-26acecac

## Overview

This document designs the storage approach for GitHub API credentials used by Spaxel to fetch Kaniko releases. The design follows organization-wide patterns for secret management using OpenBao and ExternalSecrets operator.

## Current State

### Existing Implementation
- **Location:** `mothership/internal/github/client.go` (206 lines)
- **Configuration:** `SPAXEL_GITHUB_TOKEN` environment variable (config.go:80)
- **Usage:** Fetch Kaniko releases from GitHub API
- **Rate Limits:**
  - Unauthenticated: 60 requests/hour
  - Authenticated (PAT): 5,000 requests/hour

### Current Problem
- No secure storage mechanism defined
- Credential lifecycle (creation, rotation, revocation) not documented
- No integration with organization's OpenBao-based secret management

## Design Requirements

### Functional Requirements
1. **Secure Storage:** Credentials stored in OpenBao, never in git or environment files
2. **Automatic Retrieval:** Credentials loaded via ExternalSecrets operator at pod startup
3. **Minimal Permissions:** Token scoped to minimum required permissions
4. **Rotation Support:** Clear process for token rotation without downtime

### Non-Functional Requirements
1. **Security:** Credentials never appear in transcripts, logs, or git
2. **Reliability:** Graceful degradation if credential unavailable (fallback to unauthenticated)
3. **Auditability:** Clear documentation of who has access and how to rotate
4. **Consistency:** Follow organization-wide secret management patterns

## Storage Method

### OpenBao Path

**Chosen Path:** `secret/ardenone-cluster/spaxel/github-token`

**Rationale:**
- Spaxel runs in `ardenone-cluster` (per deployment location in declarative-config)
- Follows organization pattern: `secret/<cluster>/<app>/<credential-name>`
- Owned by ardenone-cluster OpenBao instance: `http://traefik-ardenone-cluster:8200`

**Why OpenBao:**
- Organization standard for all secrets
- Automatic replication to other clusters (30-minute cycle)
- Audit logging of all access
- Integration with ExternalSecrets operator

### Alternative Considered (Not Chosen)

**Environment Variable File (e.g., `/etc/spaxel/creds.env`):**
- ❌ Requires manual distribution to all nodes
- ❌ No audit trail of access
- ❌ Rotation requires node restart coordination
- ❌ Violates organization-wide secret management standards

## Credential Structure

### OpenBao Secret Data

**Path:** `secret/ardenone-cluster/spaxel/github-token`

**Keys:**
```json
{
  "token": "ghp_exampleTokenValue1234567890",
  "created": "2026-08-28T00:00:00Z",
  "created-by": "jedarden",
  "purpose": "GitHub API access for Kaniko releases"
}
```

**Required Fields:**
- `token` — GitHub Personal Access Token (classic or fine-grained)
- `created` — ISO 8601 timestamp (RFC3339)
- `created-by` — GitHub username or operator identifier
- `purpose` — Human-readable description

**Optional Fields (for future enhancement):**
- `scopes` — List of granted scopes (for classic PATs)
- `expires` — Token expiration date (if applicable)
- `rotation-policy` — Rotation schedule or trigger conditions

### Token Format

**Classic Personal Access Token:**
- Format: `ghp_` + 40 characters
- Required scopes: `public_repo` (minimum for Kaniko)
- Recommended scopes: `public_repo`, `read:org` (if organization repos needed)
- No expiration by default

**Fine-grained Personal Access Token:**
- Format: `github_pat_` + 82 characters
- Repository permissions: Read access to `GoogleContainerTools/kaniko`
- Expiration: Required by GitHub (1 year max)
- Enhanced security audit logging

**Recommendation:** Use classic PAT for simplicity and no expiration, unless organization policy requires fine-grained tokens.

## Retrieval Mechanism

### ExternalSecret Configuration

**File:** `declarative-config/k8s/ardenone-cluster/spaxel/externalsecret.yml`

```yaml
---
# =============================================================================
# Spaxel ExternalSecret — GitHub API Token
# =============================================================================
# Pulls GitHub API token from OpenBao for Kaniko release fetching.
# OpenBao KV path: secret/ardenone-cluster/spaxel/github-token
# Expected keys:
#   token                  — GitHub Personal Access Token
# =============================================================================
apiVersion: external-secrets.io/v1beta1
kind: ExternalSecret
metadata:
  name: spaxel-github-token
  namespace: spaxel
  labels:
    app.kubernetes.io/name: spaxel
    app.kubernetes.io/component: mothership
    app.kubernetes.io/part-of: spaxel
spec:
  refreshInterval: 1h
  secretStoreRef:
    name: openbao-ardenone-cluster
    kind: ClusterSecretStore
  target:
    name: spaxel-github-token
    creationPolicy: Owner
    template:
      type: Opaque
      data:
        # Config expects SPAXEL_GITHUB_TOKEN environment variable
        SPAXEL_GITHUB_TOKEN: "{{ .token }}"
  data:
    - secretKey: token
      remoteRef:
        key: ardenone-cluster/spaxel/github-token
        property: token
```

### Deployment Integration

**Update:** `declarative-config/k8s/ardenone-cluster/spaxel/deployment.yml`

```yaml
# In the container spec:
env:
  # ... existing env vars ...
  - name: SPAXEL_GITHUB_TOKEN
    valueFrom:
      secretKeyRef:
        name: spaxel-github-token
        key: SPAXEL_GITHUB_TOKEN
        optional: true  # Allow graceful degradation if secret missing
```

### Data Flow

```
┌─────────────────┐
│ OpenBao         │
│ (ardenone-      │
│  cluster)       │
│                 │
│ secret/ardenone-│
│  cluster/spaxel │
│  /github-token  │
└────────┬────────┘
         │
         │ ExternalSecrets Operator
         │ (pulls every 1h)
         ▼
┌─────────────────┐
│ Kubernetes      │
│ Secret          │
│ (spaxel ns)     │
│                 │
│ spaxel-github-  │
│  token          │
└────────┬────────┘
         │
         │ Deployment envFrom
         ▼
┌─────────────────┐
│ Spaxel Container│
│                 │
│ SPAXEL_GITHUB_  │
│  TOKEN env var  │
└─────────────────┘
```

## Security Requirements

### Credential Creation

**Command (run on ex44 or via OpenBao UI):**

```bash
# NEVER type the token value in command line — use stdin
# Generate a new token in GitHub, then:
echo "ghp_yourActualTokenValueHere" | \
  bao kv put - \
  secret/ardenone-cluster/spaxel/github-token \
  token=- \
  created="$(date -u +"%Y-%m-%dT%H:%M:%SZ")" \
  created-by="jedarden" \
  purpose="GitHub API access for Kaniko releases"
```

**Prohibited Patterns:**
```bash
# ❌ NEVER THIS — token appears in argv, transcript, ps, bash history
bao kv put secret/ardenone-cluster/spaxel/github-token token=ghp_123...

# ❌ NEVER THIS — value visible to anyone reading process listing
bao kv put secret/x/y value="$(cat ~/github-token.txt)"
```

### Access Control

**Who Can Write:**
- Cluster administrators with OpenBao write access
- Operators with `secret/ardenone-cluster/*` write policy

**Who Can Read:**
- ExternalSecrets operator service account (automatic via ClusterSecretStore)
- Spaxel pods (via mounted Secret, not direct OpenBao access)

**Audit Logging:**
- All writes to OpenBao are logged (timestamp, caller, key)
- All ExternalSecret syncs are logged (timestamp, success/failure)

### Graceful Degradation

**If Credential Unavailable:**
1. ExternalSecret missing → Spaxel starts without `SPAXEL_GITHUB_TOKEN`
2. GitHub client falls back to unauthenticated requests (60/hour)
3. Warning logged: `[CONFIG] SPAXEL_GITHUB_TOKEN=(not set, unauthenticated GitHub API requests will be rate-limited)`

**Rate Limit Handling (Already Implemented):**
- Client detects 403 + `X-RateLimit-Remaining: 0`
- Logs warning with rate limit info
- Returns error to caller (no crash, no retry storm)

## Rotation Procedure

### Planned Rotation (No Downtime)

**Steps:**

1. **Create new token in GitHub:**
   - Go to GitHub Settings → Developer settings → Personal access tokens
   - Generate new token with `public_repo` scope
   - Copy token (never save to disk, use clipboard only)

2. **Update OpenBao (use stdin to avoid transcript):**
   ```bash
   echo "ghp_newTokenValue" | \
     bao kv put -cas=$(bao kv get -field=metadata \
       secret/ardenone-cluster/spaxel/github-token | jq .version) \
     secret/ardenone-cluster/spaxel/github-token \
     token=- \
     created="$(date -u +"%Y-%m-%dT%H:%M:%SZ")" \
     created-by="jedarden" \
     purpose="GitHub API access for Kaniko releases"
   ```
   - `-cas=<version>` ensures no concurrent write overwrites your update
   - Check metadata version before starting

3. **Wait for ExternalSecret sync (max 1 hour, or force):**
   ```bash
   kubectl --kubeconfig=/home/coding/.kube/ardenone-cluster.kubeconfig \
     delete externalsecret spaxel-github-token -n spaxel
   # ArgoCD will recreate it, triggering immediate sync
   ```

4. **Verify rollout (no pod restart needed):**
   ```bash
   # Secret updated in place, pods read new value on next request
   kubectl --kubeconfig=/home/coding/.kube/ardenone-cluster.kubeconfig \
     get secret spaxel-github-token -n spaxel -o jsonpath='{.data.SPAXEL_GITHUB_TOKEN}' \
     | base64 -d | head -c 8
   ```

5. **Revoke old token in GitHub:**
   - Go to GitHub Settings → Developer settings → Personal access tokens
   - Find old token, click "Delete"

**Rollback If Needed:**
```bash
echo "ghp_oldTokenValue" | \
  bao kv put secret/ardenone-cluster/spaxel/github-token token=-
```

### Emergency Rotation (Compromise Suspected)

**If credential may be leaked:**

1. **Immediate revocation in GitHub** (delete the token)
2. **Create new token** (as above)
3. **Update OpenBao** (as above)
4. **Force immediate sync:** `kubectl delete externalsecret ...`
5. **Audit access:** Check GitHub token usage logs for suspicious activity

**No Downtime:** Unauthenticated fallback keeps service alive during rotation.

## Implementation Checklist

**Phase 1: OpenBao Setup**
- [ ] Create OpenBao secret at `secret/ardenone-cluster/spaxel/github-token`
- [ ] Generate GitHub PAT with `public_repo` scope
- [ ] Write secret using stdin method (never as argument)
- [ ] Verify secret exists: `bao kv get secret/ardenone-cluster/spaxel/github-token`

**Phase 2: Kubernetes Configuration**
- [ ] Create ExternalSecret manifest in declarative-config
- [ ] Update Deployment to reference Secret
- [ ] Commit and push to declarative-config
- [ ] Verify ArgoCD sync applies changes

**Phase 3: Validation**
- [ ] Verify Secret created: `kubectl get secret spaxel-github-token -n spaxel`
- [ ] Verify env var set: `kubectl exec -n spaxel <pod> -- env | grep SPAXEL_GITHUB_TOKEN`
- [ ] Verify GitHub client uses token: Check logs for `[CONFIG] SPAXEL_GITHUB_TOKEN=ghp_...`
- [ ] Test Kaniko release fetch: Call `/api/kaniko/releases` endpoint
- [ ] Verify rate limit: Check `X-RateLimit-Limit` header (should be 5000, not 60)

**Phase 4: Documentation**
- [ ] Update operations runbook with rotation procedure
- [ ] Document OpenBao path in cluster inventory
- [ ] Add to infrastructure documentation

## Testing Strategy

**Unit Tests (Already Pass):**
- `mothership/internal/config/config_test.go` — env var loading
- `mothership/internal/github/client_test.go` — GitHub client (432 lines)

**Integration Tests (New):**
- Test with missing ExternalSecret (graceful degradation)
- Test with invalid token (detect 401, fallback to unauthenticated)
- Test with expired token (detect 401, log warning)

**Manual Tests:**
1. Deploy with ExternalSecret → verify authenticated requests work
2. Delete ExternalSecret → verify unauthenticated fallback works
3. Update OpenBao secret → verify ExternalSecret syncs within 1h
4. Force delete ExternalSecret → verify ArgoCD recreates it

## Monitoring and Alerting

**Metrics to Add:**
- `github_api_requests_total` — GitHub API request count
- `github_api_rate_limit_remaining` — Current rate limit remaining
- `github_api_unauthenticated_total` — Unauthenticated request count (should be 0)

**Alerts:**
- WARNING: `github_api_unauthenticated_total > 0` → Secret may be missing
- CRITICAL: `github_api_rate_limit_remaining < 100` → Approaching rate limit

**Health Check (Already Implemented):**
- `GET /health` includes GitHub API ping result
- Ping failure is logged as `[WARN] GitHub API ping failed: ...`

## Related Documentation

- **GitHub API Authentication Research:** `docs/research/github-api-authentication-kaniko-releases.md`
- **OpenBao Usage:** `CLAUDE.md` — "OpenBao — Agent Read/Write Access" section
- **ExternalSecrets Pattern:** `declarative-config/k8s/apexalgo-iad/news-trader/externalsecret.yaml`
- **Organization Secret Management:** See CLAUDE.md "OpenBao — Agent Read/Write Access"

## Open Questions

1. **Token Type:** Classic PAT (no expiration) vs fine-grained PAT (1-year expiration)?
   - **Recommendation:** Classic PAT for Kaniko (public repo, static requirement)

2. **Rotation Frequency:** How often to rotate GitHub PATs?
   - **Recommendation:** Annually or on compromise suspicion, whichever comes first

3. **Multi-Cluster:** Should spaxel run in other clusters?
   - **Recommendation:** No, single-cluster service (ardenone-cluster only)

## Approval

**Designed By:** Claude (agent)  
**Review Required:** Yes  
**Implementation Required:** No (this is design only)

**Next Steps:**
1. Review design with operator
2. Approve OpenBao path and structure
3. Create implementation bead if design approved
