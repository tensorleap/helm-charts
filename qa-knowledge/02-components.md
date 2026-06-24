# 02 · Components (deep reference)

What each component is, what it owns, and the concrete hooks a QA engineer uses to
observe and debug it. Pair with [03-data-flows.md](03-data-flows.md) for how they
interact during a run.

---

## web-ui (React 19 SPA)

**What it is.** A Vite/TypeScript single-page app (MUI + Tailwind + Emotion). Users
explore evaluated models: dashboards composed of **dashlets** (Population
Exploration scatter, Sample Analysis, Analytics charts), version control with
experiment history, model-population insights/issues, a network/architecture
editor, model tests, sample collections, and code-integration assets.

**Routing.** `react-router-dom` v7 `BrowserRouter` with a runtime `basename`
(injected via a `<base href>` by the nginx router image at serve time).
Top-level routes: `/project/:cid/*`, `/team-management` (admin), `/welcome`,
`/flags`, `/conflict-users`, `/authorization-request`, `/init-trial/*`,
`/login`, `/signup`; `/` → `/project`. **In-project navigation is query-param
driven, not nested paths:** `dashboard=<cid>` selects the active dashboard,
`panel=<DrawerTab>` selects the drawer (Tests/Insights/Issues/Collections),
`selected-version=<id>`, plus serialized `state`/`dashstate` digests.
→ Verify navigation by URL **query params**, not path segments.

**Auth.** `AuthProviderLoader` calls `getAuthProvider()` to pick **KeycloakProvider**
(OIDC, realm `tensorleap`, client `tensorleap-client`, check-sso + PKCE) or
**LocalProvider** (`demo@demo.ai`, no IdP) when the server reports
`authProvider === Local`. First-ever user → `register()` (when `getAuthStatus()`
returns `NoUsers`); otherwise `login()`.

**Data paths.**
- ES-derived data (statistics, charts/dashlets, confusion matrix, insights,
  versions, field values) → **always via node-server REST** `/api/v2`. The
  browser never talks to Elasticsearch directly.
- Bucket **reads** (visualization blobs/images) → **directly by URL**,
  same-origin via the `/session` ingress path (`clientStoragePrefixUrl`).
- Bucket **writes** (clustering JSON, uploads) → PUT to a **presigned URL**
  obtained from `getSignedUrl`/`getUploadSignedUrl`.
- Live updates → **socket.io** (`/socket.io`, event `serverMessage`).

**Runtime base URL** is derived from `window.location` + injected `<base href>`,
**not** an env var. The dev Vite server runs on `:3000` and proxies `/api`,
`/auth`, `/socket.io` → `:4000` and `/session` → `:4589`.

**QA hooks — see [06-ui-inspection.md](06-ui-inspection.md) for the full selector
catalog.** Quick facts:
- Stable selectors = element **`id`** attributes from `TOUR_SELECTORS_ENUM`
  (`src/tour/ToursConfig.tsx`), e.g. `analytics-dashlet`,
  `population-exploration-dashlet`, `version-control-pane`,
  `add-new-dashboard-button`.
- `data-testid` is **mostly dead** (only `editor-file-name-row`,
  `editor-rename-file-input` are applied). Do not rely on `src/test-ids.ts`.
- Version-control row actions are best targeted by **`aria-label`**
  (`Make this the active version`, `Expand experiment`).
- Dashlet grids are **MUI X DataGridPro** → target `.MuiDataGrid-row`,
  `[role="row"]`, `[data-field]`, `role="gridcell"`.
- Auth header is **`KBearer <jwt>`** (not `Bearer`); 401 → session-expired
  dialog; 409 → redirect to `/conflict-users` + reload.

---

## node-server (Express / tsoa API server)

**What it is.** The TypeScript/Node "brain". One image, two modes selected by env:
the long-lived **HTTP+WebSocket server** (default), or a one-shot **in-pod node-job
runner** when `JOB_TYPE`/`JOB_ID` are set (`isProcessJob()`).

**Owns:**
- The tsoa REST API under **`/api/v2`** (24 controllers, mostly POST).
- All MongoDB entities via a scoped-collection layer (scopes add unique compound
  indexes: `{cid}`, `{cid,teamId}`, `{cid,teamId,projectId}`).
