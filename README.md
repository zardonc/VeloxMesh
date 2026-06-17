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
This `CONFIG_FILE` approach is a temporary Phase 2 bootstrap path for local/static configuration. In later development, provider configuration will be managed at runtime through the Admin Console and persisted in the database.
Set the `CONFIG_FILE` environment variable to a JSON file like:
```json
{
  "routing_strategy": "least-latency",
  "default_provider": "openai-1",
  "fallback_enabled": true,
  "max_attempts": 3,
  "health_check": {
    "enabled": true,
    "interval": "30s",
    "timeout": "2s",
    "failure_threshold": 3,
    "success_threshold": 1,
    "max_concurrency": 4
  },
  "providers": [
    {
      "id": "openai-1",
      "type": "openai-compatible",
      "base_url": "https://api.openai.com/v1",
      "auth": {
        "api_key_env": "OPENAI_API_KEY"
      },
      "models": ["gpt-4o", "gpt-4o-mini"],
      "default_model": "gpt-4o-mini",
      "timeout": "30s"
    },
    {
      "id": "anthropic-1",
      "type": "anthropic",
      "base_url": "https://api.anthropic.com",
      "auth": {
        "api_key_env": "ANTHROPIC_API_KEY"
      },
      "models": ["claude-3-5-sonnet-20241022", "claude-3-haiku-20240307"],
      "default_model": "claude-3-5-sonnet-20241022"
    },
    {
      "id": "gemini-1",
      "type": "gemini",
      "base_url": "https://generativelanguage.googleapis.com/v1beta",
      "auth": {
        "api_key_env": "GEMINI_API_KEY"
      },
      "models": ["gemini-2.5-flash", "gemini-1.5-pro"],
      "default_model": "gemini-2.5-flash"
    }
  ]
}
```

#### Secret Safety
To keep your configuration secret-safe, define API keys as environment variable references (`api_key_env`) rather than hardcoding them in the JSON file. The legacy `api_key` string field remains accepted for backward compatibility, but committing raw keys is strongly discouraged.

#### Validation Rules
When starting the gateway, the configuration is strictly validated to ensure robust behavior:
- **Provider IDs**: Must be non-empty and unique.
- **Provider Types**: Supported types are `openai-compatible`, `anthropic`, and `gemini`.
- **Base URLs**: Must be non-empty and use `http` or `https` schemes.
- **Models**: The `models` list cannot be empty. If `default_model` is set, it must exist in `models`.
- **Durations**: Timeouts and intervals must be valid, non-negative duration strings (e.g., `"30s"`, `"2s"`).
- **Health Check Thresholds**: Global and provider-specific success/failure thresholds and `max_concurrency` must be at least `1`.
- **Fallback**: If enabled, `max_attempts` cannot exceed the number of configured providers. If disabled, `max_attempts` is locked to `1`.

By default, fallback across providers is enabled if more than one provider is configured. You can use the `X-Route-To` header to strictly override routing to a specific provider. When a strict override is used, fallback attempts are disabled.

*Note: This static JSON configuration path is temporary. Features like Admin API, Admin Console, PostgreSQL-backed provider configuration, Redis cache, runtime config hot reload, advanced routing, SSE streaming proxy, rate limiting, semantic cache, cost governance, or live model discovery are explicitly deferred to later phases.*
