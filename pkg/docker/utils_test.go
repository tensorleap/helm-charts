package docker

import (
	"testing"
)

func TestGetRegistry(t *testing.T) {
	tests := []struct {
		name     string
		image    string
		expected string
	}{
		{"k3s", "docker.io/rancher/k3s:v1.26.4-k3s1", "docker.io"},
		{"k3sGpu", "public.ecr.aws/tensorleap/k3s:v1.26.4-k3s1-cuda-11.8.0-ubuntu-22.04-v1", "public.ecr.aws"},
		{"k3dTools", "ghcr.io/k3d-io/k3d-tools:5.5.2", "ghcr.io"},
		{"register", "docker.io/library/registry:2", "docker.io"},
		{"checkDockerRequirement", "alpine:3.18.3", "docker.io"},

		// k3sImages
		{"klipper-helm", "docker.io/rancher/klipper-helm:v0.7.7-build20230403", "docker.io"},
		{"klipper-lb", "docker.io/rancher/klipper-lb:v0.4.3", "docker.io"},
		{"local-path-provisioner", "docker.io/rancher/local-path-provisioner:v0.0.24", "docker.io"},
		{"mirrored-coredns", "docker.io/rancher/mirrored-coredns-coredns:1.10.1", "docker.io"},
		{"mirrored-busybox", "docker.io/rancher/mirrored-library-busybox:1.34.1", "docker.io"},
		{"mirrored-traefik", "docker.io/rancher/mirrored-library-traefik:2.9.4", "docker.io"},
		{"mirrored-metrics", "docker.io/rancher/mirrored-metrics-server:v0.6.2", "docker.io"},
		{"mirrored-pause", "docker.io/rancher/mirrored-pause:3.6", "docker.io"},

		// k3sGpuImages (same as above)
		{"gpu-klipper-helm", "docker.io/rancher/klipper-helm:v0.7.7-build20230403", "docker.io"},
		{"gpu-klipper-lb", "docker.io/rancher/klipper-lb:v0.4.3", "docker.io"},
		{"gpu-local-path", "docker.io/rancher/local-path-provisioner:v0.0.24", "docker.io"},
		{"gpu-mirrored-coredns", "docker.io/rancher/mirrored-coredns-coredns:1.10.1", "docker.io"},
		{"gpu-mirrored-busybox", "docker.io/rancher/mirrored-library-busybox:1.34.1", "docker.io"},
		{"gpu-mirrored-traefik", "docker.io/rancher/mirrored-library-traefik:2.9.4", "docker.io"},
		{"gpu-mirrored-metrics", "docker.io/rancher/mirrored-metrics-server:v0.6.2", "docker.io"},
		{"gpu-mirrored-pause", "docker.io/rancher/mirrored-pause:3.6", "docker.io"},

		// serverImages
		{"eck-operator", "docker.elastic.co/eck/eck-operator:2.8.0", "docker.elastic.co"},
		{"bitnami-postgres", "docker.io/bitnami/postgresql:11.14.0-debian-10-r28", "docker.io"},
		{"busybox", "docker.io/library/busybox:1.32", "docker.io"},
		{"elasticsearch", "docker.io/library/elasticsearch:8.10.1", "docker.io"},
		{"mongo", "docker.io/library/mongo:6.0.5", "docker.io"},
		{"rabbitmq", "docker.io/library/rabbitmq:3.9.22", "docker.io"},
		{"redis", "docker.io/library/redis:latest", "docker.io"},
		{"datadog-agent", "gcr.io/datadoghq/agent:7.52.0", "gcr.io"},
		{"nvidia-plugin", "nvcr.io/nvidia/k8s-device-plugin:v0.14.0", "nvcr.io"},
		{"engine-generic-py310", "public.ecr.aws/tensorleap/engine-generic:master-874c3d5d-py310", "public.ecr.aws"},
		{"engine-generic-py38", "public.ecr.aws/tensorleap/engine-generic:master-874c3d5d-py38", "public.ecr.aws"},
		{"engine-generic-py39", "public.ecr.aws/tensorleap/engine-generic:master-874c3d5d-py39", "public.ecr.aws"},
		{"engine", "public.ecr.aws/tensorleap/engine:master-874c3d5d", "public.ecr.aws"},
		{"node-server", "public.ecr.aws/tensorleap/node-server:master-7cb11ea8", "public.ecr.aws"},
		{"pippin", "public.ecr.aws/tensorleap/pippin:master-429ecc32", "public.ecr.aws"},
		{"web-ui", "public.ecr.aws/tensorleap/web-ui:master-c992fe71", "public.ecr.aws"},
		{"keycloak", "quay.io/keycloak/keycloak:17.0.1-legacy", "quay.io"},
		{"minio", "quay.io/minio/minio:RELEASE.2021-12-20T22-07-16Z", "quay.io"},
		{"ingress-nginx-controller", "registry.k8s.io/ingress-nginx/controller:v1.10.0", "registry.k8s.io"},
		{"ingress-nginx-certgen", "registry.k8s.io/ingress-nginx/kube-webhook-certgen:v1.4.0", "registry.k8s.io"},
		{"test", "test", "docker.io"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, err := GetDockerRegistry(tt.image)
			if err != nil {
				t.Errorf("getRegistry(%q) = %v, want %v", tt.image, got, tt.expected)
			}
			if got != tt.expected {
				t.Errorf("getRegistry(%q) = %q, want %q", tt.image, got, tt.expected)
			}
		})
	}
}
