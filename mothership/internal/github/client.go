// Package github provides a GitHub API client for fetching Kaniko releases and other GitHub operations.
// It supports both authenticated (using a token) and unauthenticated access.
package github

import (
	"context"
	"fmt"
	"io"
	"log"
	"net/http"
	"strings"
)

const (
	// GitHubAPIBaseURL is the base URL for GitHub's v3 REST API
	GitHubAPIBaseURL = "https://api.github.com"
	// KanikoRepoOwner is the GitHub username/organization that owns the Kaniko repository
	KanikoRepoOwner = "GoogleContainerTools"
	// KanikoRepoName is the repository name for Kaniko
	KanikoRepoName = "kaniko"
	// DefaultUserAgent is sent on every request. GitHub's API policy requires
	// requests to identify the calling application via User-Agent and rejects
	// those that arrive without one.
	DefaultUserAgent = "Spaxel/1.0"
)

// Client represents a GitHub API client with optional authentication.
//
// Every configurable behaviour — base URL, default repository, request
// timeout, and the authentication token — lives in config, which is the
// single source of truth. Keeping one copy rather than mirroring the
// settings onto separate fields means the setters and the request methods
// can never disagree about what the client is configured to do.
type Client struct {
	// config is the GitHubConfig this client was built from. It holds the
	// base URL, the default repository, the per-request timeout, and the
	// authentication token (empty means unauthenticated).
	config GitHubConfig

	// httpClient issues the client's requests. Its timeout mirrors
	// config.Timeout as of construction.
	httpClient *http.Client
}

// NewClient creates a new GitHub API client. If token is empty, requests will be unauthenticated
// and subject to GitHub's rate limits (60 requests/hour for unauthenticated IPs).
func NewClient(token string) *Client {
	return NewClientFromConfig(NewGitHubConfig().WithToken(token))
}

// NewClientFromConfig returns a Client configured by cfg: requests go to
// cfg.BaseURL, carry cfg.Token as a bearer credential when it is non-empty,
// are bounded by cfg.Timeout, and default to cfg.RepoOwner/cfg.RepoName.
//
// BaseURL is normalised by dropping any trailing slash, so
// "https://api.github.com" and "https://api.github.com/" build the same
// request URLs. A zero Timeout would leave requests unbounded, so it falls
// back to DefaultGitHubTimeout.
//
// This is the constructor to reach for once configuration is read from the
// environment or settings rather than hardcoded; NewClient(token) is the
// shorthand for the default configuration plus a token.
func NewClientFromConfig(cfg GitHubConfig) *Client {
	cfg.BaseURL = strings.TrimRight(cfg.BaseURL, "/")
	if cfg.Timeout <= 0 {
		cfg.Timeout = DefaultGitHubTimeout
	}
	return &Client{
		config:     cfg,
		httpClient: &http.Client{Timeout: cfg.Timeout},
	}
}

// Config returns the configuration the client was built from, including the
// authentication token. The result is a copy: mutating it does not affect the
// client. String() is the log-safe view of the same values.
func (c *Client) Config() GitHubConfig {
	return c.config
}

// Clone returns an independent copy of the client. Changes to the copy —
// including through the Set* methods — leave the receiver untouched. The HTTP
// client is rebuilt from the configuration rather than shared, so the two
// clients also have separate connection pools.
func (c *Client) Clone() *Client {
	return NewClientFromConfig(c.config.Clone())
}

// String renders the client for logs. Token redaction is delegated to
// GitHubConfig.String, which reports whether a token is set without ever
// including its value.
func (c *Client) String() string {
	return fmt.Sprintf("Client{%s}", c.config.String())
}

// newRequest builds a request for path against the configured base URL with
// the client's default headers already applied: the User-Agent GitHub's API
// policy requires, the versioned Accept header, and a bearer Authorization
// header when a token is configured. path is appended to BaseURL verbatim, so
// callers include the leading slash.
func (c *Client) newRequest(ctx context.Context, method, path string) (*http.Request, error) {
	req, err := http.NewRequestWithContext(ctx, method, c.config.BaseURL+path, nil)
	if err != nil {
		return nil, err
	}

	req.Header.Set("User-Agent", DefaultUserAgent)
	req.Header.Set("Accept", "application/vnd.github+json")

	// Add authorization header if token is available
	if c.config.Token != "" {
		req.Header.Set("Authorization", "Bearer "+c.config.Token)
	}
	return req, nil
}

// do sends req and returns the response body together with its status code.
// Transport failures — connection refused, DNS lookup failure, the configured
// timeout — come back as the error with an empty body and a zero status. A
// non-2xx status is not an error here: callers decide what each status means
// for their endpoint, and the drained body is returned so it can be included
// in whatever message they build from it.
func (c *Client) do(req *http.Request) (body []byte, status int, err error) {
	resp, err := c.httpClient.Do(req)
	if err != nil {
		return nil, 0, err
	}
	defer resp.Body.Close()

	body, err = io.ReadAll(resp.Body)
	if err != nil {
		return nil, resp.StatusCode, fmt.Errorf("failed to read response body: %w", err)
	}
	return body, resp.StatusCode, nil
}

