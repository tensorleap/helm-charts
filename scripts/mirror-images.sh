#!/usr/bin/env bash
#
# Copy every third-party image the product installs into our public registry:
#
#   docker.io/library/mongo:6.0.5  ->  public.ecr.aws/tensorleap/mirror/docker.io/library/mongo:6.0.5
#
# so a vendor deleting or locking an image (quay.io/minio, Sep 2026) can't break
# installs or the airgap pack.
#
# The image list is the installer's own: `go run . create-manifest --local`, i.e.
# images.txt plus the installer images plus the k3s system images - the same set
# pack and install pull, so the mirror can't drift from them. Each ref is handled
# by one rule:
#
#   public.ecr.aws/tensorleap/mirror/<x>  copied from <x>
#   other public.ecr.aws/tensorleap/*     first-party, skipped
#   anything else (a vendor ref)          copied to public.ecr.aws/tensorleap/mirror/<x>
#
# A mirror tag is never overwritten: ECR Public has no tag immutability, and the
# copy is meant to be a frozen snapshot (floating upstream tags such as
# buildx-stable-1 stay as they were when first copied). Only linux/amd64 and
# linux/arm64 are copied. When upstream has nothing else the copy is
# byte-for-byte, same digest; otherwise the index is filtered (dropping e.g. the
# Windows images Docker Hub ships) and each platform image keeps upstream's digest.
#
# Usage (from the repo root):
#   scripts/mirror-images.sh                 create missing repos, copy missing images
#   scripts/mirror-images.sh --verify        change nothing; fail if a mirror is missing,
#                                            warn when upstream moved or disappeared
#   scripts/mirror-images.sh --dry-run       print what would be copied
#   scripts/mirror-images.sh --images-file F use the refs in F (one per line, # comments)
#                                            instead of the manifest
#
# Copying needs AWS credentials that may push to mirror/* in the utils account
# (CI: the github-actions-ecr-public-mirror role; locally: AWS_PROFILE=utils).
# --verify and --dry-run are anonymous. Needs crane and jq, plus go and yq unless
# --images-file is given. CI wrapper: .github/workflows/mirror_images.yml
#
# Kept to bash 3.2 (no mapfile, no associative arrays): macOS still ships it.

set -euo pipefail
# shellcheck source-path=SCRIPTDIR source=lib/release-common.sh
source "$(dirname "${BASH_SOURCE[0]}")/lib/release-common.sh"

FIRST_PARTY_PREFIX="public.ecr.aws/tensorleap/"
MIRROR_PREFIX="public.ecr.aws/tensorleap/mirror/"
MIRROR_REPO_PREFIX="mirror/" # the repository name inside the registry alias "tensorleap"
ECR_PUBLIC_REGION="us-east-1"
PLATFORMS="linux/amd64 linux/arm64"
MAX_ATTEMPTS=5

MODE=copy
IMAGES_FILE=""
while [ $# -gt 0 ]; do
  case "$1" in
    --verify) MODE=verify ;;
    --dry-run) MODE=dry-run ;;
    --images-file) IMAGES_FILE="${2:-}"; [ -n "$IMAGES_FILE" ] || die "--images-file needs a path"; shift ;;
    -h|--help) sed -n '2,/^# Kept to bash/p' "$0" | sed 's/^# \{0,1\}//'; exit 0 ;;
    *) die "unknown argument: $1 (see --help)" ;;
  esac
  shift
done

for tool in crane jq; do
  command -v "$tool" >/dev/null || die "$tool is required"
done
[ "$MODE" != copy ] || command -v aws >/dev/null || die "aws is required to copy"

WORK="$(mktemp -d)"
trap 'rm -rf "$WORK"' EXIT

# --- image list -------------------------------------------------------------

list_images() {
  if [ -n "$IMAGES_FILE" ]; then
    sed 's/#.*//; s/[[:space:]]//g' "$IMAGES_FILE" | grep -v '^$' | sort -u
    return
  fi
  command -v go >/dev/null || die "go is required to build the image list (or pass --images-file)"
  command -v yq >/dev/null || die "yq is required to read the manifest (or pass --images-file)"
  go run . create-manifest --local -o "$WORK/manifest.yaml" >"$WORK/manifest.log" 2>&1 ||
    { cat "$WORK/manifest.log" >&2; die "go run . create-manifest --local failed"; }
  yq '.images | .. | select(tag == "!!str")' "$WORK/manifest.yaml" | grep -v '^$' | sort -u
}

