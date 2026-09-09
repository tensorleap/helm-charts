#!/usr/bin/env bash
#
# Repoint the charts at version-numbered image tags for a production release.
#
# Nothing in the release path resolves image tags: a release ships whatever
# `image_tag:` its branch inherited, which is `master-<sha>` from the Update
# Images workflow unless the Patch workflow rewrote it to `<branch>-<sha>`.
# That left `master-*` tags load-bearing for published releases and impossible
# to garbage-collect. This script rewrites the prefix to the release version,
# keeping the commit sha that is already pinned - so `master-<sha>` becomes
# `<version>-<sha>`, pointing at the same commit's image.
#
# It deliberately does NOT re-resolve the version branch's HEAD the way the
# Patch workflow does. Those branches move: engine's 1.6.71 branch head was
# three commits ahead of what release 1.6.71 actually shipped, so re-resolving
# would silently swap in untested commits.
#
# pippin is intentionally left on its `master-*` tag: it has no version
# branches (create-external-rc-branches covers engine/node-server/web-ui/
# leap-cli only), so there is no `<version>-<sha>` pippin image to point at.
#
# Usage:
#   VERSION=1.6.75 scripts/pin-release-image-tags.sh
#   VERSION=1.6.75 DRY_RUN=1 scripts/pin-release-image-tags.sh   # plan only

set -euo pipefail

REGISTRY_PATH="tensorleap"
ENGINE_VALUES="charts/tensorleap/charts/engine/values.yaml"
NODE_SERVER_VALUES="charts/tensorleap/charts/node-server/values.yaml"
WEB_UI_VALUES="charts/tensorleap/charts/web-ui/values.yaml"
# The engine chart derives one engine-generic image per Python base from
# engine's image_tag, so every variant has to resolve too.
GENERIC_TEMPLATE="charts/tensorleap/charts/engine/templates/engine-job-config.yaml"

VERSION="${VERSION:-}"
DRY_RUN="${DRY_RUN:-}"

die() { echo "❌ $*" >&2; exit 1; }

[ -n "$VERSION" ] || die "VERSION is required (e.g. VERSION=1.6.75)"
[ -f "$ENGINE_VALUES" ] || die "run from the repo root: $ENGINE_VALUES not found"

# An rc version is not what gets released, and a `1.6.75-rc.1-<sha>` tag would
# be indistinguishable from a real one.
case "$VERSION" in
  *-rc.*) die "VERSION must have no -rc suffix (got $VERSION); run remove-rc-suffix first" ;;
esac

# ---------------------------------------------------------------- helpers ----

# Both tag writers emit `<ref>-<sha8>`, so the commit is always the last
# dash-separated field. Refuse anything else rather than invent a tag.
sha_of_tag() {
  local tag="$1" sha="${1##*-}"
  [[ "$sha" =~ ^[0-9a-f]{8}$ ]] || die "cannot read a commit sha out of tag '$tag'"
  printf '%s' "$sha"
}

read_value() {
  local file="$1" key="$2" value
  value="$(grep -E "^${key}: " "$file" | head -1 | awk '{print $2}')"
  [ -n "$value" ] || die "no '${key}:' in $file"
  printf '%s' "$value"
}

write_value() {
  local file="$1" key="$2" value="$3"
  sed -i.bak "s|^${key}: .*|${key}: ${value}|" "$file"
  rm -f "${file}.bak"
}

# public ECR needs a bearer token even for anonymous pulls. Cache one per repo.
ecr_token() {
  local repo="$1" var token
  var="ECR_TOKEN_$(echo "$repo" | tr '.-' '__')"
  token="$(eval "printf '%s' \"\${${var}:-}\"")"
  if [ -z "$token" ]; then
    token="$(curl -sS --retry 3 --retry-delay 2 \
      "https://public.ecr.aws/token/?scope=repository:${REGISTRY_PATH}/${repo}:pull&service=public.ecr.aws" \
      | sed -n 's/.*"token":"\([^"]*\)".*/\1/p')"
    [ -n "$token" ] || die "could not get an anonymous public ECR token for ${repo}"
    eval "${var}=\$token"
  fi
  printf '%s' "$token"
}

