#!/usr/bin/env bash
#
# Decide which installer (Go module) version a production release ships, so the
# leap-cli released with it pins exactly the installer code being released.
#
# The module is published purely through git tags `vA.B.C`, which have to equal
# `const Version` in pkg/version/version.go. Nothing enforced that, and because
# tags were also cut on side branches, releases kept shipping installer code
# that differs from the tag their version.go names - 1.6.74 differs from
# v0.10.16 in 7 files. So the release decides, at the commit being released:
#
#   create  tag v<version.go> does not exist yet    -> tag this release commit
#   reuse   it exists and holds this installer code -> nothing to do
#   bump    it exists with different code           -> the next free vA.B.* patch,
#           written into version.go so the manifest's installerVersion, `leap
#           server --info` and the tag all agree
#
# Minor bumps stay a developer's call: a minor change forces every older CLI to
# upgrade (see ValidateInstallerVersion in pkg/server/checks.go).
#
# Usage (from the repo root, at the commit being released):
#   scripts/resolve-installer-version.sh            # may rewrite version.go
#   DRY_RUN=1 scripts/resolve-installer-version.sh  # plan only
#
# Writes installer_version and installer_action to $GITHUB_OUTPUT when set.
# Tagging is left to the caller, after the release commit exists.

set -euo pipefail
# shellcheck source-path=SCRIPTDIR source=lib/release-common.sh
source "$(dirname "${BASH_SOURCE[0]}")/lib/release-common.sh"

VERSION_FILE="pkg/version/version.go"
DRY_RUN="${DRY_RUN:-}"

[ -f "$VERSION_FILE" ] || die "run from the repo root: $VERSION_FILE not found"

current="$(installer_version_of .)"
[[ "$current" =~ ^v([0-9]+)\.([0-9]+)\.[0-9]+$ ]] || die "unexpected installer version '$current'"
series="v${BASH_REMATCH[1]}.${BASH_REMATCH[2]}"

tag_exists() { git rev-parse -q --verify "refs/tags/$1" >/dev/null; }

if ! tag_exists "$current"; then
  action=create
  target="$current"
  reason="tag $current does not exist yet"
elif installer_code_matches "$current"; then
  action=reuse
  target="$current"
  reason="tag $current already holds this installer code"
else
  changed="$(git diff --name-only "$current" HEAD -- "${INSTALLER_PATHS[@]}" | wc -l | tr -d ' ')"
  # Count every tag in the series, including ones cut on side branches
  # (v0.10.12 and v0.10.14 exist only there), so the new one is really free.
  last="$(git tag -l "${series}.*" | sed -n "s/^${series//./\\.}\.\([0-9][0-9]*\)$/\1/p" | sort -n | tail -1)"
  action=bump
  target="${series}.$((last + 1))"
  reason="tag $current holds different installer code ($changed files differ)"
fi

if [ "$action" = bump ]; then
  if [ -n "$DRY_RUN" ]; then
    echo "DRY_RUN set - would write $target into $VERSION_FILE"
  else
    sed -i.bak "s/^const Version = \"${current}\"$/const Version = \"${target}\"/" "$VERSION_FILE"
    rm -f "${VERSION_FILE}.bak"
    [ "$(installer_version_of .)" = "$target" ] || die "failed to write $target into $VERSION_FILE"
  fi
fi

echo "Installer version: $target ($action: $reason)"

if [ -n "${GITHUB_OUTPUT:-}" ]; then
  echo "installer_version=$target" >> "$GITHUB_OUTPUT"
  echo "installer_action=$action" >> "$GITHUB_OUTPUT"
fi
if [ -n "${GITHUB_STEP_SUMMARY:-}" ]; then
  echo "**Installer:** \`$target\` - $action ($reason)" >> "$GITHUB_STEP_SUMMARY"
fi
