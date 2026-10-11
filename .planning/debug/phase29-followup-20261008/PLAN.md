# Phase 29 bounded continuation — 2026-10-08

Purpose: revalidate Formal07 against current source and locate the remaining
original 1.05 budget gap without repeating its 43,200 live requests.

The candidate contract remains 1.25 AND +40 ms for semantic miss/low-hit,
+10 ms for exact miss, hit P95 <=60 ms AND <=0.5 times each off endpoint,
and application residual P95 <=10 ms / P99 <=15 ms. Candidate acceptance
does not change the original release contract.

## Predeclared analysis and failure conditions

1. Verify every current product-source hash against the Formal07 manifest.
   Verify the evaluation digest against its independent audit. Reject drift.
2. Read only the 24 pure/low-hit semantic windows, 300 clients per window.
   Verify raw-log hashes, selected PASS, process/validation success, exact hit
   positions, finite timing, no failed transport/operation, and unique measured
   client/stage identities. Exclude nested warmup records explicitly.
3. Compute durations by subtraction WITHIN each measured request, then calculate
   quantiles. Never subtract or add separate stage quantiles. Reject negative
   decompositions beyond the documented three-decimal timing rounding tolerance.
4. Use two explicitly hypothetical sensitivities: (a) make all foreground cost
   other than observed provider and embedding HTTP zero; (b) reduce embedding
   HTTP duration uniformly while holding all other observed times and hit outcomes
   fixed. These are cost screens, not measured improvements, causal estimates,
   achievable runtime predictions, or formal acceptance.
5. Retain original endpoint comparisons and all input/script SHA256 digests in a
   new artifact. Historical files remain unchanged. No model/runtime/container,
   deadline, threshold, production configuration or product-source changes.

Missing evidence: there is no hardware lower-bound proof, no quality-approved
sub-6.5 ms semantic representation, and no current cold-start/high-load acceptance.
These gaps prevent a claim that 1.05 is physically impossible or production-ready.
