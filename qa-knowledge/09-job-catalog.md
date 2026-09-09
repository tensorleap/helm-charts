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
| `ANALYZE` | per-job **redis** + **generic-process (1 or N)** + **streaming-handler** | Sample Analysis (1), Visualizers Calculation (N), Domain Gap |
| `SYNTHETIC` (engine type) | per-job **redis** + **generic-process (1)** + **streaming-handler** | Synthetic Data Generation — **auto mode only** (`generateAutoSyntheticData`; the manual calibration mode is `SLIM_LS`) |
| `PUSH` | per-job **redis** + **generic-process (1)**, **no** streaming-handler | Push (= Code Parse + Import Model + Graph Validate phases) |
| `EXPORT_MODEL` | per-job **redis** + **generic-process (1)**, no streaming-handler | Export Model |
| `DRY_RUN_GRAPH` | per-job **redis** + **generic-process (1)**, no streaming-handler | Graph Validate |
| `STREAMING_SAMPLES_VIS` | per-job **redis** + **generic-process (1)**, no streaming-handler | Streaming Samples Vis |
| `SLIM_LS` | **NOTHING** — one single `SLIM` pod | **Population Exploration, Fetch Similar, Generate Insights, Dataset Balancing, Synthetic Data Generation (manual/calibration), Labeling Recommendation, Splitting, Unlabeled Analysis** |
| `ANALYZE_GRAPH` | **NOTHING** — engine main pod only | (graph static analysis, a phase of import) |
| `WARMUP` | a sleep-placeholder GPU **Job** (`engine-warmup-*`) | Warmup |
| node job (`EXPORT_PROJECT`/`IMPORT_PROJECT`) | one **node** Job pod (node-server image), no engine pods | Export/Copy/Import Project |

**SLIM_LS is the big one to internalize:** eight different SLIM_LS request types run as a
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
| Domain Gap | `ANALYZE` | `WorkerAnalyzer._domain_gap` → `DomainGapJobRunner` | `domain-gap-<jobId>` | mongo `domaingap`; bucket `vis/<vis_artifact_id>/domain_gap/<digest>/{stats.json, cluster.json, heatmaps.json + heatmaps}` | row in DS Curation → DOMAIN GAP tab grid |
| Population Exploration | `SLIM_LS` | `WorkerSlimLSOps.population_exploration` | `population-exploration-<jobId>` | bucket `vis/<vis_artifact_id>/population_exploration/digest/<digest>/scatter.json` | `#population-exploration-circles` (dots) |
| Fetch Similar | `SLIM_LS` | `WorkerSlimLSOps.create_cluster_filter` | `fetch-similar-<jobId>` | bucket `vis/<vis_artifact_id>/fetch_similar/<digest>/cluster.json` | filter chip + highlight in `#population-exploration-circles` |
| Generate Insights | `SLIM_LS` | `WorkerSlimLSOps.insights_calculation` | `generate-insights-<jobId>` | mongo `insights` + `versions.resources.csv_blob_path`; reads ES `es_metrics_index` | `#insight-card` under `#insights-list` |
| Dataset Balancing | `SLIM_LS` | `WorkerSlimLSOps.dataset_balancing` | `dataset-balancing-<jobId>` | mongo `datasetbalancing`; bucket `digest_<d>/dataset_balancing/*` | row in DS Curation → PRUNING tab grid |
| Synthetic Data Generation (manual) | `SLIM_LS` | `WorkerSlimLSOps.synthetic_calibration` | `synthetic-data-generation-<jobId>` | mongo `syntheticdata`; bucket `digest_<d>/synthetic-calibration/{next,best}_trials.csv` + `synthetic_top_panel.json` | row in DS Curation → SYNTHETIC tab grid |
| Synthetic Data Generation (auto) | `SYNTHETIC` | `WorkerSyntheticJob` | `synthetic-data-generation-<jobId>` (same subType label as manual) | mongo `syntheticdata` (shared collection with manual); bucket `synthetic_top_panel.json` | row in DS Curation → SYNTHETIC tab grid |
| Labeling Recommendation | `SLIM_LS` | `WorkerSlimLSOps.labeling_recommendation` | `labeling-recommendation-<jobId>` | mongo `generatedLabels`; bucket `digest_<d>/labeling/*` | row in DS Curation → LABEL NEXT tab grid |
| Splitting | `SLIM_LS` | `WorkerSlimLSOps.resplitting` | `splitting-<jobId>` | mongo `datasetsplitting`; bucket `digest_<d>/resplitting/{<jobUid>.csv, resplitting_cluster_filter.json}` | row in DS Curation → SPLITTING tab grid |
| Unlabeled Analysis | `SLIM_LS` | `WorkerSlimLSOps.unlabeled_analysis_task` → `UnlabeledAnalysis.run_unlabeled_analysis` | `unlabeled-analysis-<jobId>` | mongo `unlabeledanalysis`; bucket `vis/<vis_artifact_id>/unlabeled_analysis/<jobId>/stats.json` (analysis_id = jobId, **not** the `digest_<d>` pattern) | row in DS Curation → UNLABELED (`UNLABELED_ANALYSIS` tab value) grid; optional dashboard top panel |
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
- **Domain-gap context:** when the request carries a `domain_gap_id`, the same worker renders only the MISSING assets per sample (regular visualization and/or domain-gap heatmap) and refreshes `heatmaps.json`; heatmap render failures (e.g. `NoUsableLayersError` — no usable spatial layers) never fail the job, they're logged and skipped.

