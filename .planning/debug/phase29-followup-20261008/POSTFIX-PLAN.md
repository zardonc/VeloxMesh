# Final bounded gateway check

Real Redis GREEN: ordered same-provider publication 1.312 ms versus 199.426 ms
before the fix; legacy serialization remains 199.310 ms. Real C8/200 updates,
stale-write rejection and unrelated-provider isolation pass. Focused unit/race,
vet, tagged vet and Linux build passed. No further product changes are planned.

Run one 100-request off/on/off pure-miss bracket using the newly frozen 410-file
manifest and app-green-linux.test. Reuse CPU AVX2 2.53, Nomic v1.5 Q4_K_M,
threads4/4, original 100ms cache read deadline, memo0 and threshold0.99999.
This diagnostic leaves the owned native child at Normal priority, so it is not
an exact reproduction of Formal07's AboveNormal condition. Its generic alias
is used only inside fresh per-test API-key scopes; no historical vectors are
looked up. It does not adopt the CUDA representation or change production.

Require the same recovery gate, all 300 successful clients, no failed operations,
correct actual profile, both off endpoints and explicit P50/P95/P99 residuals.
Any endpoint drift or candidate-budget miss is retained. Do not escalate into
another long formal run to hide diagnostic variability. The user's accepted
candidate contract remains separate from original1.05; historical Formal07 is
evidence for its original source, not a fresh formal pass for the changed binary.
