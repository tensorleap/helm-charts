package manifest

import (
	"strings"

	ggcr "github.com/google/go-containerregistry/pkg/name"
)

// MirrorRegistry holds byte-for-byte copies of the third-party images the
// product installs, at <MirrorRegistry>/<upstream-host>/<upstream-path>:<tag>,
// e.g. public.ecr.aws/tensorleap/mirror/docker.io/library/mongo:6.0.5, so a
// vendor deleting or locking an image (quay.io/minio, Sep 2026) can't break
// installs. Images are copied by hand: see "Third-party images" in
// DEVELOPER-GUIDE.md.
const MirrorRegistry = "public.ecr.aws/tensorleap/mirror"

const firstPartyRegistry = "public.ecr.aws/tensorleap/"

// UsesMirror reports whether this manifest takes its third-party images from
// MirrorRegistry. Manifests published before the switch don't, and are
// installed exactly as before.
func (mnf *InstallationManifest) UsesMirror() bool {
	return strings.HasPrefix(mnf.Images.K3s, MirrorRegistry+"/")
}

// MirrorOf returns where MirrorRegistry keeps the copy of a third-party image:
//
//	docker.io/rancher/mirrored-pause:3.6 -> public.ecr.aws/tensorleap/mirror/docker.io/rancher/mirrored-pause:3.6
//	alpine:3.18.3                        -> public.ecr.aws/tensorleap/mirror/docker.io/library/alpine:3.18.3
//
// Our own images (public.ecr.aws/tensorleap/..., mirrors included) and refs
// that don't parse are returned unchanged.
func MirrorOf(image string) string {
	if strings.HasPrefix(image, firstPartyRegistry) {
		return image
	}
	ref, err := ggcr.ParseReference(image)
	if err != nil {
		return image
	}
	host := ref.Context().RegistryStr()
	if host == ggcr.DefaultRegistry {
		host = "docker.io"
	}
	separator := ":"
	if _, isDigest := ref.(ggcr.Digest); isDigest {
		separator = "@"
	}
	return MirrorRegistry + "/" + host + "/" + ref.Context().RepositoryStr() + separator + ref.Identifier()
}
