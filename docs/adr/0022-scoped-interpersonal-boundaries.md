# ADR 0022: Opt-in interpersonal boundaries and scoped human contact

Status: implemented for #51, pending independent review. Scope is synthetic,
backend and offline. Builds on verified #50 integration `72f1e1f3`.

## Decision and actual behavior

`core.Boundary` is a bounded, versioned observer-owned policy fact. Its principal,
relationship endpoint, topic, action class, validity interval, occurrence/learning
time, provenance, basis and revocation remain explicit. It is separate from
`core.Rights`: reading or deriving from an account never establishes willingness.
Only the principal's self-report establishes affirmative participation. Missing,
unknown or conflicting self-reports cannot be averaged into agreement. Other
observers' hypotheses remain distinguishable and cannot supersede a self-report.
Corrections identify earlier facts from the same observer/principal/exact scope
and basis. Current revocation tombstones prevent backdated restoration.

`core.EvaluateBoundaries` requires current affirmative preferences from every
participant affected by an action. Exact topics/classes are required for grants;
negative boundaries may cover all topics/actions. A relay retains its original
target, requires discussion and third-party preferences, and adds the relay's own
willingness. It cannot rename a money discussion to bypass a money-topic pause.
An active refusal, unknown preference, break, ending or dismissal blocks the
matching action regardless of predicted benefit. An expired break requires fresh
explicit willingness after expiry. An explicit owner correction may end it sooner.

Structured, attributed `observed_signal` fixtures distinguish ordinary disagreement
from uncertain pressure, credible pressure and credible threat. The latter three
conservatively block bilateral and third-party activity and summary sharing in
that relationship/topic, across action classes. They do not diagnose anyone or
estimate whether consent was freely chosen. Private preparation requires the
initiator's own explicit preference and may read only that initiator's account;
it does not disclose the other person's private account or contact them. The
bounded policy decision exposed by the host contains no private boundary reason.

`assistance.v2` opts into this policy using an explicit trusted `InteractionScope`.
The frozen `assistance.v1` behavior remains available for existing recorded runs.
The new host requires a `PlanningGate`: it checks boundaries **before graph context
reads or planner invocation**, again after planning/before delivery, and inside
the journal's atomic commit callback. Denied planning produces an exact WAIT
receipt without invoking the planner. Recorded and idempotent results also require
the current policy revision and exact scope; a prior result cannot authorize a
current effect. The reference host shares one mutex across policy updates and
commit and authenticates registered synthetic requests. Planning remains outside
that mutex. Scope/hash information and prior evidence are scrubbed from planner
history; only the same scope's mechanical history is eligible.

Clarification is its own consented action class. Unknown helper goals may trigger
at most two delivered clarifications per exact scope, with ten logical ticks
between them. Concurrent attempts compete at atomic commit; a changed history
invalidates the stale plan. A replay of the current ID does not consume the
budget again. New backdated request IDs are rejected by the v2 reference host.
Changing the explicit scope is a trusted host decision, not a model escape hatch.
Repeated dismissal and requested pauses produce non-intervention.

`scoped-human-actions.v1` wraps the existing `human-actions.v2` engine. Its embedded
actor stays engaged; a separate bounded relationship contact table controls the
current interaction. Leaving or withdrawing affects the target relationship, not
all other people. The action's executable class must match the trusted scope;
summary disclosure and third-party support cannot be relabelled as discussion.
Leaving/withdrawing require no recipient consent and send no notification through
a refused channel. Reconnection still requires current scoped willingness and the
underlying human eligibility rules. Unrelated safe interaction remains possible.
The wrapper changes eligibility, not subjective utility or rewards for compliance.

`examples/boundaryexperiment.Run` is an actual composition of the v2 helper host
and scoped human engine. In matched disagreement/pressure scenarios, helper
eligibility differs while the person can leave Bob and later speak with Charlie.
The example replays helper records against current policy and verifies the full
scoped human trace. It uses no live provider, outreach or external delivery.

## Version and replay compatibility

