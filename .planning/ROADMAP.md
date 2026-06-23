# Roadmap: VeloxMesh

**Created:** 2026-06-15
**Mode:** brownfield retrospective initialization
**Current focus:** Planning next milestone

## Overview

VeloxMesh is being built as vertical gateway slices. Phase 1 established the runnable Go/Chi OpenAI-compatible data-plane skeleton. Phase 2 completed the provider and adapter foundation milestone. Phase 3 completed durable control state. Phase 4 added advanced gateway features: streaming, rate limits, caching, and cost governance.

## Milestones

- ✅ **v4** — Phases 1-4 (shipped 2026-06-23)

## Phases

<details>
<summary>✅ v4 (Phases 1-4) — SHIPPED 2026-06-23</summary>

- [x] Phase 1: Gateway Walking Skeleton (1/1 complete)
- [x] Phase 2.1: Health-Aware Multi-Provider Routing (1/1 complete)
- [x] Phase 2.2: Go Version Baseline for Official Provider SDKs (1/1 complete)
- [x] Phase 2.3: Native Anthropic and Gemini Provider Adapters (1/1 complete)
- [x] Phase 2.4: Provider Reliability and Error Contract (1/1 complete)
- [x] Phase 2.5: Provider Retry and Fallback Execution (1/1 complete)
- [x] Phase 2.6: Active Provider Health Probing and Recovery (1/1 complete)
- [x] Phase 2.7: Provider Adapter Capability Contract (2/2 complete)
- [x] Phase 2.8: Provider Configuration Schema and Secret-Safe Validation (1/1 complete)
- [x] Phase 2.9: Provider Model Catalog and Routing Eligibility (1/1 complete)
- [x] Phase 2.10: Adapter Conformance Test Harness (1/1 complete)
- [x] Phase 3: Durable Control State (7/7 complete)
- [x] Phase 4: Streaming, Rate Limits, Cache, and Cost (12/12 complete)

</details>

## Gateway Runtime Modes

VeloxMesh should keep two startup modes explicit:
- **Lite mode**: SQLite-only startup for local or small deployments.
- **Full mode**: PostgreSQL + Redis startup for complete gateway functionality.

## Notes

- Phase 4 is complete. 
- Lite mode uses SQLite only; full mode requires PostgreSQL + Redis.
- Native provider SDK details stay inside adapter packages; handlers and routing consume provider-neutral contracts.
- **Rule**: Source code committed to git must not contain any hardcoded configuration information. Configuration must only be obtained from local environment variables, configuration files, or the database.

## Local Development Resources

The local development environment has been verified and configured. The following resources are available and their specific connection details, models, and credentials can be found in the local `.env` and `.env.local` files:

- **Infrastructure**: 
  - PostgreSQL Database
  - Redis Cache
- **LLM Providers**:
  - `sans` (SANS Primary, with multiple models configured)

---
*Roadmap refreshed: 2026-06-23 after v4 milestone completion*
