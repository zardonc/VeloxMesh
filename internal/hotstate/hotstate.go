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

// ConfigChangeMessage represents a notification about a provider configuration change.
// It must never contain secrets, api keys, or raw payloads per D-35.
type ConfigChangeMessage struct {
	ProviderID string    `json:"provider_id"`
	Action     string    `json:"action"` // e.g., "create", "update", "disable", "delete"
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
