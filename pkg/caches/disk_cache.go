package caches

import (
	"bytes"
	"compress/gzip"
	"context"
	"fmt"
	repb "github.com/bazelbuild/remote-apis/build/bazel/remote/execution/v2"
	"github.com/dashjay/bazel-remote-exec/pkg/interfaces"
	"github.com/dashjay/bazel-remote-exec/pkg/utils"
	"github.com/dashjay/bazel-remote-exec/pkg/utils/lru"
	"github.com/sirupsen/logrus"
	"io"
	"io/ioutil"
	"os"
	"path/filepath"
	"time"
)

const (
	HashPrefixDirPrefixLen = 4
)

type DiskCache struct {
	rootDir string
	//prefixMutex                 map[string]*sync.Mutex
	lru                         interfaces.LRU
	maxSizeBytes                int64
	finishLoadingFromFileSystem chan struct{}
}
type fileRecord struct {
	lastUseTime int64
	digest      *repb.Digest
	sizeBytes   int64
}

func NewDiskCache(rootDir string, maxSizeBytes int64) *DiskCache {
	if rootDir == "" {
		logrus.Panic("empty rootDir")
	}
	if maxSizeBytes < 0 {
		logrus.Panic("minus maxByteSize")
	}

	d := &DiskCache{
		rootDir: rootDir,
		//prefixMutex:                 make(map[string]*sync.Mutex),
		maxSizeBytes:                maxSizeBytes,
		finishLoadingFromFileSystem: make(chan struct{}),
	}

	l := lru.NewLRU(&lru.Config{
		MaxSize:  maxSizeBytes,
		RemoveFn: d.OnRemove,
		SizeFn:   d.SizeFn,
		AddFn:    nil,
	})
	d.lru = l
	return d
}
func (c *DiskCache) OnRemove(key string, value interface{}) {
	if v, ok := value.(*fileRecord); ok {
		fullPath := filepath.Join(c.rootDir, v.digest.Hash[0:HashPrefixDirPrefixLen], v.digest.Hash[HashPrefixDirPrefixLen:])
		_, err := os.Stat(fullPath)
		if err != nil {
			logrus.Errorf("try to remove file %s error: %s", fullPath, err)
			return
		}
		os.Remove(fullPath)
	}
}

func (c *DiskCache) SizeFn(key string, value interface{}) int64 {
	if v, ok := value.(*fileRecord); ok {
		return v.digest.SizeBytes
	}
	return 0
}

// Contains only call lru
func (c *DiskCache) Contains(ctx context.Context, d *repb.Digest) (bool, error) {
	return c.lru.Contains(d.Hash), nil
}

func (c *DiskCache) FindMissing(ctx context.Context, digests []*repb.Digest) ([]*repb.Digest, error) {
	var out []*repb.Digest
	for i := range digests {
		if !c.lru.Contains(digests[i].Hash) {
			out = append(out, digests[i])
		}
	}
	return out, nil
}

func (c *DiskCache) Get(ctx context.Context, d *repb.Digest) ([]byte, error) {
	v, exists := c.lru.Get(d.Hash)
	if !exists {
		return nil, os.ErrNotExist
	}
	ent, ok := v.(*lru.Entry)
	if !ok || ent == nil {
		return nil, os.ErrNotExist
	}
	digest := ent.Value.(*fileRecord).digest
	fullPath := filepath.Join(c.rootDir, digest.Hash[:HashPrefixDirPrefixLen], digest.Hash[HashPrefixDirPrefixLen:])
	content, err := c.readFileFromDisk(ctx, fullPath)
	if err != nil {
		c.lru.Remove(d.Hash)
		return nil, err
	}
	return content, nil
}

func (c *DiskCache) readFileFromDisk(ctx context.Context, fullPath string) ([]byte, error) {
	logrus.Tracef("readFileFromDist, path: %s", fullPath)
	fd, err := os.Open(fullPath)
	if err != nil {
		return nil, err
	}
	defer fd.Close()
	zr, err := gzip.NewReader(fd)
	if err != nil {
		if err == gzip.ErrHeader {
			return ioutil.ReadAll(fd)
		}
		return nil, err
	}
	output, err := ioutil.ReadAll(zr)
	if err != nil {
		return nil, err
	}
	if err = zr.Close(); err != nil {
		return nil, err
	}
	return output, nil
}

func (c *DiskCache) GetMulti(ctx context.Context, digests []*repb.Digest) (map[*repb.Digest][]byte, error) {
	var out = make(map[*repb.Digest][]byte, len(digests))
	for i := range digests {
		output, err := c.Get(ctx, digests[i])
		if err != nil {
			return nil, err
		}
		out[digests[i]] = output
	}
	return out, nil
}

func (c *DiskCache) Set(ctx context.Context, d *repb.Digest, data []byte) error {
	fullPath := filepath.Join(c.rootDir, d.Hash[:HashPrefixDirPrefixLen], d.Hash[HashPrefixDirPrefixLen:])
	_, exists := c.lru.Get(d.Hash)
	if exists {
		return nil
	}
	var buf bytes.Buffer
	wr := gzip.NewWriter(&buf)
	_, err := wr.Write(data)
	if err != nil {
		return err
	}
	err = wr.Close()
	if err != nil {
		return err
	}
	siz, err := WriteFile(ctx, fullPath, buf.Bytes())
	if err != nil {
		return err
	}
	fr := &fileRecord{
		lastUseTime: time.Now().Unix(),
		digest:      d,
		sizeBytes:   siz,
	}
	if !c.lru.Add(d.Hash, fr) {
		return fmt.Errorf("add %s error", d.Hash)
	}
	return nil
}
func WriteFile(ctx context.Context, fullPath string, data []byte) (int64, error) {
	randStr := utils.RandomString(10)

	tmpFileName := fmt.Sprintf("%s.%s.tmp", fullPath, randStr)
	err := os.MkdirAll(filepath.Dir(fullPath), 0644)
	if err != nil {
		return 0, err
	}

	defer deleteLocalFileIfExists(tmpFileName)

	if err := ioutil.WriteFile(tmpFileName, data, 0644); err != nil {
		return 0, err
	}
	return int64(len(data)), os.Rename(tmpFileName, fullPath)
}

func deleteLocalFileIfExists(filename string) {
	_, err := os.Stat(filename)
	if err == nil {
		if err := os.Remove(filename); err != nil {
			logrus.Warningf("Error deleting file %q: %s", filename, err)
		}
	}
}

func (c *DiskCache) SetMulti(ctx context.Context, kvs map[*repb.Digest][]byte) error {
	for k := range kvs {
		err := c.Set(ctx, k, kvs[k])
		if err != nil {
			return err
		}
	}
	return nil
}

func (c *DiskCache) Delete(ctx context.Context, d *repb.Digest) error {
	if c.lru.Remove(d.Hash) {
		return fmt.Errorf("remove %s success", d)
	}
	return nil
}

func (c *DiskCache) Reader(ctx context.Context, d *repb.Digest, offset int64) (io.ReadCloser, error) {
	panic("implement me")
}

func (c *DiskCache) Writer(ctx context.Context, d *repb.Digest) (io.WriteCloser, error) {
	panic("implement me")
}

var _ interfaces.Cache = (*DiskCache)(nil)
