package caches

import (
	"github.com/dashjay/bazel-remote-exec/pkg/config"
	"os"
	"testing"
)

func TestComposedWithDiskAndMemory(t *testing.T) {
	tempdir := t.TempDir()
	t.Logf("TempDir: %s", tempdir)
	defer os.Remove(tempdir)
	dc := NewDiskCache(&config.Cache{
		CacheSize: 65535,
		CacheAddr: tempdir,
	})
	mc := NewMemoryCache(&config.Cache{
		CacheSize: 65535,
	})
	cc := NewComposedCache(mc, dc, ModeReadThrough|ModeWriteThrough)
	TestCache(cc, t)
}
