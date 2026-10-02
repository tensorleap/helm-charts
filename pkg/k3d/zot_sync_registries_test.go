package k3d

import (
	"reflect"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"github.com/tensorleap/helm-charts/pkg/server/manifest"
)

// IsNeedsToReinstall compares BuildZotSyncRegistries(previousMnf) with
// BuildZotSyncRegistries(mnf) using reflect.DeepEqual. That only means
// anything if the same manifest always yields the same slice — which it did
// not while the function ranged over maps: with the 8 upstream hosts of a real
// manifest, two calls agreed about 1 time in 13, so nearly every upgrade
// recreated the cluster.
func TestBuildZotSyncRegistriesIsDeterministic(t *testing.T) {
	// Eight hosts, several with more than one prefix, like a real manifest.
	mnf := &manifest.InstallationManifest{Images: manifest.ManifestImages{
		ServerImages: []string{
			"docker.elastic.co/eck/eck-operator:2.8.0",
			"docker.io/library/elasticsearch:8.10.1",
			"docker.io/library/mongo:6.0.5",
			"docker.io/moby/buildkit:buildx-stable-1",
			"gcr.io/datadoghq/agent:7.52.0",
			"ghcr.io/project-zot/zot-linux-amd64:v2.1.15",
			"nvcr.io/nvidia/k8s-device-plugin:v0.17.0-ubi9",
			"public.ecr.aws/tensorleap/engine:master-8b6de9cc",
			"public.ecr.aws/tensorleap/node-server:master-d2cff6a6",
			"quay.io/keycloak/keycloak:26.3.2",
			"quay.io/minio/minio:RELEASE.2021-12-20T22-07-16Z",
			"registry.k8s.io/ingress-nginx/controller:v1.10.0",
		},
		K3sImages: []string{
			"docker.io/rancher/mirrored-pause:3.6",
			"docker.io/rancher/klipper-lb:v0.4.3",
		},
	}}

	first := BuildZotSyncRegistries(mnf)
	require.Len(t, first, 8, "one entry per upstream host")

	t.Run("same manifest, same slice, every time", func(t *testing.T) {
		for i := 0; i < 200; i++ {
			assert.True(t, reflect.DeepEqual(first, BuildZotSyncRegistries(mnf)), "call %d differed", i)
		}
	})

	t.Run("hosts come out in sorted host order", func(t *testing.T) {
		// Sorted by host, not by URL: docker.io resolves to registry-1.docker.io,
		// so the URL column is not itself ascending.
		hosts := []string{"docker.elastic.co", "docker.io", "gcr.io", "ghcr.io", "nvcr.io", "public.ecr.aws", "quay.io", "registry.k8s.io"}
		want := make([]string, 0, len(hosts))
		for _, h := range hosts {
			want = append(want, upstream(h))
		}
		got := make([]string, 0, len(first))
		for _, r := range first {
			require.Len(t, r.URLs, 1)
			got = append(got, r.URLs[0])
		}
		assert.Equal(t, want, got)
	})

	t.Run("prefixes within a host are sorted", func(t *testing.T) {
		for _, r := range first {
			prefixes := make([]string, 0, len(r.Content))
			for _, c := range r.Content {
				prefixes = append(prefixes, c.Prefix)
			}
			assert.IsIncreasing(t, prefixes, "host %s", r.URLs[0])
		}
	})

	t.Run("content is what it was before, just ordered", func(t *testing.T) {
		byURL := map[string][]string{}
		for _, r := range first {
			for _, c := range r.Content {
				byURL[r.URLs[0]] = append(byURL[r.URLs[0]], c.Prefix)
			}
		}
		assert.Equal(t, []string{"library/**", "moby/**", "rancher/**"}, byURL[upstream("docker.io")])
		assert.Equal(t, []string{"keycloak/**", "minio/**"}, byURL[upstream("quay.io")])
		assert.Equal(t, []string{"tensorleap/**"}, byURL[upstream("public.ecr.aws")])
	})
}

// upstream mirrors the URL resolution in BuildZotSyncRegistries.
func upstream(host string) string {
	if u, ok := registryURLMap[host]; ok {
		return u
	}
	return "https://" + host
}
