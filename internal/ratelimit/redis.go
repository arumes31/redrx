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

// Per-phase timeouts. The go-redis defaults are a 5s dial with three retries
// and backoff, so a Redis outage would turn each request into a multi-second
// stall rather than an error — and the limiter's fail-open path only handles
// errors, not hangs.
const (
	dialTimeout  = 500 * time.Millisecond
	readTimeout  = 300 * time.Millisecond
	writeTimeout = 300 * time.Millisecond
)

// NewRedisBackend connects to a redis:// URL.
func NewRedisBackend(url string) (*RedisBackend, error) {
	opts, err := redis.ParseURL(url)
	if err != nil {
		return nil, fmt.Errorf("ratelimit: parse redis url: %w", err)
	}
	opts.DialTimeout = dialTimeout
	opts.ReadTimeout = readTimeout
	opts.WriteTimeout = writeTimeout
	opts.MaxRetries = -1 // -1 disables retries; 0 would mean "use the default"
	opts.PoolTimeout = time.Second
	return &RedisBackend{client: redis.NewClient(opts)}, nil
}

// opTimeout is a hard ceiling on every Redis call. DialTimeout alone does not
// cover name resolution: when the container or pod is removed, Docker/K8s DNS
// stops answering for the hostname and getent can block for seconds — measured
// at ~2.8s per operation. go-redis honours a context deadline through both the
// pool dial and the command, so a deadline here bounds the whole thing
// including DNS, and the limiter's fail-open path turns the resulting error
// into an allowed request rather than a stall.
//
// It must sit above the sum of the per-phase timeouts, or a healthy Redis
// reached over a cold pool — one that must dial, write and read — would be cut
// off mid-connect and fail open needlessly. A small margin covers the second
// read of a pipelined MULTI/EXEC.
const opTimeout = dialTimeout + writeTimeout + readTimeout + 200*time.Millisecond

// Client exposes the connection so other subsystems (the GeoIP cache) can reuse
// it rather than opening a second one.
func (b *RedisBackend) Client() *redis.Client { return b.client }

func (b *RedisBackend) Incr(ctx context.Context, key string, window time.Duration) (int64, error) {
	ctx, cancel := context.WithTimeout(ctx, opTimeout)
	defer cancel()

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
	ctx, cancel := context.WithTimeout(ctx, opTimeout)
	defer cancel()
	return b.client.Ping(ctx).Err()
}

func (b *RedisBackend) Close() error { return b.client.Close() }
