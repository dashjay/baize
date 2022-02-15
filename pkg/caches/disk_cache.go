package caches

import (
	"context"
	"fmt"
	"github.com/dashjay/baize/pkg/interfaces"
	"io"
	"io/fs"
	"io/ioutil"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/dashjay/baize/pkg/utils/status"

	"github.com/dashjay/baize/pkg/utils"

	repb "github.com/bazelbuild/remote-apis/build/bazel/remote/execution/v2"
	"github.com/sirupsen/logrus"

	"github.com/dashjay/baize/pkg/config"

	"github.com/dashjay/baize/pkg/copy_from_buildbuddy/utils/lru"
	"github.com/dashjay/baize/pkg/copy_from_buildbuddy/utils/disk"
)

const (
	// HashPrefixDirPrefixLen is length heads hash prefix
	// get the first 4 character of hash string, then save
	// the content to rootDir/hash{0:4}/hash
	HashPrefixDirPrefixLen = 4

	diskDefaultCutoffSizeBytes = 104857600
)

// DiskCache implements the disk-based interfaces.Cache
type DiskCache struct {
	// rootDir is where to save files to
	rootDir string

	lru interfaces.LRU
	// maxSizeBytes is the max disk usage of this instance used.
	maxSizeBytes                int64
	metrics                     *Metrics
	finishLoadingFromFileSystem chan struct{}
	unitSizeLimitation          int
	instanceName                string
	cacheType                   interfaces.CacheType
}

func (c *DiskCache) WithIsolation(ctx context.Context, cacheType interfaces.CacheType, remoteInstanceName string) (interfaces.Cache, error) {
	return &DiskCache{
		rootDir:            c.rootDir,
		lru:                c.lru,
		maxSizeBytes:       c.maxSizeBytes,
		unitSizeLimitation: c.unitSizeLimitation,
		instanceName:       remoteInstanceName,
		cacheType:          cacheType,
		metrics:            c.metrics,
	}, nil
}

func (c *DiskCache) Check(ctx context.Context) error {
	b := utils.RandomBytes(4000)
	sub, cancel := context.WithTimeout(ctx, 300*time.Millisecond)
	defer cancel()
	err := c.Set(sub, utils.CalSHA256OfInput(b), b)
	if err != nil {
		return err
	}
	return nil
}

func (c *DiskCache) Size() int64 {
	return c.lru.Size()
}

type fileRecord struct {
	lastUseTime int64
	key         string
	sizeBytes   int64
}

func (c *DiskCache) key(d *repb.Digest) (string, error) {
	if !isDigestValid(d) {
		return "", status.InvalidArgumentErrorf("invalid digest %s", d.GetHash())
	}
	hash := d.GetHash()
	if len(hash) < HashPrefixDirPrefixLen {
		return "", status.FailedPreconditionErrorf("digest hash %q is way too short!", hash)
	}

	var key string
	if c.cacheType == interfaces.ActionCacheType {
		key = filepath.Join(c.cacheType.Prefix(), c.instanceName, hash[:HashPrefixDirPrefixLen], hash)
	} else {
		key = filepath.Join(c.cacheType.Prefix(), hash[:HashPrefixDirPrefixLen], hash)
	}
	return key, nil
}

func NewDiskCache(cfg *config.Cache) interfaces.Cache {
	if cfg.CacheAddr == "" {
		logrus.Panic("empty rootDir")
	}
	if cfg.CacheSize < 0 {
		logrus.Panic("minus maxByteSize")
	}

	usl := cfg.UnitSizeLimitation
	if usl == 0 {
		usl = diskDefaultCutoffSizeBytes
	}
	d := &DiskCache{
		rootDir:                     cfg.CacheAddr,
		maxSizeBytes:                cfg.CacheSize,
		metrics:                     &Metrics{},
		finishLoadingFromFileSystem: make(chan struct{}),
		unitSizeLimitation:          usl,
	}

	l, err := lru.NewLRU(&lru.Config{
		MaxSize: cfg.CacheSize,
		OnEvict: d.onRemove,
		SizeFn:  d.sizeFn,
	})
	if err != nil {
		logrus.Panic(err)
	}
	d.lru = l
	go d.loadingFromFileSystem()
	<-d.finishLoadingFromFileSystem
	go func() {
		t := time.NewTicker(time.Minute)
		for range t.C {
			logrus.Infof("Disk Cache Metrics [Hit: %d, Miss: %d, Total: %d, Hit rate: %.2f%%]\n", d.metrics.GetHit(), d.metrics.GetMiss(), d.metrics.GetTotal(), d.metrics.GetHitRate())
		}
	}()
	return d
}

// loadingFromFileSystem load all files from rootDir
// and rebuild the lru instance.
func (c *DiskCache) loadingFromFileSystem() {
	logrus.Infof("Rebuild index of rootDir %s", c.rootDir)
	count := 0
	err := filepath.WalkDir(c.rootDir, func(path string, info fs.DirEntry, err error) error {
		if err != nil {
			return err
		}
		if info == nil {
			return nil
		}
		if info.IsDir() {
			return nil
		}
		fi, err := info.Info()
		if err != nil {
			return err
		}
		key := strings.TrimLeft(path, c.rootDir)
		if !c.lru.Add(key, &fileRecord{
			lastUseTime: fi.ModTime().Unix(),
			key:         key,
			sizeBytes:   fi.Size(),
		}) {
			return fmt.Errorf("add %s error", key)
		}
		count++
		return nil
	})
	if err != nil {
		panic(err)
	}
	c.finishLoadingFromFileSystem <- struct{}{}
	logrus.Infof("Rebuild index of rootDir %s successfully, %d keys in total", c.rootDir, count)
}

