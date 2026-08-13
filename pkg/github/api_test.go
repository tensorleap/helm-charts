package github

import (
	"encoding/base64"
	"fmt"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// serveApi points the package at an httptest server for the duration of the test.
func serveApi(t *testing.T, handler http.HandlerFunc) {
	t.Helper()
	server := httptest.NewServer(handler)
	original := apiBaseURL
	apiBaseURL = server.URL
	t.Cleanup(func() {
		apiBaseURL = original
		server.Close()
	})
}

// clearTokens drops any token inherited from the developer's shell so the
// unauthenticated cases are actually unauthenticated.
func clearTokens(t *testing.T) {
	t.Helper()
	t.Setenv("GITHUB_TOKEN", "")
	t.Setenv("GH_TOKEN", "")
}

func TestApiToken(t *testing.T) {
	tests := []struct {
		name        string
		githubToken string
		ghToken     string
		expected    string
	}{
		{name: "no token set", expected: ""},
		{name: "GITHUB_TOKEN", githubToken: "gh-token", expected: "gh-token"},
		{name: "GH_TOKEN fallback", ghToken: "fallback", expected: "fallback"},
		{name: "GITHUB_TOKEN wins", githubToken: "primary", ghToken: "fallback", expected: "primary"},
		{name: "whitespace only is ignored", githubToken: "  ", ghToken: "fallback", expected: "fallback"},
		{name: "token is trimmed", githubToken: " padded\n", expected: "padded"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Setenv("GITHUB_TOKEN", tt.githubToken)
			t.Setenv("GH_TOKEN", tt.ghToken)
			assert.Equal(t, tt.expected, apiToken())
		})
	}
}

func TestGetFileContent(t *testing.T) {
	t.Run("decodes base64 content", func(t *testing.T) {
		clearTokens(t)
		var gotPath, gotRef, gotAccept, gotAuth string
		serveApi(t, func(w http.ResponseWriter, r *http.Request) {
			gotPath = r.URL.Path
			gotRef = r.URL.Query().Get("ref")
			gotAccept = r.Header.Get("Accept")
			gotAuth = r.Header.Get("Authorization")
			// The API wraps the payload at 60 columns; the decoder must tolerate it.
			encoded := base64.StdEncoding.EncodeToString([]byte("image-one\nimage-two\n"))
			_, _ = fmt.Fprintf(w, `{"content": %q}`, encoded[:8]+"\n"+encoded[8:])
		})

		content, err := GetFileContent("tensorleap", "helm-charts", "images.txt", "some-rc-branch")

		require.NoError(t, err)
		assert.Equal(t, "image-one\nimage-two\n", string(content))
		assert.Equal(t, "/repos/tensorleap/helm-charts/contents/images.txt", gotPath)
		assert.Equal(t, "some-rc-branch", gotRef)
		assert.Equal(t, "application/vnd.github+json", gotAccept)
		assert.Empty(t, gotAuth, "no Authorization header without a token")
	})

	t.Run("sends bearer token when set", func(t *testing.T) {
		clearTokens(t)
		t.Setenv("GITHUB_TOKEN", "secret-token")
		var gotAuth string
		serveApi(t, func(w http.ResponseWriter, r *http.Request) {
			gotAuth = r.Header.Get("Authorization")
			_, _ = fmt.Fprint(w, `{"content": ""}`)
		})

		_, err := GetFileContent("tensorleap", "helm-charts", "images.txt", "master")

		require.NoError(t, err)
		assert.Equal(t, "Bearer secret-token", gotAuth)
	})

	t.Run("reports an exhausted rate limit", func(t *testing.T) {
		clearTokens(t)
		serveApi(t, func(w http.ResponseWriter, r *http.Request) {
			w.Header().Set("X-RateLimit-Remaining", "0")
			w.Header().Set("X-RateLimit-Reset", "1786353705")
			w.WriteHeader(http.StatusForbidden)
			_, _ = fmt.Fprint(w, `{"message": "API rate limit exceeded for 20.1.2.3."}`)
		})

		_, err := GetFileContent("tensorleap", "helm-charts", "images.txt", "master")

		require.Error(t, err)
		assert.Contains(t, err.Error(), "403 Forbidden")
		assert.Contains(t, err.Error(), "API rate limit exceeded for 20.1.2.3.")
		assert.Contains(t, err.Error(), "GITHUB_TOKEN")
		assert.Contains(t, err.Error(), "2026-08-10T09:21:45Z", "should say when the quota resets")
	})

	t.Run("surfaces the API message on other failures", func(t *testing.T) {
		clearTokens(t)
		serveApi(t, func(w http.ResponseWriter, r *http.Request) {
			w.Header().Set("X-RateLimit-Remaining", "58")
			w.WriteHeader(http.StatusNotFound)
			_, _ = fmt.Fprint(w, `{"message": "No commit found for the ref missing-branch"}`)
		})

		_, err := GetFileContent("tensorleap", "helm-charts", "images.txt", "missing-branch")

		require.Error(t, err)
		assert.Contains(t, err.Error(), "No commit found for the ref missing-branch")
		assert.NotContains(t, err.Error(), "rate limit", "a 404 is not a quota problem")
	})
}

