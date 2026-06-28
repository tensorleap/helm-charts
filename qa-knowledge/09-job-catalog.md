# 09 · Job catalog (every non-Evaluate job)

One entry per job a QA engineer will see, beyond the Evaluate flow already covered
in [03-data-flows.md](03-data-flows.md). For each: trigger → engine job type &
worker → what it spawns → where data lands → the observables that prove it ran →
how it renders in the UI.

> Read [04-job-types-and-lifecycle.md](04-job-types-and-lifecycle.md) first for the
> status lifecycle, k8s naming rule (`formatString(subType||type)-<jobId>`), and
> label selectors. All jobs are created via the **k8s API**, dispatched in the
> engine pod by `JOB_PAYLOAD`, and report back over RabbitMQ.

---

## ⚠️ The single most useful QA heuristic: what a job spawns

The engine **job type** (not the UI subType) decides what pods appear. Watch
`kubectl -n tensorleap get pod,deploy,svc -l jobId=<jobId>` and you can tell at a
glance what *should* be there:

| Engine job type | Spawns (besides the main/worker pod) | Job subTypes that use it |
|---|---|---|
| `TRAINING` | per-job **redis** + **generic-process (N)** + **streaming-handler** | Evaluate, Update Evaluate |
| `ANALYZE` | per-job **redis** + **generic-process (1 or N)** + **streaming-handler** | Sample Analysis (1), Visualizers Calculation (N) |
| `SYNTHETIC` (engine type) | per-job **redis** + **generic-process (1)** + **streaming-handler** | (engine sample-generation; *not* the curation "Synthetic Data Generation") |
| `PUSH` | per-job **redis** + **generic-process (1)**, **no** streaming-handler | Push (= Code Parse + Import Model + Graph Validate phases) |
| `EXPORT_MODEL` | per-job **redis** + **generic-process (1)**, no streaming-handler | Export Model |
| `DRY_RUN_GRAPH` | per-job **redis** + **generic-process (1)**, no streaming-handler | Graph Validate |
| `STREAMING_SAMPLES_VIS` | per-job **redis** + **generic-process (1)**, no streaming-handler | Streaming Samples Vis |
| `SLIM_LS` | **NOTHING** — one single `SLIM` pod | **Population Exploration, Fetch Similar, Generate Insights, Dataset Balancing, Synthetic Data Generation, Labeling Recommendation, Resplitting** |
| `ANALYZE_GRAPH` | **NOTHING** — engine main pod only | (graph static analysis, a phase of import) |
| `WARMUP` | a sleep-placeholder GPU **Job** (`engine-warmup-*`) | Warmup |
| node job (`EXPORT_PROJECT`/`IMPORT_PROJECT`) | one **node** Job pod (node-server image), no engine pods | Export/Copy/Import Project |

**SLIM_LS is the big one to internalize:** seven different SLIM_LS request types run as a
*single* `SLIM_LS` pod. If you expect `redis-<jobId>`/`generic-process`/`streaming-handler`
for a Population Exploration or Insights job, you'll think it's broken — there
won't be any. The defining observable for SLIM_LS is "exactly one engine pod
labeled `jobType=SLIM_LS`, no companions" (and `hasWorker=false`).

---

## Master matrix

