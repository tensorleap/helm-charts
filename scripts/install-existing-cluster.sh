#!/usr/bin/env bash
#
# install-existing-cluster.sh -- install Tensorleap into an existing
# Kubernetes cluster from a customer's workstation.
#
# The customer brings the cluster, an ingress controller, a StorageClass, a
# DNS record, and a TLS certificate. The script does everything else.
#
# It can install from either:
#   - the public Helm repo at https://helm.tensorleap.ai (default when the
#     script is run outside a clone of the repo), or
#   - a local clone of the helm-charts repo (default when this script lives
#     inside such a clone).
#
# Run with --help for the flag list.

set -euo pipefail

# --- defaults --------------------------------------------------------------
SCRIPT_DIR="$(cd -- "$(dirname -- "${BASH_SOURCE[0]}")" && pwd)"
LOCAL_CHARTS_ROOT="$(cd -- "${SCRIPT_DIR}/.." 2>/dev/null && pwd)/charts"

# Public Helm repo (configurable via env for forks / mirrors).
HELM_REPO_NAME="${HELM_REPO_NAME:-tensorleap}"
HELM_REPO_URL="${HELM_REPO_URL:-https://helm.tensorleap.ai}"

# Required inputs.
KUBE_CONTEXT="${KUBE_CONTEXT:-}"
DOMAIN="${DOMAIN:-}"
STORAGE_CLASS="${STORAGE_CLASS:-}"
TLS_CERT="${TLS_CERT:-}"
TLS_KEY="${TLS_KEY:-}"

# Optional inputs.
SOURCE="${SOURCE:-auto}"             # auto | local | remote
VERSION="${VERSION:-}"               # only for SOURCE=remote
NAMESPACE="${NAMESPACE:-tensorleap}"
EXTRA_VALUES="${EXTRA_VALUES:-}"
TIMEOUT="${TIMEOUT:-15m}"
ASSUME_YES="${ASSUME_YES:-false}"
DRY_RUN="${DRY_RUN:-false}"

# Power-user knobs (env-only, no CLI flag -- override via env if needed).
RELEASE_NAME="${RELEASE_NAME:-tensorleap}"
INFRA_RELEASE_NAME="${INFRA_RELEASE_NAME:-tensorleap-infra}"
INGRESS_CLASS="${INGRESS_CLASS:-nginx}"
LOCAL_CHART_DIR="${LOCAL_CHART_DIR:-${LOCAL_CHARTS_ROOT}/tensorleap}"
LOCAL_INFRA_CHART_DIR="${LOCAL_INFRA_CHART_DIR:-${LOCAL_CHARTS_ROOT}/tensorleap-infra}"

# --- pretty-print helpers --------------------------------------------------
RED=$'\033[0;31m'; GREEN=$'\033[0;32m'; YELLOW=$'\033[0;33m'; BLUE=$'\033[0;34m'; BOLD=$'\033[1m'; NC=$'\033[0m'
log()  { printf '%s==>%s %s\n'  "${BLUE}"   "${NC}" "$*"; }
ok()   { printf '%s[OK]%s %s\n' "${GREEN}"  "${NC}" "$*"; }
warn() { printf '%s[WARN]%s %s\n' "${YELLOW}" "${NC}" "$*"; }
die()  { printf '%s[FAIL]%s %s\n' "${RED}"  "${NC}" "$*" >&2; exit 1; }

