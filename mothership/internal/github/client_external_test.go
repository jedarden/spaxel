// Package github_test exercises the github package the way every consumer
// outside the package must: through the import path and the exported API only.
//
// The tests in package github share the package's internals and therefore
// cannot fail when an identifier stops being exported. These tests can — an
// unexported, renamed or removed symbol here is a compile error, which is the
// property the public API surface is actually promising.
package github_test

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/spaxel/mothership/internal/github"
)

// The public surface an external consumer may reach, asserted at compile
// time. Each line is a type-correct use of an exported symbol; the block
// fails to build if any of them is unexported or changes shape.
var (
	_ github.GitHubConfig                     = github.NewGitHubConfig()
	_ github.GitHubConfig                     = github.NewGitHubConfig().WithToken("")
	_ github.GitHubConfig                     = github.NewGitHubConfig().Clone()
	_ string                                  = github.NewGitHubConfig().String()
	_ *github.Client                          = github.NewClient("")
	_ *github.Client                          = github.NewClientFromConfig(github.NewGitHubConfig())
	_ github.GitHubConfig                     = github.NewClient("").Config()
	_ *github.Client                          = github.NewClient("").Clone()
	_ string                                  = github.NewClient("").String()
	_ time.Duration                           = github.DefaultGitHubTimeout
	_ string                                  = github.GitHubAPIBaseURL
	_ string                                  = github.KanikoRepoOwner
	_ string                                  = github.KanikoRepoName
	_ string                                  = github.DefaultUserAgent
	_ func(*http.Response) bool               = github.IsRateLimited
	_ func(*http.Response) (int, int64, bool) = github.GetRateLimitInfo
	_ *github.APIError                        = &github.APIError{}
	_ github.ErrorKind                        = github.ErrorKindHTTP
	_ github.Release                          = github.Release{TagName: "v1"}
	_ github.ReleaseAsset                     = github.ReleaseAsset{Name: "a"}
)

// TestExportedMethodSurface pins the exported method set of both types. A
// method that disappears or becomes unexported is a build failure here, at
// the point of use of an external consumer rather than inside the package.
func TestExportedMethodSurface(t *testing.T) {
	client := github.NewClientFromConfig(github.NewGitHubConfig())
	cfg := github.NewGitHubConfig()

	for name, fn := range map[string]func(){
		"Client.Ping":        func() { _ = client.Ping(context.Background()) },
		"Client.Get":         func() { _, _ = client.Get(context.Background(), "") },
		"Client.GetReleases": func() { _, _ = client.GetReleases(context.Background(), "", "") },
		"Client.GetLatestRelease": func() {
			_, _ = client.GetLatestRelease(context.Background(), "", "")
		},
		"Client.SetRepoOwner":    func() { client.SetRepoOwner(github.KanikoRepoOwner) },
		"Client.SetRepoName":     func() { client.SetRepoName(github.KanikoRepoName) },
		"Client.SetBaseURL":      func() { client.SetBaseURL(github.GitHubAPIBaseURL) },
		"Client.GetRepoOwner":    func() { _ = client.GetRepoOwner() },
		"Client.GetRepoName":     func() { _ = client.GetRepoName() },
		"Client.GetBaseURL":      func() { _ = client.GetBaseURL() },
		"GitHubConfig.WithToken": func() { _ = cfg.WithToken("") },
		"GitHubConfig.Clone":     func() { _ = cfg.Clone() },
		"GitHubConfig.String":    func() { _ = cfg.String() },
	} {
		// The map above is the assertion: each entry compiles only if the
		// named exported method exists with the signature used here.
		if fn == nil {
			t.Fatalf("%s: method value missing", name)
		}
		fn()
	}
}

