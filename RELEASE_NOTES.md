# Release Notes - v1.6.92

**Release Date:** 2026-10-07
**Total Changes:** 29

---

## ✨ New Features & Improvements

- [EN-2528](https://tensorleap.atlassian.net/browse/EN-2528): Implement group archiving for filter insights drawer
- [EN-2505](https://tensorleap.atlassian.net/browse/EN-2505): Implement 'Forgot my password' flow
- [EN-2489](https://tensorleap.atlassian.net/browse/EN-2489): Update insight option selection to allow deselection on click
- [EN-2480](https://tensorleap.atlassian.net/browse/EN-2480): Enable metadata search in Top Panel table
- [EN-2474](https://tensorleap.atlassian.net/browse/EN-2474): Add notification for generate-insights-from-filter when no insights are found
- [EN-2473](https://tensorleap.atlassian.net/browse/EN-2473): Refactor progress bar to use a single shared counter
- [EN-2550](https://tensorleap.atlassian.net/browse/EN-2550): Move general settings to top bar panel
- [EN-2537](https://tensorleap.atlassian.net/browse/EN-2537): Implement MCP server for tensorleap users
- [EN-2415](https://tensorleap.atlassian.net/browse/EN-2415): Implement Split top panel
- [EN-2410](https://tensorleap.atlassian.net/browse/EN-2410): Support 3d visualizer
- [EN-2502](https://tensorleap.atlassian.net/browse/EN-2502): Unlabeled Analysis Top Panel Follow-up

## 🐛 Bug Fixes

- [EN-2555](https://tensorleap.atlassian.net/browse/EN-2555): Fix crash in pop explore when switching to failure aligned latent space
- [EN-2552](https://tensorleap.atlassian.net/browse/EN-2552): UI crash when using Shift to select a group of samples
- [EN-2544](https://tensorleap.atlassian.net/browse/EN-2544): Investigate latent space disappearance during re-evaluation in Zeitview
- [EN-2543](https://tensorleap.atlassian.net/browse/EN-2543): Insight generation fails silently when using filters in Analyze view
- [EN-2540](https://tensorleap.atlassian.net/browse/EN-2540): Fix autocomplete functionality in filter edit
- [EN-2538](https://tensorleap.atlassian.net/browse/EN-2538): web-ui: Manage License: "apply code" no longer submits, extension codes cannot be applied by clicking
- [EN-2534](https://tensorleap.atlassian.net/browse/EN-2534): Visualiser tooltip persists after selection
- [EN-2530](https://tensorleap.atlassian.net/browse/EN-2530): Optimise layout and alignment in the insight metric panel
- [EN-2524](https://tensorleap.atlassian.net/browse/EN-2524): Restore 'not equals' operator for boolean field filters
- [EN-2523](https://tensorleap.atlassian.net/browse/EN-2523): Scroll bar does not move during scrolling
- [EN-2522](https://tensorleap.atlassian.net/browse/EN-2522): View jumps to unknown positions when selecting or unselecting images
- [EN-2519](https://tensorleap.atlassian.net/browse/EN-2519): Remove broken Leap projects copy functionality
- [EN-2518](https://tensorleap.atlassian.net/browse/EN-2518): Optimise Pop Explorer refresh during insights generation
- [EN-2516](https://tensorleap.atlassian.net/browse/EN-2516): Fix persistent Nginx startup failure on EC2 reboot for Savana
- [EN-2511](https://tensorleap.atlassian.net/browse/EN-2511): node-server's check-leap-cli CI job fails on every master commit ("Update server api" step)
- [EN-2498](https://tensorleap.atlassian.net/browse/EN-2498): Evaluation remains in pending state when 'GPUs per run' exceeds available hardware
- [EN-2491](https://tensorleap.atlassian.net/browse/EN-2491): Fix multi-user installation conflict on Linux
- [EN-2484](https://tensorleap.atlassian.net/browse/EN-2484): Remove warning icon from started job 
