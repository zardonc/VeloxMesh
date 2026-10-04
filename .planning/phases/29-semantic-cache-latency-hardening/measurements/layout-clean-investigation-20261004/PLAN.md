# Collection-layout experiment: preregistration

User authorized VM test-environment cleanup before continuing. Keep all prior
local evidence, production resources and unmounted historical test volumes.
Remove only verified task-owned old /tmp test artifacts with no active process.
Use fresh uniquely labelled Qdrant volumes/containers and the exact cached image.

Failure cases before implementation: unknown/running unrelated resources;
unsafe cleanup path or owner; active test process; secret in evidence; reused
fixture volume; mismatched point/vector/payload digest; inconsistent dimensions,
index/WAL/segment settings; failed page-cache eviction verification; fixture
counts/queries/filter mismatch; missing observations; startup timeout; OOM;
selective retry; cleanup failure. Preserve every failure, never replace a trial.

Two layouts: 1 versus 323 collections, same deterministic 16,542 points,
768-dimensional vectors, payload scopes and global IDs. Explicitly disable vector
index construction in both layouts to avoid a shared-collection-only index-build
confound; identical payload scope index, WAL and segment settings. Configuration
and image/version are recorded before formal trials.

Three blocks, balanced order A/B, B/A, A/B; six starts. Each fixture is prepared
once, counts/digest verified and stopped. Before each start, discard file pages
only for these owned fixture volumes (and initially the historical test volumes),
using fsync/FADV_DONTNEED and residency verification. Do not change global kernel
cache/swap settings. This controls guest file pages, not hypervisor/disk caches.

Before measuring: at least ten consecutive low-pressure seconds, no swap-out,
swap-in <=64KiB/s, memory PSI avg10 <=0.1%, MemAvailable >=6GiB, system CPU<15%,
I/O wait<5%; 120s maximum. This is a new storage-diagnosis protocol, not a rewrite
of the previous failed zero-swap gate or a gateway release criterion.

Measure start-to-ready (120s bound), then fixed 60s idle, then identical scoped
vector queries as a separate stage. Observe guest memory/cgroup/PSI and VM/host
resources continuously. No model requests, other load tools or compilation
during trials. Keep original 1.05 gate and semantic-safety conclusions open.
