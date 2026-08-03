# Production Monitor — install this version (demo build)

One-page guide for installing the production-monitor demo build on a machine
with Docker. Verified end-to-end on 2026-08-02.

## What this branch pins

| component | branch | commit | image |
| --- | --- | --- | --- |
| helm-charts (installer + charts) | `feature-production-monitor-flag` | this branch | — |
| node-server | `feature-production-monitor` | `4e7d7e93` | `public.ecr.aws/tensorleap/node-server:feature-production-monitor-4e7d7e93` (on ECR, CI-built) |
| web-ui | `feature-production-monitor-mode` | `5ab74512a` | `public.ecr.aws/tensorleap/web-ui:feature-production-monitor-mode-5ab74512` (CI-built, on ECR) |

The chart values in this branch already pin both image tags — no flags needed
beyond `--production-monitor`.

## 1. Install

```bash
cd helm-charts && git checkout feature-production-monitor-flag
go run . install --local --production-monitor --yes
```

That is the whole install. Both images come from public ECR — nothing to
build, load or import. `--local` uses this checkout's charts (which pin the
two feature tags); `--production-monitor` is sticky across later `upgrade`s.

Requires Docker running, and ~8 GB free for the k3d cluster. On an existing
install the same command upgrades in place.

## 2. Open it

http://localhost:4589 → register / sign in → the whole UI is the production
monitor. It needs a project with at least one **evaluated** version: push +
evaluate with the leap CLI as usual, and the monitor picks the latest
evaluated version as the deployment automatically.

## Notes

- The branch carries one **demo-only commit** (`DEMO ONLY: alternate Failure
  Mode insights as Out Of Distribution`) — revert it before any merge to
  master.
- The web-ui image only exists because the branch **vendors** the api-client
  (`vendor/tensorleap-api-client-11.0.129.tgz`): the published 11.0.129 was
  generated from node-server master and lacks every monitor API, so CI could
  not build the image until the tarball was committed. Replace it with a
  published client before merging (web-ui README checklist).
- Verify the flag landed: `kubectl get cm -n tensorleap
  tensorleap-node-server-env-configmap -o yaml | grep PRODUCTION_MONITOR`
  → `"true"`.
- Full status / architecture / open items: web-ui
  `src/production-monitor/README.md` on the web-ui branch.