- Elasticsearch **aggregation queries** that build chart/dashlet data on demand.
- MinIO bucket reads/writes (job request JSON, weights, settings JSON, artifacts,
  presigned URLs).
- **Creation of engine & node k8s Jobs** from ConfigMap templates, with resource
  sizing from "auto-settings" + machine types.
- The RabbitMQ **consumer** (single durable queue `feedback`) and the **socket.io**
  server.
- Keycloak bearer validation; local JWT; API-key (CLI) bearer.

**Health & probes.** `GET /api/v2/monitor/healthCheck` → 200 with
`allModules: [{name: 'rabbitmq'|'mongo'|'elastic', status, error}]`; **503** if any
module is down. This is the readiness + liveness probe target.

**REST surface (selected, all under `/api/v2`):**
- `/auth` — login, localAuth, whoAmI, getAuthStatus, getAuthProvider, keygen,
  getApiKeyByCode, activate, startTrial, refreshLocalAuth, resolveConcurrentUsersConflict, logout
- `/projects` — addProject, getProjects, deleteProject, loadModel, importProject,
  exportProject, uploadProject, downloadProject, setEngineGenericWorkers
- `/versions` — push, pushOverride, initExperiment, loadVersion, setActiveVersion,
  get*SlimVersions, getCodeSnapshotUploadUrl, tagModel, deleteVersion, getVersionsEpochs
- `/jobs` — getSlimJobs, getTeamJobs, getJobLogs, stopJob, terminateJob, terminateAllJobs, warmup
- `/evaluate` — evaluate, continueEvaluate, resetEvaluate, updateEvaluateArtifact, continueUpdateEvaluate
- `/sessionmetrics` — getXYChart, getHeatmapChart, getTableChart, getConfusionMatrixTable,
  getRoc, getPrCurve, getF1Score, getSampleEnrichment, getFieldsValues *(ES-backed dashlet data)*
- `/dashboards` — addDashboard, getDashboard, getProjectDashboards, updateDashboard,
  deleteDashboard, getDashletFields, calcPopulationExplorationDigest
- `/visualizations` — populationExploration, sampleAnalysis, fetchSimilar,
  createSamplesVisualizations, getScatterSampleVisualizations, getVisualization
- `/insights`, `/insightsSettings`, `/datasetcuration`, `/sample-collection`,
  `/sessions-tests`, `/issues`, `/notifications`, `/teams`, `/users`, `/settings`,
  `/secret-manager`, `/metadata`, `/projectstate`, `/monitor/healthCheck` (GET)

**Mongo collections** (`db: tensorleap`): `jobs`, `versions`, `projects`, `users`,
`teams`, `notifications`, `dashboards`, `visualizations`, `insights`,
`insightsSettings`, `insightContainerLabels`, `models`, `codesnapshots`,
`exportedmodels`, `samplecollection`, `secretmanager`, `generatedLabels`,
`syntheticdata`, `datasetbalancing`, `issues`, `tests`, `externalepochdata`,
`projectstate`, `system_settings`, `system_metadata`, `db_metadata`.

**Key relations.** team → project → version (chain via
`versions.experimentId`/`parentVersionId`); `version.codeSnapshotId → codesnapshots`;
`job.{teamId,projectId,versionId,experimentId,modelId,processId}`;
`version.resources → {inference_artifact_id, vis_artifact_id, es_model_id,
es_metrics_index, es_inspection_index}` (links a version to its bucket artifacts +
ES indices). **`dashboards.items` stores only dashlet config**, not chart data.

**Elasticsearch index resolution (critical for "empty dashlet" debugging).**
Indices are **not** named by node-server — they come from the version doc's
`resources.es_metrics_index` / `es_inspection_index` (engine-assigned, prefixed by
projectId). To debug empty dashlets: `db.versions.find({},{resources:1})` →
`GET <ES>/_cat/indices`. Queries filter by `model._id.keyword = es_model_id`.
With no version selected, the scope now resolves to **empty** (it used to fall
back to a project-wide wildcard that leaked data across evaluations) — so an empty
state with no version is *intended*, not a bug.

