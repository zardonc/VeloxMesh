# Health dependency error contract

Provider health and health-store availability are separate facts. A Redis snapshot GET or decode failure retains its original server-side log and exposes ephemeral `ProviderSnapshot.ReadError`. This field is excluded from JSON, is not a provider failure counter and is not persisted as model `LastError`.

Routing excludes a candidate whose health state cannot be read. If no readable healthy candidate remains and a candidate read failed, normal, override, round-robin, capacity and Fusion selection return HTTP 503 with `health_state_unavailable`. Readable healthy peers remain usable. A readable unhealthy snapshot retains the existing `no_healthy_provider` or `unhealthy_provider_override` rejection.

The dependency error does not count as an upstream provider failure and does not trigger automatic provider retry. Public responses contain a stable category and generic message; Redis addresses and underlying exception details remain in server logs.

This change preserves the existing 50ms snapshot deadline and fail-closed behavior. It improves diagnosis; it does not make a stalled health store available. Serving a stale local snapshot would require a separate maximum-age and consistency contract, including cross-process revocation behavior.

Real Redis TCP-delay, authenticated HTTP, routing-mode, recovery and healthy-peer evidence is recorded in [the Phase 29 availability investigation](../.planning/phases/29-semantic-cache-latency-hardening/29-AVAILABILITY-INVESTIGATION-20261004.md).
