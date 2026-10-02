# 29-03 partial execution — local model

Status: **partial; not a completion summary**. `CACHE-F01`, roadmap 29-03 and Phase 29 remain open.

The user-selected local embedding model passes a real application/gateway/Qdrant paraphrase and negative flow after a real failing case drove the shared embedding-input fix in `c80be553`. Same authenticated key/model/database version switching passes. Embedding-fault forwarding and a real queue-full/close burst pass. Full backend tests, focused race tests and vet pass. Model IDs and dimensions remain configured/probed; production settings are unchanged.

Evidence: `29-LOCAL-MODEL-MEASUREMENT.md` and `measurements/local-embedding-20261001/` retain 1044 timed requests, successful and failed logs, committed code/binary identities, complete-response and separate operation quantiles, resource snapshots, concrete isolation candidates and owner boundaries.

The performance task is **failed/open**, not passed: corrected same-schedule low-hit P95 ratios 1.143 and 1.511 exceed 1.05; another off-run warmup timed out. Primary deadline failures persist even with caching off. Short bounded slices and observed ceilings do not prove long steady-state capacity. The recorder may include a late asynchronous warmup store and must be started after warmup persistence in the next release-grade baseline.

The real-model task is **partial**: only the requested local model was selected. The original second distinct model and live cross-model lifecycle remain unverified; deterministic isolation/fault coverage does not substitute for that gate. No final human-verify checkpoint is claimed ready and no final parameter approval is inferred.

Next execution must resolve stable representative primary/load conditions, complete clean-warmup sustained samples and the second distinct model flow, then present passing evidence for the plan's final human review. Production publisher/atomic cutover/deployment stop ownership remains separately required before production enablement.

## 2026-10-01 functional acceptance amendment

The user now authorizes local-model verification for this acceptance round and report-only handling of nonfunctional/environment exceptions. Fresh full backend (592 passed, two unrelated opt-in skips), race and all three selected local-model functional runners pass. `29-UAT.md` is complete for this adjusted scope, and `29-VERIFICATION.md` records the original online/performance items as deferred. The preceding next-execution paragraph describes the original release gate, not work authorized to continue during this functional acceptance. No performance pass, second-model pass, final numeric approval or production enablement is inferred.
