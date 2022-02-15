package caches

import (
	"context"
	"io"

	"github.com/dashjay/baize/pkg/utils/status"

	repb "github.com/bazelbuild/remote-apis/build/bazel/remote/execution/v2"

	"github.com/dashjay/baize/pkg/interfaces"
)

type CacheMode uint32

const (
	// ModeReadThrough means that if we Get a key from ComposedCache and not found in outer(faster) one,
	// then we will get from inner(slower) one, and set the result into outer.
	ModeReadThrough CacheMode = 1 << (iota + 1)

	// ModeWriteThrough means that if we 'Set' a key into ComposedCache, it will be not only 'Set' into inner(slower) one,
	// and also be 'Set' into outer(faster) one.
	ModeWriteThrough
)

// ComposedCache hold two caches and take the outer one as a faster one
// take inner one as slower one.
// - When invoking Get, first get from outer(faster) one, if not found in outer(faster) one,
//   then it will find in inner(slower) one.
// - If it found in outer(faster) one && ModeReadThrough was set, the result from outer(faster) one will
//   be Set into inner(slower) one.
// - If we set a pair of key and value, this key will be set into inner(slower) one first,
//   and then if ModeWriteThrough was set, this key will be set in outer(faster) one.
type ComposedCache struct {
	// outer one is faster one
	outer interfaces.Cache
	// inner one is slower one
	inner interfaces.Cache

	// mode contains ModeReadThrough, ModeWriteThrough
	mode CacheMode
}

func (c *ComposedCache) WithIsolation(ctx context.Context, cacheType interfaces.CacheType, remoteInstanceName string) (interfaces.Cache, error) {
	newInner, err := c.inner.WithIsolation(ctx, cacheType, remoteInstanceName)
	if err != nil {
		return nil, status.WrapError(err, "WithIsolation failed on inner cache")
	}
	newOuter, err := c.outer.WithIsolation(ctx, cacheType, remoteInstanceName)
	if err != nil {
		return nil, status.WrapError(err, "WithIsolation failed on outer cache")
	}
	return &ComposedCache{
		inner: newInner,
		outer: newOuter,
		mode:  c.mode,
	}, nil
}

func (c *ComposedCache) Check(ctx context.Context) error {
	err := c.inner.Check(ctx)
	if err != nil {
		return err
	}
	err = c.outer.Check(ctx)
	if err != nil {
		return err
	}
	return nil
}

// Size get the inner size + outer size
func (c *ComposedCache) Size() int64 {
	return c.inner.Size() + c.outer.Size()
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
	if outerReader, err := c.outer.Reader(ctx, d, offset); err == nil {
		return outerReader, nil
	}

	innerReader, err := c.inner.Reader(ctx, d, offset)
	if err != nil {
		return nil, err
	}

	if c.mode&ModeReadThrough != 0 && offset == 0 {
		if outerWriter, err := c.outer.Writer(ctx, d); err == nil {
			tr := &ReadCloser{
				io.TeeReader(innerReader, outerWriter),
				&MultiCloser{[]io.Closer{innerReader, outerWriter}},
			}
			return tr, nil
		}
	}

	return innerReader, nil
}

func (c *ComposedCache) Writer(ctx context.Context, d *repb.Digest) (io.WriteCloser, error) {
	innerWriter, err := c.inner.Writer(ctx, d)
	if err != nil {
		return nil, err
	}

	if c.mode&ModeWriteThrough != 0 {
		if outerWriter, err := c.outer.Writer(ctx, d); err == nil {
			dw := &doubleWriter{
				inner: innerWriter,
				outer: outerWriter,
				closeFn: func(err error) {
					if err == nil {
						outerWriter.Close()
					}
				},
			}
			return dw, nil
		}
	}

	return innerWriter, nil
}

var _ interfaces.Cache = (*ComposedCache)(nil)

type doubleWriter struct {
	inner   io.WriteCloser
	outer   io.WriteCloser
	closeFn func(err error)
}

func (d *doubleWriter) Write(p []byte) (int, error) {
	n, err := d.inner.Write(p)
	if err != nil {
		d.closeFn(err)
		return n, err
	}
	if n > 0 {
		d.outer.Write(p)
	}
	return n, err
}

func (d *doubleWriter) Close() error {
	err := d.inner.Close()
	d.closeFn(err)
	return err
}

type ReadCloser struct {
	io.Reader
	io.Closer
}
type MultiCloser struct {
	closers []io.Closer
}

func (m *MultiCloser) Close() error {
	for _, c := range m.closers {
		err := c.Close()
		if err != nil {
			return err
		}
	}
	return nil
}
