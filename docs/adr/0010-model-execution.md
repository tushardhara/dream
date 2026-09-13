# ADR-0010: bounded model attempts and exact recorded artifacts

Status: implemented for #10; review/integration evidence belongs on its PR.

## Application contracts and information boundary

`app/hws` owns provider-independent appraisal, interpretation, reconciliation and
candidate-generation contracts. A v1 response is a bounded list of enum/value/
subjective-confidence/evidence proposals with the requesting observer. It cannot
contain arbitrary instructions, executable tools, an action body or benchmark
labels. Codes are capability-specific; source references must occur in the approved
input. #11 owns actual action selection, voluntary disclosure and outcome updates.
New schema/capability/prompt/policy versions fail closed until explicitly supported.
Model identity and conservative price ceilings are supplied by trusted composition;
no mutable model alias is treated as a reproducibility or pricing guarantee.

ModelGateway first obtains `ViewService.ModelContext`: an actor's current
`InternalContext` + own-recipient `derive` approval for purpose `simulation`.
Disclosure/export permits cannot be repurposed. Scope binds the exact operator,
namespace, world, branch, run and principal. Only safe items enter ProviderInput;
no raw runtime snapshot, future schedule, GodState, labels, role headers, credentials
or caller-controlled instruction template is accepted. Policy executes before
persistent retrieval, before each attempt and after generation. The source snapshot
digest and logical time are retained privately in the intent, not sent to a provider.
They are rechecked inside reservation, successful completion and canonical commit
transactions, including same-text corrections and changed rights.

`ModelGateway.Apply` revalidates current actor policy before the deterministic
transition handler and again after it, before opening the commit transaction. It
attaches the durable result key/hash to a single runtime step. Raw Runtime/Store
ports remain trusted host components, not network authorization endpoints. #14 must
not expose constructors or let client code choose handlers/raw state. #11 must still
apply the distinct action/disclosure policy; a candidate code grants no action right.

## Durable state and reservations

Forward migration 004 adds model budgets, request metadata, restricted payloads and
application references. Every budget configuration, attempt/reservation and completion
begins with a versioned journal event in the same short transaction. Budget configuration
is immutable per exact run. Request identity includes scope, principal, key, all versions,
input and source snapshot/time; a changed request cannot reuse an existing key.

Before every external attempt, storage reserves the full configured token and spend
ceilings and increments a durable attempt/fence. It enforces total per-run ceilings,
max in-flight and max attempts. No failure refunds the reservation, including unknown
usage, timeout and a process dying after sending a request. Active requests reject
competing callers. Expired attempts can be retried with a new fence and another full
reservation; old workers cannot overwrite them. Counts survive gateway/store restart.
Rate limit/outage retries use bounded, cancellation-aware waits. Unknown costs,
unsupported capabilities, malformed/refused output and exhausted limits fail closed.

The gateway reserves the entire 48,000-byte supported wire ceiling as input tokens,
plus the output ceiling; this includes JSON escaping/schema overhead without a
model-tokenizer estimate. Configured integer
microcurrency/token upper prices must fit the per-attempt spend reservation. Reported
usage beyond the input/output/token/spend ceiling fails closed. The full reservation
is charged to the local ledger even for cheaper or uncertain results. This intentionally
trades utilization for safety; it is not an invoice reconciliation service. Provider
pricing/configuration must be owner-supplied before any live use. Fake test prices are
synthetic budget units, not a real provider price or a claim that money was spent.

Responses and schema-validated outputs are hash-bound and persisted before any
transition can commit. A lost handoff after successful save reuses the exact stored
artifact without another provider call. Canonical commit atomically checks the stored
hash, current source snapshot/time and single artifact consumption, then records its
application beside the new checkpoint. Same operation retries return the prior
receipt; a second operation cannot apply the artifact again. Unknown failure before
response persistence can repeat external work/billing; it cannot double-apply state.
There is no network call or application callback held inside a database transaction.

## Offline adapters and real transport boundary

`adapters/model.Fake` is deterministic. `Recorded` owns detached, validated artifacts
indexed by canonical request digest, preserves output bytes exactly and has neither
a network client nor fallback generation. Missing/corrupt/different-version recordings
deny. Each durable artifact is marked recorded and retains its original response's
`deterministic_fake` or `fresh_stochastic` generation label. Fresh HTTP generation is
never advertised as exact replay, including identical seed/temperature/model names.

