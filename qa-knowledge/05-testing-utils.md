# 05 · Testing utilities (cluster & back-end cookbook)

The commands a senior QA engineer runs to observe a live Tensorleap install. All
examples target namespace **`tensorleap`** and context **`k3d-tensorleap`**.

> **kubectl without a separate install:** the `leap` binary bundles one —
> `leap server tools <kubectl args>` (pre-wired to `k3d-tensorleap`), and
> `leap server tools k3d ...` for the k3d CLI. If you have your own `kubectl`,
> add `--context k3d-tensorleap -n tensorleap`. The examples below use plain
> `kubectl -n tensorleap`; substitute `leap server tools` if no kubeconfig is
> merged.

---

## Cluster & install state

```bash
# Is the cluster up? (k3d)
leap server tools k3d cluster list                 # expect cluster "tensorleap" running
docker ps | grep k3d-tensorleap                    # k3d nodes are docker containers

# Installer + data dir
leap server info                                   # installer version, data dir

# Everything in the namespace
kubectl -n tensorleap get all
kubectl -n tensorleap get pods -o wide
kubectl -n tensorleap get events --sort-by=.lastTimestamp | tail -40

# Chart versions actually deployed
kubectl -n tensorleap get deploy tensorleap-node-server -o jsonpath='{.spec.template.spec.containers[0].image}{"\n"}'
helm -n tensorleap list                            # tensorleap-infra + tensorleap releases
```

`leap server check` is a **stub** (prints only "Check command") — do not use it to
validate health.

---

## Core pod inspection

```bash
# Watch pods (great during a job run)
kubectl -n tensorleap get pods -w

# Logs (follow / previous crash / specific container)
kubectl -n tensorleap logs <pod> --follow
kubectl -n tensorleap logs <pod> --previous            # last crash
kubectl -n tensorleap logs deploy/tensorleap-node-server
kubectl -n tensorleap logs <pod> -c <container>

# Why is a pod not Running?
kubectl -n tensorleap describe pod <pod>               # Events: scheduling, OOM, ImagePull
kubectl -n tensorleap get pod <pod> -o jsonpath='{.status.containerStatuses[*].state}{"\n"}'

# Resources actually applied (auto-settings result)
kubectl -n tensorleap get pod <pod> -o jsonpath='{range .spec.containers[*]}{.name}{": "}{.resources}{"\n"}{end}'

# Env on a pod (verify JOB_PAYLOAD, REDIS_HOST, GPU vars, topics)
kubectl -n tensorleap exec <pod> -- env | sort
```

### Find the pods/resources of a specific job
```bash
JOB=<jobId>
kubectl -n tensorleap get pods,deploy,svc -l jobId=$JOB
kubectl -n tensorleap get pods -l jobType=generic-process       # engine-generics
kubectl -n tensorleap get pods -l jobType=redis                 # per-job redis
kubectl -n tensorleap get pods -l jobType=streaming-handler
kubectl -n tensorleap get jobs   -l projectId=<projectId>
```

(Full selector table: [04-job-types-and-lifecycle.md](04-job-types-and-lifecycle.md).)

---

## Port-forward map

```bash
kubectl -n tensorleap port-forward svc/tensorleap-node-server 4000:80   # REST /api/v2
kubectl -n tensorleap port-forward svc/tl-elasticsearch-es-master 9200:9200
kubectl -n tensorleap port-forward svc/mongodb 27017:27017
kubectl -n tensorleap port-forward svc/rabbitmq 15672:15672             # mgmt UI (guest/guest)
kubectl -n tensorleap port-forward svc/tensorleap-minio 9001:9001       # console
```

The app itself is reachable at **http://localhost:4589** (host port → ingress).

---

## node-server health & API

```bash
# Health (probe target): 200 healthy, 503 if mongo/elastic/rabbitmq down
curl -s http://localhost:4589/api/v2/monitor/healthCheck | jq

# Authenticated call as the CLI does (Bearer = API key)
curl -s -H "Authorization: Bearer $LEAP_API_KEY" \
     http://localhost:4589/api/v2/projects/getProjects -X POST -d '{}' | jq

# Browser-style auth uses KBearer (NOT Bearer) — see 06-ui-inspection.md
```

