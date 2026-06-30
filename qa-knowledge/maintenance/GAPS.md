# QA KB — gap log

Append-only log of maintenance runs and open gaps. Newest entry on top. Each run
(scheduled or reactive) adds a dated section: what was reviewed, what changed, and
any gaps that could not be auto-resolved (for a human to act on).

Format:
```
## YYYY-MM-DD — <scheduled | reactive | initial>
Reviewed: <repos + sha ranges, or "n/a">
Changed: <doc — one-line why> (repeat per doc; "none" if no drift)
Open gaps: <doc/topic — what's uncertain and why> ("none" if clean)
```

---

## 2026-06-29 — manual (targeted addition: collection sample-viewer grid)
Reviewed: web-ui `src/sample-collection/**` (CollectionViewDialog, CollectionVisualizationGrid, useStreamingSamplesVis, useStreamingSamplesVisStore, useCollectionSamplePage, useCollectionIndexStatus) + `src/ui/atoms` VirtualizationGrid, at web-ui master `f16249f1` (= current baseline). **Not a drift-sync** — filling a known coverage gap surfaced by BF-1060.
Changed: 03-data-flows.md — added **Flow C — Collection sample-viewer grid** (open → `ensureCollectionIndex`/`getCollectionSampleOrder` → virtual scroll renders cells live, vis fetch debounced to scroll-stop → `STREAMING_SAMPLES_VIS` in-memory render → socket push → FIFO vis cache) + collection-grid failure modes incl. the BF-1060 "keeps refreshing" gotcha and its 0-re-stream verification. 06-ui-inspection.md — cross-link from the Collections selector row. 09-job-catalog.md — Streaming Samples Vis now notes its UI consumer (`generateStreamingSamplesVis`, debounced/deduped) + Flow C link. manifest.json — mapped `web-ui/src/sample-collection/**`, `web-ui/src/ui/atoms/**`, `node-server/src/sampleCollections/**` onto 03/06/09.
Open gaps: (1) Front-end mechanics documented from a **code read**, not observed end-to-end on a fresh "building" index — re-confirm the exact `Indexing… X / Y` badge text and the `ensureCollectionIndex` poll cadence against a live cluster mid-backfill. (2) `generateStreamingSamplesVis` / `ensureCollectionIndex` / `getCollectionSampleOrder` are cited by endpoint name; node-server route handlers were not deep-verified this run (`node-server/src/sampleCollections/**` baseline unchanged). No baselines bumped (web-ui already at HEAD `f16249f1`).

## 2026-06-25 — manual refresh (engine / web-ui / helm-charts → master)
Reviewed: engine `9045cc33..026729cf`, web-ui `c0afae07..f16249f1`, helm-charts `8cbe68cb..05643d36` (all baselines were ancestors — clean diffs; 14 per-file verifiers).
Changed: 01-architecture.md — image tags (engine/engine-generic→`026729cf`, node-server→`9276bb7c`, web-ui→`f16249f1`) + chart version `1.6.28`→`1.6.33`. 03-data-flows.md & 07-failure-modes.md — exit 137 during pod **teardown** (`deletion_timestamp` set) is now classified `UNKNOWN`, not `OOM_KILLED`. 04 & 09 — added the new 7th `SLIM_LS` subtype **Resplitting** (`WorkerSlimLSOps.resplitting`, request `SlimResplittingRequest`, bucket `digest_<d>/resplitting/*`).
Open gaps: **Resplitting node-server side not shipped** — node-server master has no `resplit` reference, so the REST trigger / subType label / mongo entity / DS-Curation UI for resplitting don't exist yet; doc 09 documents the engine side and flags this. Re-verify when node-server wires it. No-change (internal/additive only): visualizationmanager, redis_vis_queue_manager, insights_calculation_manager, leaptrainer, visualizer_calculator, basestorage sample-vis helper, web-ui PE status hook (already matched after the node-server refresh), version-state.ts, and the installer Go files.

## 2026-06-25 — manual refresh (node-server → master)
Reviewed: node-server `1b44e3bb..9276bb7c` (now on `master`; baseline was an ancestor — 4 Population-Exploration commits #1757–#1760). Other repos not re-checked this run.
Changed: 09-job-catalog.md — Population Exploration: (a) PE now blocks only until the prerequisite evaluate's `insights_analysis` step finishes/skips (#1758), not for the whole evaluate job; (b) the digest no longer hashes `sample_visualizers_revision` (#1760).
Open gaps: none new. `storage.ts` changed the sample-visualizers folder `sample_visualizers_<N>` → `sample_visualizers/<N>` (#1757), but doc 09 uses the `sample_visualizers/*` glob so no edit was needed. node-server baseline bumped to `master` HEAD `9276bb7c`; the feature-branch caveat below is resolved.

## 2026-06-24 — initial
Reviewed: KB authored from helm-charts@8cbe68cb, engine@9045cc33,
node-server@1b44e3bb (since merged to master), web-ui@c0afae07.
Changed: all docs created (README, 01–10) + maintenance scaffolding.
Open gaps:
- ~~**node-server baseline was a feature branch**, not `master`.~~ **RESOLVED 2026-06-25** — reconciled to node-server `master` (HEAD `9276bb7c`); see the 2026-06-25 refresh entries above.
- **Graph jobs** (`DRY_RUN_GRAPH` vs `ANALYZE_GRAPH`, and how "Graph Validate"
  maps to each within the PUSH flow) were slightly conflated across recon sources —
  confirm precise spawn behavior and k8s names on a live cluster (noted in
  [09-job-catalog.md](../09-job-catalog.md#graph-jobs-two-distinct-things)).
- **SLIM_LS per-task memory ceilings** (insights/balancing load latent spaces in a
  single pod) — confirm OOM thresholds on a live cluster.
- **leap-cli + code-loader newly added to tracking** (2026-06-25) with null
  baselines — their docs (02/03/05/08/09) were authored from the first recon without
  these repos checked out, so the first `update-qa-knowledge` refresh should
  full-verify them and record real master SHAs. leap-cli's local checkout is on branch
  `fix-default-client-proxy`; track `master`.
- A few `file:line` anchors are snapshots and may have drifted; treat as
  named-symbol hints.
