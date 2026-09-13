<div align="center">

# Dream

### A laboratory for the lives between the lines.

Persistent people. Private worlds. Relationships that change.

[Vision](#the-vision) · [The simulator](#the-first-world) · [Architecture](#built-to-be-reused) · [Roadmap](#the-road-ahead) · [Build plan](https://github.com/tushardhara/dream/issues/1)

</div>

---

## Why Dream?

The name is inspired by **Dream of the Endless**, also known as Morpheus, from DC's *The Sandman*: a character associated with dreams and stories.

For this project, the connection is the space between a person's inner world and the world other people see: hopes, memories, fears, interpretations, and all the things left unsaid.

Dream asks what happens when those worlds meet—and how they change over time.

*An independent research/software project; not affiliated with or endorsed by DC.*

## The vision

Most software remembers that two people are connected. It remembers much less about **what exists between them**.

A relationship carries history, expectations, unfinished conversations, shared joy, conflicting interpretations, and promises that may or may not have been kept. Its participants do not necessarily experience the same relationship in the same way.

Dream is being built toward the **Intelligent Human Graph**: a permissioned, longitudinal understanding of people and their relationships, with a future goal of helping real human life—not maximizing time spent talking to an AI.

The first step is a controlled place to test the underlying ideas.

## The first world

The **Human World Simulator** is Dream's first application: a computational laboratory for changing human worlds.

Its basic unit is not a persona prompt. It is a persistent simulated person with history, current circumstances, competing motivations, imperfect memories, and incomplete knowledge of others.

Consider a fictional household:

> One partner receives an opportunity to travel. The household cannot comfortably afford for both to go. The other partner says, “It's okay.”

That sentence alone does not settle the story.

They might genuinely feel happy for their partner. They might also feel disappointed, protective, excluded—or several things at once. What happens next depends on their history, finances, fatigue, expectations, what each notices, and what each chooses to reveal.

A useful simulator must preserve these possibilities without declaring any one hidden motive inevitable.

### What we want to study

- How life events change state, beliefs, intentions, and behavior.
- Why the same person behaves differently with a spouse, friend, or manager.
- How private feelings and public behavior diverge.
- How memories, misunderstandings, and unresolved commitments accumulate.
- How overlapping groups develop norms, routines, tensions, and shared histories.
- How different choices—including silence—lead to different plausible futures.

The aim is measurable, testable behavior, not merely convincing dialogue.

## Built to be reused

**The simulator is the first host, not the boundary of the project.**

| Layer | Responsibility |
|---|---|
| Human World Core | Reusable people, evidence, perspective claims, relationships, groups, memory, rights, temporal state, and outcome contracts |
| Reusable graph application services | Ingestion, queries, corrections, revocation, and permitted exports |
| Human World Simulator | Synthetic latent state, drives, scenarios, virtual time, randomness, and branching |
| Infrastructure adapters | PostgreSQL, model providers, authentication, and API transport |
| Independent evaluation | Holdouts, behavioral comparisons, calibration, and falsifier reports |

```mermaid
flowchart TD
  Future["Future mobile or other application"] --> Graph["Reusable graph services"]
  Research["Research API / CLI"] --> HWS["Simulation application"]
  HWS --> Graph
  HWS --> Sim["Synthetic world engine"]
  Graph --> Core["Human World Core"]
  Sim --> Core
  Eval["Independent evaluator"] --> HWS
```

Interfaces are declared by their consumers; adapters implement them, and `cmd` composes the concrete implementations.

A future application must be able to use the core without creating a simulation world, importing a branch type, or gaining access to simulator-only hidden state.

The planned reuse test is concrete: a second, non-simulator client must ingest statements, preserve differing perspectives, correct and revoke evidence, and export permitted state.

### Planned technical foundation

- **Go**, in a single-module modular monolith.
- **PostgreSQL**, with versioned events, temporal projections, idempotent writes, and recovery checkpoints.
- **gRPC and an HTTP/JSON gateway**, generated from versioned protobuf contracts.
- **Provider-independent model adapters**, with deterministic fakes and recorded-response replay.
- **Virtual-time simulation**, with explicit random streams and stable event ordering.
- **Independent evaluation**, separated from generation and its context.

No model vendor defines the core. No premature microservice split is required.

## Principles that survive every version

**A perspective is not a fact.** “A believes something about B” must retain its observer, evidence, uncertainty, and time.

**People are not fixed profiles.** Stable tendencies influence behavior; events and experience can change it.

**Knowing is not permission to reveal.** Read, derive, disclose, and attribute rights are separate. Derived information must not bypass source restrictions.

**Hidden state stays hidden.** An actor sees permitted observations and their own allowed state—not another actor's private mind, future events, or evaluation labels.

**WAIT is a real action.** Silence, delay, and inaction must be represented and measured.

**Relationships are not one score.** Conflicting perspectives and multiple dimensions cannot be replaced by a universal relationship-health number.

**History matters, and so does correction.** Events have occurrence times; actors learn about them at different times. Revocation must reach derived state, caches, and exports.

**Reproducibility must be honest.** Compatible deterministic or recorded runs can be replayed exactly. A fresh model call is a new stochastic experiment, even with the same seed.

**Better prediction must not become better manipulation.** The long-term objective excludes covert persuasion, forced agreement, dependency, and engagement optimization.

## The road ahead

| Stage | Engineering objective |
|---|---|
| Foundations | Domain boundaries, events, scenario validation, virtual time, and recovery |
| Changing people | State, appraisal, drives, memory, and perspectives |
| Changing relationships | Edge-specific context, groups, information boundaries, and typed behavior |
| Research backend | Replay, counterfactual branches, authenticated APIs, exports, and operational controls |
| Evaluation and demonstration | Multi-seed comparisons, calibration readiness, and a 24-person synthetic world |
| Later | Research console, longer studies, and applications built on the same core |

The demonstration target is **24 simulated people, 8 overlapping groups, and 30+ relationship edges**, with a configurable 12-month simulated horizon. Smaller worlds come first.

A **30-day real-time study** is a separate operational milestone. It is not equivalent to advancing the simulated clock by a month.

See the [master epic and dependency-ordered tickets](https://github.com/tushardhara/dream/issues/1) for executable scope, acceptance criteria, and current progress. Issue numbers are identifiers, not execution order.

## Research, not a claim of mind-reading

Synthetic consistency is not real-human validity.

Dream's research plan includes baseline comparisons, ablations, multiple seeds, frozen holdouts, uncertainty reporting, and tests designed to expose failures—not just highlight appealing examples.

Claims about real humans require appropriate consent, independent data, and external validation. When evidence is unavailable, the result is **not tested**, not “passed.”

Production relationship advice, real-user interventions, matching, and autonomous model-weight training are outside the current backend build.

## Development status

**Early development.** This README describes the target design, not a completed feature set.

Bootstrap #2 is integrated into `backend-integration`. The project does not yet claim a runnable research release or an end-to-end simulator quickstart.

### Getting started and verification

Install Git, Make, Python 3, Go 1.27.1, a C compiler and Docker, then run `make verify` from the repository root.

`hws scenario validate examples/scenarios/quiet-overlap.yaml` validates the synthetic scenario offline. The authenticated gRPC/HTTP API is available through the transport host; `hws-api --config <file>` runs an explicit management-only profile with trusted credentials/view grants and a non-owner PostgreSQL connection. It never chooses a fake simulation policy: step/run-until require an embedding execution host with a configured handler. Development listeners are loopback-only; deployment requires TLS. See [transport composition and limits](docs/adr/0013-authenticated-transport.md). `hws-worker` provides bounded model/outbox maintenance; `hws-admin` supplies explicit migration and quarantined restore commands. See the [single-host operations runbook](docs/operations.md). Verification covers static/race checks, pinned protobuf/gateway/OpenAPI regeneration, real PostgreSQL role and revocation tests, and purge-aware backup/restore. No live provider or paid study is run by these checks.

See [Contributing](CONTRIBUTING.md), [architecture](docs/adr/0001-backend-boundaries.md), [requirements and gaps](docs/requirements.md), [pinned tools](docs/toolchain.md), and [agent workflow](docs/agent-workflow.md).

Current development follows the [revision-3 workflow](https://github.com/tushardhara/dream/issues/1):

- Ticket branches and PRs target `backend-integration`.
- Codex implements; Claude independently reviews and may integrate passing ticket PRs under the epic's gates.
- Tests and review evidence are tied to exact revisions.
- The owner approves the final merge into `main`.
- Spending, deployment, real-person data, and material scope changes require separate authorization.

Private source documents, personal data, credentials, and unapproved provider transcripts must not be published in this repository.

---

<div align="center">

**Understand the person. Preserve the perspective. Study the possibilities.**

</div>
