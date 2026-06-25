# 01 · Architecture

System overview, the component map, the communication matrix, and the deployment
topology. Read the **Corrected mental model** in [README.md](README.md) alongside
this.

---

## Component map

```
                          ┌─────────────────────────────────────────────┐
        Browser ──────────►            ingress-nginx (host :4589→:80)     │
        (web-ui SPA)       │   / → web-ui   /api,/socket.io → node-server  │
        leap CLI ──────────►   /auth/* → keycloak   /session → minio       │
                          └───────┬───────────────┬───────────────┬───────┘
                                  │               │               │
                           ┌──────▼──────┐  ┌──────▼──────┐  ┌─────▼─────┐
                           │  web-ui     │  │  Keycloak   │  │  MinIO    │
                           │ (static)    │  │ (keycloakx) │  │ "bucket"  │
                           └─────────────┘  └─────────────┘  │  session  │
                                                             └─────▲─────┘
                                  ┌───────────────────────────────┼───────────┐
                                  │            node-server         │           │
            socket.io  ◄──────────┤  Express/tsoa /api/v2          │  bucket   │
            (to browser)          │  • Mongo entities              │  r/w      │
                                  │  • ES queries → dashlets ──────┼───────────┤
                                  │  • creates k8s Jobs (BatchV1)  │           │
                                  └───▲────────────────┬──────────-┘           │
                  RabbitMQ "feedback" │                │ k8s API               │
                  (engine→node-server)│                │ create Job            │
                                      │          ┌─────▼────────────────────┐  │
                            ┌─────────┴───────┐  │  ENGINE main pod (Job)    │  │
                            │ engine-         │  │  evaluate-<jobId>         │  │
                            │ orchestrator    │  │  • WorkerTrainer          │  │
                            │ (static Deploy) │  │  • creates per-job redis, │  │
                            │ • failed-job    │  │    streaming-handler,     │  │
                            │   detection     │  │    generic-process        │  │
                            │ • active-jobs   │  └───┬──────────┬─────────┬──┘  │
                            │   report        │      │          │         │     │
                            │ • scale generic │   redis-<jobId> (per-job, 5 queues)
                            └─────────────────┘      │          │         │     │
                                          ┌──────────▼──┐  ┌────▼─────────▼──┐  │
                                          │ engine-     │  │ streaming-      │  │
                                          │ generics    │  │ handler         │  │
                                          │ (customer   │  │ • ES metrics ───┼──┼─► Elasticsearch
                                          │  code:      │  │ • latent space ─┼──┘    (tl-elasticsearch
                                          │  preprocess │  │   to bucket     │       -es-master:9200)
                                          │  +get_sample│  └─────────────────┘
                                          │  +metrics*) │
                                          └─────────────┘
   * metrics run in engine-generics only in "dedicated metrics" mode; default = inline in trainer
```

**Static, always-on workloads** (Helm-managed): `tensorleap-web-ui`,
`tensorleap-node-server`, `mongodb`, `tensorleap-minio`, `engine-orchestrator`,
`rabbitmq` (StatefulSet), `tl-elasticsearch-es-master` (via ECK operator),
`keycloak`, `ingress-nginx`, `tensorleap-registry` (Zot, in infra), `datadog`
agent (DaemonSet).

**Transient, runtime-created workloads** (created by the engine, NOT in any
chart, visible only during a job): `evaluate-<jobId>` (and other `<subtype>-<jobId>`)
Job pods, `generic-process-<jobId>` Deployment, `streaming-handler-<jobId>`
Deployment, `redis-<jobId>` Pod+Service.

---

## Communication matrix