### Domain Gap  *(new ANALYZE subtype)*
- **Trigger:** DS Curation → **DOMAIN GAP** tab → `POST /datasetcuration/generateDomainGap` `{projectId, versionId, groupAFilters, groupBFilters}`. UI validation blocks: "No model selected", and each domain needs ≥1 filter ("Domain #1/#2 needs at least one filter"). `subType='Domain Gap'`, engine `analyze_type=domain_gap`, `preferCpu=false`.
- **What it does** (`DomainGapJobRunner`): stats → A∪B population exploration → adapter → heatmaps, all **persisted to the blob dir** `vis/<vis_artifact_id>/domain_gap/<digest>/` (`stats.json`, `cluster.json`, `heatmaps.json` + heatmaps) — **no push message**; node-server reads the blobs on the standard FINISHED job-status update. `population_exploration_n_samples` is pinned to 2000 to match the PE dashlet default, so the inline scatter lands under the digest the dashlet later mints.
- **Outputs:** mongo `domaingap` entity (jobId ref); the tab row exposes stats/filter download URLs only when the files exist (`hasStatsFile`/`hasFilterFile`).
- **Success:** job FINISHED + `stats.json` in the bucket + a new row in the DOMAIN GAP tab's DataGridPro.

---

## SLIM_LS family (single pod — no redis/generic/streaming)

> Common to all: `kubectl get pods -l jobId=<jobId>` shows **one** engine pod
> `jobType=SLIM_LS`, **no** companions; `JOB_NOTIFICATION_CONFIG.SLIM_LS=false`
> (results surface via each feature's own message handler, not a generic job
> notification); `post_running` sets FINISHED/FAILED.

