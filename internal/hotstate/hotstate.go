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

// Client represents the hot state storage backend, combining multiple capabilities.
type Client interface {
	HealthSnapshotStore
	AuthCache
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
