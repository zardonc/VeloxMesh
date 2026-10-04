# Continued investigation: preregistered boundaries

User steering: establish hardware adequacy first. Defer the full warmup/load
matrices, Gemini calls and product changes until this bounded hardware screen
has been assessed. Compare idle and scheduled 1/8 RPS direct/off/on/memo paths,
retain first requests, bracket cache paths with off baselines, observe host CPU/
memory/GPU plus guest vmstat/PSI/disk/cgroups, and run component-only probes.
This screen is diagnostic and cannot approve production capacity or a release SLO.

Preserve existing source edits, evidence, historical volumes and user model services.
Inventory VM processes/containers/ports first. Use uniquely labelled fresh dependency
containers with the same cached image IDs and inherited private environment. Refuse
active container or port collisions. Do not delete historical collections or volumes,
drop kernel caches, change swap/VM sizing, deploy, or change release gates.

Failures to retain: unknown active owner, source/image mismatch, occupied port, reused
fresh volume, startup/recovery failure, skipped/unexecuted test, provider error,
request failure, drift, unsafe semantic reuse, credential audit or cleanup failure.
Backend tests keep inner and outer 60-second limits. No selective retries.

1. After the hardware screen, run fresh-volume cache-off primary one/eight-warmup balanced 24-window matrix
   under the existing recovery gate. Keep every early request and correlate model
   timings by output hash; this diagnoses loaded-model behavior, not cold loading.
2. Run the existing bracketing cache off/on/memo low-hit and pure-miss matrix
   against fresh storage, then existing exact/version/protection regressions.
3. Bound Gemini native/gateway tool comparisons to three paired executions.
   Add a real-upstream gated cancellation test that explicitly establishes both
   cancel-before-complete and complete-before-cancel; store timings and sanitized
   status/hash metadata, never raw secrets or signatures.
4. Record collection counts before/after test cleanup. Fresh storage distinguishes
   old-file interference; it does not establish production collection reclamation.
5. Stop only owned containers/processes/tunnels and verify states. Preserve stopped
   diagnostic fixture volumes for review. Unresolved owner decisions stay open.

## Executed scope and reproduction

Completed: two valid 8 RPS blocks (1,000 requests), a separate corrected 1 RPS
block (150 requests), four component probes, one native/gateway Gemini pair,
real gated cancellation/interruption/completion regressions, direct/gateway FAQ
checks, final backend regression, collection inventory and process cleanup.
The incorrectly labelled first block remains excluded; subsequent Gemini pairs
were stopped after the first real failure. The broader 24-window warmup and
full cache matrices were deferred: current measurements identify serial miss
cost, unstable low-rate primary latency and a stream-settlement defect without
justifying another broad sweep. No cold-load or production-capacity claim is made.

Run from the repository root with the existing private `.env.local` and verified
SSH known_hosts. `setup.mjs` builds the isolated runner source; build its Windows
executable and Linux tagged test binary with the repository Go dependencies.
`run.mjs` exposes explicit modes and refuses to overwrite an existing result.
For another VM run, first allocate a new unique fixture prefix/evidence directory
and review `prepare.py`/`cleanup.py`; this completed fixture is intentionally
single-use. `vmctl.go` supplies the bounded SSH script transport; the exact version
used in this run is archived under `executed-v1/`, while the current version
extracts connection setup into a helper. No private config needs to be copied.

Existing evidence can be reevaluated without calling any provider or starting VM
services: run `uv run --no-project python <this-directory>/evaluate.py hardware`
and again with `hardware-low-rate`, then `node <this-directory>/seal.mjs` to audit
credentials and hash the final evidence. Evaluator summaries remain diagnostic;
they do not approve a performance gate.
