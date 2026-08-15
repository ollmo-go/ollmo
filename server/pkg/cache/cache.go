// Package cache provides a minimal per-process TTL cache. It exists to keep
// hot-path reads (KB row, default LLM config) from hitting MySQL on every
// chat turn; writers invalidate explicitly, so entries are advisory only.
package cache

import (
	"sync"
	"time"
)

type entry[T any] struct {
	val     T
	expires time.Time
}

// TTL is a concurrency-safe string-keyed cache whose entries expire after a
// fixed TTL. Zero value is not usable; construct with NewTTL.
type TTL[T any] struct {
	mu  sync.RWMutex
	ttl time.Duration
	m   map[string]entry[T]
}

func NewTTL[T any](ttl time.Duration) *TTL[T] {
	return &TTL[T]{ttl: ttl, m: make(map[string]entry[T])}
}

func (c *TTL[T]) Get(key string) (T, bool) {
	c.mu.RLock()
	e, ok := c.m[key]
	c.mu.RUnlock()
	var zero T
	if !ok || time.Now().After(e.expires) {
		return zero, false
	}
	return e.val, true
}

func (c *TTL[T]) Set(key string, v T) {
	c.mu.Lock()
	c.m[key] = entry[T]{val: v, expires: time.Now().Add(c.ttl)}
	c.mu.Unlock()
}

func (c *TTL[T]) Delete(key string) {
	c.mu.Lock()
	delete(c.m, key)
	c.mu.Unlock()
}
