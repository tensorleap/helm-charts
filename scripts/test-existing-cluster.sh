#!/usr/bin/env bash
#
# End-to-end test for the "existing-cluster" install path documented in
# docs/INSTALL-MODES.md. Spins up a Kind cluster, installs an external
# ingress-nginx (not the bundled subchart), installs both Helm charts, runs
# five assertions, exercises helm upgrade, and validates PVC retention on
# uninstall.
#
# Idempotent: re-running reuses an existing cluster. Use `make
# test-existing-cluster-clean` to tear the cluster down.
#
# Required tools: kind, kubectl, helm, jq, curl.

set -euo pipefail

REPO_ROOT="$(cd -- "$(dirname -- "${BASH_SOURCE[0]}")/.." && pwd)"
CLUSTER_NAME="${CLUSTER_NAME:-tensorleap-test}"
KUBE_CONTEXT="kind-${CLUSTER_NAME}"
NAMESPACE="${NAMESPACE:-tensorleap}"
DOMAIN="${DOMAIN:-tensorleap.local}"
KIND_CONFIG="${REPO_ROOT}/test/kind-config.yaml"
VALUES_FILE="${REPO_ROOT}/charts/tensorleap/examples/values-kind-test.yaml"
TLS_CERT="${REPO_ROOT}/test/tls.crt"
TLS_KEY="${REPO_ROOT}/test/tls.key"
INGRESS_MANIFEST="https://raw.githubusercontent.com/kubernetes/ingress-nginx/main/deploy/static/provider/kind/deploy.yaml"

RED=$'\033[0;31m'; GREEN=$'\033[0;32m'; YELLOW=$'\033[0;33m'; BLUE=$'\033[0;34m'; NC=$'\033[0m'
log()  { printf "%s==>%s %s\n" "${BLUE}" "${NC}" "$*"; }
ok()   { printf "%s[PASS]%s %s\n" "${GREEN}" "${NC}" "$*"; }
fail() { printf "%s[FAIL]%s %s\n" "${RED}"   "${NC}" "$*" >&2; exit 1; }
warn() { printf "%s[WARN]%s %s\n" "${YELLOW}" "${NC}" "$*"; }

kc() { kubectl --context "${KUBE_CONTEXT}" "$@"; }

on_error() {
  local rc=$?
  echo
  warn "Script failed with exit code ${rc}. Dumping diagnostics:"
  kc get pods -n "${NAMESPACE}" 2>/dev/null || true
  kc get events -n "${NAMESPACE}" --sort-by=.lastTimestamp 2>/dev/null | tail -30 || true
  exit "${rc}"
}
trap on_error ERR

need() { command -v "$1" >/dev/null 2>&1 || fail "missing required tool: $1"; }
for t in kind kubectl helm jq curl docker openssl; do need "$t"; done

phase0_tls() {
  log "Phase 0/7: TLS material (self-signed cert for ${DOMAIN})"
  if [[ -s "${TLS_CERT}" && -s "${TLS_KEY}" ]]; then
    ok "reusing existing test/tls.crt + test/tls.key"
    return 0
  fi
  openssl req -x509 -newkey rsa:4096 -nodes \
    -keyout "${TLS_KEY}" -out "${TLS_CERT}" \
    -days 365 -subj "/CN=${DOMAIN}" \
    -addext "subjectAltName=DNS:${DOMAIN}" \
    >/dev/null 2>&1
  chmod 600 "${TLS_KEY}"
  ok "generated test/tls.crt + test/tls.key"
}

phase1_prerender() {
  log "Phase 1/7: pre-render chart (no cluster access required)"
  local rendered
  rendered="$(helm template tl "${REPO_ROOT}/charts/tensorleap" -n "${NAMESPACE}" \
    -f "${VALUES_FILE}" \
    --set-file global.tls.cert="${TLS_CERT}" \
    --set-file global.tls.key="${TLS_KEY}")"
  # Assert zero cluster-scoped RBAC from our own templates (subchart ones
  # would come from charts/tensorleap/charts/{ingress-nginx,datadog}, but
  # the test values disable both).
  local offenders
  offenders="$(awk '/^# Source: tensorleap\/charts\/(ingress-nginx|datadog)\//{skip=1; next}
                    /^# Source:/{skip=0}
                    !skip && /^kind: (ClusterRole|ClusterRoleBinding)$/{print}' <<<"${rendered}")"
  [[ -z "${offenders}" ]] || fail "pre-render check: our templates emit cluster-scoped RBAC: ${offenders}"
  ok "pre-render: no cluster-scoped RBAC from our templates"
}

