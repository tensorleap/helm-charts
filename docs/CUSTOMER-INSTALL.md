# Installing Tensorleap on a Customer's Cluster

A short runbook for Tensorleap engineers. Read this once before the install
session. For the full reference (data retention, edge cases, design rationale)
see [INSTALL-MODES.md](INSTALL-MODES.md).

---

## 1. What to collect from the customer

Get these **before** you start installing. Without all five, stop.


| Item               | What you need                                                                                                   | How to verify                                                           |
| ------------------ | --------------------------------------------------------------------------------------------------------------- | ----------------------------------------------------------------------- |
| `kubectl` access   | A `kubeconfig` context with permissions to create namespaces, RBAC, and CRDs                                    | `kubectl --context <ctx> auth can-i create ns` returns `yes`            |
| StorageClass       | Name of a StorageClass for `ReadWriteOnce` PVCs (e.g. `gp3` on EKS, `pd-balanced` on GKE, `managed-csi` on AKS) | `kubectl get sc <name>` -- production should be `reclaimPolicy: Retain` |
| Ingress controller | An NGINX-class ingress controller already running (the chart's bundled one is disabled in this mode)            | `kubectl get ingressclass nginx`                                        |
| DNS                | An A/CNAME record pointing the install domain at the ingress LB                                                 | `dig +short tensorleap.customer.com`                                    |
| TLS cert + key     | Real PEM cert + key for that domain (Let's Encrypt, internal PKI, paid CA -- self-signed only for testing)      | `openssl x509 -in tls.crt -noout -subject -ext subjectAltName -enddate` |


GPU nodes (optional): customer is responsible for the NVIDIA device plugin in
this mode -- we don't install it. Confirm `kubectl get pods -n kube-system | grep nvidia` if they expect GPU support.

---

## 2. Rehearse first on a local Kind cluster

Before touching a customer cluster, run the install end-to-end on a local
Kind cluster: HTTPS on a non-loopback domain (`tensorleap.local`),
self-signed cert, **external** NGINX ingress controller (not the bundled
subchart), dynamic PVCs on Kind's `standard` StorageClass, and namespace-scoped
RBAC. There are two flavours, pick the one that matches what you want to
exercise.

Required tools (both flavours): `kind`, `kubectl`, `helm`, `jq`, `curl`,
`docker`, `openssl`.

### 2a. Automated chart test (proves the chart contract)

`make test-existing-cluster` is the regression harness. It calls `helm
upgrade --install` directly (i.e. it does **not** go through
`scripts/install-existing-cluster.sh`) and runs assertions against the
rendered cluster state. Use it to validate chart changes.

```bash
make test-existing-cluster         # idempotent; reuses the Kind cluster
make test-existing-cluster-clean   # tear down when done
```

Runtime: ~5 min warm, ~10 min cold. The script
([scripts/test-existing-cluster.sh](../scripts/test-existing-cluster.sh))
runs 8 phases:

| Phase             | What it proves                                                                                                                                                                                                                                                                                                                                       |
| ----------------- | ---------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------- |
| 0 -- TLS material | self-signed cert generated at `test/tls.crt`/`test/tls.key` (gitignored)                                                                                                                                                                                                                                                                             |
| 1 -- Pre-render   | `helm template` succeeds and emits zero ClusterRole/Binding from our templates                                                                                                                                                                                                                                                                       |
| 2 -- Cluster      | Kind cluster `tensorleap-test` up with default `standard` StorageClass                                                                                                                                                                                                                                                                               |
| 3 -- Ingress      | upstream `kubernetes/ingress-nginx` kind manifest applied (proves the chart works with a user-supplied controller)                                                                                                                                                                                                                                   |
| 4 -- Install      | `helm install tensorleap-infra` then `helm install tensorleap` with values from [charts/tensorleap/examples/values-kind-test.yaml](../charts/tensorleap/examples/values-kind-test.yaml)                                                                                                                                                              |
| 5 -- Assert       | A1 zero ClusterRole/Binding owned by the release; A2 every PVC `Bound` on `standard`; A3 every PV dynamically provisioned; A4 Keycloak env reflects HTTPS+domain (`KC_HOSTNAME`, `KC_PROXY_HEADERS=xforwarded`, `KC_HOSTNAME_STRICT_HTTPS=true`); A5 HTTPS 200 on `/auth/realms/tensorleap`; A6 OIDC login URL serves 200 (no "HTTPS required" page) |
| 6 -- Upgrade      | `helm upgrade` rolls Keycloak STS and node-server Deployment without immutable-field errors                                                                                                                                                                                                                                                          |
| 7 -- Uninstall    | `helm uninstall` runs cleanly; STS-managed PVC `rabbitmq-data-rabbitmq-0` survives, Helm-owned PVCs are deleted (matches the retention caveat in [§8](#8-uninstall-and-what-happens-to-data))                                                                                                                                                        |

If A1-A6 pass here, the chart will satisfy the same guarantees on a
customer cluster. What it does **not** prove is that
`scripts/install-existing-cluster.sh` itself works -- for that, use 2b.

Phase 7 uninstalls the release. To get a live install for browser
inspection, the script prints the exact re-install command at the end; or
just run flow 2b below instead.

### 2b. Manual rehearsal of the install script (proves the install runbook)

This is the dry-run for a real customer session: same Kind cluster, but the
install itself goes through `scripts/install-existing-cluster.sh`, which
exercises the preflight checks, prerender guard, plan/confirm prompt, repo
add, helm install, and pod-readiness wait that customers will see.

```bash
# 0. Self-signed cert (skip if test/tls.crt + test/tls.key already exist)
mkdir -p test
openssl req -x509 -newkey rsa:4096 -nodes \
  -keyout test/tls.key -out test/tls.crt \
  -days 365 -subj "/CN=tensorleap.local" \
  -addext "subjectAltName=DNS:tensorleap.local"
chmod 600 test/tls.key

# 1. Kind cluster (host ports 80/443 mapped, ingress-ready label set)
kind create cluster --config test/kind-config.yaml
kubectl --context kind-tensorleap-test get sc standard

# 2. External NGINX ingress controller (the bundled subchart stays OFF)
kubectl --context kind-tensorleap-test apply \
  -f https://raw.githubusercontent.com/kubernetes/ingress-nginx/main/deploy/static/provider/kind/deploy.yaml
kubectl --context kind-tensorleap-test wait \
  --namespace ingress-nginx \
  --for=condition=ready pod \
  --selector=app.kubernetes.io/component=controller \
  --timeout=180s

# 3. Real install via the script
make install-existing-cluster ARGS="\
  --kube-context  kind-tensorleap-test \
  --domain        tensorleap.local \
  --storage-class standard \
  --tls-cert      ./test/tls.crt \
  --tls-key       ./test/tls.key \
  --yes"

# 4. Browser inspection
echo "127.0.0.1 tensorleap.local" | sudo tee -a /etc/hosts
open https://tensorleap.local/   # accept the self-signed cert warning once

# 5. Tear down
kind delete cluster --name tensorleap-test
```

Notes:

- The kube-context `kind-tensorleap-test` is created automatically from the
  `name: tensorleap-test` field in
  [test/kind-config.yaml](../test/kind-config.yaml).
- Preflight will warn that StorageClass `standard` has `reclaimPolicy=Delete`
  (Kind's default). That's expected for a rehearsal -- production clusters
  should use `Retain`.
- To rehearse the **remote** code path (`helm repo add` against
  `https://helm.tensorleap.ai`), append `--source remote --version <semver>`
  to the `ARGS` line. Default auto-detects to `local` from a clone.
- `--yes` skips the interactive plan-confirmation prompt; drop it if you
  want to read the plan before proceeding.

### What neither flavour covers

Both flavours run on Kind, so they cannot exercise:

- Cloud-specific ingress (ALB on EKS, GCE Ingress on GKE -- see
  [§6](#6-cloud-specific-gotchas)).
- Multi-AZ EBS/PD zonality.
- Pod Security Standards / OPA-Gatekeeper enforcement (e.g. GKE Autopilot
  rejects the Elasticsearch privileged init container).
- Real DNS / real TLS chain validation.

For those, run `install-existing-cluster.sh --dry-run` against the
customer's kube-context (see [§3](#3-install----recommended-path-script)
below).

---

## 3. Install -- recommended path (script)

The script does preflight checks, prints the plan, and runs Helm under the
hood. Use it for every customer install.

From a clone of this repo:

```bash
make install-existing-cluster ARGS="\
  --kube-context  <customer-context> \
  --domain        tensorleap.customer.com \
  --storage-class gp3 \
  --tls-cert      ./tls.crt \
  --tls-key       ./tls.key"
```

From a customer workstation that has only the script (no clone), pin a
specific chart release for reproducibility:

```bash
./scripts/install-existing-cluster.sh \
  --kube-context  <customer-context> \
  --domain        tensorleap.customer.com \
  --storage-class gp3 \
  --tls-cert      ./tls.crt \
  --tls-key       ./tls.key \
  --source        remote \
  --version       1.5.97
```

What the script does (`scripts/install-existing-cluster.sh`):

1. Verifies the kubectl context, StorageClass, IngressClass, and cert.
2. Adds the `tensorleap` Helm repo if needed (`https://helm.tensorleap.ai`).
3. Pre-renders the chart (`helm template`) -- catches the TLS+domain guard
  before touching the cluster.
4. Prints the plan and asks for confirmation (skip with `--yes`).
5. Installs `tensorleap-infra` (ECK CRDs), then `tensorleap`.
6. Waits for all pods Ready.

Useful flags: `--dry-run` (print commands, change nothing), `--namespace <ns>` (default `tensorleap`), `--extra-values <file>` (merged on top),
`--timeout 30m` (slow CSI provisioners). Run `--help` for the full list.

---

## 4. Install -- direct Helm path (when you can't run the script)

Two `helm upgrade --install` commands, in order:

```bash
helm repo add tensorleap https://helm.tensorleap.ai && helm repo update

helm upgrade --install tensorleap-infra tensorleap/tensorleap-infra \
  -n tensorleap --create-namespace \
  --set nvidiaGpu.enabled=false \
  --set registry.enabled=false

helm upgrade --install tensorleap tensorleap/tensorleap \
  -n tensorleap \
  --set global.namespacedInstall=true \
  --set global.create_local_volumes=false \
  --set global.storageClassName=gp3 \
  --set global.domain=tensorleap.customer.com \
  --set global.url=https://tensorleap.customer.com \
  --set global.tls.enabled=true \
  --set-file global.tls.cert=./tls.crt \
  --set-file global.tls.key=./tls.key \
  --set ingress-nginx.enabled=false \
  --set datadog.enabled=false \
  --timeout 15m
```

For a values-file flow, copy
`[charts/tensorleap/examples/values-existing-cluster.yaml](../charts/tensorleap/examples/values-existing-cluster.yaml)`,
fill in `storageClassName` / `domain` / `url`, and replace the `--set` flags
above with `-f my-values.yaml` (still pass `--set-file` for cert/key so PEM
material doesn't land in `helm get values`).

---

## 5. Verify (5 quick checks)

```bash
# All pods Ready (~10-15 pods).
kubectl get pods -n tensorleap

# Every PVC Bound on the customer's StorageClass.
kubectl get pvc -n tensorleap

# Zero cluster-scoped RBAC owned by the release.
kubectl get clusterrole,clusterrolebinding -l app.kubernetes.io/instance=tensorleap

# Keycloak realm reachable over HTTPS.
curl -s https://tensorleap.customer.com/auth/realms/tensorleap | jq .realm
# -> "tensorleap"

# OIDC discovery (proves Keycloak hostname/proxy config is correct).
curl -s https://tensorleap.customer.com/auth/realms/tensorleap/.well-known/openid-configuration | jq .issuer
```

If any pod is `CrashLoopBackOff`, start with
`kubectl logs <pod> -n tensorleap --previous` and `kubectl describe pod`.

---

## 6. Cloud-specific gotchas

### EKS

- StorageClass: prefer `gp3` over `gp2` (cheaper, better IOPS). Set
`reclaimPolicy: Retain` for production.
- Ingress: works with **NGINX Ingress Controller**. If the customer uses
**AWS Load Balancer Controller (ALB)**, our `Ingress` objects are ignored
-- they need to install NGINX or front the cluster with NGINX.
- Multi-AZ: EBS volumes are zonal. Stateful pods that get rescheduled to a
different zone can't reattach. Pin node groups to one AZ for HA-light
installs, or expect manual recovery.

### GKE

- StorageClass: prefer `pd-balanced` or `pd-ssd` with `volumeBindingMode: WaitForFirstConsumer`. The default `standard` can deadlock ECK +
Elasticsearch on multi-zone clusters.
- Ingress: GKE's default GCE Ingress (class `gce`) **doesn't match** our
`nginx` annotation. Customer must install NGINX (`helm install ingress-nginx ingress-nginx/ingress-nginx -n ingress-nginx --create-namespace`) or you
override the ingress class.
- **GKE Autopilot is not supported.** The Elasticsearch pod needs a
privileged init container (`vm.max_map_count=262144`) which Autopilot
rejects. Use Standard mode.

### AKS

- StorageClass: `managed-csi` works. Same `WaitForFirstConsumer` advice as
GKE.
- Ingress: NGINX or AGIC. AGIC needs the same overrides as ALB on EKS.

### On-prem / bare metal

- Make sure a CSI driver is wired up (Longhorn, Rook-Ceph, NetApp Trident,
...) and that the StorageClass actually provisions PVCs -- many on-prem
clusters have a default StorageClass that does nothing.
- LoadBalancer for the ingress controller usually needs MetalLB or an
external LB.

---

## 7. Upgrade

Same command, new chart version:

```bash
helm repo update
helm upgrade tensorleap tensorleap/tensorleap -n tensorleap \
  --reuse-values \
  --set-file global.tls.cert=./tls.crt \
  --set-file global.tls.key=./tls.key \
  --version 1.5.98
```

`--reuse-values` keeps the customer's existing settings; only the cert/key
need to be re-supplied because `--set-file` material isn't stored in release
metadata.

`helm history tensorleap -n tensorleap` shows what was deployed; revert with
`helm rollback tensorleap <revision>`.

---

## 8. Uninstall (and what happens to data)

```bash
helm uninstall tensorleap       -n tensorleap
helm uninstall tensorleap-infra -n tensorleap
```

PVC retention is **not uniform**:


| PVC                                                 | Survives `helm uninstall`?                                          |
| --------------------------------------------------- | ------------------------------------------------------------------- |
| `rabbitmq-data-rabbitmq-0`                          | yes (StatefulSet `volumeClaimTemplates`)                            |
| `elasticsearch-data-tl-elasticsearch-es-master-0`   | yes during chart upgrade, no when the `Elasticsearch` CR is removed |
| `mongodb-data`, `keycloak-data`, `tensorleap-minio` | **no** -- Helm-owned, deleted on uninstall                          |


Before any uninstall in production: take a Velero backup of the namespace
**or** patch annotations to keep the Helm-owned PVCs:

```bash
kubectl annotate pvc -n tensorleap mongodb-data keycloak-data tensorleap-minio \
  helm.sh/resource-policy=keep
```

For data to survive **cluster destruction**, the StorageClass must have
`reclaimPolicy: Retain` -- otherwise the underlying disks are garbage
collected with the PV.

---

## 9. When to fall back to the long doc

Reach for [INSTALL-MODES.md](INSTALL-MODES.md) when:

- The customer wants a non-`tensorleap` namespace.
- They have an existing ECK operator and want to disable our infra chart's.
- They use an external Elasticsearch (`global.elasticsearch.enabled=false`).
- You hit "HTTPS required" on the login page (HTTP + non-loopback domain).
- You need the full guarantees / desired-state matrix for a security review.

---

## Cheat sheet

```bash
# Install
make install-existing-cluster ARGS="--kube-context X --domain Y --storage-class Z --tls-cert tls.crt --tls-key tls.key"

# Health
kubectl get pods,pvc,ingress -n tensorleap
curl -s https://Y/auth/realms/tensorleap | jq .realm

# Upgrade
helm upgrade tensorleap tensorleap/tensorleap -n tensorleap --reuse-values \
  --set-file global.tls.cert=tls.crt --set-file global.tls.key=tls.key --version <X>

# Uninstall (back up first!)
helm uninstall tensorleap tensorleap-infra -n tensorleap
```

