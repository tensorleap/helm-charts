# Tensorleap QA Knowledge Base

A knowledge base for an automated **senior QA engineer** testing the Tensorleap
platform. Any agent that runs a test plan should load the relevant parts of this
KB first, so it tests against how Tensorleap *actually* works — not a guess.

The content is grounded in a full read of the four product repos
(`helm-charts`, `engine`, `node-server`, `web-ui`) plus the `leap-cli` and
`code-loader` contracts. Where the naive mental model is wrong, this KB says so
explicitly (see **Corrected mental model** below — read it once, in full).

---

## How a test-plan agent should use this KB

You are a senior QA engineer. You have years of Tensorleap experience and are
fluent with `kubectl`, `docker`/`k3d`, Redis/Mongo/Elasticsearch CLIs, and
inspecting rendered UI. When handed a test plan:

1. **Orient** — skim [01-architecture.md](01-architecture.md) for the component
   map and the corrected mental model.
2. **Map each test step to a component/flow** — use
   [02-components.md](02-components.md) and [03-data-flows.md](03-data-flows.md)
   to know *what should happen* and *what is observable* at each step.
3. **Pick the right observation tool** — start with the decision matrix in
   [10-verification-toolbox.md](10-verification-toolbox.md) (which tool for which
   claim: kubectl / ES / Mongo / Datadog / Playwright …), then the command cookbook
   in [05-testing-utils.md](05-testing-utils.md) (cluster/back-end) and
   [06-ui-inspection.md](06-ui-inspection.md) (front-end).
4. **Decide pass/fail with evidence** — never assert on UI alone if a back-end
   signal exists. A test passes only when the observable signal at each step is
   present; an empty/error UI state is a *specific* signal, not "broken" — see
   the dashlet empty-state taxonomy in [06-ui-inspection.md](06-ui-inspection.md).
5. **When something fails, classify it** — [07-failure-modes.md](07-failure-modes.md)
   maps symptoms to root causes and the exact command that confirms each.
6. **Structure the run** — [08-test-playbooks.md](08-test-playbooks.md) has
   reusable templates (push, evaluate, dashboard render, version control) and the
   rules for turning a free-form test plan into verifiable steps.

Reference material: job lifecycle & resource sizing in
[04-job-types-and-lifecycle.md](04-job-types-and-lifecycle.md), the full
per-subType catalog (Population Exploration, Insights, curation, Push, project
ops, …) in [09-job-catalog.md](09-job-catalog.md), and the which-tool-when guide in
[10-verification-toolbox.md](10-verification-toolbox.md).

> **Keeping this KB current:** the docs are maintained by a scheduled GitHub
> Action (and reactive in-run fixes) — see
> [maintenance/MAINTENANCE.md](maintenance/MAINTENANCE.md). If you (a QA agent)
> find a doc contradicts reality during a run, fix the doc and log it per that
> protocol.

---

## The system in one paragraph

A user (browser or `leap` CLI) authenticates via **Keycloak** and reaches
**node-server** through a single **ingress-nginx** entrypoint (host port **4589**
by default). **node-server** (Express/tsoa) is the brain: it stores entities in
**MongoDB**, queries **Elasticsearch** to build dashboard "dashlets", reads/writes
payloads in the **MinIO** bucket, and **creates Kubernetes Jobs** for compute. A
compute job runs the **engine** image; the main engine pod spins up its own
**per-job Redis**, an **engine-generics** (generic-process) Deployment that runs
the customer's integration code, and a **streaming-handler** Deployment that
writes metrics/metadata to Elasticsearch and latent-space vectors to the bucket.
The engine reports progress back to node-server over **RabbitMQ**, and node-server
pushes live updates to the browser over **socket.io**. A long-lived
**engine-orchestrator** watches for failed jobs and reports them. Everything runs
in a single-node **k3d** cluster (`k3d-tensorleap`, namespace `tensorleap`) on the
customer's machine, so resource sizing is done by an **auto-settings** mechanism
that must be infrastructure-agnostic.

---

## The four repos

| Repo | Path | What it is |
|---|---|---|
| `helm-charts` | `/Users/asafyehezkel/tensorleap-projects/helm-charts` | Helm charts + the `leap server` Go installer CLI (k3d lifecycle). **Primary repo / home of this KB.** |
| `engine` | `/Users/asafyehezkel/tensorleap-projects/engine` | Python compute: main engine, engine-generics, streaming-handler, orchestrator. |
| `node-server` | `/Users/asafyehezkel/tensorleap-projects/node-server` | TypeScript Express/tsoa API server + job creator. |
| `web-ui` | `/Users/asafyehezkel/tensorleap-projects/web-ui` | React 19 SPA (dashboards, version control, network editor). |
| `leap-cli` *(not checked out here)* | — | The user-facing `leap push / auth / projects / run` CLI. `leap server …` is imported from `helm-charts`. |
| `code-loader` *(not checked out here)* | — | Python contract (`@tensorleap_*` decorators) for customer integrations. |

---

## ⚠️ Corrected mental model (read this once, fully)

