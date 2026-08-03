# Production Monitor — install this version (demo build)

One-page guide for installing the production-monitor demo build on a machine
with Docker. Verified end-to-end on 2026-08-02.

## What this branch pins

| component | branch | commit | image |
| --- | --- | --- | --- |
| helm-charts (installer + charts) | `feature-production-monitor-flag` | this branch | — |
| node-server | `feature-production-monitor` | `4e7d7e93` | `public.ecr.aws/tensorleap/node-server:feature-production-monitor-4e7d7e93` (on ECR, CI-built) |
| web-ui | `feature-production-monitor-mode` | `7f54cf77` | `public.ecr.aws/tensorleap/web-ui:feature-production-monitor-mode-7f54cf77` (**NOT on ECR** — see step 1) |

The chart values in this branch already pin both image tags — no flags needed
beyond `--production-monitor`.

## 1. Get the web-ui image (one-time)

CI cannot build this web-ui branch yet: the published `@tensorleap/api-client`
11.0.129 was generated from node-server **master** and lacks the monitor APIs,
so the image must be built from a checkout that has the branch-built client.

**Option A — load a prebuilt tar** (fastest; ask whoever last built it):

```bash
docker load -i web-ui-feature-production-monitor.tar
```

**Option B — build it yourself:**

```bash
# 1. node-server branch generates the correct api-client tarball
cd node-server && git checkout feature-production-monitor && npm ci && npm run generate-client

# 2. build the web-ui bundle against it
cd ../web-ui && git checkout feature-production-monitor-mode && npm ci
npm i --no-save ../node-server/generated/client/tensorleap-api-client-11.0.129.tgz
npm run build && cp build/index.html build/404.html

# 3. wrap it in the router image with the exact pinned tag
mkdir -p /tmp/webui-img && cp -r build LICENSE.md /tmp/webui-img/
cat > /tmp/webui-img/Dockerfile <<'DOCKER'
FROM public.ecr.aws/tensorleap/web-ui-router:latest-amd
COPY LICENSE.md /LICENSE.md
COPY build /public
DOCKER
docker build -t public.ecr.aws/tensorleap/web-ui:feature-production-monitor-mode-7f54cf77 /tmp/webui-img
```

To hand the image to someone else: `docker save -o web-ui-feature-production-monitor.tar public.ecr.aws/tensorleap/web-ui:feature-production-monitor-mode-7f54cf77`

## 2. Install

```bash
cd helm-charts && git checkout feature-production-monitor-flag
go run . install --local --production-monitor --yes
```

`--local` uses this checkout's charts (with the pinned tags); the
`--production-monitor` flag is sticky across future `upgrade`s.

## 3. Import the web-ui image into the cluster

The node-server image pulls from ECR; the web-ui image must be imported from
your Docker daemon (do this right after the install finishes — on a fresh
machine the web-ui pod will sit in `ImagePullBackOff` until you do):

```bash
docker save public.ecr.aws/tensorleap/web-ui:feature-production-monitor-mode-7f54cf77 \
  | docker exec -i k3d-tensorleap-server-0 ctr images import -
kubectl delete pod -n tensorleap -l app.kubernetes.io/name=web-ui   # only if it was ImagePullBackOff
```

## 4. Open it

http://localhost:4589 → register / sign in → the whole UI is the production
monitor. It needs a project with at least one **evaluated** version: push +
evaluate with the leap CLI as usual, and the monitor picks the latest
evaluated version as the deployment automatically.

## Notes

- The branch carries one **demo-only commit** (`DEMO ONLY: alternate Failure
  Mode insights as Out Of Distribution`) — revert it before any merge to
  master.
- Verify the flag landed: `kubectl get cm -n tensorleap
  tensorleap-node-server-env-configmap -o yaml | grep PRODUCTION_MONITOR`
  → `"true"`.
- Full status / architecture / open items: web-ui
  `src/production-monitor/README.md` on the web-ui branch.
