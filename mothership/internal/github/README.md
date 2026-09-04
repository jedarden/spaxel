# GitHub API Client

Package `github` provides a GitHub API client for fetching Kaniko releases and other GitHub operations.

## Features

- **GitHub API v3 REST API client** with configurable base URL
- **Generic `Get`** for any API path, returning the raw response body
- **Token-based authentication** (Bearer token) for higher rate limits
- **Unauthenticated access** support (subject to 60 requests/hour rate limit)
- **Kaniko releases fetching** from `GoogleContainerTools/kaniko`
- **GitHub API ping** for connectivity testing
- **Rate limit detection** and information extraction
- **Default headers on every request** — a `User-Agent` identifying the
  application (`DefaultUserAgent`), which GitHub's API policy requires and
  without which requests are rejected

## Usage

### Basic Client Creation

```go
import "github.com/spaxel/mothership/internal/github"

// Unauthenticated client (60 requests/hour rate limit)
client := github.NewClient("")

// Authenticated client (5000 requests/hour rate limit)
client := github.NewClient("ghp_your_token_here")

// Fully configured client (base URL, token, default repo, timeout)
cfg := github.NewGitHubConfig().WithToken(os.Getenv("SPAXEL_GITHUB_TOKEN"))
client := github.NewClientFromConfig(cfg)
```

`NewClient(token)` is shorthand for the default configuration plus a token.
`NewClientFromConfig(GitHubConfig)` takes the full configuration and is the
constructor to use once settings come from the environment or the settings
store. A trailing slash on `BaseURL` is dropped, and a zero `Timeout` falls
back to the 30-second default.

### Fetch Any API Path

`Get` is the generic request method the typed helpers are built on. The path is
appended to the configured base URL verbatim (include the leading slash), and
the raw response body comes back as `[]byte`:

```go
ctx := context.Background()
body, err := client.Get(ctx, "/rate_limit")
if err != nil {
    log.Printf("rate limit lookup failed: %v", err)
}
```

A non-200 response is an error carrying the status code and the body the API
returned. Transport failures — connection refused, DNS lookup failure, the
configured timeout — surface as the wrapped error from the HTTP client.

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

## Public API Surface

Everything a consumer outside the package may reach is exported, and pinned by
`client_external_test.go`, which imports the package as an external consumer
would (`package github_test`) and fails to compile if any of these stops being
reachable:

| Kind | Exported identifiers |
|------|----------------------|
| Types | `Client`, `GitHubConfig` |
| Constructors | `NewClient`, `NewClientFromConfig`, `NewGitHubConfig` |
| `Client` methods | `Config`, `Clone`, `String`, `Get`, `Ping`, `GetReleases`, `GetLatestRelease`, `SetRepoOwner`, `SetRepoName`, `SetBaseURL`, `GetRepoOwner`, `GetRepoName`, `GetBaseURL` |
| `GitHubConfig` methods | `Clone`, `WithToken`, `String` |
| Package functions | `IsRateLimited`, `GetRateLimitInfo` |
| Constants | `GitHubAPIBaseURL`, `KanikoRepoOwner`, `KanikoRepoName`, `DefaultUserAgent`, `DefaultGitHubTimeout` |

`Client`'s own fields stay unexported — configuration is read back through
`Config()`, which returns a copy, so a caller cannot mutate a live client.

The import path is `github.com/spaxel/mothership/internal/github`. The
`internal/` element is Go's module privacy boundary: the package is reachable
from anywhere inside the `mothership` module (as `cmd/mothership` does), and
not from a different Go module. That matches every other package in this
module; it is what "exported" means here, and is why the boundary is asserted
by an external-package test rather than by relocating the package.

## HTTP Client Configuration

The client uses a 30-second timeout for all requests (`DefaultGitHubTimeout`).
Set `GitHubConfig.Timeout` and build with `NewClientFromConfig` to change it.
`client.String()` renders the configuration for logs and never includes the
token value, so a client is safe to log.

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
