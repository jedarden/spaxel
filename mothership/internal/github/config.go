package github

import (
	"fmt"
	"time"
)

// DefaultGitHubTimeout bounds each HTTP request issued under the default
// configuration. It matches the timeout NewClient has always applied.
const DefaultGitHubTimeout = 30 * time.Second

// GitHubConfig holds the configuration used to initialize a GitHub API client.
//
// It is a plain value type: every field is exported and comparable, so two
// configs can be compared with == and copied by assignment. The zero value is
// not a usable configuration — build one with NewGitHubConfig.
type GitHubConfig struct {
	// BaseURL is the GitHub REST API base URL, with no trailing slash. Point
	// it at a GitHub Enterprise instance or a test server as needed.
	BaseURL string

	// Token is an optional GitHub personal access token. An empty token means
	// requests are unauthenticated, which GitHub caps at 60/hour per source
	// IP; a token raises that to 5000/hour.
	Token string

	// RepoOwner and RepoName name the repository the client operates on by
	// default. The default configuration targets the Kaniko release feed.
	RepoOwner string
	RepoName  string

	// Timeout bounds each HTTP request issued under this configuration.
	Timeout time.Duration
}

// NewGitHubConfig returns a GitHubConfig with sensible defaults: the public
// GitHub API, the Kaniko release feed, no token (unauthenticated), and the
// default request timeout.
func NewGitHubConfig() GitHubConfig {
	return GitHubConfig{
		BaseURL:   GitHubAPIBaseURL,
		Token:     "",
		RepoOwner: KanikoRepoOwner,
		RepoName:  KanikoRepoName,
		Timeout:   DefaultGitHubTimeout,
	}
}

// Clone returns a copy of the configuration; mutating the copy does not affect
// the receiver.
func (c GitHubConfig) Clone() GitHubConfig {
	return c
}

// WithToken returns a copy of the configuration with Token set to token, which
// is normally sourced from SPAXEL_GITHUB_TOKEN. The receiver is unchanged. An
// empty token leaves the configuration unauthenticated.
func (c GitHubConfig) WithToken(token string) GitHubConfig {
	c.Token = token
	return c
}

// String renders the configuration for logs. The token value is never
// included — only whether one is configured.
func (c GitHubConfig) String() string {
	tokenState := "unset"
	if c.Token != "" {
		tokenState = "set"
	}
	return fmt.Sprintf("GitHubConfig{baseURL: %s, repo: %s/%s, timeout: %s, token: %s}",
		c.BaseURL, c.RepoOwner, c.RepoName, c.Timeout, tokenState)
}
