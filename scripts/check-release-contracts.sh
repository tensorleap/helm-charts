#!/usr/bin/env bash
#
# Check that the service commits a release ships agree on their contracts.
#
# web-ui calls node-server through @tensorleap/api-client, which node-server
# publishes under its own package.json version, and node-server validates
# engine messages against @tensorleap/engine-contract, which engine publishes.
# Both are published by hand, sometimes from feature branches, and every image
# locks the versions it was built with in package-lock.json. Nothing has to be
# published at release time - but nothing checked that the commits being
# shipped agree with each other either.
#
# Reads the pinned `<ref>-<sha8>` image tags from the charts, then each repo's
# files at exactly those commits:
#   FAIL  a prerelease (e.g. -rc.1) contract version
#   FAIL  web-ui's api-client is newer than the node-server being shipped
#   FAIL  node-server's engine-contract is newer than the engine being shipped
#   WARN  a consumer lags its producer (web-ui's engine-contract only warns)
#
# Usage (from the repo root; GH_TOKEN must be able to read the service repos):
#   scripts/check-release-contracts.sh
#
# Writes a one-line `contracts` summary to $GITHUB_OUTPUT and a table to
# $GITHUB_STEP_SUMMARY when those are set.

set -euo pipefail
# shellcheck source-path=SCRIPTDIR source=lib/release-common.sh
source "$(dirname "${BASH_SOURCE[0]}")/lib/release-common.sh"

ENGINE_VALUES="charts/tensorleap/charts/engine/values.yaml"
NODE_SERVER_VALUES="charts/tensorleap/charts/node-server/values.yaml"
WEB_UI_VALUES="charts/tensorleap/charts/web-ui/values.yaml"
ENGINE_CONTRACT_PACKAGE="generated/engine-contracts-package-data/package.json"

[ -f "$ENGINE_VALUES" ] || die "run from the repo root: $ENGINE_VALUES not found"
for tool in gh jq; do command -v "$tool" >/dev/null || die "$tool is required"; done

commit_of() { # <repo> <values file>
  full_sha "$1" "$(sha_of_tag "$(read_value "$2" image_tag)")"
}

locked_version() { # <package> <package-lock.json content>
  jq -r --arg p "node_modules/$1" '.packages[$p].version // empty' <<<"$2"
}

node_server_sha="$(commit_of node-server "$NODE_SERVER_VALUES")"
web_ui_sha="$(commit_of web-ui "$WEB_UI_VALUES")"
engine_sha="$(commit_of engine "$ENGINE_VALUES")"

node_server_version="$(gh_raw node-server package.json "$node_server_sha" | jq -r '.version // empty')"
node_server_lock="$(gh_raw node-server package-lock.json "$node_server_sha")"
node_server_engine_contract="$(locked_version @tensorleap/engine-contract "$node_server_lock")"
web_ui_lock="$(gh_raw web-ui package-lock.json "$web_ui_sha")"
web_ui_api_client="$(locked_version @tensorleap/api-client "$web_ui_lock")"
web_ui_engine_contract="$(locked_version @tensorleap/engine-contract "$web_ui_lock")"
engine_contract="$(gh_raw engine "$ENGINE_CONTRACT_PACKAGE" "$engine_sha" | jq -r '.version // empty')"

failures=0
rows=""
summary=""

# <contract> <consumer> <consumer version> <producer> <producer version> <fail|warn when ahead>
check() {
  local contract="$1" consumer="$2" have="$3" producer="$4" want="$5" ahead="$6" status note
  if [ -z "$have" ] || [ -z "$want" ]; then
    status="❌"
    note="could not read the version (${consumer} ${have:-?}, ${producer} ${want:-?}) - did the file layout change?"
  elif [[ "$have$want" == *-* ]]; then
    status="❌"
    note="prerelease contract - publish a release version and pin that"
  else
    case "$(version_cmp "$have" "$want")" in
      0)
        status="✅"
        note=""
        ;;
      1)
        note="${consumer} expects a newer ${contract} than ${producer} ships"
        if [ "$ahead" = fail ]; then status="❌"; else status="⚠️"; fi
        ;;
      *)
        status="⚠️"
        note="${consumer} lags ${producer}"
        ;;
    esac
  fi
  if [ "$status" = "❌" ]; then
    failures=$((failures + 1))
    echo "::error::${contract}: ${consumer} ${have:-?} vs ${producer} ${want:-?} - ${note}"
  elif [ "$status" = "⚠️" ]; then
    echo "::warning::${contract}: ${consumer} ${have} vs ${producer} ${want} - ${note}"
  fi
  rows="${rows}| ${contract} | ${consumer} ${have:-?} | ${producer} ${want:-?} | ${status} ${note} |"$'\n'
  summary="${summary:+${summary} · }${consumer} ${contract} ${have:-?} ${status}"
}

check "@tensorleap/api-client" web-ui "$web_ui_api_client" node-server "$node_server_version" fail
check "@tensorleap/engine-contract" node-server "$node_server_engine_contract" engine "$engine_contract" fail
check "@tensorleap/engine-contract" web-ui "$web_ui_engine_contract" engine "$engine_contract" warn

table="#### Contracts between the shipped services
node-server \`${node_server_sha:0:8}\` · web-ui \`${web_ui_sha:0:8}\` · engine \`${engine_sha:0:8}\`

| Contract | Consumer | Producer | |
|---|---|---|---|
${rows}"

echo "$table"
if [ -n "${GITHUB_STEP_SUMMARY:-}" ]; then
  echo "$table" >> "$GITHUB_STEP_SUMMARY"
fi
if [ -n "${GITHUB_OUTPUT:-}" ]; then
  echo "contracts=${summary}" >> "$GITHUB_OUTPUT"
fi

if [ "$failures" -gt 0 ]; then
  die "${failures} contract mismatch(es) between the services this release ships - fix the pins on the version branches (or run Patch) before releasing"
fi
echo "✅ Contracts are consistent"