### Population Exploration  *(this is a `SLIM_LS` job, NOT `ANALYZE`)*
- **Trigger:** auto-runs when the dashlet mounts. `POST /visualizations/populationExploration` then polls `POST /visualizations/getPopulationExplorationStatus` every ~3s. **Blocked only while a prerequisite Evaluate/Update-Evaluate job's `prepare_displays` step is still pending** (UserError before the job is created — only then are the scatters published and the resource pin final); once that step's event reaches `FINISHED` or `SKIPPED`, PE may run even if the evaluate job is still in progress (later steps like `visualize_samples` continue).
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
All are launched from the **DS Curation** toolbar button → `DatasetCurationDialog`
(title "DATASET CURATION LIST"), now six tabs: **LABEL NEXT** (default, tab *value*
`UNLABELED`) **/ UNLABELED** (tab *value* `UNLABELED_ANALYSIS`, see the Unlabeled
Analysis section below) **/ DOMAIN GAP / SYNTHETIC / PRUNING / SPLITTING**, via
`EvaluationAwareActionButton` (warns if the eval is incomplete). Splitting and Domain
Gap are covered in their own sections. ⚠️ The tab **labels** were reshuffled when
Unlabeled Analysis was added — the pre-existing tab (value `UNLABELED`, backs Labeling
Recommendation, component `UnlabeledTabContent.tsx`) is now labeled "LABEL NEXT", and
the label "UNLABELED" moved to the brand-new tab (value `UNLABELED_ANALYSIS`, component
`UnlabeledAnalysisTabContent.tsx`). Don't confuse the tab *value* with its *label*.

