# Provider attempt protection

Optional root configuration `provider_protection` maps provider IDs to protection profiles. It applies to runtime static and durable adapters. Missing profiles preserve the existing adapter behavior; no guessed production timeout or concurrency limit is enabled.

| Field | Meaning |
| --- | --- |
| `first_byte_timeout` | Attempt start through the first upstream HTTP response byte, including connection setup. |
| `first_content_timeout` | Attempt start through the first content/tool/finish event for a stream. For non-streaming completion or embedding, this bounds the fully decoded response body. |
| `stream_idle_timeout` | Maximum wait for subsequent meaningful upstream stream progress. Empty events/heartbeats do not reset it. Downstream backpressure pauses this idle clock. |
| `total_timeout` | Overall attempt deadline; required when a phase timeout is configured. |
| `max_inflight` | Process-local simultaneous attempts; zero disables the limit. Saturation returns HTTP 429 with `provider_concurrency_full`. |
| `resource_group` | Shared capacity pool for provider IDs using the same upstream resource, including chat and embedding. Omission uses the provider ID. |

Durations must be positive Go durations up to one hour; omitted fields disable their individual controls. Capacity is between zero and 4096. Providers in the same resource group must configure the same capacity. Protection profiles are process configuration: restart to change them. Registry replacement reuses the same pools and does not reset active attempts.

Permits cover actual completion, embedding and stream attempts. Admission rejects immediately without adding a second queue. Completion/error/cancellation releases a permit once; streaming retains it until the actual adapter reader exits. Custom adapters must honor context cancellation and close their stream channel. This is a per-process bound; multiple gateway processes need a separate aggregate capacity decision.

Distinct 504 codes identify first-byte, first-content, stream-idle and overall timeouts. These codes and local capacity rejection do not trigger automatic provider retry. Capacity rejection does not mark an upstream provider unhealthy. A cancelled request can still incur upstream provider cost; zero gateway settlement is not proof of zero provider billing.

Timeout and capacity values in Phase 29 fault/protocol tests are isolated candidates. Select production values from capacity, cold-start and user latency evidence for each provider and deployment.

Health-store read failures use a separate [health dependency error contract](health-state-errors.md); they do not establish that the upstream model is unhealthy.
