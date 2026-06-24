# 07 · Failure-mode playbook

Symptom → root cause → the exact signal that confirms it. Organized by where the
failure surfaces. Use this to **classify** a failure, not just report "it broke".

> General principle: a job has **no retries** (`backoffLimit=0`). A pod that dies
> is final; the orchestrator reconciles status. Always correlate three views:
> Mongo `jobs.status`, `kubectl get pods -l jobId=<jobId>`, and the relevant pod
> logs.

---

## A. Auth / transport (no job is created yet)

| Symptom | Root cause | Confirm |
|---|---|---|
| Login never completes; `AuthLoadingScreen` (`role=status`) never clears; repeated `GET /auth/realms/.../auth` | Keycloak `check-sso` failing: clock skew, wrong realm URL, blocked cookies; on non-localhost plain HTTP the Web-Crypto polyfill must be active or PKCE S256 fails | browser console `Error initializing auth`; no token lands; check Keycloak pod logs + realm config |
| Every `/api/v2` returns **401 "No token provided"** with an `Authorization` header present | harness/client used `Bearer` instead of **`KBearer`** | inspect the request header scheme; node-server `isUserRequest` requires `KBearer` |
| Valid token but **401 "User not found"** | Keycloak email has no `users` doc in Mongo | `db.users.findOne({"local.email":"<email>"})` is null |
| Forced to `/conflict-users`, reload loop | **409** concurrent-user conflict (`uniqueId` cookie ≠ `user.local.uniqueId`) | 409 responses; two active sessions for one user |
| **403** on some calls | demo-user or expired-license scope (`not-demo`/`licensed`) | response body `Demo user not authorized` / `License expired` |
| `/socket.io` returns HTML (`index.html`) instead of an engine.io handshake | ingress `/socket.io` path rule not matching (misroute) | `curl` the path; expect engine.io handshake, not HTML |
| CORS preflight (OPTIONS) fails | served cross-origin / basePath misconfigured | DevTools OPTIONS non-2xx |
| Live updates stop arriving (UI stale, needs manual refresh) | socket disconnect / `authentication_error`; or RabbitMQ consumer down | WS frame `authentication_error`; or rabbitmq queue depth growing with no consumer |

---

## B. Job won't start / stays PENDING

| Symptom | Root cause | Confirm |
|---|---|---|
| Job stuck `UNSTARTED`/`PENDING`, no pod | k8s Job created but pod unschedulable (no CPU/mem), or template/configmap missing | `kubectl describe job evaluate-<jobId>`; `kubectl get pods -l jobId=<jobId>`; node-server log `config-map was not found` ⇒ missing `engine-job-template-cm`/`engine-job-config` |
| Pod `Pending` "Unschedulable" | requested resources exceed node capacity (auto-settings too high for this machine) | `kubectl describe pod` Events: `Insufficient cpu/memory`; check the applied `resources` |
| `redis-<jobId>` Pending forever; main pod log `Redis pod is Unschedulable -- waiting for cluster resources` | no room for the per-job redis | `kubectl get pod redis-<jobId>`; free resources or lower sizing |
| Pod `ImagePullBackOff`/`ErrImagePull` (engine/generic/redis) | bad tag / registry unreachable / pippin build failed | `kubectl describe pod`; for redis the main pod fast-fails `Redis pod ... unrecoverable state` |
| Hangs in pippin **init** container | dependency build/registry problem | init-container logs; Zot `/v2/_catalog` (see [05](05-testing-utils.md)) |
| Concurrency rejection | another Evaluate already running for the version, or per-user limit | POST `/evaluate/evaluate` returns `An evaluate job is already running` or **429** `CONCURRENT_EVALUATE_LIMIT` |

---

## C. Evaluate runs then fails (customer-code / data)

| Symptom | Root cause | Confirm |
|---|---|---|
| `generic-process-<jobId>` log `Sample generator failed`; UI warning | customer `preprocess`/`get_sample` raised | `kubectl logs deploy/generic-process-<jobId>`; a `SampleSerializableError` is pushed |
| Job FAILED `Evaluation failed due to too many discarded samples` | >20% of samples errored (success ratio < 0.8) | main pod `_validate_evaluated_samples`; job FAILED |
| Job FAILED `Dataset code had crashed` | `DatasetScriptException` at model/inference level | main pod stack trace; `FAILED` via RabbitMQ |
| Job FAILED on encoder contract | encoder returned non-`float32` ndarray / bad `channel_dim` / sample-id type mismatch | push-time validation `… validation failed: …`; reproduce locally with `tensorleap_integration_test()` |

