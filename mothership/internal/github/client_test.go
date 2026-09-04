// Package github tests for the GitHub API client
package github

import (
	"context"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"
)

func TestNewClient(t *testing.T) {
	// Test client creation with token
	clientWithToken := NewClient("test-token-123")
	if clientWithToken == nil {
		t.Fatal("NewClient returned nil")
	}
	if got := clientWithToken.Config().Token; got != "test-token-123" {
		t.Errorf("Expected token 'test-token-123', got '%s'", got)
	}

	// Test client creation without token
	clientWithoutToken := NewClient("")
	if clientWithoutToken == nil {
		t.Fatal("NewClient returned nil")
	}
	if got := clientWithoutToken.Config().Token; got != "" {
		t.Errorf("Expected empty token, got '%s'", got)
	}

	// Verify default values
	cfg := clientWithToken.Config()
	if cfg.BaseURL != GitHubAPIBaseURL {
		t.Errorf("Expected base URL '%s', got '%s'", GitHubAPIBaseURL, cfg.BaseURL)
	}
	if cfg.RepoOwner != KanikoRepoOwner {
		t.Errorf("Expected repo owner '%s', got '%s'", KanikoRepoOwner, cfg.RepoOwner)
	}
	if cfg.RepoName != KanikoRepoName {
		t.Errorf("Expected repo name '%s', got '%s'", KanikoRepoName, cfg.RepoName)
	}
	if cfg.Timeout != DefaultGitHubTimeout {
		t.Errorf("Expected timeout %s, got %s", DefaultGitHubTimeout, cfg.Timeout)
	}
}

func TestNewClientFromConfig(t *testing.T) {
	tests := []struct {
		name string
		cfg  GitHubConfig
		want GitHubConfig
	}{
		{
			name: "keeps a fully specified configuration",
			cfg: GitHubConfig{
				BaseURL:   "https://ghe.example.com/api/v3/",
				Token:     "ghp_test",
				RepoOwner: "someone",
				RepoName:  "somewhere",
				Timeout:   5 * time.Second,
			},
			want: GitHubConfig{
				BaseURL:   "https://ghe.example.com/api/v3",
				Token:     "ghp_test",
				RepoOwner: "someone",
				RepoName:  "somewhere",
				Timeout:   5 * time.Second,
			},
		},
		{
			name: "drops a trailing slash from the base URL",
			cfg:  NewGitHubConfig().WithToken("t"),
			want: GitHubConfig{
				BaseURL:   GitHubAPIBaseURL,
				Token:     "t",
				RepoOwner: KanikoRepoOwner,
				RepoName:  KanikoRepoName,
				Timeout:   DefaultGitHubTimeout,
			},
		},
		{
			name: "replaces a zero timeout with the default",
			cfg:  GitHubConfig{BaseURL: GitHubAPIBaseURL, Timeout: 0},
			want: GitHubConfig{
				BaseURL:   GitHubAPIBaseURL,
				Token:     "",
				RepoOwner: "",
				RepoName:  "",
				Timeout:   DefaultGitHubTimeout,
			},
		},
		{
			name: "keeps an unauthenticated client when no token is set",
			cfg:  NewGitHubConfig(),
			want: GitHubConfig{
				BaseURL:   GitHubAPIBaseURL,
				Token:     "",
				RepoOwner: KanikoRepoOwner,
				RepoName:  KanikoRepoName,
				Timeout:   DefaultGitHubTimeout,
			},
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			client := NewClientFromConfig(tt.cfg)
			if client == nil {
				t.Fatal("NewClientFromConfig returned nil")
			}
			if got := client.Config(); got != tt.want {
				t.Errorf("Config() = %+v, want %+v", got, tt.want)
			}
			if client.httpClient.Timeout != tt.want.Timeout {
				t.Errorf("httpClient.Timeout = %s, want %s", client.httpClient.Timeout, tt.want.Timeout)
			}
		})
	}
}

