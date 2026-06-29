# 06 · UI inspection (front-end verification)

How to confirm the web-ui actually rendered what it should — selectors,
auth/transport facts, and how to distinguish "rendered with data" from the several
empty/error states. The web-ui has **no consistent `data-testid` convention**, so
the rules here matter.

---

## Selector strategy (in priority order)

1. **Element `id` from `TOUR_SELECTORS_ENUM`** (`web-ui/src/tour/ToursConfig.tsx`).
   These are the de-facto stable hooks (the product tour resolves them via
   `document.getElementById`). Prefer these for everything dashboard/navigation.
2. **`aria-label`** — best for version-control row actions and icon buttons.
3. **MUI structural attributes** — for DataGridPro tables/grids.
4. **Ad-hoc `data-testid`** — only the handful that are actually applied (below).
   `src/test-ids.ts` is **mostly dead**; do not trust it wholesale.

### Stable `id` selectors (high-value subset)

| Area | `id` |
|---|---|
| Network / architecture | `network-tab`, `network-editor-pane`, `network-node`, `code-integration-panel` |
| Version control | `version-control-pane`, `version-control-expand-button` |
| Dashboards | `add-new-dashboard-button`, `add-new-dashboard-dialog`, `add-new-dashlet-button`, `add-new-dashlet-menu` |
| Population Exploration | `add-population-exploration-dashlet-button`, `population-exploration-dashlet`, `population-exploration-processing`, `population-exploration-visualize-button`, `population-exploration-color-by`, `population-exploration-size-by` |
| Sample Analysis | `sample-analysis-dashlet`, `sample-analysis-dashlet-loaded-content` |
| Analytics | `analytics-dashlet` |
| Insights | `insights-panel-button`, `insights-list`, `insights-categories-tabs`, `insight-card` |
| Tests | `tests-panel-button`, `test-list`, `create-new-test-button` |
| Issues | `issues-panel-button`, `create-new-issue-button` |
| Collections | `collections-panel-button` |
| Misc | `hub-gallery`, `import-project-dialog`, `recent-projects-table-id`, `run-and-processes-table-id` |