These are the points where a reasonable-but-wrong assumption will make you write
a bad test or misdiagnose a failure. Each is verified against code.

1. **"Evaluate" is NOT an engine job type.** It is a node-server **JobSubType**
   on a job of `type: TRAINING` (`subType: 'Evaluate'`). The engine main pod
   runs **WorkerTrainer**, not a dedicated evaluator. The resulting k8s Job is
   named **`evaluate-<jobId>`**. (`node-server/src/evaluate/logic.ts`,
   `engine/.../manager/manager.py`)

2. **Jobs are created DIRECTLY via the Kubernetes API, not over RabbitMQ.**
   node-server renders the `engine-job-template-cm` ConfigMap and calls
   `BatchV1Api.createNamespacedJob`. RabbitMQ carries only the **reverse**
   direction (engine → node-server feedback) and **control** messages (stop /
   terminate). (`node-server/src/utils/engine.ts`, `src/utils/k8s.ts`)

3. **The MAIN engine pod creates the extra runtime resources** — per-job Redis
   pod+service, the streaming-handler Deployment, and the generic-process
   Deployment. Not node-server, not the orchestrator. They exist only while the
   job runs and are **never** Helm-managed. (`engine/.../manager/manager.py`,
   `deployment_manager.py`)

4. **Redis is per-job and there are multiple logical queues**, not one shared
   queue. One Redis pod `redis-<jobId>` carries: dataset request queue, dataset
   ready queue, metrics queue, visualizer queue, and the streaming queue. (See
   [03-data-flows.md](03-data-flows.md) and [02-components.md](02-components.md).)

5. **Division of labor in an Evaluate:** *engine-generics* runs the **customer**
   `preprocess` + `get_sample` to produce inputs & ground-truth; the **main
   engine pod** pops those samples, builds the model, and runs **inference**.
   Metrics are computed **inline in the trainer by default**, and in
   engine-generics only when "dedicated metrics" mode is enabled. (`engine/.../leaptrainer.py`,
   `workergenericprocessor/`)

6. **The streaming-handler scaler runs INSIDE the main engine pod** (a daemon
   thread), not in the orchestrator. The **orchestrator** (the static
   `engine-orchestrator` Deployment) does failed-job detection, active-jobs
   reporting back to node-server, and generic-process replica scaling. (`engine/.../manager/streaming_handler_scaler.py`,
   `workerenginescheduler/`)

7. **streaming-handler is not Evaluate-specific** — TRAINING, ANALYZE, and
   SYNTHETIC jobs also create one. It writes metrics+metadata to **Elasticsearch
   always**, and latent-space vectors to the **bucket only for "evaluate"
   queues**. (`engine/.../workerstreaminghandler/`)

8. **Auth schemes differ by client.** The **browser** sends a *custom*
   `Authorization: KBearer <jwt>` header (NOT standard `Bearer`). The **CLI**
   sends `Authorization: Bearer <apiKey>`. node-server branches on the `KBearer`
   prefix. A test harness that injects `Bearer` on a browser-style call gets a
   401. There is also a non-Keycloak **LocalProvider** (`demo@demo.ai`) path.
   (`web-ui/src/core/api-client.tsx`, `node-server/src/auth/authHandler.ts`)

9. **Port 4589 is the installer's default HOST port** that k3d maps to
   ingress-nginx `:80`. It does **not** appear in any ingress/nginx manifest —
   don't grep the charts for it. (`helm-charts/pkg/server/installation_params.go`)

10. **Dashlet data is queried live from Elasticsearch at view time** by
    node-server; it is not pre-stored. The `dashboards` Mongo collection stores
    only dashlet *config* (layout, type, pinned filters). The browser **never**
    queries Elasticsearch directly. (`node-server/src/modelmetrics/logic.ts`)

11. **The CLI flag is `--overwrite` / `-o`**, not `--override`.
    `--overwrite-version` is a deprecated alias. Sub-version / artifact-refresh
    behavior is driven by `-u/--update` (which implies `--eval`). (`leap-cli/cmd/root_cmd/push.go`)

12. **`leap server check` is a stub** that only prints "Check command" — it does
    no real validation. Don't rely on it. (`helm-charts/cmd/server/check.go`)

---

## Conventions used in this KB

- **Evidence pointers** are repo-relative paths, sometimes with a line number
  (`node-server/src/evaluate/logic.ts:507`). Treat line numbers as **anchors that
  may drift** — grep for the named symbol if a line doesn't match.
- **Names that matter** (cluster, namespace, services, labels, queue/index
  patterns) are quoted verbatim from code and are safe to hard-code in tests.
- **`<jobId>`** is the Mongo job `_id` (also the `jobId` pod label). **`<jobUid>`**
  / `job.uid` appears in bucket payload names. **`<versionId>`** is the model
  version (experiment) id.
- **Verify-against-live notes**: where the recon left an open question, the doc
  says "verify on a live cluster" rather than asserting.

> This KB is a snapshot of `master` across the repos. When image tags or chart
> versions are cited (see [01-architecture.md](01-architecture.md)), confirm the
> deployed build with `images.txt` and `kubectl ... get deploy -o yaml`.