func TestNewClientFromConfigTrailingSlashRequestPath(t *testing.T) {
	// A trailing slash on the base URL must not produce "//repos/..." request
	// URLs, which some servers treat as a different route.
	var gotPath string
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		gotPath = r.URL.Path
		// The typed release method decodes the body it is handed, so the stub
		// has to answer with something decodable.
		_, _ = w.Write([]byte(`{}`))
	}))
	defer srv.Close()

	client := NewClientFromConfig(GitHubConfig{BaseURL: srv.URL + "/", Timeout: time.Second})
	if _, err := client.GetLatestRelease(context.Background(), "o", "r"); err != nil {
		t.Fatalf("GetLatestRelease returned error: %v", err)
	}
	if gotPath != "/repos/o/r/releases/latest" {
		t.Errorf("request path = %q, want %q", gotPath, "/repos/o/r/releases/latest")
	}
}

func TestClientConfigReturnsCopy(t *testing.T) {
	client := NewClient("keep-me")

	cfg := client.Config()
	cfg.Token = "mutated"
	cfg.RepoName = "mutated"

	if got := client.Config(); got.Token != "keep-me" || got.RepoName != KanikoRepoName {
		t.Errorf("Config() mutation leaked into the client: %+v", got)
	}
}

func TestClientClone(t *testing.T) {
	orig := NewClientFromConfig(GitHubConfig{
		BaseURL:   GitHubAPIBaseURL,
		Token:     "tok",
		RepoOwner: KanikoRepoOwner,
		RepoName:  KanikoRepoName,
		Timeout:   DefaultGitHubTimeout,
	})

	clone := orig.Clone()
	if clone == orig {
		t.Fatal("Clone returned the same pointer")
	}
	if clone.Config() != orig.Config() {
		t.Errorf("Clone() config = %+v, want %+v", clone.Config(), orig.Config())
	}
	if clone.httpClient == orig.httpClient {
		t.Error("Clone shares the receiver's http.Client")
	}

	// Mutating the clone must not affect the receiver.
	clone.SetRepoName("other-repo")
	clone.SetBaseURL("https://ghe.example.com/")
	if got := orig.GetRepoName(); got != KanikoRepoName {
		t.Errorf("receiver RepoName changed to %q after clone mutation", got)
	}
	if got := orig.GetBaseURL(); got != GitHubAPIBaseURL {
		t.Errorf("receiver BaseURL changed to %q after clone mutation", got)
	}
}

func TestClientString(t *testing.T) {
	secret := "ghp_super_secret_token_value"

	t.Run("redacts the token value", func(t *testing.T) {
		got := NewClient(secret).String()
		if strings.Contains(got, secret) {
			t.Errorf("String() leaked the token: %q", got)
		}
		if !strings.Contains(got, "token: set") {
			t.Errorf("String() did not report the token state: %q", got)
		}
	})

	t.Run("reports an unauthenticated client", func(t *testing.T) {
		got := NewClient("").String()
		if strings.Contains(got, "token: set") {
			t.Errorf("String() reported a token for an unauthenticated client: %q", got)
		}
	})

	t.Run("identifies the target repository", func(t *testing.T) {
		got := NewClient("").String()
		if !strings.Contains(got, KanikoRepoOwner+"/"+KanikoRepoName) {
			t.Errorf("String() = %q, want it to name %s/%s", got, KanikoRepoOwner, KanikoRepoName)
		}
	})
}

