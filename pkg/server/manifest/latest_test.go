package manifest

import (
	"fmt"
	"io"
	"net/http"
	"os"
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// fakeGitHub answers like GitHub does for an IP whose anonymous REST API quota is
// spent (BF-1416): every api.github.com call is a 403, while github.com itself
// (git refs and release downloads) still works. gitDown makes the git tag
// listing fail and apiWorks makes the API list releases for the same tags.
type fakeGitHub struct {
	tags      []string
	published map[string]bool
	manifest  []byte
	apiCalls  int
	gitDown   bool
	apiWorks  bool
}

func (f *fakeGitHub) RoundTrip(req *http.Request) (*http.Response, error) {
	respond := func(status int, body string, header http.Header) (*http.Response, error) {
		if header == nil {
			header = http.Header{}
		}
		return &http.Response{
			StatusCode: status,
			Status:     fmt.Sprintf("%d %s", status, http.StatusText(status)),
			Header:     header,
			Body:       io.NopCloser(strings.NewReader(body)),
			Request:    req,
		}, nil
	}

	switch {
	case req.URL.Host == "api.github.com" && f.apiWorks:
		f.apiCalls++
		releases := make([]string, len(f.tags))
		for i, tag := range f.tags {
			releases[i] = fmt.Sprintf(`{"tag_name": %q}`, tag)
		}
		return respond(http.StatusOK, "["+strings.Join(releases, ",")+"]", nil)

	case req.URL.Host == "api.github.com":
		f.apiCalls++
		header := http.Header{}
		header.Set("X-RateLimit-Limit", "60")
		header.Set("X-RateLimit-Remaining", "0")
		header.Set("X-RateLimit-Reset", "1795157097")
		return respond(http.StatusForbidden, `{"message":"API rate limit exceeded for 203.0.113.7. (But here's the good news: Authenticated requests get a higher rate limit. Check out the documentation for more details.)","documentation_url":"https://docs.github.com/rest/overview/resources-in-the-rest-api#rate-limiting"}`, header)

	case req.URL.Host == "github.com" && req.URL.Path == "/tensorleap/helm-charts.git/info/refs":
		if f.gitDown {
			return respond(http.StatusServiceUnavailable, "Service Unavailable", nil)
		}
		return respond(http.StatusOK, refAdvertisement(f.tags), nil)

	case req.URL.Host == "github.com" && strings.HasPrefix(req.URL.Path, "/tensorleap/helm-charts/releases/download/"):
		rest := strings.TrimPrefix(req.URL.Path, "/tensorleap/helm-charts/releases/download/")
		tag, file, _ := strings.Cut(rest, "/")
		if file == "manifest.yaml" && f.published[tag] {
			return respond(http.StatusOK, string(f.manifest), nil)
		}
		return respond(http.StatusNotFound, "Not Found", nil)
	}
	return respond(http.StatusNotFound, "unexpected request "+req.URL.String(), nil)
}

// refAdvertisement renders tags the way GitHub's smart-HTTP endpoint does
// (protocol v0): a service header, a flush, HEAD with capabilities, then one
// pkt-line per ref, with a peeled "^{}" line after each annotated tag.
func refAdvertisement(tags []string) string {
	pkt := func(s string) string { return fmt.Sprintf("%04x%s", len(s)+4, s) }
	sha := strings.Repeat("a", 40)
	var b strings.Builder
	b.WriteString(pkt("# service=git-upload-pack\n"))
	b.WriteString("0000")
	b.WriteString(pkt(sha + " HEAD\x00multi_ack thin-pack side-band symref=HEAD:refs/heads/master agent=git/github\n"))
	b.WriteString(pkt(sha + " refs/heads/master\n"))
	for _, tag := range tags {
		b.WriteString(pkt(sha + " refs/tags/" + tag + "\n"))
		b.WriteString(pkt(strings.Repeat("b", 40) + " refs/tags/" + tag + "^{}\n"))
	}
	b.WriteString("0000")
	return b.String()
}

func serveFakeGitHub(t *testing.T, f *fakeGitHub) {
	t.Helper()
	original := http.DefaultTransport
	http.DefaultTransport = f
	t.Cleanup(func() { http.DefaultTransport = original })
	t.Setenv("GITHUB_TOKEN", "")
	t.Setenv("GH_TOKEN", "")
}

func newFakeGitHub(t *testing.T, tags []string, published ...string) *fakeGitHub {
	t.Helper()
	manifestBytes, err := os.ReadFile("installation-manifest-v2.yaml")
	require.NoError(t, err)
	fake := &fakeGitHub{tags: tags, published: map[string]bool{}, manifest: manifestBytes}
	for _, tag := range published {
		fake.published[tag] = true
	}
	serveFakeGitHub(t, fake)
	return fake
}

// BF-1416: `leap server install` with no --tag resolves the latest manifest. With
// the anonymous API quota spent it must still install the latest release.
func TestGetByTagLatestWhenApiRateLimited(t *testing.T) {
	// 1.6.114 is the highest by number but not by string order.
	fake := newFakeGitHub(t,
		[]string{"manifest-1.6.9", "manifest-1.6.114", "manifest-1.6.10", "tensorleap-1.6.116-rc.1", "1.6.116-rc.1"},
		"manifest-1.6.9", "manifest-1.6.10", "manifest-1.6.114")

	mnf, err := GetByTag("")

	require.NoError(t, err)
	require.NotNil(t, mnf)
	tag, err := GetLatestManifestTag()
	require.NoError(t, err)
	assert.Equal(t, "manifest-1.6.114", tag)
	assert.Zero(t, fake.apiCalls, "resolving the latest release must not call api.github.com")
}

// Higher tags that are not a released manifest are skipped: rc and feature
// manifests, and a tag whose release (and manifest.yaml) is not published yet.
func TestGetLatestManifestTagSkipsUnreleased(t *testing.T) {
	newFakeGitHub(t,
		[]string{"manifest-1.6.114", "manifest-1.6.116-rc.0", "manifest-1.7.0-fast-pca.1", "manifest-1.6.118"},
		"manifest-1.6.114", "manifest-1.6.116-rc.0", "manifest-1.7.0-fast-pca.1")

	tag, err := GetLatestManifestTag()

	require.NoError(t, err)
	assert.Equal(t, "manifest-1.6.114", tag)
}

// When the git tag listing fails, the API is the backup, with the same rule:
// the newer rc manifest is skipped.
func TestGetLatestManifestTagFallsBackToApi(t *testing.T) {
	fake := newFakeGitHub(t,
		[]string{"manifest-1.6.116-rc.0", "manifest-1.6.114", "manifest-1.6.10"},
		"manifest-1.6.116-rc.0", "manifest-1.6.114", "manifest-1.6.10")
	fake.gitDown = true
	fake.apiWorks = true

	tag, err := GetLatestManifestTag()

	require.NoError(t, err)
	assert.Equal(t, "manifest-1.6.114", tag)
	assert.Equal(t, 1, fake.apiCalls)
}

func TestGetLatestManifestTagBothDownReportsBoth(t *testing.T) {
	fake := newFakeGitHub(t, []string{"manifest-1.6.114"}, "manifest-1.6.114")
	fake.gitDown = true

	_, err := GetLatestManifestTag()

	require.Error(t, err)
	assert.Contains(t, err.Error(), "503")
	assert.Contains(t, err.Error(), "set GITHUB_TOKEN")
}