phase2_cluster() {
  log "Phase 2/7: Kind cluster up"
  if kind get clusters 2>/dev/null | grep -qx "${CLUSTER_NAME}"; then
    ok "cluster ${CLUSTER_NAME} already exists"
  else
    kind create cluster --config "${KIND_CONFIG}"
  fi
  kc wait --for=condition=Ready node --all --timeout=120s >/dev/null
  kc get sc standard >/dev/null || fail "expected default StorageClass 'standard' in Kind"
  ok "node Ready, StorageClass 'standard' present"
}

phase3_ingress() {
  log "Phase 3/7: external ingress-nginx controller"
  kc apply -f "${INGRESS_MANIFEST}" >/dev/null
  kc wait --namespace ingress-nginx \
    --for=condition=ready pod \
    --selector=app.kubernetes.io/component=controller \
    --timeout=180s >/dev/null
  ok "ingress-nginx controller Ready in namespace ingress-nginx"
}

wait_for_pvcs_to_drain() {
  # Block until no PVC in the namespace is in a Terminating state. Upper
  # bound 5 min: ECK and the StatefulSet controller can both hold finalizers
  # for a while, and re-installing while a previous PVC is still
  # Terminating triggers a hard race -- the new install registers a PVC
  # name that already exists, Helm declares success, then the old PVC
  # actually finishes deleting and the new pods come up referencing a PVC
  # that is now gone (FailedScheduling: PVC ... is being deleted).
  local deadline=$(( SECONDS + 300 ))
  while (( SECONDS < deadline )); do
    local terminating
    terminating="$(kc get pvc -n "${NAMESPACE}" -o json 2>/dev/null \
      | jq -r '[.items[] | select(.metadata.deletionTimestamp != null)] | length')"
    [[ "${terminating}" == "0" ]] && return 0
    sleep 3
  done
  fail "PVC drain timed out after 5 min -- aborting to avoid the install-race condition. Run 'kubectl get pvc -n ${NAMESPACE}' to inspect, then re-run the test."
}

phase4_install() {
  log "Phase 4/7: install Helm charts (tensorleap-infra, then tensorleap)"
  kc create namespace "${NAMESPACE}" --dry-run=client -o yaml | kc apply -f - >/dev/null

  # Idempotency: if a previous run left the main release installed, uninstall
  # it first and wait for PVCs to fully clear. Without this, re-running the
  # script races the Helm PVC re-create against the previous PVC's finalizers.
  if helm --kube-context "${KUBE_CONTEXT}" status tensorleap -n "${NAMESPACE}" >/dev/null 2>&1; then
    log "existing 'tensorleap' release detected, uninstalling for a clean run"
    helm --kube-context "${KUBE_CONTEXT}" uninstall tensorleap -n "${NAMESPACE}" >/dev/null
    wait_for_pvcs_to_drain
  fi

  helm --kube-context "${KUBE_CONTEXT}" upgrade --install tensorleap-infra \
    "${REPO_ROOT}/charts/tensorleap-infra" \
    -n "${NAMESPACE}" \
    --set nvidiaGpu.enabled=false \
    --set registry.enabled=false \
    --wait --timeout 5m >/dev/null
  ok "tensorleap-infra installed (ECK operator)"

  helm --kube-context "${KUBE_CONTEXT}" upgrade --install tensorleap \
    "${REPO_ROOT}/charts/tensorleap" \
    -n "${NAMESPACE}" \
    -f "${VALUES_FILE}" \
    --set-file global.tls.cert="${TLS_CERT}" \
    --set-file global.tls.key="${TLS_KEY}" \
    --timeout 15m >/dev/null
  ok "tensorleap release deployed (REVISION $(helm --kube-context "${KUBE_CONTEXT}" list -n "${NAMESPACE}" --filter '^tensorleap$' -o json | jq -r '.[0].revision'))"

  log "Waiting for all pods in ${NAMESPACE} to be Ready (up to 15m)..."
  kc wait --for=condition=Ready pod --all -n "${NAMESPACE}" --timeout=900s >/dev/null
  ok "all pods Ready"
}

