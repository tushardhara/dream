# ADR-0016: bounded reference demo and separate real-day study protocol

Status: implementation for #17; independent exact-SHA engineering review required.

The reference demo reuses the generic runtime/store, recorded RNG, dynamics and
behavior functions. Its durable checkpoint is a versioned period/hash; actor state
is a bounded deterministic projection of frozen scenario inputs and recorded
randomness. Each new period's draws and finite resource effects still commit
through the existing fenced runtime journal. No persistence schema, checkpoint
size, cognitive actor/outcome cap, receipt cap or time budget is increased.
This is intentionally a sparse, fixed synthetic reference policy rather than a
replacement general 24-actor model-backed cognition service. Unsupported event
injections/forks fail closed; no hidden history eviction or online training occurs.

The study is a separate evaluator-owned real-time protocol. A fixed 30-day plan
and strict reservation/report journal preserve scope, baseline, seed/variant,
provider bindings, quotas, uncertainty and restart identity. The strict fresh-append path uses the existing locked journal transaction and
rejects stale/repeated reservations rather than treating an idempotent receipt as
authorization to generate twice. It uses one pool connection and commits before
providers run; ordinary idempotent append consumers retain their retry semantics. No new table or private label store is added;
only plan/operational metadata and result hashes are journaled. Provenance links
preserve revocation. Generator processes retain #16's no-label/no-network boundary.

Provider ports are separately configured but fake/recorded-only in this protocol.
The CLI supports networkless fake images. A live study requires a separately
approved/configured host, provider budgets/credentials/infrastructure and authorized
data collection; no paid/live/deployed study is started or reported complete.
Candidate promotion remains #16's separately signed owner protocol; this controller
has no default-changing or model-weight-writing capability.

| Requirement | Evidence |
| --- | --- |
| 24 humans, 8 overlapping groups, 30+ edges; smoke/horizon | TestDemoScenarioScaleAndBoundedHorizon; 48 directed edges, 2/4/24 people, 1..12 fixed model months |
| Neutral/joyful/compound/scarce/incomplete/slow state | Scenario themes and existing dynamics; TestDemoInitialDerivationAndUnseenGroup, actor reconstruction/receipt/resource checks |
| Recorded multi-seed demo and measured operational smoke | TestDemoCLIIntegration, make demo-check; two full seed runs and retained actual timing/hardware/cost records |
| Recovery/replay/export and second host | TestTwentyFourPersonYearReconstructionAndRecovery, TestDemoArtifactCanonicalWireRoundTrip, TestDemoArtifactReplayAndPrivateExport, TestGraphClientIntegration |
| Real-day frozen plan, quotas, daily reports, restart | TestStudyFakeClockProtocolSequenceNotARealStudy, TestStudyQuotaUncertainRecoveryAndRepeatedFailure, TestStudyJournalIntegration |
| No duplicate writer or journal-held provider | TestStudyConcurrentReservationAndProviderIsolation, real-PG strict CAS and callback probe |
| No live/early/missed-window or automatic promotion | TestStudyRejectsLiveAndMissedWindows; compiled early-day rejection; #16 signed promotion tests unchanged |
| Separate scientific claims and deliberate negatives | TestDemoCannotClaimRealStudyOrAcceptCorruption; #16 source/falsifier honesty and mutation controls retained |
| Backend contract/UI proposal only | docs/research-console-contract.md; owner UI entry not granted |

See docs/backend-demo.md and docs/study-protocol.md for runnable commands, artifact
handling, operational bounds, unresolved owner configuration and scientific/source
gaps. A 360-day simulated horizon is not a 30-real-day observation study. Complete
HWS source falsifiers, independent frontier-model transfer, real-human validity,
200-family/large-scale research, live capacity and production stress are NOT TESTED.
