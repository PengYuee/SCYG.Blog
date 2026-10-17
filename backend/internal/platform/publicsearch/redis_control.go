// Package publicsearch owns the distributed HTTP admission and connection leases.
package publicsearch

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"time"

	"github.com/go-redis/redis_rate/v10"
	"github.com/google/uuid"
	"github.com/redis/go-redis/v9"

	"github.com/PengYuee/SCYG.Blog/backend/internal/platform/config"
)

// Control shares GCRA buckets and SSE leases across all Blog replicas.
type Control struct {
	client  *redis.Client
	limiter *redis_rate.Limiter
	config  config.PublicSearch
}

// New constructs and pings a bounded, non-retrying Redis connection.
func New(ctx context.Context, cfg config.PublicSearch) (*Control, error) {
	options, err := redis.ParseURL(cfg.RedisURL())
	if err != nil {
		return nil, errors.New("invalid public search Redis configuration")
	}
	options.MaxRetries = -1
	options.DialTimeout = cfg.Timeout()
	options.ReadTimeout = cfg.Timeout()
	options.WriteTimeout = cfg.Timeout()
	options.ContextTimeoutEnabled = true
	client := redis.NewClient(options)
	control := &Control{client: client, limiter: redis_rate.NewLimiter(client), config: cfg}
	bounded, cancel := context.WithTimeout(ctx, cfg.Timeout())
	defer cancel()
	if client.Ping(bounded).Err() != nil {
		_ = client.Close()
		return nil, errors.New("public search Redis unavailable")
	}
	return control, nil
}

// Close releases the owned Redis pool after HTTP streams have drained.
func (c *Control) Close() error { return c.client.Close() }

// Allow applies the explicit minute rate/burst for an IP, never an identity.
func (c *Control) Allow(ctx context.Context, bucket, ip string) (bool, time.Duration, error) {
	var rate, burst int
	switch bucket {
	case "session":
		rate, burst = c.config.SessionRate()
	case "search":
		rate, burst = c.config.SearchRate()
	case "read":
		rate, burst = c.config.ReadRate()
	default:
		return false, 0, errors.New("unknown public search bucket")
	}
	digest := sha256.Sum256([]byte(ip))
	key := c.config.Prefix() + ":public:http:" + bucket + ":" + hex.EncodeToString(digest[:])
	bounded, cancel := context.WithTimeout(ctx, c.config.Timeout())
	defer cancel()
	result, err := c.limiter.Allow(bounded, key, redis_rate.Limit{Rate: rate, Burst: burst, Period: time.Minute})
	if err != nil {
		return false, 0, errors.New("public search Redis unavailable")
	}
	return result.Allowed > 0, result.RetryAfter, nil
}

const acquireScript = `
local t=redis.call('TIME')
local now=t[1]*1000+math.floor(t[2]/1000)
redis.call('ZREMRANGEBYSCORE',KEYS[1],'-inf',now)
if redis.call('ZCARD',KEYS[1]) >= tonumber(ARGV[2]) then return 0 end
redis.call('ZADD',KEYS[1],now+tonumber(ARGV[3]),ARGV[1])
redis.call('PEXPIRE',KEYS[1],ARGV[3])
return 1`

const renewScript = `
local t=redis.call('TIME')
local now=t[1]*1000+math.floor(t[2]/1000)
redis.call('ZREMRANGEBYSCORE',KEYS[1],'-inf',now)
if not redis.call('ZSCORE',KEYS[1],ARGV[1]) then return 0 end
redis.call('ZADD',KEYS[1],now+tonumber(ARGV[2]),ARGV[1])
redis.call('PEXPIRE',KEYS[1],ARGV[2])
return 1`

// Lease is one unique expiring connection ownership token.
type Lease struct {
	control    *Control
	key, token string
}

// Acquire atomically clears expired owners and admits at most the configured connection count.
func (c *Control) Acquire(ctx context.Context, runID string) (*Lease, bool, error) {
	id, err := uuid.NewRandom()
	if err != nil {
		return nil, false, err
	}
	lease := &Lease{control: c, key: c.config.Prefix() + ":public:sse:{" + runID + "}", token: id.String()}
	bounded, cancel := context.WithTimeout(ctx, c.config.Timeout())
	defer cancel()
	result, err := c.client.Eval(bounded, acquireScript, []string{lease.key}, lease.token, c.config.SSEConnections(), c.config.LeaseTTL().Milliseconds()).Int()
	if err != nil {
		return nil, false, errors.New("public search Redis unavailable")
	}
	if result == 0 {
		return nil, false, nil
	}
	return lease, true, nil
}

// Renew succeeds only while this connection still owns an unexpired member.
func (l *Lease) Renew(ctx context.Context) (bool, error) {
	bounded, cancel := context.WithTimeout(ctx, l.control.config.Timeout())
	defer cancel()
	result, err := l.control.client.Eval(bounded, renewScript, []string{l.key}, l.token, l.control.config.LeaseTTL().Milliseconds()).Int()
	if err != nil {
		return false, errors.New("public search Redis unavailable")
	}
	return result == 1, nil
}

// Release removes only this connection, using a fresh bounded cleanup context.
func (l *Lease) Release() {
	ctx, cancel := context.WithTimeout(context.Background(), l.control.config.Timeout())
	defer cancel()
	_ = l.control.client.ZRem(ctx, l.key, l.token).Err()
}
