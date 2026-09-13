# ADR-0004: bounded scenario v1 and genesis boundary

Status: proposed by #5; independent review required.

`simulator/scenario` owns the pure schema, validation, canonical encoding,
capability gate, genesis event and actor projection. `adapters/scenario` owns YAML
syntax only; the import guard still forbids YAML dependencies in simulator/core.
The YAML organization maintained v3 package is pinned at `go.yaml.in/yaml/v3
v3.0.5` (stable Node API, no alias expansion used). Upstream:
https://github.com/yaml/go-yaml/tree/v3.0.5. It is not a transport or provider.

## Offline operator command

```
go run ./cmd/hws scenario validate examples/scenarios/quiet-overlap.yaml
cat examples/scenarios/quiet-overlap.yaml | go run ./cmd/hws scenario validate -
```

The built `bin/hws` returns exit 0 plus `{valid:true,version:1,scenario_hash:...}`;
exit 1 plus `{valid:false,errors:[{path,code,message}]}` for invalid/input errors;
exit 2 for usage. `go run` itself wraps nonzero exit codes. Errors identify JSON
paths for fields and sequence indexes. Syntax errors use `$`; limit errors during
syntax-tree walking use child indexes. File/syntax errors do not echo input data
or filesystem errors. This local operator interface is not authenticated actor
access. It consults no environment credentials, database, network or model.

## Strict v1 schema

See `examples/scenarios/quiet-overlap.yaml` for every section and Go types for
field spelling. All fields are required except explicit `omitempty` fields:
`valid.end`, reservation `resource`, `units`, `until`. Empty sets are written `[]`;
null, unknown fields, duplicate fields, coercions, merge keys, explicit tags,
anchors/aliases and multiple documents are rejected. Integers use decimal notation,
not hex/octal/underscores; IDs use core's case-sensitive ASCII ID rules. There is
no implicit date, clock, seed, adult age, horizon or world default.

- `world`: ID, exact uint64 seed and positive logical horizon (nanoseconds).
- `public`: 2..24 fictional adults (ages 18..120), structural groups, resource
  capacities/available units, observer-owned public facts with exact grants.
  Public means eligible for visibility, not automatically known or authorized.
  Group membership is explicit public structural input, not inferred affiliation.
