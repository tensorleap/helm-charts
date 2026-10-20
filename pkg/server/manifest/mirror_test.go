package manifest

import (
	"strings"
	"testing"
)

func TestMirrorOf(t *testing.T) {
	tests := []struct {
		name  string
		image string
		want  string
	}{
		{"docker hub user image", "docker.io/rancher/mirrored-pause:3.6", MirrorRegistry + "/docker.io/rancher/mirrored-pause:3.6"},
		{"docker hub official image", "docker.io/library/mongo:6.0.5", MirrorRegistry + "/docker.io/library/mongo:6.0.5"},
		{"docker hub short name", "alpine:3.18.3", MirrorRegistry + "/docker.io/library/alpine:3.18.3"},
		{"docker hub without host", "rancher/klipper-lb:v0.4.3", MirrorRegistry + "/docker.io/rancher/klipper-lb:v0.4.3"},
		{"other registry", "registry.k8s.io/ingress-nginx/controller:v1.10.0", MirrorRegistry + "/registry.k8s.io/ingress-nginx/controller:v1.10.0"},
		{"digest", "quay.io/keycloak/keycloak@sha256:" + sixtyFourHex, MirrorRegistry + "/quay.io/keycloak/keycloak@sha256:" + sixtyFourHex},
		{"first-party image", "public.ecr.aws/tensorleap/engine:1.6.100-f8e98ce1", "public.ecr.aws/tensorleap/engine:1.6.100-f8e98ce1"},
		{"already a mirror", MirrorRegistry + "/docker.io/library/mongo:6.0.5", MirrorRegistry + "/docker.io/library/mongo:6.0.5"},
		{"unparsable", "not a ref", "not a ref"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := MirrorOf(tt.image); got != tt.want {
				t.Errorf("MirrorOf(%q) = %q, want %q", tt.image, got, tt.want)
			}
		})
	}
}

const sixtyFourHex = "f3bac0ab3528b8b6c41585952410efda5c090cf705f5eeab853831ab074c3a78"

func TestUsesMirror(t *testing.T) {
	tests := []struct {
		name string
		k3s  string
		want bool
	}{
		{"mirror-era manifest", MirrorRegistry + "/docker.io/rancher/k3s:v1.26.4-k3s1", true},
		{"manifest from before the switch", "docker.io/rancher/k3s:v1.26.4-k3s1", false},
		{"no k3s image", "", false},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			mnf := &InstallationManifest{Images: ManifestImages{InstallationImages: InstallationImages{K3s: tt.k3s}}}
			if got := mnf.UsesMirror(); got != tt.want {
				t.Errorf("UsesMirror() with k3s %q = %v, want %v", tt.k3s, got, tt.want)
			}
		})
	}
}

// The images every new manifest is built from must already point at the mirror.
func TestInstallerImagesUseMirror(t *testing.T) {
	for name, image := range map[string]string{
		"k3s":                    k3sImage,
		"k3dTools":               k3dUtilsImage,
		"register":               registerImage,
		"zot":                    zotImage,
		"checkDockerRequirement": checkDockerRequirement,
	} {
		t.Run(name, func(t *testing.T) {
			if !strings.HasPrefix(image, MirrorRegistry+"/") {
				t.Errorf("%s image %q is not under %s", name, image, MirrorRegistry)
			}
		})
	}
}
