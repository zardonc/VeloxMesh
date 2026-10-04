package health

import (
	"context"
	"fmt"

	"veloxmesh/internal/hotstate"
)

func (s *RedisStore) writeSnapshot(ctx context.Context, write hotstate.SnapshotWrite) error {
	if ordered, ok := s.client.(hotstate.OrderedSnapshotWriter); ok {
		return ordered.SetSnapshotIfNewer(ctx, write)
	}
	switch write.Category {
	case "health":
		return s.client.SetHealthSnapshot(ctx, write.Key, write.Data, write.TTL)
	case "probe":
		return s.client.SetProbeSnapshot(ctx, write.Key, write.Data, write.TTL)
	case "bytes":
		return s.client.SetBytes(ctx, write.Key, write.Data, write.TTL)
	default:
		return fmt.Errorf("unsupported health snapshot category %q", write.Category)
	}
}
