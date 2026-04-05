package k3d

import (
	"strings"
	"testing"

	"github.com/tensorleap/helm-charts/pkg/server/manifest"
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
