#!/usr/bin/env bash
#
# Ship the current skills with a leap-cli release: tag tensorleap/skills and pin
# leap-cli to that tag.
#
# A leap CLI names the skills it ships with in pkg/version/skills-ref. `leap mcp
# config` installs that tag and the skills update themselves to it, so users get
# new skills only through a CLI release. Tagging the skills and editing the file
# were hand steps: skip them and the release ships old skills, or fails in
# leap-cli's "Create release" because the tag the file names was never made. So
# each CLI release takes the skills checkout as it is (main's head):
#
#   reuse   that commit already carries a skills-vN tag -> pin that tag
#   create  it doesn't -> tag it with the next free skills-vN and pin that
#
# Before anything is pushed the skills are checked against the CLI being
# released, the way leap-cli's "Create release" checks them again after the tag:
# no leap command or flag they call may be missing, and no JSON output field
# they read may have gone since the previous CLI release.
#
# Usage (the skills checkout's credentials must be allowed to push tags; needs
# git and go):
#   scripts/resolve-skills-ref.sh <skills checkout> <leap-cli checkout>
#   DRY_RUN=1 scripts/resolve-skills-ref.sh ...   # plan and check; push nothing
#
# Both modes write the tag into the leap-cli checkout's skills-ref; committing
# it is left to the caller. VERSION (the Tensorleap version) goes into the tag
# message when set. Writes skills_ref, skills_action and skills_commit to
# $GITHUB_OUTPUT when set. A leap-cli branch from before skills were pinned has
# no skills-ref: nothing is tagged and skills_ref is empty.

set -euo pipefail
# shellcheck source-path=SCRIPTDIR source=lib/release-common.sh
source "$(dirname "${BASH_SOURCE[0]}")/lib/release-common.sh"

REF_FILE="pkg/version/skills-ref"
CHECKER="scripts/skillscheck"
TAG_PREFIX="skills-v"
DRY_RUN="${DRY_RUN:-}"

skills="${1:-}"
leap_cli="${2:-}"
[ -n "$skills" ] && [ -n "$leap_cli" ] || die "usage: $0 <skills checkout> <leap-cli checkout>"
[ -d "$skills/skills" ] || die "$skills is not a tensorleap/skills checkout"
[ -f "$leap_cli/go.mod" ] || die "$leap_cli is not a leap-cli checkout"
skills="$(cd "$skills" && pwd)"
leap_cli="$(cd "$leap_cli" && pwd)"

write_outputs() {
  [ -n "${GITHUB_OUTPUT:-}" ] || return 0
  {
    echo "skills_ref=$1"
    echo "skills_action=$2"
    echo "skills_commit=$3"
  } >> "$GITHUB_OUTPUT"
}

if [ ! -f "$leap_cli/$REF_FILE" ]; then
  echo "::notice::this leap-cli branch has no $REF_FILE (it predates pinned skills); no skills are tagged for it"
  write_outputs "" none ""
  exit 0
fi
pinned="$(tr -d '[:space:]' < "$leap_cli/$REF_FILE")"

commit="$(git -C "$skills" rev-parse HEAD)"
short="$(git -C "$skills" rev-parse --short HEAD)"

# Read the tags from the remote, so a shallow or stale checkout can't hide one.
# A lightweight tag lists the commit itself; an annotated one lists it on its
# peeled `^{}` line.
remote_tags="$(git -C "$skills" ls-remote --tags origin "refs/tags/${TAG_PREFIX}*")" \
  || die "cannot list the tags of tensorleap/skills"
target="$(awk -v commit="$commit" '$1 == commit { print $2 }' <<<"$remote_tags" \
  | sed -n "s#^refs/tags/${TAG_PREFIX}\([0-9][0-9]*\)\(\^{}\)\{0,1\}\$#\1#p" | sort -n | tail -1)"
if [ -n "$target" ]; then
  action=reuse
  target="${TAG_PREFIX}${target}"
  reason="main ($short) is already tagged"
else
  last="$(sed -n "s#.*refs/tags/${TAG_PREFIX}\([0-9][0-9]*\)\(\^{}\)\{0,1\}\$#\1#p" <<<"$remote_tags" | sort -n | tail -1)"
  action=create
  target="${TAG_PREFIX}$((${last:-0} + 1))"
  reason="main ($short) has no ${TAG_PREFIX}N tag${last:+; the latest is ${TAG_PREFIX}${last}}"
fi

# leap-cli's "Create release" compares the output fields with the release
# before the tagged commit. The release commit goes on top of this HEAD, so that
# is the newest release reachable from here.
work="$(mktemp -d)"
trap 'rm -rf "$work"' EXIT
: > "$work/prev-fields.json"
prev="$(git -C "$leap_cli" describe --tags --abbrev=0 --match 'v*' HEAD 2>/dev/null || true)"
if [ -n "$prev" ] && git -C "$leap_cli" cat-file -e "$prev:$CHECKER/main.go" 2>/dev/null; then
  mkdir "$work/prev"
  git -C "$leap_cli" archive "$prev" | tar -x -C "$work/prev"
  (cd "$work/prev" && go run "./$CHECKER" fields) > "$work/prev-fields.json" \
    || die "cannot read the JSON output fields of leap-cli $prev"
fi
echo "Checking the skills at $short against this leap-cli${prev:+ (output fields since $prev)}"
(cd "$leap_cli" && go run "./$CHECKER" check "$skills/skills" "$work/prev-fields.json") \
  || die "the skills on tensorleap/skills main ($short) don't work with this leap-cli. Fix the skills on main, or bring the command they need onto this leap-cli branch, then run again"

if [ "$action" = create ]; then
  if [ -n "$DRY_RUN" ]; then
    # Nothing is created, but the remote still authenticates the push, so a
    # token that can't tag the skills stops the release before it starts.
    git -C "$skills" push --dry-run --quiet origin "${commit}:refs/tags/${target}" \
      || die "cannot push tags to tensorleap/skills - the release would fail tagging $target"
    echo "DRY_RUN set - would tag tensorleap/skills $short as $target"
  else
    git -C "$skills" -c user.name="github-actions[bot]" \
      -c user.email="github-actions[bot]@users.noreply.github.com" \
      tag -a "$target" "$commit" -m "Skills ${target}${VERSION:+, released with Tensorleap ${VERSION}}"
    git -C "$skills" push origin "refs/tags/${target}"
  fi
fi

printf '%s\n' "$target" > "$leap_cli/$REF_FILE"

echo "Skills: $target ($action: $reason; leap-cli pinned ${pinned:-nothing})"
write_outputs "$target" "$action" "$short"