func TestPing(t *testing.T) {
	tests := []struct {
		name           string
		token          string
		responseStatus int
		responseBody   string
		expectError    bool
		errorContains  string
	}{
		{
			name:           "successful authenticated ping",
			token:          "valid-token",
			responseStatus: 200,
			responseBody:   `{"login": "testuser"}`,
			expectError:    false,
		},
		{
			name:           "invalid token but API reachable",
			token:          "invalid-token",
			responseStatus: 401,
			responseBody:   `{"message": "Bad credentials"}`,
			expectError:    false,
		},
		{
			name:           "unauthenticated ping allowed",
			token:          "",
			responseStatus: 200,
			responseBody:   `{"login": ""}`,
			expectError:    false,
		},
		{
			name:           "API error",
			token:          "test-token",
			responseStatus: 500,
			responseBody:   `{"message": "Internal server error"}`,
			expectError:    true,
			errorContains:  "unexpected status 500",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			// Create test server
			server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				// Verify authorization header if token is provided
				if tt.token != "" {
					auth := r.Header.Get("Authorization")
					if auth != "Bearer "+tt.token && tt.responseStatus != 401 {
						t.Errorf("Expected Authorization header 'Bearer %s', got '%s'", tt.token, auth)
					}
				}

				// Check Accept header
				accept := r.Header.Get("Accept")
				if !strings.Contains(accept, "application/vnd.github+json") {
					t.Errorf("Expected Accept header to contain 'application/vnd.github+json', got '%s'", accept)
				}

				w.WriteHeader(tt.responseStatus)
				w.Write([]byte(tt.responseBody))
			}))
			defer server.Close()

			// Create client with test base URL
			client := NewClient(tt.token)
			client.SetBaseURL(server.URL)

			// Test ping
			ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
			defer cancel()

			err := client.Ping(ctx)

			if tt.expectError {
				if err == nil {
					t.Errorf("Expected error containing '%s', but got nil", tt.errorContains)
				}
				if !strings.Contains(err.Error(), tt.errorContains) {
					t.Errorf("Expected error to contain '%s', got '%s'", tt.errorContains, err.Error())
				}
			} else {
				if err != nil {
					t.Errorf("Expected no error, got: %v", err)
				}
			}
		})
	}
}