| UI subType | Engine type | Worker | k8s Job name | Data destinations | UI render (success selector) |
|---|---|---|---|---|---|
| Sample Analysis | `ANALYZE` | `WorkerAnalyzer._sample_analysis` | `sample-analysis-<jobId>` | mongo `visualizations`; bucket `vis/<vis_artifact_id>/sample_analysis/payloads/<guid>` | `#sample-analysis-dashlet-loaded-content` |
| Visualizers Calculation | `ANALYZE` | `WorkerAnalyzer._visualizers_calculation` | `visualizers-calculation-<jobId>` | bucket `vis/<vis_artifact_id>/sample_visualizers/*` + `visualizer_names.json` | `#population-exploration-right-panel-visualizations` tiles |
| Population Exploration | `SLIM_LS` | `WorkerSlimLSOps.population_exploration` | `population-exploration-<jobId>` | bucket `vis/<vis_artifact_id>/population_exploration/digest/<digest>/scatter.json` | `#population-exploration-circles` (dots) |
| Fetch Similar | `SLIM_LS` | `WorkerSlimLSOps.create_cluster_filter` | `fetch-similar-<jobId>` | bucket `vis/<vis_artifact_id>/fetch_similar/<digest>/cluster.json` | filter chip + highlight in `#population-exploration-circles` |
| Generate Insights | `SLIM_LS` | `WorkerSlimLSOps.insights_calculation` | `generate-insights-<jobId>` | mongo `insights` + `versions.resources.csv_blob_path`; reads ES `es_metrics_index` | `#insight-card` under `#insights-list` |
| Dataset Balancing | `SLIM_LS` | `WorkerSlimLSOps.dataset_balancing` | `dataset-balancing-<jobId>` | mongo `datasetbalancing`; bucket `digest_<d>/dataset_balancing/*` | row in DS Curation → PRUNING tab grid |
| Synthetic Data Generation | `SLIM_LS` | `WorkerSlimLSOps.synthetic_calibration` | `synthetic-data-generation-<jobId>` | mongo `syntheticdata`; bucket `digest_<d>/synthetic-calibration/{next,best}_trials.csv` | row in DS Curation → SYNTHETIC tab grid |
| Labeling Recommendation | `SLIM_LS` | `WorkerSlimLSOps.labeling_recommendation` | `labeling-recommendation-<jobId>` | mongo `generatedLabels`; bucket `digest_<d>/labeling/*` | row in DS Curation → UNLABELED tab grid |
| Resplitting *(engine-side; node trigger not yet on master)* | `SLIM_LS` | `WorkerSlimLSOps.resplitting` | `resplitting-<jobId>` | bucket `digest_<d>/resplitting/{<jobUid>.csv, resplitting_cluster_filter.json}` | DS Curation (data re-split) — verify UI |
| Push | `PUSH` | `WorkerPush` (CodeParser+ImportModel+ValidateAssets) | `push-<jobId>` | mongo `codesnapshots`,`versions`,`models`; bucket model artifacts | Version Control state PUSHING→PUSHED |
| Export Model | `EXPORT_MODEL` | `WorkerExportModel` | `export-model-<jobId>` | mongo `exportedmodels`; bucket exported file | exported-models list per version |
| Graph Validate | `DRY_RUN_GRAPH` | `WorkerGraphValidator` | `graph-validate-<jobId>` | mongo `versions.graphValidationData` | network-editor markers / push state |
| (Graph analyze) | `ANALYZE_GRAPH` | `WorkerGraphAnalyzer` | `analyze-graph-<jobId>` | none persisted (pushed to UI) | network-editor node shapes/types |
| Streaming Samples Vis | `STREAMING_SAMPLES_VIS` | `WorkerStreamingSamplesVis` + `StreamingVisRunner` | `streaming-samples-vis-<jobId>` | none (in-memory push to UI) | live visualizer preview (source `streaming-samples`) |
| Warmup | k8s placeholder Job | sleep pod (engine `WARMUP` branch is a no-op) | `engine-warmup-<teamId>-<machineTypeId>` | none (reserves GPU capacity) | no UI surface |
| Export / Copy Project | node job `EXPORT_PROJECT` | node-server in-pod runner | `export-project-<jobId>` / `copy-project-<jobId>` | bucket tar.gz (+ remote PUT for copy) | DownloadExportProjectDialog (hidden from Runs list) |
| Import Project | node job `IMPORT_PROJECT` | node-server in-pod runner | `import-project-<jobId>` | new mongo project + restored ES indices + bucket files | hub/projects table + Runs and Processes table |
| Evaluate *(see [03](03-data-flows.md))* | `TRAINING` | `WorkerTrainer` | `evaluate-<jobId>` | ES `es_metrics_index` + bucket latent space | dashlets render with data |

---

## ANALYZE family