| From → To | Protocol | Purpose | Key evidence |
|---|---|---|---|
| Browser → ingress-nginx | HTTP(S) host :4589→:80 (/:443 TLS) | single entrypoint | `pkg/server/installation_params.go`, `pkg/k3d/cluster.go` |
| web-ui → node-server | HTTP REST `/api/v2`, header `KBearer <jwt>` | all entity CRUD + chart/dashlet data | `web-ui/src/core/api-client.tsx` |
| node-server → web-ui | **socket.io**, event `serverMessage`, room=user cid | live job/notification push | `node-server/src/utils/socket.ts` |
| web-ui ↔ Keycloak | OIDC (keycloak-js, PKCE, check-sso) | login / token refresh | `web-ui/src/keycloak.ts` |
| leap CLI → node-server | HTTP REST `/api/v2`, header `Bearer <apiKey>` | push / eval / projects / jobs | `leap-cli/pkg/api/apiClient.go` |
| leap CLI → Keycloak | OIDC password grant / PKCE → `/auth/keygen` | mint long-lived API key | `leap-cli/pkg/auth/login.go` |
| web-ui → bucket (MinIO) | HTTP GET (same origin via `/session`) / PUT to presigned URL | read vis blobs/images; upload artifacts | `web-ui/src/core/useProjectStorage.tsx` |
| node-server → MongoDB | mongodb wire | entities + relations | `node-server/src/utils/mongo/mongo.ts` |
| node-server → Elasticsearch | HTTP (`@elastic/elasticsearch` v8) | aggregation queries → dashlets | `node-server/src/utils/elastic/...` |
| node-server → MinIO | S3 (MinIO client) + presigned URLs | payloads, settings JSON, artifacts | `node-server/src/utils/storage.ts` |
| node-server → k8s API | BatchV1 (`createNamespacedJob`) | **create** engine/node jobs | `node-server/src/utils/k8s.ts` |
| node-server ↔ RabbitMQ | amqp (single durable queue `feedback`) | **consume** engine feedback; publish stop/terminate to `job-control-channel-<jobId>` | `node-server/src/utils/pubsub/*` |
| engine main pod → node-server | RabbitMQ (`FEEDBACK_TOPIC`) | job status, messages, version resources | `engine/.../rabbitmq/publisher.py` |
| engine orchestrator → node-server | RabbitMQ (`FEEDBACK_TOPIC`) | `failure_jobs_report`, `active_jobs_report` | `engine/.../workerenginescheduler/` |
| node-server → engine main pod | RabbitMQ (`SUBSCRIBER_TOPIC`) | stop a running job | `engine/.../workertrainer/workertrainer.py` |
| engine main pod ↔ engine-generics ↔ streaming-handler | **per-job Redis** (pickle over RESP), 5 queues | sample requests/responses, metrics, vis, streaming output | `engine/.../infrastructure/redis/*` |
| streaming-handler → Elasticsearch | HTTP bulk index | metrics + metadata docs | `engine/.../workerstreaminghandler/` |
| streaming-handler → bucket | MinIO SDK | latent-space vectors (evaluate queues only) | `engine/.../workerstreaminghandler/` |
| main engine pod / orchestrator → k8s API | k8s python client | create/scale per-job redis, generic, streaming-handler; detect failures | `engine/.../deployment_manager/` |
| engine builds → Zot registry | OCI registry (in-cluster :5000, host :5699) | push custom dependency images (pippin) | `helm-charts/charts/tensorleap-infra/.../zot-*`, `engine` job init container |

> **The single most common protocol confusion:** node-server→engine job dispatch
> is the **k8s API**, not RabbitMQ. RabbitMQ is feedback (engine→node-server) +
> control (stop/terminate).

---

## Deployment topology

- **Cluster:** k3d cluster named `tensorleap`; server container
  `k3d-tensorleap-server-0`; kube-context **`k3d-tensorleap`**; namespace
  **`tensorleap`** (the nvidia device plugin is the only exception — it lives in
  `kube-system`, and is off by default).
- **Two Helm releases, strict order:** `tensorleap-infra` **then** `tensorleap`.
  Infra carries the **Elasticsearch CRD** (via the `eck-operator` subchart), the
  **Zot** registry, and the optional nvidia device-plugin DaemonSet. The app
  chart's `Elasticsearch` CR (`tl-elasticsearch`) can only reconcile because the
  CRD already exists — that is why ordering is mandatory.
- **Entry:** ingress-nginx is the single HTTP/HTTPS entrypoint; k3d maps host
  **4589 → container 80** (and **→ 443** when `global.tls.enabled`). Override
  with `leap server install --port`.
