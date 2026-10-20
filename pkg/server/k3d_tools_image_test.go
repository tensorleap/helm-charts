package server

import (
	"os"
	"testing"

	"github.com/tensorleap/helm-charts/pkg/server/manifest"
)

func TestUseK3dToolsImage(t *testing.T) {
	const env = "K3D_IMAGE_TOOLS" // k3d's types.K3dEnvImageTools
	tests := []struct {
		name string
		mnf  *manifest.InstallationManifest
		want string
	}{
		{"installed release's image", &manifest.InstallationManifest{Images: manifest.ManifestImages{
			InstallationImages: manifest.InstallationImages{K3dTools: "ghcr.io/k3d-io/k3d-tools:5.5.2"}}}, "ghcr.io/k3d-io/k3d-tools:5.5.2"},
		{"no manifest", nil, "unchanged"},
		{"manifest without the image", &manifest.InstallationManifest{}, "unchanged"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Setenv(env, "unchanged")
			useK3dToolsImage(tt.mnf)
			if got := os.Getenv(env); got != tt.want {
				t.Errorf("%s = %q, want %q", env, got, tt.want)
			}
		})
	}
}