phase5_assert() {
  log "Phase 5/7: assertions"

  # A1: zero cluster-scoped RBAC from our release.
  local count
  count="$(kc get clusterrole,clusterrolebinding \
    -l app.kubernetes.io/managed-by=Helm \
    -l app.kubernetes.io/instance=tensorleap \
    --no-headers 2>/dev/null | wc -l | tr -d ' ')"
  [[ "${count}" == "0" ]] || fail "A1: expected 0 ClusterRole/Binding owned by release, got ${count}"
  ok "A1: zero ClusterRole/ClusterRoleBinding owned by the tensorleap release"

  # A2: every PVC Bound on the expected StorageClass.
  kc get pvc -n "${NAMESPACE}" -o json | jq -e '
    (.items | length) >= 5
    and all(.items[]; .status.phase == "Bound" and .spec.storageClassName == "standard")
  ' >/dev/null || { kc get pvc -n "${NAMESPACE}"; fail "A2: not all PVCs Bound on 'standard'"; }
  ok "A2: all PVCs Bound on StorageClass 'standard'"

  # A3: every PV backing our PVCs was dynamically provisioned (not a chart-
  # emitted static hostPath PV).
  kc get pv -o json | jq -e '
    [.items[] | select(.spec.claimRef.namespace == "'"${NAMESPACE}"'")]
    | length >= 5
    and all(.[]; .metadata.annotations["pv.kubernetes.io/provisioned-by"] != null)
  ' >/dev/null || fail "A3: found static (non-dynamic) PVs for ${NAMESPACE} PVCs"
  ok "A3: all PVs dynamically provisioned (no static hostPath PVs from the chart)"

  # A4: Keycloak env reflects HTTPS + non-localhost domain.
  kc get sts keycloak -n "${NAMESPACE}" -o json | jq -e '
    .spec.template.spec.containers[0].env
    |    (any(.[]; .name == "KC_HOSTNAME"             and .value == "https://'"${DOMAIN}"'/auth"))
     and (any(.[]; .name == "KC_PROXY_HEADERS"        and .value == "xforwarded"))
     and (any(.[]; .name == "KC_HOSTNAME_STRICT"      and .value == "true"))
     and (any(.[]; .name == "KC_HOSTNAME_STRICT_HTTPS" and .value == "true"))
     and (any(.[]; .name == "KC_HOSTNAME_ADMIN_URL"   and .value == "https://'"${DOMAIN}"'/auth"))
  ' >/dev/null || fail "A4: Keycloak env does not reflect global.domain=${DOMAIN} + global.tls.enabled=true"
  ok "A4: Keycloak env reflects HTTPS + non-localhost domain"

  # A5: external ingress reaches Keycloak's realm endpoint over HTTPS.
  local code
  code="$(curl -k --resolve "${DOMAIN}:443:127.0.0.1" -s -o /dev/null -w '%{http_code}' \
    "https://${DOMAIN}/auth/realms/tensorleap")"
  [[ "${code}" == "200" ]] || fail "A5: expected HTTPS 200 on /auth/realms/tensorleap, got ${code}"
  ok "A5: external ingress reaches Keycloak over HTTPS (/auth/realms/tensorleap -> 200)"

  # A6: the OIDC login URL that previously triggered "HTTPS required" now
  # works on first try (regression guard for the secure-by-default contract).
  code="$(curl -k --resolve "${DOMAIN}:443:127.0.0.1" -s -o /dev/null -w '%{http_code}' \
    "https://${DOMAIN}/auth/realms/tensorleap/protocol/openid-connect/auth?client_id=tensorleap-client&redirect_uri=https%3A%2F%2F${DOMAIN}%2F&state=test&response_type=code")"
  [[ "${code}" == "200" ]] || fail "A6: OIDC login page expected HTTPS 200, got ${code}"
  ok "A6: OIDC login URL serves HTTPS 200 (no 'HTTPS required' page)"
}