# Public ECR rate-limits anonymous manifest reads, so retry a 429 rather than
# failing a release over it.
tag_exists() {
  local repo="$1" tag="$2" token code attempt
  token="$(ecr_token "$repo")"
  for attempt in 1 2 3 4 5; do
    code="$(curl -sS -o /dev/null -w '%{http_code}' \
      -H "Authorization: Bearer ${token}" \
      -H "Accept: application/vnd.oci.image.index.v1+json,application/vnd.docker.distribution.manifest.list.v2+json,application/vnd.docker.distribution.manifest.v2+json" \
      "https://public.ecr.aws/v2/${REGISTRY_PATH}/${repo}/manifests/${tag}")"
    case "$code" in
      200) return 0 ;;
      404) return 1 ;;
      429|5??) sleep $((attempt * 3)) ;;
      *) die "unexpected HTTP $code checking ${repo}:${tag}" ;;
    esac
  done
  die "public ECR kept returning HTTP $code for ${repo}:${tag}"
}

# Verify the version-prefixed tag is really published before pointing a release
# at it. A missing tag means the version branch in that repo was never built at
# this commit - run the Patch workflow on the branch and release from that.
require_tag() {
  local repo="$1" old="$2" new="$3"
  if [ "$old" = "$new" ]; then
    echo "  = ${repo}:${new} already version-pinned"
    return
  fi
  if tag_exists "$repo" "$new"; then
    echo "  + ${repo}:${old} -> ${new}"
  else
    die "public.ecr.aws/${REGISTRY_PATH}/${repo}:${new} is not published.
   The release currently pins '${old}'. That commit was never built under the
   '${VERSION}' prefix, so there is no version-numbered image to point at.
   Usually this means the release is being cut from master rather than from
   version branch ${VERSION}, or the ${repo} repo has no ${VERSION} branch.
   Fix: release from branch ${VERSION}, or run the Patch workflow on it first
   (that cuts the external branches and repins from their builds)."
  fi
}

# ------------------------------------------------------------------- plan ----

# Resolve and verify everything before writing anything, so a missing tag
# cannot leave the charts half-rewritten.
engine_old="$(read_value "$ENGINE_VALUES" image_tag)"
engine_new="${VERSION}-$(sha_of_tag "$engine_old")"

node_server_old="$(read_value "$NODE_SERVER_VALUES" image_tag)"
node_server_new="${VERSION}-$(sha_of_tag "$node_server_old")"

web_ui_old="$(read_value "$WEB_UI_VALUES" image_tag)"
web_ui_new="${VERSION}-$(sha_of_tag "$web_ui_old")"

# Read the suffixes out of the template rather than hardcoding them, so a new
# Python base added to the chart is covered without touching this script.
# Deliberately not mapfile: macOS still ships bash 3.2 and this runs locally too.
py_suffixes="$(grep -oE 'image_tag }}-py[0-9]+' "$GENERIC_TEMPLATE" | sed 's/.*}}-//' | sort -u)"
[ -n "$py_suffixes" ] || die "found no engine-generic -py suffixes in $GENERIC_TEMPLATE"

echo "Pinning release $VERSION to version-numbered image tags:"
require_tag engine "$engine_old" "$engine_new"
for suffix in $py_suffixes; do
  require_tag engine-generic "${engine_old}-${suffix}" "${engine_new}-${suffix}"
done
require_tag node-server "$node_server_old" "$node_server_new"
require_tag web-ui "$web_ui_old" "$web_ui_new"

pippin_tag="$(read_value "$ENGINE_VALUES" dependencies_image_tag)"
echo "  · pippin:${pippin_tag} left as is (no version branches in the pippin repo)"

if [ -n "$DRY_RUN" ]; then
  echo "DRY_RUN set - charts left untouched"
  exit 0
fi

# ------------------------------------------------------------------ apply ----

write_value "$ENGINE_VALUES" image_tag "$engine_new"
write_value "$NODE_SERVER_VALUES" image_tag "$node_server_new"
write_value "$WEB_UI_VALUES" image_tag "$web_ui_new"

printf '%s' "public.ecr.aws/${REGISTRY_PATH}/engine:${engine_new}" > engine-latest-image
printf '%s' "public.ecr.aws/${REGISTRY_PATH}/node-server:${node_server_new}" > node-server-latest-image
printf '%s' "public.ecr.aws/${REGISTRY_PATH}/web-ui:${web_ui_new}" > web-ui-latest-image

echo "Charts repinned for $VERSION"
