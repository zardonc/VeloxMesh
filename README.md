# VeloxMesh

VeloxMesh is a lightweight AI gateway and agent orchestration layer for routing, governing, and observing LLM traffic across multiple providers. It combines high-performance request handling with extensible agent workflows, cost control, semantic caching, and production-grade observability.

## Go Gateway Walking Skeleton (Phase 1)

This project uses a Go 1.26.1 backend utilizing the `chi` router for the API gateway.

### Setup

1. **Install Go 1.26.1**
2. **Set Environment Variables**: Copy `.env.example` to `.env` and configure `DEV_API_KEY` and `OPENAI_PRIMARY_API_KEY`.
   ```bash
   cp .env.example .env
   ```
3. **Run the gateway**:
   ```bash
   make run
   ```
4. **Run tests**:
   ```bash
   make test
   ```

### Curl Examples

#### 1. Liveness Check
```bash
curl http://localhost:8080/healthz
```

#### 2. Readiness Check
```bash
curl http://localhost:8080/readyz
```

#### 3. List Models
```bash
curl http://localhost:8080/v1/models \
  -H "Authorization: Bearer vx-dev"
```

#### 4. Chat Completions
```bash
curl -X POST http://localhost:8080/v1/chat/completions \
  -H "Authorization: Bearer vx-dev" \
  -H "Content-Type: application/json" \
  -d '{
    "model": "gpt-4o-mini",
    "messages": [{"role": "user", "content": "Hello!"}]
  }'
```

### Current Planning State

- Phase 1 gateway walking skeleton is implemented.
- Phase 2.1 health-aware multi-provider routing is planned in `.planning/phases/02-health-aware-routing/02-01-PLAN.md` but not yet implemented in source.
- Native Anthropic/Gemini adapters are planned after Go SDK baseline verification.

*Note: Features like Redis cache, PostgreSQL storage, advanced routing, admin API, SSE streaming proxy, Prometheus `/metrics`, and rate limiting are explicitly deferred to later phases.*