phase6_upgrade() {
  log "Phase 6/7: helm upgrade smoke"
  helm --kube-context "${KUBE_CONTEXT}" upgrade tensorleap \
    "${REPO_ROOT}/charts/tensorleap" \
    -n "${NAMESPACE}" \
    -f "${VALUES_FILE}" \
    --set-file global.tls.cert="${TLS_CERT}" \
    --set-file global.tls.key="${TLS_KEY}" \
    --set global.url="https://${DOMAIN}/" >/dev/null
  kc rollout status sts/keycloak -n "${NAMESPACE}" --timeout=180s >/dev/null
  kc rollout status deploy/tensorleap-node-server -n "${NAMESPACE}" --timeout=180s >/dev/null
  ok "helm upgrade succeeded and rollouts completed"
}

phase7_uninstall() {
  log "Phase 7/7: helm uninstall + PVC retention"
  helm --kube-context "${KUBE_CONTEXT}" uninstall tensorleap -n "${NAMESPACE}" >/dev/null
  wait_for_pvcs_to_drain
  # Reality check: only PVCs managed by controllers (StatefulSet
  # volumeClaimTemplates, ECK) survive uninstall on this chart. Helm-managed
  # PVCs (mongodb-data, keycloak-data, tensorleap-minio) are deleted unless
  # 'helm.sh/resource-policy: keep' is added to the templates.
  kc get pvc rabbitmq-data-rabbitmq-0 -n "${NAMESPACE}" >/dev/null 2>&1 \
    || fail "rabbitmq-data-rabbitmq-0 should survive uninstall (StatefulSet volumeClaimTemplates)"
  ok "STS-managed PVC 'rabbitmq-data-rabbitmq-0' retained after helm uninstall"
  # Note on Helm-managed PVCs (see docs/INSTALL-MODES.md 'Data retention' note).
  local helm_owned_survivors
  helm_owned_survivors="$(kc get pvc -n "${NAMESPACE}" --no-headers 2>/dev/null \
    | awk '$1 ~ /^(mongodb-data|keycloak-data|tensorleap-minio)$/ {print $1}')"
  if [[ -n "${helm_owned_survivors}" ]]; then
    warn "helm-managed PVC(s) unexpectedly retained (check resource-policy annotations): ${helm_owned_survivors}"
  else
    ok "Helm-managed PVCs (mongodb-data, keycloak-data, tensorleap-minio) deleted as expected"
  fi
}

main() {
  phase0_tls
  phase1_prerender
  phase2_cluster
  phase3_ingress
  phase4_install
  phase5_assert
  phase6_upgrade
  phase7_uninstall
  echo
  ok "All phases passed. Cluster '${CLUSTER_NAME}' left running."
  echo "    Note:       phase 7 uninstalled the release. To get a live install for"
  echo "                browser inspection, run:"
  echo
  echo "                  helm --kube-context ${KUBE_CONTEXT} upgrade --install tensorleap \\"
  echo "                    ${REPO_ROOT}/charts/tensorleap -n ${NAMESPACE} \\"
  echo "                    -f ${VALUES_FILE} \\"
  echo "                    --set-file global.tls.cert=${TLS_CERT} \\"
  echo "                    --set-file global.tls.key=${TLS_KEY}"
  echo
  echo "                Then add '127.0.0.1 ${DOMAIN}' to /etc/hosts and open"
  echo "                https://${DOMAIN}/ (accept the self-signed cert warning)."
  echo
  echo "    kubectl:    kubectl --context ${KUBE_CONTEXT} get all -n ${NAMESPACE}"
  echo "    Teardown:   make test-existing-cluster-clean"
}

main "$@"