**Auto-settings sizing.** Main/generic/slim/redis pod resources resolve from
engine-settings (`PRIORITIZE_AUTO_SETTINGS*`) vs a bucket `pods-settings` JSON at
`organizations/<team>/projects/<proj>/pods-settings/<versionId>/k8s_pods_settings.json`.
Per-dispatch, node-server uploads derived `k8s_engine_generic_settings.json` and
`k8s_engine_redis_settings.json`. Init jobs (PUSH/IMPORT_MODEL/DATASET_PARSE) get
near-unconstrained limits to avoid OOM. **GPU:** if a machine type lacks
`nvidia.com/gpu` or `num_gpus<=0`, node-server deletes the GPU limit and injects
`NVIDIA_VISIBLE_DEVICES=void` + `CUDA_VISIBLE_DEVICES=''` into the engine container.

**Key env vars** (`tensorleap-node-server-env-configmap` + `node-server-secrets`):
`PORT(4000)`, `MONGO_URI`, `RABBIT_URI`, `SUBSCRIBER_TOPIC=feedback`,
`ELASTIC_HOST`, `BUCKET_NAME`, `STORAGE_ENDPOINT`, `GATEWAY_URL`,
`KEYCLOAK_CLUSTER_URL/REALM/RESOURCE`, `ENABLE_KEYCLOAK_AUTH`, `DISABLE_AUTH`,
`NAMESPACE`, `TARGET_NAMESPACE`, `K8S_ENGINE_JOB_CONFIG(engine-job-config)`,
`MAX_ACTIVE_USERS`, `INSTALLED_SERVER_VERSION`. The config (convict) is validated
**strict at boot** — a bad/unknown env crashes startup.

**Log signatures** (pino JSON; set `PRETTY_LOGS=true`/`LOG_LEVEL=debug` locally):
`Server started {port:4000}`, `Connected to mongodb`, `Connected to RabbitMQ
server` + `activate consumer`, `WS client connected`, `WS Sending message to user`.

**`DISABLE_AUTH=true`** provisions `admin@tensorleap.local` and bypasses Keycloak —
intended for local/CI only; confirm it is **off** in any customer env you test.

---

## Engine family (engine, engine-generics, streaming-handler, orchestrator)

A Python distributed compute system. All in namespace `tensorleap`.

### Main engine pod (`python -m src_tensorleap.engine.run`)
- Reads its job payload JSON from the bucket URL in env **`JOB_PAYLOAD`**.
- Selects a worker by `JobTypeEnum`. **Evaluate** → `JobType=TRAINING` →
  `WorkerTrainer.evaluate()`.
- For TRAINING/ANALYZE/SYNTHETIC: via `Manager.run`, creates **per-job Redis**
  (Pod+Service), the **streaming-handler** Deployment, and the **generic-process**
  (engine-generics) Deployment. For PUSH/EXPORT_MODEL/DRY_RUN_GRAPH/STREAMING_SAMPLES_VIS:
  per-job Redis + a single generic-process pod.
- Builds/loads the TF/ONNX model and runs **batched inference** on samples popped
  from per-job Redis. Pushes result batches to the streaming queue.
- Runs a **`StreamingHandlerScaler` daemon thread** (this lives in the main pod,
  not the orchestrator) that scales the streaming-handler Deployment from
  push/pull counters (`ceil(pushed/pulled_per_instance)+3`, capped at 10).
- Publishes status (STARTED/FINISHED/FAILED) and messages to node-server over
  **RabbitMQ** (`FEEDBACK_TOPIC`); subscribes to a stop command on
  `SUBSCRIBER_TOPIC`.

### engine-generics (`generic-process` Deployment, `python -m src_tensorleap.engine.generic_processor`)
- Runs the **customer integration code** via code-loader: `preprocess` +
  `get_sample()` per sample index → pushes `DatasetSample` pickles to the dataset
  ready queue.
- In **dedicated-metrics mode**: pops metrics batches, runs `reporter.report_metrics`
  (customer metric functions) + loss, pushes results to the streaming queue.
- Self-sizes memory via `GenericMemoryAutoSizer` → writes to Redis key
  `memory_update:generic:<job_id>` and a bucket `pods_settings` JSON.
- Optional **fork model**: warms `pre_process` once per pod, forks to C children
  when `n_generic_instances>1` (default 1 = no fork). Init container copies code
  to a shared volume.

### streaming-handler (`python -m src_tensorleap.engine.streaming_handler`, ENGINE image)
- Polls all `streaming_*_<job_id>_queue` Redis keys in bulk (up to 500 docs or
  every 20s).