// onRemove is callback when lru remove a key
// it will delete the file from disk
func (c *DiskCache) onRemove(value interface{}) {
	if v, ok := value.(*fileRecord); ok {
		fullPath := filepath.Join(c.rootDir, v.key)
		_, err := os.Stat(fullPath)
		if err != nil {
			logrus.Errorf("try to remove file %s error: %s", fullPath, err)
			return
		}
		err = os.Remove(fullPath)
		if err != nil {
			logrus.Errorf("try to remove file %s error: %s", fullPath, err)
		} else {
			logrus.Tracef("remove file %s success", fullPath)
		}
	}
}

// sizeFn is used to cal the file byte size of a hash
func (c *DiskCache) sizeFn(value interface{}) int64 {
	if v, ok := value.(*fileRecord); ok {
		return v.sizeBytes
	}
	return 0
}

// Contains only call lru
func (c *DiskCache) Contains(ctx context.Context, d *repb.Digest) (bool, error) {
	key, err := c.key(d)
	if err != nil {
		return false, err
	}
	return c.lru.Contains(key), nil
}

func (c *DiskCache) FindMissing(ctx context.Context, digests []*repb.Digest) ([]*repb.Digest, error) {
	var out []*repb.Digest
	for i := range digests {
		if exists, err := c.Contains(ctx, digests[i]); err == nil && !exists {
			out = append(out, digests[i])
		}
	}
	return out, nil
}

func (c *DiskCache) Get(ctx context.Context, d *repb.Digest) ([]byte, error) {
	key, err := c.key(d)
	if err != nil {
		return nil, err
	}
	errNotExists := status.NotFoundErrorf("key %s not exists", key)
	_, exists := c.lru.Get(key)
	if !exists {
		c.metrics.Miss()
		return nil, errNotExists
	}
	fullPath := filepath.Join(c.rootDir, key)
	content, err := c.readFileFromDisk(ctx, fullPath)
	if err != nil {
		c.lru.Remove(key)
		c.metrics.Miss()
		return nil, errNotExists
	}
	c.metrics.Hit()
	return content, nil
}

func (c *DiskCache) readFileFromDisk(ctx context.Context, fullPath string) ([]byte, error) {
	return ioutil.ReadFile(fullPath)
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
	key, err := c.key(d)
	if err != nil {
		return err // grpcError
	}
	if len(data) > c.unitSizeLimitation {
		return errByteSizeOverCutoffSize
	}
	v, exists := c.lru.Get(key)
	if exists && v.(*fileRecord).sizeBytes == int64(len(data)) {
		return nil
	}
	siz, err := disk.WriteFile(ctx, filepath.Join(c.rootDir, key), data)
	if err != nil {
		return err
	}
	if !c.lru.Add(key, &fileRecord{
		lastUseTime: time.Now().Unix(),
		key:         key,
		sizeBytes:   int64(siz),
	}) {
		return status.InternalErrorf("add key %s to lru error", key)
	}
	return nil
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
	key, err := c.key(d)
	if err != nil {
		return err
	}
	if !c.lru.Remove(key) {
		return status.InternalErrorf("remove %s fail", d.GetHash())
	}
	return nil
}

func (c *DiskCache) Reader(ctx context.Context, d *repb.Digest, offset int64) (io.ReadCloser, error) {
	key, err := c.key(d)
	if err != nil {
		return nil, err
	}
	fullPath := filepath.Join(c.rootDir, key)
	r, err := disk.FileReader(ctx, fullPath, offset, 0)
	if err != nil {
		c.lru.Remove(key) // remove it just in case
		c.metrics.Miss()
		return nil, status.NotFoundErrorf("key %s not exists", d.GetHash())
	}
	c.lru.Contains(key) // mark the file as used.
	c.metrics.Hit()
	return r, nil
}

type dbCloseFn func(totalBytesWritten int64) error

type dbWriteOnClose struct {
	io.WriteCloser
	closeFn      dbCloseFn
	bytesWritten int64
}

func (d *dbWriteOnClose) Write(data []byte) (int, error) {
	n, err := d.WriteCloser.Write(data)
	d.bytesWritten += int64(n)
	return n, err
}

func (d *dbWriteOnClose) Close() error {
	if err := d.WriteCloser.Close(); err != nil {
		return err
	}
	return d.closeFn(d.bytesWritten)
}

func (c *DiskCache) Writer(ctx context.Context, d *repb.Digest) (io.WriteCloser, error) {
	key, err := c.key(d)
	if err != nil {
		return nil, err
	}
	fullPath := filepath.Join(c.rootDir, key)
	writeCloser, err := disk.FileWriter(ctx, fullPath)
	if err != nil {
		return nil, err
	}
	return &dbWriteOnClose{
		WriteCloser: writeCloser,
		closeFn: func(totalBytesWritten int64) error {
			c.lru.Add(key, &fileRecord{
				lastUseTime: time.Now().Unix(),
				key:         key,
				sizeBytes:   totalBytesWritten,
			})
			return nil
		},
	}, nil
}

var _ interfaces.Cache = (*DiskCache)(nil)
