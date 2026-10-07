#!/usr/bin/env bash
#
# Regenerate leap-cli's Go API client (pkg/tensorleapapi) from the node-server
# image a release ships.
#
# leap-cli's `make update-server-api` rebuilds node-server's builder image from
# source, which needs NPM_TOKEN for node-server's private packages. The
# published runtime image already carries the OpenAPI spec the server serves
# (/usr/app/generated/swagger.json, copied from the builder stage), so read it
# from there and run node-server's own generator script, fetched at the same
# commit, in the node:22-alpine + OpenJDK 17 environment node-server's builder
# uses. No NPM_TOKEN, and the client matches the exact server being released.
#
# Usage (GH_TOKEN must read tensorleap/node-server; needs docker, gh, jq, make
# and gofmt):
#   scripts/regen-leap-cli-api.sh <node-server image tag> <leap-cli checkout>
#   e.g. scripts/regen-leap-cli-api.sh 1.6.85-bcfd4699 ../leap-cli
#
# Writes previous_api_version and api_version to $GITHUB_OUTPUT when set.

set -euo pipefail
# shellcheck source-path=SCRIPTDIR source=lib/release-common.sh
source "$(dirname "${BASH_SOURCE[0]}")/lib/release-common.sh"

IMAGE_REPO="public.ecr.aws/tensorleap/node-server"
# Same base and JRE as node-server's builder stage (its Dockerfile).
GENERATOR_IMAGE="node:22-alpine"
CLIENT_DIR="pkg/tensorleapapi"

tag="${1:-}"
leap_cli="${2:-}"
[ -n "$tag" ] && [ -n "$leap_cli" ] || die "usage: $0 <node-server image tag> <leap-cli checkout>"
[ -f "$leap_cli/go.mod" ] && [ -d "$leap_cli/$CLIENT_DIR" ] || die "$leap_cli is not a leap-cli checkout"
for tool in docker gh jq make; do command -v "$tool" >/dev/null || die "$tool is required"; done

api_version_of() {
  awk '/^  version: / { print $2; exit }' "$1/$CLIENT_DIR/api/openapi.yaml"
}

sha="$(full_sha node-server "$(sha_of_tag "$tag")")"
previous="$(api_version_of "$leap_cli")"

work="$(mktemp -d)"
trap 'rm -rf "$work"' EXIT
mkdir -p "$work/generated" "$work/scripts"

echo "Reading the OpenAPI spec from ${IMAGE_REPO}:${tag}"
docker pull -q --platform linux/amd64 "${IMAGE_REPO}:${tag}" >/dev/null \
  || die "cannot pull ${IMAGE_REPO}:${tag} - if node-server's CI for ${sha:0:8} is still running, wait for it and re-run"
cid="$(docker create --platform linux/amd64 "${IMAGE_REPO}:${tag}")"
if ! docker cp "${cid}:/usr/app/generated/swagger.json" "$work/generated/swagger.json"; then
  docker rm "$cid" >/dev/null
  die "${IMAGE_REPO}:${tag} has no /usr/app/generated/swagger.json - did node-server's Dockerfile change?"
fi
docker rm "$cid" >/dev/null

echo "Fetching node-server's Go client generator at ${sha:0:8}"
gh_raw node-server scripts/generate-go-client.sh "$sha" > "$work/scripts/generate-go-client.sh"
chmod +x "$work/scripts/generate-go-client.sh"
gh_raw node-server openapitools-go.json "$sha" > "$work/openapitools-go.json"
wrapper="$(gh_raw node-server package.json "$sha" | jq -r '.devDependencies["@openapitools/openapi-generator-cli"] // empty')"
[ -n "$wrapper" ] || die "node-server@${sha:0:8} has no @openapitools/openapi-generator-cli devDependency"

# A throwaway package.json keeps npx on the locally installed wrapper and away
# from node-server's private dependencies. Files are handed back to the caller's
# uid so the cleanup trap can remove them on Linux too.
echo "Generating the client with @openapitools/openapi-generator-cli@${wrapper}"
docker run --rm -v "$work:/work" -w /work "$GENERATOR_IMAGE" sh -c "
  trap 'chown -R $(id -u):$(id -g) /work' EXIT
  set -e
  apk add --no-cache -q openjdk17-jre-headless bash >/dev/null
  echo '{\"private\": true}' > package.json
  npm install --no-save --no-audit --no-fund --loglevel=error '@openapitools/openapi-generator-cli@${wrapper}'
  ./scripts/generate-go-client.sh
"
[ -f "$work/generated/tensorleapapi/client.go" ] || die "the generator produced no client"

rm -rf "${leap_cli:?}/$CLIENT_DIR"
cp -R "$work/generated/tensorleapapi" "$leap_cli/$CLIENT_DIR"
make -C "$leap_cli" fmt >/dev/null
current="$(api_version_of "$leap_cli")"
[ -n "$current" ] || die "cannot read the API version from $leap_cli/$CLIENT_DIR/api/openapi.yaml"

echo "leap-cli API client: node-server ${previous:-?} -> ${current} (${tag})"
if [ -n "${GITHUB_OUTPUT:-}" ]; then
  echo "previous_api_version=${previous}" >> "$GITHUB_OUTPUT"
  echo "api_version=${current}" >> "$GITHUB_OUTPUT"
fi
