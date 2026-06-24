# 10 · Verification toolbox (which tool, when)

A senior QA engineer reaches for different tools depending on *what* they are
verifying. This is the decision guide: pick the tool that gives the cleanest,
most direct evidence for the claim under test. Commands for each tool are in
[05-testing-utils.md](05-testing-utils.md); UI selector rules are in
[06-ui-inspection.md](06-ui-inspection.md).

---

## Decision matrix

| What you're verifying | Primary tool | Why / notes |
|---|---|---|
| A workload/pod exists, is scheduling, restarted, OOMKilled | **kubectl** | `get pods -l …`, `describe pod` Events. Authoritative for *current* cluster state. |
| Logs of a **currently-running** pod you can name now | **kubectl logs** | `--follow`, `--previous` for the last crash. Only works while the pod exists. |
| Logs of a **finished/deleted** pod, a **past** run, by user/job, or **across** pods | **Datadog** (`ddquery`) | Engine/node jobs + engine-pod deployments are ephemeral and get deleted — `kubectl logs` returns nothing; Datadog retains them and can correlate by `@trace_id`. |
| Entity state: job status, version resources, project, insights, curation rows | **MongoDB** | `db.jobs`, `db.versions`, `db.insights`, `db.datasetbalancing`, … Source of truth for "did node-server record it". |
| Dashlet/metric **data**: index exists, doc count, aggregation buckets | **Elasticsearch** | `_cat/indices`, `<index>/_count`, `_search`. The "is there data behind the dashlet" check. |
| Job feedback flow / status stuck / live updates missing | **RabbitMQ** mgmt UI | Queue `feedback` depth + consumer presence. Status-desync usually lives here. |
| Payloads, artifacts, latent space, scatter/cluster JSON, exports | **MinIO bucket** | The job's real output often lands here before/independent of mongo flags. |
| Engine dataflow heartbeat (sample/metrics/streaming backlog) | **per-job Redis** | `LLEN` the queues (TRAINING/ANALYZE/PUSH only — SLIM_LS has none). |
| Custom dependency image built & pushed | **Zot registry** | `/v2/_catalog`, tags. A push/eval hung in the pippin init stage. |
| Cluster up, install state, chart versions | **k3d / helm / leap server** | `k3d cluster list`, `helm list`, `leap server info`. |
| The **rendered UI**: dashlet drew, selector present, login works | **Playwright** (browser MCP) | Real browser against `:4589`; assert DOM/snapshot. See the recipe below. |
| Front-end telemetry / a past user session | **Datadog RUM** | `service:web-ui`; the only place browser errors/sessions appear. |

**Golden rule:** pair every UI assertion with the matching back-end observable.
A dashlet that "looks right" over stale/empty data is the classic false pass —
confirm the ES `_count` (or bucket object) behind it.

**Live vs historical:** use **kubectl** for what's happening *now* on a pod you can
name; use **Datadog** the moment the pod is gone, the run is in the past, or you
need to follow one flow across node-server → engine → engine-pod.

---

## Datadog (historical logs, metrics, traces, RUM)

Use the **`ddquery`** skill, which drives the `plugin:datadog:mcp` server (enabled
in `~/.claude/settings.json`). Setup/troubleshoot: `/datadog:ddsetup`,
`/datadog:ddconfig`, `/datadog:ddtoolsets`.

### Log `service` names (the `service:` to query)

| `service:` | Component |
|---|---|
| `orchestrator` | engine-orchestrator container |
| `engine` | Tensorleap-side engine job containers (engine, image-dependencies-builder) |
| `engine-pod` | **user-code** generic-process deployment pods (metrics/encoders/dataloaders) |
| `node-server` | node-server deployment (+ mongodb sidecar) |
| `node-job` | node-server job pods (Export/Import Project) |
| `web-ui` | front-end RUM service |

> "engine pods" is ambiguous — search **both** `engine` (TL processing) and
> `engine-pod` (user code). The UST `service` tag (= Helm release name, usually
> `tensorleap`) is different from these per-container log `service` values.

### Query patterns
- Tags are queried **bare**: `env:<env>`, `kube_job:<name>`, `image_tag:<tag>`.
- Custom attributes use **`@`** (dropping `custom.`): `@user.email:<email>`,
  `@job.uid:<uid>`, `@trace_id:<id>`, `@status`, `@duration_seconds`.
- Workflow: (1) confirm MCP connected; (2) scope by `service`; (3) narrow by
  `@user.email` + `@job.uid` (from `kubectl describe pod` or the `kube_job` tag) or
  a tight time window; (4) use `@trace_id` only to follow one flow across services.
- Practice: locate with small probes, then fetch ~300 logs **once** with all
  fields, save to a scratch file, analyze locally. Parallel/joblib logs can share a
  millisecond — order by attribute, not timestamp.

