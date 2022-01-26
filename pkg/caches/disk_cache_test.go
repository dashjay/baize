package caches

import (
	"github.com/dashjay/bazel-remote-exec/pkg/config"
	"os"
	"testing"
)

func TestDiskCache(t *testing.T) {
	tempdir := t.TempDir()
	t.Logf("TempDir: %s", tempdir)
	defer os.Remove(tempdir)
	dc := NewDiskCache(&config.Cache{
		Enabled:   true,
		CacheSize: 65535,
		CacheAddr: tempdir,
	})
	TestCache(dc, t)
}

func BenchmarkDiskCache(b *testing.B) {
	tempdir := b.TempDir()
	b.Logf("TempDir: %s", tempdir)
	defer os.Remove(tempdir)

	dc := NewDiskCache(&config.Cache{
		Enabled:   true,
		CacheSize: 65535,
		CacheAddr: tempdir,
	})
	BenchmarkCache(dc, b)
	b.Logf("Current Size: %d", dc.lru.Size())
}
