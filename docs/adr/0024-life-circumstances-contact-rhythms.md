# ADR 0024: Life circumstances, observed contact and current applicability

Status: design for #48, based on verified #52 integration `2f248610`.
Backend and offline synthetic scope only. No human validity is claimed.

## Re-audit and separation of concerns

The merged scenario contract requires numeric adult age. Drive confidence decays
from explicit anchors; graph retrieval ranks permitted, valid records by salience
and recency. Neither implements contact expectations or evidence that a historical
fact still describes current circumstances. Relationship v2/domain consumers and
assistance v3 already preserve observer perspectives, explicit context and scoped
willingness. They will be reused rather than re-audited as an aggregate release.

The new reusable temporal contracts belong in core/application packages. They keep
historical fact validity, retrieval priority and current applicability separate.
Graph-backed corrections/revocation retain existing observer/time/permission rules.
Synthetic scenes, matched comparisons and labels belong in examples/evaluation.

## Planned contracts and policy

- Versioned, bounded circumstance records distinguish voluntarily authored
  responsibilities, transitions, preferences and relevant experiences from another
  observer's hypothesis. Evidence, confidence, valid/learned times and freshness of
  current applicability are explicit. Numeric age never assigns maturity, roles,
  obligations or trust; an explicit unknown-age scenario form preserves old bytes.
- Contact preferences are observer/relationship/channel specific. An explicit
  preference differs from an estimate using permitted observed contacts. A bounded
  estimator requires sufficient intervals and represents irregularity/uncertainty.
  Missing observations, especially other channels, do not establish no contact.
- A gap is measured against the supported expectation. Busy circumstances, agreed
  breaks, important changes and stale assumptions can justify WAIT or a bounded
  invitation to clarify. No gap diagnoses rejection or computes relationship health.
  Unsupported circumstances remain unknown; no hidden life events are invented.
- Opt-in helper and human consumers use these derived inputs in their actual
  decision paths. Existing permission and interpersonal boundary gates always win;
  clarification retains a cooldown/burden budget. Output templates do not disclose
  private circumstance records or another observer's update.
- New formats and explicit scenario migration retain legacy replay. Current replay
  rechecks access while authorized historical queries preserve what was known then.
  Graph source hashes and semantic request versions bind recorded helper decisions.

## Requirement-to-evidence plan

| Requirement | Existing foundation / new evidence |
| --- | --- |
| Optional age without maturity inference; supported life changes affect decisions | scenario version/capability checks; actual paired age-label and circumstance consumer tests |
| Different daily/occasional expectations for 2/60-day gaps | bounded explicit/estimated contact policy; matched gap tests |
| Breaks, busy responsibility, sparse/unobserved channel | scoped boundaries plus conservative temporal uncertainty; negative cases |
| Observer disagreement and private updates | separate graph scopes and approved helper context; private-update/action/replay tests |
| Corrected/revoked events and stale applicability | existing graph current/history selection plus new temporal projection; historical and current consumer assertions |
| Long-gap WAIT and bounded clarification | existing #51 helper budget plus explicit human temporal budget; no-contact and repeated-question tests |
| Matched multiseed context-aware/context-off/simple comparisons | actual helper and human paths; per-scene appropriateness, unnecessary/missed clarification, uncertainty and violation counts |
| Load-bearing inputs, not only schema | separate life-event, rhythm and staleness consumer mutations, including fresh/replay and state read/write where applicable |

Thresholds and scenario labels are engineering hypotheses, not validated social
estimators. The final implementation will document exact bounds, formats, results
and limitations here. No universal gap-to-decay rule, new supervisor, live provider,
outreach, UI, deployment or real-person source document is introduced.
