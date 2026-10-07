#!/usr/bin/env bash
#
# Check that the web-ui a release ships was built against an API the shipped
# node-server actually has.
#
# web-ui calls node-server through @tensorleap/api-client, which node-server
# publishes under its own package.json version. It is published by hand,
# sometimes from feature branches, and every image locks the version it was
# built with in package-lock.json. Nothing has to be published at release time -
# but nothing checked that the commits being shipped agree either.
#
# Reads the pinned `<ref>-<sha8>` image tags from the charts, then each repo's
# files at exactly those commits:
#   FAIL  web-ui pins a prerelease (e.g. -rc.1) api-client, or node-server's
#         own version is one
#   FAIL  web-ui's api-client is newer than the node-server being shipped
#   WARN  web-ui's api-client lags the node-server being shipped
#
# Usage (from the repo root; GH_TOKEN must be able to read the service repos):
#   scripts/check-release-contracts.sh
#
# Writes a one-line `contracts` summary to $GITHUB_OUTPUT and a table to
# $GITHUB_STEP_SUMMARY when those are set.

set -euo pipefail
# shellcheck source-path=SCRIPTDIR source=lib/release-common.sh
source "$(dirname "${BASH_SOURCE[0]}")/lib/release-common.sh"

NODE_SERVER_VALUES="charts/tensorleap/charts/node-server/values.yaml"
WEB_UI_VALUES="charts/tensorleap/charts/web-ui/values.yaml"

[ -f "$NODE_SERVER_VALUES" ] || die "run from the repo root: $NODE_SERVER_VALUES not found"
for tool in gh jq; do command -v "$tool" >/dev/null || die "$tool is required"; done

commit_of() { # <repo> <values file>
  full_sha "$1" "$(sha_of_tag "$(read_value "$2" image_tag)")"
}

node_server_sha="$(commit_of node-server "$NODE_SERVER_VALUES")"
web_ui_sha="$(commit_of web-ui "$WEB_UI_VALUES")"

node_server_version="$(gh_raw node-server package.json "$node_server_sha" | jq -r '.version // empty')"
web_ui_api_client="$(gh_raw web-ui package-lock.json "$web_ui_sha" \
  | jq -r '.packages["node_modules/@tensorleap/api-client"].version // empty')"

failed=""
if [ -z "$web_ui_api_client" ] || [ -z "$node_server_version" ]; then
  status="❌"
  note="could not read the versions - did the file layout change?"
  failed=1
elif [[ "$web_ui_api_client$node_server_version" == *-* ]]; then
  status="❌"
  note="prerelease api-client - publish a release version and pin that"
  failed=1
else
  case "$(version_cmp "$web_ui_api_client" "$node_server_version")" in
    0)
      status="✅"
      note=""
      ;;
    1)
      status="❌"
      note="web-ui expects a newer API than node-server ships"
      failed=1
      ;;
    *)
      status="⚠️"
      note="web-ui lags node-server"
      ;;
  esac
fi

table="#### api-client between the shipped services
node-server \`${node_server_sha:0:8}\` · web-ui \`${web_ui_sha:0:8}\`

| Contract | web-ui pins | node-server ships | |
|---|---|---|---|
| @tensorleap/api-client | ${web_ui_api_client:-?} | ${node_server_version:-?} | ${status} ${note} |
"

echo "$table"
if [ -n "${GITHUB_STEP_SUMMARY:-}" ]; then
  echo "$table" >> "$GITHUB_STEP_SUMMARY"
fi
if [ -n "${GITHUB_OUTPUT:-}" ]; then
  echo "contracts=web-ui api-client ${web_ui_api_client:-?} vs node-server ${node_server_version:-?} ${status}" >> "$GITHUB_OUTPUT"
fi

if [ -n "$failed" ]; then
  die "web-ui and node-server disagree on @tensorleap/api-client (${note}) - fix the pins on the version branches (or run Patch) before releasing"
fi
if [ "$status" = "⚠️" ]; then
  echo "::warning::@tensorleap/api-client: web-ui ${web_ui_api_client} vs node-server ${node_server_version} - ${note}"
else
  echo "✅ web-ui and node-server agree on @tensorleap/api-client"
fi
