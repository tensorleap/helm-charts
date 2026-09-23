# Release Notes - v1.6.81

**Release Date:** 2026-09-23
**Total Changes:** 29

---

## ✨ New Features & Improvements

- [EN-2500](https://tensorleap.atlassian.net/browse/EN-2500): Enable resizable insights panel
- [EN-2493](https://tensorleap.atlassian.net/browse/EN-2493): Display note content on hover in failure modes
- [EN-2472](https://tensorleap.atlassian.net/browse/EN-2472): Improve readability of event progress bar for large numbers
- [EN-2461](https://tensorleap.atlassian.net/browse/EN-2461): Remove horizontal line divider from bar plot tooltips
- [EN-2349](https://tensorleap.atlassian.net/browse/EN-2349): Implement detection for no-op evaluate updates
- [EN-2482](https://tensorleap.atlassian.net/browse/EN-2482): Implement tensorleap installation skill
- [EN-2412](https://tensorleap.atlassian.net/browse/EN-2412): Implement Synthetic top panel
- [EN-2214](https://tensorleap.atlassian.net/browse/EN-2214): Move version(network map) json to bucket

## 🐛 Bug Fixes

- [EN-2517](https://tensorleap.atlassian.net/browse/EN-2517): Tensorleap logs describe is not seeing events. 
- [EN-2515](https://tensorleap.atlassian.net/browse/EN-2515): Investigate slow request performance on DSO environment
- [EN-2514](https://tensorleap.atlassian.net/browse/EN-2514): Fix missing caching for Overwatch in Pop Explorer
- [EN-2512](https://tensorleap.atlassian.net/browse/EN-2512): ArgoCD has two sync operations stuck in "Running" state on prod, demo-velero since Sep 10 and dso-velero since Aug 25. Pre-existing, unrelated to this work, and probably worth a look by whoever owns Velero there.
- [EN-2510](https://tensorleap.atlassian.net/browse/EN-2510): Demo's ArgoCD rolling sync is stuck at step 1 (demo-karpenter-crd Pending since March, demo-aws-fsx-csi-driver Progressing since Sep 9), so demo values changes need manual syncs while dso rolls automatically
- [EN-2509](https://tensorleap.atlassian.net/browse/EN-2509): dso node-server crash-loops on Node heap OOM (11 restarts since Sep 9, 2 GB heap)
- [EN-2508](https://tensorleap.atlassian.net/browse/EN-2508): Job status remains 'Running' for several minutes after being stopped in Run & Processes table
- [EN-2506](https://tensorleap.atlassian.net/browse/EN-2506): Implement zot registry in EKS cluster such as in k3d
- [EN-2504](https://tensorleap.atlassian.net/browse/EN-2504): Batch size changes not reflected during evaluation
- [EN-2496](https://tensorleap.atlassian.net/browse/EN-2496): Fix broken user flow for adding samples via push override
- [EN-2490](https://tensorleap.atlassian.net/browse/EN-2490): Investigate latent space parameter in domain gap filter
- [EN-2488](https://tensorleap.atlassian.net/browse/EN-2488): Update Fetch Similar behaviour to handle top panel removal
- [EN-2487](https://tensorleap.atlassian.net/browse/EN-2487): Investigate performance degradation in DSO cloud environment visualization REST API
- [EN-2483](https://tensorleap.atlassian.net/browse/EN-2483): when installing fresh tensorleap the confirmation dialog require two clicks 
- [EN-2479](https://tensorleap.atlassian.net/browse/EN-2479): Fix missing class labels in BBox visualizer filter
- [EN-2476](https://tensorleap.atlassian.net/browse/EN-2476): WARNING level in logs- shows ERROR and WARNING 
- [EN-2475](https://tensorleap.atlassian.net/browse/EN-2475): Add the broken versions chip to the pop explorer 
- [EN-2456](https://tensorleap.atlassian.net/browse/EN-2456): Investigate zero-byte image files when using direct-FS storage fast path
- [EN-2428](https://tensorleap.atlassian.net/browse/EN-2428): Pop-explorer buttons are not reachable when there is no enough space.. 
- [EN-2423](https://tensorleap.atlassian.net/browse/EN-2423): Suppress "no metadata available" message during collection indexing
- [EN-2200](https://tensorleap.atlassian.net/browse/EN-2200): Node server fails under pressure 
