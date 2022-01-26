package caches

import (
	"context"
	"fmt"
	repb "github.com/bazelbuild/remote-apis/build/bazel/remote/execution/v2"
	"github.com/dashjay/bazel-remote-exec/pkg/config"
	"github.com/dashjay/bazel-remote-exec/pkg/interfaces"
	"github.com/go-redis/redis/v8"
	"io"
	"time"
)

const (
	redisDefaultCutoffSizeBytes = 10000000
	defaultTTL                  = time.Hour * 3
)

type RedisCache struct {
	c            *redis.Client
	maxSizeBytes int64
}

func NewRedisCache(cfg *config.Cache) interfaces.Cache {
	c := redis.NewClient(&redis.Options{Addr: cfg.CacheAddr, DB: 1})
	c.ConfigSet(context.TODO(), "maxmemory", fmt.Sprintf("%d", cfg.CacheSize))
	return &RedisCache{
		c:            c,
		maxSizeBytes: cfg.CacheSize,
	}
}

func (r *RedisCache) Contains(ctx context.Context, d *repb.Digest) (bool, error) {
	res := r.c.Get(ctx, d.GetHash())
	if res.Err() == redis.Nil {
		return false, nil
	}
	return false, res.Err()
}

func (r *RedisCache) FindMissing(ctx context.Context, digests []*repb.Digest) ([]*repb.Digest, error) {
	var out []*repb.Digest
	for i := range digests {
		if v, err := r.Contains(ctx, digests[i]); err != nil {
			return nil, err
		} else if !v {
			out = append(out, digests[i])
		}
	}
	return out, nil
}

func (r *RedisCache) Get(ctx context.Context, d *repb.Digest) ([]byte, error) {
	res := r.c.Get(ctx, d.GetHash())
	return res.Bytes()
}

func (r *RedisCache) GetMulti(ctx context.Context, digests []*repb.Digest) (map[*repb.Digest][]byte, error) {
	var err error
	out := make(map[*repb.Digest][]byte, len(digests))
	for i := range digests {
		out[digests[i]], err = r.Get(ctx, digests[i])
		if err != nil {
			return nil, err
		}
	}
	return out, nil
}

func (r *RedisCache) Set(ctx context.Context, d *repb.Digest, data []byte) error {
	if len(data) > redisDefaultCutoffSizeBytes {
		return errByteSizeOverCutoffSize
	}
	return r.c.Set(ctx, d.GetHash(), data, defaultTTL).Err()
}

func (r *RedisCache) SetMulti(ctx context.Context, kvs map[*repb.Digest][]byte) error {
	for k := range kvs {
		err := r.Set(ctx, k, kvs[k])
		if err != nil {
			return err
		}
	}
	return nil
}

func (r *RedisCache) Delete(ctx context.Context, d *repb.Digest) error {
	return r.c.Del(ctx, d.GetHash()).Err()
}

func (r *RedisCache) Reader(ctx context.Context, d *repb.Digest, offset int64) (io.ReadCloser, error) {
	panic("implement me")
}

func (r *RedisCache) Writer(ctx context.Context, d *repb.Digest) (io.WriteCloser, error) {
	panic("implement me")
}

var _ interfaces.Cache = (*RedisCache)(nil)
