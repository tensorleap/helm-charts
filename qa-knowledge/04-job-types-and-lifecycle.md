# 04 · Job types & lifecycle (reference)

The taxonomy a QA engineer needs to reason about *what* is running, *how it's
named* in Kubernetes, and *how to tell it succeeded or failed*.

---

## Two layers of "type"

There is a node-server **JobSubType** (what the UI / `leap run list` shows) and an
engine **JobTypeEnum** (what the engine pod dispatches on). They are **not** the
same list — this is the #1 source of confusion.

### Engine `JobTypeEnum` (`engine/src_tensorleap/contract/common/enums.py`)
`TRAINING`, `IMPORT_MODEL`, `ANALYZE`, `WARMUP`, `EXPORT_MODEL`, `DATASET_PARSE`,
`ANALYZE_GRAPH`, `DRY_RUN_GRAPH`, `SLIM_LS`, `STREAMING_HANDLER`,
`STREAMING_SAMPLES_VIS`, `PUSH`, `SYNTHETIC`, `TEST_STUB_FUNCTION`,
`TEST_CUSTOM_LOSS`.
**There is no `EVALUATE` engine job type.**

### node-server JobSubType (UI / `leap run list`)
`Evaluate`, `Update Evaluate`, `Population Exploration`, `Labeling Recommendation`,
`Synthetic Data Generation`, `Dataset Balancing`, `Visualizers Calculation`,
`Sample Analysis`, `Graph Validate`, `Fetch Similar`, `Export/Copy/Import Project`,
`Code Parse`, `Import Model`, `Push`, `Generate Insights`, `Streaming Samples Vis`.
node-server also has local-only node-job types `EXPORT_PROJECT`, `IMPORT_PROJECT`.

### Mapping examples

| UI / subType | Engine job type | Main-pod worker | k8s Job name |
|---|---|---|---|
| Evaluate | `TRAINING` | `WorkerTrainer.evaluate()` | `evaluate-<jobId>` |
| Update Evaluate | `TRAINING` | `WorkerTrainer` (update artifact) | `update-evaluate-<jobId>` |
| Sample Analysis / Visualizers Calculation | `ANALYZE` | `WorkerAnalyzer` | `sample-analysis-<jobId>` / `visualizers-calculation-<jobId>` |
| Population Exploration, Fetch Similar, Generate Insights, Dataset Balancing, Synthetic Data Generation, Labeling Recommendation, Resplitting | `SLIM_LS` | `WorkerSlimLSOps` | `<subtype>-<jobId>` (single `SLIM` pod) |
| Push (incl. Code Parse + Import Model + Graph Validate phases) | `PUSH` | `WorkerPush` (`CodeParser` → `ImportModel` → `ValidateAssets`) | `push-<jobId>` |
| Export Model | `EXPORT_MODEL` | `WorkerExportModel` | `export-model-<jobId>` |
| Graph Validate | `DRY_RUN_GRAPH` | `WorkerGraphValidator` | `graph-validate-<jobId>` |
| Streaming Samples Vis | `STREAMING_SAMPLES_VIS` | `WorkerStreamingSamplesVis` | `streaming-samples-vis-<jobId>` |
| Export/Copy/Import Project | node job (`EXPORT_PROJECT`/`IMPORT_PROJECT`) | in-pod node-server runner | `<subtype>-<jobId>` |

> Full per-subType detail (triggers, data destinations, UI render, failure modes)
> is in [09-job-catalog.md](09-job-catalog.md). `Code Parse` / `Import Model` /
> `Graph Validate` run as **phases inside the PUSH job**, not standalone jobs.

**k8s Job naming rule** (`node-server/src/utils/k8s.ts` `calcK8sJobName`):
`formatString(subType || type)-<jobId>`, lower-cased, `_`/space → `-`.

---

## Which jobs spawn which runtime resources

| Engine job type | per-job Redis | generic-process (engine-generics) | streaming-handler |
|---|---|---|---|
| `TRAINING` (incl. Evaluate), `ANALYZE`, `SYNTHETIC` (engine type) | ✅ pod+svc | ✅ Deployment (Sample Analysis=1, Visualizers Calc=N) | ✅ Deployment (replicas=1, autoscaled ≤10) |
| `PUSH`, `EXPORT_MODEL`, `DRY_RUN_GRAPH`, `STREAMING_SAMPLES_VIS` | ✅ | ✅ single pod | ❌ |
| `SLIM_LS` (Population Exploration, Fetch Similar, Generate Insights, Dataset Balancing, Synthetic Data Generation, Labeling Recommendation, Resplitting) | ❌ | ❌ | ❌ — a **single** `SLIM` pod, no companions |
| `ANALYZE_GRAPH` | ❌ | ❌ | ❌ — engine main pod only |
| `WARMUP` | ❌ | ❌ | ❌ — placeholder GPU Job `engine-warmup-*` |
| node job (`EXPORT_PROJECT`/`IMPORT_PROJECT`) | ❌ | ❌ | ❌ — one node-server job pod |

So a QA engineer watching an **Evaluate** should expect, transiently:
`evaluate-<jobId>` (main pod), `redis-<jobId>` (pod+svc),
`generic-process-<jobId>` (Deployment), `streaming-handler-<jobId>` (Deployment).

---

## Job status lifecycle (Mongo `jobs.status`)

```
UNSTARTED ─► PENDING ─► INITIALIZING ─► STARTED ─► FINISHED
                                           │
                                           ├─► FAILED
                                           ├─► STOPPED      (user stop)
                                           └─► TERMINATED   (user terminate)
```