- Writes **metrics + metadata → Elasticsearch** (`es_metrics_index`) **always**.
- Writes **latent-space vectors → bucket** (via `LatentSpaceDBManager`) **only for
  "evaluate" queues** (`streaming_evaluate_*`); training queues get ES only.
- Sanitizes `NaN → None` (ES rejects `NaN` and would drop the whole doc).
- Memory request 1Gi / limit 2Gi; up to 10 replicas/job; grace period 60s.

### orchestrator (`engine-orchestrator` Deployment, `python -m src_tensorleap.engine.engine_scheduler`)
- The **only static engine workload**. Uses the `deployment-manager`
  ServiceAccount RBAC.
- Detects failed pods/jobs every interval (OOMKilled, ImagePullBackOff, Evicted,
  exit 137) and reports `failure_jobs_report` / `active_jobs_report` to node-server
  over RabbitMQ — so a job whose pod died before reporting is still marked FAILED.
- Scales the generic-process Deployment to `ceil(desired_workers/children_per_pod)`.
- Reads `memory_update:generic:<job_id>` from Redis and patches the Deployment
  memory (engine-generics lacks k8s RBAC, so it signals via Redis).
- **Does NOT manage per-job Redis queue rate** (that is the trainer's
  backpressure + the main-pod streaming scaler).

### Per-job Redis & its 5 logical queues

One Redis pod `redis-<jobId>` (`redis:8.6-alpine`, port 6379,
`--maxmemory-policy noeviction`). DNS `redis-<jobId>.tensorleap.svc.cluster.local:6379`.

| Queue / key pattern | Direction | Carries |
|---|---|---|
| `dataset_<state>_to_generate_<job_id>` | engine → generic | sample IDs to fetch |
| `dataset_<state>_<job_id>_ready` | generic → engine | `DatasetSample` pickles |
| `metrics_<job_id>` | engine → generic | `RedisMetricsQueueElement` (dedicated-metrics mode) |
| `vis_calc_<job_id>` | engine → generic | visualizer batches (`visualizers_calculation`) |
| `streaming_evaluate_<job_id>_queue` / `streaming_training_<job_id>_queue` | generic/engine → streaming-handler | output docs (+ latent space for evaluate) |

Scaling/timing keys: `generic_children:<job_id>` (TTL 1800s),
`memory_update:generic:<job_id>`, `generic_process_ratio_<job_id>`,
`push_streaming_counter`, `pull_streaming_counter`, `eval_calc_time_<job_id>`.

### Engine log signatures (for pod-log assertions)

| Signal | Log line |
|---|---|
| job start | `starting engine worker...` then `Running job` (`job_type=…`) |
| per-job redis ready | `Per-job Redis ready` (with `redis_host`, `redis_port`) |
| generic deployment created | `deployment created` |
| job success | `Process done` then RabbitMQ `FINISHED` |
| job failure (unhandled) | `General Exception caught in run` then `FAILED` |
| streaming start | `Starting streaming handler service...` |
| streaming indexing | `indexing docs to elasticsearch` (+ `bulk`, `index`) |
| streaming ES failure | `failed to index docs to elasticsearch` (+ `err_vec`) |
| orchestrator scaling | `scaled generic-process deployment` |
| streaming scaler tick | `StreamingHandlerScaler tick` (+ pushed/pulled/desired) |
| OOM | orchestrator `Monitoring failed jobs` listing `OOM_KILLED` |
| backpressure | `max number of streaming objects in queue reached…` |
| pippin prebuild failure | sentinel file `/shared/logs/error_<GENERIC_CALCULATOR_IMAGE_TAG>.txt` |

**Key env vars to verify on pods:** engine — `JOB_PAYLOAD`, `ENGINE_IMAGE`,
`GENERIC_CALCULATOR_IMAGE(_TAG/_TAG_BASE)`, `REDIS_IMAGE`, `FEEDBACK_TOPIC`,
`SUBSCRIBER_TOPIC`, `ELASTIC_HOST`; generic — `JOB_PAYLOAD`, `REDIS_HOST`,
`REDIS_PORT`, `GENERIC_CALCULATOR_IMAGE_TAG`; streaming-handler — `REDIS_HOST`,
`REDIS_PORT`, `JOB_ID`, `HMAC_ACCESS_KEY_ID/SECRET`.

---

## Deployment / installer (`helm-charts`, `leap server` Go CLI)

