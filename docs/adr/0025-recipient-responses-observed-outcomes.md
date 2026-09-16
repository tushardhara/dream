# ADR 0025: Recipient responses and separate observed outcomes

Status: implemented for #53, based on verified #48 integration `aaea5c7e1f84e82afeb9c4be4a46b41c6a79ac56`.
Backend/offline synthetic only; no semantic or human validity claim.

## Re-audit and version decision

The actual merged `reconstructRelational` gives Help/Support recipients a positive
support signal and Invite recipients a positive inclusion signal from delivery
kind. A linked reply of those kinds resolves the sender's outcome as supportive.
Although `ResolveAction` requires later owned evidence, this caller supplies the
positive label. Helper experiment v1 correctly avoids calling delivery benefit,
but has no recipient-specific observed-outcome consumer.

`backend-demo.v3` and `helper-experiment.v2` introduce new behavior explicitly.
`RunDemo`/the compiled CLI now construct `ResponsiveScenario`; recorded v1/v2 demo
artifacts still dispatch to their unchanged engines and factories. New world and
export fields use `omitempty`, preserving old canonical representations. Runtime
capabilities add `relationships.v3` for explicitly authored domain accounts; the
capability list is an engine admission check, not part of an old genesis payload.
The v3 checkpoint remains canonical version/period/world-hash JSON below 256 bytes.
Legacy `human-actions.v1/v2`, `ResolveAction`, helper experiment v1 and assistance
v1–v4 contracts retain their meanings. No old reply is reinterpreted retroactively.

## Attributed observations and learning

Core `outcome-observation.v1` is an append-only ledger, bounded to 128 entries per
observer. Records bind action, optional actual reply, interaction, participant,
observer/source, sender/recipient position, expected/observed kind, domain/frame,
immediate/later phase, confidence, logical times and current data permissions.
There is no global outcome or relationship-health score. Expected sender usefulness
and recipient pressure can coexist. Unknown/censored records have neither welfare
scalars nor an appraisal. An observed unresolved appraisal also has no scalar.

Corrections append a new identity and supersede only the same observer/account,
action, position, kind and phase. Current projection uses only already-learned
corrections; historical projection keeps past knowledge. Current revocation still
applies to historical queries, and revoking a correction never resurrects its old
claim. Conflicting unsuperseded same-phase accounts remain separate and do not
train an averaged conclusion.

`recipient-response.v1` consumes only the recipient's permitted observation,
source-attributed own context, own native private appraisal/emotion/fear and draw.
The action kind identifies a delivery; it does not choose a positive label. The
policy samples supportive, dismissive, neutral, mixed or unresolved appraisals.
Missing context forces uncertainty. Explicit silence/lost observation is unknown;
declined participation/missing follow-up is censored. No category ratio is forced.

The toy readiness weights are .55 expectation, .2 trust, .15 expected reaction,
−.35 stress, −.2 contextual fear, .15 native valence, −.3 native fear and −.2 native
threat. Context values are discounted by measure and source confidence. Five
normalized weights depend on readiness, conflicting context and uncertainty; their
constants and the representative benefit/burden pairs (.6/.1, −.4/.7, 0/0, .4/.5)
are engineering assumptions, not fitted or calibrated psychological measurements.
Mixed benefit and burden are retained separately and contribute no net trust gain.

`OutcomeLearning` requires current own outcome metadata and every supporting
source. Expected sender benefit and missing observations never train. The latest
observed phase of each action contributes once; at most eight recent actions per
peer contribute to the bounded current projection, with the full ledger retained.
Supportive/dismissive observations contribute ±.1 times confidence to trust and
half that to disclosure; other categories contribute zero. Rebuilding before
clamping makes corrections replace a contribution rather than adding another one.
`LearnDomainObservations` also validates the current domain account and sources,
updates only that peer's domain/frame bucket and preserves unrelated buckets.

## Actual demo consumer

The new factory authors explicit coordination/everyday accounts with their own
context sources and numerical reports. It does not assign coefficients from role,
sex or age, or automatically migrate an old universal trust vector. Separate own
participation settings authorize discussion/coordination; willingness to discuss
an invitation does not establish that it was welcome. Scoped withdrawal/leave
blocks the affected relationship without globally withdrawing the actor.

The same native action engine receives those inputs and observed learning before
its next actual choice. Offers include invitation/help/coordination/promise and,
in discussion periods, refusal, challenge, disagreement, apology, ignoring and
reconnection. Breach offers require a retained actual pending commitment. Help
consumes finite shared time; neither promise, apology nor friendly reply proves
fulfillment, forgiveness or benefit.

Each actor processes at most one prior inbox delivery per period. It generates
an immediate own appraisal after its native choice and a distinct later appraisal
at the next observation period. An actual reply can have a friendly action kind
while its author privately feels pressure. Pending deliveries are carried within
the fixed 24×24 bound. Native delivery receipts stay mechanically unknown rather
than receiving an invented supportive outcome. Unprocessed future experience is
not scored. Sender expectations are separately attributed private records.

There are at most 24 periods, 24 actors, 576 native choices and 72 outcome records
per observer. The current evidence set remains below 256; only selected memory
proofs enter the native 16-source port. One canonical RNG draw per actor/period
stays within the existing 1024-draw budget. SHA-256 domain-separated subdraws bind
appraisals to that draw, action and phase; reconstruction checks recorded native
draws and the whole world hash. Own exports filter all new private response and
outcome fields by observer. No private appraisal is automatically sent to a peer.