### When Datadog beats kubectl
(a) the pod is already gone (ephemeral jobs/engine-pods); (b) attributing a **past**
run to a user/job/time; (c) **cross-pod** correlation via `@trace_id`
(node-server → engine → engine-pod) — kubectl can't stitch this; (d) metrics/APM
history and RUM front-end sessions that never appear in kubectl.

> **Tracing caveat:** node-server emits real APM traces (`dd-trace`, header
> `X-Datadog-Trace-Id`) and web-ui RUM is linked via `allowedTracingUrls`. **Engine
> APM is currently disabled** (pending a Python 3.10+ upgrade) — engine call-trees
> are in logs as `@span_*` attributes, not native APM spans.
> No Datadog dashboards/monitors are defined in the repos — discover any via the
> Datadog MCP, not the codebase.

---

## Front-end verification with Playwright (and the browser MCPs)

There is **no in-repo e2e suite** (no Playwright/Cypress config; web-ui has only
Jest unit tests + Storybook/Chromatic). So asserting the rendered UI is a
browser-driver task. Three browser-automation MCP servers are available in this
environment:

| MCP server | Best for |
|---|---|
| **`playwright`** (`@playwright/mcp`) | deterministic DOM assertions, snapshots, network capture — **preferred for QA assertions** |
| **`Claude_in_Chrome`** | driving a real Chrome (exploratory, uses your logged-in session) |
| **`Claude_Preview`** | quick preview/screenshot flows |

### What to point at
- **App under test:** `http://localhost:4589` (k3d host → ingress-nginx). This is
  what a driver should hit. **Not** the Vite dev server `:3000` (that's for
  front-end dev; it proxies `/api`,`/auth`,`/socket.io` → `:4000` and `/session` →
  `:4589`).
- TLS install → `https://localhost:443`.

### The login + assert recipe
1. **Navigate** to `http://localhost:4589`.
2. **Keycloak login is interactive** — it's a full redirect to
   `http://localhost:4589/auth` (realm `tensorleap`, client `tensorleap-client`),
   not a programmatic token. Fill username/password on the Keycloak page and
   submit; the SPA then stores the token and proceeds. (First-ever user is sent to
   **register**, not login.)
3. **Use a persistent-session driver** so the Keycloak token survives across
   assertions (re-login per step is slow and flaky).
4. **Assert** using the selector rules in [06-ui-inspection.md](06-ui-inspection.md):
   prefer `TOUR_SELECTORS_ENUM` element **`id`s** (`#analytics-dashlet`,
   `#population-exploration-circles`, `#sample-analysis-dashlet-loaded-content`,
   `#insights-list`, …), then `aria-label`, then MUI DataGrid attributes.
5. **Combine negative + positive assertions** for "rendered with data": none of the
   empty/error texts present, and chart geometry/rows present. Use the empty-state
   taxonomy in [03-data-flows.md](03-data-flows.md) / [09-job-catalog.md](09-job-catalog.md).
6. **Capture network + console** to tie the render to its API call (e.g. a
   `getXYChart` 200 with `charts.length>0`, a `/socket.io` 101, a presigned bucket
   GET).

### ⚠️ Auth header gotcha for any non-browser API assertion
The browser sends **`Authorization: KBearer <token>`** (custom scheme, *not*
`Bearer`). A real browser driver gets this for free. If you hand-roll a `curl`/HTTP
assertion against `/api/v2`, you must use the `KBearer ` scheme **with a token
minted via Keycloak** — a plain `Bearer` is treated as the CLI API-key path and
will 401 a browser-style call. For a quick unauthenticated smoke check use
`GET /api/v2/monitor/healthCheck`.

---

## Worked example — verify an Evaluate produced a populated dashlet

Choosing the right tool at each step:

1. **Cluster healthy?** → `kubectl get pods` + `curl …/monitor/healthCheck` (k8s + node-server).
2. **Job created & dispatched?** → `db.jobs` (Mongo) shows `type:TRAINING, subType:Evaluate`; `kubectl get jobs` shows `evaluate-<jobId>`.
3. **Runtime resources came up?** → `kubectl get pod,deploy,svc -l jobId=<jobId>` (redis + generic-process + streaming-handler).
4. **Data actually written?** → `db.versions` `resources.es_metrics_index`, then `GET <es_metrics_index>/_count` (Elasticsearch) rises to ~#samples.
5. **Job reported FINISHED to node-server?** → `db.jobs` status + (if stuck) RabbitMQ `feedback` queue/consumer.
6. **It rendered with data?** → Playwright at `:4589`: open the version's dashboard, assert `#analytics-dashlet` has chart geometry and no "No results found"; confirm the `getXYChart` 200 in the network panel.
7. **If a pod died mid-run and is gone** → Datadog `service:engine-pod @job.uid:<uid>` for the failure, or `@trace_id` to follow node-server → engine → engine-pod.

Each claim is verified with the tool that owns that fact — not inferred from a
neighboring one.