The optional OpenAI Responses adapter uses a fixed structured-output schema, no tools,
`store=false`, no background job, disabled truncation and no automatic redirects or
environment proxies. It never reads environment credentials. Zero configuration fails;
mock mode accepts only numeric loopback HTTP and forbids credentials. Live mode needs
explicit owner runtime configuration and a key, and is not wired into any default
command/test. The adapter was tested only with httptest servers. No live calls ran.
The reviewed contract follows [official structured-output documentation](https://developers.openai.com/api/docs/guides/structured-outputs): schema output still requires
handling refusals/incomplete responses and application-level validation. Reasoning
items are not retained. Raw error/refusal bodies are discarded; only typed status is
journalled. This is a wire-conformance test, not provider/model availability evidence.

## Revocation, migration and limits

Request/result payloads are stored under the actor's existing memory scope and
ordinary source lineage. Revoking any source purges every affected attempt/result.
Application references propagate tombstones to dependent runtime checkpoints and
their descendants; revoking a run purges all of its model attempt versions. Purged
results cannot be replayed. Metadata/budget reservations remain as an operational
record. `model_payloads` has enabled, forced RLS and writer-only access; ordinary
readers cannot obtain prompts/responses. Dump/restore asserts six forced-RLS tables,
all four schema versions and absence of tombstoned model payloads. Original migrations
are unchanged. Every integration test uses a fresh PostgreSQL cluster.

Caps: four capabilities; one to eight findings, each with one to sixteen unique
source IDs; input 16 sources / 4096 text bytes / 40,000 encoded bytes; output 16,384
bytes; transport request 48,000 bytes and response 65,536 bytes; combined stored
intent/artifact 65,536 bytes. Oversize combinations fail closed. Routes allow at most
three attempts, eight active attempts per run, 8192 output tokens and a one-minute
attempt timeout. Default tests use much smaller fake fixtures. Existing runtime and
memory caps remain; #11 still must size MaxReceipts32/checkpoint4096/memory limits.

Application context cancellation and database deadlines stop new usable results.
A provider is expected to honor context cancellation; a remote server may keep working
or billing after a client timeout/disconnect. In-flight caps bound authorized attempts,
not unknowable remote execution after an uncertain network failure. Prior delivered
context, old MVCC snapshots, WAL and backups cannot be retroactively recalled. Raw
ports are trusted composition, not a sandbox or permission to execute a paid run.

## Acceptance mapping

| Criterion | Tests |
| --- | --- |
| Typed versions/enums/bounds/observer/provenance, no executable prose | TestModelTypedOutputNegatives; FuzzDecodeModelOutput |
| Distinct derive capability, current actor/namespace policy | TestCognitionCannotReuseDisclosureCapability; #12 view/policy negatives |
| Deterministic fake, exact detached concurrent replay, no fallback | TestFakeRecordedExactAndConcurrent |
| Mock wire schema, hostile context, refusal/incomplete/malformed/rate/outage | TestResponsesMockConformanceAndFailures |
| Default-deny live configuration, timeout and response byte cap | TestResponsesDefaultDenyTimeoutAndSize |
| Durable response before commit; lost handoff; no double application | TestModelIntegration/DurableResponseCrashAndExactlyOnceTransition |
| Reserved retries, unknown usage/price and immutable limits | RetriesReservedBudgetAndUnknownUsage |
| Token/spend exhaustion and policy before outbound request | TokenAndSpendExhaustionBeforeProvider; PolicyBeforeOutboundContext |
| Attempt/run deadlines, late success rejection | RunAndAttemptDeadlinesDenyLateSuccess |
| Concurrent run limits and context cancellation | ConcurrentReservationAndCancellation |
| Expired-attempt fences and restart-persistent attempts | ExpiredAttemptFencingAndScopeIsolation |
| Same-text correction blocks canonical application | ContextChangeBeforeCanonicalCommitDenies |
| Permit removal during handler blocks canonical commit | PolicyRevokedDuringTransitionDeniesCommit |
| Provider callback revocation; physical artifact purge; derived run invalidation | RevocationDuringProvider; DurableResponseCrashAndExactlyOnceTransition |
| Restricted model-table access, forward upgrade and restore/purge | TestScopedReaderIntegration; V3UpgradePreservesExistingRun; mandatory migration-check |

`make verify` runs normal/race tests, architecture guards, vet, Python sentinels,
fresh-PG integration/migration/restore/purge and builds/help. Optional live-provider
and soak targets remain closed. Scientific realism, real-human validity, full source
traceability and complete HWS §6 names remain unverified, not model gateway claims.
