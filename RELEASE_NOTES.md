# Release Notes - v1.6.109

**Release Date:** 2026-11-05
**Total Changes:** 50

---

## ✨ New Features & Improvements

- [EN-2607](https://tensorleap.atlassian.net/browse/EN-2607): Maintain platform state within the analyse skill
- [EN-2586](https://tensorleap.atlassian.net/browse/EN-2586): Implement Mixpanel event tracking for report generation
- [EN-2585](https://tensorleap.atlassian.net/browse/EN-2585): Fix inconsistent colour coding in Pop Explorer when metrics are missing across multiple models
- [EN-2584](https://tensorleap.atlassian.net/browse/EN-2584): Add Tier property to Mixpanel user profiles
- [EN-2583](https://tensorleap.atlassian.net/browse/EN-2583): Audit top panels jobs for Mixpanel event attributes
- [EN-2578](https://tensorleap.atlassian.net/browse/EN-2578): Support leap run stop command for integration skill
- [EN-2577](https://tensorleap.atlassian.net/browse/EN-2577): Add option to export static metadata dashboards to dynamic dashboards in domain gap top panel
- [EN-2575](https://tensorleap.atlassian.net/browse/EN-2575): Collections Export 
- [EN-2564](https://tensorleap.atlassian.net/browse/EN-2564): Display Insight number in Mobileye failure mode top panel
- [EN-2563](https://tensorleap.atlassian.net/browse/EN-2563): Add domain selection option to domain gap top panel
- [EN-2560](https://tensorleap.atlassian.net/browse/EN-2560): Implement instance-level integration defaults and UI controls
- [EN-2539](https://tensorleap.atlassian.net/browse/EN-2539): Add histogram option to dashlet with X-axis only selection
- [EN-2536](https://tensorleap.atlassian.net/browse/EN-2536): Add a select button for label next
- [EN-2527](https://tensorleap.atlassian.net/browse/EN-2527): Implement an 'In Review' drawer for insight navigation
- [EN-2495](https://tensorleap.atlassian.net/browse/EN-2495): Implement download latent space button for analysis and debugging
- [EN-2471](https://tensorleap.atlassian.net/browse/EN-2471): Add standard deviation intervals to dashboard line graphs
- [EN-2611](https://tensorleap.atlassian.net/browse/EN-2611): Implement support dialog
- [EN-2610](https://tensorleap.atlassian.net/browse/EN-2610): Free tier project should restrict users to have one code origin
- [EN-2609](https://tensorleap.atlassian.net/browse/EN-2609): Implement contact tensorleap dialog and lambda function for free tier
- [EN-2606](https://tensorleap.atlassian.net/browse/EN-2606): Implement lambda for checking if license is confirmed
- [EN-2605](https://tensorleap.atlassian.net/browse/EN-2605): Implement almbda function that activated license from the email
- [EN-2600](https://tensorleap.atlassian.net/browse/EN-2600): Implement registration page that host the registration form
- [EN-2599](https://tensorleap.atlassian.net/browse/EN-2599): Verify that token has limited time to be redeemable
- [EN-2598](https://tensorleap.atlassian.net/browse/EN-2598): Support Confirmation token that signed by a certain email or domain
- [EN-2597](https://tensorleap.atlassian.net/browse/EN-2597): Support colored token for intermediate installation 
- [EN-2596](https://tensorleap.atlassian.net/browse/EN-2596): license key should support generate token by email or domain
- [EN-2595](https://tensorleap.atlassian.net/browse/EN-2595): Implement lambda function for user submit in landing page
- [EN-2594](https://tensorleap.atlassian.net/browse/EN-2594): Generate form with user email and all relevant details in landing page and trigger lambda function on submit
- [EN-2582](https://tensorleap.atlassian.net/browse/EN-2582): Gather all tl_analysis skill reports into the platform
- [EN-2569](https://tensorleap.atlassian.net/browse/EN-2569): Free tier
- [EN-2567](https://tensorleap.atlassian.net/browse/EN-2567): Insights panel should support failure modes only
- [EN-2566](https://tensorleap.atlassian.net/browse/EN-2566): Explore how we should do pull request review in AI world

## 🐛 Bug Fixes

- [EN-2608](https://tensorleap.atlassian.net/browse/EN-2608): Fix Accuracy, Precision, Recall and F1 metrics in confusion metric dashlet
- [EN-2592](https://tensorleap.atlassian.net/browse/EN-2592): Investigate GPU detection failure in Lumana main engine pod
- [EN-2581](https://tensorleap.atlassian.net/browse/EN-2581): Investigate missing validation data in dashlets for Blueriver v1.6.100
- [EN-2580](https://tensorleap.atlassian.net/browse/EN-2580): Investigate model push failure in Tensorleap version 1.6.85/1.6.100 airgap environments
- [EN-2576](https://tensorleap.atlassian.net/browse/EN-2576): Fix 'model_id' missing argument error in leap push with --no-wait
- [EN-2574](https://tensorleap.atlassian.net/browse/EN-2574): Fix false positive starvation warning in Slim labeling jobs
- [EN-2572](https://tensorleap.atlassian.net/browse/EN-2572): Fix OOM crashes in Update Evaluate path by aligning pod settings with Evaluate path
- [EN-2568](https://tensorleap.atlassian.net/browse/EN-2568): Fix class selection logic in bbox visualiser
- [EN-2562](https://tensorleap.atlassian.net/browse/EN-2562): Latent spaces that filtered in evaluate (less than 15%..) are visible in DS-curation dropdown
- [EN-2556](https://tensorleap.atlassian.net/browse/EN-2556): Investigate Out of Memory (OOM) error on newest server when running multiple pop explorers
- [EN-2554](https://tensorleap.atlassian.net/browse/EN-2554): DS Curation: Unlabeled- missing settings button and progress bar
- [EN-2553](https://tensorleap.atlassian.net/browse/EN-2553): Splitting top panel- the name of distribution is above the arrows < > 
- [EN-2546](https://tensorleap.atlassian.net/browse/EN-2546): Prevent duplicate filters when applying from insights
- [EN-2545](https://tensorleap.atlassian.net/browse/EN-2545): Insight filters incorrectly display 'Applied' status when a filter is disabled
- [EN-2533](https://tensorleap.atlassian.net/browse/EN-2533): Remove broken fields on Domain Gap
- [EN-2353](https://tensorleap.atlassian.net/browse/EN-2353): Fix poetry.lock hash mismatch and installation failure in Pippin

## 📝 Other

- [EN-2095](https://tensorleap.atlassian.net/browse/EN-2095): Video visualization
- [EN-2094](https://tensorleap.atlassian.net/browse/EN-2094): DS curation - (pruning)
