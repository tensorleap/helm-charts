package zot

import (
	"encoding/json"
	"fmt"
	"net/http"
	"strings"
	"time"

	"github.com/tensorleap/helm-charts/pkg/log"
)

type catalogResponse struct {
	Repositories []string `json:"repositories"`
}

type tagsResponse struct {
	Name string   `json:"name"`
	Tags []string `json:"tags"`
}

// PruneExceptImageList removes all image tags from the Zot registry that are
// NOT in the keepImages list. Repos listed in preserveRepos are skipped
// entirely (used to protect DnD-pushed images like custom engine-generic builds).
// Zot's built-in GC will reclaim orphaned blobs.
//
// Deletion is digest-safe: since multiple tags can share a manifest digest,
// we first resolve keep-tag digests, then only delete digests that no
// keep-tag points to.
func PruneExceptImageList(registryURL string, keepImages []string, preserveRepos []string) error {
	keepSet := buildKeepSet(keepImages)
	preserveSet := make(map[string]bool, len(preserveRepos))
	for _, r := range preserveRepos {
		preserveSet[r] = true
	}

	client := &http.Client{Timeout: 5 * time.Second}

	repos, err := listRepos(client, registryURL)
	if err != nil {
		return fmt.Errorf("listing Zot catalog: %w", err)
	}

	totalDeleted := 0
	for _, repo := range repos {
		if preserveSet[repo] {
			log.Infof("Preserving Zot repo %s (DnD images)", repo)
			continue
		}

		tags, err := listTags(client, registryURL, repo)
		if err != nil {
			log.Warnf("Failed to list tags for %s: %v", repo, err)
			continue
		}
		if len(tags) == 0 {
			continue
		}

		keepDigests := make(map[string]bool)
		var deleteCandidates []string
		skipRepo := false

		for _, tag := range tags {
			if keepSet[repo+":"+tag] {
				digest, err := getManifestDigest(client, registryURL, repo, tag)
				if err != nil {
					log.Warnf("Cannot resolve digest for keep-tag %s:%s, skipping repo: %v", repo, tag, err)
					skipRepo = true
					break
				}
				keepDigests[digest] = true
			} else {
				deleteCandidates = append(deleteCandidates, tag)
			}
		}
		if skipRepo {
			continue
		}

		for _, tag := range deleteCandidates {
			digest, err := getManifestDigest(client, registryURL, repo, tag)
			if err != nil {
				log.Warnf("Failed to get digest for %s:%s: %v", repo, tag, err)
				continue
			}
			if keepDigests[digest] {
				continue
			}

			if err := deleteByDigest(client, registryURL, repo, digest); err != nil {
				log.Warnf("Failed to delete %s:%s from Zot: %v", repo, tag, err)
			} else {
				totalDeleted++
			}
		}
	}

	if totalDeleted == 0 {
		log.Info("Nothing to delete from Zot registry")
	} else {
		log.Infof("Deleted %d image tags from Zot registry", totalDeleted)
	}
	return nil
}

func listRepos(client *http.Client, registryURL string) ([]string, error) {
	resp, err := client.Get(registryURL + "/v2/_catalog")
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("unexpected status %d", resp.StatusCode)
	}
	var catalog catalogResponse
	if err := json.NewDecoder(resp.Body).Decode(&catalog); err != nil {
		return nil, err
	}
	return catalog.Repositories, nil
}

func listTags(client *http.Client, registryURL, repo string) ([]string, error) {
	resp, err := client.Get(fmt.Sprintf("%s/v2/%s/tags/list", registryURL, repo))
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("unexpected status %d for %s", resp.StatusCode, repo)
	}
	var tags tagsResponse
	if err := json.NewDecoder(resp.Body).Decode(&tags); err != nil {
		return nil, err
	}
	return tags.Tags, nil
}

