---
name: update-qa-knowledge
description: Refresh the helm-charts qa-knowledge/ QA knowledge base by diffing the latest source masters (engine, node-server, web-ui, helm-charts, leap-cli, code-loader) against the docs and patching only what drifted, then opening a PR. Use when the user asks to update / refresh / sync / re-verify the QA knowledge base, or after a product or architecture change the docs should reflect. Manual, on-demand maintenance.
---

# Update the QA knowledge base

Manually refresh `qa-knowledge/` so it matches the current code. This is the
on-demand counterpart to the (future) auto-trigger — see
`qa-knowledge/maintenance/MAINTENANCE.md`.

## Preconditions

- Run from inside the **helm-charts** repo (it contains `qa-knowledge/`).
- The source repos must be available as **local siblings**: `../engine`,
  `../node-server`, `../web-ui`, `../leap-cli`, `../code-loader` (helm-charts itself =
  repo root). If any is missing, tell the user and stop — do not guess.
- Bring siblings to the latest master first: `git -C ../<repo> fetch origin master`.
  **Do not switch the user's branches** — read `origin/master` for diffing. Skip the
  fetch if the user says to use the repos as-is.

## Procedure

Follow the verification procedure in **`qa-knowledge/maintenance/prompt.md`**
exactly, with these source paths (no `$KB_SRC_*` env vars in a manual run):

| repo | path |
|---|---|
| helm-charts | `.` (repo root) |
| engine | `../engine` |
| node-server | `../node-server` |
| web-ui | `../web-ui` |
| leap-cli | `../leap-cli` |
| code-loader | `../code-loader` |

In short, per that prompt:
1. For each repo, read `built_from_sha` from `qa-knowledge/maintenance/kb-state.json`
   and compare against `origin/master` HEAD. Use `git diff --name-only
   <sha>..origin/master` when the baseline is an ancestor; otherwise fall back to a
   ~60-day window and treat that repo's docs as full re-verify (note it).
2. Map changed files to impacted docs via `qa-knowledge/maintenance/manifest.json`.
3. Re-verify each impacted doc against the source — **grep the named symbols**, don't
   trust line numbers. Patch only what is actually wrong, with the smallest correct
   edit; update evidence pointers.
4. Bump the per-repo baselines in `kb-state.json` and prepend a dated run entry to
   `qa-knowledge/maintenance/GAPS.md` (what was reviewed, what changed, open gaps).

## Output

- Make the doc edits on a **new kebab-case branch** `qa-kb-update-<helm-charts-short-sha>`
  (never `/` in the branch name).
- Commit (`docs(qa-knowledge): refresh KB against latest masters`), push, and open a
  **PR** for review with `gh pr create`. Do **not** commit to `master`.
- Print a concise summary: impacted docs, what changed, and any open gaps a human
  should resolve.
- If nothing drifted, say so and skip the PR (optionally bump `last_verified` and add
  a "no drift detected" GAPS entry).

## Guardrails

- Touch only files under `qa-knowledge/`. Never edit the source repos.
- Don't invent facts — if a change can't be confirmed from the source, leave the doc
  as-is and log it as an open gap.
- Preserve each doc's structure, tone, and the "corrected mental model" framing.