## Actual helper experiment and report port

The helper's existing contract delivers fixed advice only to its requesting user.
`RunResponsiveAssistance` preserves this: a delivered coordination suggestion adds
an optional affordance in a later matching domain/frame/scope. Already available native coordination options are not duplicated or attributed to
advice. Only an independently selected new Coordinate affordance reaches the
other human as an advice-linked action. An `OutcomeAction`
receipt binds that actual choice, effect time and actual reply identities to its originating advice. Advice delivery alone
creates neither another person's observation nor a benefit label.

Both humans keep acting in no-assistant, explicit-preference, single-perspective
and multi-perspective arms. The same domain and recipient policies run in all
arms. Native incoming actions generate private immediate/later observations and
feed subsequent native learning. Worlds have 2–8 actors and 1–16 frames; retained
native outcome and evidence bounds remain enforced. Frame observation/sharing and
follow-up settings are explicit fictional participant choices.

The helper step port receives only ordinal, clock, seed and its prior interaction
for replay. It receives no native private state, private expectation, future frame,
recipient appraisal or research label. Public helper context is held fixed while
private recipient conditions vary. Only an explicit participant sharing choice
adds permission for an outcome report. `assistance.OutcomeReports` checks current
read permission, helper identity, the actual action receipt, participants,
domain/frame, chronology and self attribution. Reports omit private model state,
source/parent lineage and private account IDs. Shared report identifiers hash only
the permitted output fields; private observation identities and replay-input hashes
stay internal. Changing only private identity/lineage leaves the shared report
byte-identical. Shared reports remain separate
records; they are not automatically fed back into the planner. Supporting source
permission for native learning is checked independently of report permission.

The demo is a sparse nanosecond-clock consumer; the helper fixture uses bounded
logical ticks. This ticket does not silently reinterpret either as the logical-day
temporal experiment or change #48's clarification cooldown semantics.

## Acceptance evidence and limits

- `TestRecipientContextAndPrivateStateChangeAppraisalNotActionKind` uses matched
  invitations/draws, independently varies context/private state and action kind,
  and exercises all five categories.
- `TestResponsiveActualConsumers` runs actual five- and 24-person, 24-period worlds;
  seed 11 exercises all categories, later observations, replies, refusal/repair,
  scoped withdrawal and commitment-breach affordances. The runs have respectively
  5/10 dismissive and 24/66 unresolved recipient observations.
- `TestResponsiveObservedLearningChangesActualChoice` and
  `TestHelperObservedLearningChangesNativeChoice` change only observation
  availability, preserve native RNG and prove later candidate probabilities change.
- `TestActualHelperRecipientOutcomes` uses the real graph/helper host, native human
  choices and recipient policy. It preserves expected-helpful/observed-pressured
  accounts, explicit native action links and all categories, including delayed ones.
- Missing-observation tests exercise silence, loss, declined participation and
  missing follow-up through both real consumers. No missing record acquires a
  positive or negative welfare scalar.
- Core/native correction tests and
  `TestDomainObservationConsumerRebuildsCorrectionWithoutCrossFrameTransfer` cover
  contribution replacement, historical knowledge, current source/account rights
  and preservation of unrelated finance/business memory.
- `TestHelperReportsCurrentRightsCorrectionsAndActionBinding` checks corrections,
  historical reports, revoked correction non-resurrection and rejected action,
  recipient and time mismatches. Private-context invariance and actual v3 own
  export tests cover bounded privacy controls.
- `TestAdviceDoesNotDuplicateExistingHumanOption` compares actual no-helper and
  multi-perspective runs with an existing coordination option. Advice neither
  changes the native choices nor receives an action/outcome attribution.
- Recorded native/runtime/helper replay and corruption tests cover the new paths;
  existing v1/v2 tests continue to exercise frozen legacy reconstruction.
- Eighteen compiling mutations were caught by behavioral assertions: unconditional
  supportive labels; ignored context/private state; ignored corrections; learning
  from sender expectations; cross-domain reads; missing-as-observed; ignored demo
  or helper learning; bypassed report rights; foreign export; wrong-domain writes;
  ignored action binding; leaked private account IDs; duplicate native options;
  observations before the action effect; unrecorded reply identities; private-input
  fingerprints in shared report identities. Sources were restored after
  every mutation. This is bounded sensitivity evidence, not exhaustive proof.

The committed [16-seed report](../evaluation/recipient-response-v1.json) covers six
conditions × four arms × sixteen seeds, 384 runs and 6,144 native choices. Generate
it with `go run ./cmd/response-report --out docs/evaluation/recipient-response-v1.json`
under the repository disk guard. Enabled helper arms are identical in this fixture:
25 advice-linked native actions per condition, and no demonstrated advantage for
multi-perspective assistance. Welcome versus unwanted appraisals differ, while
silence/loss produces no welfare labels. All tested private-condition changes leave
matched helper interaction bytes unchanged. Immediate/later records are dependent
observations, not independent participant samples. See the report notes for bounds.

Full verification and independent Claude acceptance are recorded on the exact-SHA
PR checkpoint, not inferred from these focused tests. No UI, deployment, paid/live
provider, real-person data, live outreach, real study or human-validity claim.
