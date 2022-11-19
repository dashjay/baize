package caches

import (
	"bytes"
	"context"
	"fmt"
	"io"
	"path/filepath"
	"strconv"
	"strings"
	"time"

	"github.com/dashjay/baize/pkg/utils/status"

	"github.com/dashjay/baize/pkg/utils"

	repb "github.com/bazelbuild/remote-apis/build/bazel/remote/execution/v2"
	"github.com/go-redis/redis/v8"
	"github.com/sirupsen/logrus"

	"github.com/dashjay/baize/pkg/cc"
	"github.com/dashjay/baize/pkg/interfaces"
)

const (
	redisDefaultCutoffSizeBytes = 1024 * 1024 * 10
	defaultTTL                  = time.Hour * 3
)

type RedisCache struct {
	c                  *redis.Client
	maxSizeBytes       int64
	unitSizeLimitation int
	instanceName       string
	cacheType          interfaces.CacheType
}

func (r *RedisCache) WithIsolation(ctx context.Context, cacheType interfaces.CacheType, remoteInstanceName string) (interfaces.Cache, error) {
	return &RedisCache{c: r.c, maxSizeBytes: r.maxSizeBytes, unitSizeLimitation: r.unitSizeLimitation, instanceName: remoteInstanceName, cacheType: cacheType}, nil
}

func (r *RedisCache) Check(ctx context.Context) error {
	if err := r.c.Ping(ctx).Err(); err != nil {
		return err
	}
	b := utils.RandomBytes(redisDefaultCutoffSizeBytes / 2)
	sub, cancel := context.WithTimeout(ctx, 300*time.Millisecond)
	defer cancel()
	err := r.Set(sub, utils.CalSHA256OfInput(b), b)
	if err != nil {
		return err
	}
	return nil
}

func (r *RedisCache) Size() int64 {
	ctx := context.Background()
	output := r.c.Info(ctx, "memory").String()
	const find = "used_memory:"
	i := strings.Index(output, find)
	if i < 0 {
		return -1
	}
	strNum := ""
	for j := i + len(find); j < len(output); j++ {
		if output[j] < '0' || output[j] > '9' {
			break
		}
		strNum = output[i+len(find) : j]
	}
	i, err := strconv.Atoi(strNum)
	if err != nil {
		logrus.Errorf("atoi [%s] error: %s", strNum, err)
		return -1
	}
	return int64(i)
}

func NewRedisCache(cfg *cc.Cache) interfaces.Cache {
	c := redis.NewClient(&redis.Options{Addr: cfg.CacheAddr, DB: 1})
	c.ConfigSet(context.TODO(), "maxmemory", fmt.Sprintf("%d", cfg.CacheSize))
	usl := cfg.UnitSizeLimitation
	if usl <= 0 {
		usl = redisDefaultCutoffSizeBytes
	}
	return &RedisCache{
		c:                  c,
		maxSizeBytes:       cfg.CacheSize,
		unitSizeLimitation: usl,
	}
}

func (r *RedisCache) key(d *repb.Digest) (string, error) {
	if !isDigestValid(d) {
		return "", fmt.Errorf("invalid digest %s", d.GetHash())
	}
	var key string
	if r.cacheType == interfaces.ActionCacheType {
		key = filepath.Join(r.cacheType.Prefix(), r.instanceName, d.GetHash())
	} else {
		key = filepath.Join(r.cacheType.Prefix(), d.GetHash())
	}
	return key, nil
}
func (r *RedisCache) Contains(ctx context.Context, d *repb.Digest) (bool, error) {
	key, err := r.key(d)
	if err != nil {
		return false, err
	}
	res := r.c.Get(ctx, key)
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
	key, err := r.key(d)
	if err != nil {
		return nil, err
	}
	res := r.c.Get(ctx, key)
	if res.Err() != nil {
		return nil, status.NotFoundErrorf("key %s not exists", d.GetHash())
	}
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
	key, err := r.key(d)
	if err != nil {
		return err
	}
	if len(data) > r.unitSizeLimitation {
		return errByteSizeOverCutoffSize
	}
	return r.c.Set(ctx, key, data, defaultTTL).Err()
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
	key, err := r.key(d)
	if err != nil {
		return err
	}
	return r.c.Del(ctx, key).Err()
}

func (r *RedisCache) Reader(ctx context.Context, d *repb.Digest, offset int64) (io.ReadCloser, error) {
	// Locking and key prefixing are handled in Get.
	buf, err := r.Get(ctx, d)
	if err != nil {
		return nil, err
	}
	br := bytes.NewReader(buf)
	br.Seek(offset, 0)
	length := int64(len(buf))
	if length > 0 {
		return io.NopCloser(io.LimitReader(br, length)), nil
	}
	return io.NopCloser(br), nil
}

func (r *RedisCache) Writer(ctx context.Context, d *repb.Digest) (io.WriteCloser, error) {
	_, err := r.key(d)
	if err != nil {
		return nil, err
	}
	var buffer bytes.Buffer
	return &setOnClose{
		Buffer: &buffer,
		c: func(b *bytes.Buffer) error {
			return r.Set(ctx, d, b.Bytes())
		},
	}, nil
}

var _ interfaces.Cache = (*RedisCache)(nil)
