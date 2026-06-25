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

## 2026-06-24 — initial
Reviewed: KB authored from helm-charts@8cbe68cb, engine@9045cc33,
node-server@en-per-project-generic-workers (1b44e3bb), web-ui@c0afae07.
Changed: all docs created (README, 01–10) + maintenance scaffolding.
Open gaps:
- **node-server baseline is a feature branch** (`en-per-project-generic-workers`),
  not `master`. The first `update-qa-knowledge` refresh should treat
  node-server-dependent docs as full re-verify and reset the baseline to `master` HEAD.
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