usage() {
  cat <<EOF
${BOLD}Tensorleap installer for an existing Kubernetes cluster${NC}

Required:
  --kube-context <name>     kubectl context to install into
  --domain <fqdn>           Public domain (e.g. tensorleap.example.com)
  --storage-class <name>    Cluster StorageClass for dynamic PVCs (e.g. gp3)
  --tls-cert <path>         Path to TLS certificate (PEM)
  --tls-key  <path>         Path to TLS private key (PEM)

Chart source:
  --source <auto|local|remote>
                            Where to fetch the charts from. Default: auto
                              auto   -- local if this script lives inside a
                                        clone, otherwise remote.
                              local  -- charts/ directory next to this script.
                              remote -- public Helm repo (${HELM_REPO_URL}).
  --version <semver>        Chart version to install (only for --source remote).
                            Default: latest in the repo.

Optional:
  --namespace <ns>          Default: tensorleap
  --extra-values <path>     Additional values file merged on top of the defaults
  --timeout <duration>      Helm timeout (default: 15m)
  --yes, -y                 Skip the interactive confirmation prompt
  --dry-run                 Print the helm/kubectl commands, do nothing
  -h, --help                Show this help and exit

Every flag has a matching env-var fallback (KUBE_CONTEXT, DOMAIN, ...). For
rarely-overridden things use env vars only:
  RELEASE_NAME, INFRA_RELEASE_NAME -- helm release names
  INGRESS_CLASS                    -- expected IngressClass (default: nginx)
  LOCAL_CHART_DIR, LOCAL_INFRA_CHART_DIR -- override paths under --source local
  HELM_REPO_NAME, HELM_REPO_URL    -- override the public Helm repo
  INFRA_VERSION                    -- pin the infra chart version too
                                      (--version pins the main chart only)

Examples:

  # Customer install from the public Helm repo (latest version, no clone needed):
  $(basename "$0") \\
    --kube-context my-prod \\
    --domain      tensorleap.example.com \\
    --storage-class gp3 \\
    --tls-cert    ./tls.crt \\
    --tls-key     ./tls.key

  # Same install, but pin to a specific chart release for reproducibility:
  $(basename "$0") ... --version 1.5.97

  # Developer install from a local clone (auto-detected):
  cd /path/to/helm-charts && scripts/$(basename "$0") ...
EOF
}

# --- arg parsing -----------------------------------------------------------
while (($#)); do
  case "$1" in
    --kube-context)        KUBE_CONTEXT="$2"; shift 2 ;;
    --namespace)           NAMESPACE="$2"; shift 2 ;;
    --domain)              DOMAIN="$2"; shift 2 ;;
    --storage-class)       STORAGE_CLASS="$2"; shift 2 ;;
    --tls-cert)            TLS_CERT="$2"; shift 2 ;;
    --tls-key)             TLS_KEY="$2"; shift 2 ;;
    --source)              SOURCE="$2"; shift 2 ;;
    --version)             VERSION="$2"; shift 2 ;;
    --extra-values)        EXTRA_VALUES="$2"; shift 2 ;;
    --timeout)             TIMEOUT="$2"; shift 2 ;;
    --yes|-y)              ASSUME_YES="true"; shift ;;
    --dry-run)             DRY_RUN="true"; shift ;;
    -h|--help)             usage; exit 0 ;;
    *)                     die "unknown argument: $1 (use --help)" ;;
  esac
done

# --- input validation ------------------------------------------------------
[[ -n "${KUBE_CONTEXT}"  ]] || die "missing --kube-context"
[[ -n "${DOMAIN}"        ]] || die "missing --domain"
[[ -n "${STORAGE_CLASS}" ]] || die "missing --storage-class"
[[ -n "${TLS_CERT}"      ]] || die "missing --tls-cert"
[[ -n "${TLS_KEY}"       ]] || die "missing --tls-key"
[[ -s "${TLS_CERT}"      ]] || die "TLS cert not found or empty: ${TLS_CERT}"
[[ -s "${TLS_KEY}"       ]] || die "TLS key not found or empty: ${TLS_KEY}"
if [[ -n "${EXTRA_VALUES}" ]]; then
  [[ -s "${EXTRA_VALUES}" ]] || die "extra values file not found or empty: ${EXTRA_VALUES}"
fi

need() { command -v "$1" >/dev/null 2>&1 || die "missing required tool: $1"; }
for t in kubectl helm openssl jq; do need "$t"; done

# --- resolve chart source --------------------------------------------------
# Settles two variables once and for all, used by every helm invocation:
#   CHART_REF       -- e.g. "/abs/path/to/charts/tensorleap"  or  "tensorleap/tensorleap"
#   INFRA_CHART_REF -- same shape for the infra chart
#   SOURCE_DESC     -- human-readable string for the install plan
case "${SOURCE}" in
  auto)
    if [[ -d "${LOCAL_CHART_DIR}" && -d "${LOCAL_INFRA_CHART_DIR}" ]]; then
      SOURCE="local"
    else
      SOURCE="remote"
    fi
    ;;
  local|remote) ;;
  *) die "invalid --source ${SOURCE}: expected auto|local|remote" ;;
esac