func TestGetReleasesPage(t *testing.T) {
	t.Run("decodes releases", func(t *testing.T) {
		clearTokens(t)
		serveApi(t, func(w http.ResponseWriter, r *http.Request) {
			_, _ = fmt.Fprint(w, `[{"name": "1.6.61", "tag_name": "1.6.61", "assets": [{"name": "manifest.yaml", "browser_download_url": "https://example.test/manifest.yaml"}]}]`)
		})

		releases, err := GetReleasesPage("tensorleap", "helm-charts", 1, 10)

		require.NoError(t, err)
		require.Len(t, releases, 1)
		assert.Equal(t, "1.6.61", releases[0].TagName)
		require.Len(t, releases[0].Assets, 1)
		assert.Equal(t, "manifest.yaml", releases[0].Assets[0].Name)
	})

	t.Run("clamps paging parameters", func(t *testing.T) {
		clearTokens(t)
		tests := []struct {
			name            string
			page, perPage   int
			wantPage        string
			wantPerPageSize string
		}{
			{name: "below range", page: 0, perPage: 0, wantPage: "1", wantPerPageSize: "1"},
			{name: "in range", page: 3, perPage: 50, wantPage: "3", wantPerPageSize: "50"},
			{name: "above range", page: 2, perPage: 500, wantPage: "2", wantPerPageSize: "100"},
		}

		for _, tt := range tests {
			t.Run(tt.name, func(t *testing.T) {
				var gotPage, gotPerPage string
				serveApi(t, func(w http.ResponseWriter, r *http.Request) {
					gotPage = r.URL.Query().Get("page")
					gotPerPage = r.URL.Query().Get("per_page")
					_, _ = fmt.Fprint(w, `[]`)
				})

				_, err := GetReleasesPage("tensorleap", "helm-charts", tt.page, tt.perPage)

				require.NoError(t, err)
				assert.Equal(t, tt.wantPage, gotPage)
				assert.Equal(t, tt.wantPerPageSize, gotPerPage)
			})
		}
	})

	t.Run("reports an exhausted rate limit", func(t *testing.T) {
		clearTokens(t)
		serveApi(t, func(w http.ResponseWriter, r *http.Request) {
			w.Header().Set("X-RateLimit-Remaining", "0")
			w.WriteHeader(http.StatusForbidden)
			_, _ = fmt.Fprint(w, `{"message": "API rate limit exceeded for 20.1.2.3."}`)
		})

		_, err := GetReleasesPage("tensorleap", "helm-charts", 1, 10)

		require.Error(t, err)
		assert.Contains(t, err.Error(), "GITHUB_TOKEN")
	})
}
