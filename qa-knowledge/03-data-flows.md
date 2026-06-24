# 03 · Data flows (step-by-step, with observables)

The two flows a QA engineer must know cold. Each step lists **who acts**, **the
mechanism**, and **the observable** (what you check to prove the step happened).
Observables are the backbone of any test assertion.

---

## Flow A — End-to-end Evaluate

Trigger (`leap push --eval` or UI **Evaluate**) → node-server → engine main pod →
per-job Redis + engine-generics + streaming-handler → Elasticsearch + bucket →
dashlets → web-ui.

> Recall from [README.md](README.md): job creation is via the **k8s API**, not
> RabbitMQ; "Evaluate" is `type:TRAINING / subType:Evaluate`; the **main engine
> pod** creates the runtime resources.

| # | Actor | Action | Observable (how QA verifies) |
|---|---|---|---|
| 1 | CLI / web-ui | POST `/evaluate/evaluate` `{versionId, projectId, batchSize, noVisualization}` through nginx :4589 | node-server access log `POST /evaluate/evaluate 200`; browser Network tab returns a Job with status `UNSTARTED` |
| 2 | node-server (evaluate logic) | Guards (`assertWithinConcurrentEvaluateLimit`, `assertNoActiveEvaluateJob`); allocates `inference_artifact_id`, `vis_artifact_id`, `es_model_id` on the version; writes `evaluateParams` | `db.versions.findOne({_id})` shows `resources.*` populated + `evaluateParams`. Over limit → HTTP **429** `CONCURRENT_EVALUATE_LIMIT` |
| 3 | node-server | Inserts job `{type:'TRAINING', subType:'Evaluate', status:'UNSTARTED', trigger:'Manual'}`; builds `EvaluateRequest` | `db.jobs` has the new doc; job id returned to client |
| 4 | node-server (`engine.ts`) | Uploads request JSON to bucket; reads `engine-job-template-cm`; resolves machine type + auto-settings; uploads `k8s_engine_generic_settings.json` + `k8s_engine_redis_settings.json`; **creates the k8s Job**; marks job `PENDING` | `kubectl -n tensorleap get jobs` shows **`evaluate-<jobId>`** with labels `jobId=<jobId>`, `projectId=<projectId>`, `hasWorker=true`; bucket has the payload JSON + `pods-settings/<versionId>/…`; job → `PENDING`; UI gets a PENDING notification |
| 5 | k8s + main engine pod | Schedules the pod; optional **pippin** init container builds a custom generic image; entrypoint pulls payload, registers RabbitMQ feedback, publishes `STARTED`, dispatches to `Manager.run` | `kubectl get pods -l jobId=<jobId>` Running; pod log `starting engine worker...` → `manager is running`; job `PENDING→INITIALIZING→STARTED` |
| 6 | main engine pod (`Manager.run`) | Creates **per-job Redis** Pod+Service (waits for PING), then the **streaming-handler** Deployment (replicas=1) and the **generic-process** Deployment | `kubectl -n tensorleap get pod,svc,deploy -l jobId=<jobId>` → `redis-<jobId>` Pod+Svc, `deploy/streaming-handler-<jobId>`, `deploy/generic-process-<jobId>`; pod log `Per-job Redis ready`; `redis-cli -h redis-<jobId> PING → PONG` |
| 7 | main engine pod (LeapTrainer) | Publishes version resources (`es_metrics_index`, `es_inspection_index`) to node-server; loads weights; writes inspection report to ES; enqueues sample ids to the dataset `to_generate` queue; starts streaming scaler | `db.versions` now has `resources.es_metrics_index/es_inspection_index`; `GET <ES>/_cat/indices` shows the metrics + inspection indices; `redis-cli LLEN dataset_<state>_to_generate_<jobId> > 0` |
| 8 | engine-generics | Runs customer `preprocess`+`get_sample` → inputs + ground-truth; pushes ready samples to `dataset_<state>_<jobId>_ready`; self-sizes memory | `kubectl logs deploy/generic-process-<jobId>` `samples generator: generate single sample`; `LLEN dataset_<state>_<jobId>_ready` rises; customer error → `Sample generator failed` + a UI warning |
| 9 | main engine pod (`evaluate_epoch`) | Pops ready samples, runs **inference**, computes metrics inline (default), pushes batches to `streaming_evaluate_<jobId>_queue` (or `metrics_<jobId>` in dedicated mode) | pod log `Pushing elements to redis key: streaming_evaluate_<jobId>_queue`; `LLEN streaming_evaluate_<jobId>_queue > 0`; backpressure = push loop blocks when len > threshold |
| 10 | streaming-handler | Pulls bulks (≤500 or 20s), uploads latent space to bucket (evaluate queues only), indexes metrics+metadata to ES (`es_metrics_index`) | `kubectl logs deploy/streaming-handler-<jobId>` `indexing docs to elasticsearch`; `GET <es_metrics_index>/_count` rises; bucket latent-space objects appear under `inference_artifact_id` |
| 11 | main engine pod (scaler thread) | Scales streaming-handler replicas from push/pull counters | `kubectl get deploy streaming-handler-<jobId>` replica count changes; scaler log lines |
| 12 | main engine pod (completion) | Waits for metrics + streaming queue drain; runs `post_processing_evaluate` (visualizers/insights unless `noVisualization`); publishes `FINISHED` + final resources; Job completes; runtime resources torn down | `kubectl get job evaluate-<jobId>` `succeeded=1`; per-job pods/deploys/svc disappear; job `FINISHED`; UI flips version to evaluated |
| 13 | node-server (RabbitMQ subscriber) | Throughout: consumes feedback (`update_job_status`, `update_session_action`, `failure_jobs_report`, `active_jobs_report`, version resources); updates Mongo; forwards to browser via socket.io | node-server log handler activity; Mongo job/version update in real time; WS `serverMessage` frames in DevTools |
| 14 | orchestrator | Independently polls k8s for failed/active pods; reports so a pod that died silently is still marked FAILED | `engine-orchestrator` logs; job flips `FAILED` even after an OOMKill that never reported |
| 15 | web-ui + node-server (dashlets) | On opening dashboards, UI requests dashlet data; node-server runs ES aggregations vs `es_metrics_index` + reads bucket payloads; UI renders | Network tab: dashlet/model-metrics requests return data; UI renders populated charts. **Empty dashlet ⇒ `es_metrics_index` missing/empty ⇒ eval didn't finish** |