| | Dataset Balancing | Synthetic Data Generation | Labeling Recommendation |
|---|---|---|---|
| endpoint | `/datasetcuration/generateDatasetBalancing` | `/datasetcuration/generateSyntheticData` | `/datasetcuration/generateLabels` |
| `slim_request_type` | `dataset_balancing` (algo PRUNING) | `synthetic_calibration` | `labeling_recommendation` (algo CORESET) |
| mongo entity | `datasetbalancing` | `syntheticdata` | `generatedLabels` |
| bucket output | `digest_<d>/dataset_balancing/{dataset_balancing-recommendations.csv[.tar.gz], dataset_balancing_cluster_filter.json}` + `dataset_balancing_stats.json` (node-server checks for/exposes it as `statsFileUrl`; ⚠️ engine `master` doesn't write this file yet — see gotcha below) | `digest_<d>/synthetic-calibration/{next_trials.csv, best_trials.csv}` + `synthetic_top_panel.json` (both manual and auto flows write this; node-server exposes it as `statsFileUrl`) | `digest_<d>/labeling/{labeling-recommendations.csv, labeling_cluster_filter.json, labeling_stats.json, suggested_cluster.json}` |
| UI tab | PRUNING | SYNTHETIC | LABEL NEXT (tab value `UNLABELED`) |
| validation block | no model / no dashboard / no pop-exp dashlet | "Target is empty" / "No sources added" | "No model selected" |

- **Success (all):** job FINISHED + entity row present + the output file(s) exist in
  the bucket + a new row in the tab's DataGridPro. **Note:** a job can be FINISHED
  while the output file is absent (e.g. optimizer produced no trials) → the UI row
  shows no download. Don't treat FINISHED alone as success — check the bucket file.
- **Labeling Recommendation → "Apply as dashboard top panel":** each LABEL NEXT-tab
  row can mint a dashboard top panel from its `suggestedClusterFileUrl` (needs
  `statsFileUrl` too) via `applyUnlabeledTopPanel` (web-ui `UnlabeledTabContent.tsx`).
  While that panel is open, the Population Exploration dashlet no longer just
  filters down to the suggested samples — it swaps its cluster filter to the
  recommendation's balanced/`filterFileUrl` blob so the dashlet keeps showing the
  labeled, suggested, and not-chosen populations together (`useLabelingPopulationClusterSwap`
  in `web-ui/src/dashboard/top-panel/useTopPanelState.ts`); every other dashlet is
  still filtered to just the suggested samples.
- **Dataset Balancing → "Apply as dashboard top panel":** same pattern on the
  PRUNING tab — a row with both `statsFileUrl` and `filterFileUrl` shows an
  "Apply as dashboard top panel" action (`handleApplyTopPanelClick` in web-ui
  `BalancingTabContent.tsx`) that mounts a 4th top-panel kind (`kind: 'pruning'`,
  `applyPruningTopPanel` in `DashboardContext.tsx`) rendering the coverage curve
  / kept-vs-pruned metadata distribution from the stats blob
  (`PruningTopPanel.tsx`), replaces the dashboard's global filters with the
  run's training-split cluster filter, and joins the run's version to the
  dashboard's selection — same one-batched-write shape as `applyDomainGapTopPanel`.
  **⚠️ Currently dormant:** `statsFileUrl` is only set once the bucket has
  `dataset_balancing_stats.json`, which engine `master` doesn't produce yet
  (in progress on a separate branch) — so today every PRUNING row only has
  `filterFileUrl`, and the row falls back to the pre-existing "Apply filter to
  Population Exploration" action instead. Re-check once the engine side ships.
- **⚠️ Synthetic confusion:** the SYNTHETIC tab now has **two modes**, both labeled
  `subType='Synthetic Data Generation'` (same k8s job name, same `syntheticdata`
  mongo collection):
  - **Manual** → `POST /datasetcuration/generateSyntheticData` → **`SLIM_LS`**
    calibration/optimizer job (single SLIM pod, `slim_request_type=synthetic_calibration`,
    `preferCpu=true`).
  - **Auto** → `POST /datasetcuration/generateAutoSyntheticData` → engine
    **`SYNTHETIC`** job (`WorkerSyntheticJob`, `SyntheticJobRequest` with
    `simulation_names`/`target_filters`/`initial_simulation_filters`; the engine
    generates simulation parameters itself, so `simulations_data` is sent empty) —
    this one **does** spawn redis+generic+streaming.
  Tell them apart by the pod signature / `jobType` label, not the subType.

### Splitting (resplitting)
The 7th of 8 `SLIM_LS` request types: `slim_request_type=resplitting`, worker
`WorkerSlimLSOps.resplitting`. It re-splits the dataset across train/val/test:
groups samples by `keep_together_metadata`, stratifies across `split_across_metadata`
(request `SlimResplittingRequest`, subsets mapped to the engine's numeric
`DataStateEnum` training=0/validation=1/test=2).
- **Trigger:** DS Curation → **SPLITTING** tab → `POST /datasetcuration/generateDatasetSplitting`
  `{projectId, versionId, splitsToResplit, keepTogetherMetadata, splitAcrossMetadata}`;
  `subType='Splitting'`, `preferCpu=true`. UI validation: "No model selected".
- **Spawns:** a single `SLIM` pod (no redis/generic/streaming), like the other SLIM_LS jobs; k8s job `splitting-<jobId>`.
- **Outputs:** mongo `datasetsplitting` entity; bucket `digest_<d>/resplitting/{<jobUid>.csv, resplitting_cluster_filter.json}` — the CSV is named `<jobUid>.csv` by the engine (uid = job.cid), only the filter filename is fixed.
- **Success:** job FINISHED + a new row in the SPLITTING tab's DataGridPro.

### Unlabeled Analysis  *(new SLIM_LS subtype)*
An 8th `SLIM_LS` request type: `slim_request_type=unlabeled_analysis`
(`SlimRequestTypeEnum.unlabeled_analysis`, `engine/src_tensorleap/contract/workerslimlsops/request/slimlsopsrequest.py`),
dispatched to `WorkerSlimLSOps.unlabeled_analysis_task` → `UnlabeledAnalysis.run_unlabeled_analysis`
(`engine/src_tensorleap/workers/workerslimlsops/workerslimlsops.py`,
`engine/src_tensorleap/trainer/ds_curation/unlabeled_analysis.py`). It triages a filtered unlabeled
population against three questions: out-of-distribution clusters, similarity to known
low-performance "aggressor" clusters, and a per-sample trust/confidence score
(`engine/src_tensorleap/contract/workerslimlsops/response/unlabeledanalysisstats.py`).
- **Trigger:** DS Curation dialog's **UNLABELED** tab (tab *value* `UNLABELED_ANALYSIS`, distinct
  from the older `UNLABELED` tab which is now labeled "LABEL NEXT" and still backs Labeling
  Recommendation) → `POST /datasetcuration/generateUnlabeledAnalysis` `{projectId, versionId,
  filters?, latentSpaceType?, elementInstance?}` (`node-server/src/dataset-curation/controller.ts`,
  `logic.ts: generateUnlabeledAnalysis`); `subType='Unlabeled Analysis'`, `preferCpu=true`. The
  request also carries `aggressors` (the version's current low-performance insight refs, via
  `getAggressorRefs`) and `insights_counter` pinned to the live revision.
