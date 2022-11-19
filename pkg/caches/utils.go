package caches

import (
	repb "github.com/bazelbuild/remote-apis/build/bazel/remote/execution/v2"

	"github.com/dashjay/baize/pkg/cc"
	"github.com/dashjay/baize/pkg/interfaces"
)

func isDigestValid(digest *repb.Digest) bool {
	return digest != nil && len(digest.GetHash()) == 64
}

func GenerateCacheFromConfig(cacheCfg *cc.CacheConfig) interfaces.Cache {
	var out interfaces.Cache
	updateCache := func(cache interfaces.Cache, mode CacheMode) {
		if out == nil {
			out = cache
		} else {
			out = NewComposedCache(cache, out, mode)
		}
	}
	if cacheCfg.InmemoryCache.Enabled {
		updateCache(NewMemoryCache(cacheCfg.RedisCache), ModeReadThrough|ModeWriteThrough)
	}
	if cacheCfg.RedisCache.Enabled {
		updateCache(NewRedisCache(cacheCfg.RedisCache), ModeReadThrough|ModeWriteThrough)
	}
	if cacheCfg.DiskCache.Enabled {
		updateCache(NewDiskCache(cacheCfg.DiskCache), 0)
	}
	return out
}
