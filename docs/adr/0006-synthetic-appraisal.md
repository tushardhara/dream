# ADR-0006: bounded synthetic appraisal, with explicit source coverage

Status: proposed by #7; independent review required.

`simulator/dynamics` owns synthetic substrate, latent variables, decay, appraisal
and reference tendencies. These are experimental parameters and equations, not
psychological facts, diagnoses, fixed personality/sin labels, core identity
attributes or real-human recommendations. `app/hws.AppraisalHandler` integrates
them through #6's transactional runtime using a consumer-defined PerceptionSource.
No model provider, learned weights, online training, action selection or API is
introduced. #11 consumes the appraisal stage; it must not repeat it.

## Registry and parameter coverage

This document preserves the legacy `ticket7-subset.v1` implementation and its
wire contract. The owner has now supplied all22 HWS section6 drive names in
issue40. `simulator/drives`, codec3/registry `hws-section6.v1`, implements that
complete registry; see ADR0017. This six-variable model remains available for
old recorded state/replay only and does not substitute for the new registry.

| Stable registry ID | Baseline | Half-life | Appraisal gain | Coverage |
| --- | ---: | ---: | ---: | --- |
| fatigue | .2 | 8h | .25 | active reference fatigue/recovery pressure |
| scarcity_opportunity | .3 | 12h | .30 | active: higher means scarcity, lower opportunity |
| care | .5 | 24h | .25 | active reference care tendency |
| status | .3 | 12h | .25 | active status-threat pressure |
| belonging | .5 | 24h | .25 | active unmet-belonging pressure |
| slow_residue | .1 | 168h | .08 | active slow accumulated residue |
| Complete22-name §6 registry | see ADR0017 | see ADR0017 | see ADR0017 | separate version; old ordinals retained |

Numbers are versioned synthetic defaults, not calibrated human estimates. The
six-variable order is part of the codec. New registry names/parameters require a
new registry/model/codec version, explicit source coverage and a reviewed migration;
never silently reinterpret old array positions or existing checkpoints. V1 rejects
unknown registry/model versions. The registry is extensible by versioned append/
migration, not mutable plugin registration or silent defaults for unknown names.

The stable substrate contains six baselines plus reactivity and plasticity in
[0,1], both default .5. Appraisal never changes this substrate. Each latent variable
has bounded level, subjective confidence, a decay anchor/anchor confidence and
virtual anchor time. State has an explicit updated time and bounded causal refs.
Scenario drive strengths can seed corresponding known variables; unknown drive
names fail execution by this handler. Nonneutral initial scenario emotion fields
also fail explicitly: emotion-parameter mapping is not implemented by this
six-concept subset. They are not silently ignored. The existing #5 canary fixture
therefore remains a scenario validation/boundary fixture, not an assertion that
this handler executes all its later-stage fields.

## Perception and one appraisal owner

Perception is structured input: event/actor IDs, occurred and learned-at time,
subjective confidence and nine signals in [0,1]: effort, rest, scarcity,
opportunity, other need, support, status threat, inclusion and exclusion.
Occurred ≤ learned ≤ appraisal time; earlier events may be learned/appraised later.
Signals are not extracted from arbitrary private prose. The trusted perception
source must resolve **current self read and derive** rights for purpose `simulation`.
The pure value validator denies missing, foreign, malformed or revoked rights;
rights values alone are not authentication or consent provenance. #12 still owns
the full runtime information service and prior-lineage authorization/revocation.
No disclosure/export/attribution permission follows from appraisal permission.

Stage owner is `appraisal.v1`. Its key is SHA-256 of the JSON string tuple
`[stage, actor, event]`; callers cannot defeat it with a fresh arbitrary transition
key. The state records the event and canonical perception digest. The same event
returns an explicit already-applied rationale without changing state, even at a
later requested time. Changed content for that event fails. Permissions are checked
before duplicate recognition, so a revoked event cannot use the replay shortcut.
Grant order and signed zero do not change perception identity. #11 must consume
this stage result or recognize its key, not re-appraise it under another action
owner. New observed outcomes may arrive as new permitted events with bounded
signals; this is not online model training.

All applied event IDs remain in causal references and the duplicate ledger;
explicit research interventions add their own cause. Nothing is silently evicted.
The ledger and cause set cap at 32 entries. Exhaustion fails closed rather than
allowing old events to be appraised again. A later durable stage-ledger extension
must preserve that guarantee, not discard history for space.

## Reference equations

At appraisal time, first decay state. With `f` the current fatigue, define raw
components in registry order (signal names abbreviated here only):