New contracts: `interpersonal-boundary.v1`, `interaction-scope.v1`, `assistance.v2`,
`scoped-human-actions.v1`, `boundary-experiment.v1`. Unknown versions fail closed.
New optional v1 fields omit from JSON when absent. No existing core state bundle,
legacy contact codec, `human-actions.v2` engine or `helper-experiment.v1` format is
reinterpreted. Four exact pre-change hashes (actor, request, interaction and full
helper experiment) are frozen in `TestFrozen50CodecsAndExperiment`.

A legacy left/withdrawn actor cannot be wrapped implicitly because its affected
relationship is unknown. A migration requires an explicit scope decision; this
change supplies no automatic guess. New scoped actor and boundary codecs reject
unknown fields/versions and enforce bounds. Replay is mechanical, not proof of
psychological or intervention validity.

## Acceptance evidence map

| #51 criterion | Code | Load-bearing tests |
| --- | --- | --- |
| Directional, time/topic/action scoped evidence; owned corrections/revocation | `core/boundary.go`, reference boundary authority | `TestBoundaryConsentIsDirectionalScopedAndNotInferred`, `TestBoundaryBreakExpiryCorrectionsRevocationAndRelay`, `TestRevokedMetadataAndHypothesisCorrectionCannotReviveConsent` |
| Preplanning, delivery, atomic commit and replay gate independent of data grants | `app/assistance/host.go`, `BoundaryGate`, `Local` | `TestScopedConsentBeforePlannerAndDataReads`, `TestScopedRevocationPlanningCommitAndReplay`, `TestScopedVersionsPrivateAccountsAndForgedWaitReplay` |
| Money pause/relay veto; unrelated conversation | core gate, helper host, scoped human wrapper | `TestRelayRequiresUnderlyingAndThirdPartyConsent`, `TestScopedMoneyPauseRelayAndUnrelatedConversation`, `TestScopedLeavePreservesUnrelatedConversationAndReplay` |
| Pressure distinguished from disagreement; no forced reconnection | core gate and action filtering | `TestScopedPressureVersusDisagreementAndPrivatePreparation`, `TestScopedNoContactBeatsReconnectUtilityAndClassRelabelling`, `TestRealScopedHelperAndHumanConsumerReplay` |
| Break expiry, repeated dismissal, bounded clarification | core gate, boundary/history revision | `TestPressureDiffersFromDisagreementAndEndingIsNotBeneficialConsent`, `TestScopedBreakExpiryNeedsFreshPreferenceAndCorrectionsAreOwned`, `TestScopedClarificationCooldownBudgetAndBackdating`, `TestScopedConcurrentClarificationsCommitOneEffect` |
| Versioned replay/history and unknown willingness | codecs, matches, planner history filtering | `TestScopedHistoryHidesBoundaryHashAndOtherScopes`, `TestScopedMissingBoundaryWaitAndUnilateralDistance`, `TestFrozen50CodecsAndExperiment` |

Seven intentional mutation controls removed the preplanning read gate, atomic
commit callback, relay discussion gate, cross-class pressure restriction, expiry
cutoff, scoped human contact and history hash scrub. Each failed its named
behavioral test; original source was restored before full verification.

## Limits and conservative choices

This is a fixture-driven eligibility policy, not a validated coercion detector,
consent inference model, counseling method or outcome claim. Trusted hosts must
supply complete relevant policy snapshots and correctly attribute intended scope;
raw language interpretation, deceptive scope labeling and real-person assessment
are outside this ticket. Pressure uncertainty blocks facilitation rather than
assigning an unvalidated risk score. Logical cooldown units are simulation ticks,
not an empirically recommended interval. Reference policy/history are bounded and
in memory, not production persistence. All retained boundary records contribute to
the revision, so even an unrelated update can reject a stale plan conservatively.
Local human decisions are pure snapshot calculations; an external host executing
them would still need atomic current-policy revalidation. The actual local helper
host exercises that commit boundary. Legacy v1 is preserved, not retroactively
claimed to enforce the new opt-in policy.
