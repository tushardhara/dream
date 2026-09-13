# ADR-0001: reusable domain within a Go modular monolith

Status: accepted by epic #1 revision 3; implementation foundation in #2.

One Go module, PostgreSQL, provider-independent typed adapters and eventual gRPC
with generated HTTP/JSON gateway. cmd composes dependencies. No Rust runtime,
graph DB, mandatory vectors, Kafka, Kubernetes, microservices or consumer UI.

| Package | Owns | Dependency boundary |
| --- | --- | --- |
| core | principals, claims/evidence/time, graph/memory/rights contracts | core and non-transport standard library only |
| app/graph | reusable ingestion/query/correction/revocation/export | core, app/graph; no simulator or concrete adapters |
| simulator | world/run/branch/step, latent dynamics, virtual time/RNG | core, simulator; no adapters/transport/evals |
| app/hws | operations/leases/jobs/orchestration | core, simulator, app/graph, app/hws; consumer-defined ports |
| adapters | Postgres/model/auth/transport/telemetry implementations | implements consumer ports; no domain duplication |
| evals | independent labels/calibration/comparisons | generation must not import evals |
| cmd | configuration/composition | no domain decisions |

The source parser checks imports in every Go file independent of custom tags,
GOOS or GOARCH, including tests. It excludes testdata/vendor/hidden directories.
These are fixture/dependency directories, not permitted homes for production
code. Local imports in protected packages are allowlisted, preventing indirect
escapes through local helper packages; external dependencies are conservatively
excluded until explicitly reviewed. net packages are transport and forbidden.
This is an import guard, not proof of purity or all semantic invariants. Compiling
all target platforms/tag combinations is not claimed; import checking covers them.

World IDs stay in simulator. Future hosts can import core/app/graph without world
creation. Durable state later starts with versioned events, generic event streams
and simulator associations; logical time and operational lease time are distinct.
Baseline authorization/budgets/audit arrive with their consumers. #12 precedes
models. No placeholder network authentication is introduced in this scaffold.

Alternative rejected: a simulation-centric generic framework would make worlds
compulsory for other hosts. Microservices/extra storage would add operational
surface without this initial scope requiring it. Future tickets implement behavior;
empty package markers make no business-function or scientific-validity claim.