```
fatigue = effort - rest + .25 * scarcity * effort
scarcity_opportunity = scarcity - opportunity + .25 * f * effort
care = other_need * (1 - .5*f) + .3*support - .3*effort*f
status = status_threat + .4*scarcity*status_threat - .25*support
belonging = exclusion - inclusion + .25*status_threat - .3*support
slow_residue = .4*status_threat + .4*exclusion + .2*scarcity
               + .3*status_threat*exclusion - .2*support - .1*rest
```

Clamp each raw component to [-1,1]. Scale it by its registry gain and
`(.1 + .9*reactivity) * plasticity * perception_confidence`. Add to the decayed
level, saturate to [0,1], and quantize to 1e-9. If a component changes, confidence
blends toward perception confidence using that same shared scale; otherwise keep
its decayed confidence. Reset anchors at this actual event boundary. These simple
interactions are intentional testable hypotheses, not explanatory mental traces.
Compound threat/exclusion has a distinct residue term; fatigue suppresses the
reference care response. Plasticity zero prevents event-induced variable changes.

`Response(state)` returns five bounded **scores**, not probabilities or selected
actions: rest, approach, support, defend and WAIT. Rest/WAIT increase with fatigue
and residue; support is reduced by fatigue/scarcity, while care increases it.
The exact equations live in the pinned implementation and are covered by an
intervention test. They have no engagement or forced-agreement objective. Full
choice/consent/outcome integration remains #11. `Intervene` is a pure research
counterfactual helper; a host must persist/branch interventions as events rather
than mutate a live checkpoint through a storage backdoor.

## Decay and numerical contract

For each variable, with baseline `b`, anchor `a`, anchor time `t0` and half-life
`h`, the value at time `t` is `Q(b + (a-b) * 2^(-(t-t0)/h))`. Confidence uses
`Q(anchor_confidence * 2^(-(t-t0)/h))`. `Q` clamps to [0,1], rounds to nearest 1e-9
and normalizes signed zero. Logical times are integer nanoseconds; no wall time,
PID, provider result or RNG draw enters these transitions.

Time-only Advance updates displayed values/time while preserving anchors. Thus
arbitrary time-step subdivision gives the same final computation instead of
feeding intermediate quantized values back into decay. Tests compare 997-way and
fuzzed subdivisions against direct evaluation, and an analytic half-life vector.
The promised invariance is for **time-only subdivision**, not for inserting new
appraisal/intervention events, which deliberately establish new anchors. The
stored-value consistency tolerance is 1e-9; quantization error per evaluated value
is at most 5e-10 plus floating-point evaluation error. Nonfinite input/state is
rejected before saturation. Cross-language/architecture transcendental-function
bit equivalence is not certified; the Go toolchain/model/codec are pinned.

## Rationales, checkpointing and limits

Rationales contain a stage/key/cause, at most seven enum codes, and six bounded
numeric deltas. No free-text rationale field, raw event text, hidden label or
unrestricted chain-of-thought is stored. The decoder validates the enum set and
ties the rationale key to an applied actor/event. Confidence remains subjective;
no calibration or independent scoring claim is made.

The runtime handler reconstructs its initial state only from pinned genesis and
versioned defaults, then uses stored canonical state on restart. Non-observing
actors decay with virtual time but do not appraise another actor's event. Denied
perception fails the whole transition before any checkpoint commit. The full
research checkpoint is not an actor/model view; use #12's information boundary
before model access. Changes still go through #6's fenced atomic event/checkpoint
commit. The real-PG test verifies restart/retry without double appraisal.

Pure state encoding is strict canonical versioned JSON, sorted causal/receipt
sets, normalized signed zero, SHA-256 identity, ≤64 KiB. Unknown/duplicate fields,
noncanonical encoding and unsupported versions fail. The runtime bundle retains
#6's **4,096-byte handler-state ceiling**; it has not been increased to make tests
pass. Two-adult reference fixtures fit; larger populations/histories may hit this
ceiling before the 32-receipt per-actor limit. That is an explicit bounded-demo
limitation, not a 24-human/long-run claim. The future snapshot/storage work must
provide a reviewed larger-state representation while retaining idempotency and
revocation. Budget failure does not commit partial appraisal state.

Tests cover persistence/plasticity, causal retention, compound interactions,
saturation, intervention response changes, analytic/subdivision decay, nonfinite
values, rights/actor/time boundaries, stage idempotency, codec and bounded rationale,
runtime recovery and PostgreSQL checkpoint retries. Registry completeness remains
UNVERIFIED. Real-human validity, learned calibration, complete emotion dynamics,
full §6 parameter coverage, long runs and action selection remain NOT TESTED or
with their later owning ticket. No private source attachments are published.
