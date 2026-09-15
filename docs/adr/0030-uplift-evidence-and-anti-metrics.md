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

The shipped fixture deliberately demonstrates **no uplift**: two arms are
`not-tested` and the multi-perspective arm is `inconclusive`. `make uplift-check`
asserts this against the compiled binary and is wired into `verify`.

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

### What R1 now is, and is not

Three consumers execute: ordinary life, assistance, and recipient response.
All four arms run. All five delayed-outcome dispositions — resolved,
unresolved, missing, censored and not_instrumented — are exercised, and
`uplift-check` asserts they are, because refusing to claim uplift is trivially
satisfied by measuring nothing. Every comparison is now decided on paired
evidence and every one is `inconclusive` on **observed** harm rather than on
absent measurement.

**Two of eight families are covered.** That is not R1 met, and nothing here
should be read as meeting it. `make uplift-check` prints the six uncovered
families and their reasons on every run.
