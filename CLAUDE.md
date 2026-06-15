# helm-charts

This repo ships two things:

1. A Go CLI (`leap server ...`) that installs Tensorleap into a local k3d-based Kubernetes cluster. Entry: `main.go` → `cmd/server/root.go`.
2. Helm charts under `charts/` that define the Tensorleap app.

## Layout

- `cmd/server/` — cobra subcommands (`install`, `upgrade`, `reinstall`, `uninstall`, `check`, `pack`, `run`, `stop`, `create-manifest`, `tools`). One subcommand per file.
- `pkg/` — implementation, grouped by concern: `helm/` (Helm v3 SDK wrappers), `k3d/` (cluster lifecycle), `k8s/`, `docker/`, `containerd/`, `zot/` (local registry), `server/` (install flow, manifest, airgap), `local/`, `log/`, `utils/`, `version/`, `github/`.
- `charts/tensorleap/` — umbrella app chart. Local subcharts: `engine`, `node-server`, `web-ui`. External deps: `ingress-nginx`, `keycloakx`, `datadog`.
- `charts/tensorleap-infra/` — installed **before** `tensorleap`. Carries CRDs via the `eck-operator` subchart and cluster-scoped resources (nvidia device plugin, zot registry).
- `images.txt` — generated list of every image referenced by the charts. Regenerate with `make update-images`.

**Install ordering:** `tensorleap-infra` (CRDs, ECK operator) → `tensorleap` (uses the `Elasticsearch` CR). Do not merge these charts.

## Common commands

```
make build-helm       # pull helm deps into charts/ (required before --local)
make update-images    # regenerate images.txt
make test             # go test ./...
make lint             # golangci-lint
go run . install --local   # run installer against local charts
```

## Versioning

- Bump `charts/tensorleap/Chart.yaml` `version:` on every chart-affecting change. **Minor-version bumps (`1.5.x → 1.6.0`) signal that cluster reinstall is required** — the installer compares minor versions to decide reinstall (see `DEVELOPER-GUIDE.md`).
- Bump `charts/tensorleap-infra/Chart.yaml` when infra changes.
- Bump `pkg/version/version.go` for installer (Go CLI) changes, then tag the repo.

## Go conventions

- CLI entrypoints in `cmd/server/`; business logic MUST live in `pkg/`. Tests next to code (`foo_test.go`), table-driven with `t.Run` subtests.
- **Errors:** wrap with `fmt.Errorf("...: %w", err)` carrying the inputs that identify the call. Branch via `errors.Is`/`errors.As`, never string matching. At the cobra `RunE` boundary, return the error — the runner logs and sets the exit code.
- **Context:** every call that does I/O or crosses a goroutine takes `ctx context.Context` first. In cobra handlers use `cmd.Context()` and propagate to helm/k3d/docker. Don't store `ctx` in structs; don't pass `nil`.
- **Logging:** use `pkg/log` helpers (`log.Printf`, `log.Info`, `log.SendCloudReport`). Do not call `fmt.Printf` for runtime output or `logrus` directly outside `pkg/log`. User-facing messages stay short; technical detail goes to debug logs.
- **Single-gateway packages:** call into `pkg/helm` (never import `helm.sh/helm/v3` from `cmd/` or other `pkg/` subpackages). `pkg/k3d` is the only gateway to `k3d/v5`; `pkg/docker` is the only gateway to docker. Don't bypass them. Names/limits are constants at the top of the package file (`CLUSTER_NAME`, `STORAGE_EVICTION_THRESHOLD_GB`) — reuse, don't repeat magic numbers.
- **Helm SDK:** all operations require `ctx` and a prepared `HelmConfig`. Honor `client.Wait = true` for installs/upgrades users wait on; set explicit `Timeout` (hours, not minutes, for large clusters).
- **Flags:** each command has a `*Flags` struct with `SetFlags(cmd *cobra.Command)`. Compose via embedding (e.g. `InstallFlags` embeds `InstallationSourceFlags`). Defaults set in `SetFlags`; document each flag in its description.
- **Concurrency:** use `errgroup` with `SetLimit` for fan-out — no unbounded goroutines. Every goroutine respects `ctx.Done()`.
- **Format/lint:** `gofmt -w`; CI enforces `make check-fmt` and `make lint`. Imports grouped stdlib / third-party / internal (`goimports`).
- **Testing:** run `go test ./...` and `go test -race ./...` before pushing. Prefer fakes over mocks; `httptest.NewServer` for HTTP; avoid sleeps (`require.Eventually` or a fake clock).

### External-facing stability

`pkg/server.RunInstallCmd` and similar are consumed by `leap-cli` (separate repo). Do **not** change signatures without coordinating a `leap-cli` bump (see `DEVELOPER-GUIDE.md`).

## Helm / K8s manifest conventions

- Preserve existing indentation and helper usage (match `nindent N`). Wrap conditional manifests at file level (`{{- if .Values.foo.enabled }}...{{- end }}`), not per-field. Never hand-roll labels when `chart.labels` / `chart.selectorLabels` exist.
- Every container MUST set `resources.requests` and `resources.limits`. For stateful components (mongodb, elasticsearch, rabbitmq, minio, keycloak) memory `requests == limits`. Set probes explicitly on new workloads.
- **Persistent volumes** support two modes via `global.create_local_volumes`:
  - `true` — a static `PersistentVolume` with `hostPath` emitted alongside the PVC, bound via `claimRef` (k3d single-node, `/var/lib/tensorleap/standalone/storage/<name>`).
  - `false` — PVC relies on a cluster `StorageClass` (`global.storageClassName`).
  - New PVCs MUST follow this dual pattern. Reference: `charts/tensorleap/charts/node-server/templates/mongodb-pvc.yaml`. Never commit PVC size reductions — shrinking is unsupported.
- **Subcharts:** edit `engine`, `node-server`, `web-ui` locally. Configure external subcharts (`ingress-nginx`, `keycloakx`, `datadog`, `eck-operator`) through the parent `values.yaml` keyed by dependency name/alias — never edit tgz contents.
- **Chart.yaml:** dependencies pinned to exact versions; update both the version and the bundled tarball (`helm dependency build`).
- **Image tags:** `image_tag` values for `engine`, `node-server`, `web-ui`, pippin are updated by the `Update images` workflow from stable tags in the source repos. When editing by hand, also update the `*-latest-image` files at repo root AND run `make update-images`.

### Elasticsearch / ECK

- The `Elasticsearch` CR lives in `charts/tensorleap/templates/elasticsearch.yaml`. The CRD is installed by `tensorleap-infra` via `eck-operator`.
- If you change `spec.version`, also update the explicit `image:` field so `update-images` picks it up.
- `ES_JAVA_OPTS` heap must be ≤ 50% of the container memory limit AND ≤ 31Gi.
- `vm.max_map_count=262144` is set via a privileged initContainer on the ES pod — keep it.

## After editing charts

```
make build-helm
helm lint charts/tensorleap
helm template test charts/tensorleap > /tmp/out.yaml   # inspect diff
make update-images
make validate-images
```

## Commits

Imperative mood, capitalized first letter. Branches kebab-case (`-`, never `/`).
