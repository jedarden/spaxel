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
	if clientWithToken.token != "test-token-123" {
		t.Errorf("Expected token 'test-token-123', got '%s'", clientWithToken.token)
	}

	// Test client creation without token
	clientWithoutToken := NewClient("")
	if clientWithoutToken == nil {
		t.Fatal("NewClient returned nil")
	}
	if clientWithoutToken.token != "" {
		t.Errorf("Expected empty token, got '%s'", clientWithoutToken.token)
	}

	// Verify default values
	if clientWithToken.baseURL != GitHubAPIBaseURL {
		t.Errorf("Expected base URL '%s', got '%s'", GitHubAPIBaseURL, clientWithToken.baseURL)
	}
	if clientWithToken.repoOwner != KanikoRepoOwner {
		t.Errorf("Expected repo owner '%s', got '%s'", KanikoRepoOwner, clientWithToken.repoOwner)
	}
	if clientWithToken.repoName != KanikoRepoName {
		t.Errorf("Expected repo name '%s', got '%s'", KanikoRepoName, clientWithToken.repoName)
	}
}

func TestPing(t *testing.T) {
	tests := []struct {
		name           string
		token          string
		responseStatus int
		responseBody   string
		expectError    bool
		errorContains   string
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
			errorContains:   "unexpected status 500",
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
		expectError    bool
		errorContains   string
	}{
		{
			name:           "successful releases fetch",
			token:          "test-token",
			owner:          "testowner",
			repo:           "testrepo",
			responseStatus: 200,
			responseBody:   `[{"tag_name": "v1.0.0", "name": "Release 1.0.0"}]`,
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
			errorContains:   "not found",
		},
		{
			name:           "unauthenticated rate limited",
			token:          "",
			owner:          "testowner",
			repo:           "testrepo",
			responseStatus: 200,
			responseBody:   `[{"tag_name": "v1.0.0"}]`,
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

			body, err := client.GetReleases(ctx, tt.owner, tt.repo)

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
				if body == nil {
					t.Errorf("Expected response body, got nil")
				}
			}
		})
	}
}

func TestGetLatestRelease(t *testing.T) {
	tests := []struct {
		name           string
		token          string
		owner          string
		repo           string
		responseStatus int
		responseBody   string
		expectError    bool
		errorContains   string
	}{
		{
			name:           "successful latest release fetch",
			token:          "test-token",
			owner:          "testowner",
			repo:           "testrepo",
			responseStatus: 200,
			responseBody:   `{"tag_name": "v1.2.0", "name": "Latest Release"}`,
			expectError:    false,
		},
		{
			name:           "no releases found",
			token:          "test-token",
			owner:          "testowner",
			repo:           "emptyrepo",
			responseStatus: 404,
			responseBody:   `{"message": "Not Found"}`,
			expectError:    true,
			errorContains:   "failed to fetch latest release",
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

			body, err := client.GetLatestRelease(ctx, tt.owner, tt.repo)

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
				if body == nil {
					t.Errorf("Expected response body, got nil")
				}
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
		headers map[string]string
		expected bool
	}{
		{
			name:     "rate limited (403 with zero remaining)",
			headers: map[string]string{
				"X-RateLimit-Remaining": "0",
			},
			expected: true,
		},
		{
			name:     "not rate limited (403 with requests remaining)",
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
		name           string
		headers        map[string]string
		expectedRem    int
		expectedReset   int64
		expectedAuth   bool
	}{
		{
			name: "authenticated request with full headers",
			headers: map[string]string{
				"X-RateLimit-Remaining": "4999",
				"X-RateLimit-Reset":     "1234567890",
				"X-RateLimit-Limit":      "5000",
			},
			expectedRem:  4999,
			expectedReset: 1234567890,
			expectedAuth: true,
		},
		{
			name: "unauthenticated request",
			headers: map[string]string{
				"X-RateLimit-Remaining": "59",
				"X-RateLimit-Reset":     "1234567890",
				"X-RateLimit-Limit":      "60",
			},
			expectedRem:  59,
			expectedReset: 1234567890,
			expectedAuth: false,
		},
		{
			name:           "missing headers",
			headers:        map[string]string{},
			expectedRem:    0,
			expectedReset: 0,
			expectedAuth:   false,
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
