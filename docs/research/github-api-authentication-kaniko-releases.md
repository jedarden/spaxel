# GitHub API Authentication for Kaniko Releases

**Research Date:** 2026-08-28  
**Status:** Implementation Already Exists in Spaxel

## Overview

This research documents how to authenticate with the GitHub API to fetch Kaniko releases. **Key finding:** Spaxel already has a complete, production-ready GitHub API client implementation in `mothership/internal/github/` specifically designed for fetching Kaniko releases.

## Authentication Method

### GitHub Personal Access Token (PAT) - Bearer Token

**Authentication Type:** Bearer Token (Personal Access Token)  
**Header Format:** `Authorization: Bearer <token>`

**Implementation Details:**
```go
// From mothership/internal/github/client.go
if c.token != "" {
    req.Header.Set("Authorization", "Bearer "+c.token)
}
req.Header.Set("Accept", "application/vnd.github+json")
```

### Token Configuration

**Environment Variable:** `SPAXEL_GITHUB_TOKEN`  
**Scope Required:** `public_repo` (minimum for public repositories like Kaniko)  
**Optional vs Required:** Optional but highly recommended (see rate limits below)

**How to Create a Token:**
1. Go to GitHub Settings → Developer settings → Personal access tokens → Tokens (classic)
2. Generate new token with `public_repo` scope
3. Set as environment variable: `export SPAXEL_GITHUB_TOKEN="ghp_your_token_here"`
4. For fine-grained PATs (recommended by GitHub), grant repository read permissions

### Token Types Supported

1. **Classic Personal Access Tokens** - Widely supported, simpler setup
2. **Fine-grained Personal Access Tokens** - Enhanced security, more granular permissions (recommended by GitHub as of 2026)

## Rate Limits and Restrictions

### Rate Limits (as of 2026)

| Request Type | Rate Limit | Notes |
|-------------|------------|-------|
| **Unauthenticated** | 60 requests/hour | Per IP address |
| **Authenticated (PAT)** | 5,000 requests/hour | Per token |
| **GitHub Apps (org-owned)** | 15,000 requests/hour | Not currently used in spaxel |

### Secondary Rate Limits
- REST API endpoints: Maximum 900 points per minute
- Point-based system varies by endpoint complexity

### Detection in Spaxel
```go
// Rate limit detection function
func IsRateLimited(resp *http.Response) bool {
    return resp.StatusCode == 403 && resp.Header.Get("X-RateLimit-Remaining") == "0"
}

// Extract rate limit info from headers
func GetRateLimitInfo(resp *http.Response) (remaining int, reset int64, authenticated bool)
```

**Response Headers Checked:**
- `X-RateLimit-Remaining`: Number of requests remaining in current window
- `X-RateLimit-Reset`: Unix timestamp when rate limit resets
- `X-RateLimit-Limit`: Total limit (5000 for authenticated, 60 for unauthenticated)

## GitHub API Endpoint Structure for Kaniko

### Base URLs
- **API Base URL:** `https://api.github.com`
- **API Version:** v3 REST API

### Kaniko Repository
- **Owner:** `GoogleContainerTools`
- **Repository:** `kaniko`
- **Status:** ⚠️ **ARCHIVED** (archived on June 3, 2025)
- **Latest Release:** v1.24.0 (released May 21, 2025)
- **Likely Final Release:** v1.24.0 (no further releases expected due to archival)

### Release Endpoints

#### List All Releases
```
GET https://api.github.com/repos/GoogleContainerTools/kaniko/releases
```

**Example curl command:**
```bash
# Unauthenticated (60/hour limit)
curl -H "Accept: application/vnd.github+json" \
  https://api.github.com/repos/GoogleContainerTools/kaniko/releases

# Authenticated (5000/hour limit)
curl -H "Accept: application/vnd.github+json" \
  -H "Authorization: Bearer $SPAXEL_GITHUB_TOKEN" \
  https://api.github.com/repos/GoogleContainerTools/kaniko/releases
```

#### Get Latest Release
```
GET https://api.github.com/repos/GoogleContainerTools/kaniko/releases/latest
```

**Example curl command:**
```bash
curl -H "Accept: application/vnd.github+json" \
  -H "Authorization: Bearer $SPAXEL_GITHUB_TOKEN" \
  https://api.github.com/repos/GoogleContainerTools/kaniko/releases/latest
```

#### Get Release by Tag
```
GET https://api.github.com/repos/GoogleContainerTools/kaniko/releases/tags/v1.24.0
```

### Implementation in Spaxel

**Client Methods:**
```go
// Get all releases
func (c *Client) GetReleases(ctx context.Context, owner, repo string) ([]byte, error)

// Get latest release
func (c *Client) GetLatestRelease(ctx context.Context, owner, repo string) ([]byte, error)

// Health check
func (c *Client) Ping(ctx context.Context) error
```

**Usage Example:**
```go
// From mothership/cmd/mothership/main.go
ghClient := githubclient.NewClient(cfg.GitHubToken)
if err := ghClient.Ping(ctx); err != nil {
    log.Printf("[WARN] GitHub API ping failed: %v", err)
}

// Fetch Kaniko releases
releases, err := ghClient.GetReleases(ctx, "GoogleContainerTools", "kaniko")
```

## Existing Spaxel Implementation

### File Locations