func TestGetReleases(t *testing.T) {
	tests := []struct {
		name           string
		token          string
		owner          string
		repo           string
		responseStatus int
		responseBody   string
		wantReleases   int
		wantTagName    string
		wantAssets     int
		wantAssetName  string
		wantAssetSize  int64
		expectError    bool
		errorContains  string
	}{
		{
			name:           "successful releases fetch",
			token:          "test-token",
			owner:          "testowner",
			repo:           "testrepo",
			responseStatus: 200,
			responseBody: `[{"id": 101, "tag_name": "v1.0.0", "name": "Release 1.0.0", "draft": false, "prerelease": false,
				"assets": [{"id": 7, "name": "releases_1.0.0.yaml", "browser_download_url": "https://example.com/a.yaml", "size": 2048, "content_type": "application/yaml"}]}]`,
			wantReleases:  1,
			wantTagName:   "v1.0.0",
			wantAssets:    1,
			wantAssetName: "releases_1.0.0.yaml",
			wantAssetSize: 2048,
			expectError:   false,
		},
		{
			name:           "unknown fields are ignored",
			token:          "test-token",
			owner:          "testowner",
			repo:           "testrepo",
			responseStatus: 200,
			responseBody:   `[{"tag_name": "v2.0.0", "some_future_field": {"nested": true}}]`,
			wantReleases:   1,
			wantTagName:    "v2.0.0",
			expectError:    false,
		},
		{
			name:           "empty release list",
			token:          "test-token",
			owner:          "testowner",
			repo:           "testrepo",
			responseStatus: 200,
			responseBody:   `[]`,
			wantReleases:   0,
			expectError:    false,
		},
		{
			name:           "repository not found",
			token:          "test-token",
			owner:          "testowner",
			repo:           "nonexistent",
			responseStatus: 404,
			responseBody:   `{"message": "Not Found"}`,
			expectError:    true,
			errorContains:  "not found",
		},
		{
			name:           "unparsable body is a parse error",
			token:          "test-token",
			owner:          "testowner",
			repo:           "testrepo",
			responseStatus: 200,
			responseBody:   `<html>gateway error</html>`,
			expectError:    true,
			errorContains:  "could not be parsed",
		},
		{
			name:           "unauthenticated rate limited",
			token:          "",
			owner:          "testowner",
			repo:           "testrepo",
			responseStatus: 200,
			responseBody:   `[{"tag_name": "v1.0.0"}]`,
			wantReleases:   1,
			wantTagName:    "v1.0.0",
			expectError:    false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			// Create test server
			server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				// Verify request path
				expectedPath := "/repos/" + tt.owner + "/" + tt.repo + "/releases"
				if r.URL.Path != expectedPath {
					t.Errorf("Expected path '%s', got '%s'", expectedPath, r.URL.Path)
				}

				// Check authorization header if token is provided
				if tt.token != "" {
					auth := r.Header.Get("Authorization")
					expectedAuth := "Bearer " + tt.token
					if auth != expectedAuth {
						t.Errorf("Expected Authorization header '%s', got '%s'", expectedAuth, auth)
					}
				}

				w.WriteHeader(tt.responseStatus)
				w.Write([]byte(tt.responseBody))
			}))
			defer server.Close()

			// Create client with test base URL
			client := NewClient(tt.token)
			client.SetBaseURL(server.URL)

			// Test GetReleases
			ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
			defer cancel()

			releases, err := client.GetReleases(ctx, tt.owner, tt.repo)

			if tt.expectError {
				if err == nil {
					t.Errorf("Expected error containing '%s', but got nil", tt.errorContains)
				}
				if !strings.Contains(err.Error(), tt.errorContains) {
					t.Errorf("Expected error to contain '%s', got '%s'", tt.errorContains, err.Error())
				}
				return
			}
			if err != nil {
				t.Fatalf("Expected no error, got: %v", err)
			}
			if len(releases) != tt.wantReleases {
				t.Fatalf("len(releases) = %d, want %d", len(releases), tt.wantReleases)
			}
			if tt.wantReleases == 0 {
				return
			}
			if tt.wantTagName != "" && releases[0].TagName != tt.wantTagName {
				t.Errorf("TagName = %q, want %q", releases[0].TagName, tt.wantTagName)
			}
			if len(releases[0].Assets) != tt.wantAssets {
				t.Fatalf("len(assets) = %d, want %d", len(releases[0].Assets), tt.wantAssets)
			}
			if tt.wantAssets == 0 {
				return
			}
			asset := releases[0].Assets[0]
			if asset.Name != tt.wantAssetName {
				t.Errorf("asset.Name = %q, want %q", asset.Name, tt.wantAssetName)
			}
			if asset.Size != tt.wantAssetSize {
				t.Errorf("asset.Size = %d, want %d", asset.Size, tt.wantAssetSize)
			}
			if asset.BrowserDownloadURL == "" {
				t.Error("asset.BrowserDownloadURL is empty, want the download URL")
			}
		})
	}
}

