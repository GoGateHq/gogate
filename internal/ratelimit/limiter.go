// Copyright (c) 2026 Opportunation / GoGate Authors
// SPDX-License-Identifier: AGPL-3.0-or-later

package ratelimit

import (
	"context"
	"crypto/rand"
	"encoding/hex"
	"fmt"
	"log/slog"
	"sync/atomic"
	"time"

	"github.com/redis/go-redis/v9"

	"github.com/gogatehq/gogate/internal/config"
)

// memberCounter is a fallback monotonic counter used to generate unique ZADD
// members when crypto/rand is unavailable.
var memberCounter atomic.Uint64

// slidingWindowScript is a Lua script that implements a sliding-window rate
// limiter using a Redis sorted set. Each request is recorded as a member
// scored by its timestamp in microseconds. Expired members are pruned on
// every call.
//
// KEYS[1] = rate limit key
// ARGV[1] = window start (now - window) in microseconds
// ARGV[2] = now in microseconds
// ARGV[3] = max allowed requests (limit)
// ARGV[4] = window size in milliseconds (for PEXPIRE)
// ARGV[5] = unique member value
//
// Returns: {current_count, 0=allowed/1=denied}
var slidingWindowScript = redis.NewScript(`
local key = KEYS[1]
local window_start = tonumber(ARGV[1])
local now = tonumber(ARGV[2])
local limit = tonumber(ARGV[3])
local window_ms = tonumber(ARGV[4])
local member = ARGV[5]

redis.call('ZREMRANGEBYSCORE', key, 0, window_start)
local count = redis.call('ZCARD', key)

if count < limit then
    redis.call('ZADD', key, now, member)
    redis.call('PEXPIRE', key, window_ms)
    return {count + 1, 0}
end

redis.call('PEXPIRE', key, window_ms)
return {count, 1}
`)

// Result holds the outcome of a rate limit check.
type Result struct {
	Allowed   bool
	Limit     int
	Remaining int
	ResetAt   time.Time
}

// Limiter performs Redis-backed sliding-window rate limiting.
type Limiter struct {
	client    *redis.Client
	keyPrefix string
	failOpen  bool
	logger    *slog.Logger
}

// NewLimiter creates a rate limiter connected to Redis.
func NewLimiter(redisCfg config.RedisConfig, rlCfg config.RateLimitConfig, logger *slog.Logger) (*Limiter, error) {
	client := redis.NewClient(&redis.Options{
		Addr:         redisCfg.Addr,
		Password:     redisCfg.Password,
		DB:           redisCfg.DB,
		DialTimeout:  redisCfg.DialTimeout,
		ReadTimeout:  redisCfg.ReadTimeout,
		WriteTimeout: redisCfg.WriteTimeout,
	})

	ctx, cancel := context.WithTimeout(context.Background(), redisCfg.DialTimeout)
	defer cancel()

	if err := client.Ping(ctx).Err(); err != nil {
		// If fail-open, log the error and proceed; otherwise fail startup.
		if rlCfg.IsFailOpen() {
			logger.Warn("redis unavailable at startup, rate limiting will be bypassed", "addr", redisCfg.Addr, "error", err)
		} else {
			client.Close()
			return nil, fmt.Errorf("redis ping failed: %w", err)
		}
	}

	return &Limiter{
		client:    client,
		keyPrefix: rlCfg.KeyPrefix,
		failOpen:  rlCfg.IsFailOpen(),
		logger:    logger,
	}, nil
}

// Allow checks whether a request identified by key is within the rate limit.
// The window is always 1 minute, and limit is the max RPM for the service.
func (l *Limiter) Allow(ctx context.Context, key string, limit int) (Result, error) {
	if limit <= 0 {
		return Result{Allowed: true, Limit: 0, Remaining: 0}, nil
	}

	now := time.Now()
	windowStart := now.Add(-1 * time.Minute)
	fullKey := l.keyPrefix + key

	nowMicro := now.UnixMicro()
	// Append a unique suffix to ensure distinct ZADD members even when
	// concurrent requests share the same microsecond timestamp.
	// Primary: 4 random bytes. Fallback: monotonic atomic counter
	// so uniqueness is guaranteed even if crypto/rand fails.
	var suffix string
	var rndBuf [4]byte
	if _, err := rand.Read(rndBuf[:]); err != nil {
		suffix = fmt.Sprintf("c%d", memberCounter.Add(1))
	} else {
		suffix = hex.EncodeToString(rndBuf[:])
	}
	member := fmt.Sprintf("%d-%s", nowMicro, suffix)

	res, err := slidingWindowScript.Run(ctx, l.client, []string{fullKey},
		windowStart.UnixMicro(),
		nowMicro,
		limit,
		60000, // 1 minute in milliseconds
		member,
	).Int64Slice()

	if err != nil {
		l.logger.Error("rate limit redis error", "key", fullKey, "error", err)
		if l.failOpen {
			return Result{Allowed: true, Limit: limit, Remaining: 0, ResetAt: now.Add(time.Minute)}, nil
		}
		return Result{Allowed: false, Limit: limit, Remaining: 0, ResetAt: now.Add(time.Minute)}, err
	}

	count := int(res[0])
	denied := res[1] == 1
	remaining := limit - count
	if remaining < 0 {
		remaining = 0
	}

	return Result{
		Allowed:   !denied,
		Limit:     limit,
		Remaining: remaining,
		ResetAt:   now.Add(time.Minute),
	}, nil
}

// Ping checks Redis connectivity for health checks.
func (l *Limiter) Ping(ctx context.Context) error {
	return l.client.Ping(ctx).Err()
}

// Close shuts down the Redis connection.
func (l *Limiter) Close() error {
	return l.client.Close()
}