if [[ "${SOURCE}" == "local" ]]; then
  [[ -d "${LOCAL_CHART_DIR}"       ]] || die "local chart dir not found: ${LOCAL_CHART_DIR} (set LOCAL_CHART_DIR or run from a clone)"
  [[ -d "${LOCAL_INFRA_CHART_DIR}" ]] || die "local infra chart dir not found: ${LOCAL_INFRA_CHART_DIR}"
  [[ -z "${VERSION}" ]] || warn "--version is ignored when --source=local"
  CHART_REF="${LOCAL_CHART_DIR}"
  INFRA_CHART_REF="${LOCAL_INFRA_CHART_DIR}"
  SOURCE_DESC="local (${LOCAL_CHARTS_ROOT})"
else
  CHART_REF="${HELM_REPO_NAME}/tensorleap"
  INFRA_CHART_REF="${HELM_REPO_NAME}/tensorleap-infra"
  SOURCE_DESC="remote ${HELM_REPO_URL}${VERSION:+ @ ${VERSION}}"
fi

# --- helpers ---------------------------------------------------------------
kc() { kubectl --context "${KUBE_CONTEXT}" "$@"; }

# Run a kubectl/helm command, or print a copy/paste-ready version of it
# under --dry-run.
run_helm()    { run helm    --kube-context "${KUBE_CONTEXT}" "$@"; }
run_kubectl() { run kubectl --context      "${KUBE_CONTEXT}" "$@"; }
run() {
  if [[ "${DRY_RUN}" == "true" ]]; then
    printf '%s[dry-run]%s ' "${YELLOW}" "${NC}"
    printf '%q ' "$@"
    printf '\n'
  else
    "$@"
  fi
}

confirm() {
  [[ "${ASSUME_YES}" == "true" ]] && return 0
  printf '\n%sProceed with the install?%s [y/N] ' "${BOLD}" "${NC}"
  read -r answer
  [[ "${answer}" =~ ^[Yy]$ ]] || die "aborted by user"
}

# Helm value flags shared by template / upgrade --install. The cert and key
# are injected via --set-file so PEM material never lands inside helm
# release metadata. The other --set lines pin the secure-by-default
# contract for existing-cluster installs.
helm_value_args() {
  local args=(
    -n "${NAMESPACE}"
    --set "global.namespacedInstall=true"
    --set "global.create_local_volumes=false"
    --set "global.storageClassName=${STORAGE_CLASS}"
    --set "global.domain=${DOMAIN}"
    --set "global.url=https://${DOMAIN}"
    --set "global.tls.enabled=true"
    --set-file "global.tls.cert=${TLS_CERT}"
    --set-file "global.tls.key=${TLS_KEY}"
    --set "ingress-nginx.enabled=false"
    --set "datadog.enabled=false"
  )
  [[ "${SOURCE}" == "remote" && -n "${VERSION}" ]] && args+=(--version "${VERSION}")
  [[ -n "${EXTRA_VALUES}" ]] && args+=(-f "${EXTRA_VALUES}")
  printf '%s\n' "${args[@]}"
}

# --- preflight -------------------------------------------------------------
preflight() {
  log "Pre-flight checks"

  kc cluster-info >/dev/null 2>&1 \
    || die "kubectl context '${KUBE_CONTEXT}' cannot reach the cluster"
  ok "kubectl context '${KUBE_CONTEXT}' reachable"

  local helm_major
  helm_major="$(helm version --short 2>/dev/null | sed -nE 's/^v?([0-9]+).*/\1/p')"
  [[ "${helm_major}" == "3" ]] || die "Helm v3 required, got: $(helm version --short 2>/dev/null)"
  ok "Helm v3 detected"

  if kc get namespace "${NAMESPACE}" >/dev/null 2>&1; then
    ok "namespace '${NAMESPACE}' exists"
  else
    warn "namespace '${NAMESPACE}' does not exist -- it will be created"
  fi

  kc get storageclass "${STORAGE_CLASS}" >/dev/null 2>&1 \
    || die "StorageClass '${STORAGE_CLASS}' not found"
  local reclaim
  reclaim="$(kc get storageclass "${STORAGE_CLASS}" -o jsonpath='{.reclaimPolicy}')"
  [[ "${reclaim}" == "Retain" ]] \
    || warn "StorageClass '${STORAGE_CLASS}' has reclaimPolicy=${reclaim:-Delete} -- production should use Retain"
  ok "StorageClass '${STORAGE_CLASS}' present (reclaimPolicy=${reclaim:-Delete})"

  if kc get ingressclass "${INGRESS_CLASS}" >/dev/null 2>&1; then
    ok "IngressClass '${INGRESS_CLASS}' present"
  else
    warn "IngressClass '${INGRESS_CLASS}' not found -- the chart's Ingress objects expect it"
  fi

  if openssl x509 -in "${TLS_CERT}" -noout -checkend 86400 >/dev/null 2>&1; then
    local subj san not_after
    subj="$(openssl x509 -in "${TLS_CERT}" -noout -subject 2>/dev/null | sed 's/^subject= *//')"
    san="$(openssl x509 -in "${TLS_CERT}" -noout -ext subjectAltName 2>/dev/null | tail -1 | sed 's/^[[:space:]]*//')"
    not_after="$(openssl x509 -in "${TLS_CERT}" -noout -enddate 2>/dev/null | sed 's/notAfter=//')"
    ok "TLS cert OK (${subj}, expires ${not_after})"
    if [[ -n "${san}" ]] && ! printf '%s' "${san}" | grep -qE "(^|,| )DNS:${DOMAIN}([,$]| )?"; then
      warn "cert SAN does not include DNS:${DOMAIN} -- browsers will reject it"
    fi
  else
    warn "could not verify TLS cert (expired within 24h, or unreadable)"
  fi
}

