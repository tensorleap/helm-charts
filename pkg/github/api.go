package github

import (
	"encoding/base64"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"os"
	"strconv"
	"strings"
	"time"
)

// apiBaseURL is a var rather than a const so tests can point the client at an
// httptest server.
var apiBaseURL = "https://api.github.com"

type Release struct {
	Name    string `json:"name"`
	TagName string `json:"tag_name"`
	Assets  []struct {
		Name        string `json:"name"`
		DownloadUrl string `json:"browser_download_url"`
	} `json:"assets"`
}

// apiToken returns the token used to authenticate against the GitHub API, or an
// empty string when none is set. Unauthenticated calls are capped at 60 requests
// per hour per source IP; CI runners share egress IPs, so that budget is often
// already spent by other tenants and the API answers 403. A token raises the cap
// to 5000 requests per hour.
func apiToken() string {
	for _, key := range []string{"GITHUB_TOKEN", "GH_TOKEN"} {
		if token := strings.TrimSpace(os.Getenv(key)); token != "" {
			return token
		}
	}
	return ""
}

// getApi issues an authenticated GET against the GitHub API when a token is
// available, and an anonymous one otherwise.
func getApi(url string) (*http.Response, error) {
	req, err := http.NewRequest(http.MethodGet, url, nil)
	if err != nil {
		return nil, fmt.Errorf("failed building request for %q: %w", url, err)
	}
	req.Header.Set("Accept", "application/vnd.github+json")
	if token := apiToken(); token != "" {
		req.Header.Set("Authorization", "Bearer "+token)
	}
	return http.DefaultClient.Do(req)
}

// rateLimitHint reports whether the response failed because the API quota is
// spent, so the error says so instead of looking like a missing file.
func rateLimitHint(resp *http.Response) string {
	if resp.Header.Get("X-RateLimit-Remaining") != "0" {
		return ""
	}
	hint := "; GitHub API rate limit exhausted for this IP - set GITHUB_TOKEN to raise it from 60 to 5000 requests/hour"
	if reset, err := strconv.ParseInt(resp.Header.Get("X-RateLimit-Reset"), 10, 64); err == nil {
		hint += fmt.Sprintf(" (resets at %s)", time.Unix(reset, 0).UTC().Format(time.RFC3339))
	}
	return hint
}

// apiError carries GitHub's own message, which the API returns in the body. The
// status line alone does not distinguish a spent rate limit from a real denial.
func apiError(action, url string, resp *http.Response) error {
	var apiErr struct {
		Message string `json:"message"`
	}
	detail := ""
	body, _ := io.ReadAll(resp.Body)
	if err := json.Unmarshal(body, &apiErr); err == nil && apiErr.Message != "" {
		detail = " - " + apiErr.Message
	}
	return fmt.Errorf("failed to %s from %q: status %s%s%s", action, url, resp.Status, detail, rateLimitHint(resp))
}

func GetReleasesPage(owner, repo string, page, per_page int) ([]Release, error) {

	if page < 1 {
		page = 1
	}
	if per_page < 1 {
		per_page = 1
	} else if per_page > 100 {
		per_page = 100
	}

	url := fmt.Sprintf("%s/repos/%s/%s/releases?page=%v&per_page=%v", apiBaseURL, owner, repo, page, per_page)

	var releases []Release

	res, err := getApi(url)
	if err != nil {
		return nil, err
	}
	defer res.Body.Close()

	if res.StatusCode < 200 || res.StatusCode >= 300 {
		return nil, apiError("list releases", url, res)
	}

	err = json.NewDecoder(res.Body).Decode(&releases)
	if err != nil {
		return nil, err
	}

	return releases, nil
}

func GetTagArtifact(owner, repo, fileName, tag string) ([]byte, error) {
	url := fmt.Sprintf("https://github.com/%s/%s/releases/download/%s/%s", owner, repo, tag, fileName)

	return getFileByUrl(url)
}

// getFileByUrl fetches a release asset from github.com (not the API), which is
// served without authentication and is not subject to the API rate limit.
func getFileByUrl(url string) ([]byte, error) {
	resp, err := http.Get(url)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()
	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		return nil, fmt.Errorf("failed getting file from (%s): %v", url, resp.StatusCode)
	}
	imagesFile, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, err
	}
	return imagesFile, nil
}

func GetFileContent(owner, repo, filePath, ref string) ([]byte, error) {

	url := fmt.Sprintf("%s/repos/%s/%s/contents/%s?ref=%s", apiBaseURL, owner, repo, filePath, ref)

	resp, err := getApi(url)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return nil, apiError("retrieve file", url, resp)
	}

	var fileData struct {
		Content string `json:"content"`
	}
	err = json.NewDecoder(resp.Body).Decode(&fileData)
	if err != nil {
		return nil, err
	}

	decodedContent, err := base64.StdEncoding.DecodeString(fileData.Content)
	if err != nil {
		return nil, err
	}

	return decodedContent, nil
}
