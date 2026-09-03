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
	"time"
)

const (
	// GitHubAPIBaseURL is the base URL for GitHub's v3 REST API
	GitHubAPIBaseURL = "https://api.github.com"
	// KanikoRepoOwner is the GitHub username/organization that owns the Kaniko repository
	KanikoRepoOwner = "GoogleContainerTools"
	// KanikoRepoName is the repository name for Kaniko
	KanikoRepoName = "kaniko"
)

// Client represents a GitHub API client with optional authentication.
type Client struct {
	httpClient *http.Client
	token      string
	baseURL    string
	repoOwner  string
	repoName   string
}

// NewClient creates a new GitHub API client. If token is empty, requests will be unauthenticated
// and subject to GitHub's rate limits (60 requests/hour for unauthenticated IPs).
func NewClient(token string) *Client {
	return &Client{
		httpClient: &http.Client{
			Timeout: 30 * time.Second,
		},
		token:     token,
		baseURL:   GitHubAPIBaseURL,
		repoOwner: KanikoRepoOwner,
		repoName:  KanikoRepoName,
	}
}

// Ping verifies GitHub API accessibility by making a lightweight authenticated request.
// It returns an error if the API is unreachable or returns an unexpected status code.
// This is a simple health check - it doesn't verify the token has any specific scopes.
func (c *Client) Ping(ctx context.Context) error {
	// Use the root endpoint which requires minimal quota
	req, err := http.NewRequestWithContext(ctx, "GET", c.baseURL+"/user", nil)
	if err != nil {
		return fmt.Errorf("failed to create ping request: %w", err)
	}

	// Add authorization header if token is available
	if c.token != "" {
		req.Header.Set("Authorization", "Bearer "+c.token)
	}
	req.Header.Set("Accept", "application/vnd.github+json")

	resp, err := c.httpClient.Do(req)
	if err != nil {
		return fmt.Errorf("GitHub API ping failed: %w", err)
	}
	defer resp.Body.Close()

	// We accept 200 OK (authenticated), 401 (token invalid but API reachable), or 403 (forbidden)
	// Any of these means the API endpoint is accessible
	if resp.StatusCode != 200 && resp.StatusCode != 401 && resp.StatusCode != 403 {
		body, _ := io.ReadAll(resp.Body)
		return fmt.Errorf("GitHub API ping returned unexpected status %d: %s", resp.StatusCode, string(body))
	}

	log.Printf("[GITHUB] API ping successful (status %d)", resp.StatusCode)
	return nil
}

// GetReleases fetches releases from a GitHub repository.
// Returns the raw JSON response body and any error encountered.
func (c *Client) GetReleases(ctx context.Context, owner, repo string) ([]byte, error) {
	url := fmt.Sprintf("%s/repos/%s/%s/releases", c.baseURL, owner, repo)

	req, err := http.NewRequestWithContext(ctx, "GET", url, nil)
	if err != nil {
		return nil, fmt.Errorf("failed to create releases request: %w", err)
	}

	if c.token != "" {
		req.Header.Set("Authorization", "Bearer "+c.token)
	}
	req.Header.Set("Accept", "application/vnd.github+json")

	resp, err := c.httpClient.Do(req)
	if err != nil {
		return nil, fmt.Errorf("failed to fetch releases: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode == 404 {
		return nil, fmt.Errorf("repository %s/%s not found", owner, repo)
	}

	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, fmt.Errorf("failed to read response body: %w", err)
	}

	if resp.StatusCode != 200 {
		return nil, fmt.Errorf("GitHub API returned status %d: %s", resp.StatusCode, string(body))
	}

	return body, nil
}

// GetLatestRelease fetches the latest release from a GitHub repository.
// Returns the raw JSON response body and any error encountered.
func (c *Client) GetLatestRelease(ctx context.Context, owner, repo string) ([]byte, error) {
	url := fmt.Sprintf("%s/repos/%s/%s/releases/latest", c.baseURL, owner, repo)

	req, err := http.NewRequestWithContext(ctx, "GET", url, nil)
	if err != nil {
		return nil, fmt.Errorf("failed to create latest release request: %w", err)
	}

	if c.token != "" {
		req.Header.Set("Authorization", "Bearer "+c.token)
	}
	req.Header.Set("Accept", "application/vnd.github+json")

	resp, err := c.httpClient.Do(req)
	if err != nil {
		return nil, fmt.Errorf("failed to fetch latest release: %w", err)
	}
	defer resp.Body.Close()

	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, fmt.Errorf("failed to read response body: %w", err)
	}

	if resp.StatusCode != 200 {
		return nil, fmt.Errorf("failed to fetch latest release: GitHub API returned status %d: %s", resp.StatusCode, string(body))
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
	c.repoOwner = owner
}

// SetRepoName sets a custom repository name (useful for testing or forks).
func (c *Client) SetRepoName(name string) {
	c.repoName = name
}

// SetBaseURL sets a custom base URL (useful for testing or GitHub Enterprise).
func (c *Client) SetBaseURL(url string) {
	c.baseURL = strings.TrimRight(url, "/")
}

// GetRepoOwner returns the current repository owner.
func (c *Client) GetRepoOwner() string {
	return c.repoOwner
}

// GetRepoName returns the current repository name.
func (c *Client) GetRepoName() string {
	return c.repoName
}

// GetBaseURL returns the current base URL.
func (c *Client) GetBaseURL() string {
	return c.baseURL
}
