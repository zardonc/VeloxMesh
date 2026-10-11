---
status: investigating
trigger: Continue Phase 29 diagnosis after the memory upgrade, controlling variables.
updated: 2026-10-04
---

## Evidence and hypotheses

The previous complete run retained 4,800 successful foreground requests. All
14 cache-off requests above 250ms were in off-before positions 1–3. The slowest
first requests reused a connection and wrote it in about 0.03ms; most did not
coincide with VM swap-in. Delay was predominantly before the nonstreaming
provider's first byte, which cannot distinguish engine scheduling from decode.

Hypotheses: one primary warmup does not warm all execution slots; the first
window follows binary/tunnel setup; host inference scheduling changes latency.
The existing 347 Qdrant collections are a separate scaling hypothesis. Do not
delete collections or treat a changing volume as a RAM-only experiment.

## Preregistered failure cases

Missing or skipped named tests; incorrect warmup counts; warmup HTTP failure;
formal HTTP failure; changed cache profile; incomplete sample/stage coverage;
model inventory changes; LM timing observer parse failure; recovery not achieved
within 120 seconds; overwrite of evidence; new Qdrant collections. These are
failures or explicit limitations, never replacement runs or passing skips.

## Next experiment

Cache OFF only, six ABBA/BAAB blocks, one versus eight identical sequential
primary warmups. Both arms wait two seconds. Every window sends 100 questions
at 125ms with ceiling four; keep all early samples. Upload once and hold one
tunnel session throughout. Capture LM output hashes and numeric engine timing
without retaining output text. Record recovery separately before the matrix:
ten consecutive seconds with zero swap I/O, low block I/O and low system/wait
CPU; record MemAvailable and memory PSI. No compilation or inventory probing
inside formal windows. This experiment does not retest cache-on SLOs.

## Results

Recovery failed at 120 seconds: 119 intervals, 84 with swap activity, maximum
six consecutive quiet seconds. No formal primary-warmup window ran. A separate
seven-condition idle experiment completed, with only one existing component
running at a time. Qdrant alone caused significant startup swap-out/I/O, unlike
Redis/PostgreSQL; its final 6.30GiB cgroup usage was mostly mapped file cache.
Stopped-volume metadata showed 63.80GiB apparent versus 212.27MiB allocated,
with many sparse WAL/payload/vector chunks across 347 unchanged collections.

Collection-per-scope and point-only expiry cleanup are confirmed in source.
Collection count causality, per-file residency and primary early-request engine
timing remain open. See 29-RECOVERY-INVESTIGATION-20261004.md. No criteria were
relaxed; no product changes, collection deletion or production enablement.