### Evaluate failure modes (full catalog in [07-failure-modes.md](07-failure-modes.md))
- Customer code error in `get_sample` → `Sample generator failed`; if >20% samples
  discarded → `Evaluation failed due to too many discarded samples` (job FAILED).
- `DatasetScriptException` → UI `Dataset code had crashed`, job FAILED.
- OOMKill (under-provisioned auto-settings) → pod `OOMKilled`/exit 137/`Evicted`;
  orchestrator maps all to `OOM_KILLED`.
- ImagePull on engine/generic/redis → pod `ImagePullBackOff`; redis-specific fast-fail.
- Redis backpressure → `LLEN streaming_evaluate_<jobId>_queue` stays high; `noeviction`
  means a too-small redis OOMs rather than evicts.
- ES NaN/indexing drops → `failed to index docs to elasticsearch`; `_count` < evaluated samples.
- ES 503 low disk → `low available disk space (<150GB)`.
- Feedback lost → reconciled to FAILED by orchestrator active/failed-jobs polling.
- Concurrency guard → `An evaluate job is already running` / HTTP 429.

---

## Flow B — Auth + UI render (dashboard/dashlet)

Browser → nginx :4589 → Keycloak → web-ui → node-server REST/socket.io →
Mongo/ES/bucket → rendered dashlet.

