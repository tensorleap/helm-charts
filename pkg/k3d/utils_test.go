package k3d

import (
	"strings"
	"testing"

	"github.com/tensorleap/helm-charts/pkg/server/manifest"
	"gopkg.in/yaml.v3"
)

func TestCreateMirrorFromManifest(t *testing.T) {
	mfs := manifest.InstallationManifest{
		Images: manifest.ManifestImages{
			ServerImages: []string{
				"docker.elastic.co/elasticsearch/elasticsearch:7.10.2",
				"docker.io/library/rabbitmq:3.9.22",
				"quay.io/minio/minio:RELEASE.2021-12-20T22-07-16Z",
				"registry.k8s.io/ingress-nginx/controller:v1.8.0",
				"registry.k8s.io/ingress-nginx/kube-webhook-certgen:v20230407",
				"public.ecr.aws/tensorleap/engine:master-40246f22-stable",
				"public.ecr.aws/tensorleap/node-server:master-cc43e60b-stable",
				"public.ecr.aws/tensorleap/web-ui:master-6ea417b8-stable",
			},
			K3sImages: []string{
				"docker.io/rancher/klipper-helm:v0.7.7-build20230403",
				"docker.io/rancher/klipper-lb:v0.4.3",
			},
		},
	}

	t.Run("airgap mirrors all registries", func(t *testing.T) {
		mirrorConfig, err := CreateMirrorFromManifest(&mfs, 5000, true)
		if err != nil {
			t.Fatal(err)
		}
		for _, host := range []string{"docker.io", "docker.elastic.co", "quay.io", "registry.k8s.io", "public.ecr.aws", "tensorleap-registry:5000"} {
			if !strings.Contains(mirrorConfig, host) {
				t.Errorf("airgap mirrorConfig missing mirror for %s", host)
			}
		}
	})

	t.Run("online mirrors tensorleap-registry and public.ecr.aws", func(t *testing.T) {
		mirrorConfig, err := CreateMirrorFromManifest(&mfs, 5000, false)
		if err != nil {
			t.Fatal(err)
		}
		for _, host := range []string{"tensorleap-registry:5000", "public.ecr.aws"} {
			if !strings.Contains(mirrorConfig, host) {
				t.Errorf("online mirrorConfig missing mirror for %s", host)
			}
		}
		for _, host := range []string{"docker.elastic.co", "quay.io", "registry.k8s.io"} {
			if strings.Contains(mirrorConfig, host+":") {
				t.Errorf("online mirrorConfig should NOT mirror %s", host)
			}
		}
	})
}

func TestCreateMirrorFromManifestRewritesK3sImagesToTheMirror(t *testing.T) {
	type mirror struct {
		Endpoint []string          `yaml:"endpoint"`
		Rewrite  map[string]string `yaml:"rewrite"`
	}
	dockerHubMirror := func(t *testing.T, mfs *manifest.InstallationManifest, isAirgap bool) (mirror, bool) {
		t.Helper()
		config, err := CreateMirrorFromManifest(mfs, 5000, isAirgap)
		if err != nil {
			t.Fatal(err)
		}
		var parsed struct {
			Mirrors map[string]mirror `yaml:"mirrors"`
		}
		if err := yaml.Unmarshal([]byte(config), &parsed); err != nil {
			t.Fatalf("registries.yaml doesn't parse: %v\n%s", err, config)
		}
		m, ok := parsed.Mirrors["docker.io"]
		return m, ok
	}
	mirrorEra := &manifest.InstallationManifest{Images: manifest.ManifestImages{
		InstallationImages: manifest.InstallationImages{K3s: manifest.MirrorRegistry + "/docker.io/rancher/k3s:v1.26.4-k3s1"},
		K3sImages:          []string{"docker.io/rancher/mirrored-pause:3.6"},
	}}
	beforeTheMirror := &manifest.InstallationManifest{Images: manifest.ManifestImages{
		InstallationImages: manifest.InstallationImages{K3s: "docker.io/rancher/k3s:v1.26.4-k3s1"},
		K3sImages:          []string{"docker.io/rancher/mirrored-pause:3.6"},
	}}

	t.Run("online, mirror-era manifest: rancher/* rewritten to the mirror", func(t *testing.T) {
		m, ok := dockerHubMirror(t, mirrorEra, false)
		if !ok {
			t.Fatal("no docker.io mirror")
		}
		if len(m.Endpoint) != 1 || m.Endpoint[0] != "https://public.ecr.aws" {
			t.Errorf("endpoint = %v, want [https://public.ecr.aws]", m.Endpoint)
		}
		want := map[string]string{"^rancher/(.*)$": "tensorleap/mirror/docker.io/rancher/$1"}
		if len(m.Rewrite) != 1 || m.Rewrite["^rancher/(.*)$"] != want["^rancher/(.*)$"] {
			t.Errorf("rewrite = %v, want %v", m.Rewrite, want)
		}
	})

	t.Run("online, manifest from before the mirror: docker.io untouched", func(t *testing.T) {
		if m, ok := dockerHubMirror(t, beforeTheMirror, false); ok {
			t.Errorf("unexpected docker.io mirror %+v", m)
		}
	})

	t.Run("airgap, mirror-era manifest: docker.io goes to Zot, no rewrite", func(t *testing.T) {
		m, ok := dockerHubMirror(t, mirrorEra, true)
		if !ok {
			t.Fatal("no docker.io mirror")
		}
		if len(m.Endpoint) != 1 || m.Endpoint[0] != "http://127.0.0.1:5000" || len(m.Rewrite) != 0 {
			t.Errorf("docker.io mirror = %+v, want Zot only", m)
		}
	})
}
