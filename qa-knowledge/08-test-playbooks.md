# 08 · Test playbooks (turning a test plan into verifiable steps)

How a QA agent converts a free-form test plan into deterministic, evidence-backed
steps, plus reusable templates for the most common Tensorleap flows.

---

## How to consume a test plan

For every test-plan item, produce a step with these five fields:

1. **Action** — the exact UI action or CLI command (with concrete project/version ids).
2. **Pre-conditions** — auth mode, cluster healthy, project/version exists,
   no conflicting running job.
3. **Expected back-end observable** — the Mongo/k8s/Redis/ES/bucket/RabbitMQ
   signal from [03-data-flows.md](03-data-flows.md) / [05-testing-utils.md](05-testing-utils.md).
4. **Expected front-end observable** — the selector/state from
   [06-ui-inspection.md](06-ui-inspection.md) (always paired with #3 — never UI-only).
5. **Pass/fail rule** — explicit, including which empty/error states are
   acceptable vs failures.

Rules of thumb:
- **Never assert on the UI alone** when a back-end signal exists (a green UI over
  empty/stale data is the classic false pass).
- **An empty/error state is a specific signal**, not "broken" — classify it via the
  taxonomy in [03](03-data-flows.md) / [07](07-failure-modes.md).
- **Watch transient resources live** (`kubectl get pods -w -l jobId=<jobId>`) —
  per-job redis/generic/streaming-handler vanish after FINISHED, so capture them
  during the run.
- **Establish a clean baseline** before destructive/stateful tests (note existing
  jobs/versions so you can attribute new ones).
- **Capture provenance** (image tags, chart version) so a failure can be tied to a
  build.

---

## Playbook 1 — Pre-flight (run before any functional test)

| Step | Command | Pass |
|---|---|---|
| Cluster up | `leap server tools k3d cluster list` | cluster `tensorleap` running |
| Namespace healthy | `kubectl -n tensorleap get pods` | node-server, web-ui, mongodb, minio, rabbitmq, ES master, keycloak, engine-orchestrator all `Running`/`Ready` |
| node-server health | `curl -s localhost:4589/api/v2/monitor/healthCheck | jq` | 200, all modules `status` ok |
| ES healthy | `kubectl -n tensorleap get elasticsearch tl-elasticsearch` | health green/yellow |
| Build provenance | compare pod images to `images.txt` | matches the build under test |
| Auth mode known | `getAuthProvider()` (or UI behavior) | Keycloak vs Local recorded |

If any fail, stop and classify via [07-failure-modes.md](07-failure-modes.md).

---

## Playbook 2 — `leap push` (code parse + model import + validate)

**Action:** `leap auth login <url> -k <apiKey>` → from a project dir with
`leap.yaml` → `leap push -n <name> --type <JSON_TF2|ONNX|...> [-m <model-path>]`.

| Phase | Back-end observable | Front-end observable |
|---|---|---|
| Auth | `~/.config/tensorleap/config.yaml` has `current_env`; `auth whoami` ok | n/a |
| Bundle | CLI bundles code to a temp tar.gz | n/a |
| Job created | `db.jobs` new doc subType `Push`/`Code Parse`/`Import Model`; `kubectl get jobs -l projectId=<p>` shows `push-<jobId>` (engine `PUSH` → `WorkerPush`) | a new run appears in `run-and-processes-table-id` |
| Parse + validate | engine-generics runs `CodeParser.parse()` then `ImportModel.import_and_validate()`; pod logs | n/a |
| Success | job `FINISHED`; new version/model in `db.versions`/`db.models` | version appears in `version-control-pane` history |
| Failure | CLI prints a **ValidateAsset report** + graph-validation errors mapped to nodes | (CLI) |

**Pass:** job FINISHED, version created, no ValidateAsset errors.
**Cheapest pre-check:** run the integration's `tensorleap_integration_test()`
locally first — it runs the binder and raises `… validation failed: …` before any
push.

**Overwrite / sub-version:** `-o/--overwrite <id|name>` targets an existing
version; `-u/--update {metadata|metric|metric_config|viz}` (implies `--eval`)
chooses a full re-evaluate vs an update-evaluate-artifact run. (The flag is
`--overwrite`, **not** `--override`.)

---

## Playbook 3 — Evaluate end-to-end (the flagship test)

**Action:** `leap push -e -b latest` (or UI **Evaluate** on a version).

Watch live: `kubectl -n tensorleap get pods,deploy,svc -w -l jobId=<jobId>`.

| Checkpoint | Observable | Pass condition |
|---|---|---|
| Request accepted | `POST /evaluate/evaluate` 200 → Job `UNSTARTED`; `db.jobs` new `type:TRAINING, subType:Evaluate` | job doc exists |
| Job dispatched | `kubectl get jobs` → `evaluate-<jobId>` (labels `jobId`,`projectId`,`hasWorker=true`); job `PENDING` | Job object created |
| Runtime resources | `redis-<jobId>` (pod+svc), `generic-process-<jobId>`, `streaming-handler-<jobId>` appear | all three present; `redis-cli PING → PONG` |
| ES indices | `db.versions` `resources.es_metrics_index` set; `GET /_cat/indices` shows it | index created |
| Sample gen | `LLEN dataset_<state>_<jobId>_ready` rises; generic log `samples generator: generate single sample` | ready queue advancing |
| Inference + stream | `LLEN streaming_evaluate_<jobId>_queue` > 0; main pod `Pushing elements to redis key: streaming_*` | queue active |
| Indexing | streaming log `indexing docs to elasticsearch`; `GET <es_metrics_index>/_count` rises | count → ~ number of samples |
| Completion | `kubectl get job evaluate-<jobId>` `succeeded=1`; runtime pods gone; job `FINISHED`; version flagged evaluated | all true |
| Dashboards | open the version's dashboards; dashlets render with data | per [06](06-ui-inspection.md): negative + positive assertions pass |

**Pass:** the compound success signal from
[04-job-types-and-lifecycle.md](04-job-types-and-lifecycle.md) (Mongo FINISHED +
`succeeded=1` + pods gone + `_count > 0`) **and** the dashboards render with data.
**On failure:** classify via [07-failure-modes.md](07-failure-modes.md) sections C/D/E.

---

## Playbook 4 — Dashboard / dashlet render

**Action:** open a project → a dashboard with Analytics / Population Exploration /
Sample Analysis dashlets; apply a global filter; select a version.

| Step | Back-end | Front-end |
|---|---|---|
| Fields resolve | `POST /dashboards/getDashletFields` 200 | none of the dashletFields error/empty texts |
| Version selected | `?selected-version=<id>` in URL | not `Select a version to see data.` |
| Chart query | `POST /sessionmetrics/getXYChart` 200 with `charts[].length>0` | inside `#analytics-dashlet`: chart geometry, no `No results found`, no spinner |
| Filter applied | request body carries `filters` (`mapToEsFilters`); ES `_search` buckets change | result set changes consistently with the filter |
| Population Exploration | population-exploration job + presigned bucket fetch ok | `population-exploration-processing` → `population-exploration-dashlet` (scatter) |
| Live refresh | after a fresh Evaluate, `serverMessage` WS frame arrives | a prior `No results found` dashlet repopulates without reload |

**Pass:** data render (positive assertion) confirmed against a non-empty ES query.
**Distinguish** the empty states — only the "error/down" ones are failures (see the
decision tree in [07](07-failure-modes.md#f-dashlet-empty--which-empty-decision-tree)).

---

## Playbook 5 — Version control & history

**Action:** in `version-control-pane`, expand experiment history, set a version
active, re-evaluate.

| Step | Back-end | Front-end |
|---|---|---|
| History loads | `getProjectSlimVersions` returns the chain (`experimentId`/`parentVersionId`) | rows in the versions table |
| Set active | `setActiveVersion` updates Mongo `versions` | `aria-label="Make this the active version"` reflects new active |
| Expand | — | `aria-label="Expand experiment"` reveals sub-versions |
| Re-evaluate | new Evaluate job (Playbook 3) tied to that version | new run appears; dashlets update for the version |

**Pass:** active version persists across reload (`db.versions` + `?selected-version`),
and re-evaluate produces a fresh FINISHED job whose metrics show in the dashboards.

---

## Playbook 6 — Job control (stop / terminate)

**Action:** start a long Evaluate, then **Stop** (graceful) or **Terminate** (hard)
from the UI / `leap run`.

| Step | Back-end | Front-end |
|---|---|---|
| Stop | node-server publishes to `job-control-channel-<jobId>` (RabbitMQ); engine main pod receives stop | job → `STOPPED` |
| Terminate | k8s Job/pods deleted | job → `TERMINATED`; runtime pods (`-l jobId=<jobId>`) removed |

**Pass:** status reaches `STOPPED`/`TERMINATED` in Mongo **and** the per-job pods
are gone. A job stuck `STARTED` after stop → suspect RabbitMQ control path or a
non-responsive pod (orchestrator should still reconcile).

---

## Reporting a result (template)

```
TEST: <name>            RESULT: PASS | FAIL | BLOCKED
Build: node-server <tag> / engine <tag> / web-ui <tag> / chart <ver>
Steps verified: <which checkpoints passed>
Evidence:
  - back-end: <mongo/k8s/redis/es/bucket/rabbitmq output>
  - front-end: <selector/state + Network/WS frame>
Failure classification (if FAIL): <section in 07-failure-modes.md> + confirming command output
Notes / flakiness / env caveats:
```

Always attach the **confirming command output**, not a paraphrase — a QA finding is
only as good as its evidence.
