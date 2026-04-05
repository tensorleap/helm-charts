package zot

import (
	"testing"
)

func TestImageToZotRef(t *testing.T) {
	tests := []struct {
		image    string
		wantRepo string
		wantTag  string
	}{
		{"docker.io/library/mongo:6.0.5", "library/mongo", "6.0.5"},
		{"docker.io/library/redis:latest", "library/redis", "latest"},
		{"docker.io/rancher/klipper-lb:v0.4.3", "rancher/klipper-lb", "v0.4.3"},
		{"public.ecr.aws/tensorleap/engine:master-5c2018ec", "tensorleap/engine", "master-5c2018ec"},
		{"registry.k8s.io/ingress-nginx/controller:v1.10.0", "ingress-nginx/controller", "v1.10.0"},
		{"docker.elastic.co/eck/eck-operator:2.8.0", "eck/eck-operator", "2.8.0"},
		{"quay.io/minio/minio:RELEASE.2021-12-20T22-07-16Z", "minio/minio", "RELEASE.2021-12-20T22-07-16Z"},
		{"quay.io/keycloak/keycloak:26.3.2", "keycloak/keycloak", "26.3.2"},
		{"gcr.io/datadoghq/agent:7.52.0", "datadoghq/agent", "7.52.0"},
		{"ghcr.io/project-zot/zot-linux-amd64:v2.1.15", "project-zot/zot-linux-amd64", "v2.1.15"},
		{"alpine:3.18.3", "library/alpine", "3.18.3"},
		// No tag → empty
		{"docker.io/library/mongo", "", ""},
	}

	for _, tt := range tests {
		t.Run(tt.image, func(t *testing.T) {
			gotRepo, gotTag := imageToZotRef(tt.image)
			if gotRepo != tt.wantRepo || gotTag != tt.wantTag {
				t.Errorf("imageToZotRef(%q) = (%q, %q), want (%q, %q)",
					tt.image, gotRepo, gotTag, tt.wantRepo, tt.wantTag)
			}
		})
	}
}

func TestBuildKeepSet(t *testing.T) {
	images := []string{
		"docker.io/library/mongo:6.0.5",
		"public.ecr.aws/tensorleap/engine:master-xxx",
		"alpine:3.18.3",
	}
	keep := buildKeepSet(images)

	if !keep["library/mongo:6.0.5"] {
		t.Error("expected library/mongo:6.0.5 in keep set")
	}
	if !keep["tensorleap/engine:master-xxx"] {
		t.Error("expected tensorleap/engine:master-xxx in keep set")
	}
	if !keep["library/alpine:3.18.3"] {
		t.Error("expected library/alpine:3.18.3 in keep set")
	}
	if keep["library/nginx:1.25-alpine"] {
		t.Error("library/nginx:1.25-alpine should not be in keep set")
	}
}
