# AGENTS.md

## Project

VeloxMesh is a Go/Chi AI gateway. The gateway exposes OpenAI-compatible data-plane endpoints and hides provider-specific behavior behind Go provider adapters.

Read these project-level planning files before planning or implementing:

- `.planning/PROJECT.md`
- `.planning/REQUIREMENTS.md`
- `.planning/ROADMAP.md`
- `.planning/STATE.md`

## Current State

- Phase 1 is implemented: Go gateway walking skeleton, static auth, `/healthz`, `/readyz`, `/v1/models`, `/v1/chat/completions`, static routing, pass-through admission, and one OpenAI-compatible adapter.
- Phase 2.1 is planned but not implemented in source: multi-provider static config, in-memory health store, health-aware routing, readiness/model aggregation updates, and tests.
- The current code still has single-provider env config and `StaticRouter`.
- Go baseline in `go.mod` is `1.26.1`.

## Important Constraints

- Keep the gateway Go-first. Do not introduce a Node/TypeScript gateway implementation.
- Preserve the client-facing OpenAI-compatible API contract.
- Keep provider-native request/response mapping inside provider adapter packages.
- Do not log API keys, authorization headers, raw prompts, or sensitive provider payloads.
- Do not add PostgreSQL, Redis, Admin API, streaming, semantic cache, or cost governance unless the active phase explicitly scopes it.

## Workflow Notes

- Use `.planning/phases/02-health-aware-routing/02-01-PLAN.md` as the next implementation plan.
- If planning native Anthropic/Gemini adapters, first settle the Go baseline and prefer the official Anthropic Go SDK where practical.
- Run `go test ./...` after Go changes.

