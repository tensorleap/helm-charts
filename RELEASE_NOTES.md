# Release Notes - v1.6.68

**Release Date:** 2026-08-16
**Total Changes:** 16

---

## ✨ New Features & Improvements

- [EN-2448](https://tensorleap.atlassian.net/browse/EN-2448): Run that finished successfully- remove the warning icon
- [EN-1939](https://tensorleap.atlassian.net/browse/EN-1939): Add streaming lines from pippin logs to CLI for progress indication
- [EN-1906](https://tensorleap.atlassian.net/browse/EN-1906): Add download option for metadata distribution values in insights
- [EN-2414](https://tensorleap.atlassian.net/browse/EN-2414): Implement Labeling top panel
- [EN-2411](https://tensorleap.atlassian.net/browse/EN-2411): Implement Failure mode top panel
- [EN-2407](https://tensorleap.atlassian.net/browse/EN-2407): Implement statefull scheduler mechanism with considering cluster resources

## 🐛 Bug Fixes

- [EN-2452](https://tensorleap.atlassian.net/browse/EN-2452): Fix duplicated dashlets causing black screen for Infineon (v1.6.58)
- [EN-2446](https://tensorleap.atlassian.net/browse/EN-2446): Remove error icons when terminating evaluate on visualise step
- [EN-2444](https://tensorleap.atlassian.net/browse/EN-2444): Update Karpenter configuration to use stable nodes for Redis
- [EN-2443](https://tensorleap.atlassian.net/browse/EN-2443): leap server install INFO- patched coredns 
- [EN-2442](https://tensorleap.atlassian.net/browse/EN-2442): Pushes spuriously FAIL before the k8s job is ever created, with an empty error report
- [EN-2441](https://tensorleap.atlassian.net/browse/EN-2441):  QUEUED status never reaches the UI for PUSH / insights jobs
- [EN-2438](https://tensorleap.atlassian.net/browse/EN-2438): Black screen error when duplicating PE view
- [EN-2437](https://tensorleap.atlassian.net/browse/EN-2437): Make the 'add dashboard' available during evaluate job 
- [EN-2433](https://tensorleap.atlassian.net/browse/EN-2433): Investigate mixed state of samples in Asensus evaluation
- [EN-1890](https://tensorleap.atlassian.net/browse/EN-1890): Session runs with different visualization names are not well supported in SA
