// This file holds the integration coverage the hermetic tests cannot provide:
// a real fetch of a real public repository's releases over the network.
//
// It is gated behind SPAXEL_GITHUB_LIVE_TEST=1 rather than run by
// `go test ./...`, so the ordinary suite stays hermetic and does not fail on
// a runner without egress or during a GitHub outage. Run it with:
//
//	SPAXEL_GITHUB_LIVE_TEST=1 go test ./internal/github/ -run Live -v
package github_test

import (
	"context"
	"fmt"
	"log"
	"os"
	"testing"
	"time"

	"github.com/spaxel/mothership/internal/github"
)

// liveTestEnabled reports whether the caller opted into network tests.
func liveTestEnabled(t *testing.T) bool {
	t.Helper()
	if v, ok := os.LookupEnv("SPAXEL_GITHUB_LIVE_TEST"); ok && v == "1" {
		return true
	}
	t.Skip("set SPAXEL_GITHUB_LIVE_TEST=1 to run this test against the live GitHub API")
	return false
}

// TestGetReleasesLiveIntegration fetches the public Kaniko release feed with
// no token at all, which is the deployment path the mothership actually
// takes: public repositories answer unauthenticated requests, so no
// credential is needed to list releases. It asserts the shape of the decoded
// values rather than any particular release, so a new upstream release does
// not break it.
func TestGetReleasesLiveIntegration(t *testing.T) {
	if !liveTestEnabled(t) {
		return
	}

	// NewGitHubConfig defaults to https://api.github.com and an empty token:
	// exactly the unauthenticated public-API case this test is for.
	client := github.NewClientFromConfig(github.NewGitHubConfig())

	ctx, cancel := context.WithTimeout(context.Background(), github.DefaultGitHubTimeout)
	defer cancel()

	releases, err := client.GetReleases(ctx, github.KanikoRepoOwner, github.KanikoRepoName)
	if err != nil {
		t.Fatalf("GetReleases(%s/%s) failed: %v", github.KanikoRepoOwner, github.KanikoRepoName, err)
	}
	if len(releases) == 0 {
		t.Fatalf("GetReleases(%s/%s) returned no releases; a public repo with a release feed must list at least one",
			github.KanikoRepoOwner, github.KanikoRepoName)
	}

	seen := make(map[string]bool, len(releases))
	for i, r := range releases {
		if r.TagName == "" {
			t.Errorf("release[%d] has an empty TagName", i)
		}
		if r.ID == 0 {
			t.Errorf("release[%d] (%s) has a zero ID", i, r.TagName)
		}
		if r.HTMLURL == "" {
			t.Errorf("release[%d] (%s) has an empty HTMLURL", i, r.TagName)
		}
		// The API omits timestamps on some legacy releases, so only the ones
		// present must be well-formed — never a specific value.
		for _, field := range []struct{ name, value string }{
			{"CreatedAt", r.CreatedAt},
			{"PublishedAt", r.PublishedAt},
		} {
			if field.value == "" {
				continue
			}
			if _, err := time.Parse(time.RFC3339, field.value); err != nil {
				t.Errorf("release[%d] (%s) %s %q is not RFC3339: %v", i, r.TagName, field.name, field.value, err)
			}
		}
		seen[r.TagName] = true
	}

	// The list endpoint and the single-release endpoint must agree that the
	// latest published release exists. /releases/latest can disagree with
	// releases[0] when the newest entry is a draft or a prerelease, so the
	// assertion is membership, not ordering.
	latest, err := client.GetLatestRelease(ctx, github.KanikoRepoOwner, github.KanikoRepoName)
	if err != nil {
		t.Fatalf("GetLatestRelease(%s/%s) failed: %v", github.KanikoRepoOwner, github.KanikoRepoName, err)
	}
	if latest.TagName == "" {
		t.Fatal("GetLatestRelease returned an empty TagName")
	}
	if !seen[latest.TagName] {
		t.Errorf("latest release %q is absent from the %d releases listed; tag names seen: %v",
			latest.TagName, len(releases), tagNames(releases))
	}
}

// TestGetReleasesLiveGitHubEnterpriseBaseURL exercises the base-URL override
// against the same public API by constructing the client the way a GitHub
// Enterprise deployment would — the host changes, the code path does not.
func TestGetReleasesLiveGitHubEnterpriseBaseURL(t *testing.T) {
	if !liveTestEnabled(t) {
		return
	}

	cfg := github.NewGitHubConfig()
	cfg.BaseURL = "https://api.github.com/" // trailing slash must be normalised away
	client := github.NewClientFromConfig(cfg)
	if got := client.GetBaseURL(); got != github.GitHubAPIBaseURL {
		t.Fatalf("GetBaseURL() = %q, want the normalised %q", got, github.GitHubAPIBaseURL)
	}

	ctx, cancel := context.WithTimeout(context.Background(), github.DefaultGitHubTimeout)
	defer cancel()

	releases, err := client.GetReleases(ctx, github.KanikoRepoOwner, github.KanikoRepoName)
	if err != nil {
		t.Fatalf("GetReleases via overridden base URL failed: %v", err)
	}
	if len(releases) == 0 {
		t.Fatal("GetReleases via overridden base URL returned no releases")
	}
}

// tagNames returns the tag names of releases, for error messages only.
func tagNames(releases []github.Release) []string {
	names := make([]string, 0, len(releases))
	for _, r := range releases {
		names = append(names, r.TagName)
	}
	return names
}

// ExampleClient_GetReleases documents the unauthenticated fetch of a public
// repository's releases. It carries no Output comment, so `go test` compiles
// it without executing it — a live example that ran on every test invocation
// would make the suite depend on the network.
func ExampleClient_GetReleases() {
	// Public repositories need no token; NewGitHubConfig is unauthenticated
	// and targets https://api.github.com by default.
	client := github.NewClientFromConfig(github.NewGitHubConfig())

	releases, err := client.GetReleases(context.Background(), "GoogleContainerTools", "kaniko")
	if err != nil {
		// Every failure is an *APIError: branch on Kind, not on message text.
		log.Fatalf("fetching kaniko releases: %v", err)
	}

	for _, r := range releases {
		fmt.Printf("%s published %s\n", r.TagName, r.PublishedAt)
	}
}
