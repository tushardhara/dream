# ADR-0018: Opt-in action lifecycle and typed fictional disclosure

Status: proposed for #41; replaces no legacy wire contract.
Source: owner-supplied HWS §9 grammar and revision-2 clarification in
https://github.com/tushardhara/dream/issues/41. The complete private PRDs remain
unverified; these 27 supplied action names and the lifecycle are available.

`CognitiveService.Policy = human-actions.v2` explicitly selects new-run behavior.
The existing ModelGateway still produces validated, recorded interpretation
findings with observer-owned evidence. The trusted CognitivePlanner proposes
bounded affordances and evidence-backed context; model prose never becomes a
command. The service supplies capabilities, derived beliefs and disclosure grants;
CognitiveHandler selects and commits through the existing runtime transaction,
resource accounting, model-use binding, deadlines and idempotent operation log.
No alternate executor or provider is introduced. The five-person demonstration
and full relationship/context integration are #42, not additional requirements
invented by this action interface.

## Wire and replay boundary

`behavior.Registry`, `Offer`, `Choose`, and `cognitive.v1` retain their v1 semantics,
including observe/self_disclose/third_party_support. New `ActionOffer`,
`ActionDecision`, and `cognitive.v2` are separate types. A v2 checkpoint includes
its policy and SHA-256 of the source-ordered definition table. Unknown versions,
registry mismatch, mixed v1/v2 frames, and silent upgrades are rejected. A fork
cannot relabel nonempty v1 data as v2 or vice versa; its existing time, consumed
budget, resource state and authority checks remain in force. Neither this API nor
the #40 pure UpgradeLegacy conversion grants branch authority.

The runtime's 4096-byte state cap and 65536-byte expanded codec cap remain. There
are at most four actors, sixteen tracked outcomes, sixteen commitments, eight
memories per actor, and sixteen retained evidence IDs per memory. Exhaustion is
an explicit error; no eviction of receipts or silent reset. This is a bounded
reference implementation, not yet the #42 five-person host. Exact byte-pinned v1
fixtures come from the unchanged baseline (see simulator/behavior/testdata).
Recorded/event replay validates logical state hashes without provider generation.

## Per-action contracts

Every non-WAIT action has duration 1..1,000,000 logical units, cannot finish after
the horizon, and sets the actor's availability to its completion time. Required
recipients must be present, distinct from the actor; a left actor can only WAIT
or reconnect. An unavailable actor can WAIT. Optional recipients may be omitted.
All listed evidence must be approved for the current observer. Every delivered
speech act is a bounded typed event, not proof of its recipient's agreement or
reaction. Without a permitted typed disclosure transform it contains no private
source text. Ordinary WAIT always remains a candidate.

| Action | Recipient / evidence | Additional eligibility | Effect and disclosure | Resolution |
|---|---|---|---|---|
| say | required / yes | ordinary | statement; optional permitted mode | later response |
| ask | required / yes | ordinary | question, no source text | later response |
| answer | required / yes | ordinary | answer; optional permitted mode | later response |
| reveal | required / yes | known trust/reaction, readiness >=0 | full own proposition | later response |
| partially_reveal | required / yes | known trust, readiness >=-.3 | partial own proposition | later response |
| hide | none / yes | ordinary | private suppression, no delivery | unknown/censored |
| lie | required / yes | typed own-fiction grant | false proposition, observed as say; intent private | later response |
| joke | required / yes | typed own-fiction grant | humorous transform | later response |
| challenge | required / yes | ordinary | contest claim | later response |
| apologize | required / yes | ordinary | acknowledge harm, no invented forgiveness | later response |
| support | required / yes | ordinary | express support; optional permitted mode | later response |
| complain | required / yes | ordinary | express dissatisfaction | later response |
| argue | required / yes | ordinary | disagree, no forced agreement | later response |
| withdraw | optional / no | ordinary | withdrawn contact state, optional notice | unknown or later response |
| coordinate | required / yes | ordinary | coordination proposal, no allocation | later response |
| invite | required / yes | ordinary | invitation, no invented acceptance | later response |
| decline | required / yes | ordinary | refusal, no resource change | later response |
| promise | required / yes | new ID, available units, due between finish and horizon | create pending commitment; reserve no resource | later explicit fulfillment/breach |
| break_promise | required / yes | matching own pending commitment | mark broken and deliver breach | explicit breach, reaction unknown |
| help | required / yes | spendable units; optional matching pending commitment before due | atomically consume resource and deliver help | later response; fulfillment needs explicit observation |
| ignore | optional / yes | ordinary | availability changes, no delivery | unknown/censored |
| delay | optional / no | ordinary | availability changes, optional notice | unknown or later response |
| change_topic | required / yes | typed own-fiction grant | topic-change transform | later response |
| seek_third_party_support | required / yes | ordinary | ask recipient for support; no third-party private text | later response |
| reconnect | required / yes | actor withdrawn/left | engaged contact state and notice | later response |
| leave | optional / no | ordinary | left state, removed from available recipients, optional notice | unknown or later response |
| wait | none / no | always | no delivery/allocation/availability effect | unknown/censored; outage separate |