func TestGetLatestRelease(t *testing.T) {
	tests := []struct {
		name             string
		token            string
		owner            string
		repo             string
		responseStatus   int
		responseBody     string
		wantTag          string
		wantName         string
		wantPrerelease   bool
		wantPublishField string
		expectError      bool
		errorContains    string
	}{
		{
			name:             "successful latest release fetch",
			token:            "test-token",
			owner:            "testowner",
			repo:             "testrepo",
			responseStatus:   200,
			responseBody:     `{"id": 5, "tag_name": "v1.2.0", "name": "Latest Release", "draft": false, "prerelease": true, "published_at": "2026-01-02T03:04:05Z", "html_url": "https://github.com/o/r/releases/v1.2.0"}`,
			wantTag:          "v1.2.0",
			wantName:         "Latest Release",
			wantPrerelease:   true,
			wantPublishField: "2026-01-02T03:04:05Z",
			expectError:      false,
		},
		{
			name:           "no releases found",
			token:          "test-token",
			owner:          "testowner",
			repo:           "emptyrepo",
			responseStatus: 404,
			responseBody:   `{"message": "Not Found"}`,
			expectError:    true,
			errorContains:  "failed to fetch latest release",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			// Create test server
			server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				// Verify request path
				expectedPath := "/repos/" + tt.owner + "/" + tt.repo + "/releases/latest"
				if r.URL.Path != expectedPath {
					t.Errorf("Expected path '%s', got '%s'", expectedPath, r.URL.Path)
				}

				w.WriteHeader(tt.responseStatus)
				w.Write([]byte(tt.responseBody))
			}))
			defer server.Close()

			// Create client with test base URL
			client := NewClient(tt.token)
			client.SetBaseURL(server.URL)

			// Test GetLatestRelease
			ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
			defer cancel()

			release, err := client.GetLatestRelease(ctx, tt.owner, tt.repo)

			if tt.expectError {
				if err == nil {
					t.Errorf("Expected error containing '%s', but got nil", tt.errorContains)
				}
				if !strings.Contains(err.Error(), tt.errorContains) {
					t.Errorf("Expected error to contain '%s', got '%s'", tt.errorContains, err.Error())
				}
				return
			}
			if err != nil {
				t.Fatalf("Expected no error, got: %v", err)
			}
			if release == nil {
				t.Fatal("Expected release, got nil")
			}
			if release.TagName != tt.wantTag {
				t.Errorf("TagName = %q, want %q", release.TagName, tt.wantTag)
			}
			if release.Name != tt.wantName {
				t.Errorf("Name = %q, want %q", release.Name, tt.wantName)
			}
			if release.Prerelease != tt.wantPrerelease {
				t.Errorf("Prerelease = %v, want %v", release.Prerelease, tt.wantPrerelease)
			}
			if release.PublishedAt != tt.wantPublishField {
				t.Errorf("PublishedAt = %q, want %q", release.PublishedAt, tt.wantPublishField)
			}
		})
	}
}

func TestSettersAndGetters(t *testing.T) {
	client := NewClient("test-token")

	// Test SetRepoOwner
	client.SetRepoOwner("custom-owner")
	if owner := client.GetRepoOwner(); owner != "custom-owner" {
		t.Errorf("Expected repo owner 'custom-owner', got '%s'", owner)
	}

	// Test SetRepoName
	client.SetRepoName("custom-repo")
	if name := client.GetRepoName(); name != "custom-repo" {
		t.Errorf("Expected repo name 'custom-repo', got '%s'", name)
	}

	// Test SetBaseURL
	client.SetBaseURL("https://custom.github.com")
	if url := client.GetBaseURL(); url != "https://custom.github.com" {
		t.Errorf("Expected base URL 'https://custom.github.com', got '%s'", url)
	}
}

func TestIsRateLimited(t *testing.T) {
	tests := []struct {
		name     string
		headers  map[string]string
		expected bool
	}{
		{
			name: "rate limited (403 with zero remaining)",
			headers: map[string]string{
				"X-RateLimit-Remaining": "0",
			},
			expected: true,
		},
		{
			name: "not rate limited (403 with requests remaining)",
			headers: map[string]string{
				"X-RateLimit-Remaining": "100",
			},
			expected: false,
		},
		{
			name:     "not rate limited (200 status)",
			headers:  map[string]string{},
			expected: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			resp := &http.Response{
				StatusCode: 403,
				Header:     http.Header{},
			}
			for k, v := range tt.headers {
				resp.Header.Set(k, v)
			}

			result := IsRateLimited(resp)
			if result != tt.expected {
				t.Errorf("Expected rate limited=%v, got %v", tt.expected, result)
			}
		})
	}
}

