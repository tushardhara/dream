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
