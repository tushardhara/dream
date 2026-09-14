# Revision-3 unattended integration and recovery

Authority: https://github.com/tushardhara/dream/issues/1, revision 3.
This replaces earlier owner-per-ticket merging and main-based dependencies.

Execution order: #2 → #3 → #4 → #5 → #6 → #7 → #8 → #9 → #12 → #10 → #11 →
#13 → #14 → #15 → #16 → #17. Written ticket dependencies are authoritative.

## Resume without chat history

1. Read live epic/ticket comments, all open PRs and their reviews/checks. Inspect
   local status/branches/worktrees and remote branches before editing. Preserve
   existing work; reuse the active ticket branch/PR. At most one unmerged
   implementation PR; do not create a competing writer or stacked PR.
2. Fetch origin/backend-integration. Match every dependency's INTEGRATED record
   to the actual integrated commit and verify its ancestry on this branch.
   Inspect changes and passing post-merge checks; closure/checklists alone do not
   satisfy dependencies. Red integration pauses the queue for a focused fix PR.
3. If no ticket is active, choose the earliest ready ticket. Branch from current
   origin/backend-integration. Keep scope to that ticket. No main-based branches.
4. Run applicable acceptance checks including make verify. Open/update one PR
   targeting exactly backend-integration; document commands, coverage, failures,
   limitations and full base/head SHAs. Refresh the target into the ticket branch
   before final review (ordinary merge, no force push); target must be an ancestor.
5. Post `READY_FOR_CLAUDE base=<sha> head=<sha>` with evidence. Read Claude's full
   review and inline findings; fix on the same PR, rerun checks, push and request
   exact-SHA re-review. Codex never approves or merges.

## Claude integration gate

Claude reads the actual implementation/tests and independently runs all applicable
checks in isolation, including setup/CI workflow review for #2. Required unrun
checks are BLOCKED. Post `CLAUDE_REVIEW base=<sha> head=<sha>
verdict=PASS|CHANGES_REQUESTED|BLOCKED` and acceptance evidence. All blocking
findings must be resolved, PR CI green, and base must be an ancestor of reviewed
head. Re-fetch both refs immediately before integration; if either changed,
refresh/test/review again. Respect native checks and required reviewers. Same-account
model-attributed comments establish model independence, not native independent
approval; if required native approval is unavailable, report BLOCKED.

Claude alone may squash-merge a passing ticket into backend-integration, using
an expected-head-SHA merge operation. Never enable blind auto-merge, bypass
protections or trigger a merge bot from comments. Record:

`INTEGRATED issue=#N pr=#P base=<sha> reviewed_head=<sha> integrated_commit=<sha>`

Verify the resulting commit, target branch and post-merge CI/make verify before
marking the epic checklist integrated (not released). Keep ticket issues open.
Codex refreshes integration and immediately resumes the next ready ticket only
after this evidence exists. A plain PASS is insufficient.

## Waiting, blockers and interruption

Use supported event/watch facilities or bounded polling while the session is
active. No new external automation infrastructure and no indefinite background
execution claim. Persist a checkpoint before interruption/runtime expiry:

```
CHECKPOINT role=Codex issue=#N branch=<branch> pr=#P
base=<full-sha> head=<full-sha>
last_processed_review=<URL or none>
remaining_findings=<list or none>
tests=<commands and results>
next_action=<specific action>
blocker=<exact missing requirement or none>
```

Missing authority/material choices get one concise BLOCKED comment describing
safe completed work and the owner's exact decision. Code failures require
investigation/fix/re-review, never weaker tests or invented passes. Review or CI
pending means wait/checkpoint; it does not authorize bypassing the gate.

## Final handoff

After #17 engineering gate passes, Codex opens/updates ONE backend-integration →
main PR containing all integrated ticket/PR/commit records, whole-program
verification, replay/reuse/security evidence, migrations, cost/operational limits
and unfinished scientific gates. Claude reviews the aggregate exact SHAs and
posts a recommendation; only the owner merges main. Never push main, release,
deploy, start paid/live studies, publish private originals/real data, change
permissions/protections, reset shared history or materially change architecture,
privacy/evaluation/promotion policy under ordinary ticket authorization.

At bootstrap both branches reported unprotected; this workflow does not establish
server-side protection. Do not claim it does.

## Shared VPC disk recovery safeguards

Both roles must follow [test-resource-safety.md](test-resource-safety.md) before
local heavy work. Use the foreground role guard, account retained work/cache,
and checkpoint before stopping. Never launch a new supervisor or competing worker.
