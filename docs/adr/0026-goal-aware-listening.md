# ADR-0026: bounded participant-owned listening and clarification

Status: implementation for #54; independent integration review pending.

## Re-audit and decision

The verified #53 integration (`f72e4abd576c97755ece235744d2348f3ad7e0a8`)
contains fixed helper goal prompts, current boundary/disclosure enforcement,
observer-owned outcomes and typed model provider contracts. It has no executable
original-statement → interpretation proposal → participant confirmation/correction
flow. Adding another successful acknowledgement would not satisfy #54.

Add `listening-account.v1` in core and a separate `listening-flow.v1` application
host. Reuse `core.EvaluateBoundaries`, `graph.PolicyService` capabilities, its
strict disclosure writer, and the existing `hws.ModelProvider`, `ProviderInput`,
`ModelOutput`, `ModelArtifact` and immutable `model.Recorded` contracts. Do not
change the existing assistance v1–v4, model.v1, cognition.v1 or demo v1–v3 semantics.
This is a separately versioned consumer, not a new provider gateway or supervisor.

## Ownership, meaning and goals

A participant submits their original wording, attributed observation reports,
values, alternative hypotheses, mixed feelings, desired help and communication
preferences. Hypotheses cannot assert certainty. The account source and speaker
must match a currently authorized memory event's observer, original reporter and
self subject. The selected domain/frame must match exactly. The reference host
authenticates submission and permits correction only of the speaker's current
account in the same pair/domain/frame. Corrections append memory events, preserving
past records; current-source authorization and graph supersession prevent stale
accounts from driving later assistance.

The helper receives each participant's account independently. No partner account
means unknown feelings and goals. Joining supplies another attributed account,
not corroboration. Exact authored clause-key/text comparisons distinguish different
observation reports from different values and alternative hypotheses; this is not
semantic contradiction detection. Every observation's factual support remains
`participant_report_only`. No independent factual evidence is supplied by these
fixtures, so no factual dispute is adjudicated and no winner is selected. Values
are not facts requiring agreement.

The model adapter sends one current policy-approved account per typed request.
It accepts one validated interpretation proposal (`uncertain`, `supported` or
`contradicted`), with exact observer/evidence binding. It accepts only the synthetic
fake-generation protocol used by these fixtures. Missing/malformed output yields
WAIT; it cannot introduce a motive, winner, new source, goal or free-form text.
Proposal confidence is capped at .5 and is explicitly not empirically calibrated.
Even a supported model proposal cannot confirm an unconfirmed account.

Desired help is participant-selected: unknown asks whether listening,
understanding, a practical plan or a pause is wanted; unconfirmed meaning asks for
clarification/correction or a pause. Confirmed supported accounts can receive a
listening acknowledgement, a reflection prompt or an optional practical-next-step
prompt. Pause and unsupported language stop model calls. Another participant's
private interpretation cannot choose this person's prompt. No path demands
agreement or maximal conversation. At most two clarification prompts per
participant/session/domain/frame, at least ten logical units apart, are permitted.

## Sharing, consent and revocation

Private preparation needs only the requesting person's current willingness; an
absent or unwilling partner and credible-pressure signals do not remove this
independent value. Joint mode uses the existing bilateral discussion gate. Summary
sharing additionally needs current summary-sharing willingness. An invitation to
shared discussion is a fixed optional template, gated by bilateral discussion
preferences and returned only to the requesting person. It sends no outreach.

Reading a private account is not permission to share it. A confirmed participant
selects a separate authored summary source. Its exact text is quoted by the
existing `AssistantDisclosure`/`ShareOnRequest` writer with current recipient grants,
authorship and complete source lineage. The helper does not generate a paraphrase.
The reference fixture gives these explicitly selected words public *sensitivity*
to satisfy the standing strict third-party-disclosure policy; exact grants still
limit recipient and operation. This is not public publication or blanket access.
Declared restricted parents prevent even a source-identifying paraphrase from
passing. Unconfirmed or substituted summary selections fail closed.

Helper-private records retain both permitted accounts and model proposals. The
user response contains only that person's own account, fixed non-identifying
prompts and separately permitted chosen words tagged with their speaker. It does
not contain the partner's private account, source IDs, lineage, private hashes,
feelings or interpretation. The fixed prompt never incorporates private wording.
Neither a world snapshot nor evaluator truth is accepted by any input port.

Authorization, selected account identities, boundary preferences and all graph
capabilities are checked before return and under the journal's atomic commit.
Corrections/revocation during a model call or immediately before commit deny the
entire response. Idempotent replay rechecks current rights and returns committed
bytes without another model call; it cannot resurrect revoked material. The
reference `Local` implements this contract with one shared lock and bounded memory.
It is an executable offline host, not a deployed durable storage implementation.

## Exact tested language and accessibility scope