**Charts.** `charts/tensorleap` (umbrella: engine, node-server, web-ui subcharts +
elasticsearch CR, minio, rabbitmq, keycloak, ingress-nginx, datadog) and
`charts/tensorleap-infra` (eck-operator → ES CRD, Zot, nvidia plugin). External
deps pinned exactly; bundled tarballs updated via `helm dependency build`.

**`leap server` subcommands** (`cmd/server/`): `install`
`[--local --yes --data-dir --tag --gpu --port]`, `upgrade` (always latest; minor
bump → reinstall), `reinstall`, `uninstall` `[--purge|--cleanup|--clear-data]`,
`run`/`up`/`start`, `stop`/`down`, `check` *(stub — prints only "Check command")*,
`pack`/`pack-installation` (airgap), `create-manifest`, **`tools`** (embedded k3d +
kubectl pre-wired to context `k3d-tensorleap`).

**Single-gateway rule (for code-aware tests/repro):** all Helm ops go through
`pkg/helm`; k3d through `pkg/k3d`; docker through `pkg/docker`. Names/limits are
constants at the top of each package (`CLUSTER_NAME`, `KUBE_NAMESPACE`,
`DefaultHttpPort`).

**Host data paths** (`global.create_local_volumes=true`):
`/var/lib/tensorleap/standalone/{storage,storage/elasticsearch,storage/keycloak,registry,logs,manifests}`.
Base dir overridable with `leap server install --data-dir`.

---

## leap CLI + code-integration contract

**`leap` binary** = `leap-cli` command tree + the `leap server …` subtree imported
from `helm-charts`. Config at `~/.config/tensorleap/config.yaml`
(`current_env`, `envs.<name>.api_url`, `envs.<name>.api_key`; override with
`--config` or `TL_CLI_CONFIG_FILE`).

**Commands relevant to QA:**
- `leap auth login [url] -k <apiKey>` (or `-u/-p`), `auth logout/select/whoami/licence`.
- `leap push` — the central command: reads `leap.yaml`, bundles code into a tar.gz,
  parses it + imports/validates the model; flags `-n/--name`,
  `--type [JSON_TF2/ONNX/PB_TF2/H5_TF2]`, `--branch`, `--secretId`,
  `-m/--model-path`, `-e/--eval`, `-b/--batch <n|latest>` (needs `--eval`),
  **`-o/--overwrite <id|name>`** (NOT `--override`), `-u/--update {metadata|metric|metric_config|viz}`
  (implies `--eval`), `--no-wait`, `--novis`, `--yes`.
- `leap projects {create,list,select,info,delete,copy,export,import,publish,push,set-secret}`.
- `leap run {list,logs <runId>}` — CLI view of engine jobs (filter by JobSubType / status).
- `leap server …` — cluster lifecycle + embedded `kubectl`/`k3d`.

> There is **no** `leap dataset` command and **no** `leap_mapping.yaml`. The
> dataset/integration is the decorator-based `leap_integration.py` binder.

**`leap.yaml`** (workspace, at project root): `projectId`, `secretId`, `branch`,
`entryFile` (default `leap_integration.py`), `include[]`, `exclude[]`,
`pythonVersion`. Missing → push errors `cannot detect leap.yaml file`.

**Code-integration contract** (`code-loader`, `@tensorleap_*` decorators in
`leap_integration.py`): `tensorleap_load_model`, `tensorleap_preprocess`,
`tensorleap_input_encoder(name, channel_dim)`, `tensorleap_gt_encoder(name)`,
`tensorleap_custom_metric(name, direction, ...)`,
`tensorleap_custom_visualizer(name, visualizer_type: LeapDataType)`,
`tensorleap_custom_loss`, `tensorleap_metadata`, `tensorleap_integration_test`.
Encoder contract: `(idx, preprocess: PreprocessResponse) -> np.ndarray` of dtype
**float32**; `channel_dim` must be `-1` or positive. `tensorleap_integration_test()`
runs the whole binder locally — the cheapest pre-push validation a QA engineer can run.

Enums: `LeapDataType {Image,Text,Graph,HorizontalBar,ImageMask,TextMask,
ImageWithBBox,ImageWithHeatmap,Video}`; `MetricDirection {Upward,Downward}`;
`DataStateType {training,validation,test,unlabeled,additional}`;
`DatasetMetadataType {float,string,int,boolean}`.
