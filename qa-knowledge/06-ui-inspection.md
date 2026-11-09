# 06 · UI inspection (front-end verification)

How to confirm the web-ui actually rendered what it should — selectors,
auth/transport facts, and how to distinguish "rendered with data" from the several
empty/error states. The web-ui has **no consistent `data-testid` convention**, so
the rules here matter.

---

## Selector strategy (in priority order)

> **The former `TOUR_SELECTORS_ENUM` stable-id scheme is gone.** The Guiding Tour
> feature was removed (`src/tour/` deleted entirely) and with it every DOM `id`
> that enum used to assign — they "existed only as tour targets" and were deleted
> with **no replacement** (confirmed: none of the old id strings — `analytics-dashlet`,
> `population-exploration-dashlet`, `network-tab`, `version-control-pane`, etc. —
> appear anywhere in web-ui any more). For most dashboard/navigation areas there is
> now **no stable selector at all**; fall back to visible text, tooltip `title`, or
> DOM structure/position. The table below reflects actual current state per area.

1. **The few real stable hooks that still exist** — `data-testid="expand-node-details"`
   (`src/layer-details/CodeIntegration.tsx`) and `id="EDIT_PROJECT_FORM"`
   (`FORM_ID` in `src/actions-dialog/ProjectDialogs.tsx`). Use these where applicable.
2. **`aria-label`** — best for version-control row actions and icon buttons (still
   intact; unaffected by the tour removal).
3. **MUI structural attributes** — for DataGridPro tables/grids (`.MuiDataGrid-row`,
   `[data-rowindex]`, etc. — also unaffected).
4. **Visible text / tooltip `title` / DOM position** — the only option left for
   most dashlet and drawer-tab areas (see table). `src/test-ids.ts` is tiny (7 keys)
   and mostly unused; don't trust it wholesale.

### Selector status by area (post Guiding-Tour removal)

| Area | Current selector |
|---|---|
| Network / architecture tab, editor pane, node | **None.** No page-level "Architecture" tab exists any more; drawer content (`src/network-editor/NetworkDrawer.tsx`) is plain text ("node details"/"code"). |
| Code integration panel | `data-testid="expand-node-details"` (`src/layer-details/CodeIntegration.tsx`) — a real, usable hook. |
| Version control | **None.** Fallback: `Tooltip title="Open Version Control"`. |
| Dashboards — add dashboard/dashlet | **None.** Fallback: visible uppercase text "CREATE NEW DASHBOARD" (behind a kebab menu) / "NEW DASHLET". |
| Population Exploration dashlet | **None.** Fallback: dashlet topbar label text / grid position. |
| Sample Analysis dashlet | **None.** No "loaded" state marker exists to wait on; use chart-geometry presence instead. |
| Analytics dashlet | **None.** |
| Insights panel | **None** (plain-text `DrawerSideTab`, `src/dashboard/DashboardDrawer/DashboardDrawer.tsx`). |
| Tests panel | **None** (plain-text tab "tests"). |
| Issues panel | **Not reachable in the live UI at all** — `DrawerTabsEnum.Issues` is defined and still has a content renderer, but is no longer included in the drawer's visible tab list (`drawerFeautures` in `DashboardDrawer.tsx` only lists Insights/Tests/Collections/Reports). Treat "Issues" as currently inaccessible via the UI; flag to a human rather than relying on this doc's old selector. |
| Collections panel | **None** (plain-text tab "collections"). |
| Reports panel *(new)* | Plain-text tab "reports" — a new drawer tab for the Analysis Reports feature, not previously documented (see open gap in GAPS.md). |
| Import/new/edit project dialog | The "import project" dialog no longer exists (only New/Edit/Update project dialogs remain). `EditProjectDialog`'s `aria-labelledby="form-dialog-title"` is a dead reference. Real hook: `id="EDIT_PROJECT_FORM"` (`FORM_ID`, `ProjectDialogs.tsx`). |
| Recent-projects table | **None.** MUI `DataGridPro` with `getRowId` on `cid`; fallback: "RECENT PROJECTS" title text, or MUI's row `data-id={cid}` once the cid is already known. |
| Runs & processes table | **None.** Fallback: tooltip title "Runs & Processes", or MUI row `data-id={jobId}`. |