// TestExternalConsumerRoundTrip drives the documented consumer flow end to
// end against a stub API: read a token from configuration, build a config,
// construct a client from it, and interpret both a release payload and the
// rate-limit headers on the responses. Only exported API is touched.
func TestExternalConsumerRoundTrip(t *testing.T) {
	const token = "external-test-token"

	var gotAuth, gotAccept, gotUserAgent []string
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		gotAuth = r.Header.Values("Authorization")
		gotAccept = r.Header.Values("Accept")
		gotUserAgent = r.Header.Values("User-Agent")

		switch r.URL.Path {
		case "/repos/GoogleContainerTools/kaniko/releases":
			_ = json.NewEncoder(w).Encode([]map[string]any{{
				"tag_name": "v1.0.0",
				"name":     "Release 1.0.0",
				"assets": []map[string]any{{
					"name":                 "releases_1.0.0.yaml",
					"browser_download_url": "http://" + r.Host + "/download/releases_1.0.0.yaml",
					"size":                 2048,
				}},
			}})
		case "/repos/GoogleContainerTools/kaniko/releases/latest":
			_ = json.NewEncoder(w).Encode(map[string]any{"tag_name": "v1.0.0"})
		case "/rate_limit":
			_ = json.NewEncoder(w).Encode(map[string]any{"rate": map[string]any{"remaining": 4999}})
		default:
			http.NotFound(w, r)
		}
	}))
	defer srv.Close()

	cfg := github.NewGitHubConfig().
		WithToken(token).
		Clone()
	cfg.BaseURL = srv.URL + "/" // trailing slash exercises the documented normalisation
	cfg.Timeout = 5 * time.Second

	client := github.NewClientFromConfig(cfg)
	releases, err := client.GetReleases(context.Background(), cfg.RepoOwner, cfg.RepoName)
	if err != nil {
		t.Fatalf("GetReleases: %v", err)
	}
	latest, err := client.GetLatestRelease(context.Background(), cfg.RepoOwner, cfg.RepoName)
	if err != nil {
		t.Fatalf("GetLatestRelease: %v", err)
	}
	// The generic path-based request reaches the same stub through the same
	// client, so the default headers it documents must hold there too.
	if _, err := client.Get(context.Background(), "/rate_limit"); err != nil {
		t.Fatalf("Get: %v", err)
	}

	// Every request was authenticated and identified the way the package
	// documents, so a consumer building on the same headers sees them too.
	if len(gotAuth) != 1 || gotAuth[0] != "Bearer "+token {
		t.Errorf("Authorization = %v, want exactly ['Bearer <token>']", gotAuth)
	}
	if len(gotAccept) != 1 || gotAccept[0] != "application/vnd.github+json" {
		t.Errorf("Accept = %v, want exactly ['application/vnd.github+json']", gotAccept)
	}
	if len(gotUserAgent) != 1 || gotUserAgent[0] != github.DefaultUserAgent {
		t.Errorf("User-Agent = %v, want exactly [%q]", gotUserAgent, github.DefaultUserAgent)
	}

	// The typed release methods decode the payload for the consumer: the
	// release fields and the asset's download URL arrive as struct fields, not
	// as JSON to be interpreted again.
	if len(releases) != 1 {
		t.Fatalf("len(releases) = %d, want 1", len(releases))
	}
	if releases[0].TagName != "v1.0.0" {
		t.Errorf("releases[0].TagName = %q, want %q", releases[0].TagName, "v1.0.0")
	}
	if len(releases[0].Assets) != 1 {
		t.Fatalf("len(releases[0].Assets) = %d, want 1", len(releases[0].Assets))
	}
	asset := releases[0].Assets[0]
	if asset.Name != "releases_1.0.0.yaml" || asset.Size != 2048 {
		t.Errorf("asset = %+v, want name releases_1.0.0.yaml size 2048", asset)
	}
	if asset.BrowserDownloadURL == "" {
		t.Error("asset.BrowserDownloadURL is empty, want the download URL")
	}
	if latest == nil || latest.TagName != "v1.0.0" {
		t.Errorf("latest = %+v, want tag_name v1.0.0", latest)
	}
}

// TestExportedRateLimitHelpers confirms the package-level response helpers
// answer a consumer's real question — is this 403 a rate limit, and how much
// of the window is left — using only exported API.
func TestExportedRateLimitHelpers(t *testing.T) {
	tests := []struct {
		name              string
		status            int
		remaining         string
		reset             string
		limit             string
		wantRateLimited   bool
		wantRemaining     int
		wantReset         int64
		wantAuthenticated bool
	}{
		{name: "rate limited", status: http.StatusForbidden, remaining: "0", reset: "1700000000", limit: "5000",
			wantRateLimited: true, wantRemaining: 0, wantReset: 1700000000, wantAuthenticated: true},
		{name: "quota left", status: http.StatusOK, remaining: "4999", reset: "1700000000", limit: "5000",
			wantRateLimited: false, wantRemaining: 4999, wantReset: 1700000000, wantAuthenticated: true},
		{name: "unauthenticated", status: http.StatusOK, remaining: "59", reset: "1700000000", limit: "60",
			wantRateLimited: false, wantRemaining: 59, wantReset: 1700000000, wantAuthenticated: false},
		{name: "unparsable counts read as zero", status: http.StatusOK, remaining: "many", reset: "soon", limit: "60",
			wantRateLimited: false, wantRemaining: 0, wantReset: 0, wantAuthenticated: false},
		{name: "absent headers read as zero", status: http.StatusOK, remaining: "", reset: "", limit: "",
			wantRateLimited: false, wantRemaining: 0, wantReset: 0, wantAuthenticated: false},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
				w.Header().Set("X-RateLimit-Remaining", tt.remaining)
				w.Header().Set("X-RateLimit-Reset", tt.reset)
				w.Header().Set("X-RateLimit-Limit", tt.limit)
				w.WriteHeader(tt.status)
			}))
			defer srv.Close()

			resp, err := srv.Client().Get(srv.URL)
			if err != nil {
				t.Fatalf("GET: %v", err)
			}
			defer resp.Body.Close()

			if got := github.IsRateLimited(resp); got != tt.wantRateLimited {
				t.Errorf("IsRateLimited() = %v, want %v", got, tt.wantRateLimited)
			}
			remaining, reset, authenticated := github.GetRateLimitInfo(resp)
			if remaining != tt.wantRemaining {
				t.Errorf("remaining = %d, want %d", remaining, tt.wantRemaining)
			}
			if reset != tt.wantReset {
				t.Errorf("reset = %d, want %d", reset, tt.wantReset)
			}
			if authenticated != tt.wantAuthenticated {
				t.Errorf("authenticated = %v, want %v", authenticated, tt.wantAuthenticated)
			}
		})
	}
}