Promise creation records an intention, not a reservation or success. Help spends
actual resources but leaves a referenced promise pending until an observer learns
an explicit fulfilled/broken result. Fulfillment occurrence must be after help
completion and no later than the deadline; it may be learned later. Expiry alone
cannot prove breach. Duplicate/foreign evidence and repeated learning fail closed.

## Lifecycle ownership and evidence

| Stage | Owner | Persisted evidence / tests |
|---|---|---|
| 1 perceive | CognitiveService + planner | current capability/input binding; service revocation cases |
| 2 retrieve | ViewService.ModelContext | permitted source IDs, observer memory; forbidden/foreign sources |
| 3 appraise | drives.Appraise | event-keyed appraisal receipt; duplicate rejected |
| 4 update latent | drives.Appraise/Advance | new 22-drive state and retained factors; intervention sensitivity |
| 5 activate/deactivate drives | ChooseAction | last-decision active flags, bounded appraisal/emotion/fear |
| 6 interpretations | typed ModelGateway | recorded artifact hash/findings; outage not_applicable |
| 7 beliefs | CognitiveService/ChooseAction | observer-attributed bounded beliefs; outage not_applicable |
| 8 candidates | registry/eligibleAction | WAIT plus validated permitted affordances; all27 tests |
| 9 consequences | ChooseAction | bounded subjective consequences, drive/context/memory ablations |
| 10 select | runtime seeded RNG/ChooseAction | draw, normalized 90% weighted +10% exploration; replay |
| 11 disclosure | policy Write + ChooseAction | policy-bound mode, private suppression/intention; nine-mode tests |
| 12 act/WAIT | CognitiveHandler/runtime | prepared becomes done only in committed state; resource/delivery tests |
| 13 later response | ResolveAction | pending/observed/censored/outage statuses and timed evidence |
| 14 learning | ResolveAction | only observer's memory changes after evidence; supportive/dismissive tests |

The stage array describes lifecycle ownership, not the wall-clock order of model
IO: generation happens before the transaction, while appraisal and choice are pure
inside the executor. Later stages remain pending until evidence; a later decision
never rewrites an earlier unknown outcome into success. Decoder validation rejects
missing stage statuses. Denied preparation commits nothing. Operational WAIT
requires durable exhausted retryable provider failure and is counted separately
from ordinary behavioral WAIT. Refusal and policy failure are not outages.

## Disclosure and privacy

Nine modes are full, partial, softened, joke, deflection, lie, omission,
topic_change and silence. Typed `FictionExperience` represents a synthetic own
feeling with bounded intensity. `FictionDraft` selects a deterministic transform
of one permitted proposition; no free output string is certified by that path.
Partial text withholds the specific feeling/intensity; full renders its quantized
degree. Source IDs remain attached. Actor identity, source ownership, consent and
provenance are checked before writing and again before commit. Revocation denies
both saved model application and derived snapshot/replay access.

These transforms are exclusively SyntheticSelfDisclosure. They cannot run in
AssistantDisclosure, reveal someone else's latent truth or bypass restricted
rights. A fictional lie is delivered as a statement; its deceptive intention and
suppressed disclosure stay private. Research views retain their separate grants;
ActorView returns only that actor's v2 private state. Private active flags and
rationale describe the last decision, not a fresh diagnosis at every time instant.

Trust, supplied relationship-role expectation, sensitivity, fear, pride, shame,
expected reaction, protective intent, social norm, stress and prior outcome each
contribute confidence-weighted terms to disclosure readiness. Zero context is
unknown, never consent. Role names alone have no stereotyped score. All eleven
terms have isolated sensitivity tests. These weights, thresholds and linguistic
propositions are explicit research hypotheses, not validated psychology. There is
no engagement, notification-response, agreement or friendship-score objective.
