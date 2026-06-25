#!/usr/bin/env bash
# Headless runner for the QA KB maintenance flow (a building block for a future
# auto-trigger). Runs the same agent (qa-knowledge/maintenance/prompt.md) against the
# SIBLING source repos via `claude -p`, then opens a PR via gh.
# For interactive use, prefer the `update-qa-knowledge` skill (/update-qa-knowledge).
#
# Requires: a local `claude` CLI (authenticated), `gh` (authenticated), and the
# source repos checked out as siblings of helm-charts.
#
# Usage:
#   ./qa-knowledge/maintenance/run-local.sh            # incremental, syncs siblings to origin/master first
#   KB_FORCE_FULL=true ./qa-knowledge/maintenance/run-local.sh
#   KB_NO_FETCH=true   ./qa-knowledge/maintenance/run-local.sh   # use sibling repos as-is
set -euo pipefail

# Resolve helm-charts repo root (two levels up from this script).
SCRIPT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
HELM_CHARTS_ROOT="$(cd "$SCRIPT_DIR/../.." && pwd)"
PARENT_DIR="$(cd "$HELM_CHARTS_ROOT/.." && pwd)"

export KB_SRC_HELM_CHARTS="$HELM_CHARTS_ROOT"
export KB_SRC_ENGINE="$PARENT_DIR/engine"
export KB_SRC_NODE_SERVER="$PARENT_DIR/node-server"
export KB_SRC_WEB_UI="$PARENT_DIR/web-ui"
export KB_SRC_LEAP_CLI="$PARENT_DIR/leap-cli"
export KB_SRC_CODE_LOADER="$PARENT_DIR/code-loader"
export KB_FORCE_FULL="${KB_FORCE_FULL:-false}"

for d in "$KB_SRC_ENGINE" "$KB_SRC_NODE_SERVER" "$KB_SRC_WEB_UI" "$KB_SRC_LEAP_CLI" "$KB_SRC_CODE_LOADER"; do
  if [ ! -d "$d/.git" ]; then
    echo "ERROR: expected source repo at $d (clone it as a sibling of helm-charts)" >&2
    exit 1
  fi
done

# Bring siblings to their latest master unless told not to (does not switch your branch;
# fetches origin/master so the agent can diff against it).
if [ "${KB_NO_FETCH:-false}" != "true" ]; then
  for d in "$KB_SRC_ENGINE" "$KB_SRC_NODE_SERVER" "$KB_SRC_WEB_UI" "$KB_SRC_LEAP_CLI" "$KB_SRC_CODE_LOADER"; do
    echo ">> fetching origin/master in $d"
    git -C "$d" fetch --quiet origin master || echo "WARN: fetch failed in $d (continuing with local state)"
  done
fi

cd "$HELM_CHARTS_ROOT"
BRANCH="qa-kb-local-update-$(git rev-parse --short HEAD)"
git switch -c "$BRANCH" 2>/dev/null || git switch "$BRANCH"

echo ">> running KB maintenance agent (force_full=$KB_FORCE_FULL)"
claude -p "$(cat qa-knowledge/maintenance/prompt.md)" \
  --permission-mode acceptEdits \
  --allowedTools "Read,Edit,Write,Grep,Glob,Bash(git*)" \
  --max-turns 120 \
  --output-format text || true

if git diff --quiet -- qa-knowledge/; then
  echo ">> no doc changes — nothing to PR."
  exit 0
fi

git add qa-knowledge/
git commit -m "docs(qa-knowledge): sync KB with latest source masters (local run)"
git push -u origin "$BRANCH"
gh pr create --fill --label qa-knowledge --label automated \
  --title "docs(qa-knowledge): sync KB with latest masters (local)" \
  --body "Local QA KB maintenance run. Review the doc diff; see qa-knowledge/maintenance/GAPS.md for the run log."
echo ">> PR opened from $BRANCH"