### Sample Analysis
- **Trigger:** add a Sample Analysis dashlet + select a sample → `POST /visualizations/sampleAnalysis` `{versionId, projectId, sampleIdentity, algo}`.
- **Spawns:** redis + generic-process(1) + streaming-handler + scaler thread.
- **Observables:** `kubectl get pods -l jobId=<jobId>` → MAIN + redis + 1 generic-process + streaming-handler; new mongo `visualizations` doc `type='sample_analysis'`; bucket payload under `vis/<vis_artifact_id>/sample_analysis/payloads/<guid>`; job FINISHED.
- **Success:** job FINISHED + `visualizations` doc + viz blob; UI `#sample-analysis-dashlet-loaded-content` shows heatmaps.
- **Failure:** visualizer crash → a `TextData "Visualizer has crashed"` item but **job still FINISHED**; missing weights → degraded activation maps; pod scheduling failure → FAILED.
- **UI empty:** no sample selected → `Select a sample to view its assets` (`#sample-analysis-no-sample-selected`).

### Visualizers Calculation
- **Trigger:** Population Exploration dashlet → **Visualize** button (`#population-exploration-visualize-button`) after selecting samples → `POST /visualizations/createSamplesVisualizations`; refresh bumps `sample_visualizers_revision`.
- **Spawns:** redis + generic-process **(N replicas** = internal "visualizers" process count → visible fan-out**)** + streaming-handler + scaler.
- **Observables:** k8s `visualizers-calculation-<jobId>`; multiple generic-process pods for the jobId; redis work queue `vis_calc_<jobId>` drains to 0; per-sample blobs + `visualizer_names.json` in bucket; `wait_for_all_vis_processes` completes ≤300s.
- **Dedup:** concurrent duplicate → HTTP **208 AlreadyReported**.
- **Failure:** queue not drained in 300s → timeout; under-scheduled replicas (small cluster) → stuck queue; individual visualizer crash → `has_error` item, job still completes.

---

## SLIM_LS family (single pod — no redis/generic/streaming)

> Common to all: `kubectl get pods -l jobId=<jobId>` shows **one** engine pod
> `jobType=SLIM_LS`, **no** companions; `JOB_NOTIFICATION_CONFIG.SLIM_LS=false`
> (results surface via each feature's own message handler, not a generic job
> notification); `post_running` sets FINISHED/FAILED.

### Population Exploration  *(this is a `SLIM_LS` job, NOT `ANALYZE`)*
- **Trigger:** auto-runs when the dashlet mounts. `POST /visualizations/populationExploration` then polls `POST /visualizations/getPopulationExplorationStatus` every ~3s. **Blocked only while a prerequisite Evaluate/Update-Evaluate job's `insights_analysis` step is still pending** (UserError before the job is created); once that step's event reaches `FINISHED` or `SKIPPED`, PE may run even if the evaluate job is still in progress (later steps like `visualize_samples` continue).
- **The digest** is minted server-side in one place (`calcPopulationExplorationDigest`) from the population params + `insightsRevision` + seed count + teamId; the client-passed digest is ignored (note: `sample_visualizers_revision` is **no longer** part of the digest). A new Evaluate bumps `insightsRevision` + the seed count → new digest → new scatter path → UI re-runs. (Status `NOT_FOUND` after a new eval is *expected*, not a bug.)
- **Success:** `scatter.json` present at `vis/<vis_artifact_id>/population_exploration/digest/<digest>/scatter.json` → status FINISHED; UI renders dots in `#population-exploration-circles` (inside `#population-exploration-dashlet`).
- **UI states:** processing → `#population-exploration-processing` ("processing…"); empty → "No samples"; error → "Population Exploration creation failed" + Retry, or "Evaluate failed" when the prerequisite eval failed.
- **Failure:** `NoSamplesInLS` (no LS samples match filters); stale LS indicators → "run evaluate with population exploration again".

### Fetch Similar
- **Trigger:** multi-select samples → **Fetch Similar** action (MUI button `label="Fetch Similar"`) → `POST /visualizations/fetchSimilar` then `getFetchSimilarStatus`. **Re-runs only if the prior status is FAILED.** Scheduled CPU (`preferCpu=true`).
- **Result is a cluster FILTER**, not a dashlet: `cluster.json` at `vis/<vis_artifact_id>/fetch_similar/<digest>/cluster.json`; the UI applies it as a fetch-similar filter and highlights matching samples in the scatter.
- **Failure:** `FetchSimilarNoCandidatesError` ("Fetch similar isn't possible on the entire filtered population") → FAILED; a stale STARTED job blocks re-trigger.

