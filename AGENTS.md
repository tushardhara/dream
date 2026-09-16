# Dream agent instructions

Read the live epic and the current ticket on GitHub, then docs/agent-workflow.md,
docs/requirements.md and docs/adr/0001-backend-boundaries.md before changing code.
The owner's authorization on the live epic governs what a ticket may change.

- One module: github.com/tushardhara/dream. Backend only. Keep core importable without worlds.
- `main` is the only long-lived branch. Branch one ticket branch from current `main`
  (or from the integration branch a live epic names); open ONE PR per ticket targeting
  that branch. Keep one unmerged implementation PR at a time; never stack or duplicate.
- The implementer never approves or merges its own PR. An independent reviewer reads
  exact base/head SHAs and posts a verdict. Only the owner merges `main`. No
  comment-driven merge bot. Never push `main` directly.
- No paid provider calls, deployments, real/private-person data, destructive
  shared-history edits, permissions/branch-protection changes or material scope
  changes without explicit owner authority.
- Dependencies require verified integration evidence and current Git ancestry, not
  closed issues alone.
- Add meaningful negative tests. Run `make verify` (or every non-Docker gate, with the
  Docker gates stated BLOCKED, never PASS). Never weaken, skip or quarantine a test or
  gate to get green. Preserve frozen replay pins and legacy codecs.
- Default tests use synthetic fixtures, no credentials or paid services. Keep
  provider generation outside transactions; preserve observer/evidence/time/uncertainty.
- Post SHA-bound handoffs and checkpoints on GitHub; fix findings on the same PR.
  Resume existing work from GitHub, the durable mailbox; never claim background
  execution after a session ends. See docs/agent-workflow.md for the current
  program, integration rules and restart details.