# Make sure `helm <chart>` resolves. For local: nothing to do. For remote:
# add and refresh the repo. helm repo {add,update} only touch local Helm
# state on the operator's workstation, so they always run -- even in
# dry-run mode -- because the prerender check needs an up-to-date index.
ensure_chart_source() {
  if [[ "${SOURCE}" == "local" ]]; then
    return 0
  fi
  log "Ensuring Helm repo '${HELM_REPO_NAME}' (${HELM_REPO_URL}) is configured"
  if helm repo list -o json 2>/dev/null \
      | jq -e --arg n "${HELM_REPO_NAME}" --arg u "${HELM_REPO_URL}" \
            'any(.[]; .name == $n and (.url == $u or .url == ($u + "/")))' >/dev/null
  then
    ok "Helm repo '${HELM_REPO_NAME}' already configured"
  else
    helm repo add "${HELM_REPO_NAME}" "${HELM_REPO_URL}" --force-update >/dev/null
    ok "Helm repo '${HELM_REPO_NAME}' added"
  fi
  helm repo update "${HELM_REPO_NAME}" >/dev/null
  ok "Helm repo '${HELM_REPO_NAME}' index updated"
}

prerender_check() {
  log "Helm pre-render check (catches the TLS+domain guard before any apply)"
  local args=(template "${RELEASE_NAME}" "${CHART_REF}")
  while IFS= read -r v; do args+=("$v"); done < <(helm_value_args)
  helm --kube-context "${KUBE_CONTEXT}" "${args[@]}" >/dev/null \
    || die "helm template failed -- aborting before any cluster changes"
  ok "Helm pre-render OK"
}

# --- summary ---------------------------------------------------------------
print_plan() {
  cat <<EOF

${BOLD}Install plan${NC}
  Cluster (kube-context):  ${KUBE_CONTEXT}
  Namespace:               ${NAMESPACE}
  Domain (public):         ${DOMAIN}
  Storage class:           ${STORAGE_CLASS}
  TLS cert / key:          ${TLS_CERT} / ${TLS_KEY}
  Chart source:            ${SOURCE_DESC}
  Main release:            ${RELEASE_NAME}        (${CHART_REF})
  Infra release:           ${INFRA_RELEASE_NAME}  (${INFRA_CHART_REF})
  Extra values file:       ${EXTRA_VALUES:-(none)}
  Helm timeout:            ${TIMEOUT}
  Dry run:                 ${DRY_RUN}
EOF
}

# --- install steps ---------------------------------------------------------
ensure_namespace() {
  log "Ensuring namespace '${NAMESPACE}' exists"
  if [[ "${DRY_RUN}" == "true" ]]; then
    printf '%s[dry-run]%s kubectl --context %q create namespace %q --dry-run=client -o yaml | kubectl --context %q apply -f -\n' \
      "${YELLOW}" "${NC}" "${KUBE_CONTEXT}" "${NAMESPACE}" "${KUBE_CONTEXT}"
    return 0
  fi
  kubectl --context "${KUBE_CONTEXT}" create namespace "${NAMESPACE}" \
    --dry-run=client -o yaml \
    | kubectl --context "${KUBE_CONTEXT}" apply -f - >/dev/null
}

