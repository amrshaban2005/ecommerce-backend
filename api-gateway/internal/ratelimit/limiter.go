// Package ratelimit provides Redis-backed fixed-window rate limiting per client IP.
package ratelimit

import (
	"context"
	"fmt"
	"log/slog"
	"strconv"
	"time"

	"github.com/redis/go-redis/v9"
)

// Limiter implements a Redis-backed 60-second counter per client IP.
type Limiter struct {
	redis   *redis.Client
	limit   int
	timeout time.Duration
}

// Result holds the rate limit check outcome.
type Result struct {
	Allowed    bool
	Remaining  int
	Limit      int
	ResetAfter time.Duration
}

// Option configures a Limiter.
type Option func(*Limiter)

// WithCheckTimeout overrides the Redis check timeout.
func WithCheckTimeout(timeout time.Duration) Option {
	return func(l *Limiter) {
		if timeout > 0 {
			l.timeout = timeout
		}
	}
}

// New creates a Limiter with the given Redis client and per-IP limit (req/min).
func New(rdb *redis.Client, limit int, opts ...Option) *Limiter {
	l := &Limiter{redis: rdb, limit: limit, timeout: 10 * time.Millisecond}

	for _, opt := range opts {
		opt(l)
	}
	return l
}

// Check performs a fixed-window check for the given client IP.
// Returns the limit result. If Redis is unavailable, allows the request (fail-open)
// and logs a warning.
func (l *Limiter) Check(ctx context.Context, clientIP string) Result {
	limit := l.limit
	key := fmt.Sprintf("ip:%s:ratelimit", clientIP)

	ctx, cancel := context.WithTimeout(ctx, l.timeout)
	defer cancel()

	// Lua script for an atomic fixed-window counter:
	// increment counter, set TTL=60s on first call, return (count, ttl)
	script := redis.NewScript(`
local key = KEYS[1]
local limit = tonumber(ARGV[1])
local current = redis.call('INCR', key)
if current == 1 then
  redis.call('EXPIRE', key, 60)
end
local ttl = redis.call('TTL', key)
return {current, ttl, limit}
`)

	result, err := script.Run(ctx, l.redis, []string{key}, limit).Slice()
	if err != nil {
		slog.Warn("redis ratelimit unavailable, fail-open",
			"client_ip", clientIP,
			"error", err,
			"service", "api-gateway",
		)
		return Result{Allowed: true, Remaining: limit, Limit: limit, ResetAfter: 60 * time.Second}
	}

	current := toInt(result[0])
	ttl := toInt(result[1])
	if ttl < 0 {
		ttl = 60
	}

	remaining := limit - current
	if remaining < 0 {
		remaining = 0
	}

	return Result{
		Allowed:    current <= limit,
		Remaining:  remaining,
		Limit:      limit,
		ResetAfter: time.Duration(ttl) * time.Second,
	}
}

func toInt(v any) int {
	switch val := v.(type) {
	case int64:
		return int(val)
	case string:
		i, _ := strconv.Atoi(val)
		return i
	}
	return 0
}
