# Shared VPC test resource safety

Owner recovery instruction applies to **both Codex and Claude**. No new supervisor
is authorized. Reuse the existing role worktree; inspect GitHub, local dirty and
untracked files and workers first. A sandbox process listing cannot establish
that the host has no other workers. Do not start another agent to resolve that.

Before every local build, test, clone or mutation run, use:

```
python3 scripts/disk-guard.py --role codex --account /path/to/older-owned-cache --account /path/to/older-owned-checkout -- make verify
```

Claude uses `--role claude` in its separate review worktree. Account **all** retained
role temporary/build material (including old checkouts and caches) with repeated
`--account`; shared caches are conservatively charged in full. The current tree,
role state/cache/scratch directory, Go module/tool cache and role-labelled Docker
images are accounted automatically. Never adopt/delete another role's files.
The guard refuses a second guarded command for the same role using flock.
This does not discover or lock an already-running unguarded host worker.

The foreground guard checks each accounted filesystem before launching and every
60 seconds: at least 30 GiB available, less than 90% blocks used, less than 90%
inodes used (percentages rounded up, matching df), and less than 10 GiB accounted role footprint. On violation it sends
SIGTERM only to its own command process group, allows 20 seconds for cleanup,
then kills that group if needed. It stops allocating new test scratch at the
threshold. Leave checkpoints/evidence in `~/.local/state/dream-recovery`, outside
scratch. Cache/scratch/logs are separate under `~/.local/state/dream-tests/ROLE`.
Logs rotate with three retained generations, each bounded to 2 MiB; preserve
needed full evidence separately before another run, within the footprint limit.

Disposable containers and images carry a random exact `dream.test.run` label and
role label. The wrapper removes only that run's containers/images in its finally
path; the individual scripts also retain their name-specific cleanup. PostgreSQL
uses its own fresh tmpfs cluster. Go temporary work lives under the owned scratch
and is removed on normal completion/failure/interruption. Mutation scripts handle
SIGTERM through their source-restoration finally paths. No global Docker prune,
volume removal, broad /tmp deletion, worktree deletion or shared cache cleanup.
Docker builds use the legacy builder for local guarded runs and `--force-rm` to
avoid accumulating BuildKit cache or failed intermediate containers.

Limits: this is a **cooperative sampled monitor**, not an OS-enforced hard quota.
Allocation may cross a threshold between samples; other processes can fill the
shared disk. Docker daemon transient/intermediate storage is not fully attributable
from role image labels. Host worker inventory may be unavailable in a sandbox.
SIGKILL, host crash or daemon loss can prevent cleanup: inspect the persisted run
ID and exact labelled resources on recovery; never blindly start another worker
or clean unrelated resources. No persistent monitor exists after the command
ends. If strict kernel quotas or complete daemon accounting are required, report
that unavailable safeguard rather than asserting this wrapper provides it.

CI still runs the full `make verify`; local agents must wrap it. The guard changes
resource handling, never test assertions, timeouts, fixture limits or acceptance.

Preflight failures write a blocked `checkpoint.json` before any child launches.
A competing caller writes bounded latest `denied.json` instead, preserving the
active owner's checkpoint. If the Docker CLI exists but daemon inventory fails,
the guard reports unavailable role accounting and denies; it never silently skips
that inventory. A sandbox unable to meet the absolute 30 GiB floor remains blocked;
there is no relative floor or role override. Independent required Docker/PG checks
remain unrun where the daemon is unavailable, regardless of CI status elsewhere.

For a busy shared host, local agents may set `GOMAXPROCS=2 GOFLAGS=-p=2` to bound Go
build/runtime parallelism. This does not change tests, assertions, resource ceilings
or command timeouts. A footprint scan still fails closed if it exceeds 30 seconds.
