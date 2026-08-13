package k3d

import (
	"testing"

	"github.com/stretchr/testify/assert"
)

func TestFilterMissingImages(t *testing.T) {
	// A representative slice of real `ctr -n k8s.io images ls` output: full
	// docker.io/-qualified refs, plus one non-docker.io registry.
	listing := `docker.io/rancher/mirrored-pause:3.6                    application/vnd.oci.image.manifest.v1+json sha256:169745... linux/amd64
ghcr.io/project-zot/zot-linux-amd64:v2.1.15             application/vnd.oci.image.manifest.v1+json sha256:173217... linux/amd64
docker.io/rancher/klipper-helm:v0.7.7-build20230403     application/vnd.oci.image.manifest.v1+json sha256:263a98... linux/amd64
`

	tests := []struct {
		name    string
		listing string
		images  []string
		want    []string
	}{
		{
			name:    "all present, some needing the docker.io/ prefix match",
			listing: listing,
			images: []string{
				"rancher/mirrored-pause:3.6",                  // present as docker.io/rancher/mirrored-pause:3.6
				"ghcr.io/project-zot/zot-linux-amd64:v2.1.15", // present verbatim, no docker.io/ prefix
				"rancher/klipper-helm:v0.7.7-build20230403",
			},
			want: nil,
		},
		{
			name:    "one missing",
			listing: listing,
			images: []string{
				"rancher/mirrored-pause:3.6",
				"rancher/mirrored-metrics-server:v0.6.2", // not in listing
			},
			want: []string{"rancher/mirrored-metrics-server:v0.6.2"},
		},
		{
			name:    "all missing",
			listing: listing,
			images:  []string{"rancher/does-not-exist:1.0"},
			want:    []string{"rancher/does-not-exist:1.0"},
		},
		{
			name:    "empty images list",
			listing: listing,
			images:  nil,
			want:    nil,
		},
		{
			name:    "empty listing",
			listing: "",
			images:  []string{"rancher/mirrored-pause:3.6"},
			want:    []string{"rancher/mirrored-pause:3.6"},
		},
		{
			// Regression: a naive substring check would false-positive-match
			// requested "rancher/mirrored-pause:3.6" against a listed
			// "...pause:3.60" row (no exact "...pause:3.6" row present here),
			// wrongly treating the requested image as present.
			name:    "tag-prefix collision is not a false match",
			listing: "docker.io/rancher/mirrored-pause:3.60                   application/vnd.oci.image.manifest.v1+json sha256:aaaaaa... linux/amd64\n",
			images:  []string{"rancher/mirrored-pause:3.6"},
			want:    []string{"rancher/mirrored-pause:3.6"},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := filterMissingImages(tt.listing, tt.images)
			assert.Equal(t, tt.want, got)
		})
	}
}