---

## Elasticsearch (dashlet data lives here)

```bash
ES=http://localhost:9200    # after port-forward; security disabled, anonymous superuser
curl -s $ES/_cluster/health | jq '.status'             # green/yellow/red
curl -s "$ES/_cat/indices?v"                           # list indices
curl -s "$ES/_cat/indices?v" | grep <projectId>        # this project's metrics indices

# Count docs in a version's metrics index (rises during Evaluate)
curl -s "$ES/<es_metrics_index>/_count" | jq '.count'

# Inspect a sample doc / mapping (debug "no aggregatable fields")
curl -s "$ES/<es_metrics_index>/_mapping" | jq
curl -s "$ES/<es_metrics_index>/_search?size=1" | jq '.hits.hits[0]._source'

# ECK CR health (the operator reconciles the StatefulSet)
kubectl -n tensorleap get elasticsearch tl-elasticsearch
```

To find `<es_metrics_index>`: read the version doc in Mongo
(`db.versions.findOne({_id:...},{resources:1})` → `resources.es_metrics_index`).
**Empty dashlet → first check this index exists and has docs.**

---

## MongoDB (entities)

```bash
# Shell in (no auth on default install)
kubectl -n tensorleap exec -it deploy/mongodb -- mongosh tensorleap
# or after port-forward:  mongosh mongodb://localhost:27017/tensorleap
```
```javascript
show collections
db.jobs.find({}, {type:1, subType:1, status:1, versionId:1}).sort({_id:-1}).limit(10)
db.jobs.findOne({_id: ObjectId("<jobId>")})            // status, failure reason
db.versions.findOne({_id: ObjectId("<versionId>")}, {resources:1, evaluateParams:1})
db.users.findOne({"local.email":"<email>"})            // 401 "User not found" debugging
db.projects.find({}, {name:1}).limit(20)
db.dashboards.findOne({_id: ObjectId("<id>")})         // dashlet CONFIG only (no chart data)
```

Scope indexes are unique compound keys (`{cid}`, `{cid,teamId}`,
`{cid,teamId,projectId}`) — a duplicate-key write error usually means a scope
collision, not corruption.

---

## Per-job Redis (the engine dataflow)

Redis exists **only while the job runs**, one pod per job.

```bash
JOB=<jobId>
# Exec into the per-job redis (or port-forward its pod)
kubectl -n tensorleap exec -it redis-$JOB -- redis-cli PING        # expect PONG

# Queue depths — the heartbeat of an Evaluate
kubectl -n tensorleap exec redis-$JOB -- redis-cli LLEN dataset_training_to_generate_$JOB
kubectl -n tensorleap exec redis-$JOB -- redis-cli LLEN dataset_training_${JOB}_ready
kubectl -n tensorleap exec redis-$JOB -- redis-cli LLEN metrics_$JOB
kubectl -n tensorleap exec redis-$JOB -- redis-cli LLEN streaming_evaluate_${JOB}_queue
kubectl -n tensorleap exec redis-$JOB -- redis-cli GET  generic_process_ratio_$JOB    # 0..1
kubectl -n tensorleap exec redis-$JOB -- redis-cli GET  memory_update:generic:$JOB
kubectl -n tensorleap exec redis-$JOB -- redis-cli INFO memory | grep used_memory     # near maxmemory ⇒ OOM risk
```

Interpretation:
- `to_generate` high + `ready` ~0 → engine-generics not keeping up (customer code
  slow or failing).
- `streaming_*_queue` stays high → streaming-handler backpressure (watch its replica count).
- `used_memory` near `maxmemory` with `noeviction` → redis will OOM (sizing too small).

