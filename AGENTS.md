# Dream agent instructions

Read live GitHub epic #1 and the current ticket, then docs/agent-workflow.md,
docs/requirements.md and docs/adr/0001-backend-boundaries.md before changing code.
The owner's revision-3 authorization governs the sequential backend program.

- One module: github.com/tushardhara/dream. Backend only. Keep core importable without worlds.
- Codex implements/tests on one ticket branch from current backend-integration;
  opens/updates one implementation PR into backend-integration; never approves or merges.
- Claude independently reviews exact base/head and may squash-integrate ONLY into
  backend-integration after every epic gate. No comment-driven merge bot.
- Never push/merge main. The owner merges the final aggregate PR. No paid calls,
  deployments, real/private data publication, destructive shared-history edits,
  permissions changes or material scope changes without explicit owner authority.
- Dependencies require verified INTEGRATED evidence and current Git ancestry,
  not closed issues or main. Preserve one unmerged implementation PR at a time.
- Add meaningful negative tests. Run make verify. Never weaken a gate to get green.
- Default tests use synthetic fixtures, no credentials or paid services. Keep
  provider generation outside transactions; preserve observer/evidence/time/uncertainty.
- Post SHA-bound READY_FOR_CLAUDE/checkpoints to GitHub. Fix findings on the same PR.
  Resume existing work, using GitHub as the durable mailbox; do not claim background
  execution after the session ends. See workflow for integration and restart details.