Authored English and Spanish versions of `Fine` / `Está bien`, their explicit
participant corrections and fixed clarification/goal/pause prompts are tested in
literal and indirect preference conditions. The budget example uses English
participant-authored text. No automatic translation or language detection occurs.
Other valid language tags return WAIT without model calls. English/Spanish template
correctness here is fixture coverage, not evidence of open-ended multilingual
understanding.

Literal/indirect wording preferences change confirmation prompts; short-turn and
plain-language preferences change the rendered text; text/transcript preferences
select plain text or a speaker-labelled transcript. These are self-declared inputs.
No age, cultural identity, disability, diagnosis, gender or relationship label is
used to infer a communication preference. There is no voice/sign-language model
or UI, and no claim that these templates establish accessibility adequacy.

## Acceptance mapping

| Ticket requirement | Executable evidence |
| --- | --- |
| Versioned statement, attributed reports, hypotheses, uncertainty, goals, correction and optional summary | core/listening.go; ListeningHost.Execute; TestListeningAccountPreservesMixedFeelingsAndUncertainty; TestCurrentAccountAndCorrectionAuthority |
| Single-person and absent partner; independent accounts | TestMixedFeelingsAndPartnerPrivacy; TestDeclinedMediationAndUnequalPowerKeepPrivatePreparation; TestBudgetFightKeepsBothAccountsAndDifferentGoals |
| Existing typed model ports, recorded synthetic fixtures and orchestration | adapters/model/listening.go; RecordedFor uses ModelArtifact and model.NewRecorded; TestListeningUsesTypedProviderAndRejectsUnsupportedClaims; TestTypedInterpretationActuallyChangesNextAssistance |
| Mixed feelings, literal/indirect wording, supported-language pairs and declared preferences | TestAmbiguousFineAndExplicitCorrectionChangeAssistance; TestMixedFeelingsAndPartnerPrivacy; TestDeclaredCommunicationPreferencesAffectRendering |
| Chosen summaries/invitations through boundary and disclosure gates | TestInvitationAndSummaryNeedTheirOwnCurrentConsent; TestSharingGrantsRevocationAndRestrictedParaphraseLineage |
| Listening/understanding/action/pause; facts distinct from values | TestBudgetFightKeepsBothAccountsAndDifferentGoals; TestClarificationRespectsCooldownBudgetAndPause; TestCurrentAccountAndCorrectionAuthority |
| Contradictory budget accounts, different goals, no invented summary/winner | TestBudgetFightKeepsBothAccountsAndDifferentGoals; typed winner/extra-intent negatives; compiled listening-check |
| Ambiguous fine remains uncertain; correction changes assistance | TestAmbiguousFineAndExplicitCorrectionChangeAssistance; TestUnconfirmedMeaningCannotBePromotedByConfidentModel |
| Happy promotion and sad time apart coexist; concealed state not exposed | TestMixedFeelingsAndPartnerPrivacy; TestJointPartnerPrivateInterpretationCannotIdentifySourceInQuestion |
| Private/joint grants differ; revocation blocks sharing and identifying paraphrases | TestListeningRequestRejectsPrivateToJointGrantSubstitution; TestSharingGrantsRevocationAndRestrictedParaphraseLineage; TestRevocationDuringModelPreventsCommitAndReplay; TestRevocationAtAtomicCommitBlocksPreviouslyValidatedSummary |
| Unknown partner, declined mediation and unequal power preserve independent value | TestDeclinedMediationAndUnequalPowerKeepPrivatePreparation; TestBoundaryWithdrawalDuringModelPreventsDelivery |

Other controls cover source/observer/report binding, unknown fields/versions,
unsupported model claims, missing interpreter, correction scope, current private
read revocation, concurrent idempotency and clarification limits. Mutation results
and full exact-SHA verification are recorded in the PR handoff rather than treating
test names as proof that every guard is load-bearing.

## Execution and limits

`make verify` includes the existing complete gates plus `listening-check`.
`./bin/hws-listening` executes the bounded budget-fight host with synthetic recorded
model fixtures. The compiled check asserts different goals, exact chosen words,
private account isolation and byte-identical replay of this deterministic fixture.

Bounds: two fictional participants, 32 requests/committed turns, 32 memory entries
per participant, existing 64-boundary log bound, at most two model calls per turn,
one account source and one optional summary source per participant, 2 KiB account
encoding, six clauses/four feelings, and the existing 4 KiB policy context/output
limits. The example retains at most 4096 bounded policy audits. Current logical
time is exact; this is not a real-time scheduler. Old saved demo/assistance replay
keeps its original dispatch. No migration or reinterpretation of those records is
required.

Engineering orchestration, isolation and replay are testable here. Genuine
language-model listening, semantic summary quality, calibration, cultural or
accessibility adequacy, real-human validity and helpful-AI uplift remain
**NOT_TESTED**. Recorded fixtures are authored synthetic outputs, not independent
language-model evidence. No paid/live provider, real-person data, live outreach,
UI, deployment or real study is introduced. Later #55–#58 work remains gated on
verified integration of this ticket.
