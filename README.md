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
  -H "X-Route-To: openai-primary" \
  -d '{
    "model": "gpt-4o-mini",
    "messages": [{"role": "user", "content": "Hello!"}]
  }'
```

### Multi-Provider Configuration (Phase 2)

VeloxMesh supports dynamic routing across multiple providers via a JSON configuration file.
Set the `CONFIG_FILE` environment variable to a JSON file like:
```json
{
  "routing_strategy": "least-latency",
  "default_provider": "openai-1",
  "providers": [
    {
      "id": "openai-1",
      "type": "openai-compatible",
      "base_url": "https://api.openai.com/v1",
      "api_key": "YOUR_KEY",
      "models": ["gpt-4o", "gpt-4o-mini"]
    }
  ]
}
```

*Note: Features like Redis cache, PostgreSQL storage, advanced routing, admin API, SSE streaming proxy, Prometheus `/metrics`, and rate limiting are explicitly deferred to later phases.*
