Related ticket: #N
Target: main (or the integration branch a live epic names, e.g. `post-merge-integration` for epic #76)
Base SHA: <full SHA>
Head SHA: <full SHA>

## Behavior and acceptance evidence

Map each ticket criterion to code/test/command and observed results, one line per
criterion. Include adversarial and negative cases. For a coverage ticket, record the
ablation: the named guard replaced in compiling form turns exactly the new test red,
restoring it turns the test green, and the restored tree is byte-identical.
Confirm the current target base is an ancestor of this head.

## Checks and limitations

List exact commands and results. Label required checks you could not run BLOCKED
(never PASS) and optional studies NOT RUN. State synthetic-only scope, resource caps,
frozen replay pins kept, and remaining risks. Never weaken a gate to get green.

## Handoff

Reviewer handoff marker, if the live epic uses one (e.g. `READY_FOR_CODEX issue=#N base=<sha> head=<sha>`).
Last processed review: <URL or none>. Remaining findings: <list or none>.
The author never approves or merges their own PR; only the owner merges `main`.