### Generate Insights
- **Trigger:** Insights panel (`#insights-panel-button`) → "Regenerate insights" confirm; also auto on dashboard load + Insights Settings dialog. `POST /insights/generateInsights` per version. **Precondition:** the version must have an ES metrics index, else UserError "Version has no ES metric index" and no job.
- **Outputs:** per-insight docs in mongo `insights` (status `InReview`, stamped with `insightsCounter`); `versions.resources.csv_blob_path` + `vis_resources.insights_revision`; bumps `populationExplorationDigestSeedCount` once (→ one pop-exp re-run). Engine uploads the insights CSV to the bucket; reads `es_metrics_index`.
- **Success:** job FINISHED + `insights` docs at the current `insightsCounter` + `#insight-card` cards under `#insights-list` + `csv_blob_path` populated.
- **Failure / gotchas:** SLIM pod OOM (insights load latent spaces in one pod, no scaling) → FAILED; **empty insights list → FINISHED with no cards** (often mistaken for failure); revision mismatch → UI shows wrong-revision/empty list.

### Dataset Balancing  ·  Synthetic Data Generation  ·  Labeling Recommendation (DS Curation)
All three are launched from the **DS Curation** toolbar button → `DatasetCurationDialog`
(title "DATASET CURATION LIST") tabs **PRUNING / SYNTHETIC / UNLABELED**, via
`EvaluationAwareActionButton` (warns if the eval is incomplete).

| | Dataset Balancing | Synthetic Data Generation | Labeling Recommendation |
|---|---|---|---|
| endpoint | `/datasetcuration/generateDatasetBalancing` | `/datasetcuration/generateSyntheticData` | `/datasetcuration/generateLabels` |
| `slim_request_type` | `dataset_balancing` (algo PRUNING) | `synthetic_calibration` | `labeling_recommendation` (algo CORESET) |
| mongo entity | `datasetbalancing` | `syntheticdata` | `generatedLabels` |
| bucket output | `digest_<d>/dataset_balancing/{recommendations.csv[.tar.gz], cluster_filter.json}` | `digest_<d>/synthetic-calibration/{next_trials.csv, best_trials.csv}` | `digest_<d>/labeling/{recommendations.csv, cluster_filter.json}` |
| UI tab | PRUNING | SYNTHETIC | UNLABELED |
| validation block | no model / no dashboard / no pop-exp dashlet | "Target is empty" / "No sources added" | "No model selected" |

- **Success (all):** job FINISHED + entity row present + the output file(s) exist in
  the bucket + a new row in the tab's DataGridPro. **Note:** a job can be FINISHED
  while the output file is absent (e.g. optimizer produced no trials) → the UI row
  shows no download. Don't treat FINISHED alone as success — check the bucket file.
- **⚠️ Synthetic confusion:** "Synthetic Data Generation" here is the **`SLIM_LS`
  calibration/optimizer** job (single pod). It is *not* the separate engine
  `JobTypeEnum.SYNTHETIC` sample-generation worker (which *does* spawn
  redis+generic+streaming). If you see redis/streaming pods, you're looking at the
  wrong thing.

### Resplitting  *(engine-side as of engine master; node trigger not yet on node-server master)*
A 7th `SLIM_LS` request type added engine-side: `slim_request_type=resplitting`, worker
`WorkerSlimLSOps.resplitting` → `Resplitting.run_resplitting` (`trainer/ds_curation/resplitting.py`).
It re-splits the dataset across train/val/test: groups samples by `keep_together_metadata`,
stratifies across `split_across_metadata`, KMeans-clusters feature vectors, and assigns
clusters to splits by `train/val/test_ratio` (request `SlimResplittingRequest`).
- **Spawns:** a single `SLIM` pod (no redis/generic/streaming), like the other SLIM_LS jobs; k8s job `resplitting-<jobId>`.
- **Bucket:** `digest_<d>/resplitting/{<jobUid>.csv, resplitting_cluster_filter.json}` (`get_resplitting_csv_path` / `get_resplitting_cluster_filter_path`).
- **⚠️ Gap:** node-server master has **no** `resplit` reference yet — the REST trigger, subType label, and mongo entity are not shipped on node-server master. Verify the node-server side + the DS Curation UI entry once wired (logged in `maintenance/GAPS.md`).

