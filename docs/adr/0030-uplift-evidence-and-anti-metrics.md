# ADR 0030: Matched-arm uplift evaluation, evidence tiers and anti-metrics

Status: accepted (implementation of #58, epic #49)

## Context

#58 asks for an honest matched comparison of assistant policies across the
implemented human scenarios. The risk it names is not a missing feature but a
dishonest result: an evaluation that credits a helper with uplift it did not
earn, or that treats engagement as welfare.

## Decision

`evals/uplift.go` adds an opt-in `uplift-evaluation.v1` envelope. It changes no
frozen policy, codec or replay pin, and adds no capability to existing reports.

1. **Four matched arms.** `none`, `simple`, `single_perspective`,
   `multi_perspective`. Arms must share an identical initial world and
   exogenous-event hash, and must use *distinct* RNG streams; a shared stream is
   rejected because it couples the arms. Humans act in every arm: an arm in
   which nobody acts is rejected as a broken control, and the no-assistant arm
   may not perform helper actions.
2. **RVE-like evidence tiers.** `system_assertion`, `prompted_response`,
   `behavioural_observation`, `independently_attributed_later`. A record may not
   claim a tier above the one it was derived from. Every record must carry
   `SYNTHETIC` provenance, so no tier here can be recorded as real-human proof.
3. **Anti-metrics by contract.** Action counts, message counts, notification
   opens, session time, acceptance rate and "emotional attachment" are
   disqualified as benefit in the validator, not by convention.
4. **Unknown is not zero.** Benefit, burden and appropriateness reuse
   `core.GroupQuantity` from #56, so an unobserved burden stays unknown rather
   than reading as no burden. Missing, censored, unresolved and discordant
   per-person results are retained verbatim.
5. **Uplift is never positive by construction.** `CompareArms` reports
   `not-tested` when no independently attributed later evidence exists and
   `inconclusive` when that evidence does not separate the arms. Helper activity
   alone can never produce a `pass`.

## Consequences and limits

The evaluation deliberately demonstrates **no uplift**. The original shipped
fixture had two arms `not-tested` and only the multi-perspective arm executed and
`inconclusive`; since R1 closed (see the addendum) all four arms execute through
real consumers across all eight families, the assistance consumer runs the
multi-perspective arm, and every comparison is `inconclusive` on observed harm.
`make uplift-check` asserts this against the compiled binary and is wired into
`verify`.

This is engineering measurement, not science. Real-human validity, live-provider
semantic quality, cross-model transfer and a real 30-day study are all
`NOT_TESTED` and are not addressed here. No person represented in these fixtures
is real. The comparison establishes that the harness reports honestly; it
establishes nothing about whether any assistant policy helps anyone.

`evals` remains importable only by the evaluator and `cmd/hws-eval`; the uplift
summary is exposed through that existing binary rather than by widening
`internal/architecture/boundary.go`.

## Addendum: decisions taken while closing R1

Five decisions were needed to execute real consumers rather than fixtures. Each
changed behaviour, so each is recorded here rather than left in a commit body.

**1. RNG stream independence is per domain, not per arm.** #58 asks for
"matched initial worlds/exogenous events **and** independent versioned RNG
streams" in one sentence. Read as per-arm, the two halves contradict each
other: giving each arm its own stream gives each arm different exogenous
events. Read as per-domain — human decisions, exogenous events and the helper
drawing from separate versioned streams so one cannot perturb another — both
hold, and the matching survives. The repository's own generator settles it:
`NewAssistanceManifest` derives its human, exogenous and helper seeds without
reference to the arm. `ArmRun.Streams` therefore requires distinct domains,
distinct seeds per domain, and an identical stream set across every arm of a
comparison. **This is an interpretation of the criterion. The enforcement is
stricter under it, not weaker, but it should be reviewed as an interpretation.**

**2. Coverage is a receipt, not a name.** Coverage was decided by
`strings.Contains` over the scenario name, so a scenario called
`repair_of_ordinary_joy` credited `repair` without executing it. A comparison
now declares its family explicitly, and coverage is computed from
`ExecutedManifest` — arms that produced outcomes for people in the roster —
crediting a family only where the manifest holds a matched control and
candidate in one scenario and seed.

**3. Arms that produced identical output cannot show uplift.** `ArmRun` carries
a `PolicyHash`: the receipt of what the arm's policy actually produced, and the
only identifier permitted to differ between arms. Where two arms share it they
performed the same intervention whatever they are labelled, and `CompareArms`
names them and refuses to read a difference. On the real ordinary consumer this
immediately showed that `simple` and `single_perspective` are identical in
three of its four families.

**4. `not_instrumented` is distinct from `missing`.** `missing` and `censored`
mean an instrument existed and its value did not arrive, which is adverse.
Wiring a consumer that measures no delayed outcome at all would have reported
every person as `missing`, manufacturing harm findings out of the absence of an
instrument. `not_instrumented` says the question was never asked and is
reported as uncertainty.

**5. An uncovered family states why it is uncovered.** Six families remain
unexecuted, and the report now distinguishes a family that is merely unwired
from one no consumer can form a matched comparison for. Of the six:
`selective_boundaries`, `role_domain_trust` and `group_burden` have consumers
that take no arm at all; `life_changes` is arm-varying but has no no-assistant
control, which this contract requires and will not fabricate; `repair` and
`conflict_goals` have no arm-executing consumer. **Closing these requires
implementing arms in those consumers, not wiring in the evaluator.**

**6. Correct restraint is not a missing outcome.** The ordinary consumer only
records an experience where the helper actually acted, so in the two families
where it correctly stays silent on every opportunity no person has any
experience at all. Defaulting those to `missing` scored warranted restraint as
an adverse outcome — 32 fabricated harm qualifications, and precisely backwards
for a project whose own ticket says a good policy may correctly remain silent.
`no_intervention` now says that nothing happened, is reported as uncertainty,
and is upgraded to `missing` only where the helper DID act and no outcome
arrived for that person. `uplift-check` asserts the distinction is exercised.

**7. A harm column nothing writes to is not a measurement.**
`BoundaryViolations` was declared, validated, read as harm and serialised — and
never assigned by any adapter. Every person reported zero boundary violations,
which a reader could easily take as evidence the arms respected boundaries. It
was evidence that nothing looked. The recipient-response consumer documents an
invariance the helper must satisfy, so the adapter now executes it: each scene
is re-run at the same arm and seed with EVERY private recipient condition
changed and nothing else, and a difference in the helper's interactions is
recorded as a boundary violation attributed to whoever's state leaked. The
result is still zero — but it is now a measured zero. `UpliftSummary.Checks`
names the probes a run actually executed, and `uplift-check` requires the
boundary probe to be among them, so this distinction is machine-visible rather
than a claim in a document.

The detector is exercised against two genuinely different helper outputs so it
cannot silently stop detecting, and the variant is refused if it would not
change every private condition: varying one would pass while the helper leaked
the other. This is a bounded invariance check over the conditions these scenes
carry, not an exhaustive privacy proof, and it says so in the report.

## Addendum 2: closing R1 — arms implemented in the consumers

The owner ruled on PR #72 that implementing the arm/control paths in the
existing offline synthetic consumers is this ticket's work rather than a scope
expansion. All eight required families now execute through matched arms: 55
comparisons, 187 executed arms, all four arms, and no uplift claimed anywhere.

**The control is enforced, never conventional.** Four consumers already had an
arm-aware helper path where the assistance contract forbids the no-assistant
arm from selecting any action but WAIT. Three flows had no arm concept at all —
group, repair and listening — and each gained one as an **opt-in versioned
envelope** (`group-assistance.v2`, `repair-flow.v2`, `listening-flow.v2`),
leaving v1 byte-identical for every existing caller. In all three the control
returns before any capability is approved and before any history is read: a
helper that never looks, not one that looks and answers WAIT. An arm on a v1
request is rejected rather than silently ignored, which is how an evaluation
ends up comparing several labels for one policy.

**The arms map onto what each helper already computes.** The group and repair
helpers both build one perspective per participant, so simple assistance takes
none, single perspective takes the asking user's own, and multi perspective
takes every affected member's. The listening flow's mode and share flags already
decide whose accounts the helper may read and whether separately authored
summaries are disclosed. Nothing was invented to fill an arm.

**Where an arm has no faithful realisation it is reported unexecuted.**
Explicit-preference assistance has none in the listening flow, which requires at
least one account proposal by contract, so the nearest configuration would be
identical to single perspective. Multi-perspective has none in the temporal
consumer. Both are reported not executed rather than listed as a second label
for one policy — which the evaluation's own policy-receipt rule would catch and
name anyway.

### Three further defects this closed

**A crippled control is an asymmetry, not an absence.** The contract rejected
any arm in which nobody acted. In the temporal consumer whether people act is a
property of the SCENE and identical across arms — the eligibility gate leaves
everyone a single option in `daily_2`, `agreed_break`, `no_contact` and
`sparse`, and elsewhere everyone has a real alternative and correctly declines.
Both are results this evaluation exists to see, and the guard made half the
life_changes scenes unusable. What #58 asks to detect is a control crippled
RELATIVE to the candidates, so that is what is now checked, where arms are
comparable. `PersonOutcome.CouldAct` separates "chose not to" from "had no
choice", read from the real candidate list.

**Whether the assistant reached the people at all.** `ArmRun.HumanHash` records
what the PEOPLE decided. In every consumer wired here the humans decide
byte-identically across arms: the assistant's output is not an input to their
choice. That does not invalidate a comparison — an intervention can change what
someone experiences without changing what they do — but a null result there is
not evidence about the policy. It qualifies a finding rather than refusing one,
and fires for 103 arm-pair/scenario combinations on the real run.

**A trap declined.** `RepairResponse.Expectation` reads "unresolved" in the
no-assistant arm and "sustained_follow_through_observed" in the others, and is
tempting to map onto a delayed outcome. It would be wrong: the relationship
history is the same authored schedule in every arm, and the control reads
"unresolved" because the helper did not look, not because anything went worse
for anyone. Mapping it would have made the control appear to harm people and
handed every candidate arm an uplift it did not earn — exactly the fabricated
result #58 asks to detect. What the helper reported is the policy receipt, not
the outcome.

### What R1 now is, and is not

Three consumers execute: ordinary life, assistance, and recipient response.
All four arms run. All six delayed-outcome dispositions — resolved,
unresolved, missing, censored, not_instrumented and no_intervention — are
exercised, and
`uplift-check` asserts they are, because refusing to claim uplift is trivially
satisfied by measuring nothing. Every comparison is now decided on paired
evidence and every one is `inconclusive` on **observed** harm rather than on
absent measurement.

**All eight families are covered**, and `uplift-check` asserts it rather than
printing it, so losing one fails the gate.

What is still NOT established: seven families draw from a single
undifferentiated RNG stream because their consumers do not separate human,
exogenous and helper draws (`uplift-check` prints the family list on every run);
six of the eight (`conflict_goals`, `group_burden`, `life_changes`, `repair`,
`role_domain_trust`, `selective_boundaries`) carry no delayed-outcome instrument
at all, so their comparisons are behavioural only (recorded per person as
`not_instrumented` in the report JSON; the gate does not yet print it — #83);
and in 103 arm-pair/scenario combinations the assistant's output never reached
the human decision (printed on every run). Every comparison is `inconclusive` on
observed harm. No arm is credited with uplift, and none of this establishes human
validity. #75 tracks the instruments, streams and decision routing.