# docker.io's implicit forms, written out: alpine:3 -> docker.io/library/alpine:3,
# rancher/k3s:v1 -> docker.io/rancher/k3s:v1. Anything with a registry host is kept.
normalize() {
  local ref="$1" first="${1%%/*}"
  if [ "$ref" = "$first" ] || { [[ "$first" != *.* ]] && [[ "$first" != *:* ]] && [ "$first" != localhost ]; }; then
    ref="docker.io/$ref"
  fi
  ref="${ref/#index.docker.io\//docker.io/}"
  local path="${ref#docker.io/}"
  if [ "$path" != "$ref" ] && [[ "$path" != */* ]]; then
    ref="docker.io/library/$path"
  fi
  printf '%s' "$ref"
}

# --- registry helpers -------------------------------------------------------

is_not_found() {
  case "$1" in
    *MANIFEST_UNKNOWN* | *NAME_UNKNOWN* | *"status code 404"* | *"404 Not Found"* | *RepositoryNotFound*) return 0 ;;
  esac
  return 1
}

# Prints the digest of $1 (with --platform $2 when given), "missing" when the
# registry says it doesn't exist. Retries anything else - public ECR answers 429
# to bursts of anonymous reads - and fails only when that keeps happening.
digest_of() {
  local ref="$1" platform="${2:-}" out attempt
  for attempt in $(seq 1 "$MAX_ATTEMPTS"); do
    if [ -n "$platform" ]; then
      out="$(crane digest --platform "$platform" "$ref" 2>&1)" && { printf '%s' "$out"; return 0; }
    else
      out="$(crane digest "$ref" 2>&1)" && { printf '%s' "$out"; return 0; }
    fi
    if is_not_found "$out"; then printf 'missing'; return 0; fi
    [ "$attempt" -lt "$MAX_ATTEMPTS" ] && sleep $((attempt * attempt * 2))
  done
  echo "$out" >&2
  return 1
}

# Runs "$@" until it succeeds, backing off between attempts.
with_retry() {
  local attempt
  for attempt in $(seq 1 "$MAX_ATTEMPTS"); do
    "$@" && return 0
    [ "$attempt" -lt "$MAX_ATTEMPTS" ] && sleep $((attempt * attempt * 2))
  done
  return 1
}

# The platforms of an index, one "os/arch" per line; nothing for a single image.
platforms_of() {
  crane manifest "$1" | jq -r '.manifests[]?.platform | "\(.os)/\(.architecture)"' | sort -u
}

ensure_repo() {
  local name="$1" upstream="$2" out
  out="$(aws ecr-public describe-repositories --region "$ECR_PUBLIC_REGION" \
    --repository-names "$name" 2>&1)" && return 0
  case "$out" in *RepositoryNotFoundException*) ;; *) echo "$out" >&2; return 1 ;; esac
  out="$(aws ecr-public create-repository --region "$ECR_PUBLIC_REGION" --repository-name "$name" \
    --catalog-data "description=Unmodified mirror of ${upstream} (third-party image used by Tensorleap installs)" \
    2>&1)" && return 0
  # Two runs racing to create the same repository is fine.
  case "$out" in *RepositoryAlreadyExistsException*) return 0 ;; esac
  echo "$out" >&2
  return 1
}

login() {
  # A private docker config so a local run doesn't write into ~/.docker.
  export DOCKER_CONFIG="$WORK/docker"
  mkdir -p "$DOCKER_CONFIG"
  aws ecr-public get-login-password --region "$ECR_PUBLIC_REGION" |
    crane auth login public.ecr.aws -u AWS --password-stdin >/dev/null 2>&1 ||
    die "cannot log in to public.ecr.aws - are AWS credentials for the utils account set?"
}

# Copies $1 to $2: byte-for-byte when upstream only has the platforms we keep,
# otherwise an index filtered down to them.
copy_image() {
  local src="$1" dst="$2" platforms extra p
  platforms="$(with_retry platforms_of "$src")" || return 1
  extra=""
  for p in $platforms; do
    case " $PLATFORMS " in *" $p "*) ;; *) extra="$extra $p" ;; esac
  done
  if [ -z "$extra" ]; then
    with_retry crane copy "$src" "$dst" >/dev/null 2>"$WORK/copy.err" || { cat "$WORK/copy.err" >&2; return 1; }
    return 0
  fi
  local kept="" args=""
  for p in $PLATFORMS; do
    case " $(echo $platforms) " in *" $p "*) kept="$kept $p"; args="$args --platform $p" ;; esac
  done
  [ -n "$kept" ] || { echo "upstream has none of: $PLATFORMS (only:$(echo " $platforms"))" >&2; return 1; }
  # shellcheck disable=SC2086 # $args is a list of flags
  with_retry crane index filter "$src" $args -t "$dst" >/dev/null 2>"$WORK/copy.err" ||
    { cat "$WORK/copy.err" >&2; return 1; }
}

# Compares the kept platforms of $1 (upstream) and $2 (mirror). Prints
# "same", "moved", "gone" (upstream no longer has it) or "error".
compare() {
  local src="$1" dst="$2" platforms p u m
  u="$(digest_of "$src")" || { printf 'error'; return 0; }
  [ "$u" != missing ] || { printf 'gone'; return 0; }
  m="$(digest_of "$dst")" || { printf 'error'; return 0; }
  [ "$u" != "$m" ] || { printf 'same'; return 0; }
  platforms="$(with_retry platforms_of "$src")" || { printf 'error'; return 0; }
  [ -n "$platforms" ] || { printf 'moved'; return 0; } # single image with another digest
  for p in $PLATFORMS; do
    case " $(echo $platforms) " in *" $p "*) ;; *) continue ;; esac
    u="$(digest_of "$src" "$p")" || { printf 'error'; return 0; }
    m="$(digest_of "$dst" "$p")" || { printf 'error'; return 0; }
    [ "$u" = "$m" ] || { printf 'moved'; return 0; }
  done
  printf 'same'
}

# --- main ---------------------------------------------------------------------

[ "$MODE" != copy ] || login

ROWS=""
FAILED=0
add_row() { ROWS="${ROWS}| $1 | \`$2\` | $3 |"$'\n'; echo "$1  $2  $3"; }

images="$(list_images)"
[ -n "$images" ] || die "no images found"
echo "Mode: $MODE - $(echo "$images" | wc -l | tr -d ' ') images in the list"

for ref in $images; do
  [[ "$ref" != *@* ]] || { add_row "❌ unsupported" "$ref" "digest refs are not mirrored"; FAILED=1; continue; }
  if [[ "$ref" == "$MIRROR_PREFIX"* ]]; then
    dst="$ref"
    src="${ref#"$MIRROR_PREFIX"}"
  elif [[ "$ref" == "$FIRST_PARTY_PREFIX"* ]]; then
    continue
  else
    src="$(normalize "$ref")"
    dst="${MIRROR_PREFIX}${src}"
  fi
  repo="${MIRROR_REPO_PREFIX}${src%:*}"

  if ! state="$(digest_of "$dst")"; then
    add_row "❌ error" "$dst" "cannot read the mirror"; FAILED=1; continue
  fi

  case "$MODE:$state" in
    verify:missing)
      add_row "❌ missing" "$dst" "not mirrored yet"; FAILED=1 ;;
    verify:*)
      case "$(compare "$src" "$dst")" in
        same) add_row "✅ ok" "$dst" "matches upstream" ;;
        moved) add_row "⚠️ upstream moved" "$dst" "\`$src\` now points elsewhere; the mirror keeps the copied snapshot" ;;
        gone) add_row "⚠️ upstream gone" "$dst" "\`$src\` no longer exists upstream; the mirror still has it" ;;
        *) add_row "⚠️ unchecked" "$dst" "could not read \`$src\`" ;;
      esac ;;
    dry-run:missing)
      add_row "➕ would copy" "$dst" "from \`$src\`" ;;
    copy:missing)
      if ensure_repo "$repo" "$src" && copy_image "$src" "$dst"; then
        add_row "➕ copied" "$dst" "from \`$src\`"
      else
        add_row "❌ copy failed" "$dst" "from \`$src\`"; FAILED=1
      fi ;;
    *)
      add_row "✅ present" "$dst" "already mirrored, left as is" ;;
  esac
done

if [ -n "${GITHUB_STEP_SUMMARY:-}" ]; then
  {
    echo "### Third-party image mirror ($MODE)"
    echo
    echo "| Status | Mirror | Note |"
    echo "|---|---|---|"
    printf '%s' "$ROWS"
  } >>"$GITHUB_STEP_SUMMARY"
fi

[ "$FAILED" -eq 0 ] || die "some images are not mirrored - see above"
echo "✅ done"
