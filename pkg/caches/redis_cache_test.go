package caches

import (
	"github.com/dashjay/bazel-remote-exec/pkg/config"
	"testing"
)

func TestRedisCache(t *testing.T) {
	m := NewRedisCache(&config.Cache{
		CacheAddr: ":6379",
		CacheSize: 1024 * 1024 * 1024,
	})
	TestCache(m, t)
}

func BenchmarkRedisCache(b *testing.B) {
	cfg := &config.Cache{
		CacheAddr: ":6379",
		CacheSize: 1024 * 1024 * 1024,
	}
	m := NewRedisCache(cfg)
	BenchmarkCache(m, b)
}
