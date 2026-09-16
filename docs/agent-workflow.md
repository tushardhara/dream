# Agent workflow: single-branch main, ticket PRs, owner merge

Authority: the live epic on GitHub and the owner's comments on it. Read both before
touching code. `main` is the only long-lived branch; the two earlier integration
branches are merged and deleted (see History) and must not be recreated.

## Branching and PRs

1. Branch from current `origin/main`: `<role>/<ticket-number>-<short-name>`.
   A live epic may name a temporary integration branch instead (epic #76 uses
   `post-merge-integration`, created by the integrator from `main` at
   `0d1e59cf9042b80d4be491ecea0635be27ffe3a9`); branch from and target that branch
   when it says so, and never create it yourself.
2. Keep scope to the ticket. One implementation PR per ticket, one unmerged
   implementation PR at a time. No stacked PRs, no second writer on one ticket.
3. Fill the PR template: map every ticket criterion to code, test and command;
   record every "How to test" step with its observed result; state exact base and
   head SHAs; label required checks you could not run BLOCKED, never PASS.
4. Before handoff, refresh the target branch into the ticket branch with an
   ordinary merge (no rebase, no force-push) and confirm the target head is an
   ancestor of your head.
5. Post the handoff marker the epic names (currently
   `READY_FOR_CODEX issue=#N base=<sha> head=<sha>`) on the PR, followed by the
   criterion-to-code mapping, commands and results, ablation evidence and
   limitations. Then wait with bounded polling; do not self-approve or merge.

## Review and integration gate

The independent reviewer reads the actual code and tests at the exact head,
reruns the applicable checks in isolation, and posts
`<ROLE>_REVIEW base=<sha> head=<sha> verdict=PASS|CHANGES_REQUESTED|BLOCKED`
with evidence. Findings are fixed on the same PR; the implementer reruns the
affected checks and ablations and posts a new handoff with the new head. Where an
epic delegates integration to the reviewer, the reviewer squash-integrates a
passing PR into the epic's integration branch using an expected-head-SHA merge
and records `INTEGRATED issue=#N pr=#P base=<sha> reviewed_head=<sha>
integrated_commit=<sha>`; the next ticket starts only after that record exists and
post-merge CI is green. The owner alone merges `main`, and alone decides product
and design questions: a ticket that needs one gets a single `BLOCKED_ON_OWNER`
comment with options and a recommendation, and work moves to the next ticket.

## Test-coverage tickets

A coverage ticket is done only when the named ablation (guard replaced by
`if false` or the stated compiling equivalent) turns exactly the new test red,
restoring the source turns it green, and the restored tree is byte-identical to
the committed one. Record the ablation output in the PR body. Never weaken, skip,
quarantine or loosen a test or gate. Preserve frozen replay pins and legacy
codecs; regenerate a committed evaluation report only when the ticket says the
change is intended, in the same commit, with the diff explained.

## Waiting, blockers and interruption

Use bounded polling of GitHub while the session is active. No new external
automation infrastructure and no indefinite background-execution claim. Persist
a checkpoint on the ticket before interruption or runtime expiry:

```
CHECKPOINT role=<role> issue=#N branch=<branch> pr=#P
base=<full-sha> head=<full-sha>
last_processed_review=<URL or none>
remaining_findings=<list or none>
tests=<commands and results>
next_action=<specific action>
blocker=<exact missing requirement or none>
```

On resume, read live GitHub first (epic, ticket, open PRs, reviews, checks) and
inspect local branches and dirty files before editing. Reuse the existing ticket
branch and PR; never assume an earlier session is still running and never create
a competing writer or duplicate PR. Code failures require investigation, fix and
re-review, never weaker tests or invented passes. Review or CI pending means wait
or checkpoint; it does not authorize bypassing the gate.

## Standing limits

Never push `main`, release, deploy, start paid or live studies, publish private
originals or real-person data, change permissions or branch protections, reset
shared history, or materially change architecture, privacy, evaluation or
promotion policy under ordinary ticket authorization. This workflow does not
establish server-side branch protection and does not claim to.

## Shared VPC disk recovery safeguards

Follow [test-resource-safety.md](test-resource-safety.md) before local heavy work.
Use the foreground role guard, account retained work and caches, and checkpoint
before stopping. Never launch a new supervisor or competing worker.

## History

- **Revision 3 (epic #1, tickets #2–#17; alignment epic #39):** Codex implemented
  one ticket at a time on branches from `backend-integration`; Claude reviewed each
  PR at exact SHAs and squash-integrated passing tickets; the owner merged the
  aggregate PR #38 into `main` at `7b42a380ac0c2ff6dc70182322b44433d6c0f06f`.
- **Epic #49 (tickets #50–#58):** the same roles on `human-context-integration`;
  the owner merged the aggregate PR #70 into `main` at
  `0d1e59cf9042b80d4be491ecea0635be27ffe3a9`. Both integration branches were then
  deleted.
- **Epic #76 (post-merge hardening):** Claude implements, Codex reviews and
  integrates into `post-merge-integration`; the owner merges `main`.

Per-ticket review and integration records live as comments on the tickets and
PRs; they are the durable evidence, not this page.
