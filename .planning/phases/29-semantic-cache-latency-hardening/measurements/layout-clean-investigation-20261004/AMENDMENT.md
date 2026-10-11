# Attempt 01 tooling failure and complete replacement matrix

Attempt 01 stopped after one shared-collection trial and 100 successful scoped
queries. Both resource observers emitted a normal observer-stop and exited zero.
The runner then called SSH Session.Close after Session.Wait had consumed channel
termination. Redundant Close returned EOF twice; these errors aborted the matrix.
Retain attempt 01, its binary hashes, sources, observations and failure log.

Correct the runner by retaining Session.Wait errors and output-file close errors,
and omitting the redundant post-Wait channel Close. The parent SSH client still
closes at function exit. No observer error is suppressed. Do not count attempt 01
as a completed comparison or selectively retry its missing trials.

Matrix 02 uses two further fresh volumes, the same cached image, unchanged Linux
probe, dataset and protocol, and reruns all six balanced trials. Paths have v2
fixture names and matrix-02 evidence/remote directories. Evict the two stopped
attempt-01 fixture volumes before the new matrix to remove their file-page
residency. Baseline criteria and all failures remain unchanged.
