# ADR-0005: atomic virtual-time checkpoints with fenced writers

Status: proposed by #6; independent review required.

`simulator/runtime` owns pure virtual-time input ordering, the versioned RNG and
logical transitions. `app/hws.Runtime` coordinates a consumer-defined RuntimeStore
and a separate OperationalClock. `adapters/postgres.Store` implements persistence,
using the existing generic journal and simulator associations. No world/run IDs
are added to core. No provider, API server or production worker loop is added.

## Host interface and operation boundaries

A host validates a `hws.Manifest` and calls `CreateRun`. The manifest identifies
trusted actor/namespace, world/branch/run, validated scenario genesis, hard budgets,
and maximum operational duration. CreateRun persists a versioned generic
`scenario.genesis` event, the validated restricted payload and initial paused
checkpoint atomically. An identical retry returns the current checkpoint; a
changed manifest at the same world/branch fails. V1 permits one run per branch;
new branches and replay/counterfactual semantics belong to #13.

Acquire a lease, then use `Runtime.Execute(scope, lease, key, command)`:

| Command | Commit semantics |
| --- | --- |
| resume | paused → running, same logical state |
| step | one queued input while running; receipt Done=true |
| run-until | at most one input per call; repeat the identical key/command until Done=true |
| inject | one strictly future observation inserted durably; caller-supplied stable ID |
| pause | takes effect at its successful commit; interrupts any pending run-until |
| cancel | terminal at its successful commit; interrupts pending run-until |

Command keys are actor/namespace/run scoped. Request digest includes the complete
command, not lease/PID/wall time. Completed retries return the original receipt;
changed requests fail. A pending run-until retry continues from its latest durable
checkpoint, never restarts. Only one unfinished operation exists per run (including
a database unique partial index). A different operation conflicts unless it is
pause/cancel. Interrupted receipts are terminal and cannot resume the old command;
explicitly resume then issue a new run-until key. There is no ambiguous last-run or
last-command selection. The host drives bounded calls; no implicit unbounded loop.