- `actors`: exactly one per human; private observer-owned facts, initial knowledge,
  evidence-backed memories and perspective-owned relationships. Each knowledge
  record requires self read for purpose `simulation`, learned at zero; foreign
  private references are rejected. Private bootstrap grants stay in self contexts;
  later sharing requires separately authorized events. Memory/relationship sources
  must already be known. Their bootstrap text is a synthetic initial assertion,
  not a computed inference or calibrated/global relationship truth. No derived
  rights are fabricated. Runtime conversion into core records must preserve the
  source lineage and apply core derivation rights if computing new claims (#8/#9).
- `research`: synthetic emotions/drives, bounded by existing simulator contracts,
  and opaque fixture-design labels. Labels are not scientific outcome scores.
- `future`: actor-addressed observations or resource reservations. Event times
  satisfy `0 < at < horizon`; reservations are `[at,until)` with positive units,
  `until <= horizon`. Releases precede allocations at the same logical instant;
  overlapping units cannot exceed initially available capacity. Checks avoid
  int64 addition overflow. Unavailable initial units remain unavailable throughout
  this v1 reservation validation; no replenishment dynamics are implied.

All entity IDs (humans, groups, resources, facts, memories, relationships, labels,
scheduled events) are unique within the scenario. Actor/latent sections reference
human IDs and cannot repeat. Fact valid intervals begin at zero; an optional end
is positive and no later than the horizon. Omitted end means no asserted expiry.
Initial occurred/learned time is zero; no wall-recorded time is in scenario input.
The future runtime records ingestion time separately.

YAML input is bounded to 1 MiB before parsing. The syntax tree is traversed without
expanding aliases, with maximum depth 24 and 20,000 nodes. Each collection is capped
at 1,024 items, individual domain strings at 4,096 bytes, aggregate domain entities
at 1,024, canonical JSON at 1 MiB. Programmatic inputs also have collection/node
budgets. The YAML library first builds a size-bounded tree; the domain depth limit
is applied immediately afterward. This is not a streaming constant-memory parser.

## Canonical identity and schedules

Canonical v1 is UTF-8 Go struct-field-order JSON, no insignificant whitespace,
standard JSON escaping, signed numeric zero normalized positive, empty sets `[]`.
All collections are sets: canonical JSON byte order of each normalized member,
except future events, which use `(at ascending, ID ASCII ascending)`. Unique IDs
make ties total. Permission grants and group members are sets too. SHA-256 hashes
these exact bytes, including world seed, private/research input, capabilities and
future schedule; no wall time, YAML formatting/comments or input ordering enters
identity. The hash is an operator artifact and never included in actor views.
The full payload/hash must remain in restricted research storage when #6 persists
it. No partial actor view or stripped scenario is interchangeable with this hash.
The committed fixture has byte and SHA-256 goldens (digest independently generated
with Python hashlib); property tests permute inputs and preserve large integers.

Schedule sort specifies deterministic ties, not a runtime implementation. Future
#6 must preserve this order plus release-before-allocation capacity semantics;
#7/#11 implement behavior. Gaps with no scheduled event permit WAIT but do not
force a decision or certify realism. The three-adult fixture has overlapping
2-person groups and a 12-hour quiet initial period; no provider is invoked.

## Genesis, views and execution gate

`Scenario.Genesis(engine)` returns a versioned `scenario.genesis` event at logical
zero with world ID, canonical payload and payload hash, only after validation and
engine capability checks. `Genesis.Validate(engine)` independently checks envelope,
exact canonical bytes, hash/world consistency and capabilities. This includes
rejecting trailing bytes, duplicate JSON fields and noncanonical input. The event
is simulator-local, not a mutable world initializer or database write. #6 must
persist validated genesis before materializing state, using #4 generic journal
plus separate simulator association. Schema validation alone never starts a run.

Engine support requires v1 plus declared and inferred capabilities: `genesis.v1`,
`resources.v1`, `memory.v1`, `relationships.v1`, `latent.v1`, `schedule.v1` as used.
Unknown declared capabilities are valid descriptive requirements but execution
fails unless the supplied engine explicitly supports them. Omitting `requires`
entries cannot disable inferred gates. The CLI validates syntax/domain only; no
engine exists yet and it does not claim that a scenario is executable here.
Test engines are explicit fakes, not runnable simulations.

`Scenario.View(actor)` is a deep-copy, allowlisted initial projection: public
human/group/resource structure, authorized known public/self facts, and self
memories/relationships. It has no seed, horizon, hash, schedule, labels, latent
state or other actor section. Unknown actors fail. Caller authenticates actor
selection; full research Scenario is not a safe model input. This is a bootstrap
boundary, not #12's trusted runtime information service or revocation projection.
No real-person data appears in fixtures. Mutation of a returned view cannot alter
scenario state. Arbitrary author-written text cannot be proven free of secrets;
this protects structural boundaries, not semantic classification of prose.

## Evolution and remaining gates

V1 rejects other versions and unsupported event kinds. There is no silent default
migration, automatic downgrade or historic decoder. A future schema migration
must be a separately versioned, reviewed conversion that validates source and
target, preserves the original source hash and records a new genesis lineage/hash;
old fixtures and goldens remain available. Never mutate an existing run's genesis.
New execution fields must add inferred capabilities and negative old-engine tests.

No database migrations, actors, scheduler, provider calls, dynamics, snapshots,
replay guarantees or scientific validation are implemented by this ticket. The
source PRDs remain unavailable. The canonical format is versioned but has not been
certified cross-language; any second implementation must match the byte goldens.
