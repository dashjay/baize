package caches

import (
	"context"
	repb "github.com/bazelbuild/remote-apis/build/bazel/remote/execution/v2"
	"github.com/dashjay/bazel-remote-exec/pkg/config"
	"github.com/dashjay/bazel-remote-exec/pkg/interfaces"
	"github.com/dashjay/bazel-remote-exec/pkg/utils/lru"
	cmap "github.com/orcaman/concurrent-map"
	"io"
)

const (
	// memoryDefaultCutoffSizeBytes set the max size of one object which use memory cache
	// which means that if object size over memoryDefaultCutoffSizeBytes, it can not be set to memory cache
	memoryDefaultCutoffSizeBytes = 200
)

type MemoryCache struct {
	l interfaces.LRU
	c cmap.ConcurrentMap
}

type MapEntry struct {
	Key  string
	Size int64
}

func NewMemoryCache(cache *config.Cache) *MemoryCache {
	c := cmap.New()
	l := lru.NewLRU(&lru.Config{
		MaxSize: cache.CacheSize,
		RemoveFn: func(key string, value interface{}) {
			c.Remove(key)
		},
		SizeFn: func(key string, value interface{}) int64 {
			return value.(*MapEntry).Size
		},
		AddFn: nil,
	})
	return &MemoryCache{
		l: l,
		c: c,
	}
}

func (m *MemoryCache) Contains(ctx context.Context, d *repb.Digest) (bool, error) {
	exists := m.l.Contains(d.GetHash())
	return exists, nil
}

func (m *MemoryCache) FindMissing(ctx context.Context, digests []*repb.Digest) ([]*repb.Digest, error) {
	var out []*repb.Digest
	for i := range digests {
		if exists := m.l.Contains(digests[i].GetHash()); !exists {
			out = append(out, digests[i])
		}
	}
	return out, nil
}
func (m *MemoryCache) Get(ctx context.Context, d *repb.Digest) ([]byte, error) {
	if m.l.Contains(d.GetHash()) {
		if v, exists := m.c.Get(d.GetHash()); exists {
			if val, ok := v.([]byte); ok {
				return val, nil
			}
			// assert error
		}
		// not found in concurrentMap
	}

	m.c.Remove(d.GetHash())
	m.l.Remove(d.GetHash())
	return nil, errNotFound{key: d.GetHash()}
}

func (m *MemoryCache) GetMulti(ctx context.Context, digests []*repb.Digest) (map[*repb.Digest][]byte, error) {
	out := make(map[*repb.Digest][]byte, len(digests))
	for i := range digests {
		content, err := m.Get(ctx, digests[i])
		if err != nil {
			return nil, err
		}
		out[digests[i]] = content
	}
	return out, nil
}

func (m *MemoryCache) Set(ctx context.Context, d *repb.Digest, data []byte) error {
	if len(data) > memoryDefaultCutoffSizeBytes {
		return errByteSizeOverCutoffSize
	}
	m.c.Set(d.GetHash(), data)
	m.l.Add(d.GetHash(), &MapEntry{Key: d.GetHash(), Size: int64(len(data))})
	return nil
}

func (m *MemoryCache) SetMulti(ctx context.Context, kvs map[*repb.Digest][]byte) error {
	for k := range kvs {
		err := m.Set(ctx, k, kvs[k])
		if err != nil {
			return err
		}
	}
	return nil
}

func (m *MemoryCache) Delete(ctx context.Context, d *repb.Digest) error {
	m.l.Remove(d.GetHash())
	m.c.Remove(d.GetHash())
	return nil
}

func (m *MemoryCache) Reader(ctx context.Context, d *repb.Digest, offset int64) (io.ReadCloser, error) {
	panic("implement me")
}

func (m *MemoryCache) Writer(ctx context.Context, d *repb.Digest) (io.WriteCloser, error) {
	panic("implement me")
}

var _ interfaces.Cache = (*MemoryCache)(nil)