- **Spawns:** a single `SLIM` pod (no redis/generic/streaming), like the other SLIM_LS jobs; k8s job `unlabeled-analysis-<jobId>`.
- **Outputs:** mongo `unlabeledanalysis` entity (`node-server/src/dataset-curation/db.ts`, collection
  `unlabeledanalysis`) tracking `hasStatsFile`; bucket `stats.json` written by
  `UnlabeledAnalysisStore` under `vis/<vis_artifact_id>/unlabeled_analysis/<analysis_id>/`, where
  `analysis_id` is the job id — unlike Dataset Balancing/Splitting/Labeling this is **not** keyed by
  `digest_<d>` (it mirrors `DomainGapStore`'s per-run, version-coupled directory instead). Per-sample
  OOD/trust results are written separately as display metadata (for the Population Exploration
  scatter), not into `stats.json`.
- **Success:** job FINISHED + `stats.json` present (`hasStatsFile=true`) + a new row in the UNLABELED
  tab's DataGridPro.
- **Apply as dashboard top panel:** a row can mint a top panel via `applyUnlabeledAnalysisTopPanel`
  (`web-ui/src/dashboard/DashboardContext.tsx`), gated by `useTopPanelUnlabeledAnalysisRecord`
  (`web-ui/src/dashboard/top-panel/useTopPanelState.ts`) and rendered by
  `web-ui/src/dashboard/top-panel/UnlabeledAnalysisTopPanel.tsx` — same one-panel-at-a-time pattern
  as Domain Gap/Pruning's top panels. Uses icon `web-ui/src/ui/icons/ood-cluster-icon.svg`.
- **Note:** `WorkerSlimLSOps._unlabeled_analysis_request` (`workerslimlsops.py`) still contains a
  fallback path that re-tags a `synthetic_calibration`-shaped request as `unlabeled_analysis`,
  described in its docstring as a transitional shim from "before node knows how to send
  `unlabeled_analysis`". node-server's `generateUnlabeledAnalysis` (`logic.ts`) already sends the
  real `slimRequestType: 'unlabeled_analysis'`, so that fallback should be dead in practice — worth
  a live-cluster spot-check if this job ever appears to run with an empty/wrong request shape.

---

## PUSH and its phases

### Push  (`PUSH` / `WorkerPush`)
- **Trigger:** primarily the **`leap push`** CLI (also code-integration panel). `POST /versions/push` (new version + model upload) or `POST /versions/pushOverride` (re-push to an existing version, reuses model). Web-ui renders **status only** — there is no primary push button in the SPA. CLI note: passing `-n/--name` with no `--overwrite` target now signals intent to create a **new** version and skips the interactive overwrite prompt (`wantsNewVersion` in `leap-cli/cmd/root_cmd/push.go`); it still prompts for `--model-path` if omitted.
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
- **UI consumer:** backs the **collection sample-viewer grid** — the grid requests per-viewport visualizations via `generateStreamingSamplesVis` (debounced on scroll-stop, deduped against an in-memory FIFO cache) and receives them as `source='streaming-samples'` socket pushes. End-to-end: [Flow C in 03](03-data-flows.md#flow-c--collection-sample-viewer-grid).
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