func deleteByDigest(client *http.Client, registryURL, repo, digest string) error {
	url := fmt.Sprintf("%s/v2/%s/manifests/%s", registryURL, repo, digest)
	req, err := http.NewRequest(http.MethodDelete, url, nil)
	if err != nil {
		return err
	}
	resp, err := client.Do(req)
	if err != nil {
		return err
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusAccepted && resp.StatusCode != http.StatusOK {
		return fmt.Errorf("delete %s@%s returned status %d", repo, digest, resp.StatusCode)
	}
	return nil
}

func getManifestDigest(client *http.Client, registryURL, repo, tag string) (string, error) {
	url := fmt.Sprintf("%s/v2/%s/manifests/%s", registryURL, repo, tag)
	req, err := http.NewRequest(http.MethodHead, url, nil)
	if err != nil {
		return "", err
	}
	// Accept both OCI and Docker manifest types
	req.Header.Set("Accept", strings.Join([]string{
		"application/vnd.oci.image.manifest.v1+json",
		"application/vnd.oci.image.index.v1+json",
		"application/vnd.docker.distribution.manifest.v2+json",
		"application/vnd.docker.distribution.manifest.list.v2+json",
	}, ", "))

	resp, err := client.Do(req)
	if err != nil {
		return "", err
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		return "", fmt.Errorf("HEAD %s returned status %d", url, resp.StatusCode)
	}
	digest := resp.Header.Get("Docker-Content-Digest")
	if digest == "" {
		return "", fmt.Errorf("no Docker-Content-Digest header for %s:%s", repo, tag)
	}
	return digest, nil
}

// DnDPreserveRepos returns the Zot repo paths that should be preserved during
// pruning because they may contain DnD-pushed custom images (e.g. engine-generic
// builds with user-specific dependencies). The repos are identified by the
// "engine-generic" convention in the manifest image list.
func DnDPreserveRepos(manifestImages []string) []string {
	seen := make(map[string]bool)
	var repos []string
	for _, img := range manifestImages {
		repo, _ := imageToZotRef(img)
		if repo != "" && strings.Contains(repo, "engine-generic") && !seen[repo] {
			seen[repo] = true
			repos = append(repos, repo)
		}
	}
	return repos
}

// buildKeepSet converts manifest image references (e.g. "docker.io/library/mongo:6.0.5")
// into Zot repo+tag keys (e.g. "library/mongo:6.0.5").
func buildKeepSet(images []string) map[string]bool {
	keep := make(map[string]bool, len(images))
	for _, img := range images {
		repo, tag := imageToZotRef(img)
		if repo != "" && tag != "" {
			keep[repo+":"+tag] = true
		}
	}
	return keep
}

// imageToZotRef converts a full image reference to the repo path and tag
// as stored in Zot. Zot stores images under the path portion after the
// registry host, e.g.:
//
//	"docker.io/library/mongo:6.0.5"              → ("library/mongo", "6.0.5")
//	"public.ecr.aws/tensorleap/engine:master-xx" → ("tensorleap/engine", "master-xx")
//	"registry.k8s.io/ingress-nginx/ctrl:v1.10.0" → ("ingress-nginx/ctrl", "v1.10.0")
//	"alpine:3.18.3"                               → ("library/alpine", "3.18.3")
func imageToZotRef(image string) (repo, tag string) {
	// Split off tag
	lastColon := strings.LastIndex(image, ":")
	if lastColon == -1 {
		return "", ""
	}
	tag = image[lastColon+1:]
	nameWithHost := image[:lastColon]

	// Split into host and path. A component is a registry host if it
	// contains a dot or a colon (port), otherwise it's part of the path.
	parts := strings.SplitN(nameWithHost, "/", 2)
	if len(parts) == 1 {
		// No slash at all → Docker Hub official image (e.g. "alpine")
		return "library/" + parts[0], tag
	}

	firstPart := parts[0]
	if strings.Contains(firstPart, ".") || strings.Contains(firstPart, ":") {
		// First component is a registry host → strip it
		repo = parts[1]
	} else {
		// No dots/colons → Docker Hub user image (e.g. "rancher/k3s")
		repo = nameWithHost
	}
	return repo, tag
}
