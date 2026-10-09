# Current small-model device screen

Before execution: compare native CPU AVX2 2.53 versus native CUDA12 AVX2 2.53
with the identical installed Nomic v1.5 Q4_K_M weight SHA256 used by Formal07.
This is a deployment comparison, not identical compiler/kernel/device causality.

- One CPU arm and one GPU arm, each 30 varied questions at 125 ms arrivals,
  concurrency ceiling 4; no chat model or gateway/container traffic.
- Each arm has 3 separately retained fixed-vector calls and 2 warmups.
  A 2-second quiet interval precedes measured arrivals. No failures are replaced.
- Same context/batch/ubatch 512, pooling mean, L2 normalization, CPU threads 4/4,
  normal priority, new alias and port 1236. CPU layers 0 versus GPU layers 99.
- CUDA child PATH alone includes its manifest-declared installed vendor directory.
  No global environment, runtime preference, power plan or shared service changes.
- Verify actual GPU offloading in the server log, finite 768-dimensional vectors,
  same-input vector differences, CPU counters and sampled GPU clocks.
- Each child has a hard 55-second watchdog; request deadline 1 second; owned
  children and monitor are stopped and port freedom verified after each arm.
- Record all requests, dispatch lag, failures, startup errors and provenance.
  The diagnostic is not formal P95 evidence. If GPU P95 is not below 8 ms,
  do not expand this device route merely to seek a passing sample. A faster
  result would still require vector/quality and concurrent-chat qualification.

Current preflight: no LM Studio models loaded, GPU idle P8, 7.18 GiB available
memory, approximately 17.6% sampled host CPU, port 1236 free. These are a short
screen, not a capacity proof. Original 1.05 and candidate budgets remain distinct.

User steering: application causes take priority; candidate-budget closure is
acceptable if the original target remains unmet after bounded investigation.
