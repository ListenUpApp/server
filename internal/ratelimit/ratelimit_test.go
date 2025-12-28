package ratelimit

import (
	"context"
	"testing"
	"time"
)

func TestKeyedRateLimiter_Allow(t *testing.T) {
	tests := []struct {
		name     string
		rps      float64
		burst    int
		key      string
		calls    int
		wantPass int
	}{
		{
			name:     "burst allows initial requests",
			rps:      1,
			burst:    3,
			key:      "test",
			calls:    3,
			wantPass: 3,
		},
		{
			name:     "exceeding burst blocks",
			rps:      1,
			burst:    2,
			key:      "test",
			calls:    5,
			wantPass: 2,
		},
		{
			name:     "different keys are independent",
			rps:      1,
			burst:    1,
			key:      "key1",
			calls:    1,
			wantPass: 1,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			rl := New(tt.rps, tt.burst)
			defer rl.Stop()

			passed := 0
			for i := 0; i < tt.calls; i++ {
				if rl.Allow(tt.key) {
					passed++
				}
			}

			if passed != tt.wantPass {
				t.Errorf("Allow() passed %d, want %d", passed, tt.wantPass)
			}
		})
	}
}

func TestKeyedRateLimiter_Wait(t *testing.T) {
	rl := New(10, 1) // 10 rps, burst of 1
	defer rl.Stop()

	ctx, cancel := context.WithTimeout(context.Background(), 500*time.Millisecond)
	defer cancel()

	// First call should succeed immediately
	start := time.Now()
	err := rl.Wait(ctx, "test")
	if err != nil {
		t.Errorf("first Wait() failed: %v", err)
	}
	if time.Since(start) > 50*time.Millisecond {
		t.Error("first Wait() should be immediate")
	}

	// Second call should wait ~100ms (1/10 rps)
	start = time.Now()
	err = rl.Wait(ctx, "test")
	if err != nil {
		t.Errorf("second Wait() failed: %v", err)
	}
	elapsed := time.Since(start)
	if elapsed < 80*time.Millisecond || elapsed > 200*time.Millisecond {
		t.Errorf("second Wait() took %v, want ~100ms", elapsed)
	}
}

func TestKeyedRateLimiter_WaitContextCancelled(t *testing.T) {
	rl := New(0.1, 1) // Very slow: 1 request per 10 seconds
	defer rl.Stop()

	// Exhaust the burst
	rl.Allow("test")

	// Try to wait with short timeout
	ctx, cancel := context.WithTimeout(context.Background(), 50*time.Millisecond)
	defer cancel()

	err := rl.Wait(ctx, "test")
	if err == nil {
		t.Error("Wait() should fail when context canceled")
	}
}

func TestKeyedRateLimiter_IndependentKeys(t *testing.T) {
	rl := New(1, 1)
	defer rl.Stop()

	// Exhaust key1
	rl.Allow("key1")
	if rl.Allow("key1") {
		t.Error("key1 should be exhausted")
	}

	// key2 should still work
	if !rl.Allow("key2") {
		t.Error("key2 should be independent and allowed")
	}
}
