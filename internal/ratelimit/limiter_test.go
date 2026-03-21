// Copyright (c) 2026 Opportunation / GoGate Authors
// SPDX-License-Identifier: AGPL-3.0-or-later

package ratelimit

import (
	"context"
	"io"
	"log/slog"
	"testing"
	"time"

	"github.com/alicebob/miniredis/v2"

	"github.com/gogatehq/gogate/internal/config"
)

func testLogger() *slog.Logger {
	return slog.New(slog.NewTextHandler(io.Discard, nil))
}

func mustStartLimiter(t *testing.T, mr *miniredis.Miniredis) *Limiter {
	t.Helper()
	limiter, err := NewLimiter(
		config.RedisConfig{
			Addr:         mr.Addr(),
			DialTimeout:  2 * time.Second,
			ReadTimeout:  2 * time.Second,
			WriteTimeout: 2 * time.Second,
		},
		config.RateLimitConfig{
			DefaultRPM: 5,
			KeyPrefix:  "test:rl:",
		},
		testLogger(),
	)
	if err != nil {
		t.Fatalf("new limiter: %v", err)
	}
	t.Cleanup(func() { limiter.Close() })
	return limiter
}

func TestAllowWithinLimit(t *testing.T) {
	t.Parallel()
	mr := miniredis.RunT(t)
	limiter := mustStartLimiter(t, mr)

	for i := 0; i < 5; i++ {
		res, err := limiter.Allow(context.Background(), "client1:/api", 5)
		if err != nil {
			t.Fatalf("allow: %v", err)
		}
		if !res.Allowed {
			t.Fatalf("request %d should be allowed", i+1)
		}
		if res.Remaining != 5-i-1 {
			t.Fatalf("expected remaining %d, got %d", 5-i-1, res.Remaining)
		}
	}
}

func TestDenyOverLimit(t *testing.T) {
	t.Parallel()
	mr := miniredis.RunT(t)
	limiter := mustStartLimiter(t, mr)

	for i := 0; i < 5; i++ {
		_, _ = limiter.Allow(context.Background(), "client2:/api", 5)
	}

	res, err := limiter.Allow(context.Background(), "client2:/api", 5)
	if err != nil {
		t.Fatalf("allow: %v", err)
	}
	if res.Allowed {
		t.Fatal("6th request should be denied")
	}
	if res.Remaining != 0 {
		t.Fatalf("expected remaining 0, got %d", res.Remaining)
	}
}

func TestSlidingWindowExpiry(t *testing.T) {
	t.Parallel()
	mr := miniredis.RunT(t)
	limiter := mustStartLimiter(t, mr)

	for i := 0; i < 5; i++ {
		_, _ = limiter.Allow(context.Background(), "client3:/api", 5)
	}

	// Fast-forward past the 1-minute window.
	mr.FastForward(61 * time.Second)

	res, err := limiter.Allow(context.Background(), "client3:/api", 5)
	if err != nil {
		t.Fatalf("allow: %v", err)
	}
	if !res.Allowed {
		t.Fatal("request should be allowed after window expiry")
	}
}

func TestKeyIsolation(t *testing.T) {
	t.Parallel()
	mr := miniredis.RunT(t)
	limiter := mustStartLimiter(t, mr)

	for i := 0; i < 5; i++ {
		_, _ = limiter.Allow(context.Background(), "clientA:/api", 5)
	}

	// Different key should have its own counter.
	res, err := limiter.Allow(context.Background(), "clientB:/api", 5)
	if err != nil {
		t.Fatalf("allow: %v", err)
	}
	if !res.Allowed {
		t.Fatal("different key should not be rate limited")
	}
}

func TestFailOpenOnRedisError(t *testing.T) {
	t.Parallel()
	mr := miniredis.RunT(t)
	limiter := mustStartLimiter(t, mr)

	// Shut down Redis.
	mr.Close()

	res, err := limiter.Allow(context.Background(), "client4:/api", 5)
	// fail_open defaults to true, so should allow and return nil error.
	if err != nil {
		t.Fatalf("expected nil error in fail-open mode, got: %v", err)
	}
	if !res.Allowed {
		t.Fatal("fail-open should allow request when Redis is down")
	}
}

func TestZeroLimitAlwaysAllows(t *testing.T) {
	t.Parallel()
	mr := miniredis.RunT(t)
	limiter := mustStartLimiter(t, mr)

	res, err := limiter.Allow(context.Background(), "client5:/api", 0)
	if err != nil {
		t.Fatalf("allow: %v", err)
	}
	if !res.Allowed {
		t.Fatal("zero limit should always allow")
	}
}