The handler receives deep-cloned state, current input, an explicit virtual Clock
and a named Random object. It runs before a transaction starts. It cannot mutate
the stored/current state by modifying its input. It returns bounded next-state
text and up to 128 strictly future observations. This ticket uses deterministic
fake handlers only; synthetic human dynamics and typed actions are #7/#11. The
handler is a trusted pure extension point, not untrusted code or a model boundary;
full research state must never be sent to a model (#12 precedes model integration).
Application services are not authenticated endpoints; #14 composes trusted scope.

## Ordering, RNG and deterministic identity

Queue order is `(logical at, priority, ID ASCII)`: releases priority 0, other
scenario/injected observations and reservations priority 1. Original scheduled
IDs are preserved. Reservation releases use `release:` plus SHA-256 of the source
ID, allowing long source IDs without exceeding core's ID length; any expanded-ID
collision fails closed. At equal time, releases occur before allocations, matching
#5 half-open capacity semantics. Same-time ordinary events preserve #5 `(at,ID)`
order. Run-until includes every input at its target time; with no input before the
target it advances only the logical clock. Empty queue completes the run.

Injection/generated input must have `at > current logical time`. Retroactive and
same-time injection are rejected; caller must eventually create an explicit branch
via #13 for counterfactual past changes. Injected IDs cannot reuse any previously
seen ID. Reservations come from validated genesis; injected resource mutations
are rejected in v1. Releases/allocations update bounded capacity in the checkpoint.

`sha256-counter.v1` draws the high 64 bits of SHA-256 over UTF-8
`"sha256-counter.v1\0" + stream ID + "\0"`, followed by big-endian uint64 seed and
position. Positions start at zero; streams advance independently. At most 64
streams and 1,024 draws per transition; position overflow fails. The version and
independent Python-derived golden vector pin the algorithm. This is simulation
randomness, not token generation or a promise of deterministic fresh providers.

Every committed transition stores its input, named positions/values drawn, output,
generated inputs and the updated checkpoint in one transaction with a journal
event, outbox row, simulator association and operation receipt. Logical state
hash is SHA-256 of struct-order JSON (Go sorts map keys). It includes version,
genesis hash/payload, budgets, time, queue, seen IDs, RNG positions, resource state,
status and bounded handler state. Operational deadline, holder/fence/PID, run/branch
identity and journal-envelope IDs are outside the hash. Therefore different run
IDs and worker restarts can match the same logical trajectory. This is not #13's
snapshot/replay protocol, and mutable provider generation is not implemented.

## Lease and commit protocol

Each actor/namespace/world/branch has one row-locked lease. Claim requires an
expired/unheld lease and increments a monotonic bigint fencing token. Even the same
holder must renew explicitly; overlapping claims fail. Renewal requires exact
holder/token and an unexpired lease. TTL is bounded to 1ms..1min. PostgreSQL
clock_timestamp is authoritative, read after acquiring the row lock; service time
is merely an optional early-work check via OperationalClock. A host clock ahead of the database may skip computation, but its expiry hint cannot terminate a run: the database rejects the hint while its own deadline remains open. A regression test exercises that clock skew. Every claim/renewal
appends a v1 operational audit record with explicit run, token and wall times.

A checkpoint write requires exact holder/token, unexpired lease and optimistic
revision. It rechecks expiry after the writes before committing. The branch row
stays locked throughout; a replacement cannot claim then be overwritten by an old
writer. An expired writer's computed output is discarded. Immutable genesis,
engine/RNG version and budgets cannot change in a checkpoint; logical time/counters
cannot move backward or advance more than one transition per commit. The adapter
is an internal trusted port, not an externally exposed arbitrary-state API.

#4's advisory lock 41001 remains necessary for generic journal sequence/commit
ordering and projector checkpoints. This ticket narrows runtime use to the short
write phase: handlers, leases and cross-world computation do not hold it. Different
worlds can compute concurrently; journal writes are still serialized. Removing it
without redesigning the projector's sequence semantics would be a correctness
regression. This is an explicit single-host throughput limitation for #15, not a
claim of parallel database commits or performance certification.

## Budgets, interruption and restricted storage

Per-run steps and applied-input events are bounded to 1..1,000,000; logical horizon
must fit genesis. These logical event counters do not count operational pause or
lease audit records. Queue ≤4,096, seen IDs ≤16,384, handler state ≤4,096 bytes,
checkpoint/event payload ≤4 MiB, generated inputs ≤128 per transition. Invalid
handler outputs fail the command with no advancement; the host must fix or cancel
rather than retry the same invalid handler indefinitely. Explicit step/event/
horizon exhaustion commits terminal `budget` with no over-budget transition.
Maximum operational duration is positive and ≤24h, fixed at CreateRun by database
time, never extended by resume/reclaim. Expiry at the commit boundary replaces a
computed transition with a budget-only checkpoint, preserving RNG/time/counters. If the deadline crosses during SQL writes, those writes roll back and the service makes one bounded commit retry that writes only the budget checkpoint, without re-running the handler.
A caller context cancelled before commit produces no effects. A stuck handler is
outside the transaction: its lease expires and a new process can reclaim; the
host must terminate an uncooperative process. This library is not a process
supervisor and makes no terminal-disconnection persistence claim.

Schema v2 adds runtime head, payload, command and lease-audit tables. Only the
existing writer role can access these tables. No actor/private/research reader
role implicitly receives them. The runtime event chain uses generic parent lineage
and restricted research payload class with explicit self derive permission.
Revocation purges checkpoint payloads and operation requests (which may contain
private injected text), while generic tombstones prevent load/continuation even
before a caller retries. Lease/head/audit identifiers remain as operational
metadata; they do not contain the scenario or injected text. The backup/restore
gate now checks runtime purges too. Pre-revocation backups still require retained
tombstone replay before use, as documented in ADR-0003; no historical erasure claim.

## Validation and remaining work

Tests cover clean-vs-killed child-process trajectory hashes, simultaneous duplicate
workers, optimistic conflicts, expired-owner reclaim/stale commits/renewal,
expiry during a delayed transaction, injected SQL fault rollback of RNG/output/
receipt, pause/cancel boundaries, run-until recovery, deadline budget override,
restricted role grants, revocation purge/resume denial, and forward v1→v2 upgrade
preserving existing journal data. Pure tests cover tie order, inclusive targets,
release-before-allocation, named RNG independence/positions/golden/draw cap,
injection rejection, logical budgets and handler-failure immutability.

Claude's #5 follow-ups now assert specific parser guard codes and the exact
foreign-knowledge path/message. An older storage test now scopes learned-at rows
to its own fixture and checks the exact (learner,time) pairs; it no longer assumes
there are no other simulator consumers in the database. No assertion was removed.

CLI execution, persistent worker hosting and network APIs are not included in #6;
application operations are callable Go services. There are no LLMs, live providers,
paid runs, production migrations, deployments, real-person data or scientific
realism claims. Human dynamics, retrieval, full runtime information policy,
replay, API/auth and operational performance gates remain with their tickets.
