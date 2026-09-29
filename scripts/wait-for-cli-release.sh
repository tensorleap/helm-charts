#!/usr/bin/env bash
#
# Wait until leap-cli's "Create release" run for <tag> has published the release
# with its binaries, failing as soon as that run fails.
#
# The tag push starts the run in tensorleap/leap-cli; the release is created by
# its `build` job and the Windows installer is attached by
# `build-windows-installer`. Its `validate-install` job is left to report on its
# own - Release Production runs its own install test against this exact tag.
#
# Usage (GH_TOKEN must read tensorleap/leap-cli):
#   scripts/wait-for-cli-release.sh v0.0.163

set -euo pipefail
# shellcheck source-path=SCRIPTDIR source=lib/release-common.sh
source "$(dirname "${BASH_SOURCE[0]}")/lib/release-common.sh"

REPO="tensorleap/leap-cli"
WORKFLOW="release.yml"
FIND_TIMEOUT=300   # seconds for the run to show up after the tag push
RUN_TIMEOUT=1800   # seconds for the build jobs to finish

tag="${1:-}"
[ -n "$tag" ] || die "usage: $0 <leap-cli tag>"

is_released() {
  local names
  names="$(gh release view "$tag" --repo "$REPO" --json assets --jq '.assets[].name' 2>/dev/null)" || return 1
  grep -Fx "leap-linux-amd64" <<<"$names" >/dev/null \
    && grep -Fx "LeapCLIInstaller_${tag}_windows_amd64.exe" <<<"$names" >/dev/null
}

if is_released; then
  echo "✅ ${tag} is already released: https://github.com/${REPO}/releases/tag/${tag}"
  exit 0
fi

# A tag push lists the tag as the run's head branch.
run_id=""
deadline=$((SECONDS + FIND_TIMEOUT))
while [ -z "$run_id" ]; do
  run_id="$(gh run list --repo "$REPO" --workflow "$WORKFLOW" --event push --limit 30 \
    --json databaseId,headBranch --jq "[.[] | select(.headBranch == \"${tag}\")][0].databaseId // empty")"
  [ -n "$run_id" ] && break
  [ "$SECONDS" -lt "$deadline" ] || die "no \"Create release\" run started in ${REPO} for ${tag} within ${FIND_TIMEOUT}s"
  sleep 10
done
url="https://github.com/${REPO}/actions/runs/${run_id}"
echo "Following ${url}"

deadline=$((SECONDS + RUN_TIMEOUT))
while :; do
  states="$(gh run view "$run_id" --repo "$REPO" --json jobs \
    --jq '.jobs[] | select(.name == "build" or .name == "build-windows-installer") | "\(.name) \(.status) \(.conclusion)"')"
  pending=0
  for job in build build-windows-installer; do
    line="$(grep -E "^${job} " <<<"$states" || true)"
    if [ -z "$line" ]; then
      pending=1
      continue
    fi
    read -r _ status conclusion <<<"$line"
    if [ "$status" != completed ]; then
      pending=1
    elif [ "$conclusion" != success ]; then
      die "leap-cli job '${job}' finished with '${conclusion}': ${url}"
    fi
  done
  [ "$pending" = 0 ] && break
  [ "$SECONDS" -lt "$deadline" ] || die "timed out after ${RUN_TIMEOUT}s waiting for ${url}"
  sleep 20
done

is_released || die "the run finished but release ${tag} is missing its binaries: https://github.com/${REPO}/releases/tag/${tag}"
echo "✅ ${tag} released: https://github.com/${REPO}/releases/tag/${tag}"
