package ratelimit

import (
	"context"
	"fmt"
	"time"

	"github.com/redis/go-redis/v9"
)

// RedisBackend shares limit counters across replicas, matching the
// "redis://..." RATELIMIT_STORAGE_URL the compose stack sets.
type RedisBackend struct {
	client *redis.Client
}

// NewRedisBackend connects to a redis:// URL.
func NewRedisBackend(url string) (*RedisBackend, error) {
	opts, err := redis.ParseURL(url)
	if err != nil {
		return nil, fmt.Errorf("ratelimit: parse redis url: %w", err)
	}
	return &RedisBackend{client: redis.NewClient(opts)}, nil
}

// Client exposes the connection so other subsystems (the GeoIP cache) can reuse
// it rather than opening a second one.
func (b *RedisBackend) Client() *redis.Client { return b.client }

func (b *RedisBackend) Incr(ctx context.Context, key string, window time.Duration) (int64, error) {
	pipe := b.client.TxPipeline()
	incr := pipe.Incr(ctx, key)
	// A generous TTL margin keeps the bucket alive for its whole window even if
	// the first hit lands near a boundary.
	pipe.Expire(ctx, key, window+time.Minute)
	if _, err := pipe.Exec(ctx); err != nil {
		return 0, err
	}
	return incr.Val(), nil
}

func (b *RedisBackend) Ping(ctx context.Context) error {
	return b.client.Ping(ctx).Err()
}

func (b *RedisBackend) Close() error { return b.client.Close() }
