# VeloxMesh — pending integration

Updated 2026-10-08. This branch includes all Phase 27–29 commits and planning
history. These notes describe the proposed main integration, not a deployment.

## Gateway behavior

- Stream terminal outcomes and usage settlement are finalized once across
  ordinary, buffered and Fusion paths. Failed/cancelled streams do not debit
  clients; terminal protocol faults remain visible.
- OpenAI-compatible, Anthropic and Gemini tool definitions, choices, fragments
  and result IDs share strict validation and capability-aware routing. Tools
  remain client-executed; Gemini continuations must preserve thought signatures.
- Trusted answer profiles explicitly choose disabled, exact or experimental
  semantic reuse. Reads, writes and shutdown are bounded; scope/version and
  model isolation, readiness and validated lookup-vector reuse avoid unsafe or
  redundant work. Tools and streaming bypass answer caching.
- Optional provider deadlines and shared attempt capacity bound upstream work.
  Unreadable health state fails closed; ordered Redis snapshot publication avoids
  redundant same-key network serialization without relaxing the 50 ms deadline.

## Validation and integration decision

Product source at `214422cb4ca0d40e455ce3dd7468323dbd092005` matches all 411 files
in the latest test manifest. Retained results: 613 backend PASS, 2 explicit SKIP,
0 FAIL; 83 affected race PASS; 7 real-Redis PASS; 24 unique live cache/gateway PASS.
Thirty-six matched performance windows contain 3,600 successful requests and
nine passing candidate scenario/interval checks. These results are separate
runs and must not be added into one test count.

The owner accepted the [candidate budget](docs/cache-reuse-policy.md#accepted-integration-budget--2026-10-08)
for code integration. Semantic miss/low-hit P95 is 119.954/117.114 ms;
the original 1.05 ratio still fails. The three-block check does not replace
the historical 144-window Formal07, which used an older binary.

Production cache remains disabled with an empty allowlist. Business-approved
semantic quality/seed provenance, empty-collection reclamation, historical
upstream EOFs, longer/higher-load capacity and production activation remain open.
See [verification and evidence](.planning/phases/29-semantic-cache-latency-hardening/29-VERIFICATION.md).
