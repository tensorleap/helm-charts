# Installing Tensorleap on Your Kubernetes Cluster

This guide installs Tensorleap into a namespace of an existing Kubernetes
cluster using Helm. It takes about 15 minutes once the prerequisites are in
place.

## Prerequisites

You need:

- A Kubernetes cluster (1.27+) and a `kubectl` context with admin rights to
  create namespaces, RBAC, and CRDs. EKS, GKE Standard, AKS, and on-prem all
  work.
- A `StorageClass` for `ReadWriteOnce` PVCs. Examples: `gp3` (EKS),
  `pd-balanced` (GKE), `managed-csi` (AKS). For production, set
  `reclaimPolicy: Retain`.
- An ingress controller already running with the `nginx` IngressClass.
  Verify with `kubectl get ingressclass nginx`.
- A DNS record (e.g. `tensorleap.yourcompany.com`) pointing at your ingress
  controller's load balancer.
- A TLS certificate and key for that domain (PEM files).
- `helm` v3 and `kubectl` installed locally.
- (Optional, for GPU workloads) the NVIDIA device plugin already installed in
  the cluster.

> GKE Autopilot, EKS Fargate, and other restricted environments are not
> supported -- Elasticsearch needs a privileged init container.

## Install

Tensorleap ships an installer script that runs preflight checks, prints a
plan, and installs both Helm charts (`tensorleap-infra` first, then
`tensorleap`) into the `tensorleap` namespace.

```bash
curl -fsSL https://raw.githubusercontent.com/tensorleap/helm-charts/master/scripts/install-existing-cluster.sh \
  -o install-existing-cluster.sh
chmod +x install-existing-cluster.sh

./install-existing-cluster.sh \
  --kube-context  <your-kubectl-context> \
  --domain        tensorleap.yourcompany.com \
  --storage-class gp3 \
  --tls-cert      ./tls.crt \
  --tls-key       ./tls.key
```

Add `--version 1.5.97` to pin a specific chart release, or `--dry-run` to
preview the commands without changing anything. Run `--help` for the full
list of flags.

The script will ask for confirmation before installing. Wait for all pods to
become Ready (~10 minutes on a warm cluster).

## Verify

```bash
kubectl get pods -n tensorleap
kubectl get pvc  -n tensorleap
curl -s https://tensorleap.yourcompany.com/auth/realms/tensorleap | jq .realm
# -> "tensorleap"
```

Open `https://tensorleap.yourcompany.com/` in your browser. First-time setup
instructions are on the welcome screen.

## Upgrade

Re-run the same command with a newer `--version`. Helm performs a rolling
upgrade.

```bash
./install-existing-cluster.sh \
  --kube-context  <your-kubectl-context> \
  --domain        tensorleap.yourcompany.com \
  --storage-class gp3 \
  --tls-cert      ./tls.crt \
  --tls-key       ./tls.key \
  --version       <new-version> \
  --yes
```

## Uninstall

```bash
helm uninstall tensorleap       -n tensorleap
helm uninstall tensorleap-infra -n tensorleap
```

> **Back up first.** Some PVCs (`mongodb-data`, `keycloak-data`,
> `tensorleap-minio`) are deleted on `helm uninstall`. Use Velero or
> app-level dumps before uninstalling, and use a StorageClass with
> `reclaimPolicy: Retain` if you need data to survive cluster destruction.

## Support

If anything fails, send us:

- Output of `kubectl get pods,pvc,events -n tensorleap`
- Output of `helm list -n tensorleap` and `helm history tensorleap -n tensorleap`
- The arguments you passed to the installer (redact the cert/key paths)

Contact: support@tensorleap.ai
