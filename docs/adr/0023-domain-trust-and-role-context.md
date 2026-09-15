# ADR 0023: Domain trust and overlapping role contexts

Status: implemented for #52, pending independent review. Backend, synthetic and
offline only; based on verified #51 integration `00996ca9`.

## Decision and consumer behavior

Relationship context v2 is one observer's attributed account for one other person,
one supported domain and one role context. The fixed registry contains childcare,
finances, confidentiality, practical coordination and emotional support. An account
has its own record ID, validity interval, numerical measure confidence and evidence
references. `RoleContext` is an explicit frame key (for example family/business);
`ContextSource` is the observer-specific evidence supporting that frame. Different
observers can discuss the same frame without sharing private source IDs. Type labels
(spouse, friend, sibling, business partner) supply no coefficients or access rights.
Trust, expectations and prior outcomes are scoped by the containing account.

`SelectRelationship` selects only the requested observer, pair, domain, frame and
time. Concurrent conflicting accounts remain present. A unique permitted account
can establish the frame; an explicit frame or account can disambiguate. An explicit
account selects a view without overwriting other accounts. Missing context remains
unknown; no legacy unscoped vector or other domain is used as fallback. Selection
accepts at most 16 accounts. Unsupported domains fail validation.

`relation.v3` binds the account ID to its outer graph record. Frame and measure
sources remain permission-checked lineage, and helpers must explicitly retrieve
their metadata. Corrections may change values with new record IDs but cannot change
the domain or frame of an existing relation; historical views and revocation use
the existing graph machinery. Comparisons cannot subtract different domain/frame
vectors. Each observer's graph scope remains separate.

`assistance.v3` opts into a focus alongside #51's scope. The topic must match the
domain; summary sharing requires confidentiality. After the interpersonal gate
and graph approval, the host selects one currently permitted account per required
observer. Multi-perspective planning preserves each account separately: choosing
the user's account cannot resolve a different observer's conflicting accounts.
Missing/ambiguous context returns exact `relationship_context` WAIT before planner
invocation. Frame, account and measure confidence remain distinct and multiply in
the reference planner. Its coordination proposal requires each perspective to have
known positive trust and supporting expectations; it never averages opposition
into consensus. These coefficients are engineering controls, not a social model.
Other explicit goals retain the bounded fixed templates. All actions remain subject
to independent scoped willingness and data rights before planning and at commit.

The no-assistant baseline stays disabled. The simple explicit-preference baseline
has no graph inference; it needs an explicit frame (not an account ID) to follow
the user's goal. It assigns no numerical trust. Single and multi arms use the
actual approved graph records. An externally supplied planner has no authority to
override unknown context, request identity, consent, data policy or templates;
its semantic treatment of known measures is not independently inferred by the host.

`domain-human-actions.v1` wraps the existing scoped human consumer, which still
calls the common appraisal, action choice and observed-outcome learning functions.
Selected account/frame/source metadata must have current self read/derive rights.
Native unscoped relationship memory/beliefs are forbidden in this opt-in wrapper.
Actual supportive/adverse responses update only the domain/frame bucket recorded
by the original decision receipt. The caller cannot relabel the outcome's domain.
There is **no automatic transfer across domains or role frames**. Later consumption
and learning require current permission for retained outcome, account and source
metadata. Unknown/ambiguous context allows WAIT or unilateral leaving/withdrawal.
Non-confidentiality trust cannot enable disclosure, even with a data grant.

A human actor retains at most eight domain/frame buckets and sixteen outcome
receipts; codecs also enforce 128 KiB. Capacity exhaustion returns an error rather
than silently deleting learning or broadening its meaning. Source/evidence bounds
and existing helper context byte/count limits remain in force. No worker, scheduler,
provider or transport is added.

## Compatibility and replay

New formats are relationship context v2, `relation.v3`, `assistance.v3`,
`domain-human-actions.v1`, `relationship-focus.v1` and `domain-experiment.v1`.
Scenario execution infers the `relationships.v3` capability from content, even if
`Requires` is removed. `WithDomainContexts` migrates only explicitly authored v2
accounts with already-known evidence; `BindRelationshipDomain` requires explicit
replacement measures and never copies a legacy trust vector implicitly. Scenario
accounts remain capped at eight per actor. Existing actor/scenario/relation/helper
formats and frozen #50 bytes remain unchanged. The old relationship adapter rejects
v2 context explicitly; callers must opt into the domain consumer rather than drop
new fields. Legacy policies are not claimed to enforce new opt-in restrictions.

Helper request hashes include focus; evidence hashes pin the raw approved records,
including still-readable content and confidence changes. Both external and stored
replay recheck current policy, boundary revision and context selection. Planner
history is filtered by exact focus/scope and scrubbed of source/hash/focus bindings.
Human decisions pin accounts and metadata; the combined example compares the full
trace on replay. Mechanical replay does not establish psychological validity.

`examples/domainexperiment.Run` retrieves Alice's own current graph records for
the human consumer and separately runs the actual multi-perspective helper host.
It holds identities and synthetic words constant while authored frame evidence
varies. No Bob-only account enters Alice's input. The helper is not asserted to
cause the human decision; this experiment tests shared context consumption.

## Acceptance evidence

| Criterion | Implementation and behavioral tests |
| --- | --- |
| Bounded, attributed domain/frame accounts with uncertainty | core selection/migration tests; graph `TestDomainRelationCodecBindsAccountFrameAndVersion`; helper `TestDomainHelperUnknownFrameConfidenceAndMissingMetadata` |
| Childcare outcomes affect actual choices, not financial trust or secrets | `ResolveDomainAction`, `ChooseDomainAction`; `TestLearnedChildcareTrustDoesNotTransferToFinancesOrSecrets`, `TestLearnedDomainMemoryRequiresCurrentEvidenceAndSameFrame` |
| Same words/people, different frames or acknowledged ambiguity | actual helper/human tests `TestDomainHelperActuallyUsesFrameDomainAndSeparateAccounts`, `TestDomainAndRoleContextsDriveActualHumanChoices`, combined `TestActualDomainConsumersAndReplay` |
| Labels supply no trust or rights | label permutations in both consumer tests; `TestDomainHelperConsentAndUnknownContextCannotBeForged`; existing scoped willingness controls |
| Conflicts/observers remain distinct through corrections, revocation, retrieval, planning and replay | graph domain retrieval/correction/comparison tests; helper conflict, atomic revocation, metadata, history and still-permitted replay tests; human current-source and codec tests |
| Scenario/codec migrations and retained legacy behavior | `TestDomainScenarioMigrationCapabilityAndLegacyBytes`; explicit core migration tests; `TestFrozen50CodecsAndExperiment`; existing legacy relation/human/scenario tests |

Eight deliberate mutations removed domain selection, frame selection, learned-domain
isolation, learned-frame isolation, helper measure consumption, replay evidence
binding, frame uncertainty and per-observer explicit selection. Each was caught
by a named behavioral test. The explicit-selection mutation initially survived
because a positive multi-observer control was missing; that test was added and
the mutation then failed. Source was restored after each check. Exact-SHA
verification and command results are recorded in the PR/checkpoint.
No human benefit, stereotype validity, inferred consent, real-world transfer rule,
natural-language frame detection or validated relationship model is claimed.
