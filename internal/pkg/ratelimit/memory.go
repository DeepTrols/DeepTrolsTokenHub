package ratelimit

import (
	"context"
	"sync"
	"time"
)

// cleanupInterval is how often the background sweeper removes expired entries,
// bounding memory under bursty/fabricated traffic from many distinct keys.
const cleanupInterval = 30 * time.Second

type memoryEntry struct {
	count    int
	resetsAt time.Time
}

// MemoryRateLimiter is an in-memory fixed-window limiter, guarded by a mutex.
// A background sweeper periodically removes expired entries so the map does
// not grow without bound under a flood of one-shot keys. Close stops the
// sweeper.
type MemoryRateLimiter struct {
	mu      sync.Mutex
	entries map[string]*memoryEntry
	done    chan struct{}
}

// NewMemoryRateLimiter returns a ready-to-use in-memory limiter.
func NewMemoryRateLimiter() *MemoryRateLimiter {
	m := &MemoryRateLimiter{
		entries: make(map[string]*memoryEntry),
		done:    make(chan struct{}),
	}
	go m.sweepLoop()
	return m
}

// sweepLoop periodically purges expired entries until Close is called.
func (m *MemoryRateLimiter) sweepLoop() {
	ticker := time.NewTicker(cleanupInterval)
	defer ticker.Stop()
	for {
		select {
		case <-ticker.C:
			m.sweep()
		case <-m.done:
			return
		}
	}
}

// sweep deletes all entries whose window has expired.
func (m *MemoryRateLimiter) sweep() {
	now := time.Now()
	m.mu.Lock()
	defer m.mu.Unlock()
	for k, e := range m.entries {
		if now.After(e.resetsAt) {
			delete(m.entries, k)
		}
	}
}

// Allow grants the request if `key` is within `limit` calls for `window`.
func (m *MemoryRateLimiter) Allow(_ context.Context, key string, limit int, window time.Duration) (bool, time.Duration, error) {
	if limit <= 0 || window <= 0 {
		return false, 0, nil
	}

	now := time.Now()
	m.mu.Lock()
	defer m.mu.Unlock()

	entry, ok := m.entries[key]
	if !ok || now.After(entry.resetsAt) {
		// First request in a window (or a fresh window): count starts at 1.
		m.entries[key] = &memoryEntry{count: 1, resetsAt: now.Add(window)}
		return true, 0, nil
	}

	entry.count++
	if entry.count > limit {
		retryAfter := time.Until(entry.resetsAt)
		if retryAfter < 0 {
			retryAfter = 0
		}
		return false, retryAfter, nil
	}
	return true, 0, nil
}

// Close stops the background sweeper and clears all tracked entries.
func (m *MemoryRateLimiter) Close() {
	close(m.done)
	m.mu.Lock()
	defer m.mu.Unlock()
	m.entries = make(map[string]*memoryEntry)
}
