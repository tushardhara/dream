# ADR 0025: Recipient responses and separate observed outcomes

Status: design for #53, based on verified #48 integration `aaea5c7e`.
Backend/offline synthetic only; no semantic or human validity claim.

## Re-audit and gap

The actual merged `reconstructRelational` still gives Help/Support recipients a
positive support signal and Invite recipients a positive inclusion signal solely
from delivery kind. A linked reply of one of those kinds resolves the sender's
outcome as supportive. Existing `ResolveAction` does require later owned evidence,
but its demo caller invents that positive label. The helper v1 experiment correctly
does not call delivery benefit; it has no recipient-specific outcome path yet.
Existing native outcomes allow supportive/dismissive/neutral and single resolution;
corrections and mixed/unresolved evidence need a new, explicit contract.

## Decision

Add a bounded reusable outcome-observation ledger in core, with action/reply and
interaction links, observer/participant, domain/context, immediate/later phase,
expected versus observed kind, attribution, times, confidence and permissions.
Corrections append and supersede only the same observer-owned account identity.
Current projection and historical projection differ by known time; no global truth
is synthesized. Unknown/censored records contain no benefit/appraisal assertion.

A new synthetic recipient policy uses the recipient's permitted incoming
observation, own attributed context, native private appraisal/state and a separate
recorded random draw. Response categories are supportive, dismissive, neutral,
mixed and unresolved. Action kind identifies the delivery; it cannot manufacture
a supportive appraisal. Silence, lost observation, declined research participation
and missing follow-up remain missing/censored, not positive or negative evidence.

Expected sender benefit does not train observed relationship outcomes. Actual
owned appraisals and voluntarily reported later evidence feed a bounded learning
projection; corrections replace the appropriate contribution rather than applying
another gain to an already resolved event. Domains/frames and observers stay
separate. Mixed evidence retains benefit and burden separately, with no forced net
positive interpretation. All thresholds/probabilities are engineering hypotheses.

Introduce explicit new demo/helper-experiment versions. Preserve frozen v1/v2 demo
and helper v1 replay; current demo composition must exercise the new consumer at
five and 24 people. Expand scenario offers where refusal, disagreement, withdrawal
or repair are relevant. Persist action/reply links, separate immediate/later
observations and corrections. Export only the authorized observer's accounts.

The helper receives only separately permitted outcome reports, never native private
state, research labels or evaluator truth. Both humans continue acting in every
helper arm, and an invitation remains an option rather than a selected human action.
Helper outcome records distinguish asserted/expected benefit from observed reports.

## Planned evidence

- Matched welcome/unwanted invitations at the same kind and seed, actual recipient
  appraisals and subsequent native learning/choices; WAIT and adverse/null outcomes.
- Sender feels helpful while recipient feels pressured: both accounts persist and
  do not collapse into a single positive result or an automatic trust increment.
- Missing observation/participation/follow-up and silence preserve unknown/censoring.
- Correction/adverse later evidence changes current derived learning; old ledger
  bytes and authorized historical projection remain intact.
- Actual five-person and larger demo, helper experiment, observer export/privacy,
  current permission changes, recorded replay/version migration and source lineage.
- Independent mutations for unconditional supportive labels, ignored own context,
  ignored private state, correction contribution and observer/domain separation.

Final implementation, exact bounds, results and limitations will replace this
plan before the SHA-bound review handoff. No fixed positive/negative quotas, global
relationship-health score, UI, deployment, live provider or real-person data.
