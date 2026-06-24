# Keeping the QA knowledge base current

The KB drifts as the product changes. We keep it current with **two layers**, and
**all** automated changes land as a **PR for review** (never a silent commit).

| Layer | Trigger | Cost | Scope |
|---|---|---|---|
| **Scheduled** (primary) | GitHub Action, weekly + manual | out-of-band; never in a normal session | diffs the latest masters, patches impacted docs |
| **Reactive** | a QA agent hits a doc-vs-reality gap during a real run | ~free; only when a gap is actually observed | fixes the specific doc + logs the gap |

We deliberately do **not** diff product-vs-docs on every session — that's what the
scheduled layer is for.

---

## Layer 1 — Scheduled GitHub Action (env-agnostic)

`.github/workflows/qa-kb-maintenance.yml` runs **weekly (Mon 06:00 UTC)** and on
manual dispatch. It:

1. Checks out helm-charts (where the KB lives) and the **latest `master`** of
   `engine`, `node-server`, `web-ui` into `_src/<repo>/`.
2. Diffs each repo since the recorded baseline in
   [`kb-state.json`](kb-state.json), maps changes to docs via
   [`manifest.json`](manifest.json), and runs the agent in
   [`prompt.md`](prompt.md) to patch **only** the impacted docs.
3. Bumps the baseline SHAs and prepends a run entry to [`GAPS.md`](GAPS.md).
4. Opens a PR (`qa-kb-auto-update`) for review.

It takes the **latest masters** and is **not coupled to any local machine** — it
runs entirely in CI.

### One-time setup (required secrets)

Add these as repo (or org) secrets:

| Secret | Purpose |
|---|---|
| `ANTHROPIC_API_KEY` | auth for the Claude Code CLI. (If you standardize on Bedrock/Vertex, set the corresponding `CLAUDE_CODE_USE_*` env + creds in the workflow instead.) |
| `KB_SOURCE_REPOS_TOKEN` | a PAT / fine-grained token with **read** access to `tensorleap/engine`, `tensorleap/node-server`, `tensorleap/web-ui` (these are private; the default `GITHUB_TOKEN` cannot read other repos). PR creation uses the default `GITHUB_TOKEN`. |

Until both secrets exist, the scheduled job simply fails on the run step and does
nothing else — it does not affect any other workflow.

### Knobs
- **Cadence:** edit the `cron:` in the workflow.
- **Full re-verify:** dispatch manually with `force_full: true`.
- **Model:** pin one by adding `--model <id>` to the `claude -p` call (left
  unpinned to avoid staleness).

### First run caveat (node-server)
The KB was built partly from node-server's `en-per-project-generic-workers` branch,
so its baseline SHA is **not on `master`**. On the first scheduled run the agent
cannot do an incremental diff for node-server and will treat node-server-dependent
docs as full re-verify (it says so in the GAPS.md entry), then reset the baseline to
`master` HEAD. Subsequent runs are incremental.

---

## Layer 2 — Reactive in-run fixes (for QA agents)

When you (a QA agent running a test plan) discover that a doc contradicts reality —
a renamed endpoint, a changed selector id, a job that behaves differently:

1. **Fix the specific doc** with the smallest correct edit; update the evidence
   pointer (grep for the current line).
2. **Append a dated entry to [`GAPS.md`](GAPS.md)**: what was wrong, what you
   changed, and the evidence.
3. **Open a PR** with the doc change (branch `qa-kb-reactive-<short-desc>`,
   kebab-case — never `/`). Do not commit directly to the default branch.
4. If you can't confirm the correct value from the source, **don't guess** — log it
   in GAPS.md as an open gap for the scheduled run / a human.

This way gaps that are actually hit get fixed immediately, while the scheduled layer
catches drift in areas no test happened to touch.

---

## The files here

| File | Role |
|---|---|
| [`MAINTENANCE.md`](MAINTENANCE.md) | this doc |
| [`manifest.json`](manifest.json) | doc → source-path-globs dependency map |
| [`kb-state.json`](kb-state.json) | per-repo baseline SHAs the KB was verified against |
| [`prompt.md`](prompt.md) | the maintenance agent's instructions (used by the Action and the local script) |
| [`GAPS.md`](GAPS.md) | append-only run log + open gaps |
| [`run-local.sh`](run-local.sh) | optional: run the same flow on your machine against sibling repos |

**Extending:** when you add a doc to `qa-knowledge/`, add it to `manifest.json` with
the source globs it depends on, so the maintenance layer keeps it in sync.

---

## Local fallback

If you prefer to run it on your machine (e.g. before the secrets are set up), use
[`run-local.sh`](run-local.sh). It expects the source repos as siblings of
helm-charts (`../engine`, `../node-server`, `../web-ui`), runs the same agent
against your local `claude`, and opens a PR via `gh`. The GitHub Action remains the
recommended path because it always uses fresh masters and needs no local checkout.