---

## PUSH and its phases

### Push  (`PUSH` / `WorkerPush`)
- **Trigger:** primarily the **`leap push`** CLI (also code-integration panel). `POST /versions/push` (new version + model upload) or `POST /versions/pushOverride` (re-push to an existing version, reuses model). Web-ui renders **status only** — there is no primary push button in the SPA.
- **Spawns:** redis + generic-process(1, priorityClass `low-medium-priority`), **no** streaming-handler.
- **Phases inside the one job:** Code Parse (`CodeParser.parse()`) → Import Model (`ImportModel.import_and_validate()`) → Graph Validate (`ValidateAssets`). Job events: `dataset_parse → load_data → parsing_model → convert_to_tensorleap_format → (build/run/testing) `.
- **Outputs:** mongo `codesnapshots` (`testStatus` = `testSuccess`/`testFail`, parseResult/setup/modelSetup), `versions` (`data`=ModelGraph, `modelHash`, `modelId`), `models`; bucket uploaded model + weights `.h5` + `graph_assets-<uuid>.json` + engine file contract.
- **Success:** job FINISHED; `codesnapshot.testStatus='testSuccess'`; version has `data`+`modelHash`; `graph_validator` published with no error; Version Control state → **PUSHED** ("Pushed", with a Run-evaluate action).
- **Failure (CLI surfaces a ValidateAsset report):** Code Parse `is_valid=false` → "Dataset parse failed" (import skipped); unsupported layer → "Import model error, unsupported layer"/"ONNX is not supported on your machine"; graph validation errors → push fails (`PUSH_FAILED`); pod OOM/crash → `codesnapshot.testStatus='testFail'`, job FAILED.

### Code Parse (`DATASET_PARSE`) and Import Model (`IMPORT_MODEL`)
These remain `JobTypeEnum` values + notification configs, but in the unified flow
they run as **phases inside the PUSH pod** (no standalone job/manager branch). Their
observables are the engine messages `source='dataset_parse'` and `source='import_model'`
consumed by node-server, and the mongo writes listed above. The Code Parse result
renders in the **code-integration panel** (`#code-integration-panel`); the imported
model graph renders in the version's network/graph view.

---

## Graph jobs (two distinct things)

> The recon found the two graph jobs slightly conflated across sources. Treat this
> section as "verify on a live cluster" if precise behavior matters for a test.

### Graph Validate — `DRY_RUN_GRAPH` / `WorkerGraphValidator`
- Runs the graph on `/cpu:0`; **spawns** redis + generic-process(1) (`hasWorker=true`).
- Emitted as the `graph_validator` phase during Push/import; result persisted to
  `versions.graphValidationData` (`updateGraphValidation`). Node marks the job FAILED
  if any node has an error.
- **UI:** network-editor per-node validation markers (`web-ui/src/network-editor/graph-calculation/GraphValidate.ts`) + a general-error banner; in the push flow, validation errors fail the push and a notification "Graph validation found N issue(s): …" appears.
- **Failure:** `MissingInputTensorException`, "Dataset Error: …", or any visualizer/loss/metric node error → `graph_has_error` → FAILED.

### Graph analyze — `ANALYZE_GRAPH` / `WorkerGraphAnalyzer`
- **Spawns nothing** (engine main pod only, `hasWorker=false`). Static analysis (per-node output shapes/dtypes); no sample inference.
- **Not persisted in node-server** — routed to an `unsupportedGraphAnalyzerHandler` and pushed straight to the web-ui (network-editor node calculated-data annotations). No notification (`ANALYZE_GRAPH` notify=false).

---

## Streaming Samples Vis (`STREAMING_SAMPLES_VIS`)
- **Trigger:** auto (`createStreamingSamplesVisJob`, `trigger='Auto'`, `preferCpu=true`), deduped against a live k8s job. Per-sample requests are pushed to RabbitMQ queue `<visArtifactId>-streaming-samples-visualizations`.
- **Spawns:** redis + generic-process(1), **no** streaming-handler, no autoscaler; the main pod is pinned tiny (1Gi / 100–500m).
- **No persistence:** visualized items are computed **in memory** and pushed to the UI as `source='streaming-samples'`. The job ends on subscriber timeout.
- **⚠️ vs Visualizers Calculation:** Streaming Samples Vis runs ONE visualizer on demand for an explicit `sample_identities` list, in-memory, single generic-process, no persistence. Visualizers Calculation is a *batch* job that scales generic-process replicas + streaming-handler + autoscaler and **persists** results.
- **Failure (per-sample, has_error):** "Unknown visualizer"; "Sample X does not exist in this session run" (stale `visArtifactId` from a different run); "Visualization error: …".

