package hotstate

import (
	"context"
	"fmt"
	"time"
)

// HealthSnapshotStore defines the backend storage for health/probe snapshots.
type HealthSnapshotStore interface {
	GetHealthSnapshot(ctx context.Context, providerID string) ([]byte, error)
	SetHealthSnapshot(ctx context.Context, providerID string, data []byte, ttl time.Duration) error
	GetProbeSnapshot(ctx context.Context, providerID string) ([]byte, error)
	SetProbeSnapshot(ctx context.Context, providerID string, data []byte, ttl time.Duration) error
}

// AuthCache defines the backend storage for data-plane API key auth caching.
type AuthCache interface {
	GetCachedAuthResult(ctx context.Context, tokenHash string) (bool, error)
	CacheAuthResult(ctx context.Context, tokenHash string, allowed bool, ttl time.Duration) error
}

// ByteCache defines the backend storage for generic byte-slice caching.
type ByteCache interface {
	GetBytes(ctx context.Context, key string) ([]byte, error)
	SetBytes(ctx context.Context, key string, data []byte, ttl time.Duration) error
	Delete(ctx context.Context, key string) error
}

// AtomicLimiter defines the backend storage for atomic fixed-window counters.
type AtomicLimiter interface {
	// CheckAndIncrement checks if the limit is exceeded. If not, it increments the counter.
	// It returns the current count and whether the request is allowed.
	CheckAndIncrement(ctx context.Context, key string, limit int64, window time.Duration) (int64, bool, error)
}

// SessionBlacklist defines the backend storage for blacklisted sessions.
type SessionBlacklist interface {
	IsBlacklisted(ctx context.Context, sessionID string) (bool, error)
	BlacklistSession(ctx context.Context, sessionID string, ttl time.Duration) error
}

// Constants for ConfigChangeMessage Event Types
const (
	EventProvider      = "provider"
	EventCombo         = "combo"
	EventSemanticRules = "semantic_rules"
	EventAPIKey        = "api_key"
	EventLimitRule     = "limit_rule"
	EventVectorPolicy  = "vector_policy"
)

// ConfigChangeMessage represents a notification about a configuration change.
// It must never contain secrets, api keys, or raw payloads per D-35.
type ConfigChangeMessage struct {
	Type       string    `json:"type"`        // The type of event (e.g., provider, combo, api_key)
	TargetID   string    `json:"target_id"`   // The ID of the affected resource
	ProviderID string    `json:"provider_id"` // Deprecated/legacy compatibility
	Action     string    `json:"action"`      // e.g., "create", "update", "disable", "delete"
	Revision   int64     `json:"revision"`
	Timestamp  time.Time `json:"timestamp"`
}

// Subscription represents an active subscription to config changes.
type Subscription interface {
	Channel() <-chan *ConfigChangeMessage
	Close() error
}

// ConfigChangePublisher allows publishing config change notifications.
type ConfigChangePublisher interface {
	PublishConfigChange(ctx context.Context, msg *ConfigChangeMessage) error
}

// ConfigChangeSubscriber allows subscribing to config change notifications.
type ConfigChangeSubscriber interface {
	SubscribeConfigChanges(ctx context.Context) (Subscription, error)
}

// Client represents the hot state storage backend, combining multiple capabilities.
type Client interface {
	HealthSnapshotStore
	AuthCache
	ByteCache
	AtomicLimiter
	SessionBlacklist
	ConfigChangePublisher
	ConfigChangeSubscriber
	Ping(ctx context.Context) error
	Close() error
}

// NamespacedKey generates a consistent key with the configured namespace.
func NamespacedKey(namespace, component, id string) string {
	if namespace == "" {
		return fmt.Sprintf("unnamespaced:%s:%s", component, id) // should be prevented by config validation
	}
	return fmt.Sprintf("%s:%s:%s", namespace, component, id)
}
