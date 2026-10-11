# Bounded application and CUDA gateway qualification

The user prioritizes exclusion of application defects and permits retaining the
validated candidate budgets if the original 1.05 target remains unmet.

Current findings: 15 focused backend regressions passed without skips/failures.
The 30/30 component arms returned CPU P95 13.564 ms and CUDA P95 4.382 ms.
CUDA vectors differ from CPU (minimum cosine 0.98516 on three inputs), requiring
a fresh model/cache identity; semantic quality is not approved by speed alone.

Next: one bracket with 100 requests per window, off / semantic pure miss / off.
Reuse the unchanged Formal07 app binary after hash verification; use the same
125 ms arrivals, concurrency ceiling 4, memo 0, threshold 0.99999 and deadlines.
Each backend process retains its Go and outer 60-second limits. Start only the
three stopped historical isolated dependencies; enforce no_populate and the
existing recovery gate, preserve all volumes, and stop owned services afterward.

The runner is derived with a Go overlay, changing only the native model alias,
the local native forwarding port to 1236, and adding the unchanged recovery gate
before selected live tests. The original runner source, binary and historical
results are untouched. New runner/overlay/runtime identities are retained.

Load the existing Qwen primary model only for this diagnostic and unload that
owned model after verification. Use the new CUDA alias exclusively, preserving
the original runtime preference and the shared LM Studio HTTP server. The
native CUDA child has its own PATH and no shared priority/power-setting changes.

Validate all measured/raw request, transport and operation records, actual miss
count, metadata, selected PASS, and both off endpoints. Failed or drifting
windows remain in the evidence. A three-window diagnostic cannot replace the
144-window formal run or prove the original performance contract. Do not expand
to another full formal run merely because the small comparison looks promising.