---

## Warmup (`WARMUP`)
- **Trigger:** `useServerWarmup` calls `POST /jobs/warmup` on user activity, throttled once/10min; gated by the `WARMUP` engine setting (default true).
- **What it actually is:** a k8s **placeholder Job** `engine-warmup-<teamId>-<machineTypeId>` with `WARMUP_MAX_JOBS` parallel pods that each request `nvidia.com/gpu:1` at `warmup-priority` and just `sleep $WARMUP_TIMEOUT_SEC`. Its purpose is to **reserve/keep GPU node capacity warm** (prevent autoscaler scale-down, pre-pull the engine image). The engine `manager.run` `WARMUP` branch is a no-op that only logs "Engine is up and warm".
- **Observe** the `warmup-job=true,created-by=node-server` Job, **not** an engine worker. No UI surface, no persisted status. Success = pods Running then `.status.succeeded`, and subsequent real jobs schedule without cold-start delay.

---

## Project jobs (node jobs — no engine pods)

Export/Copy/Import Project run in an **in-pod node-server runner** (the node-server
image started with `JOB_*` env, `isProcessJob()`), created from `node-job-template-cm`.
They are **not** engine jobs (excluded from engine active-jobs reconciliation).

### Export / Copy Project (`EXPORT_PROJECT`)
- **Trigger:** Projects table → "Download / Export Project" → `POST /projects/exportProject` (Copy = export then HTTP PUT the tar.gz to the target env's upload URL). Synchronous download: `GET /projects/downloadProject/{projectId}` (no job).
- **subType:** `Copy Project` when `copyToUrl` is set, else `Export Project`. **Hidden from the default Runs list.**
- **Outputs:** tar.gz in the bucket (mongo dump + project storage files + project ES indices + team data). If a cached export exists and no `copyToUrl`, **no k8s job** is created (job inserted FINISHED).
- **Stages:** Export (→ Copy). **Failure:** export build error → FAILED; Copy PUT non-2xx → "Copy project failed with status …".

### Import Project (`IMPORT_PROJECT`)
- **Trigger:** Hub gallery (`#hub-gallery`) "Import" → `#import-project-dialog`, or Projects table "Upload project". `POST /projects/importProject` `{importUrl, projectMeta}`. May chain a PUSH for the imported model.
- **Stages:** Download → Import Data (mongo) → Import code-integration → Import Elastic (reindex) → Import Storage (bucket).
- **Outputs:** new project (status `importing`→`visible`) + restored project-scoped collections (versions/models/codesnapshots/dashboards/issues/tests) + ES indices + bucket files.
- **Failure:** any stage throws → the partially-created project is **deleted** (job record kept), pod exits 1, status FAILED; duplicate name → UserError.
- **UI:** new project card in hub/recent-projects + a row in the Runs and Processes table (`#run-and-processes-table-id`); `IMPORT_PROJECT` notifies on completion.

---

## engine-orchestrator (always-on, not a per-request job)
The static `engine-orchestrator` Deployment (`python -m src_tensorleap.engine.engine_scheduler`,
SA `deployment-manager`, container `orchestrator`, `app=engine`). Loop: monitor
failed jobs/pods, report `active_jobs_report` + `failure_jobs_report` to node-server,
scale generic-process Deployments, clean orphan `jobId`-labeled resources, emit
starvation (pod Pending >300s) and memory-leak (20 consecutive RAM increases)
warnings. **If it's down:** TRAINING/ANALYZE throughput stalls (no autoscale),
orphan pods accumulate, OOM/ImagePull failures are never surfaced (jobs look hung).
Logs: "Starting engine scheduler service…", "Monitoring failed jobs", "scaled
generic-process deployment", "deleting orphan per-job resource".
