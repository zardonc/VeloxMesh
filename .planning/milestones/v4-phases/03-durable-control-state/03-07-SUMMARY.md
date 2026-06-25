# Phase 03-07 Summary: Durable Control State - Pub/Sub

## Objectives Achieved
1. **Redis Config-Change Pub/Sub**: Extended `hotstate` package with `ConfigChangePublisher` and `ConfigChangeSubscriber` interfaces. Added implementations for `RedisClient` and no-op implementations for `LocalHotState`.
2. **Admin Service Publisher Wiring**: Updated `AdminProviderService` to accept a `ConfigChangePublisher` and emit `{namespace}:config-change` messages upon successful creation, update, disable, or delete of a provider per D-38.
3. **App Subscriber Wiring**: Added `StartConfigChangeSubscriber` to the `App` which asynchronously listens to config-change messages and triggers `App.ReloadProviders` to keep local cluster state consistent per D-38.
4. **Secret Handling Guarantees**: Configured `ConfigChangeMessage` fields such that it does not leak any secrets, raw prompts, or payloads, maintaining strict separation from the data plane per D-35.
5. **Documentation**: Updated the `README.md` to document the Redis Configuration (`REDIS_NAMESPACE`, `REDIS_HEALTH_TTL`, `REDIS_AUTH_CACHE_TTL`, `REDIS_CONFIG_CHANGE_CHANNEL`, `REDIS_DEGRADE_TO_LOCAL`) and the multi-instance consistency mode behavior per D-36, D-37, and D-39.

## Technical Details
- **hotstate**:
  - `ConfigChangeMessage` DTO implemented.
  - `RedisClient.PublishConfigChange` and `SubscribeConfigChanges` wired to Redis pub/sub.
- **controlstate**:
  - `AdminProviderService` augmented with `publisher hotstate.ConfigChangePublisher`.
- **app**:
  - `App.StartConfigChangeSubscriber` invokes cluster-wide state reloads safely.
- **tests**:
  - Unit and Integration tests written and adjusted.
  - Integration `redis_hotstate_test.go` checks safe struct fields via reflection and skipped graceful pub/sub checks.

This concludes Phase 3!