(Queue name reference: [02-components.md](02-components.md#per-job-redis--its-5-logical-queues).)

---

## MinIO bucket (`session`)

```bash
# Console UI
kubectl -n tensorleap port-forward svc/tensorleap-minio 9001:9001   # creds foobarbaz / foobarbazqux

# CLI via mc inside the pod (or your own mc against the port-forward)
kubectl -n tensorleap exec -it deploy/tensorleap-minio -- sh
#   mc alias set local http://localhost:9000 foobarbaz foobarbazqux
#   mc ls -r local/session/organizations/<teamId>/projects/<projectId>/
```

What lives where (paths a QA engineer checks):
- Job payload JSON: `organizations/<team>/jobs/<jobId>/...`
- Pods settings: `organizations/<team>/projects/<proj>/pods-settings/<versionId>/k8s_pods_settings.json`
- Latent space (Evaluate): under the version's `inference_artifact_id` dir
- Vis payloads/images: project dir, served to the browser via `/session`

---

## RabbitMQ (engine ↔ node-server feedback)

```bash
kubectl -n tensorleap port-forward svc/rabbitmq 15672:15672    # http://localhost:15672 guest/guest
```
- Queue **`feedback`** (durable) = engine/orchestrator → node-server. A growing
  depth with no consumer means node-server isn't consuming (job status stuck).
- Per-job control queue `job-control-channel-<jobId>` = node-server → engine
  (stop/terminate).
- If status updates stop flowing to the UI after a job completes, suspect RabbitMQ
  (consumer absent / connection lost) before blaming socket.io.

---

## Zot registry (custom dependency images)

```bash
REG=http://127.0.0.1:5699           # default host port; in-cluster zot is :5000
curl -s $REG/v2/_catalog | jq                      # repos
curl -s $REG/v2/<repo>/tags/list | jq              # tags
curl -sI $REG/v2/<repo>/manifests/<tag>            # HEAD → digest
```
pippin (the dependency builder init container) pushes the customer's custom
generic image here; an Evaluate that hangs in the pippin init stage is a registry
or build problem — check the init-container logs and this catalog.

---

## docker / k3d / containerd

```bash
docker ps                                          # k3d-tensorleap-server-0 etc.
docker stats --no-stream                           # host-level CPU/mem pressure (OOM context)
docker exec -it k3d-tensorleap-server-0 sh         # into the k3d node
#   crictl ps ; crictl images ; crictl logs <id>   # containerd inside the node
leap server tools k3d kubeconfig merge tensorleap  # (re)merge kubeconfig
```

---

## Image / version provenance

- `images.txt` (repo root) is the generated list of every image the charts
  reference; regenerate with `make update-images`, validate with
  `make validate-images`.
- `*-latest-image` files at repo root hold the stable tags for engine,
  node-server, web-ui, pippin (updated by the `Update images` workflow).
- To prove a pod runs the expected build:
  `kubectl -n tensorleap get pod <pod> -o jsonpath='{.spec.containers[*].image}'`
  and compare to `images.txt`.

---

## Existing automated tests (to reuse fixtures/patterns, not e2e UI)

| Repo | Stack | Where |
|---|---|---|
| node-server | Jest unit + integration | `tests/unit/**`, `tests/integration/{login,enginemessages,disableAuth}.test.ts`; `make test` |
| engine | pytest | `test/test_unit/**`, `test/test_integration/workers/test_worker*.py` (incl. streaming-handler, metricsrunner, push) |
| helm-charts | Go | `pkg/**/*_test.go`; `make test` |
| leap-cli | Go | `pkg/code/utils_test.go`, `pkg/model/insights_test.go`, `pkg/version/compat_test.go` |
| web-ui | Jest (`@testing-library`) + Storybook/Chromatic | no Cypress/Playwright e2e found |
| code-loader | pytest | `tests/test_dataset_loader*.py` |

> No browser-driven e2e suite exists in `web-ui` today. Full
> push→eval→dashboard verification is the QA agent's job, using
> [06-ui-inspection.md](06-ui-inspection.md) for the front-end and the commands
> above for the back-end. (Confirm whether an external e2e repo exists for the
> build under test.)