---

## D. Resource / infra (OOM, backpressure, ES)

| Symptom | Root cause | Confirm |
|---|---|---|
| Pod `OOMKilled` / exit **137** / phase `Evicted` | under-provisioned memory (auto-settings too low, batch too big) | `kubectl get pod -l jobId=<jobId>`; `kubectl describe pod`; orchestrator maps all → `OOM_KILLED` and FAILs the job. Knobs: machine type, `batch_memory_multiple`, `batchSize` |
| Throughput stalls mid-run | Redis backpressure: streaming-handler can't keep up | `LLEN streaming_evaluate_<jobId>_queue` stays high; main pod push loop log repeats; streaming-handler replicas at max (10) |
| Per-job redis crashes under load | `maxmemory-policy noeviction` + too-small redis → OOM instead of evict | `redis-cli INFO memory` `used_memory` near `maxmemory`; redis pod restart |
| ES `_count` lower than evaluated samples; partial/empty dashlets | streaming-handler dropped docs (NaN sanitation failure or bulk error) | streaming-handler log `failed to index docs to elasticsearch` (`err_vec`) |
| Job FAILED `API error (503): low available disk space (<150GB)` | ES disk watermark | ES node disk; `kubectl get elasticsearch tl-elasticsearch` health |
| Dashlets empty, eval stalls writing metrics | ES CR unhealthy / operator issue | `kubectl get elasticsearch -n tensorleap`; ECK operator + ES pod logs |

---

## E. Status / feedback desync

| Symptom | Root cause | Confirm |
|---|---|---|
| Mongo job stuck `STARTED` but pod is gone | engine pod died before publishing `FAILED` | orchestrator `active_jobs_report` reconciles to FAILED after misses; compare Mongo vs `kubectl get job`; orchestrator logs |
| UI shows job "running" forever; dashlets never auto-refresh after completion | RabbitMQ down / consumer absent → status never published | rabbitmq mgmt UI queue `feedback` depth growing, no consumer; node-server reconnect logs; no `serverMessage` WS frames |
| Job FINISHED in Mongo but per-job pods linger | teardown delayed | engine Jobs keep `ttlSecondsAfterFinished=36000`; pods/deploys for the job should disappear shortly after FINISHED |

---

## F. Dashlet "empty" — which empty? (decision tree)

```
Dashlet shows no chart
├─ "Loading..."                      → transient; wait / re-check
├─ "Sorry, there was an error..."    → getDashletFields failed → ES mapping endpoint / ES down  → check ES (05)
├─ "Training/Evaluation process is
│   required to visualize data"      → no aggregatable fields in mapping → eval never wrote usable metrics
├─ "Select a version to see data."   → user state: no version selected → select a version
├─ "No results found" (NoDataChart)  → getXYChart returned charts:[]
│     ├─ index missing               → GET /_cat/indices | grep <teamId>  (eval didn't finish / wrong version)
│     ├─ index exists, _count 0      → metrics never written (job FAILED upstream)
│     └─ filters exclude everything  → clear/relax global+local filters, recheck
└─ "No data" (single cell)           → that aggregation bucket is empty
```

(Texts and their sources: [03-data-flows.md](03-data-flows.md#dashlet-empty-state-taxonomy-do-not-conflate-these).)

---

## G. Bucket / payload

| Symptom | Root cause | Confirm |
|---|---|---|
| Population Exploration stuck on `population-exploration-processing` | presigned URL host not rewritten → browser hits an in-cluster MinIO host it can't reach; or X-Amz-Expires expired | DevTools: blob request DNS/timeout or **403 SignatureDoesNotMatch** |
| Vis image/blob 0-byte or `net::ERR` | same as above, or object missing in bucket | `mc ls` the expected path (see [05](05-testing-utils.md)) |

---

## Always-correlate checklist (when unsure)

1. `db.jobs.findOne({_id})` — status + failure reason.
2. `kubectl get pods -l jobId=<jobId>` — phase / restart / OOM.
3. `kubectl logs` the failing pod (`--previous` if it restarted).
4. For data issues: ES `_count` on `es_metrics_index` + redis `LLEN`.
5. For status-desync: rabbitmq `feedback` queue + node-server consumer.
6. For UI: the empty-state text + the matching Network/WS frame.
