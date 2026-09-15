# ADR-0028: attributed group history, agreed care and shared capacity

Status: implemented for #56; independent exact-SHA integration required.

The post-#55 inspection found group memberships/pattern storage in graph relations,
while demo inclusion/exclusion still originated mainly in the joy/compound theme
mapping. A roster was not a participation observation. Existing floating scalars
also could not distinguish unobserved burden from an observed zero.

Add `group-history.v1` core records, `group-assistance.v1` helper proposals and an
opt-in `group-demo.v1` native-engine consumer. Existing scenario/demo v1/v2/v3,
cognitive policies and replay codecs remain unchanged. No stored records are
rewritten; import the new history with its strict bounded codec. This does not
claim compatibility with future schemas or migrate historical theme signals.

A published decision records an intention, affected people (including all members
and non-attending caregivers), invitations, required task totals and up to three
options. It cannot declare another person's agreement. Each self-authored member
account independently records planning capacity/approved options or experienced
participation, co-presence, effort, benefit and burden. Unknown/censored quantities
have no number; observed zero is an explicit number. No household score, coalition
rank, majority truth, caring trait or automatic membership permission is produced.

Two distinct observed occasions with the same intention derive a repeated practice
for that observer. A disputed stance stays attributed. Quiet/absent/declined records
do not imply omission or consent. Corrections replace the exact member/decision/
phase business identity; late corrections do not make old occasions current.
Revoking a correction does not resurrect its predecessor. Complete lineage rights
are checked with exact read AND derive (and helper on-request sharing) grants.
Root publication grants are explicit; private peer histories cannot enter another
native actor's appraisal. Different group/role contexts remain separate.

The reusable helper returns feasible, explicitly agreed task alternatives with all
member perspectives separate. For the authored care case, shared-care uses two
shared-time units instead of solo-care's three and divides four care units between
two consenting people. Covered-care can avoid an already committed caregiver only
when the other person independently approved it and has capacity. There is no
minimised happiness score or automatic reassignment. Missing capacity/agreement or
any affected person's coordination boundary removes executable alternatives.
Private preparation requires the requester's scoped willingness.

`examples/groupclient` imports core/application contracts without a simulator.
Its bounded in-memory transaction authenticates request identity and rechecks
current records, boundaries and the entire cross-role reservation portfolio before
delivery or explicit reservation. Correction/revocation/delivery/allocation share
one mutex. A later change invalidates stale replay. Imported reservations must
match the original options and physical capacities; duplicate decision business
keys cannot create extra capacity. Reservations do not become observations of
benefit, and revoking evidence does not refund a committed resource. Overlap checks
are conservative across the proposed window; a complete trusted portfolio is a
host requirement. No scheduling, release protocol, persistence service or worker
is added.

`simulator/demo.RunGroups` feeds each actor's own permitted history into existing
native scoped action selection and independent recipient response. It replaces
theme inclusion/exclusion with actual known participation ratios. Observed effort
and burden activate bounded cues; unknown remains explicit in the trace. Repeated
practice and self-dispute affect setting appraisal, observed co-presence constrains
recipient availability, and missing own agreement removes Coordinate. Existing
fictional response preferences, dyadic states and RNG draws are held fixed across
history interventions. `examples/groupexperiment` composes this native consumer
with the second helper host. Only a native Coordinate choice plus all current
agreements/boundaries executes the declared shared-care preference; WAIT and other
choices do not reserve resources. The helper itself never executes or contacts.

| #56 criterion | Concrete evidence |
| --- | --- |
| Bounded group decisions, independent participation/capacity/effort | `core/group_history.go`; direct contract rejection and strict codec tests |
| Multiple affected people, unknown vs zero, resource effects | `TestGroupPerPersonEffectsUnknownAndPrivate`, `TestGroupQuantityUnknownIsNotZero`, `TestGroupReservationConsentAndReplay` |
| Actual history/attendance/agreement choice effects | `TestGroupNativeHistoryChoicesAndReplay`, `TestGroupNativeAgreementAndPresence` |
| Repeated/disputed/corrected practice, no label table | `TestGroupCorrectionsPatternsAndCurrentPermissions`, `TestGroupCorrectedPracticeChangesNativeAppraisal` |
| Newcomers, absent/quiet members, overlapping roles/private data | `TestGroupPerPersonEffectsUnknownAndPrivate`, `TestGroupOverlappingRolesResourcesAndNoRefund`, `TestGroupNoImplicitGrantsOrFutureEvidence` |
| Feasible helper alternatives, all boundaries, immutable obligation | `TestGroupReservationConsentAndReplay`, `TestGroupCurrentBoundaryCorrectionAndRevocationReplay` |
| Five and 24 people, replay, current rights and concurrency | paired seeded native tests, `TestGroupDeliverySerializesRevocationAndOwnsOutputs`, compiled `make group-check` |

The Go consumer tests invoke the contracts directly with adversarial business keys,
not only malformed fixtures from normal producers. Compiling ablations and their
attributed failing controls are recorded with the PR's exact-head evidence.

Limits: all histories, agreements, care units and benefits are authored synthetic
reports. Native choices use existing uncalibrated synthetic dynamics; an independent
recipient decision is not evidence of successful repair or human realism. The
fixture's intention and fixed shared-care preference are not learned cultural
norms. Repetition is an attributed bounded pattern, not a universal rule. This is
an operational backend demonstration with `HumanValidity=NOT_TESTED` and
`GlobalWelfare=NOT_AGGREGATED`; no real people, provider calls, outreach, UI,
deployment, human study or scientific-validity claim is included.
