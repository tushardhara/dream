# Research-console backend handoff and proposed UI backlog

Backend only. Owner UI entry is still a decision after accepting the engineering,
API/privacy/security evidence and limitations. No UI implementation, deployment,
owner approval, real-human validation or scientific completeness is implied here.
The proposed backlog is review material for that decision, not an approved UI start.

The authoritative wire contract is `api/proto/dream/v1/service.proto` and its
checked-in generated OpenAPI/gRPC/gateway artifacts. There are no invented routes
or alternative authentication schemes in this handoff.

| Operation | Existing RPC | Client responsibility |
| --- | --- | --- |
| Validate/load synthetic scenario | ValidateScenario, CreateWorld | Show structured validation and exact configured scope; do not infer execution authority |
| Bounded run controls | Control, GetOperation | Preserve operation ID and poll the same operation; distinguish paused/running/budget/completed and virtual vs wall time |
| Freeze/compare/reproduce | CaptureSnapshot, Fork, Replay | Keep exact handles/hashes; distinguish recorded replay from fresh stochastic experiments |
| Own perspective | ActorView | Use configured identity; never replace observer-specific uncertainty with global truth |
| Authorized research/audit/usage | ResearchView | Research role and scope required; preserve missing/unknown costs and contextual audit provenance |
| External agent interface | ExternalObserve, ExternalAct | Trusted credentials and permitted source context; no arbitrary hidden-state access |
| Export lifecycle | SubmitExport, DownloadExport | Preserve export ID/page token; revalidation may revoke a later page; delivered bytes cannot be recalled |

The standalone API executable remains a management host unless an execution
handler is explicitly composed by a trusted backend host. The new demo CLI is an
operator/reproducibility entry point, not an anonymously callable HTTP service.
The fixed demo does not add a network-specific projection route or general fork
policy. Existing authenticated small-fixture cognition/replay/fork/API checks remain
separate from the sparse 24-person reference demo. A future execution-host product
must select the appropriate handler and cannot infer it from an untrusted role header.

`hws-demo` supplies bounded run/replay/own-export artifacts. `hws-eval` supplies
independent evaluator reports and explicit study journal actions. Do not mix
these data into generation: the import/process boundary remains authoritative.
The evaluator and study CLI need their own trusted operator configuration; no
study-start or promotion button has been added to the public API.

Proposed UI work, contingent on owner entry approval:

1. Read-only scenario validator and operational status view with exact scope,
   operation IDs and clear simulated-time/real-time labels.
2. Actor-perspective inspection showing evidence, uncertainty, learned/valid time,
   missingness and revocation outcomes; no omniscient merged relationship score.
3. Authorized snapshot/replay/export tools with hash provenance, partial-page
   errors and an explicit fresh-experiment distinction.
4. Research reports with three separate panels: engineering acceptance, synthetic
   behavioral evidence, and human validity. NOT TESTED must stay visible.
5. Study plan/checkpoint/daily-quota inspection. Starting live infrastructure,
   changing study criteria, approving candidates and UI entry remain owner actions.

No automatic model-weight update, engagement/friendliness objective, hidden
privacy guarantee, or one-month delivery promise belongs in this backlog. Complete
source HWS registries/falsifiers and real-human transfer remain unresolved research
gates, not silently green prerequisites or an excuse to invent evidence.