> **Collection sample-viewer grid:** clicking a collection's **view**/grid icon opens
> a sample grid with its own load path (per-collection ES index → cursor pages →
> virtual scroll → on-demand streaming-vis cache), distinct from a dashlet — see
> [Flow C in 03-data-flows.md](03-data-flows.md#flow-c--collection-sample-viewer-grid).

### `aria-label` selectors (version control)
`Make this the active version`, `Different evaluation generation`,
`Expand experiment` / `Collapse experiment`. (~25 files use `aria-label`.)

### MUI DataGridPro (dashlet tables, projects table, running dialog)
Target `.MuiDataGrid-row`, `[role="row"]`, `[data-rowindex]`, `[data-field]`,
`role="gridcell"`. Used in `ProjectsTable`, `ProTable`, `RunningDialog`,
`SampleVisDisplay`.

### Actually-applied `data-testid`s
`editor-file-name-row`, `editor-rename-file-input`,
`custom-visualization-filter-bar-expand`, `save-changes-button`,
`discard-changes-button`, `edit-button`, `full-screen-button`,
`expand-node-details`. (Everything else in `test-ids.ts` is unused.)

---

## URL / navigation assertions

In-project state is **query-param driven**, not nested paths. Verify navigation by
reading params, not the path:

| Param | Meaning |
|---|---|
| `/project/<projectCid>` | the project |
| `?dashboard=<cid>` | the selected dashboard |
| `?panel=<DrawerTab>` | the open drawer (Tests/Insights/Issues/Collections) |
| `?selected-version=<id>` | the selected model version |
| `?state=…` / `?dashstate=…` | serialized UI state digests |

---

## Auth & transport facts (critical for any harness)

- **Browser REST auth header is `Authorization: KBearer <jwt>`** — a *custom*
  scheme, **not** standard `Bearer`. node-server branches on the `KBearer` prefix;
  a harness that sends `Bearer` on a browser-style call gets **401 "No token
  provided"**. (`Bearer <apiKey>` is the CLI path.)
- **socket.io auth is in the handshake `auth` payload**, not an HTTP header:
  `io(WS_URL,{path:'/socket.io', auth:{Authorization:'KBearer <token>'}})`.
  Asserting on a WS *header* will mislead.
- **Single origin**: UI, `/api`, `/auth`, `/session`, `/socket.io` are all the
  same origin (ingress path routing). A cross-origin request is a misconfiguration.
- **REST base path** is `<origin>/api/v2`, derived from `window.location` + the
  injected `<base href>` — not an env var.
- **401** → session-expired dialog. **409** → redirect to `/conflict-users` +
  reload (concurrent-user conflict). **403** → demo-user / license scope failure.
- **Auth mode** can be **Keycloak** or **Local** (`demo@demo.ai`, no IdP) — check
  `getAuthProvider()` before debugging an "auth loop". First-ever user is sent to
  **register**, not login.

---

## Proving a dashlet rendered *with data*

A dashlet passes through layered states. Decide pass/fail by combining a
**negative** assertion (none of the empty/error markers) with a **positive** one
(chart geometry present).

### Negative assertions (none of these texts should be present for a "has data" pass)

| Text | Source stage | What it means |
|---|---|---|
| `Loading...` | dashletFields | still loading (transient) |
| `Sorry, there was an error fetching the visualization's config` | dashletFields | config fetch failed (ES mapping/down) — **error** |
| `Training/Evaluation process is required to visualize data` | dashletFields | no aggregatable/numeric fields — pre-data |
| `Select a version to see data.` | Analytics dashlet | no model version selected — user state |
| `No results found` | MultiCharts (NoDataChart) | ES `getXYChart` returned `charts:[]` |
| `No data` | MultiCharts cell | that cell's aggregation empty |

### Positive assertions (a real render shows)
- The dashlet's container `id` (`analytics-dashlet`,
  `population-exploration-dashlet`, `sample-analysis-dashlet-loaded-content`, …) is present.
- No `CircularProgress` spinner inside it.
- Chart geometry: SVG/canvas cells, axis labels, plotted series (not the
  `GraphIcon` placeholder).
- For Population Exploration: it transitions
  `population-exploration-processing` → `population-exploration-dashlet`
  (the scatter map appears).

### Tie the UI state back to the back-end
- `No results found` → check `GET /_cat/indices | grep <teamId>` and
  `GET <es_metrics_index>/_count`. Missing index or zero count ⇒ the Evaluate/Train
  job didn't write metrics (or filters exclude everything).
- Stuck on `population-exploration-processing` → check the population-exploration
  job and the presigned bucket fetch (see [07-failure-modes.md](07-failure-modes.md)).
- Dashlet doesn't refresh after an Evaluate completes → suspect the
  RabbitMQ→socket.io path (no `serverMessage` frame), not the chart code.

---

## Network/console signals worth capturing

When inspecting in a browser (DevTools or an automation driver):
- **Network**: `POST /api/v2/.../getXYChart` → 200 with `charts[].length>0` for a
  data render; a `/socket.io` request upgrading to **101**; presigned bucket GETs
  carrying `X-Amz-Signature`.
- **WS frames**: `authenticated` on connect; `serverMessage` for live updates;
  `authentication_error` on bad token.
- **Console**: `Error initializing auth` (Keycloak init failure);
  `authentication_error` (socket).
- **RUM**: actions are tagged with `data-track-action` (e.g.
  `train-dialog-submitted`) and visible in Datadog RUM under service `web-ui` (PROD).

---

## Tooling note

This repo's CLI is non-interactive, so live UI inspection is done with a browser
automation tool (the environment exposes browser/preview MCP tools). The selector
and state rules above are tool-agnostic — apply them whether you drive Chrome,
Playwright, or read a DOM snapshot. Always pair a UI assertion with the
corresponding back-end observable from [05-testing-utils.md](05-testing-utils.md);
a green UI over stale/empty data is the classic false pass.
