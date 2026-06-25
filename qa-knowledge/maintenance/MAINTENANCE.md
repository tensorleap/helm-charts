# Keeping the QA knowledge base current

The KB drifts as the product changes. We keep it current with an **on-demand manual
refresh** plus **reactive in-run fixes**. All automated edits land as a **PR for
review** (never a silent commit). A scheduled/auto trigger is intentionally left for
**later** — the same logic underneath will power it.

| Layer | Trigger | When |
|---|---|---|
| **Manual refresh** (primary) | the `update-qa-knowledge` skill | on demand, after a product/architecture change |
| **Reactive** | a QA agent hits a doc-vs-reality gap during a real run | whenever a gap is actually observed |
| **Auto** (planned, not enabled) | cron / GitHub Action wrapping the same procedure | later, once the manual flow is trusted |

We deliberately do **not** diff product-vs-docs on every session.

---

## Layer 1 — Manual refresh: the `update-qa-knowledge` skill

Run the **`update-qa-knowledge`** skill (`.claude/skills/update-qa-knowledge/`) from
the helm-charts repo whenever you want to bring the docs back in line with the code
(e.g. after merging a feature that changes a job, endpoint, selector, or topology).
It:

1. Reads the baselines in [`kb-state.json`](kb-state.json) and the latest masters of
   the source repos (local siblings `../engine`, `../node-server`, `../web-ui`,
   `../leap-cli`, `../code-loader`; helm-charts = repo root).
2. Diffs each repo since its baseline, maps changes to docs via
   [`manifest.json`](manifest.json), and re-verifies **only the impacted docs** using
   the procedure in [`prompt.md`](prompt.md).
3. Patches only what drifted, bumps the baselines, and prepends a run entry to
   [`GAPS.md`](GAPS.md).
4. Opens a PR (`qa-kb-update-*`) for review.

**Preconditions:** the source repos must be checked out as **siblings** of
helm-charts (`../engine`, `../node-server`, `../web-ui`, `../leap-cli`,
`../code-loader`). If a sibling is missing, the skill stops and tells you.

Invoke it in Claude Code with `/update-qa-knowledge` (or just ask "update the QA
knowledge base").

### First run caveat (node-server) — resolved 2026-06-25
The KB was originally built partly from node-server's `en-per-project-generic-workers`
branch. That branch has since merged to `master`; the baseline was reconciled to
node-server `master` (HEAD `9276bb7c`) and the affected doc (`09-job-catalog.md`)
updated — see [`GAPS.md`](GAPS.md). node-server now diffs incrementally like the other
repos. (`leap-cli` + `code-loader` still carry null baselines — their first refresh
full-verifies; see [`kb-state.json`](kb-state.json).)

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
   in GAPS.md as an open gap for the next manual refresh / a human.

This way gaps that are actually hit get fixed immediately, while the manual refresh
catches drift in areas no test happened to touch.

---

## Future — auto-trigger (not enabled now, by request)

When the manual flow is trusted, wrap the **same** procedure in a cron or a
manual-dispatch GitHub Action so it runs without a local checkout, against fresh
masters. The building blocks are already here:

- [`prompt.md`](prompt.md) — the verification logic; honors `$KB_SRC_*` env paths so
  it works both in a manual/sibling run and a CI checkout.
- [`run-local.sh`](run-local.sh) — a headless `claude -p` runner that does the same
  against sibling repos (the basis for a scripted/CI invocation).

An Action would: check out the source repos, set `$KB_SRC_*`, run `prompt.md`, and
open a PR. It would need secrets `ANTHROPIC_API_KEY` and a token with **read** access
to `tensorleap/engine`, `tensorleap/node-server`, `tensorleap/web-ui`,
`tensorleap/leap-cli`, `tensorleap/code-loader` (the default `GITHUB_TOKEN` can't read
other repos). We are **not** enabling it yet.

---

## The files here

| File | Role |
|---|---|
| [`MAINTENANCE.md`](MAINTENANCE.md) | this doc |
| [`manifest.json`](manifest.json) | doc → source-path-globs dependency map |
| [`kb-state.json`](kb-state.json) | per-repo baseline SHAs the KB was verified against |
| [`prompt.md`](prompt.md) | the verification procedure (used by the skill and any future auto-trigger) |
| [`GAPS.md`](GAPS.md) | append-only run log + open gaps |
| [`run-local.sh`](run-local.sh) | headless `claude -p` runner (building block for the future auto-trigger) |
| `.claude/skills/update-qa-knowledge/` | the manual-refresh skill |

**Extending:** when you add a doc to `qa-knowledge/`, add it to `manifest.json` with
the source globs it depends on, so the refresh keeps it in sync.