- `UNSTARTED` → set on insert (step 3 of Flow A).
- `PENDING` → after the k8s Job object is created (step 4).
- `INITIALIZING`/`STARTED` → mapped from pod phase + engine `STARTED` feedback.
- `FINISHED`/`FAILED` → from engine RabbitMQ feedback **or** orchestrator
  reconciliation.
- **Auto-FAILED**: a job `UNSTARTED` past a grace window, or one absent from the
  orchestrator's `active_jobs_report` after enough misses, is marked `FAILED` even
  with no engine feedback.

**Success signal (compound):** Mongo job `FINISHED` **and** `kubectl get job
<name>` `succeeded=1` **and** per-job runtime pods gone **and** (for Evaluate)
`GET <es_metrics_index>/_count > 0`.

**Failure signal:** Mongo job `FAILED` (with a `failure_reason`/message), or main
pod log `General Exception caught in run`, or a pod in
`OOMKilled`/`ImagePullBackOff`/`Evicted`/exit 137.

**Job logs:** `POST /api/v2/jobs/getJobLogs` gathers the last 2000 (8000 for
dataset-parse) lines per container plus `kubectl describe pod`, keyed by pod; logs
are also archived to the bucket on `FINISHED`/`FAILED`.

---

## k8s labels & selectors (memorize these)

| Workload | Selector |
|---|---|
| all pods of one job | `-l jobId=<jobId>` |
| engine-generics pods | `-l jobType=generic-process` |
| per-job redis | `-l jobType=redis` |
| streaming-handler pods | `-l jobType=streaming-handler` |
| all engine pods | `-l app=engine` |
| jobs of a project | `-l projectId=<projectId>` |
| slim jobs | `-l jobType=SLIM_LS` |
| engine runtime jobs | `-l engineJob=true` |
| node jobs | `-l app=tensorleap-node-job` |
| warmup jobs | `-l warmup-job=true,created-by=node-server` |
| node-server | `-l app=tensorleap-node-server` |
| web-ui | `-l app=tensorleap-web-ui` |
| orchestrator | `-l app=engine` (Deployment `engine-orchestrator`, container `orchestrator`) |
| mongodb / rabbitmq / minio | `-l app=mongodb` / `-l app=rabbitmq` / `-l app=minio,release=tensorleap` |
| elasticsearch pod | `-l elasticsearch.k8s.elastic.co/cluster-name=tl-elasticsearch` |
| zot registry | `-l app=tensorleap-registry` |

Deployments per job: `kubectl get deploy -n tensorleap -l jobId=<jobId>` →
`generic-process-<jobId>` and `streaming-handler-<jobId>`.

Engine Jobs carry `ttlSecondsAfterFinished=36000`, `backoffLimit=0`
(no retries); node jobs `activeDeadlineSeconds=28800`. The
`image-dependencies-builder` (pippin) init container builds custom dep images and
pushes to `tensorleap-registry:5000`.

---

## Resource sizing: auto-settings vs manual

Two layers, both observable:

1. **node-server** resolves requests/limits per pod role (main / generic / slim /
   redis) from engine-settings (`PRIORITIZE_AUTO_SETTINGS*` keys, the "Prioritize
   Auto-Settings" UI toggle) and `engine-job-config` machine types
   (`defaultCpuType`/`defaultGpuType`). It uploads
   `k8s_engine_generic_settings.json` + `k8s_engine_redis_settings.json` to the
   bucket per dispatch and renders the engine Job from `engine-job-template-cm`.
   Init jobs get near-unconstrained limits to avoid OOM.

2. **engine-generics** runs `GenericMemoryAutoSizer` at runtime: writes observed
   memory to Redis `memory_update:generic:<jobId>` and a bucket `pods_settings`
   JSON; the **orchestrator** reads the Redis key and patches the generic-process
   Deployment memory (engine-generics has no k8s RBAC, so Redis is the signal).

**Why this matters for QA:** sizing must be **infra-agnostic** (the customer's
machine is unknown). Under-provisioning surfaces as OOMKill; the relevant knobs
are the machine type, the "Prioritize Auto-Settings" toggle, `batch_memory_multiple`
in pods-settings, and `batchSize`. To inspect what was actually applied:
`kubectl get pod -l jobId=<jobId> -o jsonpath='{..resources}'` and read the bucket
`pods-settings/<versionId>/k8s_pods_settings.json`.

**GPU absence:** node-server strips the `nvidia.com/gpu` limit and injects
`NVIDIA_VISIBLE_DEVICES=void` + `CUDA_VISIBLE_DEVICES=''` when the machine type has
no GPU — verify these envs on the engine pod when debugging unexpected
CPU-only/GPU placement.

---

## Open questions to confirm on a live cluster

- Exact resources/affinity/SA of the **main engine Job** pod (created by
  node-server, not by the engine deployment-manager).
- `SLIM_LS` is confirmed single-pod (no redis/generic/streaming) — see [09-job-catalog.md](09-job-catalog.md). Still worth confirming the per-task memory ceiling on a live cluster (insights/balancing load latent spaces in one pod and can OOM).
- Whether `STREAMING_SAMPLES_VIS` differs from `ANALYZE visualizers_calculation`
  in what is streamed/stored.
- Whether the fork-based generic-process (`n_generic_instances > 1`) is enabled in
  the build under test (default 1 = no fork).
