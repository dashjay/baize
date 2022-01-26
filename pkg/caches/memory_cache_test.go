package caches

import (
	"github.com/dashjay/bazel-remote-exec/pkg/config"
	"testing"
)

func TestMemoryCache(t *testing.T) {
	m := NewMemoryCache(&config.Cache{
		CacheSize: 1024 * 1024 * 1024,
	})
	TestCache(m, t)
}

func BenchmarkMemoryCache(b *testing.B) {
	cfg := &config.Cache{
		CacheSize: 1024 * 1024,
	}
	m := NewMemoryCache(cfg)
	BenchmarkCache(m, b)
	b.Logf("max size %d, current size %d", cfg.CacheSize, m.l.Size())
}
