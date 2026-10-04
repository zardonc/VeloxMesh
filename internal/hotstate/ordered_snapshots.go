package hotstate

import (
	"context"
	"fmt"
	"time"

	"github.com/redis/go-redis/v9"
)

type SnapshotWrite struct {
	Category, Key, Version string
	Data                   []byte
	TTL                    time.Duration
}

type OrderedSnapshotWriter interface {
	SetSnapshotIfNewer(context.Context, SnapshotWrite) error
}

// Compare fixed-width nanosecond strings rather than Lua doubles, which cannot
// exactly represent the timestamp. Value and version share atomic expiry.
var orderedSnapshotScript = redis.NewScript(`
local previous = redis.call("GET", KEYS[2])
if previous and previous >= ARGV[1] then return 0 end
redis.call("SET", KEYS[1], ARGV[2], "PX", ARGV[3])
redis.call("SET", KEYS[2], ARGV[1], "PX", ARGV[3])
return 1
`)

func (r *RedisClient) SetSnapshotIfNewer(ctx context.Context, write SnapshotWrite) error {
	if write.Key == "" || write.Version == "" || write.TTL < time.Millisecond {
		return fmt.Errorf("invalid ordered snapshot identity or TTL")
	}
	switch write.Category {
	case "health", "probe", "bytes":
	default:
		return fmt.Errorf("unsupported snapshot category %q", write.Category)
	}
	key := NamespacedKey(r.namespace, write.Category, write.Key)
	versionKey := NamespacedKey(r.namespace, "snapshot-version", write.Category+":"+write.Key)
	return orderedSnapshotScript.Run(ctx, r.client, []string{key, versionKey}, write.Version, write.Data, write.TTL.Milliseconds()).Err()
}
