package caches

import (
	"context"
	repb "github.com/bazelbuild/remote-apis/build/bazel/remote/execution/v2"
	"github.com/dashjay/bazel-remote-exec/pkg/interfaces"
	"io"
)

type CacheMode uint32

const (
	ModeReadThrough CacheMode = 1 << (iota + 1)
	ModeWriteThrough
)

type ComposedCache struct {
	inner interfaces.Cache
	outer interfaces.Cache
	mode  CacheMode
}

func NewComposedCache(inner, outer interfaces.Cache, mode CacheMode) interfaces.Cache {
	return &ComposedCache{
		inner: inner,
		outer: outer,
		mode:  mode,
	}
}

func (c *ComposedCache) Contains(ctx context.Context, d *repb.Digest) (bool, error) {
	outerExists, err := c.outer.Contains(ctx, d)
	if err != nil && outerExists {
		return outerExists, nil
	}

	return c.inner.Contains(ctx, d)
}

func (c *ComposedCache) FindMissing(ctx context.Context, digests []*repb.Digest) ([]*repb.Digest, error) {
	missing, err := c.outer.FindMissing(ctx, digests)
	if err != nil {
		missing = digests
	}
	if len(missing) == 0 {
		return nil, nil
	}
	return c.inner.FindMissing(ctx, missing)
}

func (c *ComposedCache) Get(ctx context.Context, d *repb.Digest) ([]byte, error) {
	outerRsp, err := c.outer.Get(ctx, d)
	if err == nil {
		return outerRsp, nil
	}

	innerRsp, err := c.inner.Get(ctx, d)
	if err != nil {
		return nil, err
	}
	if c.mode&ModeReadThrough != 0 {
		c.outer.Set(ctx, d, innerRsp)
	}

	return innerRsp, nil
}

func (c *ComposedCache) GetMulti(ctx context.Context, digests []*repb.Digest) (map[*repb.Digest][]byte, error) {
	foundMap := make(map[*repb.Digest][]byte, len(digests))
	if outerFoundMap, err := c.outer.GetMulti(ctx, digests); err == nil {
		for d, data := range outerFoundMap {
			foundMap[d] = data
		}
	}
	stillMissing := make([]*repb.Digest, 0)
	for _, d := range digests {
		if _, ok := foundMap[d]; !ok {
			stillMissing = append(stillMissing, d)
		}
	}
	if len(stillMissing) == 0 {
		return foundMap, nil
	}

	innerFoundMap, err := c.inner.GetMulti(ctx, stillMissing)
	if err != nil {
		return nil, err
	}
	for d, data := range innerFoundMap {
		foundMap[d] = data
	}
	return foundMap, nil
}

func (c *ComposedCache) Set(ctx context.Context, d *repb.Digest, data []byte) error {
	if err := c.inner.Set(ctx, d, data); err != nil {
		return err
	}
	if c.mode&ModeWriteThrough != 0 {
		c.outer.Set(ctx, d, data)
	}
	return nil
}

func (c *ComposedCache) SetMulti(ctx context.Context, kvs map[*repb.Digest][]byte) error {
	if err := c.inner.SetMulti(ctx, kvs); err != nil {
		return err
	}
	if c.mode&ModeWriteThrough != 0 {
		c.outer.SetMulti(ctx, kvs)
	}
	return nil
}

func (c *ComposedCache) Delete(ctx context.Context, d *repb.Digest) error {
	if err := c.inner.Delete(ctx, d); err != nil {
		return err
	}
	if c.mode&ModeWriteThrough != 0 {
		c.outer.Delete(ctx, d)
	}
	return nil
}

func (c *ComposedCache) Reader(ctx context.Context, d *repb.Digest, offset int64) (io.ReadCloser, error) {
	panic("implement me")
}

func (c *ComposedCache) Writer(ctx context.Context, d *repb.Digest) (io.WriteCloser, error) {
	panic("implement me")
}

var _ interfaces.Cache = (*ComposedCache)(nil)
