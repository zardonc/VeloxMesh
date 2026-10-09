# Same-provider health publication head-of-line investigation

Observed before any product edit: gateway-gpu-01 off request diag-32 spends
14.261 ms in health_provider_sync; overlapping diag-33 waits 14.255 ms between
admission and provider_complete. This supports, but alone does not prove, a
same-provider publication gate convoy. No global mutex spans Redis I/O in the
current implementation; earlier global-lock fixes remain intact.

Current RedisStore.publish serializes each provider/model/probe key across
network I/O. RedisClient already implements OrderedSnapshotWriter with an atomic
version comparison, so same-key serialization may be redundant for that client.
Unordered clients still require the existing gate to prevent stale writes.

Before implementation, add a real Redis/TCP-relay regression for both clients:
hold the first response for 200 ms under an isolated 500 ms sync deadline, then
publish the second same-provider update. Ordered client must complete the second
publication under 40 ms and preserve both pending increments. An interface-hiding
wrapper around the same real Redis client must retain serialization. Capture
RED before product edits. Test-only widened timings make the ordering failure
observable; the production 50 ms deadline must remain unchanged.

If RED confirms the gate cause, bypass only the per-key gate for clients that
implement atomic ordered publication. Keep synchronous bounded publication,
error logging, local immutable snapshots and legacy client behavior. No new
goroutines, retries, weaker health reads or production configuration changes.

Validation expands only after GREEN: existing real concurrent snapshot, stale
publication and unrelated-provider isolation tests; focused health/hotstate
tests and race; static checks; then a small current-binary gateway bracket if
needed. Historical Formal07 stays attached to its original source hash and may
not be represented as a formal result for the changed binary.
