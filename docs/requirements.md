# Requirements matrix and source gaps

Authority is the executable summaries in epic #1 and tickets #2–#17. Original
PRD attachments are unavailable and are not published here. The source reference
register below enumerates every cited section/ID but does **not** invent per-section
meaning from missing text. Full source traceability remains UNVERIFIED. The String
Between Us supplies intent, not additional implementation scope.

## Executable invariant mapping

These labels E1–E12 refer to epic correctness rules, not invented IHG source IDs.

| Requirement | Engineering owner | #2 evidence / deferred gate |
| --- | --- | --- |
| Reusable core; no compulsory worlds; package ownership | #2, #3, #9 | TestRepositoryBoundaries, TestRules; second-host proof deferred #9 |
| E1 versioned durable events, scoped idempotency | #3, #4 | DEFERRED; no persistence implemented |
| E2 distinct occurred/valid/learned/recorded times and corrections | #3, #4, #8 | DEFERRED; temporal/as-of tests required |
| E3 injected time/RNG, stable order, canonical hashes | #5, #6 | DEFERRED; deterministic fixtures/runtime tests |
| E4 recorded/fake exact replay; fresh generation stochastic | #10, #13 | DEFERRED; no replay claim |
| E5 fenced writer, crash-safe checkpoints, no provider calls in transactions | #4, #6, #10 | DEFERRED; recovery/duplicate-effect tests |
| E6 auth/manifests/audit/budget negatives with first consumers; #12 before models | #4, #6, #12, #15 | workflow fixes dependency order; no consumers yet |
| E7 fictional own-experience disclosure differs from IHG restricted-context policy | #12, #11 | DEFERRED; explicit policy tests |
| E8 abstraction disabled by default; unknown evidence denies/WAITs | #12 | DEFERRED; no privacy guarantees claimed |
| E9 immediate revocation and derivative invalidation; replay cannot resurrect purge | #4, #12, #13 | DEFERRED; retention/backups must be documented |
| E10 trusted credentials; no exposed placeholder auth | #14 | command scaffolds open no network sockets; actual auth DEFERRED |
| E11 bounded daily state; offline heldout candidate promotion | #7, #11, #16 | DEFERRED; no online training/promotion |
| E12 explicit paid/live budget/infrastructure configuration | #10, #15, #17 | default CI has no live calls; optional targets fail closed |
| Observer/evidence/uncertainty/time preserved; no global relationship truth or engagement objective | #3 onward | DEFERRED domain/behavior tests |
| Independent evaluation; generation must not import evals | #2, #16 | TestRules forbids simulator/app/evals imports; research harness deferred |
| Backend-only, PostgreSQL-only local compose | #2 | compose.yaml and explicit CLI scaffolds |
| Negative architecture invariant including tagged/platform source | #2 | TestIllegalTaggedImport and TestPlatformFilesAndMalformedSource; testdata/illegal/core/bad.go |
| Clean checkout, race/build/help, PR and integration CI | #2 | make verify, .github/workflows/verify.yml; remote results in PR |
| Durable SHA-bound review/integration; owner-only main | #2 | AGENTS.md, agent-workflow.md, PR template; independent review pending |
| 24 humans / 8 groups / 30+ edges after 2–4 actor fixtures | #5, #17 | DEFERRED; no simulation implemented |
| 12-month virtual horizon distinct from real-time 30-day study | #17 | DEFERRED; live study NOT RUN |
| 200 scenario families / 10,000+ scale; preregistered ablations/holdouts/calibration | #16, #17 / later research | DEFERRED research targets; not bootstrap acceptance |
| Real-human transfer/validity; UI entry by owner engineering approval | #16, #17 / owner | NOT TESTED without real-human dataset; UI implementation excluded |

## Source reference register

Every row is a missing-source gap, not a verified section-level interpretation.
Ticket summaries constrain implementation in the interim; obtain authorized source
access to complete traceability without publishing originals or private data.