// Get issues a GET request for path against the configured base URL and
// returns the raw response body. It is the generic request method the typed
// helpers below are built on: path is appended to BaseURL verbatim (leading
// slash included), so callers can reach endpoints those helpers do not wrap.
//
// A non-200 response is an error carrying the status code and whatever body
// the API returned. Transport failures — connection refused, DNS lookup
// failure, the configured timeout — surface as the wrapped error from the
// HTTP client.
func (c *Client) Get(ctx context.Context, path string) ([]byte, error) {
	req, err := c.newRequest(ctx, http.MethodGet, path)
	if err != nil {
		return nil, fmt.Errorf("failed to create request: %w", err)
	}

	body, status, err := c.do(req)
	if err != nil {
		return nil, fmt.Errorf("GitHub API request failed: %w", err)
	}
	if status != http.StatusOK {
		return nil, fmt.Errorf("GitHub API returned status %d: %s", status, body)
	}
	return body, nil
}

// Ping verifies GitHub API accessibility by making a lightweight authenticated request.
// It returns an error if the API is unreachable or returns an unexpected status code.
// This is a simple health check - it doesn't verify the token has any specific scopes.
func (c *Client) Ping(ctx context.Context) error {
	// Use the root endpoint which requires minimal quota
	req, err := c.newRequest(ctx, http.MethodGet, "/user")
	if err != nil {
		return fmt.Errorf("failed to create ping request: %w", err)
	}

	body, status, err := c.do(req)
	if err != nil {
		return fmt.Errorf("GitHub API ping failed: %w", err)
	}

	// We accept 200 OK (authenticated), 401 (token invalid but API reachable), or 403 (forbidden)
	// Any of these means the API endpoint is accessible
	if status != 200 && status != 401 && status != 403 {
		return fmt.Errorf("GitHub API ping returned unexpected status %d: %s", status, body)
	}

	log.Printf("[GITHUB] API ping successful (status %d)", status)
	return nil
}

// GetReleases fetches releases from a GitHub repository.
// Returns the raw JSON response body and any error encountered.
func (c *Client) GetReleases(ctx context.Context, owner, repo string) ([]byte, error) {
	req, err := c.newRequest(ctx, http.MethodGet, fmt.Sprintf("/repos/%s/%s/releases", owner, repo))
	if err != nil {
		return nil, fmt.Errorf("failed to create releases request: %w", err)
	}

	body, status, err := c.do(req)
	if err != nil {
		return nil, fmt.Errorf("failed to fetch releases: %w", err)
	}

	if status == 404 {
		return nil, fmt.Errorf("repository %s/%s not found", owner, repo)
	}
	if status != 200 {
		return nil, fmt.Errorf("GitHub API returned status %d: %s", status, body)
	}

	return body, nil
}

// GetLatestRelease fetches the latest release from a GitHub repository.
// Returns the raw JSON response body and any error encountered.
func (c *Client) GetLatestRelease(ctx context.Context, owner, repo string) ([]byte, error) {
	req, err := c.newRequest(ctx, http.MethodGet, fmt.Sprintf("/repos/%s/%s/releases/latest", owner, repo))
	if err != nil {
		return nil, fmt.Errorf("failed to create latest release request: %w", err)
	}

	body, status, err := c.do(req)
	if err != nil {
		return nil, fmt.Errorf("failed to fetch latest release: %w", err)
	}

	if status != 200 {
		return nil, fmt.Errorf("failed to fetch latest release: GitHub API returned status %d: %s", status, body)
	}

	return body, nil
}

// IsRateLimited checks if the given HTTP response indicates GitHub rate limiting.
// GitHub rate limit info is returned in headers:
// - X-RateLimit-Remaining: number of requests remaining in current window
// - X-RateLimit-Reset: Unix timestamp when rate limit resets
func IsRateLimited(resp *http.Response) bool {
	return resp.StatusCode == 403 && resp.Header.Get("X-RateLimit-Remaining") == "0"
}

// GetRateLimitInfo extracts and returns GitHub rate limit information from response headers.
// Returns remaining requests, reset timestamp (Unix), and whether authentication was used.
// A header value that fails to parse is treated as zero rather than left as a stale count.
func GetRateLimitInfo(resp *http.Response) (remaining int, reset int64, authenticated bool) {
	remainingStr := resp.Header.Get("X-RateLimit-Remaining")
	if remainingStr != "" {
		if _, err := fmt.Sscanf(remainingStr, "%d", &remaining); err != nil {
			remaining = 0
		}
	}

	resetStr := resp.Header.Get("X-RateLimit-Reset")
	if resetStr != "" {
		if _, err := fmt.Sscanf(resetStr, "%d", &reset); err != nil {
			reset = 0
		}
	}

	// Authenticated requests get 5000/hour, unauthenticated get 60/hour
	authenticated = resp.Header.Get("X-RateLimit-Limit") == "5000"

	return remaining, reset, authenticated
}

// SetRepoOwner sets a custom repository owner (useful for testing or forks).
func (c *Client) SetRepoOwner(owner string) {
	c.config.RepoOwner = owner
}

// SetRepoName sets a custom repository name (useful for testing or forks).
func (c *Client) SetRepoName(name string) {
	c.config.RepoName = name
}

// SetBaseURL sets a custom base URL (useful for testing or GitHub Enterprise).
// A trailing slash is dropped, matching NewClientFromConfig's normalisation.
func (c *Client) SetBaseURL(url string) {
	c.config.BaseURL = strings.TrimRight(url, "/")
}

// GetRepoOwner returns the current repository owner.
func (c *Client) GetRepoOwner() string {
	return c.config.RepoOwner
}

// GetRepoName returns the current repository name.
func (c *Client) GetRepoName() string {
	return c.config.RepoName
}

// GetBaseURL returns the current base URL.
func (c *Client) GetBaseURL() string {
	return c.config.BaseURL
}
