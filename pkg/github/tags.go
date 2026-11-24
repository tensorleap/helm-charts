package github

import (
	"bufio"
	"errors"
	"fmt"
	"io"
	"net/http"
	"strconv"
	"strings"
)

// webBaseURL is github.com itself (git and release downloads), as opposed to the
// REST API at api.github.com.
const webBaseURL = "https://github.com"

// ListTags returns the repository's tag names from git's smart-HTTP ref
// advertisement (<repo>.git/info/refs). That endpoint is not part of the REST
// API, so it works without a token and does not count against the API's
// 60 requests/hour anonymous limit.
func ListTags(owner, repo string) ([]string, error) {
	url := fmt.Sprintf("%s/%s/%s.git/info/refs?service=git-upload-pack", webBaseURL, owner, repo)
	resp, err := http.Get(url)
	if err != nil {
		return nil, err
	}
	defer func() { _ = resp.Body.Close() }()
	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("failed to list tags from %q: status %s", url, resp.Status)
	}
	tags, err := parseTagRefs(resp.Body)
	if err != nil {
		return nil, fmt.Errorf("failed to parse tags from %q: %w", url, err)
	}
	return tags, nil
}

// parseTagRefs reads a protocol-v0 ref advertisement: pkt-lines of four hex
// digits (the length, including those four) followed by "<sha> <ref>\n"; the
// first ref also carries "\x00<capabilities>". "0000" is a flush-pkt.
func parseTagRefs(r io.Reader) ([]string, error) {
	br := bufio.NewReader(r)
	var tags []string
	for {
		var size [4]byte
		if _, err := io.ReadFull(br, size[:]); err != nil {
			if errors.Is(err, io.EOF) {
				return tags, nil
			}
			return nil, err
		}
		n, err := strconv.ParseUint(string(size[:]), 16, 16)
		if err != nil {
			return nil, fmt.Errorf("invalid pkt-line length %q", size[:])
		}
		if n == 0 {
			continue
		}
		if n < 4 {
			return nil, fmt.Errorf("invalid pkt-line length %d", n)
		}
		line := make([]byte, n-4)
		if _, err := io.ReadFull(br, line); err != nil {
			return nil, err
		}
		s := strings.TrimSuffix(string(line), "\n")
		if i := strings.IndexByte(s, 0); i >= 0 {
			s = s[:i]
		}
		_, ref, ok := strings.Cut(s, " ")
		if !ok || strings.HasSuffix(ref, "^{}") {
			continue
		}
		if tag, ok := strings.CutPrefix(ref, "refs/tags/"); ok {
			tags = append(tags, tag)
		}
	}
}

// ReleaseAssetExists reports whether the release for tag has the named asset. A
// tag can exist before its release is published, or without one at all.
func ReleaseAssetExists(owner, repo, tag, fileName string) (bool, error) {
	url := fmt.Sprintf("%s/%s/%s/releases/download/%s/%s", webBaseURL, owner, repo, tag, fileName)
	resp, err := http.Head(url)
	if err != nil {
		return false, err
	}
	defer func() { _ = resp.Body.Close() }()
	switch {
	case resp.StatusCode >= 200 && resp.StatusCode < 300:
		return true, nil
	case resp.StatusCode == http.StatusNotFound:
		return false, nil
	}
	return false, fmt.Errorf("failed to check %q: status %s", url, resp.Status)
}