func TestGetRateLimitInfo(t *testing.T) {
	tests := []struct {
		name          string
		headers       map[string]string
		expectedRem   int
		expectedReset int64
		expectedAuth  bool
	}{
		{
			name: "authenticated request with full headers",
			headers: map[string]string{
				"X-RateLimit-Remaining": "4999",
				"X-RateLimit-Reset":     "1234567890",
				"X-RateLimit-Limit":     "5000",
			},
			expectedRem:   4999,
			expectedReset: 1234567890,
			expectedAuth:  true,
		},
		{
			name: "unauthenticated request",
			headers: map[string]string{
				"X-RateLimit-Remaining": "59",
				"X-RateLimit-Reset":     "1234567890",
				"X-RateLimit-Limit":     "60",
			},
			expectedRem:   59,
			expectedReset: 1234567890,
			expectedAuth:  false,
		},
		{
			name:          "missing headers",
			headers:       map[string]string{},
			expectedRem:   0,
			expectedReset: 0,
			expectedAuth:  false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			resp := &http.Response{
				Header: http.Header{},
			}
			for k, v := range tt.headers {
				resp.Header.Set(k, v)
			}

			remaining, reset, authenticated := GetRateLimitInfo(resp)

			if remaining != tt.expectedRem {
				t.Errorf("Expected remaining=%d, got %d", tt.expectedRem, remaining)
			}
			if reset != tt.expectedReset {
				t.Errorf("Expected reset=%d, got %d", tt.expectedReset, reset)
			}
			if authenticated != tt.expectedAuth {
				t.Errorf("Expected authenticated=%v, got %v", tt.expectedAuth, authenticated)
			}
		})
	}
}

func TestGetReturnsRawBody(t *testing.T) {
	const payload = `{"login":"octocat","id":1}`

	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		w.Write([]byte(payload))
	}))
	defer srv.Close()

	client := NewClientFromConfig(GitHubConfig{BaseURL: srv.URL, Timeout: time.Second})
	got, err := client.Get(context.Background(), "/users/octocat")
	if err != nil {
		t.Fatalf("Get returned error: %v", err)
	}
	if string(got) != payload {
		t.Errorf("Get body = %q, want %q", got, payload)
	}
}

func TestGetJoinsConfiguredBaseURL(t *testing.T) {
	var gotPath, gotRawQuery string
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		gotPath, gotRawQuery = r.URL.Path, r.URL.RawQuery
		w.WriteHeader(http.StatusOK)
	}))
	defer srv.Close()

	// The trailing slash exercises the same normalisation the typed
	// constructors document; the path must still join onto the base URL.
	client := NewClientFromConfig(GitHubConfig{BaseURL: srv.URL + "/", Timeout: time.Second})
	if _, err := client.Get(context.Background(), "/rate_limit?resource=core"); err != nil {
		t.Fatalf("Get returned error: %v", err)
	}
	if gotPath != "/rate_limit" {
		t.Errorf("request path = %q, want %q", gotPath, "/rate_limit")
	}
	if gotRawQuery != "resource=core" {
		t.Errorf("request query = %q, want %q", gotRawQuery, "resource=core")
	}
}

