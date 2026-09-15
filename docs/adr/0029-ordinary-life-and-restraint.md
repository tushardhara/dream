# ADR 0029: ordinary life, authentic words and restraint

Status: proposed in #57; engineering acceptance requires exact-SHA review and integration.

## Existing code and decision

The post-#56 base already has current permissioned memory, exact-span disclosure,
scoped human actions, boundaries and group task reservations. Its fixed helper
v1 templates do not provide the four requested ordinary primitives. Reuse those
ports through `core/ordinary.go`, `app/assistance/ordinary.go` and a bounded second
host `examples/ordinaryclient`. No model, new scheduler, UI or live delivery is added.
The opt-in `ordinary-life.v1` envelope does not reinterpret any prior codec/policy.

Appreciation and memories contain exact authenticated participant submissions.
`Origin` alone proves nothing: the host binds the actor, observer, reporter and
subject to the source, and graph policy revalidates read, derive and exact-recipient
share rights, current lineage and private ancestors. The existing exact-span
writer validates the source record before words are extracted. The generic arm
uses optional fixed wording without attributing sentiments; both enabled arms
retain identical current safety gates. Public sensitivity is not blanket sharing.
The API actor argument represents trusted composition, not an authentication server.

Every affected person must explicitly want the activity, expect positive benefit,
be available for its full window and permit the proposed effort. Unknowns remain
unknown. Overlapping group duties consume actual personal capacity. An explicit
plan additionally retains existing group agreements, boundaries, resources and
exact task assignments; its actual task load must fit the declared effort. Only
a separate authenticated reservation operation executes it. In the fixed control,
manual matching spends three coordination-time units and slot matching one, while
both assign one care unit to each person. This is a defined synthetic resource
saving, not a general claim of reduced human burden.

Current host time must equal the registered request time. Registration, snapshot,
permission checks, fixed rendering and response commit share one bounded critical
section with corrections, revocation, boundaries and reservation. Cancellation
before commit leaves no response. Replay recomputes current authority and budgets;
it is not permission to replay revoked words. No model runs inside this section.

Quiet and unsupported opportunities WAIT. At most two helper interruptions occur
per forty logical units, at least ten units apart. Two unanswered opportunities
in the same activity pause it; an explicit negative later report pauses immediately.
Fresh willingness or a newer welcomed report can reopen the activity, while the
independent current boundary gate still applies. Silence is not a character or
relationship score. Mechanical pause decisions use bounded participant reports;
raw anecdotes are not part of that projection.

## Acceptance evidence

| #57 requirement | Consumer evidence |
| --- | --- |
| Four ordinary primitives, exact authentic words | `TestOrdinaryPrimitivesAndExactAuthoredWords` |
| Current benefit, availability, effort and permission | `TestOrdinaryMissingBenefitTimingAndCapacityWait`, `TestOrdinaryPermissionedStoryAndReplay` |
| Authorship, corrections, private/revoked ancestors | `TestOrdinaryAuthorityCorrectionAndAncestors`, attributable `TestOrdinaryCoreGuards` |
| Cancellation and revocation ordering | `TestOrdinaryAtomicRevocationAndCancellation` under race detector |
| Group duties and explicit low-burden plan | `TestOrdinaryOverlappingGroupDuties`, `TestOrdinaryPlanCannotUnderstateEffort`, `TestOrdinaryCoordinationReducesDefinedBurdenWithoutOffload` |
| Budget, dismissal and repeated non-participation | `TestOrdinaryBudgetDismissalAndLaterExperience`, `TestOrdinaryRepeatedNonparticipationPauses` |
| Separate later burden, preference and participation | `TestOrdinaryIndependentObservedBurden` and authenticated `Observe` business-key tests |
| Quiet life, native declines, null/adverse comparison | `TestNativeOrdinaryFamilies`, compiled `make ordinary-check` |

## Bounded native comparison and limits

`examples/ordinaryexperiment` feeds only permitted mechanical offers into the
existing scoped native policy. Unsupported opportunities produce no social offer,
so the native policy's existing social-action score floor cannot force an ordinary
suggestion. Two fictional people make independent daily choices in all arms.
Across four families, eight seeds and three arms (96 runs), each has twelve helper
opportunities and twenty-four daily choices. Quiet/unknown/disabled runs have twelve
helper WAITs; enabled ordinary families still have a majority. Actual recipient
Decline choices block subsequent game opportunities without updating either
person's relationship memory. No frozen action policy is changed.

Later participation and benefit labels are explicit authored fictional self-reports,
not inferred from a delivered suggestion, selected invitation, reply or reservation.
They include unknown and adverse reports. Native declines can produce a separate
self-decline receipt, with unknown benefit. Per-person expected preferences, later
benefit/burden, native choices and actual resource receipts are separate fields.
No average welfare or engagement objective is computed.

The generic/permitted-context comparison is intentionally allowed to be null:
wording is not semantically interpreted by this native policy, and its choices,
resource receipts and later reports are identical between enabled arms. Exact
words differ only where permitted. This validates engineering connections and
restraint, not that personalization improves enjoyment. The disabled-helper arm
is a continuity control, not the four-arm evaluation required separately by #58.
The compiled gate records a bounded `bin/ordinary-report.json` and replay hash.
Human validity is `NOT_TESTED`, welfare `NOT_AGGREGATED`; no live study, provider,
calendar, messaging, paid call or real-person data is used.
