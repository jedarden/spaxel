# GitHub API Client

Package `github` provides a GitHub API client for fetching Kaniko releases and other GitHub operations.

## Features

- **GitHub API v3 REST API client** with configurable base URL
- **Token-based authentication** (Bearer token) for higher rate limits
- **Unauthenticated access** support (subject to 60 requests/hour rate limit)
- **Kaniko releases fetching** from `GoogleContainerTools/kaniko`
- **GitHub API ping** for connectivity testing
- **Rate limit detection** and information extraction

## Usage

### Basic Client Creation

```go
import "github.com/spaxel/mothership/internal/github"

// Unauthenticated client (60 requests/hour rate limit)
client := github.NewClient("")

// Authenticated client (5000 requests/hour rate limit)
client := github.NewClient("ghp_your_token_here")
```

### Ping GitHub API

```go
ctx := context.Background()
err := client.Ping(ctx)
if err != nil {
    log.Printf("GitHub API ping failed: %v", err)
}
```

### Fetch Kaniko Releases

```go
ctx := context.Background()
releases, err := client.GetReleases(ctx, "GoogleContainerTools", "kaniko")
if err != nil {
    log.Printf("Failed to fetch Kaniko releases: %v", err)
}
```

### Fetch Latest Release

```go
ctx := context.Background()
latest, err := client.GetLatestRelease(ctx, "GoogleContainerTools", "kaniko")
if err != nil {
    log.Printf("Failed to fetch latest Kaniko release: %v", err)
}
```

## Configuration

The GitHub API client is configured via the `SPAXEL_GITHUB_TOKEN` environment variable:

```bash
# Set in your shell or environment
export SPAXEL_GITHUB_TOKEN="ghp_your_token_here"

# Or in docker-compose.yml
environment:
  - SPAXEL_GITHUB_TOKEN=${GITHUB_TOKEN:-}
```

### Getting a GitHub Token

1. Go to GitHub Settings → Developer settings → Personal access tokens → Tokens (classic)
2. Generate a new token with `public_repo` scope (minimum for public repositories)
3. For private repositories, add appropriate scopes
4. Set the token as `SPAXEL_GITHUB_TOKEN`

## Rate Limits

- **Unauthenticated**: 60 requests per hour
- **Authenticated**: 5000 requests per hour

## Testing

Run the test script to verify GitHub API connectivity:

```bash
./scripts/test-github-api.sh
```

This tests:
- GitHub API endpoint accessibility
- Authentication configuration (if token is set)
- Kaniko releases endpoint availability
- Actual data retrieval

## HTTP Client Configuration

The client uses a 30-second timeout for all requests. This can be modified in the `NewClient` function if needed.

## Dependencies

- Go standard library `net/http`, `context`, `io`, `log`, `fmt`, `time`, `strings`
- No external dependencies required

## Kaniko Repository

The client defaults to fetching releases from:
- **Owner**: `GoogleContainerTools`
- **Repository**: `kaniko`

This can be customized using `SetRepoOwner()` and `SetRepoName()` methods.

## Error Handling

All methods return descriptive errors:
- Network failures
- HTTP errors (4xx, 5xx)
- Repository not found (404)
- Rate limiting (403)
- Invalid responses

## Integration

This client is intended to be used by:
- OTA system for fetching Kaniko release information
- Update management system
- CI/CD integration

See `mothership/internal/config/config.go` for configuration integration.