| Document | Section / invariant ID | Status |
| --- | --- | --- |
| HWS PRD v0.1 | §2 | UNVERIFIED: original text unavailable; summarized scope above |
| HWS PRD v0.1 | §3 | UNVERIFIED: original text unavailable; summarized scope above |
| HWS PRD v0.1 | §4 | UNVERIFIED: original text unavailable; summarized scope above |
| HWS PRD v0.1 | §5 | UNVERIFIED: original text unavailable; summarized scope above |
| HWS PRD v0.1 | §6 | UNVERIFIED: original text unavailable; summarized scope above |
| HWS PRD v0.1 | §7 | UNVERIFIED: original text unavailable; summarized scope above |
| HWS PRD v0.1 | §8 | UNVERIFIED: original text unavailable; summarized scope above |
| HWS PRD v0.1 | §9 | UNVERIFIED: original text unavailable; summarized scope above |
| HWS PRD v0.1 | §10 | UNVERIFIED: original text unavailable; summarized scope above |
| HWS PRD v0.1 | §11 | UNVERIFIED: original text unavailable; summarized scope above |
| HWS PRD v0.1 | §12 | UNVERIFIED: original text unavailable; summarized scope above |
| HWS PRD v0.1 | §13 | UNVERIFIED: original text unavailable; summarized scope above |
| HWS PRD v0.1 | §14 | UNVERIFIED: original text unavailable; summarized scope above |
| HWS PRD v0.1 | §15 | UNVERIFIED: original text unavailable; summarized scope above |
| HWS PRD v0.1 | §16 | UNVERIFIED: original text unavailable; summarized scope above |
| HWS PRD v0.1 | §17 | UNVERIFIED: original text unavailable; summarized scope above |
| HWS PRD v0.1 | §18 | UNVERIFIED: original text unavailable; summarized scope above |
| HWS PRD v0.1 | §19 | UNVERIFIED: original text unavailable; summarized scope above |
| HWS PRD v0.1 | §20 | UNVERIFIED: original text unavailable; summarized scope above |
| IHG canonical PRD v1.1 | 5 | UNVERIFIED: original text unavailable; summarized invariants above |
| IHG canonical PRD v1.1 | 14 | UNVERIFIED: original text unavailable; summarized invariants above |
| IHG canonical PRD v1.1 | 15 | UNVERIFIED: original text unavailable; summarized invariants above |
| IHG canonical PRD v1.1 | 16 | UNVERIFIED: original text unavailable; summarized invariants above |
| IHG canonical PRD v1.1 | 17 | UNVERIFIED: original text unavailable; summarized invariants above |
| IHG canonical PRD v1.1 | 18 | UNVERIFIED: original text unavailable; summarized invariants above |
| IHG canonical PRD v1.1 | 77 | UNVERIFIED: original text unavailable; summarized invariants above |
| IHG canonical PRD v1.1 | 78 | UNVERIFIED: original text unavailable; summarized invariants above |
| IHG canonical PRD v1.1 | 79 | UNVERIFIED: original text unavailable; summarized invariants above |
| IHG canonical PRD v1.1 | 80 | UNVERIFIED: original text unavailable; summarized invariants above |
| IHG canonical PRD v1.1 | 81 | UNVERIFIED: original text unavailable; summarized invariants above |
| IHG canonical PRD v1.1 | 82 | UNVERIFIED: original text unavailable; summarized invariants above |
| IHG canonical PRD v1.1 | 83 | UNVERIFIED: original text unavailable; summarized invariants above |
| IHG canonical PRD v1.1 | 93 | UNVERIFIED: original text unavailable; summarized invariants above |
| IHG canonical PRD v1.1 | 94 | UNVERIFIED: original text unavailable; summarized invariants above |
| IHG canonical PRD v1.1 | 95 | UNVERIFIED: original text unavailable; summarized invariants above |
| IHG canonical PRD v1.1 | 96 | UNVERIFIED: original text unavailable; summarized invariants above |
| IHG canonical PRD v1.1 | 97 | UNVERIFIED: original text unavailable; summarized invariants above |
| IHG canonical PRD v1.1 | 98 | UNVERIFIED: original text unavailable; summarized invariants above |
| IHG canonical PRD v1.1 | 99 | UNVERIFIED: original text unavailable; summarized invariants above |
| IHG canonical PRD v1.1 | 100 | UNVERIFIED: original text unavailable; summarized invariants above |
| IHG canonical PRD v1.1 | 101 | UNVERIFIED: original text unavailable; summarized invariants above |
| IHG canonical PRD v1.1 | 102 | UNVERIFIED: original text unavailable; summarized invariants above |
| IHG canonical PRD v1.1 | 103 | UNVERIFIED: original text unavailable; summarized invariants above |
| IHG canonical PRD v1.1 | 104 | UNVERIFIED: original text unavailable; summarized invariants above |
| IHG canonical PRD v1.1 | 105 | UNVERIFIED: original text unavailable; summarized invariants above |
| IHG canonical PRD v1.1 | 106 | UNVERIFIED: original text unavailable; summarized invariants above |
| IHG canonical PRD v1.1 | 102A | UNVERIFIED: original text unavailable; summarized invariants above |
| IHG canonical PRD v1.1 | 102B | UNVERIFIED: original text unavailable; summarized invariants above |
| IHG canonical PRD v1.1 | 102C | UNVERIFIED: original text unavailable; summarized invariants above |
| IHG canonical PRD v1.1 | 102D | UNVERIFIED: original text unavailable; summarized invariants above |
| IHG canonical PRD v1.1 | 102E | UNVERIFIED: original text unavailable; summarized invariants above |
| IHG canonical PRD v1.1 | 102F | UNVERIFIED: original text unavailable; summarized invariants above |
| IHG canonical PRD v1.1 | 102G | UNVERIFIED: original text unavailable; summarized invariants above |
| IHG canonical PRD v1.1 | 102H | UNVERIFIED: original text unavailable; summarized invariants above |
| IHG canonical PRD v1.1 | 102I | UNVERIFIED: original text unavailable; summarized invariants above |
| IHG canonical PRD v1.1 | 193 | UNVERIFIED: original text unavailable; summarized invariants above |
| IHG canonical PRD v1.1 | 201 | UNVERIFIED: original text unavailable; summarized invariants above |
| IHG canonical PRD v1.1 | 202A | UNVERIFIED: original text unavailable; summarized invariants above |
| IHG canonical PRD v1.1 | 203 | UNVERIFIED: original text unavailable; summarized invariants above |
| IHG canonical PRD v1.1 | 204 | UNVERIFIED: original text unavailable; summarized invariants above |
| IHG canonical PRD v1.1 | 205 | UNVERIFIED: original text unavailable; summarized invariants above |
| IHG canonical PRD v1.1 | 206 | UNVERIFIED: original text unavailable; summarized invariants above |
| IHG canonical PRD v1.1 | 207 | UNVERIFIED: original text unavailable; summarized invariants above |
| IHG canonical PRD v1.1 | 208 | UNVERIFIED: original text unavailable; summarized invariants above |
| IHG canonical PRD v1.1 | 209 | UNVERIFIED: original text unavailable; summarized invariants above |
| IHG canonical PRD v1.1 | 210 | UNVERIFIED: original text unavailable; summarized invariants above |
