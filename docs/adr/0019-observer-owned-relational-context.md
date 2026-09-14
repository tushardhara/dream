# ADR-0019: observer-owned relationship context in the common action engine

Status: implementation for #42, independent review pending.
Source: #42 revision 2 supplies HWS §7 fields and the founder's fictional
five-person reference fixture. No full private-document or human-validity audit.

## Representation and permissions

`core.RelationshipContext` is reusable without simulator/world imports. It binds
one observer to one other person, multiple relationship types, valid time, and
bounded attributed measures. Missing measures are unknown. Fourteen detail kinds
reference separately stored claims/memories: origin, history, views of the
relationship/other, communication, positive/friction patterns, significant
memories, open loops, commitments, recent events, current state, trajectory and
hypotheses. Referenced records retain observer, subject, proposition, evidence,
confidence, source and temporal metadata. References grant no access and never
convert another person's actual private state into this observer's belief.

The graph's `relation.v2:` envelope embeds this typed account. Its observer,
endpoints, types and validity must match the outer envelope. Every context source
must occur in permissioned lineage; SafeContext performs the same checks. Existing
MemoryService/PolicyService transactions own authorization, corrections, and
revocation propagation. Correction preserves earlier known-as-of claims and
snapshots. Revocation denies subsequent reads and invalidates affected derivatives.
An unrelated runtime that never consumed the source need not be destroyed.
Already delivered offline exports and external backups cannot be recalled.

Bounds remain explicit: 8 types, 14 detail categories with at most 8 source IDs
apiece, 13 measures; graph text 2048 bytes and outer payload 4096 bytes. These are
independent limits, so a maximally populated account may exceed the encoded budget
and fail closed. Rich prose/history stays in referenced records. No truncation,
eviction or invented neutral values makes an oversized input fit.

## Behavior and consumers

`behavior.ApplyRelationship` translates currently permitted observer reports into
#41 disclosure context and relationship memory; a host-selected focused interaction
also supplies #40 history, relationship-support and expected-safety appraisal cues.
Trust, closeness, prior outcome and friction affect subjective consequences;
expectation, sensitivity, fear, pride, shame, expected reaction, protective intent,
norms and stress retain the #41 disclosure mechanisms. These coefficients are
synthetic engineering hypotheses, not psychological facts. Type labels have no
numeric defaults and no gender or family stereotype coefficients.

CognitiveService obtains typed profiles from its approved graph context; the
planner cannot supply them as authority. CognitiveHandler and the demo both call
the same relationship adapter and ChooseAction/22-drive appraisal implementation.
Unretrieved/revoked sources, foreign perspectives, expired context and ambiguous
same-recipient context fail closed. Unknown relationships leave unrelated permitted
actions, including WAIT, available. Disclosure permission is a separate gate.
The bootstrap scenario forbids sharing another person's initial private records;
the demo does not override this rule to make disclosure counts look realistic.

## Versioned five- and 24-person demo

RunDemo/CLI now create `backend-demo.v2` through RelationalScenario. The five adults
are H/W/S/A/B: H-W spouses, H-S siblings, H-A/H-B friends, W-A acquaintances, W-S
sibling-in-law plus acquaintance/history. S being H's sibling is a declared fixture
assumption. All six edges have separate directional reports; W-B knowledge is
absent. Reports are assigned by edge/direction, not sex or role. Contradictory
perspectives are retained.

The 24-person fixture has 104 directional edges across couples, sibling links and friendships, and eight overlapping
groups. Only perceived group events and previous addressed deliveries enter an
actor's inputs. Role/history ablations exercise actual candidate distributions.
No global relationship score, engagement objective or relationship-preservation
mandate is introduced.

The new scenario extension requires the `relationships.v2` engine capability;
older engines cannot silently ignore it. Frozen Scenario/v1 factories, action v1
and recorded v1 demo artifacts remain supported without changing fixture bytes.
The new checkpoint is a canonical version/period/world-hash projection over frozen
genesis and named seeded draws. It preserves every drive receipt, action decision
and outcome in derived history, with at most 24 people × 24 periods (576 decisions).
The checkpoint remains below 256 bytes in tested fixtures, within the unchanged
4096-byte runtime cap. Recovery reconstructs and compares the entire world hash
and recorded draws. The model-backed cognitive.v2 ledger remains bounded to 2–4
actors; the five-/24-person demo uses the versioned derived projection, not a larger
or silently reset cognitive ledger. No general unbounded-world scale is claimed.

Observed responses carry an explicit reply-to decision ID; the sender must have
observed the earlier delivery before choosing that reply. Occurrence and learning
times stay separate. Responses resolve only the linked observer-owned action;
co-presence, silence and another actor's hidden state never become positive learning.
Unobserved outcomes remain unknown. No fake completion of learning/study is implied.

## Evidence mapping

| Requirement | Checks |
| --- | --- |
| Supplied source fields, uncertainty and bounds | TestRelationshipSourceFieldsAndUnknown |
| Exact five-person topology, contradictions, W-B unknown | TestFivePersonDirectionalTopology |
| Same actor/event/seed distributions; role/history/state ablations; reversal | TestRelationalControlledSpecificityAndAblations |
| Foreign, revoked, future/expired and ambiguous inputs | TestRelationshipAccessAndTimeFailClosed |
| Graph version, source/observer/subject/time and safe-context envelope | TestRelationshipV2CodecPermissionEnvelope |
| Actual cognitive handler distribution and appraisal sensitivity | TestRelationalContextReachesCognitiveHandler |
| Five/24 common-engine checkpoints, draws, replay and recovery | TestRelationalFiveAnd24CommonEngineRecovery |
| 24-person role ablations, group visibility, seed alternatives | TestRelationalDemoRolesGroupsPrivacyAndSeeds |
| Older engine rejection | TestRelationalScenarioRequiresVersionedCapability |
| Observer-only host export and version binding | TestRelationalDemoActorExportAndVersionBinding |
| Durable correction/as-of, snapshot/replay/export revocation | TestRelationalCorrectionSnapshotReplayRevocation (real PostgreSQL) |
| Existing formats | Frozen legacy action/cognitive/demo fixture tests; originals unchanged |

Full verification, mutation controls, exact-SHA CI and independent review evidence
are recorded on the ticket PR. A test name in this table is not a claim that every
required command has already passed. Scientific validity, human transfer and
cross-model transfer remain NOT_TESTED; the real 30-day study remains NOT_RUN.
