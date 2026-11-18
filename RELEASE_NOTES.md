# Release Notes - v1.6.114

**Release Date:** 2026-11-18
**Total Changes:** 33

---

## ✨ New Features & Improvements

- [EN-2635](https://tensorleap.atlassian.net/browse/EN-2635): Allow adding comments to completed DS curation flow runs
- [EN-2591](https://tensorleap.atlassian.net/browse/EN-2591): Implement error bar toggle for visualisations
- [EN-2589](https://tensorleap.atlassian.net/browse/EN-2589): Enable user-defined names and sections for insight generation jobs
- [EN-2579](https://tensorleap.atlassian.net/browse/EN-2579): Add schemaVersion to leap run info

## 🐛 Bug Fixes

- [EN-2643](https://tensorleap.atlassian.net/browse/EN-2643): "Edit" and "Delete" tooltips hover awkwardly over the date in Secret Manager
- [EN-2642](https://tensorleap.atlassian.net/browse/EN-2642): "All" filter in Runs & Processes shows fewer versions and runs than "Current"
- [EN-2641](https://tensorleap.atlassian.net/browse/EN-2641): Generating insights from filter prompts sample/instance depression when no selected model has an Instance Latent space
- [EN-2640](https://tensorleap.atlassian.net/browse/EN-2640): Orchestrator counts engine warmup pods as infra, causing ledger to underestimate available memory and unnecessary job queuing
- [EN-2639](https://tensorleap.atlassian.net/browse/EN-2639): Fetch similar is not working even though there are no filters in the dashboard
- [EN-2638](https://tensorleap.atlassian.net/browse/EN-2638): DS curation dialog opens with broken model
- [EN-2637](https://tensorleap.atlassian.net/browse/EN-2637): PR description skill generates text that is too verbose to review
- [EN-2636](https://tensorleap.atlassian.net/browse/EN-2636): Filters leak between consecutive "generate insight from filter" jobs
- [EN-2633](https://tensorleap.atlassian.net/browse/EN-2633): UI does not re-check license after initial load, allowing gated actions after license expiry
- [EN-2632](https://tensorleap.atlassian.net/browse/EN-2632): Long version name looks bad on drawer
- [EN-2631](https://tensorleap.atlassian.net/browse/EN-2631): Failure Mode drawer gives no indication when Evaluate / Regenerate Failure Modes is running
- [EN-2630](https://tensorleap.atlassian.net/browse/EN-2630): Recommendations shown when there are no results, and previous run gives no indication it returned nothing
- [EN-2628](https://tensorleap.atlassian.net/browse/EN-2628): Web UI re-downloads ~10.8 MB main bundle on every page load (Cache-Control: max-age=0)
- [EN-2627](https://tensorleap.atlassian.net/browse/EN-2627): Synthetic data dialog: loader stuck forever when adding a dataset filter
- [EN-2626](https://tensorleap.atlassian.net/browse/EN-2626): Console errors printed when dragging and dropping below the legend
- [EN-2625](https://tensorleap.atlassian.net/browse/EN-2625): Deleted samples still appear in collection while deletion is processing on the server
- [EN-2624](https://tensorleap.atlassian.net/browse/EN-2624): Regression in 1.6.100: pop. explore LS list in instance projects shows both image and instance latent spaces together
- [EN-2623](https://tensorleap.atlassian.net/browse/EN-2623): UI: metadata string truncated too early with excess padding before cluster metric
- [EN-2622](https://tensorleap.atlassian.net/browse/EN-2622): New report tab is not working
- [EN-2621](https://tensorleap.atlassian.net/browse/EN-2621): Version control text is not centered for new projects
- [EN-2617](https://tensorleap.atlassian.net/browse/EN-2617): Improve contrast colors on top panel info dialog
- [EN-2615](https://tensorleap.atlassian.net/browse/EN-2615): Expired license still allows most compute operations (server checks expiry only on evaluate)
- [EN-2602](https://tensorleap.atlassian.net/browse/EN-2602): Fix metadata property value display in insights top panel
- [EN-2590](https://tensorleap.atlassian.net/browse/EN-2590): Adjust y-axis scale for individual plots
- [EN-2588](https://tensorleap.atlassian.net/browse/EN-2588): Fix Dashlet hover behavior for model highlighting
- [EN-2587](https://tensorleap.atlassian.net/browse/EN-2587): Update dashlet colour palette to improve differentiation
- [EN-2573](https://tensorleap.atlassian.net/browse/EN-2573): Fix Failure Mode top panel graph resizing and text truncation
- [EN-2571](https://tensorleap.atlassian.net/browse/EN-2571): Fix metric selection in Blueriver insight settings
- [EN-2558](https://tensorleap.atlassian.net/browse/EN-2558): Dataset curation table- delete Previous Recommendations from table is not terminating the job