// TestDefaultHeadersOnEveryRequest asserts the headers GitHub's API policy
// expects are applied by the shared request builder rather than per method, so
// a method added later cannot silently ship without them.
func TestDefaultHeadersOnEveryRequest(t *testing.T) {
	requests := map[string]func(*Client) error{
		"Get":              func(c *Client) error { _, err := c.Get(context.Background(), "/user"); return err },
		"Ping":             func(c *Client) error { return c.Ping(context.Background()) },
		"GetReleases":      func(c *Client) error { _, err := c.GetReleases(context.Background(), "o", "r"); return err },
		"GetLatestRelease": func(c *Client) error { _, err := c.GetLatestRelease(context.Background(), "o", "r"); return err },
	}

	tests := []struct {
		name     string
		token    string
		wantAuth string
	}{
		{name: "unauthenticated client omits Authorization", token: "", wantAuth: ""},
		{name: "token client sends bearer credentials", token: "tok-1", wantAuth: "Bearer tok-1"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			for method, call := range requests {
				t.Run(method, func(t *testing.T) {
					var gotUA, gotAccept, gotAuth string
					srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
						gotUA = r.Header.Get("User-Agent")
						gotAccept = r.Header.Get("Accept")
						gotAuth = r.Header.Get("Authorization")
						// The typed release methods decode what they are handed,
						// so each endpoint the map reaches answers with its own
						// decodable shape.
						switch r.URL.Path {
						case "/repos/o/r/releases":
							_, _ = w.Write([]byte(`[]`))
						default:
							_, _ = w.Write([]byte(`{}`))
						}
					}))
					defer srv.Close()

					client := NewClientFromConfig(GitHubConfig{BaseURL: srv.URL, Token: tt.token, Timeout: time.Second})
					if err := call(client); err != nil {
						t.Fatalf("%s returned error: %v", method, err)
					}

					if gotUA != DefaultUserAgent {
						t.Errorf("User-Agent = %q, want %q", gotUA, DefaultUserAgent)
					}
					if !strings.Contains(gotAccept, "application/vnd.github+json") {
						t.Errorf("Accept = %q, want it to contain %q", gotAccept, "application/vnd.github+json")
					}
					if gotAuth != tt.wantAuth {
						t.Errorf("Authorization = %q, want %q", gotAuth, tt.wantAuth)
					}
				})
			}
		})
	}
}

func TestGetReportsErrorStatus(t *testing.T) {
	tests := []struct {
		name       string
		status     int
		body       string
		wantStatus string
		wantBody   string
	}{
		{
			name:       "not found",
			status:     http.StatusNotFound,
			body:       `{"message":"Not Found"}`,
			wantStatus: "404",
			wantBody:   "Not Found",
		},
		{
			name:       "server error",
			status:     http.StatusInternalServerError,
			body:       "boom",
			wantStatus: "500",
			wantBody:   "boom",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
				w.WriteHeader(tt.status)
				w.Write([]byte(tt.body))
			}))
			defer srv.Close()

			client := NewClientFromConfig(GitHubConfig{BaseURL: srv.URL, Timeout: time.Second})
			got, err := client.Get(context.Background(), "/repos/o/r")
			if err == nil {
				t.Fatalf("Get succeeded with body %q, want an error for status %d", got, tt.status)
			}
			if !strings.Contains(err.Error(), tt.wantStatus) {
				t.Errorf("error %q does not mention status %s", err, tt.wantStatus)
			}
			if !strings.Contains(err.Error(), tt.wantBody) {
				t.Errorf("error %q does not include the response body %q", err, tt.wantBody)
			}
		})
	}
}

// TestGetTransportErrors covers the failures that happen before a response
// exists: nothing is listening, and the request outlives the configured
// timeout. Both must come back as errors rather than panics or empty bodies.
func TestGetTransportErrors(t *testing.T) {
	t.Run("connection refused", func(t *testing.T) {
		srv := httptest.NewServer(http.HandlerFunc(func(http.ResponseWriter, *http.Request) {}))
		url := srv.URL
		srv.Close() // nothing is listening on url any more

		client := NewClientFromConfig(GitHubConfig{BaseURL: url, Timeout: time.Second})
		body, err := client.Get(context.Background(), "/user")
		if err == nil {
			t.Fatal("Get against a closed server succeeded, want an error")
		}
		if body != nil {
			t.Errorf("body = %q on a transport failure, want nil", body)
		}
	})

	t.Run("timeout", func(t *testing.T) {
		srv := httptest.NewServer(http.HandlerFunc(func(_ http.ResponseWriter, _ *http.Request) {
			time.Sleep(500 * time.Millisecond)
		}))
		defer srv.Close()

		client := NewClientFromConfig(GitHubConfig{BaseURL: srv.URL, Timeout: 50 * time.Millisecond})
		if _, err := client.Get(context.Background(), "/slow"); err == nil {
			t.Fatal("Get past the client timeout succeeded, want an error")
		} else if !strings.Contains(err.Error(), "Client.Timeout") {
			t.Errorf("error %q does not identify the client timeout", err)
		}
	})
}
