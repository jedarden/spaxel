# GitHub API Client

Package `github` provides a GitHub API client for fetching Kaniko releases and other GitHub operations.

## Features

- **GitHub API v3 REST API client** with configurable base URL
- **Generic `Get`** for any API path, returning the raw response body
- **Token-based authentication** (Bearer token) for higher rate limits
- **Unauthenticated access** support (subject to 60 requests/hour rate limit)
- **Typed release parsing** — `Release`/`ReleaseAsset` structs decoded from
  the release endpoints, no JSON left for the caller to interpret
- **One typed error** — every failure is an `*APIError` classified by
  `ErrorKind` (HTTP, rate limit, parse, transport), branchable without
  matching message text
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
configured timeout — come back as an `*APIError` with Kind
`ErrorKindTransport` wrapping the underlying cause. Both are the same
`*APIError` type described under [Error Handling](#error-handling).

### Ping GitHub API

```go
ctx := context.Background()
err := client.Ping(ctx)
if err != nil {
    log.Printf("GitHub API ping failed: %v", err)
}
```

### Fetch Kaniko Releases

The release methods decode GitHub's payload into typed values: tag name,
title, draft/prerelease flags, publish timestamps, and the asset list with
each asset's name, download URL and size. Unknown fields in the response are
ignored, so a new field upstream does not break parsing.

```go
ctx := context.Background()
releases, err := client.GetReleases(ctx, "GoogleContainerTools", "kaniko")
if err != nil {
    log.Printf("Failed to fetch Kaniko releases: %v", err)
}
for _, r := range releases {
    for _, a := range r.Assets {
        log.Printf("%s: %s (%d bytes) -> %s", r.TagName, a.Name, a.Size, a.BrowserDownloadURL)
    }
}
```

### Fetch Latest Release

```go
ctx := context.Background()
latest, err := client.GetLatestRelease(ctx, "GoogleContainerTools", "kaniko")
if err != nil {
    log.Printf("Failed to fetch latest Kaniko release: %v", err)
}
log.Printf("latest is %s", latest.TagName)
```

A repository with no releases answers 404, which surfaces as a not-found
`*APIError` (see [Error Handling](#error-handling)).

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

### Token Refresh

Not applicable: the client authenticates with a static personal access token
read once at start-up, and GitHub provides no API to refresh a PAT — it stays
valid until it expires or is revoked, and rotating it means deploying a new
`SPAXEL_GITHUB_TOKEN` value (in Kubernetes, an ExternalSecret refresh plus a
pod restart). No refresh flow is built in, and none is needed while the
credential is a PAT. A rejected token is not retried: it surfaces as a
non-temporary `*APIError` (`IsUnauthorized`), so a caller's retry loop stops
instead of spinning on a credential that cannot recover on its own.
GitHub App installation tokens, which do expire on their own, are a different
authentication mode this client does not implement.

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
| Types | `Client`, `GitHubConfig`, `APIError`, `ErrorKind`, `RateLimit`, `Release`, `ReleaseAsset` |
| Constructors | `NewClient`, `NewClientFromConfig`, `NewGitHubConfig` |
| `Client` methods | `Config`, `Clone`, `String`, `Get`, `Ping`, `GetReleases`, `GetLatestRelease`, `SetRepoOwner`, `SetRepoName`, `SetBaseURL`, `GetRepoOwner`, `GetRepoName`, `GetBaseURL` |
| `GitHubConfig` methods | `Clone`, `WithToken`, `String` |
| `APIError` methods | `Error`, `Unwrap`, `IsNotFound`, `IsUnauthorized`, `IsRateLimited`, `Temporary` |
| `RateLimit` methods | `Exhausted`, `ResetsAt` |
| Package functions | `IsRateLimited`, `GetRateLimitInfo` |
| Constants | `GitHubAPIBaseURL`, `KanikoRepoOwner`, `KanikoRepoName`, `DefaultUserAgent`, `DefaultGitHubTimeout`, `ErrorKindHTTP`, `ErrorKindRateLimit`, `ErrorKindParse`, `ErrorKindTransport` |

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

- Go standard library `net/http`, `context`, `io`, `log`, `fmt`, `time`, `strings`, `encoding/json`
- No external dependencies required

## Kaniko Repository

The client defaults to fetching releases from:
- **Owner**: `GoogleContainerTools`
- **Repository**: `kaniko`

This can be customized using `SetRepoOwner()` and `SetRepoName()` methods.

## Error Handling

Every failing method returns an `*APIError`, whether the request never
completed, the API answered with an error status, or a 2xx body would not
decode. `Kind` classifies the failure:

| Kind | Meaning |
|------|---------|
| `ErrorKindHTTP` | the API answered with a non-2xx status |
| `ErrorKindRateLimit` | a 403 carrying `X-RateLimit-Remaining: 0` — the quota is used up |
| `ErrorKindParse` | a 2xx response whose body would not decode into the typed value |
| `ErrorKindTransport` | the failure happened before a complete response existed: connection refused, DNS failure, the configured timeout, a body read that failed partway |

Branch on the kind, or on the helpers, rather than on message text:

```go
releases, err := client.GetReleases(ctx, "GoogleContainerTools", "kaniko")
var apiErr *github.APIError
if errors.As(err, &apiErr) {
    switch {
    case apiErr.IsNotFound():
        // no such repository, or no releases
    case apiErr.IsUnauthorized():
        // the token was rejected — stop and replace it, retrying cannot help
    case apiErr.IsRateLimited():
        // back off until apiErr.RateLimit.ResetsAt()
    case apiErr.Temporary():
        // retrying later can succeed (transport failure, 429, 5xx)
    }
}
```

A rejected credential answers 401 with GitHub's `Bad credentials` message.
`IsUnauthorized()` classifies it and `Temporary()` is false for it, so a
caller that retries transient failures will not spin on an expired token. The
error text carries the status and GitHub's message but never the token value,
and neither does `Client.String()`. `Ping` deliberately treats a 401 as
"API reachable, credential invalid" and returns nil.

The error carries the method, the API path (never the base URL, so a
configured token or host is not echoed into logs), the status code, GitHub's
own `message` and `documentation_url` when the body carried them, the quota
headers, and up to 4 KiB of the response body. The underlying cause stays
reachable via `Unwrap`, so `errors.Is(err, context.DeadlineExceeded)` works
for a timed-out request. `Get` keeps returning the raw body for paths the
typed methods do not wrap; its failures are `*APIError`s too.

## Response Parsing

`GetReleases` and `GetLatestRelease` decode into `Release` and `ReleaseAsset`
(`releases.go`), covering the fields this client's consumers read: release
`id`, `tag_name`, `name`, `draft`, `prerelease`, `created_at`, `published_at`
and `html_url`, plus per asset the `id`, `name`, `browser_download_url`,
`size` and `content_type`. Parsing is strict — a 2xx body that does not
decode is an `ErrorKindParse` error carrying the offending body, rather than
silently returning zero values.

## Integration

This client is intended to be used by:
- OTA system for fetching Kaniko release information
- Update management system
- CI/CD integration

See `mothership/internal/config/config.go` for configuration integration.
