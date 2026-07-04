# Requirements: VeloxMesh

**Defined:** 2026-07-03
**Core Value:** Client applications can call one OpenAI-compatible gateway endpoint and reliably reach the right LLM provider through a low-latency, observable, provider-agnostic routing layer.

## v7.4 Requirements

### Scheduler Foundation

- [x] **SCH-01**: Operators can leave Scheduler disabled and the gateway uses FIFO queue scoring without startup failure.
- [x] **SCH-02**: Gateway can call Scheduler `BatchScoreTasks` over gRPC with a 15ms timeout and FIFO fallback on failure or timeout.
- [x] **SCH-03**: Gateway can queue scheduled tasks through a `QueueBackend` with Redis ZSET as the primary backend and an in-memory min-heap as single-node fallback.
- [x] **SCH-04**: Operators can run a separate heuristic Scheduler service with gRPC scoring plus HTTP `/health` and `/metrics` endpoints.

### Priority and Scoring

- [x] **PRIO-01**: Gateway resolves task priority only from trusted configuration, service headers, or structured request fields.
- [x] **PRIO-02**: Gateway enforces tenant max-priority limits and high-priority quotas, silently downgrading claims that exceed policy.
- [x] **SCORE-01**: Scheduler computes static virtual deadline scores from enqueue time, predicted latency, priority multiplier, and uncertainty penalty.
- [x] **SCORE-02**: Cold-start Scheduler classifies structured/rule task types and estimates latency from configured heuristic tables.

### Feedback and Observability

- [ ] **FEED-01**: Gateway records enqueue feature snapshots and completion labels for scheduler training without storing raw prompts, authorization headers, API keys, or provider secrets.
- [x] **OBS-01**: Gateway and Scheduler expose logs and metrics for queue depth, scheduler call latency, scheduler errors, breaker state, priority downgrades, scoring duration, and classification source.
- [ ] **OBS-02**: Operators can compare prediction quality by scheduler type, version, and task type during heuristic versus ONNX rollout.

### Model Path

- [ ] **ML-01**: Offline tooling can export completed training samples, train and evaluate a P70 output-token predictor, and publish versioned model artifacts.
- [ ] **ML-02**: ONNX Scheduler can load model artifacts once at startup and return predicted latency, confidence, and scheduler version without per-request model reload.
- [ ] **ML-03**: Gateway can route traffic between heuristic and ONNX Scheduler backends for A/B comparison and rollback.

## Future Requirements

### Scheduler Enhancements

- **QDR-01**: ONNX Scheduler can optionally add Qdrant semantic-neighbor features when an instance is configured.
- **ANOM-01**: ONNX Scheduler can optionally use anomaly detection to make out-of-distribution tasks more conservative.
- **SLA-01**: Gateway can promote tasks that exceed tenant-specific SLA waiting thresholds.
- **ADMIN-02**: BFF/Admin Console can display queue and scheduler health after Phase 11 exists.

## Out of Scope

| Feature | Reason |
| --- | --- |
| Prompt-derived priority | Prompt text is untrusted and must not affect scheduling priority. |
| Scheduler-owned queueing or execution | Gateway remains responsible for intake, queue storage, task execution, and result persistence. |
| Dynamic Redis score re-ranking | Static virtual deadlines avoid Redis write amplification and preserve simple aging behavior. |
| Mandatory ONNX model at cold start | Heuristic scoring must work before enough training data exists. |
| BFF/Admin Console UI | This milestone is backend scheduling infrastructure; UI stays deferred. |

## Traceability

| Requirement | Phase | Status |
| --- | --- | --- |
| SCH-01 | Phase 14 | Complete |
| SCH-02 | Phase 14 | Complete |
| SCH-03 | Phase 14 | Complete |
| SCH-04 | Phase 14 | Complete |
| PRIO-01 | Phase 14 | Complete |
| PRIO-02 | Phase 14 | Complete |
| SCORE-01 | Phase 14 | Complete |
| SCORE-02 | Phase 14 | Complete |
| OBS-01 | Phase 14 | Complete |
| FEED-01 | Phase 15 | Pending |
| ML-01 | Phase 15 | Pending |
| ML-02 | Phase 15 | Pending |
| OBS-02 | Phase 16 | Pending |
| ML-03 | Phase 16 | Pending |
| QDR-01 | Future | Future |
| ANOM-01 | Future | Future |
| SLA-01 | Future | Future |
| ADMIN-02 | Future | Future |

**Coverage:**

- v7.4 requirements: 14 total
- Mapped to phases: 14
- Unmapped: 0

---
*Requirements defined: 2026-07-03*
*Last updated: 2026-07-03 after starting v7.4 Gateway Scheduler*
