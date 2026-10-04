CREATE INDEX IF NOT EXISTS idx_semantic_cache_readiness
ON semantic_cache_entries (scope, model, expires_at)
WHERE enabled = true;
