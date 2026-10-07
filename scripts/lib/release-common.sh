#!/usr/bin/env bash
#
# Helpers shared by the release scripts. Source it, don't run it:
#   source "$(dirname "${BASH_SOURCE[0]}")/lib/release-common.sh"
#
# Kept to bash 3.2 (no mapfile, no associative arrays): macOS still ships it and
# these scripts run locally too.

die() { echo "❌ $*" >&2; exit 1; }

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

# Prints 1, 0 or -1 as version $1 is newer than, equal to, or older than $2.
version_cmp() {
  if [ "$1" = "$2" ]; then
    echo 0
  elif [ "$(printf '%s\n%s\n' "$1" "$2" | sort -V | tail -1)" = "$1" ]; then
    echo 1
  else
    echo -1
  fi
}

# Highest production version released so far, from the manifest-X.Y.Z tags.
# Needs the tags fetched (a checkout with fetch-depth: 0).
latest_production_version() {
  git tag -l 'manifest-*' \
    | sed -n 's/^manifest-\([0-9][0-9]*\.[0-9][0-9]*\.[0-9][0-9]*\)$/\1/p' \
    | sort -V | tail -1
}

# The installer (Go module) version a checkout embeds, e.g. v0.10.19.
installer_version_of() {
  local file="$1/pkg/version/version.go" version
  version="$(sed -n 's/^const Version = "\(v[0-9][0-9]*\.[0-9][0-9]*\.[0-9][0-9]*\)"$/\1/p' "$file")"
  [ -n "$version" ] || die "cannot read the installer version from $file"
  printf '%s' "$version"
}

# The files leap-cli embeds from this module. version.go is included on purpose:
# reusing a tag has to mean reusing the Version constant the CLI reports too.
INSTALLER_PATHS=('*.go' ':(exclude)*_test.go' go.mod go.sum pkg/helm/resources)

# Succeeds when <tag> holds the same installer code as <commit> (default HEAD).
installer_code_matches() {
  git diff --quiet "$1" "${2:-HEAD}" -- "${INSTALLER_PATHS[@]}"
}

# Full commit sha for a short sha in tensorleap/<repo>.
full_sha() {
  local repo="$1" short="$2" sha
  sha="$(gh api "repos/tensorleap/${repo}/commits/${short}" --jq .sha)" \
    || die "cannot resolve ${repo}@${short} (is GH_TOKEN allowed to read tensorleap/${repo}?)"
  printf '%s' "$sha"
}

# Raw content of tensorleap/<repo>:<path> at <ref>.
gh_raw() {
  local repo="$1" path="$2" ref="$3"
  gh api -H 'Accept: application/vnd.github.raw' "repos/tensorleap/${repo}/contents/${path}?ref=${ref}" \
    || die "cannot read ${repo}:${path} at ${ref}"
}
