# ADR-0021: separately permissioned offline helper (#50)

Status: proposed for exact-SHA review under epic #49.

## Current-main inspection and decision

Baseline main `7b42a380ac0c2ff6dc70182322b44433d6c0f06f` is the owner merge
of PR38. Its tree equals the completed #44 review's final tree; that aggregate
audit is reused, not restarted. Current-main Verify run34879265259 passed.

`app/hws/cognitive.go` and `simulator/behavior/action_choice.go` implement human
cognition, including a separate versioned27-action grammar. They are not an IHG
helper. `app/hws/model_types.go` provides cognitive proposals for those actors.
`app/hws/views.go:External` and `app/graph/policy.go` provide a reusable approved
external context boundary, including current rights, lineage, as-of and revocation
checks. Existing experiment Baseline returns WAIT for the *human*: it cannot be
relabelled no-assistant. Existing evaluator variants and historical codecs stay
unchanged. This inspection is scoped to #50's actual consumers.

Add `app/assistance`, depending only on core and graph, and an orchestration adapter
in app/hws. No simulator type appears in the reusable port. Import rules also
protect the independent `examples/assistanceclient` host against simulator and
HWS imports. This is a bounded application use case, not a new framework.

| Component | Identity / observations | Authority / action |
| --- | --- | --- |
| Human engine | Synthetic actor, own perceived event/private drives | Existing human-actions.v2 choice and effects |
| Helper planner | Different helper principal; explicit requesting user/goal; approved observed claims only | Proposes typed candidates; cannot deliver |
| Helper host | Trusted request registration and exact graph capability | Checks eligibility before planning, after planning and at atomic commit |
| Offline delivery journal | Separate helper/user history and scoped request digest | Idempotent fixed-template mechanical delivery; no inferred outcome |
| Evaluator / experiment | Separate world and manifest | No evaluator or latent-state input to planner |

## Small executable scope

`assistance.v1` supports WAIT, clarification of an unknown goal, acknowledgement of
an explicit listening request and a proposal to consider coordination. Understand
without sufficient supported action and pause both permit WAIT. Reconciliation is
not a default goal. Actions have fixed reason codes and no free-form delivery text.
Only the requesting person can receive a template; source identifiers, paraphrases
and third-party accounts are not part of the delivered content. No joint summary,
language-model understanding, contact-boundary intelligence or positive uplift is
claimed. Those are later ticket scopes, subject to their own gates.

No-assistant and explicit-preference arms receive no graph context and never call
the planner. Single-perspective receives at most the user's own permitted account;
multi-perspective receives separately attributed permitted accounts. Multi does
not mean corroboration, consensus or permission to disclose. The fake's simple
fixed template is intentionally not a scientific comparison of semantic quality.

`examples/assistanceclient` is an actual second host: bounded in-memory graph
journal, policy service, trusted request registration, eligibility and an atomic
helper event/delivery store. Permission updates and commit use one lock; a private
transaction context lets graph policy revalidation join the final critical section.
Planning runs outside the lock. This is a cooperative trusted in-process boundary,
not isolation against malicious Go code, durable production storage, a network API
or a live messaging system. Restart uses retained experiment records and fresh
current permissions; invalid replay has no fresh-provider fallback.

## Experiment and compatibility

`helper-experiment.v1` runs the same `behavior.ChooseAction` engine for every arm.
Two synthetic adults receive eight bounded human turns with speaking, disagreement,
cooperation and WAIT affordances. A delivered coordination template adds an option
to the user's next turn; it does not choose the human action or mark it successful.
The first affected frame is recorded separately from actual decision divergence.

The manifest pins world/helper versions, fixture digest, arm and independent human,
exogenous and helper seeds. The fixture supplies two bounded external resource-availability events. Their
units are realized from ExogenousSeed before any human/helper decision, recorded
separately, and applied to the human situation resource map. Matched arms assert
identical realized events. Changing only that seed changes availability and the
actual eligibility of Help while the human/helper seeds remain fixed.
Human/helper draws are keyed by domain and ordinal, so extra helper draws cannot
shift human draws. A recorded run must match the complete manifest, request/evidence
digests (including approved source contents) and reconstructed final transcript. Every helper replay path rechecks
current graph rights; purged sources cannot be resurrected. Duplicate delivery IDs
also validate the complete recorded envelope. Historical requests see only helper
interactions at or before their own clock; old source refs/hashes are omitted from
planner history. Recorded and stored results use the same arm validator: disabled
arms cannot deliver and simple arms cannot replace the explicit-preference template.
Pre-commit checking is deliberately repeated inside atomic journal validation.
The reference host retains at most32 interactions and1024 sanitized
policy audit records, denying new work at capacity.

New opt-in types and a new CLI require no migration of schemas1–6, cognitive.v1/v3,
legacy six-drive/action registries or recorded reference-experiment.v1 runs.
Unknown new versions deny. Existing legacy replay golden tests remain required.

Participant outcome records preserve observer, participant, source, confidence,
occurred/learned time, expected-versus-observed kind and unknown/censored status.
A delivery is only a mechanical interaction receipt. The host creates no positive
outcome. Later #53 adds recipient-generated outcome consumers and corrections.

## Evidence map

| #50 criterion | Code / executable control |
| --- | --- |
| Separate identities and reusable contracts | app/assistance; architecture negative import tests; examples/assistanceclient.Run |
| Bounded explicit goal, candidates/WAIT, independent history | Request/Result validation, Host.Execute, ExplicitPreference; TestSecondHostAndArms, TestOutageUnknownPauseAndInjection |
| Per-participant outcome distinction | Outcome.Validate; TestOutcomeDoesNotInventBenefit |
| Four arms with humans continuing | hws.RunAssistance; TestMatchedArmsReplayAndHumanActivity |
| Independent streams and matched exogenous events | AssistanceManifest, realized seeded resource events; TestExogenousSeedControlsActualHumanResources, pre-intervention and WAIT-arm equality |
| No latent/future/label/foreign claims | Only graph SafeContextItem enters Input; forbidden-source/time/mode negatives before planner |
| Same normal/error/replay privacy gates | TestCurrentAccessNormalErrorReplay; source capability revalidation at commit |
| WAIT / eligibility / outage | Zero delivery for WAIT/outage; refusal and invalid recipient/evidence negatives |
| Replay and second host | Manifest/request binding negatives, full replay equality; actual second-host execution |
| Real minimal scenario | `go run ./cmd/hws-assistance -seed 11`; same consumer invoked by required tests |

Local verification and CI results, including failed attempts, are recorded in the
SHA-bound PR evidence. Synthetic fixtures verify engineering behavior; real-human
validity, live-provider semantic competence and scientific uplift remain NOT_TESTED.

Review follow-ups add TestPlannerHistoryScrubsPriorEvidence for old source references/hashes and arm-policy negatives for both recorded and stored results. Request.Validate enforces the same total16-source bound as the consuming host. All tests run through actual consumers; app/assistance intentionally has no separate mirror-only test suite.
