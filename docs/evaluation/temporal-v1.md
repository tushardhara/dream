# Temporal context: bounded synthetic evaluation v1

Run `go run ./cmd/temporal-report -out docs/evaluation/temporal-v1.json` under the repository disk guard. The committed JSON is the actual 16-seed output (seeds 1–16), not a manually authored expected-result fixture. Twelve scenes use two synthetic adults, a daily or occasional own preference, 2/60-day gaps, caregiving, a work transition, stale experience, agreed break, sparse coverage, an unobserved channel, observer disagreement and no-contact.

The helper compares opt-in assistance v4, its domain v3 policy without temporal interpretation, and the simple explicit-goal baseline. All use the same explicit unknown goal and scoped willingness. Both humans still run when the helper waits. Their permitted graph inputs and temporal policy stay identical across helper arms. A separate paired human ablation removes temporal eligibility while retaining the same domain engine, source records, boundaries and random draw. These are engineering controls, not a trial of helper influence on human responses (deferred to #53).

| Consumer / policy | Trials | Label-appropriate | Unnecessary clarification | Missed clarification | Privacy / boundary violations |
| --- | ---: | ---: | ---: | ---: | ---: |
| helper / temporal | 192 | 192 | 0 | 0 | 0 / 0 |
| helper / context_off | 192 | 80 | 112 | 0 | 0 / 0 |
| helper / simple | 192 | 80 | 112 | 0 | 0 / 0 |
| human / temporal | 384 | 336 | 0 | 48 | 0 / 0 |
| human / context_off | 384 | 220 | 116 | 48 | 0 / 0 |

A trial is one actual helper choice or one actor choice, not one message. The fixed helper has no seed-dependent variation; 192 helper rows per policy are matched repeated controls, not 192 independent samples of human uncertainty. Human draws vary by seed and actor. Every seed is paired across arms; no selection of favorable seeds occurs.

The temporal human retains WAIT and misses 48 of the synthetic clarification opportunities. Removing context leaves those 48 misses unchanged and adds 116 unnecessary clarifications. The two helper baselines are identical here: a null comparison, because neither consumes this temporal interpretation and both clarify the same explicit unknown goal. The temporal helper's 192/192 agreement is expected against these deliberately authored engineering labels; it does not establish validity or generalization. No statistical significance or human benefit is claimed.

Uncertainty is measured separately from intervention frequency. The actual human temporal decisions agree with the declared routine/unusual-gap/busy/changed/stale/unknown categories in 384/384 cases. These categories and freshness/coverage thresholds are hypotheses. Agreement is not calibration against real people. Baselines expose no temporal uncertainty estimate, so their status score is omitted rather than reported as zero error. Core and actual-helper controls separately exercise the uncertain minimum-four-observation estimator, irregular samples, competing preferences and hypotheses.

Privacy counts inspect actual planner input for research/hidden-phone canaries and actual human evidence for foreign observers. Boundary counts inspect actual delivered/selected actions against the independently authored break/end conditions. Zero observed violations is bounded evidence only. Additional host tests change a private circumstance, source permission, author, chronology and replayed evidence; they check restraint without exposing another person's circumstance in the fixed outgoing reason. No free-form private explanation is generated.

Scenario migration creates day-zero owned temporal claims and explicit unknown age. Scheduled observations add contacts on days 1, 2 and 3 or voluntarily supplied life events. A current complete-channel diary is an explicitly authored coverage statement; it is not inferred from silence. One synthetic tick is one logical day in this opt-in contract. Changing ages 18/35/75/120/unknown, or unseen phone activity, leaves actual graph-derived helper and human decisions identical at matched seeds. Changing supported life, rhythm or stale-context inputs changes actual eligibility/probabilities; it need not flip every sampled choice.

All examples are synthetic and offline. This evaluation does not estimate real-world relationship health, rejection, maturity, obligations, model validity, appropriate cooldowns or optimal contact frequency. Consent/data rights remain independent gates. History, uncertainty and author/source times are retained; no unobserved event is invented.

## Behavioral mutation controls

Thirteen independent, compiling single-input mutations were each rejected by actual consumer assertions. Each source file was restored before the next mutation. The final unmodified focused suite is run separately. The read-scope mutations leave writes intact; the receipt-write mutation leaves reads intact.

| Mutation | Actual failing control |
| --- | --- |
| life-input | `TestActualTemporalCompositionScenesAndReplay/caregiving` |
| rhythm-input | `TestActualTemporalCompositionScenesAndReplay/occasional_60` |
| stale-input | `TestActualTemporalCompositionScenesAndReplay/stale_experience` |
| coverage-input | `TestActualTemporalCompositionScenesAndReplay/unobserved_channel` |
| source-author | `TestTemporalHelperRejectsForgedSourceAuthorshipAndFreshness/author` |
| source-chronology | `TestTemporalHelperRejectsForgedSourceAuthorshipAndFreshness/chronology` |
| replay-exact-wait | `TestTemporalHelperExactWaitAndCurrentTimeCannotBeForged` |
| current-clock | `TestTemporalHelperExactWaitAndCurrentTimeCannotBeForged` |
| human-burden-read-peer | `TestTemporalClarificationReadScopeDoesNotSpendAnotherPairOrTopicBudget` |
| human-burden-read-topic | `TestTemporalClarificationReadScopeDoesNotSpendAnotherPairOrTopicBudget` |
| human-burden-write | `TestTemporalHumanClarificationBudgetAndCodec` |
| history-channel | `TestTemporalHelperHistoryFiltersChannelAndScrubsProvenance` |
| history-scrub | `TestTemporalHelperHistoryFiltersChannelAndScrubsProvenance` |