> **Collection sample-viewer grid:** clicking a collection's **view**/grid icon opens
> a sample grid with its own load path (per-collection ES index → cursor pages →
> virtual scroll → on-demand streaming-vis cache), distinct from a dashlet — see
> [Flow C in 03-data-flows.md](03-data-flows.md#flow-c--collection-sample-viewer-grid).

### `aria-label` selectors (version control)
`Make this the active version`, `Different evaluation generation`,
`Expand experiment` / `Collapse experiment`. (~35 files use `aria-label`.)

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

In-project state is mostly **query-param driven** (the drawer panel is the one
nested-path exception). Verify navigation by reading params:

| Param | Meaning |
|---|---|
| `/project/<projectCid>` | the project |
| `?dashboard=<cid>` | the selected dashboard |
| `.../panel/<tab>` | the open drawer — a **path segment**, not a query param (`tab` ∈ `insights`/`issues`/`tests`/`collections`, `URLS_ENUM.PANEL` in `src/url/url-builder.ts`) |
| `?selected-version=<id>` | the selected model version |
| `?state=…` / `?dashstate=…` | serialized UI state digests |
| `?unlock=<feature>` | one-shot deep link (e.g. from the integration skill) that opens the Free Tier "Contact us" dialog for `<feature>`; `LicenseBlockedDialog` consumes and strips the param on mount (`web-ui/src/trial/licenseBlocked.ts` `UNLOCK_PARAM`) |

---

## Auth & transport facts (critical for any harness)

- **Browser REST auth header is `Authorization: KBearer <jwt>`** — a *custom*
  scheme, **not** standard `Bearer`. node-server branches on the `KBearer` prefix;
  a harness that sends `Bearer` on a browser-style call gets **401 "No token
  provided"**. (`Bearer <apiKey>` is the CLI path.)
- **socket.io auth is in the handshake `auth` payload**, not an HTTP header.
  `auth` is a **callback**, not a static object — it re-runs on every
  (re)connect and fetches a fresh token first, so a reconnect after expiry
  still authenticates: `io(WS_URL,{path:'/socket.io', auth:(cb) =>
  getFreshToken().then((fresh) => cb({Authorization:`KBearer ${fresh}`}))})`
  (`useSocketIO.tsx`). Asserting on a WS *header* will mislead.
- **Single origin**: UI, `/api`, `/auth`, `/session`, `/socket.io` are all the
  same origin (ingress path routing). A cross-origin request is a misconfiguration.
- **REST base path** is `<origin>/api/v2`, derived from `window.location` + the
  injected `<base href>` — not an env var.
- **401** → session-expired dialog. **409** → redirect to `/conflict-users` +
  reload (concurrent-user conflict). **403** → demo-user / license scope failure,
  or (body `code: "LICENSE_LIMIT"`) a Free Tier cap — on Free Tier this renders
  the **`FreeTierBlockedDialog`** ("Contact us": send-a-message / schedule-a-meeting
  tabs) instead of the generic contact-trial dialog (see
  [07-failure-modes.md](07-failure-modes.md)).
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
| `CircularProgress` spinner (no text) | dashletFields (`ChartLoading`) | still loading (transient; `Loading...` text appears only in sample-vis displays) |
| `Sorry, there was an error fetching the visualization's config` | dashletFields | config fetch failed (ES mapping/down) — **error** |
| `Training/Evaluation process is required to visualize data` | dashletFields | no aggregatable/numeric fields — pre-data |
| `Select a version to see data.` | Analytics dashlet | no model version selected — user state |
| `No results found` | MultiCharts (NoDataChart) | ES `getXYChart` returned `charts:[]` |
| `No data` | MultiCharts cell | that cell's aggregation empty |

### Positive assertions (a real render shows)
- The dashlet container has no stable `id`/`data-testid` any more (the old
  `analytics-dashlet`/`population-exploration-dashlet`/
  `sample-analysis-dashlet-loaded-content` ids were tour-only and are gone —
  see the selector-status table above). Locate the container by its dashlet
  topbar label text or grid position instead.
- No `CircularProgress` spinner inside it.
- Chart geometry: SVG/canvas cells, axis labels, plotted series (not the
  `GraphIcon` placeholder).
- For Population Exploration: the processing placeholder (text "processing...")
  is replaced by the scatter map appearing (no `id` to assert on any more —
  use absence of the placeholder text + presence of chart geometry).
- For Population Exploration **re-visualization** (filters/version changed after
  data already rendered): the dashlet keeps the previous scatter map visible
  (dimmed/desaturated canvas) and overlays a small status pill instead of
  reverting to the processing placeholder — text-only, no `id`/`data-testid`:
  "Refreshing population…" → "Updated · just now · `<n>` samples" (auto-hides
  after ~3s) or "Refresh failed" with a "Retry" button on error
  (`ScatterAnalyzerView.tsx`, `usePopulationExploration.ts`). Assert on this
  pill's text via DOM snapshot/accessible-name, not a selector.

### Tie the UI state back to the back-end
- `No results found` → check `GET /_cat/indices | grep <teamId>` and
  `GET <es_metrics_index>/_count`. Missing index or zero count ⇒ the Evaluate/Train
  job didn't write metrics (or filters exclude everything).
- Stuck on the Population Exploration "processing..." placeholder → check the
  population-exploration job and the presigned bucket fetch (see
  [07-failure-modes.md](07-failure-modes.md)).
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
