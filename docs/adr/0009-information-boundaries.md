# ADR-0009: current authority precedes information access

Status: implemented for ticket #12; independent review and integration are separate gates.

## Decision

`app/graph.PolicyService` accepts a bounded proposal with exact actor, recipient,
purpose, operation, memory scope, times, source IDs and a trusted context binding.
Every source and all evidence/supersession ancestors must be currently readable and
carry the requested right. Read, derive, disclose, attribute, aggregate, match,
retain, export and share-on-request remain distinct; unknown evidence returns WAIT.
Source text cannot supply policy instructions or change these fields.

ApprovedContext has unexported fields and a service-specific issuer. It retains a
detached proposal and a digest of the complete current memory snapshot, including
recorded revisions, rights, lineage and tombstones, not merely text. Byte-identical
corrections still invalidate old authority. Revalidation checks the original
binding, trusted authority and fresh journal state before supplying SafeContext and
again after a writer returns. Different consumers/purposes need fresh decisions.
Capabilities are process-local references, not serialized bearer tokens; restarting
or replacing policy/grants requires a new service and approvals. There is no mutable
policy promotion endpoint. Unrelated memory changes conservatively invalidate them.

HWS binds namespace/world/branch/run/operator/principal and a configured caller,
view kind and purpose. Approvals must match the current runtime revision and virtual
time, so advancing time expires them even without a memory write. Operational
recorded-as-of cannot exceed the trusted clock. Explicit grant removal takes effect
on the next access. Grants and shared capabilities have concurrent race tests.

ActorObservation contains only the scenario's genesis allowlist. ActorSelfState
contains the actor's own fictional initial profile and, when initialized, current
scalar drive/confidence values. It excludes other actors, causal receipts/hashes,
future events, research labels and scores. Current memories use approved contexts;
ongoing perception/action delivery belongs to #11. Unknown runtime checkpoint data
fails closed. ExternalAgentView contains only approved memory items, no self or
GodState. ResearchGodState requires a separate exact-realm research/read grant;
external callers cannot request it or cause its reader to execute. The graph-only
second host still cannot import HWS or simulator under the repository import guard.

## Disclosure and output

Verified simulated humans may voluntarily disclose their own fictional information
with explicit recipient/disclose rights. Every ancestor must also be their own
subject and observation. This does not grant the recipient general read access.
An assistant cannot disclose another principal's restricted information, including
through a derivative labelled as self-information. Trusted ingestion supplies the
observer/subject/rights; policy does not infer attribution from arbitrary prose.

SafeContext preserves observer, subject, memory kind, confidence, logical times and
lineage. It omits other audiences' grants, system metadata and research scores.
The downstream fake writer can select only bounded UTF-8 quotation spans from this
context. ValidatedOutput retains immutable attribution and confidence for each span;
unknown sources, changed metadata and malformed spans cannot add secret text.
This is a quotation protocol, not a proof that arbitrary generated paraphrases are
private. No model calls, provider adapters or action execution are implemented here.
A callback runs outside transactions. Revocation during it prevents output, but
cannot recall context bytes that the callback already received while authorized.

Cross-perspective abstraction/comparison is always WAIT. There is no enable flag
or source-count/posterior/coalition threshold. Versioned AbstractionAssessment is
research-only: missing evidence is attribution_unknown and supplied evidence still
is not a production privacy proof. Separately approved contexts cannot be combined
under inherited authority; a new abstraction proposal is refused without attempting
to prove that differencing is safe.

## Storage and audit

Forward migration 003 enables and forces RLS on private/research payloads and the
runtime checkpoint payload table; observable RLS is also forced. Restricted readers
start with zero rows. A migration administrator provisions exact authenticated
`session_user` + actor + namespace + payload-class rows in reader_scopes, itself
forced-RLS and self-visible only. Neither runtime writer nor readers may create
mappings. SET ROLE or custom GUC/header values cannot substitute another login.
Runtime payloads remain writer-only; research access goes through application grants.
The trusted service writer has an explicit all-row policy and is not a table owner.
FORCE RLS also filters non-superuser owner SELECT, but schema administrators/owners
can change policies: real runtime credentials must not own schema (#14). Mapping
revocation affects subsequent statements; old MVCC snapshots and previously returned
bytes are not retroactively erased. Purge does not promise erasure from WAL/backups.

Policy approval, revalidation, writer output and direct actor/research/external view
access require a DecisionRecorder. PostgreSQL records versioned private journal
events with binding, stage, ALLOW/WAIT and request-relative clause ordinals only.
No source text, hidden identities, grants, raw infrastructure errors or labels enter
the audit record. Audit failure strips approval/context/output and fails closed.
Random audit IDs are operational identifiers, not simulation RNG or logical replay
state. Invalid permit checks that return before policy do not retrieve data; direct
view denials and policy decisions are audited. The audit is not a disclosure log of
private source contents.

## Limits and acceptance mapping

Context: at most 16 sources, 4096 text bytes and 32 KiB encoded metadata. Output:
1–16 spans, 4096 text bytes and 32 KiB encoded output. Existing 4096-byte memory
payload / 512-record scope caps remain. ViewService accepts at most 128 configured
grants and nine distinct operations each. These are bounded fixture limits, not
long-horizon performance evidence. #10 owns bounded providers/billing; #11 must size
runtime receipts/checkpoint and memory limits deliberately, without hiding failures.

| Requirement | Executable evidence |
| --- | --- |
| Explicit own disclosure; direct and indirect assistant denial | TestExplicitSyntheticDisclosureAndAssistantBoundary |
| Unknown binding, scope, recipient, rights, future and label denial | TestUnknownBindingRightAudienceAndFutureDeny; TestActorResearchExternalBoundaries |
| Opaque capability, cache, revocation and injected output | TestCapabilityRevocationAndOutputInjection; TestCapabilityInvalidatesOnSameTextRevisionAndAudienceChange |
| Exact runtime epoch/time and current authority | TestApprovalExpiresOnRuntimeRevisionOrTime; TestViewScopeBindingAndCurrentRuntimeRevocation |
| Every right; hostile source text cannot request GodState | TestPolicyChecksEachRightAndUntrustedPrompt |
| Sanitized denials, abstraction always WAIT | TestPolicyDenialHasNoSecretsAndAbstractionNeverApproves |
| Required audit and immutable attribution | TestPolicyAuditAndAttributionAreMandatory |
| Bounds, invalid spans, shared capability race checks | TestPolicyBudgetsMalformedOutputAndSharedCapability |
| Own-state allowlist, research/external separation, grant revocation race | TestActorResearchExternalBoundaries; TestActorSelfStateStripsOtherDynamicsAndCausalHashes; TestConcurrentViewsAndGrantRevocation |
| Real storage: scoped reads, FORCE RLS, default deny, upgrade | TestScopedReaderIntegration and its subtests; migration-check fresh cluster + dump/restore |
| Real policy audit and purge during writer, future source denial | TestInformationPolicyIntegration |

`make verify` includes all normal/race tests, import guards, vet, fresh PostgreSQL
migration/integration/dump-restore/purge checks and command builds/help. Every database
run requires a fresh cluster because test LOGIN roles are cluster-scoped. Current
run evidence and exact SHAs live on the ticket PR, not an assumed pass in this ADR.

Trusted composition is not network authentication or a Go security sandbox. Raw
stores, constructors and researcher capabilities must never be exposed to clients
or model tools. Real credential binding is #14. Original PRDs and the complete HWS
§6 registry remain UNVERIFIED; this ticket makes no research/privacy certification.
