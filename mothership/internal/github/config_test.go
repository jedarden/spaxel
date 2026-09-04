package github

import (
	"strings"
	"testing"
	"time"
)

func TestNewGitHubConfig(t *testing.T) {
	tests := []struct {
		name string
		want GitHubConfig
	}{
		{
			name: "defaults target the public Kaniko release feed unauthenticated",
			want: GitHubConfig{
				BaseURL:   "https://api.github.com",
				Token:     "",
				RepoOwner: "GoogleContainerTools",
				RepoName:  "kaniko",
				Timeout:   30 * time.Second,
			},
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := NewGitHubConfig(); got != tt.want {
				t.Errorf("NewGitHubConfig() = %+v, want %+v", got, tt.want)
			}
		})
	}
}

func TestNewGitHubConfigDeterministic(t *testing.T) {
	// Configs must be comparable with == and two default builds must be equal,
	// so a caller can detect "changed vs default" without a field-by-field diff.
	a, b := NewGitHubConfig(), NewGitHubConfig()
	if a != b {
		t.Errorf("two NewGitHubConfig() results differ: %+v vs %+v", a, b)
	}
}

func TestGitHubConfigClone(t *testing.T) {
	orig := NewGitHubConfig().WithToken("tok-123")

	got := orig.Clone()
	if got != orig {
		t.Errorf("Clone() = %+v, want %+v", got, orig)
	}

	got.RepoName = "other-repo"
	if orig.RepoName != KanikoRepoName {
		t.Errorf("mutating the clone changed the receiver: RepoName = %q", orig.RepoName)
	}
}

func TestGitHubConfigWithToken(t *testing.T) {
	tests := []struct {
		name      string
		token     string
		wantToken string
	}{
		{name: "set a token", token: "ghp_test", wantToken: "ghp_test"},
		{name: "empty token clears", token: "", wantToken: ""},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			orig := NewGitHubConfig()
			got := orig.WithToken(tt.token)

			if got.Token != tt.wantToken {
				t.Errorf("WithToken(%q).Token = %q, want %q", tt.token, got.Token, tt.wantToken)
			}
			if got.BaseURL != orig.BaseURL || got.RepoOwner != orig.RepoOwner ||
				got.RepoName != orig.RepoName || got.Timeout != orig.Timeout {
				t.Errorf("WithToken altered other fields: got %+v, receiver %+v", got, orig)
			}
			if orig.Token != "" {
				t.Errorf("WithToken mutated the receiver: Token = %q", orig.Token)
			}
		})
	}
}

func TestGitHubConfigString(t *testing.T) {
	secret := "ghp_super_secret_token_value"

	t.Run("redacts the token value", func(t *testing.T) {
		got := NewGitHubConfig().WithToken(secret).String()
		if strings.Contains(got, secret) {
			t.Errorf("String() leaked the token: %q", got)
		}
		if !strings.Contains(got, "token: set") {
			t.Errorf("String() = %q, want it to report token: set", got)
		}
	})

	t.Run("reports unset token", func(t *testing.T) {
		got := NewGitHubConfig().String()
		if !strings.Contains(got, "token: unset") {
			t.Errorf("String() = %q, want it to report token: unset", got)
		}
	})

	t.Run("includes the non-secret fields", func(t *testing.T) {
		got := NewGitHubConfig().String()
		for _, want := range []string{GitHubAPIBaseURL, KanikoRepoOwner, KanikoRepoName, "30s"} {
			if !strings.Contains(got, want) {
				t.Errorf("String() = %q, want it to contain %q", got, want)
			}
		}
	})
}