install_infra() {
  log "Installing/upgrading infra chart '${INFRA_RELEASE_NAME}'"
  # NOTE: the user-supplied --version pins the main chart only. The infra
  # chart (ECK operator + CRDs) follows its own, slower-moving version
  # stream, and customers almost always want the latest. Override with the
  # INFRA_VERSION env var if a specific infra chart version is required.
  local args=(
    upgrade --install "${INFRA_RELEASE_NAME}" "${INFRA_CHART_REF}"
    -n "${NAMESPACE}"
    --set "nvidiaGpu.enabled=false"
    --set "registry.enabled=false"
    --wait --timeout "${TIMEOUT}"
  )
  [[ "${SOURCE}" == "remote" && -n "${INFRA_VERSION:-}" ]] && args+=(--version "${INFRA_VERSION}")
  run_helm "${args[@]}"
  ok "infra chart deployed"
}

install_main() {
  log "Installing/upgrading main chart '${RELEASE_NAME}'"
  local args=(upgrade --install "${RELEASE_NAME}" "${CHART_REF}")
  while IFS= read -r v; do args+=("$v"); done < <(helm_value_args)
  args+=(--timeout "${TIMEOUT}")
  run_helm "${args[@]}"
  ok "main chart deployed (release ${RELEASE_NAME}, namespace ${NAMESPACE})"
}

wait_ready() {
  if [[ "${DRY_RUN}" == "true" ]]; then
    warn "dry-run: skipping pod readiness wait"
    return 0
  fi
  log "Waiting for pods in '${NAMESPACE}' to become Ready"

  # Convert TIMEOUT (e.g. "15m", "300s") to seconds. Default to 15m.
  local seconds
  case "${TIMEOUT}" in
    *m) seconds=$(( ${TIMEOUT%m} * 60 )) ;;
    *s) seconds="${TIMEOUT%s}" ;;
    *)  seconds=900 ;;
  esac
  local deadline=$(( SECONDS + seconds ))

  # Polling loop instead of `kubectl wait --for=condition=Ready pod --all`,
  # which has a known footgun: in many kubectl versions it watches forever
  # for newly-appearing pods and never returns even when every existing pod
  # is already Ready.
  while (( SECONDS < deadline )); do
    local total not_ready
    total="$(kc get pods -n "${NAMESPACE}" --no-headers 2>/dev/null | wc -l | tr -d ' ')"
    if [[ "${total}" == "0" ]]; then
      sleep 5; continue
    fi
    not_ready="$(kc get pods -n "${NAMESPACE}" -o json 2>/dev/null \
      | jq -r '[.items[]
                 | select((.status.phase // "") != "Succeeded")
                 | select(((.status.conditions // [])
                           | map(select(.type=="Ready"))
                           | first
                           | .status // "False") != "True")]
                | length')"
    if [[ "${not_ready}" == "0" ]]; then
      ok "all ${total} pod(s) in '${NAMESPACE}' Ready"
      return 0
    fi
    printf '   waiting... %s/%s pod(s) not yet Ready (%ds remaining)\n' \
      "${not_ready}" "${total}" "$(( deadline - SECONDS ))"
    sleep 10
  done
  warn "timeout reached -- inspect with: kubectl --context ${KUBE_CONTEXT} get pods -n ${NAMESPACE}"
}

print_postinstall() {
  cat <<EOF

${GREEN}${BOLD}Tensorleap install complete.${NC}

Next steps:

  1. Make sure DNS for ${BOLD}${DOMAIN}${NC} resolves to your ingress controller.

  2. Open the app:
        https://${DOMAIN}/

  3. Verify in the cluster:
        kubectl --context ${KUBE_CONTEXT} get pods   -n ${NAMESPACE}
        kubectl --context ${KUBE_CONTEXT} get pvc    -n ${NAMESPACE}
        kubectl --context ${KUBE_CONTEXT} get ingress -n ${NAMESPACE}

  4. Confirm Keycloak is reachable:
        curl -s https://${DOMAIN}/auth/realms/tensorleap | jq .realm

  5. Re-run with the same arguments to upgrade.

  6. Uninstall (see docs/INSTALL-MODES.md for the data-retention caveats):
        helm --kube-context ${KUBE_CONTEXT} uninstall ${RELEASE_NAME}        -n ${NAMESPACE}
        helm --kube-context ${KUBE_CONTEXT} uninstall ${INFRA_RELEASE_NAME}  -n ${NAMESPACE}

EOF
}

# --- main ------------------------------------------------------------------
main() {
  preflight
  ensure_chart_source
  prerender_check
  print_plan
  confirm
  ensure_namespace
  install_infra
  install_main
  wait_ready
  print_postinstall
}

main "$@"