- **Ingress routes:** `/` → `tensorleap-web-ui:8080`; `/api` & `/socket.io` →
  `tensorleap-node-server:80`; `/auth/realms` & `/auth/resources` →
  `keycloak-http`; `/session` → `tensorleap-minio:9000`.
- **Persistence (two modes via `global.create_local_volumes`):**
  - `true` (k3d single-node default): static `hostPath` PVs under
    `/var/lib/tensorleap/standalone/storage/<name>`.
  - `false`: PVCs bind to `global.storageClassName`.
  - PVCs: `mongodb-data` (8Gi), `elasticsearch-data-tl-elasticsearch-es-master-0`
    (60Gi), `tensorleap-minio` (2Gi), `keycloak-data` (8Gi), `rabbitmq-data`
    (500Mi). **Never shrink a PVC.**

### Services, ports, images (verify against `images.txt`)

| Workload | Service:port | Image (snapshot) |
|---|---|---|
| web-ui | `tensorleap-web-ui:8080` | `public.ecr.aws/tensorleap/web-ui:master-f16249f1` |
| node-server | `tensorleap-node-server:80` → pod `:4000` | `public.ecr.aws/tensorleap/node-server:master-9276bb7c` |
| mongodb | `mongodb:27017` | `mongo:6.0.5` |
| rabbitmq | `rabbitmq:5672` (amqp) + `:15672` (mgmt UI) | `rabbitmq:3.9.22` |
| minio (bucket `session`) | `tensorleap-minio:9000` (api) + `:9001` (console) | `minio RELEASE.2021-12-20T22-07-16Z` |
| elasticsearch | `tl-elasticsearch-es-master:9200` | `elasticsearch:8.10.1` (ECK operator `2.8.0`) |
| keycloak | `keycloak-http` | `keycloak:26.3.2` |
| ingress-nginx | `:80/:443` (host 4589/443) | `ingress-nginx/controller:v1.10.0` |
| zot registry | `tensorleap-registry:5000` (host 5699) | `zot v2.1.15` |
| engine / orchestrator / engine jobs | (no static svc; orchestrator is a Deployment) | `public.ecr.aws/tensorleap/engine:master-026729cf` |
| engine-generics | (runtime Deployment) | `public.ecr.aws/tensorleap/engine-generic:master-026729cf-py{38,39,310,312}` |
| pippin (dep builder) | (job init container) | `public.ecr.aws/tensorleap/pippin:master-26a41e94` |
| per-job redis | `redis-<jobId>:6379` (runtime) | `docker.io/library/redis:8.6-alpine` |
| datadog agent | DaemonSet | `gcr.io/datadoghq/agent:7.52.0` |

### Debug credentials (default local install)

| Component | Creds |
|---|---|
| MinIO root | `foobarbaz` / `foobarbazqux` (`minio-secret`) |
| RabbitMQ | `guest` / `guest` |
| Keycloak admin | `admin` / `admin`, realm `tensorleap`, client `tensorleap-client` |
| MongoDB | URI `mongodb://mongodb/tensorleap` (no auth, TLS off) |
| Elasticsearch | security disabled, anonymous superuser |

> These are default standalone values; a customer/cloud install will differ. Read
> them live from the relevant Secret/ConfigMap rather than assuming.

---

## Versioning signals a QA engineer should track

- `charts/tensorleap/Chart.yaml` `version` — a **minor** bump (`1.5.x → 1.6.0`)
  signals **cluster reinstall required** (the installer compares minor versions;
  reinstall wipes the cluster). Current snapshot: tensorleap `1.6.33`, infra `1.1.8`.
- `pkg/version/version.go` — the Go installer (`leap server`) version. `leap server info` prints it.
- Reinstall is also triggered by an infra version change or a manifest
  appVersion/schemaVersion change (see `DEVELOPER-GUIDE.md`).

See [04-job-types-and-lifecycle.md](04-job-types-and-lifecycle.md) for job
mechanics and [05-testing-utils.md](05-testing-utils.md) for how to inspect any
of the above on a live cluster.