**Core Implementation:**
- `mothership/internal/github/client.go` (206 lines) - GitHub API client
- `mothership/internal/github/client_test.go` (432 lines) - Comprehensive test suite
- `mothership/internal/github/README.md` - Full documentation with examples

**Configuration:**
- `mothership/internal/config/config.go` (line 80) - `SPAXEL_GITHUB_TOKEN` environment variable
- `mothership/cmd/mothership/main.go` - Client initialization and startup ping

**Testing:**
- `scripts/test-github-api.sh` - Test script for connectivity, authentication, and data retrieval

### Features Already Implemented

✅ Bearer token authentication  
✅ Unauthenticated fallback (60/hour)  
✅ Authenticated requests (5000/hour)  
✅ Rate limit detection and handling  
✅ Kaniko-specific constants (`GoogleContainerTools/kaniko`)  
✅ Error handling for 404, rate limits, network failures  
✅ 30-second request timeout  
✅ Ping endpoint for API health checks  
✅ Comprehensive test coverage with mock server  

### HTTP Client Configuration

```go
httpClient: &http.Client{
    Timeout: 30 * time.Second,
}
```

All requests use a 30-second timeout to prevent hanging connections.

## Testing and Validation

### Test Script
```bash
./scripts/test-github-api.sh
```

**Tests:**
- GitHub API endpoint accessibility
- Authentication configuration (if token is set)
- Kaniko releases endpoint availability
- Actual data retrieval

### Manual Testing Commands

```bash
# Test unauthenticated access
curl -i https://api.github.com/repos/GoogleContainerTools/kaniko/releases/latest

# Test authenticated access
curl -i \
  -H "Authorization: Bearer $SPAXEL_GITHUB_TOKEN" \
  -H "Accept: application/vnd.github+json" \
  https://api.github.com/repos/GoogleContainerTools/kaniko/releases/latest

# Check rate limit status
curl -i https://api.github.com/rate_limit
```

## Kaniko-Specific Information

### Current Status
- **Repository:** [GoogleContainerTools/kaniko](https://github.com/GoogleContainerTools/kaniko)
- **Status:** Archived ⚠️ (June 3, 2025)
- **Latest Stable Release:** v1.24.0 (May 21, 2025)
- **Likely Final Release:** v1.24.0

### Executor Image References
- Standard: `gcr.io/kaniko-project/executor:v1.24.0`
- Debug: `gcr.io/kaniko-project/executor:debug-v1.24.0`

### Related Spaxel Research
- `docs/kaniko-version-research.md` - Kaniko v1.24.0 research
- `notes/research-kaniko-v1.24.0-stable-release.md` - Detailed v1.24.0 analysis
- `notes/bf-38dbu.md` - Kaniko TARGETPLATFORM build arg fix
- `notes/bf-38dbu-completion.md` - Kaniko cache poisoning issue

## Recommendations

### For Production Use

1. **Use Authentication:** Always set `SPAXEL_GITHUB_TOKEN` for production deployments
   - Unauthenticated: 60 requests/hour (insufficient for any meaningful operations)
   - Authenticated: 5000 requests/hour (sufficient for CI/CD and monitoring)

2. **Token Security:**
   - Store token in OpenBao: `secret/<cluster>/spaxel/github_token`
   - Never commit tokens to git
   - Use fine-grained PATs when possible (enhanced security)

3. **Rate Limit Handling:**
   - Implement exponential backoff when rate limited
   - Cache release information to reduce API calls
   - Monitor `X-RateLimit-Remaining` header

### For Kaniko Given Archive Status

Since Kaniko is archived as of June 3, 2025:
- **No further releases expected** - v1.24.0 is likely the final release
- Consider caching release information permanently
- API calls are still useful for verification and historical data
- May want to pin to v1.24.0 permanently in configuration

## Sources

### GitHub Documentation
- [Authenticating to the REST API](https://docs.github.com/rest/authentication/authenticating-to-the-rest-api)
- [Rate limits for the REST API](https://docs.github.com/rest/using-the-rest-api/rate-limits-for-the-rest-api)
- [REST API endpoints for releases](https://docs.github.com/en/rest/releases/releases)
- [REST API endpoints for release assets](https://docs.github.com/en/rest/releases/assets)

### Community Resources
- [Download release asset from private repo using PAT](https://github.com/orgs/community/discussions/47453)
- [Introducing fine-grained personal access tokens](https://github.blog/security/application-security/introducing-fine-grained-personal-access-tokens-for-github/)

### Kaniko Resources
- [Kaniko Releases Page](https://github.com/GoogleContainerTools/kaniko/releases)
- [Kaniko CHANGELOG.md](https://github.com/GoogleContainerTools/kaniko/blob/main/CHANGELOG.md)
- [Kaniko RELEASE.md](https://github.com/GoogleContainerTools/kaniko/blob/master/RELEASE.md)

## Conclusion

**The GitHub API authentication infrastructure for fetching Kaniko releases is already fully implemented in spaxel.** The implementation is production-ready with comprehensive error handling, rate limit detection, and test coverage. No additional implementation work is required for basic GitHub API authentication and Kaniko release fetching.

**Key Takeaway:** Set `SPAXEL_GITHUB_TOKEN` environment variable to enable authenticated requests (5000/hour) instead of unauthenticated requests (60/hour).