| # | Actor | Action | Observable |
|---|---|---|---|
| 1 | Browser | Open `http://localhost:4589`; k3d maps host 4589 → ingress-nginx :80 (4589 is the installer default, **not** in any ingress manifest) | first `GET /` → `index.html` 200 |
| 2 | ingress-nginx | Path routing: `/api`,`/socket.io` → node-server; `/auth/realms`,`/auth/resources` → keycloak; else → web-ui | all of UI/API/auth share one origin (no cross-origin) |
| 3 | web-ui | `getAuthProvider()` → Local vs Keycloak; shows `AuthLoadingScreen` (`role=status`, `aria-label='Loading authentication'`) until known | POST `…/getAuthProvider` 200; DOM element `role=status` present during load |
| 4 | web-ui Keycloak adapter | `keycloak.init({onLoad:'check-sso'})`; if unauthenticated → `getAuthStatus()`: `NoUsers` → `register()`, else `login()` (redirect) | `GET /auth/realms/tensorleap/...` 200; redirect to Keycloak login; cookies `KEYCLOAK_SESSION`/`KC_RESTART` on `/auth` |
| 5 | Keycloak | Credentials → auth code → PKCE token exchange | `POST …/openid-connect/token` 200 `{access_token, refresh_token, id_token}`; URL cleaned of code/state; app shell renders |
| 6 | web-ui api-client | Silent `refreshToken()`, then header **`Authorization: KBearer <token>`** (custom scheme), `credentials:'include'`, basePath `<origin>/api/v2` | DevTools: any `/api/v2` request → `Authorization: KBearer eyJ…`; a `uniqueId` cookie is sent |
| 7 | node-server auth | `expressAuthentication` → `getAuthUser`: `isUserRequest` requires `KBearer` prefix; decode email → `usersDb.findByEmail`; `validateConcurrentUsers` vs cookie `uniqueId` → 409 on mismatch; scope checks | 401 `No token provided` (missing/wrong scheme); 401 `User not found`; 409 `Conflict user connections`; 403 demo/license |
| 8 | web-ui socket.io | `io(WS_URL,{path:'/socket.io', withCredentials:true, auth:{Authorization:'KBearer <token>'}})`; listens `serverMessage`,`authenticated`,`connect_error` | DevTools → WS: `/socket.io` upgrade 101; **auth is in the handshake `auth` payload, not a header**; server emits `authenticated` |
| 9 | node-server socket server | Reads `handshake.auth.Authorization` → find user → `socket.join(userId)` → emit `authenticated`; `sendMessage(userId,msg)` emits `serverMessage` | node-server log `WS client connected` → `WS Sending message to user`; bad token → `authentication_error` |
| 10 | web-ui dashboard | Dashlet resolves shared fields via `getDashletFields` (ES mappings); branches into empty/error/data states | POST `…/dashboards/getDashletFields` 200; the DOM **text** is the QA signal (see taxonomy below) |
| 11 | web-ui Analytics dashlet | Wraps content in `<div id="analytics-dashlet">`; if no version selected → `Select a version to see data.` else renders the chart | DOM id `analytics-dashlet` present; empty-version text distinguishes "no version" from "no data" |
| 12 | web-ui XYViz → node-server | `getXYChart({projectId, versions, filters(mapToEsFilters), xField, yField, aggregation})` | POST `…/sessionmetrics/getXYChart` 200 `{charts:[…]}`; `charts:[]` = empty-ES case |
| 13 | node-server → Elasticsearch | Builds aggregation query vs the team/version index; runs `_search` | `GET /<index>/_search` returns buckets; confirm index: `GET /_cat/indices?v | grep <teamId>`; node-server log `Index exists: <bool>` |
| 14 | web-ui MultiCharts (render proof) | `isLoading` → spinner; `!charts.length||error` → `NoDataChart` ("No results found"); per-cell no-data → `No data` span; else draws cells | **Assert negatively** (no "No results found", no "No data" span, no spinner) **and positively** (chart SVG/canvas inside `#analytics-dashlet`, axis labels, plotted series) |
| 15 | node-server storage (bucket) | For sample/population payloads, returns MinIO **presigned GET** URLs (host rewritten to same origin) or streams the object; UI GETs the blob | DevTools: request to a presigned URL (`X-Amz-Signature`/`Expires`) 200; `#population-exploration-processing` while preparing → `#population-exploration-dashlet` when rendered |
| 16 | engine → RabbitMQ → node-server → socket.io → UI | Job status flows back; node-server emits `serverMessage`; UI refetches | node-server log `WS Sending message to user`; rabbitmq mgmt UI shows queue activity; after an Evaluate completes, a previously "No results found" dashlet repopulates **without page reload** |

### Auth/UI failure modes (full catalog in [07-failure-modes.md](07-failure-modes.md))
- **Auth redirect loop**: check-sso fails (clock skew / wrong realm URL / blocked
  cookies; on non-localhost plain HTTP the Web-Crypto polyfill must be active or
  PKCE S256 fails) → `AuthLoadingScreen` never clears.
- **Wrong auth scheme → 401**: harness sending `Bearer` instead of `KBearer`.
- **User not in Mongo → 401** `User not found` despite a valid token.
- **Concurrent-user 409 loop** → forced to `/conflict-users`.
- **CORS / nginx misroute**: `/socket.io` returning the web-ui `index.html` (HTML)
  instead of an engine.io handshake means the `/socket.io` path rule isn't matching.
- **Socket disconnect / auth fail**: WS `authentication_error`; live updates stop.
- **Empty dashlet** — distinguish the causes (next).

---

## Dashlet empty-state taxonomy (do not conflate these)

| DOM text | Stage | Meaning | Is it a bug? |
|---|---|---|---|
| `Loading...` | dashletFields | still fetching config | transient |
| `Sorry, there was an error fetching the visualization's config` | dashletFields | `getDashletFields` failed (ES mapping endpoint / ES down) | **yes** — investigate ES |
| `Training/Evaluation process is required to visualize data` | dashletFields | no aggregatable/numeric fields in the mapping | no — pre-data state |
| `Select a version to see data.` | Analytics dashlet | no model version selected | no — user input state |
| `No results found` (NoDataChart) | MultiCharts | `getXYChart` returned `charts:[]` (index missing / no metrics / filters exclude all) | depends — see [07](07-failure-modes.md) |
| `No data` (per cell) | MultiCharts cell | that cell's aggregation is empty | depends |

A **rendered** dashlet = none of the above texts present **and** chart geometry
(SVG/canvas, axis labels, series) inside the dashlet's `id` container.
