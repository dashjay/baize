package caches

import (
	"bytes"
	"context"
	"fmt"
	"io"
	"path/filepath"
	"time"

	"github.com/dashjay/baize/pkg/utils/status"

	"github.com/dashjay/baize/pkg/utils"

	repb "github.com/bazelbuild/remote-apis/build/bazel/remote/execution/v2"
	cmap "github.com/orcaman/concurrent-map"

	"github.com/dashjay/baize/pkg/cc"
	"github.com/dashjay/baize/pkg/copy_from_buildbuddy/utils/lru"
	"github.com/dashjay/baize/pkg/interfaces"
)

const (
	// memoryDefaultCutoffSizeBytes set the max size of one object which use memory cache
	// which means that if object size over memoryDefaultCutoffSizeBytes, it can not be set to memory cache
	memoryDefaultCutoffSizeBytes = 200
)

type MemoryCache struct {
	l                  interfaces.LRU
	c                  cmap.ConcurrentMap
	unitSizeLimitation int
	instanceName       string
	cacheType          interfaces.CacheType
}

func (m *MemoryCache) WithIsolation(ctx context.Context, cacheType interfaces.CacheType, remoteInstanceName string) (interfaces.Cache, error) {
	return &MemoryCache{l: m.l, c: m.c, unitSizeLimitation: m.unitSizeLimitation, instanceName: remoteInstanceName, cacheType: cacheType}, nil
}

func (m *MemoryCache) Check(ctx context.Context) error {
	b := utils.RandomBytes(memoryDefaultCutoffSizeBytes)
	sub, cancel := context.WithTimeout(ctx, 300*time.Millisecond)
	defer cancel()
	err := m.Set(sub, utils.CalSHA256OfInput(b), b)
	if err != nil {
		return err
	}
	return nil
}

func (m *MemoryCache) key(d *repb.Digest) (string, error) {
	if !isDigestValid(d) {
		return "", fmt.Errorf("invalid digest %s", d.GetHash())
	}
	var key string
	if m.cacheType == interfaces.ActionCacheType {
		key = filepath.Join(m.cacheType.Prefix(), m.instanceName, d.GetHash())
	} else {
		key = filepath.Join(m.cacheType.Prefix(), d.GetHash())
	}
	return key, nil
}

func (m *MemoryCache) Size() int64 {
	return m.l.Size()
}

type MapEntry struct {
	Key  string
	Size int64
}

func NewMemoryCache(cfg *cc.Cache) interfaces.Cache {
	c := cmap.New()
	l, err := lru.NewLRU(&lru.Config{
		MaxSize: cfg.CacheSize,
		OnEvict: func(value interface{}) {
			c.Remove(value.(*MapEntry).Key)
		},
		SizeFn: func(value interface{}) int64 {
			return value.(*MapEntry).Size
		},
	})
	if err != nil {
		panic(err)
	}
	usl := cfg.UnitSizeLimitation
	if usl <= 0 {
		usl = memoryDefaultCutoffSizeBytes
	}
	return &MemoryCache{
		l:                  l,
		c:                  c,
		unitSizeLimitation: usl,
	}
}

func (m *MemoryCache) Contains(ctx context.Context, d *repb.Digest) (bool, error) {
	key, err := m.key(d)
	if err != nil {
		return false, err
	}
	exists := m.l.Contains(key)
	return exists, nil
}

func (m *MemoryCache) FindMissing(ctx context.Context, digests []*repb.Digest) ([]*repb.Digest, error) {
	var out []*repb.Digest
	for i := range digests {
		if exists, err := m.Contains(ctx, digests[i]); err == nil && !exists {
			out = append(out, digests[i])
		}
	}
	return out, nil
}
func (m *MemoryCache) Get(ctx context.Context, d *repb.Digest) ([]byte, error) {
	key, err := m.key(d)
	if err != nil {
		return nil, err
	}
	if m.l.Contains(key) {
		if v, exists := m.c.Get(key); exists {
			if val, ok := v.([]byte); ok {
				return val, nil
			}
			// assert error
		}
		// not found in concurrentMap
	}

	m.c.Remove(key)
	m.l.Remove(key)
	return nil, status.NotFoundErrorf("key %s not exists", key)
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
	key, err := m.key(d)
	if err != nil {
		return err
	}
	if len(data) > m.unitSizeLimitation {
		return errByteSizeOverCutoffSize
	}
	m.c.Set(key, data)
	m.l.Add(key, &MapEntry{Key: key, Size: int64(len(data))})
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
	key, err := m.key(d)
	if err != nil {
		return err
	}
	m.l.Remove(key)
	m.c.Remove(key)
	return nil
}

func (m *MemoryCache) Reader(ctx context.Context, d *repb.Digest, offset int64) (io.ReadCloser, error) {
	buf, err := m.Get(ctx, d)
	if err != nil {
		return nil, err
	}
	r := bytes.NewReader(buf)
	r.Seek(offset, 0)
	length := int64(len(buf))
	if length > 0 {
		return io.NopCloser(io.LimitReader(r, length)), nil
	}
	return io.NopCloser(r), nil
}

func (m *MemoryCache) Writer(ctx context.Context, d *repb.Digest) (io.WriteCloser, error) {
	_, err := m.key(d)
	if err != nil {
		return nil, err
	}
	var buffer bytes.Buffer
	return &setOnClose{
		Buffer: &buffer,
		c: func(b *bytes.Buffer) error {
			if b.Len() > m.unitSizeLimitation {
				return errByteSizeOverCutoffSize
			}
			return m.Set(ctx, d, b.Bytes())
		},
	}, nil
}

var _ interfaces.Cache = (*MemoryCache)(nil)

type closeFn func(b *bytes.Buffer) error

type setOnClose struct {
	*bytes.Buffer
	c closeFn
}

func (d *setOnClose) Close() error {
	return d.c(d.Buffer)
}
